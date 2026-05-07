package handler

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/models"
)

// NotebookListRepository is the production/smoke boundary for the two
// formerly empty-envelope list routes. Production uses Postgres; explicit
// smoke mode uses the in-memory repository.
type NotebookListRepository interface {
	ListNotebooks(ctx context.Context, params ListNotebooksParams) ([]models.Notebook, int64, error)
	ListSessions(ctx context.Context, notebookID uuid.UUID) ([]models.Session, error)
}

type ListNotebooksParams struct {
	Search  string
	Page    int64
	PerPage int64
}

type PostgresNotebookListRepository struct{ Pool *pgxpool.Pool }

func (p PostgresNotebookListRepository) ListNotebooks(ctx context.Context, params ListNotebooksParams) ([]models.Notebook, int64, error) {
	pattern := "%" + params.Search + "%"
	offset := (params.Page - 1) * params.PerPage

	var total int64
	if err := p.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM notebooks WHERE name ILIKE $1`, pattern).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := p.Pool.Query(ctx, `
        SELECT id, name, description, owner_id, default_kernel, created_at, updated_at
        FROM notebooks WHERE name ILIKE $1
        ORDER BY updated_at DESC LIMIT $2 OFFSET $3`,
		pattern, params.PerPage, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	notebooks := []models.Notebook{}
	for rows.Next() {
		nb, err := scanNotebook(rows)
		if err != nil {
			return nil, 0, err
		}
		notebooks = append(notebooks, nb)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return notebooks, total, nil
}

func (p PostgresNotebookListRepository) ListSessions(ctx context.Context, notebookID uuid.UUID) ([]models.Session, error) {
	rows, err := p.Pool.Query(ctx, `
        SELECT id, notebook_id, kernel, status, started_by, created_at, last_activity
        FROM sessions WHERE notebook_id = $1
        ORDER BY created_at DESC`, notebookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := []models.Session{}
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}
