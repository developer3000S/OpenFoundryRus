// Package repo holds SQL queries + embedded migrations for
// dataset-versioning-service.
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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies every embedded migration in lex order. Idempotent.
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

// DB is the pgx subset used by Repo; pgxpool.Pool and pgxmock pools both satisfy it.
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Repo wraps the SQL surface for datasets, versions, branches and files.
type Repo struct{ Pool DB }

const datasetSelect = `SELECT id, name, description, format, storage_path,
	size_bytes, row_count, owner_id, tags, current_version, created_at, updated_at
	FROM datasets`

func (r *Repo) ListDatasets(ctx context.Context, ownerID *uuid.UUID, limit int) ([]models.Dataset, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	var (
		rows pgx.Rows
		err  error
	)
	if ownerID != nil {
		rows, err = r.Pool.Query(ctx, datasetSelect+` WHERE owner_id = $1 ORDER BY updated_at DESC LIMIT $2`, *ownerID, limit)
	} else {
		rows, err = r.Pool.Query(ctx, datasetSelect+` ORDER BY updated_at DESC LIMIT $1`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.Dataset, 0)
	for rows.Next() {
		v, err := scanDataset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) GetDataset(ctx context.Context, id uuid.UUID) (*models.Dataset, error) {
	row := r.Pool.QueryRow(ctx, datasetSelect+` WHERE id = $1`, id)
	v, err := scanDataset(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) CreateDataset(ctx context.Context, body *models.CreateDatasetRequest, ownerID uuid.UUID) (*models.Dataset, error) {
	id := uuid.New()
	format := "parquet"
	if body.Format != nil && *body.Format != "" {
		format = *body.Format
	}
	tags := body.Tags
	if tags == nil {
		tags = []string{}
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO datasets
		    (id, rid, name, description, format, storage_path, owner_id, tags)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, name, description, format, storage_path, size_bytes,
		           row_count, owner_id, tags, current_version, created_at, updated_at`,
		id, "ri.foundry.main.dataset."+id.String(), strings.TrimSpace(body.Name), body.Description, format,
		strings.TrimSpace(body.StoragePath), ownerID, tags,
	)
	return scanDataset(row)
}

func (r *Repo) UpdateDataset(ctx context.Context, id uuid.UUID, body *models.UpdateDatasetRequest) (*models.Dataset, error) {
	current, err := r.GetDataset(ctx, id)
	if err != nil || current == nil {
		return current, err
	}
	desc := current.Description
	if body.Description != nil {
		desc = *body.Description
	}
	tags := current.Tags
	if body.Tags != nil {
		tags = body.Tags
	}
	size := current.SizeBytes
	if body.SizeBytes != nil {
		size = *body.SizeBytes
	}
	rowCount := current.RowCount
	if body.RowCount != nil {
		rowCount = *body.RowCount
	}
	row := r.Pool.QueryRow(ctx,
		`UPDATE datasets SET
		    description = $2, tags = $3, size_bytes = $4, row_count = $5,
		    updated_at = $6
		  WHERE id = $1
		  RETURNING id, name, description, format, storage_path, size_bytes,
		            row_count, owner_id, tags, current_version, created_at, updated_at`,
		id, desc, tags, size, rowCount, time.Now().UTC(),
	)
	return scanDataset(row)
}

func (r *Repo) DeleteDataset(ctx context.Context, id uuid.UUID) (bool, error) {
	cmd, err := r.Pool.Exec(ctx, `DELETE FROM datasets WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

type rowLikeT interface{ Scan(...any) error }

func scanDataset(r rowLikeT) (*models.Dataset, error) {
	v := &models.Dataset{}
	if err := r.Scan(&v.ID, &v.Name, &v.Description, &v.Format, &v.StoragePath,
		&v.SizeBytes, &v.RowCount, &v.OwnerID, &v.Tags, &v.CurrentVersion,
		&v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	if v.Tags == nil {
		v.Tags = []string{}
	}
	return v, nil
}

const versionSelect = `SELECT id, dataset_id, version, message, size_bytes,
	row_count, storage_path, transaction_id, created_at FROM dataset_versions`

// ErrConflict reports a unique-key conflict that should map to HTTP 409.
var ErrConflict = errors.New("conflict")

func IsConflict(err error) bool {
	if errors.Is(err, ErrConflict) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (r *Repo) GetDatasetForOwner(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.Dataset, error) {
	row := r.Pool.QueryRow(ctx, datasetSelect+` WHERE id = $1 AND owner_id = $2`, id, ownerID)
	v, err := scanDataset(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) ListVersions(ctx context.Context, datasetID uuid.UUID) ([]models.DatasetVersion, error) {
	rows, err := r.Pool.Query(ctx, versionSelect+` WHERE dataset_id = $1 ORDER BY version DESC`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.DatasetVersion, 0)
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) GetVersion(ctx context.Context, datasetID uuid.UUID, version int32) (*models.DatasetVersion, error) {
	row := r.Pool.QueryRow(ctx, versionSelect+` WHERE dataset_id = $1 AND version = $2`, datasetID, version)
	v, err := scanVersion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) CreateVersion(ctx context.Context, datasetID uuid.UUID, body *models.CreateDatasetVersionRequest) (*models.DatasetVersion, error) {
	version := int32(0)
	if body.Version != nil {
		version = *body.Version
	} else {
		err := r.Pool.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) + 1 FROM dataset_versions WHERE dataset_id = $1`, datasetID).Scan(&version)
		if err != nil {
			return nil, err
		}
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO dataset_versions
		    (id, dataset_id, version, message, size_bytes, row_count, storage_path, transaction_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, dataset_id, version, message, size_bytes, row_count, storage_path, transaction_id, created_at`,
		uuid.New(), datasetID, version, body.Message, body.SizeBytes, body.RowCount,
		strings.TrimSpace(body.StoragePath), body.TransactionID,
	)
	v, err := scanVersion(row)
	if IsConflict(err) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	_, err = r.Pool.Exec(ctx, `UPDATE datasets SET current_version = GREATEST(current_version, $2), updated_at = $3 WHERE id = $1`, datasetID, version, time.Now().UTC())
	return v, err
}

const branchSelect = `SELECT id,
	COALESCE(rid, 'ri.foundry.main.branch.' || id::text) AS rid,
	dataset_id,
	COALESCE(dataset_rid, 'ri.foundry.main.dataset.' || dataset_id::text) AS dataset_rid,
	name, parent_branch_id, head_transaction_id, created_from_transaction_id,
	last_activity_at, labels, fallback_chain, version, base_version,
	description, is_default, created_at, updated_at FROM dataset_branches`

func (r *Repo) EnsureDefaultBranch(ctx context.Context, dataset *models.Dataset) error {
	var exists bool
	if err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dataset_branches WHERE dataset_id = $1)`, dataset.ID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err := r.Pool.Exec(ctx,
		`INSERT INTO dataset_branches (id, dataset_id, dataset_rid, name, version, base_version, description, is_default)
		 VALUES ($1, $2, 'ri.foundry.main.dataset.' || $2::text, 'main', $3, $3, 'Default branch', TRUE)`,
		uuid.New(), dataset.ID, dataset.CurrentVersion,
	)
	if IsConflict(err) {
		return nil
	}
	return err
}

func (r *Repo) ListBranches(ctx context.Context, datasetID uuid.UUID) ([]models.DatasetBranch, error) {
	rows, err := r.Pool.Query(ctx, branchSelect+` WHERE dataset_id = $1 ORDER BY is_default DESC, name ASC`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.DatasetBranch, 0)
	for rows.Next() {
		v, err := scanBranch(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) GetBranch(ctx context.Context, datasetID uuid.UUID, name string) (*models.DatasetBranch, error) {
	row := r.Pool.QueryRow(ctx, branchSelect+` WHERE dataset_id = $1 AND name = $2`, datasetID, name)
	v, err := scanBranch(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) CreateBranch(ctx context.Context, dataset *models.Dataset, body *models.CreateDatasetBranchRequest) (*models.DatasetBranch, error) {
	sourceVersion := dataset.CurrentVersion
	if body.SourceVersion != nil {
		sourceVersion = *body.SourceVersion
	}
	if sourceVersion != dataset.CurrentVersion {
		var exists bool
		if err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dataset_versions WHERE dataset_id = $1 AND version = $2)`, dataset.ID, sourceVersion).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("source version does not exist")
		}
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO dataset_branches (id, dataset_id, dataset_rid, name, version, base_version, description, is_default)
		 VALUES ($1, $2, 'ri.foundry.main.dataset.' || $2::text, $3, $4, $4, $5, FALSE)
		 RETURNING id, COALESCE(rid, 'ri.foundry.main.branch.' || id::text), dataset_id,
		           COALESCE(dataset_rid, 'ri.foundry.main.dataset.' || dataset_id::text), name,
		           parent_branch_id, head_transaction_id, created_from_transaction_id,
		           last_activity_at, labels, fallback_chain, version, base_version,
		           description, is_default, created_at, updated_at`,
		uuid.New(), dataset.ID, strings.TrimSpace(body.Name), sourceVersion, body.Description,
	)
	v, err := scanBranch(row)
	if IsConflict(err) {
		return nil, ErrConflict
	}
	return v, err
}

func scanVersion(r rowLikeT) (*models.DatasetVersion, error) {
	v := &models.DatasetVersion{}
	if err := r.Scan(&v.ID, &v.DatasetID, &v.Version, &v.Message, &v.SizeBytes,
		&v.RowCount, &v.StoragePath, &v.TransactionID, &v.CreatedAt); err != nil {
		return nil, err
	}
	return v, nil
}

func scanBranch(r rowLikeT) (*models.DatasetBranch, error) {
	v := &models.DatasetBranch{}
	var labels []byte
	if err := r.Scan(&v.ID, &v.RID, &v.DatasetID, &v.DatasetRID, &v.Name,
		&v.ParentBranchID, &v.HeadTransactionID, &v.CreatedFromTransactionID,
		&v.LastActivityAt, &labels, &v.FallbackChain, &v.Version, &v.BaseVersion,
		&v.Description, &v.IsDefault, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	if len(labels) == 0 {
		labels = []byte(`{}`)
	}
	v.Labels = labels
	if v.FallbackChain == nil {
		v.FallbackChain = []string{}
	}
	return v, nil
}

const fileSelect = `SELECT df.id, df.dataset_id, df.transaction_id, df.logical_path,
	df.physical_uri, df.size_bytes, df.sha256, df.created_at,
	COALESCE(t.committed_at, t.started_at, df.created_at) AS modified_at,
	df.deleted_at,
	CASE WHEN df.deleted_at IS NULL THEN 'active' ELSE 'deleted' END AS status
	FROM dataset_files df
	LEFT JOIN dataset_transactions t ON t.id = df.transaction_id`

// ListFiles returns the branch-effective dataset_files projection. The
// dataset_files table is maintained by migrations from committed transaction
// file rows; this query adds branch head cutoff and prefix filtering.
func (r *Repo) ListFiles(ctx context.Context, datasetID uuid.UUID, branch string, prefix string) ([]models.DatasetFile, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}
	prefix = strings.TrimLeft(prefix, "/")
	rows, err := r.Pool.Query(ctx, fileSelect+`
		WHERE df.dataset_id = $1
		  AND ($3 = '' OR df.logical_path LIKE $3 || '%')
		  AND (
		    NOT EXISTS (SELECT 1 FROM dataset_branches b WHERE b.dataset_id = $1 AND b.name = $2)
		    OR COALESCE(t.committed_at, t.started_at, df.created_at) <= COALESCE((
		      SELECT COALESCE(ht.committed_at, ht.started_at)
		        FROM dataset_branches b
		        JOIN dataset_transactions ht ON ht.id = b.head_transaction_id
		       WHERE b.dataset_id = $1 AND b.name = $2
		       LIMIT 1
		    ), 'infinity'::timestamptz)
		  )
		ORDER BY df.logical_path ASC, df.created_at DESC`, datasetID, branch, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.DatasetFile, 0)
	seen := map[string]struct{}{}
	for rows.Next() {
		v, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[v.LogicalPath]; ok {
			continue
		}
		seen[v.LogicalPath] = struct{}{}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) GetFile(ctx context.Context, datasetID uuid.UUID, fileID uuid.UUID) (*models.DatasetFile, error) {
	row := r.Pool.QueryRow(ctx, fileSelect+` WHERE df.dataset_id = $1 AND df.id = $2`, datasetID, fileID)
	v, err := scanFile(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func scanFile(r rowLikeT) (*models.DatasetFile, error) {
	v := &models.DatasetFile{}
	if err := r.Scan(&v.ID, &v.DatasetID, &v.TransactionID, &v.LogicalPath,
		&v.PhysicalURI, &v.SizeBytes, &v.SHA256, &v.CreatedAt, &v.ModifiedAt,
		&v.DeletedAt, &v.Status); err != nil {
		return nil, err
	}
	return v, nil
}

// GetTransactionStatus returns the status for a dataset transaction.
func (r *Repo) GetTransactionStatus(ctx context.Context, datasetID uuid.UUID, transactionID uuid.UUID) (string, bool, error) {
	var status string
	err := r.Pool.QueryRow(ctx, `SELECT status FROM dataset_transactions WHERE dataset_id = $1 AND id = $2`, datasetID, transactionID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return status, true, nil
}
