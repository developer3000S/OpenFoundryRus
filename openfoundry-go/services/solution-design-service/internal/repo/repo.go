package repo

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/services/solution-design-service/internal/models"
)

type Repo struct {
	Pool *pgxpool.Pool
}

func (r *Repo) ListPrimary(ctx context.Context) ([]models.PrimaryItem, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, payload, created_at FROM solution_diagrams ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.PrimaryItem, 0)
	for rows.Next() {
		var p models.PrimaryItem
		if err := rows.Scan(&p.ID, &p.Payload, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repo) CreatePrimary(ctx context.Context, payload []byte) (models.PrimaryItem, error) {
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO solution_diagrams (id, payload) VALUES ($1, $2) RETURNING id, payload, created_at`,
		uuid.New(), payload)
	var p models.PrimaryItem
	if err := row.Scan(&p.ID, &p.Payload, &p.CreatedAt); err != nil {
		return p, err
	}
	return p, nil
}

func (r *Repo) GetPrimary(ctx context.Context, id uuid.UUID) (*models.PrimaryItem, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT id, payload, created_at FROM solution_diagrams WHERE id = $1`, id)
	var p models.PrimaryItem
	err := row.Scan(&p.ID, &p.Payload, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repo) ListSecondary(ctx context.Context, parentID uuid.UUID) ([]models.SecondaryItem, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, parent_id, payload, created_at
           FROM solution_references
          WHERE parent_id = $1
          ORDER BY created_at DESC LIMIT 200`, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.SecondaryItem, 0)
	for rows.Next() {
		var s models.SecondaryItem
		if err := rows.Scan(&s.ID, &s.ParentID, &s.Payload, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repo) CreateSecondary(ctx context.Context, parentID uuid.UUID, payload []byte) (models.SecondaryItem, error) {
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO solution_references (id, parent_id, payload)
           VALUES ($1, $2, $3) RETURNING id, parent_id, payload, created_at`,
		uuid.New(), parentID, payload)
	var s models.SecondaryItem
	if err := row.Scan(&s.ID, &s.ParentID, &s.Payload, &s.CreatedAt); err != nil {
		return s, err
	}
	return s, nil
}
