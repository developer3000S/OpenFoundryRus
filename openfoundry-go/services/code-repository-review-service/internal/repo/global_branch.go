package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/models"
)

// PgUniqueViolation is the SQLSTATE for unique_violation.
const PgUniqueViolation = "23505"

// IsUniqueViolation tells handlers when to return 409 Conflict.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == PgUniqueViolation
}

// GlobalBranchRepo is a thin wrapper around the Postgres pool.
type GlobalBranchRepo struct {
	Pool *pgxpool.Pool
}

func (r *GlobalBranchRepo) CreateBranch(ctx context.Context, req models.CreateGlobalBranchRequest, createdBy string) (models.GlobalBranch, error) {
	desc := ""
	if req.Description != nil {
		desc = *req.Description
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO global_branches (id, name, description, parent_global_branch, created_by)
              VALUES ($1, $2, $3, $4, $5)
            RETURNING id, rid, name, parent_global_branch, description,
                      created_by, created_at, archived_at`,
		uuid.New(), req.Name, desc, req.ParentGlobalBranch, createdBy,
	)
	return scanBranch(row)
}

func (r *GlobalBranchRepo) GetBranch(ctx context.Context, id uuid.UUID) (*models.GlobalBranch, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT id, rid, name, parent_global_branch, description,
                      created_by, created_at, archived_at
               FROM global_branches WHERE id = $1`,
		id,
	)
	b, err := scanBranch(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *GlobalBranchRepo) ListBranches(ctx context.Context) ([]models.GlobalBranch, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, rid, name, parent_global_branch, description,
                      created_by, created_at, archived_at
               FROM global_branches
              ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.GlobalBranch, 0)
	for rows.Next() {
		b, err := scanBranch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *GlobalBranchRepo) AddLink(ctx context.Context, globalBranchID uuid.UUID, req models.CreateGlobalBranchLinkRequest) (models.GlobalBranchLink, error) {
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO global_branch_resource_links
                  (global_branch_id, resource_type, resource_rid, branch_rid, status, last_synced_at)
                VALUES ($1, $2, $3, $4, 'in_sync', NOW())
                ON CONFLICT (global_branch_id, resource_type, resource_rid)
                DO UPDATE SET branch_rid = EXCLUDED.branch_rid,
                              status     = 'in_sync',
                              last_synced_at = EXCLUDED.last_synced_at
                RETURNING global_branch_id, resource_type, resource_rid,
                          branch_rid, status, last_synced_at`,
		globalBranchID, req.ResourceType, req.ResourceRID, req.BranchRID,
	)
	var l models.GlobalBranchLink
	err := row.Scan(&l.GlobalBranchID, &l.ResourceType, &l.ResourceRID, &l.BranchRID, &l.Status, &l.LastSyncedAt)
	return l, err
}

func (r *GlobalBranchRepo) ListLinks(ctx context.Context, globalBranchID uuid.UUID) ([]models.GlobalBranchLink, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT global_branch_id, resource_type, resource_rid,
                      branch_rid, status, last_synced_at
                 FROM global_branch_resource_links
                WHERE global_branch_id = $1
                ORDER BY resource_type, resource_rid`,
		globalBranchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.GlobalBranchLink, 0)
	for rows.Next() {
		var l models.GlobalBranchLink
		if err := rows.Scan(&l.GlobalBranchID, &l.ResourceType, &l.ResourceRID, &l.BranchRID, &l.Status, &l.LastSyncedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// LinkCounts returns the (total, drifted, archived) tuple consumed by
// the GlobalBranchSummary.
func (r *GlobalBranchRepo) LinkCounts(ctx context.Context, globalBranchID uuid.UUID) (total, drifted, archived int64, err error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT
                COUNT(*) AS total,
                COUNT(*) FILTER (WHERE status = 'drifted')  AS drifted,
                COUNT(*) FILTER (WHERE status = 'archived') AS archived
              FROM global_branch_resource_links
              WHERE global_branch_id = $1`,
		globalBranchID,
	)
	if scanErr := row.Scan(&total, &drifted, &archived); scanErr != nil {
		return 0, 0, 0, fmt.Errorf("link counts: %w", scanErr)
	}
	return total, drifted, archived, nil
}

// UpdateLinksForBranch flips status for every link tied to the given
// per-plane branch RID. Returns rows affected. Used by the Kafka
// subscriber when a plane reports a branch event.
func (r *GlobalBranchRepo) UpdateLinksForBranch(ctx context.Context, branchRID, status string) (int64, error) {
	tag, err := r.Pool.Exec(ctx,
		`UPDATE global_branch_resource_links
                SET status = $2, last_synced_at = NOW()
              WHERE branch_rid = $1`,
		branchRID, status,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// scanBranch is shared between QueryRow and Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanBranch(s rowScanner) (models.GlobalBranch, error) {
	var b models.GlobalBranch
	err := s.Scan(&b.ID, &b.RID, &b.Name, &b.ParentGlobalBranch, &b.Description, &b.CreatedBy, &b.CreatedAt, &b.ArchivedAt)
	return b, err
}
