// Package repo holds SQL queries + embedded migrations for
// ingestion-replication-service.
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

	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/models"
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

const ingestJobSelect = `SELECT id, name, namespace, spec, status,
	kafka_connector_name, flink_deployment_name, error, created_at, updated_at
	FROM ingest_jobs`

func (r *Repo) ListIngestJobs(ctx context.Context, namespace, status string) ([]models.IngestJob, error) {
	clauses := []string{}
	args := []any{}
	if namespace != "" {
		clauses = append(clauses, fmt.Sprintf("namespace = $%d", len(args)+1))
		args = append(args, namespace)
	}
	if status != "" {
		clauses = append(clauses, fmt.Sprintf("status = $%d", len(args)+1))
		args = append(args, status)
	}
	sql := ingestJobSelect
	if len(clauses) > 0 {
		sql += " WHERE " + strings.Join(clauses, " AND ")
	}
	sql += " ORDER BY created_at DESC LIMIT 500"
	rows, err := r.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.IngestJob, 0)
	for rows.Next() {
		v, err := scanIngestJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) GetIngestJob(ctx context.Context, id uuid.UUID) (*models.IngestJob, error) {
	row := r.Pool.QueryRow(ctx, ingestJobSelect+` WHERE id = $1`, id)
	v, err := scanIngestJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) CreateIngestJob(ctx context.Context, body *models.CreateIngestJobRequest) (*models.IngestJob, error) {
	id := uuid.New()
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO ingest_jobs (id, name, namespace, spec)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, name, namespace, spec, status,
		           kafka_connector_name, flink_deployment_name, error,
		           created_at, updated_at`,
		id, strings.TrimSpace(body.Name), strings.TrimSpace(body.Namespace), body.Spec,
	)
	return scanIngestJob(row)
}

func (r *Repo) UpdateIngestJob(ctx context.Context, id uuid.UUID, body *models.UpdateIngestJobRequest) (*models.IngestJob, error) {
	current, err := r.GetIngestJob(ctx, id)
	if err != nil || current == nil {
		return current, err
	}
	status := current.Status
	if body.Status != nil {
		status = *body.Status
	}
	kafka := current.KafkaConnectorName
	if body.KafkaConnectorName != nil {
		kafka = body.KafkaConnectorName
	}
	flink := current.FlinkDeploymentName
	if body.FlinkDeploymentName != nil {
		flink = body.FlinkDeploymentName
	}
	errMsg := current.Error
	if body.Error != nil {
		errMsg = body.Error
	}
	row := r.Pool.QueryRow(ctx,
		`UPDATE ingest_jobs SET
		    status = $2, kafka_connector_name = $3,
		    flink_deployment_name = $4, error = $5, updated_at = $6
		  WHERE id = $1
		  RETURNING id, name, namespace, spec, status,
		            kafka_connector_name, flink_deployment_name, error,
		            created_at, updated_at`,
		id, status, kafka, flink, errMsg, time.Now().UTC(),
	)
	return scanIngestJob(row)
}

func (r *Repo) DeleteIngestJob(ctx context.Context, id uuid.UUID) (bool, error) {
	cmd, err := r.Pool.Exec(ctx, `DELETE FROM ingest_jobs WHERE id = $1`, id)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

type rowLikeT interface{ Scan(...any) error }

func scanIngestJob(r rowLikeT) (*models.IngestJob, error) {
	v := &models.IngestJob{}
	if err := r.Scan(&v.ID, &v.Name, &v.Namespace, &v.Spec, &v.Status,
		&v.KafkaConnectorName, &v.FlinkDeploymentName, &v.Error,
		&v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	return v, nil
}

const streamSelect = `SELECT id, name, description, status, schema, source_binding,
	retention_hours, partitions, consistency_guarantee, stream_type, compression,
	ingest_consistency, pipeline_consistency, checkpoint_interval_ms, kind,
	owner_id, created_at, updated_at FROM streaming_streams`

func (r *Repo) ListStreams(ctx context.Context, ownerID uuid.UUID, status string) ([]models.StreamDefinition, error) {
	args := []any{ownerID}
	sql := streamSelect + ` WHERE owner_id = $1`
	if status != "" {
		args = append(args, status)
		sql += fmt.Sprintf(` AND status = $%d`, len(args))
	}
	sql += ` ORDER BY created_at DESC LIMIT 500`
	rows, err := r.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.StreamDefinition, 0)
	for rows.Next() {
		v, err := scanStream(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) GetStream(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.StreamDefinition, error) {
	row := r.Pool.QueryRow(ctx, streamSelect+` WHERE id = $1 AND owner_id = $2`, id, ownerID)
	v, err := scanStream(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) CreateStream(ctx context.Context, body *models.CreateStreamRequest, ownerID uuid.UUID) (*models.StreamDefinition, error) {
	v, err := normalizeCreateStream(body)
	if err != nil {
		return nil, err
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO streaming_streams
		 (id, name, description, status, schema, source_binding, retention_hours,
		  partitions, consistency_guarantee, stream_type, compression, ingest_consistency,
		  pipeline_consistency, checkpoint_interval_ms, kind, owner_id)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		 RETURNING id, name, description, status, schema, source_binding, retention_hours,
		           partitions, consistency_guarantee, stream_type, compression,
		           ingest_consistency, pipeline_consistency, checkpoint_interval_ms, kind,
		           owner_id, created_at, updated_at`,
		uuid.New(), v.Name, v.Description, v.Status, v.Schema, v.SourceBinding,
		v.RetentionHours, v.Partitions, v.ConsistencyGuarantee, v.StreamType,
		v.Compression, v.IngestConsistency, v.PipelineConsistency,
		v.CheckpointIntervalMS, v.Kind, ownerID,
	)
	return scanStream(row)
}

func (r *Repo) UpdateStream(ctx context.Context, id uuid.UUID, body *models.UpdateStreamRequest, ownerID uuid.UUID) (*models.StreamDefinition, error) {
	cur, err := r.GetStream(ctx, id, ownerID)
	if err != nil || cur == nil {
		return cur, err
	}
	applyStreamUpdate(cur, body)
	if err := validateStream(cur); err != nil {
		return nil, err
	}
	row := r.Pool.QueryRow(ctx,
		`UPDATE streaming_streams SET name=$3, description=$4, status=$5, schema=$6,
		 source_binding=$7, retention_hours=$8, partitions=$9, consistency_guarantee=$10,
		 stream_type=$11, compression=$12, ingest_consistency=$13, pipeline_consistency=$14,
		 checkpoint_interval_ms=$15, kind=$16, updated_at=$17
		 WHERE id=$1 AND owner_id=$2
		 RETURNING id, name, description, status, schema, source_binding, retention_hours,
		           partitions, consistency_guarantee, stream_type, compression,
		           ingest_consistency, pipeline_consistency, checkpoint_interval_ms, kind,
		           owner_id, created_at, updated_at`,
		id, ownerID, cur.Name, cur.Description, cur.Status, cur.Schema, cur.SourceBinding,
		cur.RetentionHours, cur.Partitions, cur.ConsistencyGuarantee, cur.StreamType,
		cur.Compression, cur.IngestConsistency, cur.PipelineConsistency,
		cur.CheckpointIntervalMS, cur.Kind, time.Now().UTC(),
	)
	return scanStream(row)
}

func normalizeCreateStream(body *models.CreateStreamRequest) (*models.StreamDefinition, error) {
	v := &models.StreamDefinition{
		Name:                 strings.TrimSpace(body.Name),
		Description:          body.Description,
		Status:               defaultString(body.Status, "active"),
		Schema:               body.Schema,
		SourceBinding:        body.SourceBinding,
		RetentionHours:       defaultInt32(body.RetentionHours, 72),
		Partitions:           defaultInt32(body.Partitions, 3),
		ConsistencyGuarantee: defaultString(body.ConsistencyGuarantee, "at-least-once"),
		StreamType:           defaultString(body.StreamType, "STANDARD"),
		Compression:          body.Compression != nil && *body.Compression,
		IngestConsistency:    defaultString(body.IngestConsistency, "AT_LEAST_ONCE"),
		PipelineConsistency:  defaultString(body.PipelineConsistency, "AT_LEAST_ONCE"),
		CheckpointIntervalMS: defaultInt32(body.CheckpointIntervalMS, 2000),
		Kind:                 defaultString(body.Kind, "INGEST"),
	}
	if len(v.Schema) == 0 {
		v.Schema = []byte(`{"fields":[],"primary_key":null,"watermark_field":null}`)
	}
	if len(v.SourceBinding) == 0 {
		v.SourceBinding = []byte(`{"connector_type":"kafka","endpoint":"kafka://stream/default","format":"json","config":{}}`)
	}
	return v, validateStream(v)
}

func applyStreamUpdate(cur *models.StreamDefinition, body *models.UpdateStreamRequest) {
	if body.Name != nil {
		cur.Name = strings.TrimSpace(*body.Name)
	}
	if body.Description != nil {
		cur.Description = *body.Description
	}
	if body.Status != nil {
		cur.Status = *body.Status
	}
	if len(body.Schema) > 0 {
		cur.Schema = body.Schema
	}
	if len(body.SourceBinding) > 0 {
		cur.SourceBinding = body.SourceBinding
	}
	if body.RetentionHours != nil {
		cur.RetentionHours = *body.RetentionHours
	}
	if body.Partitions != nil {
		cur.Partitions = *body.Partitions
	}
	if body.ConsistencyGuarantee != nil {
		cur.ConsistencyGuarantee = *body.ConsistencyGuarantee
	}
	if body.StreamType != nil {
		cur.StreamType = *body.StreamType
	}
	if body.Compression != nil {
		cur.Compression = *body.Compression
	}
	if body.IngestConsistency != nil {
		cur.IngestConsistency = *body.IngestConsistency
	}
	if body.PipelineConsistency != nil {
		cur.PipelineConsistency = *body.PipelineConsistency
	}
	if body.CheckpointIntervalMS != nil {
		cur.CheckpointIntervalMS = *body.CheckpointIntervalMS
	}
	if body.Kind != nil {
		cur.Kind = *body.Kind
	}
}

func validateStream(v *models.StreamDefinition) error {
	if v.Name == "" {
		return fmt.Errorf("stream name is required")
	}
	if v.Partitions < 1 || v.Partitions > 256 {
		return fmt.Errorf("partitions must be between 1 and 256")
	}
	if v.RetentionHours < 1 {
		return fmt.Errorf("retention_hours must be positive")
	}
	if v.CheckpointIntervalMS < 100 || v.CheckpointIntervalMS > 86400000 {
		return fmt.Errorf("checkpoint_interval_ms out of range")
	}
	if !jsonValid(v.Schema) || !jsonValid(v.SourceBinding) {
		return fmt.Errorf("schema and source_binding must be valid JSON")
	}
	if !oneOf(v.ConsistencyGuarantee, "at-most-once", "at-least-once", "exactly-once") {
		return fmt.Errorf("invalid consistency_guarantee")
	}
	if !oneOf(v.StreamType, "STANDARD", "HIGH_THROUGHPUT", "COMPRESSED", "HIGH_THROUGHPUT_COMPRESSED") {
		return fmt.Errorf("invalid stream_type")
	}
	if v.IngestConsistency == "EXACTLY_ONCE" {
		return fmt.Errorf("streaming sources only support AT_LEAST_ONCE for ingest_consistency")
	}
	if !oneOf(v.IngestConsistency, "AT_LEAST_ONCE", "EXACTLY_ONCE") || !oneOf(v.PipelineConsistency, "AT_LEAST_ONCE", "EXACTLY_ONCE") {
		return fmt.Errorf("invalid consistency")
	}
	return nil
}

func scanStream(r rowLikeT) (*models.StreamDefinition, error) {
	v := &models.StreamDefinition{}
	if err := r.Scan(&v.ID, &v.Name, &v.Description, &v.Status, &v.Schema,
		&v.SourceBinding, &v.RetentionHours, &v.Partitions, &v.ConsistencyGuarantee,
		&v.StreamType, &v.Compression, &v.IngestConsistency, &v.PipelineConsistency,
		&v.CheckpointIntervalMS, &v.Kind, &v.OwnerID, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	return v, nil
}

const cdcStreamSelect = `SELECT id, slug, source_kind, source_ref, upstream_topic, primary_keys,
	watermark_column, incremental_mode, status, owner_id, created_at, updated_at FROM cdc_streams`

func (r *Repo) ListCdcStreams(ctx context.Context, ownerID uuid.UUID) ([]models.CdcStream, error) {
	rows, err := r.Pool.Query(ctx, cdcStreamSelect+` WHERE owner_id = $1 ORDER BY slug`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.CdcStream, 0)
	for rows.Next() {
		v, err := scanCdcStream(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (r *Repo) RegisterCdcStream(ctx context.Context, body *models.RegisterCdcStreamRequest, ownerID uuid.UUID) (*models.CdcStream, *models.IncrementalCheckpoint, *models.ResolutionState, error) {
	if strings.TrimSpace(body.Slug) == "" || strings.TrimSpace(body.SourceKind) == "" || strings.TrimSpace(body.SourceRef) == "" {
		return nil, nil, nil, fmt.Errorf("slug, source_kind and source_ref required")
	}
	mode := defaultString(body.IncrementalMode, "log_based")
	if !oneOf(mode, "append_only", "upsert", "soft_delete", "hard_delete", "log_based") {
		return nil, nil, nil, fmt.Errorf("invalid incremental_mode")
	}
	primaryKeys, err := json.Marshal(body.PrimaryKeys)
	if err != nil {
		return nil, nil, nil, err
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO cdc_streams (id, slug, source_kind, source_ref, upstream_topic, primary_keys, watermark_column, incremental_mode, status, owner_id)
		 VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,'registered',$9)
		 ON CONFLICT (slug) DO UPDATE SET source_kind=EXCLUDED.source_kind, source_ref=EXCLUDED.source_ref,
		 upstream_topic=EXCLUDED.upstream_topic, primary_keys=EXCLUDED.primary_keys, watermark_column=EXCLUDED.watermark_column,
		 incremental_mode=EXCLUDED.incremental_mode, updated_at=NOW()
		 WHERE cdc_streams.owner_id = EXCLUDED.owner_id
		 RETURNING id, slug, source_kind, source_ref, upstream_topic, primary_keys, watermark_column, incremental_mode, status, owner_id, created_at, updated_at`,
		uuid.New(), strings.TrimSpace(body.Slug), body.SourceKind, body.SourceRef, body.UpstreamTopic, primaryKeys, body.WatermarkColumn, mode, ownerID,
	)
	stream, err := scanCdcStream(row)
	if err != nil {
		return nil, nil, nil, err
	}
	cp, err := r.ensureCheckpoint(ctx, stream.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	res, err := r.ensureResolution(ctx, stream.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	return stream, cp, res, nil
}

func (r *Repo) GetCdcStream(ctx context.Context, id uuid.UUID, ownerID uuid.UUID) (*models.CdcStream, error) {
	row := r.Pool.QueryRow(ctx, cdcStreamSelect+` WHERE id = $1 AND owner_id = $2`, id, ownerID)
	v, err := scanCdcStream(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) GetCheckpoint(ctx context.Context, streamID uuid.UUID, ownerID uuid.UUID) (*models.IncrementalCheckpoint, error) {
	row := r.Pool.QueryRow(ctx, `SELECT cp.stream_id, cp.last_offset, cp.last_lsn, cp.last_event_at, cp.records_observed, cp.records_applied, cp.updated_at FROM cdc_incremental_checkpoints cp JOIN cdc_streams s ON s.id = cp.stream_id WHERE cp.stream_id=$1 AND s.owner_id=$2`, streamID, ownerID)
	v, err := scanCheckpoint(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) GetResolution(ctx context.Context, streamID uuid.UUID, ownerID uuid.UUID) (*models.ResolutionState, error) {
	row := r.Pool.QueryRow(ctx, `SELECT rs.stream_id, rs.status, rs.watermark, rs.conflict_count, rs.pending_resolutions, rs.notes, rs.updated_at FROM cdc_resolution_state rs JOIN cdc_streams s ON s.id = rs.stream_id WHERE rs.stream_id=$1 AND s.owner_id=$2`, streamID, ownerID)
	v, err := scanResolution(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) ensureCheckpoint(ctx context.Context, streamID uuid.UUID) (*models.IncrementalCheckpoint, error) {
	row := r.Pool.QueryRow(ctx, `INSERT INTO cdc_incremental_checkpoints (stream_id) VALUES ($1) ON CONFLICT (stream_id) DO UPDATE SET stream_id=EXCLUDED.stream_id RETURNING stream_id, last_offset, last_lsn, last_event_at, records_observed, records_applied, updated_at`, streamID)
	return scanCheckpoint(row)
}
func (r *Repo) ensureResolution(ctx context.Context, streamID uuid.UUID) (*models.ResolutionState, error) {
	row := r.Pool.QueryRow(ctx, `INSERT INTO cdc_resolution_state (stream_id, status) VALUES ($1, 'lagging') ON CONFLICT (stream_id) DO UPDATE SET stream_id=EXCLUDED.stream_id RETURNING stream_id, status, watermark, conflict_count, pending_resolutions, notes, updated_at`, streamID)
	return scanResolution(row)
}

func scanCdcStream(r rowLikeT) (*models.CdcStream, error) {
	v := &models.CdcStream{}
	if err := r.Scan(&v.ID, &v.Slug, &v.SourceKind, &v.SourceRef, &v.UpstreamTopic, &v.PrimaryKeys, &v.WatermarkColumn, &v.IncrementalMode, &v.Status, &v.OwnerID, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	return v, nil
}
func scanCheckpoint(r rowLikeT) (*models.IncrementalCheckpoint, error) {
	v := &models.IncrementalCheckpoint{}
	if err := r.Scan(&v.StreamID, &v.LastOffset, &v.LastLSN, &v.LastEventAt, &v.RecordsObserved, &v.RecordsApplied, &v.UpdatedAt); err != nil {
		return nil, err
	}
	return v, nil
}
func scanResolution(r rowLikeT) (*models.ResolutionState, error) {
	v := &models.ResolutionState{}
	if err := r.Scan(&v.StreamID, &v.Status, &v.Watermark, &v.ConflictCount, &v.PendingResolutions, &v.Notes, &v.UpdatedAt); err != nil {
		return nil, err
	}
	return v, nil
}

func defaultString(v, d string) string {
	if strings.TrimSpace(v) == "" {
		return d
	}
	return strings.TrimSpace(v)
}
func defaultInt32(v *int32, d int32) int32 {
	if v == nil {
		return d
	}
	return *v
}
func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
func jsonValid(v []byte) bool { return len(v) > 0 && json.Valid(v) }

func (r *Repo) ApplyCheckpoint(ctx context.Context, streamID uuid.UUID, ownerID uuid.UUID, update *models.CheckpointUpdate) (*models.IncrementalCheckpoint, error) {
	if update == nil {
		return r.GetCheckpoint(ctx, streamID, ownerID)
	}
	current, err := r.GetCheckpoint(ctx, streamID, ownerID)
	if err != nil || current == nil {
		return current, err
	}
	lastOffset := current.LastOffset
	if update.LastOffset != nil {
		lastOffset = update.LastOffset
	}
	lastLSN := current.LastLSN
	if update.LastLSN != nil {
		lastLSN = update.LastLSN
	}
	lastEventAt := current.LastEventAt
	if update.LastEventAt != nil {
		lastEventAt = update.LastEventAt
	}
	recordsObserved := current.RecordsObserved
	if update.RecordsObserved != nil {
		recordsObserved = *update.RecordsObserved
	}
	recordsApplied := current.RecordsApplied
	if update.RecordsApplied != nil {
		recordsApplied = *update.RecordsApplied
	}
	row := r.Pool.QueryRow(ctx,
		`UPDATE cdc_incremental_checkpoints cp SET last_offset=$3, last_lsn=$4, last_event_at=$5,
		 records_observed=$6, records_applied=$7, updated_at=NOW()
		 FROM cdc_streams s WHERE cp.stream_id=s.id AND cp.stream_id=$1 AND s.owner_id=$2
		 RETURNING cp.stream_id, cp.last_offset, cp.last_lsn, cp.last_event_at, cp.records_observed, cp.records_applied, cp.updated_at`,
		streamID, ownerID, lastOffset, lastLSN, lastEventAt, recordsObserved, recordsApplied)
	v, err := scanCheckpoint(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *Repo) ApplyResolution(ctx context.Context, streamID uuid.UUID, ownerID uuid.UUID, update *models.ResolutionUpdate) (*models.ResolutionState, error) {
	if update == nil {
		return r.GetResolution(ctx, streamID, ownerID)
	}
	current, err := r.GetResolution(ctx, streamID, ownerID)
	if err != nil || current == nil {
		return current, err
	}
	status := current.Status
	if update.Status != nil {
		status = *update.Status
	}
	watermark := current.Watermark
	if update.Watermark != nil {
		watermark = update.Watermark
	}
	conflicts := current.ConflictCount
	if update.ConflictCount != nil {
		conflicts = *update.ConflictCount
	}
	pending := current.PendingResolutions
	if update.PendingResolutions != nil {
		pending = *update.PendingResolutions
	}
	notes := current.Notes
	if update.Notes != nil {
		notes = update.Notes
	}
	row := r.Pool.QueryRow(ctx,
		`UPDATE cdc_resolution_state rs SET status=$3, watermark=$4, conflict_count=$5,
		 pending_resolutions=$6, notes=$7, updated_at=NOW()
		 FROM cdc_streams s WHERE rs.stream_id=s.id AND rs.stream_id=$1 AND s.owner_id=$2
		 RETURNING rs.stream_id, rs.status, rs.watermark, rs.conflict_count, rs.pending_resolutions, rs.notes, rs.updated_at`,
		streamID, ownerID, status, watermark, conflicts, pending, notes)
	v, err := scanResolution(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return v, err
}
