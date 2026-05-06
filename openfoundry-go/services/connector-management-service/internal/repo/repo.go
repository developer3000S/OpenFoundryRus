// Package repo holds SQL queries + embedded migrations for
// connector-management-service.
package repo

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

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

type Repo struct{ Pool *pgxpool.Pool }

const connectionSelect = `SELECT id, name, connector_type, config, status,
	owner_id, last_sync_at, created_at, updated_at FROM connections`

func (r *Repo) ListConnections(ctx context.Context, ownerID *uuid.UUID) ([]models.Connection, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if ownerID != nil {
		rows, err = r.Pool.Query(ctx, connectionSelect+` WHERE owner_id = $1 ORDER BY created_at DESC LIMIT 500`, *ownerID)
	} else {
		rows, err = r.Pool.Query(ctx, connectionSelect+` ORDER BY created_at DESC LIMIT 500`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Connection, 0)
	for rows.Next() {
		v, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) GetConnection(ctx context.Context, id uuid.UUID) (*models.Connection, error) {
	row := r.Pool.QueryRow(ctx, connectionSelect+` WHERE id = $1`, id)
	v, err := scanConnection(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) CreateConnection(ctx context.Context, body *models.CreateConnectionRequest, ownerID uuid.UUID) (*models.Connection, error) {
	id := uuid.New()
	cfg := body.Config
	if len(cfg) == 0 {
		cfg = []byte(`{}`)
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO connections (id, name, connector_type, config, owner_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, name, connector_type, config, status, owner_id,
		           last_sync_at, created_at, updated_at`,
		id, strings.TrimSpace(body.Name), body.ConnectorType, cfg, ownerID,
	)
	return scanConnection(row)
}

func (r *Repo) UpdateConnection(ctx context.Context, id uuid.UUID, body *models.UpdateConnectionRequest) (*models.Connection, error) {
	current, err := r.GetConnection(ctx, id)
	if err != nil || current == nil {
		return current, err
	}
	name := current.Name
	if body.Name != nil {
		name = *body.Name
	}
	cfg := current.Config
	if len(body.Config) > 0 {
		cfg = body.Config
	}
	status := current.Status
	if body.Status != nil {
		status = *body.Status
	}
	row := r.Pool.QueryRow(ctx,
		`UPDATE connections SET name = $2, config = $3, status = $4, updated_at = $5
		 WHERE id = $1
		 RETURNING id, name, connector_type, config, status, owner_id,
		           last_sync_at, created_at, updated_at`,
		id, name, cfg, status, time.Now().UTC(),
	)
	return scanConnection(row)
}

func (r *Repo) DeleteConnection(ctx context.Context, id uuid.UUID) (bool, error) {
	cmd, err := r.Pool.Exec(ctx, `DELETE FROM connections WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

type rowLikeT interface{ Scan(...any) error }

func scanConnection(r rowLikeT) (*models.Connection, error) {
	v := &models.Connection{}
	if err := r.Scan(&v.ID, &v.Name, &v.ConnectorType, &v.Config, &v.Status,
		&v.OwnerID, &v.LastSyncAt, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	return v, nil
}
