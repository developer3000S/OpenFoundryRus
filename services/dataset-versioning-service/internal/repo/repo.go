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
	size_bytes, row_count, owner_id, tags, current_version, active_branch,
	metadata, health_status, current_view_id,
	rid, parent_folder_rid, folder_path, project_id, project_rid, path,
	resource_visibility, deleted_at, created_at, updated_at
	FROM datasets`

const (
	defaultParentFolderRID = "ri.openfoundry.main.folder.root"
	defaultFolderPath      = "/datasets"
	defaultProjectID       = "default"
	defaultProjectRID      = "ri.openfoundry.main.project.default"
	defaultVisibility      = models.ResourceVisibilityPrivate
)

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
		rows, err = r.Pool.Query(ctx, datasetSelect+` WHERE owner_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT $2`, *ownerID, limit)
	} else {
		rows, err = r.Pool.Query(ctx, datasetSelect+` WHERE deleted_at IS NULL ORDER BY created_at DESC LIMIT $1`, limit)
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
	row := r.Pool.QueryRow(ctx, datasetSelect+` WHERE id = $1 AND deleted_at IS NULL`, id)
	v, err := scanDataset(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) GetDatasetIncludingDeleted(ctx context.Context, id uuid.UUID) (*models.Dataset, error) {
	row := r.Pool.QueryRow(ctx, datasetSelect+` WHERE id = $1`, id)
	v, err := scanDataset(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

// BuildDatasetStoragePath mirrors Rust `build_dataset_storage_path`: the
// Bronze prefix is trimmed of surrounding whitespace and slashes; if it
// ends up empty the path falls back to the literal `datasets` segment.
func BuildDatasetStoragePath(prefix string, datasetID uuid.UUID) string {
	normalized := strings.Trim(strings.TrimSpace(prefix), "/")
	if normalized == "" {
		return "datasets/" + datasetID.String()
	}
	return normalized + "/" + datasetID.String()
}

func firstNonEmpty(values ...*string) string {
	for _, ptr := range values {
		if ptr == nil {
			continue
		}
		if trimmed := strings.TrimSpace(*ptr); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeFolderPath(raw string) string {
	trimmed := strings.Trim(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return defaultFolderPath
	}
	return "/" + trimmed
}

func normalizeResourcePath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	trimmed = "/" + strings.Trim(trimmed, "/")
	if trimmed == "/" {
		return ""
	}
	return trimmed
}

func BuildDatasetResourcePath(folderPath, displayName string) string {
	base := normalizeFolderPath(folderPath)
	name := strings.TrimSpace(displayName)
	if base == "/" {
		return "/" + name
	}
	return strings.TrimRight(base, "/") + "/" + name
}

func parentPath(resourcePath string) string {
	path := normalizeResourcePath(resourcePath)
	if path == "" {
		return defaultFolderPath
	}
	idx := strings.LastIndex(path, "/")
	if idx <= 0 {
		return "/"
	}
	return path[:idx]
}

func normalizeBranchName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", fmt.Errorf("%w: branch name is required", ErrValidation)
	}
	if _, err := uuid.Parse(name); err == nil {
		return "", fmt.Errorf("%w: invalid branch name", ErrValidation)
	}
	if strings.HasPrefix(name, "ri.") {
		return "", fmt.Errorf("%w: invalid branch name", ErrValidation)
	}
	return name, nil
}

func datasetDefaultBranchName(dataset *models.Dataset) (string, error) {
	if dataset != nil && strings.TrimSpace(dataset.ActiveBranch) != "" {
		return normalizeBranchName(dataset.ActiveBranch)
	}
	return "main", nil
}

func createDatasetDefaultBranchName(body *models.CreateDatasetRequest) (string, error) {
	if body != nil {
		if name := firstNonEmpty(body.ActiveBranch, body.DefaultBranch, body.DefaultBranchName); name != "" {
			return normalizeBranchName(name)
		}
	}
	return "main", nil
}

func datasetLinks(id uuid.UUID) *models.DatasetLinks {
	encoded := id.String()
	return &models.DatasetLinks{
		Self:    "/datasets/" + encoded,
		Preview: "/datasets/" + encoded,
		Lineage: "/lineage?dataset=" + encoded,
	}
}

func hydrateDataset(v *models.Dataset) *models.Dataset {
	if v == nil {
		return nil
	}
	if v.RID == "" {
		v.RID = "ri.foundry.main.dataset." + v.ID.String()
	}
	if v.DisplayName == "" {
		v.DisplayName = v.Name
	}
	if v.ParentFolderRID == "" {
		v.ParentFolderRID = defaultParentFolderRID
	}
	if v.FolderPath == "" {
		v.FolderPath = defaultFolderPath
	}
	if v.ProjectID == "" {
		v.ProjectID = defaultProjectID
	}
	if v.ProjectRID == "" {
		v.ProjectRID = defaultProjectRID
	}
	if v.ResourceVisibility == "" {
		v.ResourceVisibility = defaultVisibility
	}
	if v.Path == "" {
		v.Path = BuildDatasetResourcePath(v.FolderPath, v.Name)
	}
	if v.Links == nil {
		v.Links = datasetLinks(v.ID)
	}
	return v
}

// CreateDataset inserts a fresh dataset row with declarative-only
// defaults (storage path under the Bronze lakehouse prefix, format
// `parquet`, health `unknown`, metadata `{}`). Runtime version rows are
// owned by the versioning pipeline, not by this insert.
func (r *Repo) CreateDataset(ctx context.Context, body *models.CreateDatasetRequest, ownerID uuid.UUID) (*models.Dataset, error) {
	id := uuid.New()
	if body.ID != nil && *body.ID != uuid.Nil {
		id = *body.ID
	}
	name := strings.TrimSpace(body.Name)
	if name == "" && body.DisplayName != nil {
		name = strings.TrimSpace(*body.DisplayName)
	}
	format := "parquet"
	if body.Format != nil && *body.Format != "" {
		format = strings.ToLower(*body.Format)
	}
	tags := body.Tags
	if tags == nil {
		tags = []string{}
	}
	description := ""
	if body.Description != nil {
		description = *body.Description
	}
	metadata := []byte(`{}`)
	if len(body.Metadata) > 0 {
		metadata = body.Metadata
	}
	health := "unknown"
	if body.HealthStatus != nil && *body.HealthStatus != "" {
		health = *body.HealthStatus
	}
	activeBranch, err := createDatasetDefaultBranchName(body)
	if err != nil {
		return nil, err
	}
	storagePath := BuildDatasetStoragePath("bronze", id)
	parentFolderRID := firstNonEmpty(body.ParentFolderRID, body.ParentFolderRid)
	if parentFolderRID == "" {
		parentFolderRID = defaultParentFolderRID
	}
	folderPath := normalizeFolderPath(firstNonEmpty(body.FolderPath))
	projectID := firstNonEmpty(body.ProjectID)
	if projectID == "" {
		projectID = defaultProjectID
	}
	projectRID := firstNonEmpty(body.ProjectRID)
	if projectRID == "" {
		projectRID = defaultProjectRID
	}
	visibility := strings.ToLower(firstNonEmpty(body.ResourceVisibility, body.Visibility))
	if visibility == "" {
		visibility = defaultVisibility
	}
	resourcePath := normalizeResourcePath(firstNonEmpty(body.Path))
	if resourcePath == "" {
		resourcePath = BuildDatasetResourcePath(folderPath, name)
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO datasets
		    (id, rid, name, description, format, storage_path, owner_id, tags,
		     active_branch, metadata, health_status, parent_folder_rid, folder_path,
		     project_id, project_rid, path, resource_visibility)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		 RETURNING id, name, description, format, storage_path, size_bytes,
		           row_count, owner_id, tags, current_version, active_branch,
		           metadata, health_status, current_view_id, rid, parent_folder_rid,
		           folder_path, project_id, project_rid, path, resource_visibility,
		           deleted_at, created_at, updated_at`,
		id, "ri.foundry.main.dataset."+id.String(), name, description,
		format, storagePath, ownerID, tags, activeBranch, metadata, health, parentFolderRID,
		folderPath, projectID, projectRID, resourcePath, visibility,
	)
	return scanDataset(row)
}

// UpdateDataset mirrors Rust's PATCH semantics while keeping dataset resource
// placement fields consistent across rename, move and explicit path updates.
func (r *Repo) UpdateDataset(ctx context.Context, id uuid.UUID, body *models.UpdateDatasetRequest) (*models.Dataset, error) {
	current, err := r.GetDataset(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, nil
	}
	name := current.Name
	if body.Name != nil {
		name = strings.TrimSpace(*body.Name)
	} else if body.DisplayName != nil {
		name = strings.TrimSpace(*body.DisplayName)
	}
	parentFolderRID := current.ParentFolderRID
	if next := firstNonEmpty(body.ParentFolderRID, body.ParentFolderRid); next != "" {
		parentFolderRID = next
	}
	folderPath := current.FolderPath
	if body.FolderPath != nil {
		folderPath = normalizeFolderPath(*body.FolderPath)
	}
	projectID := current.ProjectID
	if next := firstNonEmpty(body.ProjectID); next != "" {
		projectID = next
	}
	projectRID := current.ProjectRID
	if next := firstNonEmpty(body.ProjectRID); next != "" {
		projectRID = next
	}
	visibility := current.ResourceVisibility
	if next := firstNonEmpty(body.ResourceVisibility, body.Visibility); next != "" {
		visibility = strings.ToLower(next)
	}
	resourcePath := current.Path
	if body.Path != nil {
		if normalized := normalizeResourcePath(*body.Path); normalized != "" {
			resourcePath = normalized
			folderPath = parentPath(normalized)
		}
	} else if body.Name != nil || body.DisplayName != nil || body.FolderPath != nil {
		resourcePath = BuildDatasetResourcePath(folderPath, name)
	}
	row := r.Pool.QueryRow(ctx,
		`UPDATE datasets SET
		    name           = $2,
		    description    = COALESCE($3, description),
		    tags           = COALESCE($4, tags),
		    owner_id       = COALESCE($5, owner_id),
		    metadata       = COALESCE($6, metadata),
		    health_status  = COALESCE($7, health_status),
		    current_view_id = COALESCE($8, current_view_id),
		    parent_folder_rid = $9,
		    folder_path = $10,
		    project_id = $11,
		    project_rid = $12,
		    path = $13,
		    resource_visibility = $14,
		    updated_at     = NOW()
		  WHERE id = $1 AND deleted_at IS NULL
		  RETURNING id, name, description, format, storage_path, size_bytes,
		            row_count, owner_id, tags, current_version, active_branch,
		            metadata, health_status, current_view_id, rid, parent_folder_rid,
		            folder_path, project_id, project_rid, path, resource_visibility,
		            deleted_at, created_at, updated_at`,
		id, name, body.Description, body.Tags, body.OwnerID,
		jsonOrNil(body.Metadata), body.HealthStatus, body.CurrentViewID,
		parentFolderRID, folderPath, projectID, projectRID, resourcePath, visibility,
	)
	v, err := scanDataset(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func jsonOrNil(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

func (r *Repo) DeleteDataset(ctx context.Context, id uuid.UUID) (bool, error) {
	cmd, err := r.Pool.Exec(ctx, `UPDATE datasets SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

func (r *Repo) RestoreDataset(ctx context.Context, id uuid.UUID) (*models.Dataset, error) {
	row := r.Pool.QueryRow(ctx, `UPDATE datasets SET deleted_at = NULL, updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, description, format, storage_path, size_bytes,
		          row_count, owner_id, tags, current_version, active_branch,
		          metadata, health_status, current_view_id, rid, parent_folder_rid,
		          folder_path, project_id, project_rid, path, resource_visibility,
		          deleted_at, created_at, updated_at`, id)
	v, err := scanDataset(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) HardDeleteDataset(ctx context.Context, id uuid.UUID) (bool, error) {
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
		&v.ActiveBranch, &v.Metadata, &v.HealthStatus, &v.CurrentViewID,
		&v.RID, &v.ParentFolderRID, &v.FolderPath, &v.ProjectID, &v.ProjectRID,
		&v.Path, &v.ResourceVisibility, &v.DeletedAt,
		&v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	if v.Tags == nil {
		v.Tags = []string{}
	}
	if len(v.Metadata) == 0 {
		v.Metadata = []byte(`{}`)
	}
	return hydrateDataset(v), nil
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
	row := r.Pool.QueryRow(ctx, datasetSelect+` WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL`, id, ownerID)
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
	if err := r.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dataset_branches WHERE dataset_id = $1 AND deleted_at IS NULL AND archived_at IS NULL)`, dataset.ID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	branchName, err := datasetDefaultBranchName(dataset)
	if err != nil {
		return err
	}
	_, err = r.Pool.Exec(ctx,
		`INSERT INTO dataset_branches (id, dataset_id, dataset_rid, name, version, base_version, description, is_default)
		 VALUES ($1, $2, 'ri.foundry.main.dataset.' || $2::text, $3, $4, $4, 'Default branch', TRUE)`,
		uuid.New(), dataset.ID, branchName, dataset.CurrentVersion,
	)
	if IsConflict(err) {
		return nil
	}
	return err
}

func (r *Repo) ListBranches(ctx context.Context, datasetID uuid.UUID) ([]models.DatasetBranch, error) {
	rows, err := r.Pool.Query(ctx, branchSelect+` WHERE dataset_id = $1 AND deleted_at IS NULL AND archived_at IS NULL ORDER BY is_default DESC, name ASC`, datasetID)
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
	name, err := normalizeBranchName(name)
	if err != nil {
		return nil, err
	}
	row := r.Pool.QueryRow(ctx, branchSelect+` WHERE dataset_id = $1 AND name = $2 AND deleted_at IS NULL AND archived_at IS NULL`, datasetID, name)
	v, err := scanBranch(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) CreateBranch(ctx context.Context, dataset *models.Dataset, body *models.CreateDatasetBranchRequest) (*models.DatasetBranch, error) {
	name, err := normalizeBranchName(body.Name)
	if err != nil {
		return nil, err
	}
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
		uuid.New(), dataset.ID, name, sourceVersion, body.Description,
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

const fileSelect = `SELECT df.id, df.dataset_id, df.transaction_id,
	'ri.foundry.main.transaction.' || df.transaction_id::text AS transaction_rid,
	df.logical_path, df.physical_uri, df.size_bytes, df.media_type, df.sha256,
	df.row_count_hint, df.storage_location, df.created_at,
	COALESCE(t.committed_at, t.started_at, df.created_at) AS modified_at,
	df.deleted_at,
	CASE WHEN df.deleted_at IS NULL THEN 'active' ELSE 'deleted' END AS status
	FROM dataset_files df
	LEFT JOIN dataset_transactions t ON t.id = df.transaction_id`

// ListFiles returns the branch-effective logical file projection. Physical
// dataset_files rows are retained for audit and storage retention, so latest
// file listings must replay committed transactions instead of reading every
// undeleted physical row directly.
func (r *Repo) ListFiles(ctx context.Context, datasetID uuid.UUID, branch string, prefix string) ([]models.DatasetFile, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "main"
	}
	prefix = strings.TrimLeft(prefix, "/")

	view, err := r.computeViewAt(ctx, datasetID, branch, viewCutoff{})
	if err != nil {
		return nil, err
	}
	out := make([]models.DatasetFile, 0, len(view.Files))
	for _, file := range view.Files {
		logicalPath := strings.TrimLeft(file.LogicalPath, "/")
		if prefix != "" && !strings.HasPrefix(logicalPath, prefix) {
			continue
		}
		v, err := r.fileForViewRef(ctx, datasetID, file)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LogicalPath < out[j].LogicalPath })
	return out, nil
}

func (r *Repo) fileForViewRef(ctx context.Context, datasetID uuid.UUID, ref models.RuntimeViewFile) (*models.DatasetFile, error) {
	if ref.IntroducedBy != nil && *ref.IntroducedBy != uuid.Nil {
		row := r.Pool.QueryRow(ctx, fileSelect+`
			WHERE df.dataset_id = $1
			  AND df.transaction_id = $2
			  AND df.logical_path = $3
			  AND df.deleted_at IS NULL
			ORDER BY df.created_at DESC
			LIMIT 1`, datasetID, *ref.IntroducedBy, ref.LogicalPath)
		v, err := scanFile(row)
		if err == nil {
			return v, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	return syntheticDatasetFileFromViewRef(datasetID, ref), nil
}

func syntheticDatasetFileFromViewRef(datasetID uuid.UUID, ref models.RuntimeViewFile) *models.DatasetFile {
	transactionID := uuid.Nil
	transactionRID := ""
	if ref.IntroducedBy != nil {
		transactionID = *ref.IntroducedBy
		transactionRID = "ri.foundry.main.transaction." + ref.IntroducedBy.String()
	}
	now := time.Now().UTC()
	stableID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(datasetID.String()+"\x00"+transactionID.String()+"\x00"+ref.LogicalPath+"\x00"+ref.PhysicalPath))
	return &models.DatasetFile{
		ID:              stableID,
		DatasetID:       datasetID,
		TransactionID:   transactionID,
		TransactionRID:  transactionRID,
		LogicalPath:     ref.LogicalPath,
		Path:            ref.LogicalPath,
		PhysicalURI:     ref.PhysicalPath,
		SizeBytes:       ref.SizeBytes,
		StorageLocation: []byte(`{}`),
		CreatedAt:       now,
		ModifiedAt:      now,
		UpdatedTime:     now,
		Status:          "active",
	}
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
	if err := r.Scan(&v.ID, &v.DatasetID, &v.TransactionID, &v.TransactionRID,
		&v.LogicalPath, &v.PhysicalURI, &v.SizeBytes, &v.MediaType, &v.SHA256,
		&v.RowCountHint, &v.StorageLocation, &v.CreatedAt, &v.ModifiedAt,
		&v.DeletedAt, &v.Status); err != nil {
		return nil, err
	}
	v.Path = v.LogicalPath
	v.ContentType = v.MediaType
	v.UpdatedTime = v.ModifiedAt
	if len(v.StorageLocation) == 0 {
		v.StorageLocation = []byte(`{}`)
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
