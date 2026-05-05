// Package repo holds SQL queries + embedded migrations for sdk-generation-service.
package repo

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/services/sdk-generation-service/internal/models"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies every embedded `migrations/*.sql` file in lex order.
// Idempotent (CREATE TABLE IF NOT EXISTS).
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

// Repo wraps the SQL surface for jobs + publications.
type Repo struct{ Pool *pgxpool.Pool }

// ─── Jobs ───────────────────────────────────────────────────────────────

// ListJobs returns the most recent jobs (limit 200), matching the Rust impl.
func (r *Repo) ListJobs(ctx context.Context) ([]models.Job, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, payload, created_at FROM sdk_generation_jobs
		 ORDER BY created_at DESC LIMIT 200`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Job, 0)
	for rows.Next() {
		var j models.Job
		if err := rows.Scan(&j.ID, &j.Payload, &j.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// CreateJob inserts a new job and returns the persisted row.
func (r *Repo) CreateJob(ctx context.Context, id uuid.UUID, payload json.RawMessage) (*models.Job, error) {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO sdk_generation_jobs (id, payload) VALUES ($1, $2)
		 RETURNING id, payload, created_at`,
		id, payload,
	)
	j := &models.Job{}
	if err := row.Scan(&j.ID, &j.Payload, &j.CreatedAt); err != nil {
		return nil, err
	}
	return j, nil
}

// GetJob looks up a job by id. Returns nil, nil when not found.
func (r *Repo) GetJob(ctx context.Context, id uuid.UUID) (*models.Job, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT id, payload, created_at FROM sdk_generation_jobs WHERE id = $1`,
		id,
	)
	j := &models.Job{}
	if err := row.Scan(&j.ID, &j.Payload, &j.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return j, nil
}

// ─── Publications ───────────────────────────────────────────────────────

// ListPublications returns the most recent publications for a job (limit 200).
func (r *Repo) ListPublications(ctx context.Context, parentID uuid.UUID) ([]models.Publication, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, parent_id, payload, created_at FROM sdk_generation_publications
		 WHERE parent_id = $1 ORDER BY created_at DESC LIMIT 200`,
		parentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Publication, 0)
	for rows.Next() {
		var p models.Publication
		if err := rows.Scan(&p.ID, &p.ParentID, &p.Payload, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CreatePublication inserts a new publication for `parentID`.
func (r *Repo) CreatePublication(ctx context.Context, id, parentID uuid.UUID, payload json.RawMessage) (*models.Publication, error) {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO sdk_generation_publications (id, parent_id, payload) VALUES ($1, $2, $3)
		 RETURNING id, parent_id, payload, created_at`,
		id, parentID, payload,
	)
	p := &models.Publication{}
	if err := row.Scan(&p.ID, &p.ParentID, &p.Payload, &p.CreatedAt); err != nil {
		return nil, err
	}
	return p, nil
}
