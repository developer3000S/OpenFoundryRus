// Package repo holds SQL queries + embedded migrations for
// connector-management-service.
package repo

import (
	"context"
	"embed"
	"encoding/json"
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

func (r *Repo) GetConnectionForOwner(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.Connection, error) {
	row := r.Pool.QueryRow(ctx, connectionSelect+` WHERE id = $1 AND owner_id = $2`, id, ownerID)
	v, err := scanConnection(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

const syncJobSelect = `SELECT d.id, d.source_id, d.output_dataset_id, d.file_glob, d.schedule_cron, d.created_at
	FROM batch_sync_defs d JOIN connections c ON c.id = d.source_id`

func (r *Repo) ListSyncJobs(ctx context.Context, sourceID uuid.UUID, ownerID uuid.UUID) ([]models.SyncJob, error) {
	rows, err := r.Pool.Query(ctx, syncJobSelect+` WHERE d.source_id = $1 AND c.owner_id = $2 ORDER BY d.created_at DESC`, sourceID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.SyncJob, 0)
	for rows.Next() {
		v, err := scanSyncJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) GetSyncJob(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.SyncJob, error) {
	row := r.Pool.QueryRow(ctx, syncJobSelect+` WHERE d.id = $1 AND c.owner_id = $2`, id, ownerID)
	v, err := scanSyncJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) CreateSyncJob(ctx context.Context, body *models.CreateSyncJobRequest, ownerID uuid.UUID) (*models.SyncJob, error) {
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO batch_sync_defs (id, source_id, output_dataset_id, file_glob, schedule_cron)
		 SELECT $1, c.id, $3, $4, $5 FROM connections c WHERE c.id = $2 AND c.owner_id = $6
		 RETURNING id, source_id, output_dataset_id, file_glob, schedule_cron, created_at`,
		uuid.New(), body.SourceID, body.OutputDatasetID, body.FileGlob, body.ScheduleCron, ownerID,
	)
	v, err := scanSyncJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) UpdateSyncJob(ctx context.Context, id uuid.UUID, body *models.UpdateSyncJobRequest, ownerID uuid.UUID) (*models.SyncJob, error) {
	current, err := r.GetSyncJob(ctx, id, ownerID)
	if err != nil || current == nil {
		return current, err
	}
	output := current.OutputDatasetID
	if body.OutputDatasetID != nil {
		output = *body.OutputDatasetID
	}
	fileGlob := current.FileGlob
	if body.FileGlob != nil {
		fileGlob = body.FileGlob
	}
	schedule := current.ScheduleCron
	if body.ScheduleCron != nil {
		schedule = body.ScheduleCron
	}
	row := r.Pool.QueryRow(ctx,
		`UPDATE batch_sync_defs d SET output_dataset_id = $2, file_glob = $3, schedule_cron = $4
		  FROM connections c WHERE d.source_id = c.id AND d.id = $1 AND c.owner_id = $5
		  RETURNING d.id, d.source_id, d.output_dataset_id, d.file_glob, d.schedule_cron, d.created_at`,
		id, output, fileGlob, schedule, ownerID,
	)
	return scanSyncJob(row)
}

func (r *Repo) RunSyncJob(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.SyncRun, error) {
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO sync_runs (id, sync_def_id, status)
		 SELECT $1, d.id, 'running' FROM batch_sync_defs d JOIN connections c ON c.id = d.source_id
		 WHERE d.id = $2 AND c.owner_id = $3
		 RETURNING id, sync_def_id, status, started_at, finished_at, bytes_written, files_written, error,
		           ingest_job_id, dataset_version_id, content_hash`,
		uuid.New(), id, ownerID,
	)
	v, err := scanSyncRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func scanSyncJob(r rowLikeT) (*models.SyncJob, error) {
	v := &models.SyncJob{}
	if err := r.Scan(&v.ID, &v.SourceID, &v.OutputDatasetID, &v.FileGlob, &v.ScheduleCron, &v.CreatedAt); err != nil {
		return nil, err
	}
	return v, nil
}

func scanSyncRun(r rowLikeT) (*models.SyncRun, error) {
	v := &models.SyncRun{}
	if err := r.Scan(&v.ID, &v.SyncDefID, &v.Status, &v.StartedAt, &v.FinishedAt, &v.BytesWritten,
		&v.FilesWritten, &v.Error, &v.IngestJobID, &v.DatasetVersionID, &v.ContentHash); err != nil {
		return nil, err
	}
	return v, nil
}

var validVirtualProviders = map[string]bool{
	"AMAZON_S3": true, "AZURE_ABFS": true, "BIGQUERY": true, "DATABRICKS": true,
	"FOUNDRY_ICEBERG": true, "GCS": true, "SNOWFLAKE": true,
}

var validVirtualTableTypes = map[string]bool{
	"TABLE": true, "VIEW": true, "MATERIALIZED_VIEW": true, "EXTERNAL_DELTA": true,
	"MANAGED_DELTA": true, "MANAGED_ICEBERG": true, "PARQUET_FILES": true,
	"AVRO_FILES": true, "CSV_FILES": true, "OTHER": true,
}

var ErrConflict = errors.New("conflict")

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}

func (r *Repo) EnableVirtualTableSource(ctx context.Context, sourceRID string, body *models.EnableVirtualTableSourceRequest) (*models.VirtualTableSourceLink, error) {
	provider := strings.TrimSpace(body.Provider)
	if !validVirtualProviders[provider] {
		return nil, fmt.Errorf("invalid provider: %s", body.Provider)
	}
	cfg := body.IcebergCatalogConfig
	if len(cfg) == 0 {
		cfg = []byte(`null`)
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO virtual_table_sources_link (source_rid, provider, virtual_tables_enabled, iceberg_catalog_kind, iceberg_catalog_config)
		 VALUES ($1, $2, TRUE, $3, $4)
		 ON CONFLICT (source_rid) DO UPDATE SET virtual_tables_enabled = TRUE, provider = EXCLUDED.provider,
		     iceberg_catalog_kind = EXCLUDED.iceberg_catalog_kind, iceberg_catalog_config = EXCLUDED.iceberg_catalog_config,
		     updated_at = NOW()
		 RETURNING source_rid, provider, virtual_tables_enabled, code_imports_enabled, export_controls,
		           auto_register_project_rid, auto_register_enabled, auto_register_interval_seconds,
		           auto_register_tag_filters, iceberg_catalog_kind, iceberg_catalog_config, created_at, updated_at`,
		sourceRID, provider, body.IcebergCatalogKind, cfg,
	)
	return scanVirtualTableSourceLink(row)
}

func (r *Repo) CreateVirtualTable(ctx context.Context, sourceRID string, actorID string, body *models.CreateVirtualTableRequest) (*models.VirtualTable, error) {
	if !validVirtualTableTypes[body.TableType] {
		return nil, fmt.Errorf("invalid table_type: %s", body.TableType)
	}
	name := ""
	if body.Name != nil {
		name = strings.TrimSpace(*body.Name)
	}
	if name == "" {
		name = body.Locator.DefaultDisplayName()
	}
	if name == "" || strings.TrimSpace(body.ProjectRID) == "" {
		return nil, fmt.Errorf("project_rid and name/locator are required")
	}
	locator, err := body.Locator.CanonicalJSON()
	if err != nil {
		return nil, err
	}
	actor := actorID
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO virtual_tables (id, source_rid, project_rid, name, parent_folder_rid, locator, table_type,
		     schema_inferred, capabilities, markings, properties, created_by)
		 SELECT $1, l.source_rid, $3, $4, $5, $6::jsonb, $7, '[]'::jsonb, '{}'::jsonb, $8, '{}'::jsonb, $9
		 FROM virtual_table_sources_link l WHERE l.source_rid = $2 AND l.virtual_tables_enabled
		 RETURNING id, rid, source_rid, project_rid, name, parent_folder_rid, locator, table_type,
		           schema_inferred, capabilities, update_detection_enabled, update_detection_interval_seconds,
		           last_observed_version, last_polled_at,
		           COALESCE(update_detection_consecutive_failures, 0), update_detection_next_poll_at,
		           markings, properties, created_by, created_at, updated_at`,
		uuid.New(), sourceRID, strings.TrimSpace(body.ProjectRID), name, body.ParentFolderRID,
		locator, body.TableType, body.Markings, actor,
	)
	v, err := scanVirtualTable(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	return v, err
}

func (r *Repo) ListVirtualTables(ctx context.Context, ownerID string, project, source string, limit int) ([]models.VirtualTable, error) {
	if limit < 1 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	query := virtualTableSelect + ` WHERE created_by = $1`
	args := []any{ownerID}
	if project != "" {
		args = append(args, project)
		query += fmt.Sprintf(` AND project_rid = $%d`, len(args))
	}
	if source != "" {
		args = append(args, source)
		query += fmt.Sprintf(` AND source_rid = $%d`, len(args))
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d`, len(args))
	rows, err := r.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.VirtualTable, 0)
	for rows.Next() {
		v, err := scanVirtualTable(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

const virtualTableSelect = `SELECT id, rid, source_rid, project_rid, name, parent_folder_rid, locator, table_type,
	schema_inferred, capabilities, update_detection_enabled, update_detection_interval_seconds,
	last_observed_version, last_polled_at, COALESCE(update_detection_consecutive_failures, 0), update_detection_next_poll_at,
	markings, properties, created_by, created_at, updated_at FROM virtual_tables`

func (r *Repo) GetVirtualTable(ctx context.Context, rid string, ownerID string) (*models.VirtualTable, error) {
	row := r.Pool.QueryRow(ctx, virtualTableSelect+` WHERE rid = $1 AND created_by = $2`, rid, ownerID)
	v, err := scanVirtualTable(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func scanVirtualTableSourceLink(r rowLikeT) (*models.VirtualTableSourceLink, error) {
	v := &models.VirtualTableSourceLink{}
	if err := r.Scan(&v.SourceRID, &v.Provider, &v.VirtualTablesEnabled, &v.CodeImportsEnabled,
		&v.ExportControls, &v.AutoRegisterProjectRID, &v.AutoRegisterEnabled, &v.AutoRegisterIntervalSeconds,
		&v.AutoRegisterTagFilters, &v.IcebergCatalogKind, &v.IcebergCatalogConfig, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	return v, nil
}

func scanVirtualTable(r rowLikeT) (*models.VirtualTable, error) {
	v := &models.VirtualTable{}
	if err := r.Scan(&v.ID, &v.RID, &v.SourceRID, &v.ProjectRID, &v.Name, &v.ParentFolderRID,
		&v.Locator, &v.TableType, &v.SchemaInferred, &v.Capabilities, &v.UpdateDetectionEnabled,
		&v.UpdateDetectionIntervalSeconds, &v.LastObservedVersion, &v.LastPolledAt,
		&v.UpdateDetectionConsecutiveFailures, &v.UpdateDetectionNextPollAt, &v.Markings,
		&v.Properties, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	if v.Markings == nil {
		v.Markings = []string{}
	}
	return v, nil
}

const mediaSetSyncSelect = `SELECT m.id, m.source_id, m.sync_type, m.target_media_set_rid,
	m.subfolder, m.filters, m.schedule_cron, m.created_at
	FROM media_set_syncs m JOIN connections c ON c.id = m.source_id`

func (r *Repo) ListMediaSetSyncs(ctx context.Context, sourceID uuid.UUID, ownerID uuid.UUID) ([]models.MediaSetSync, error) {
	rows, err := r.Pool.Query(ctx, mediaSetSyncSelect+` WHERE m.source_id = $1 AND c.owner_id = $2 ORDER BY m.created_at DESC`, sourceID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.MediaSetSync, 0)
	for rows.Next() {
		v, err := scanMediaSetSync(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) GetMediaSetSync(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.MediaSetSync, error) {
	row := r.Pool.QueryRow(ctx, mediaSetSyncSelect+` WHERE m.id = $1 AND c.owner_id = $2`, id, ownerID)
	v, err := scanMediaSetSync(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) CreateMediaSetSync(ctx context.Context, sourceID uuid.UUID, body *models.CreateMediaSetSyncRequest, ownerID uuid.UUID) (*models.MediaSetSync, error) {
	filters, err := json.Marshal(body.Filters)
	if err != nil {
		return nil, err
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO media_set_syncs (id, source_id, sync_type, target_media_set_rid, subfolder, filters, schedule_cron)
		 SELECT $1, c.id, $3, $4, $5, $6, $7 FROM connections c WHERE c.id = $2 AND c.owner_id = $8
		 RETURNING id, source_id, sync_type, target_media_set_rid, subfolder, filters, schedule_cron, created_at`,
		uuid.New(), sourceID, string(body.Kind), strings.TrimSpace(body.TargetMediaSetRID), strings.Trim(body.Subfolder, "/"), filters, body.ScheduleCron, ownerID,
	)
	v, err := scanMediaSetSync(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) UpdateMediaSetSync(ctx context.Context, id uuid.UUID, body *models.UpdateMediaSetSyncRequest, ownerID uuid.UUID) (*models.MediaSetSync, error) {
	current, err := r.GetMediaSetSync(ctx, id, ownerID)
	if err != nil || current == nil {
		return current, err
	}
	kind := current.Kind
	if body.Kind != nil {
		kind = *body.Kind
	}
	target := current.TargetMediaSetRID
	if body.TargetMediaSetRID != nil {
		target = strings.TrimSpace(*body.TargetMediaSetRID)
	}
	subfolder := current.Subfolder
	if body.Subfolder != nil {
		subfolder = strings.Trim(*body.Subfolder, "/")
	}
	filters := current.Filters
	if body.Filters != nil {
		filters = *body.Filters
	}
	schedule := current.ScheduleCron
	if body.ScheduleCron != nil {
		schedule = body.ScheduleCron
	}
	if errs := models.ValidateMediaSetSyncConfig(kind, target, filters, schedule); len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}
	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return nil, err
	}
	row := r.Pool.QueryRow(ctx,
		`UPDATE media_set_syncs m SET sync_type = $2, target_media_set_rid = $3, subfolder = $4, filters = $5, schedule_cron = $6
		 FROM connections c WHERE m.source_id = c.id AND m.id = $1 AND c.owner_id = $7
		 RETURNING m.id, m.source_id, m.sync_type, m.target_media_set_rid, m.subfolder, m.filters, m.schedule_cron, m.created_at`,
		id, string(kind), target, subfolder, filtersJSON, schedule, ownerID,
	)
	return scanMediaSetSync(row)
}

func scanMediaSetSync(r rowLikeT) (*models.MediaSetSync, error) {
	v := &models.MediaSetSync{}
	var kind string
	var filters []byte
	if err := r.Scan(&v.ID, &v.SourceID, &kind, &v.TargetMediaSetRID, &v.Subfolder, &filters, &v.ScheduleCron, &v.CreatedAt); err != nil {
		return nil, err
	}
	v.Kind = models.MediaSetSyncKind(kind)
	if len(filters) == 0 {
		filters = []byte(`{}`)
	}
	if err := json.Unmarshal(filters, &v.Filters); err != nil {
		return nil, err
	}
	return v, nil
}
