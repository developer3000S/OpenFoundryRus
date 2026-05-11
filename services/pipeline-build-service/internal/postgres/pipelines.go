package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
)

const pipelineSelectColumns = `id, name, description, owner_id, COALESCE(draft_dag, dag) AS dag, status,
	COALESCE(pipeline_type, 'BATCH') AS pipeline_type,
	COALESCE(lifecycle, 'DRAFT') AS lifecycle,
	schedule_config, retry_policy, next_run_at,
	external_config, incremental_config, streaming_config, distributed_config, compute_profile_id, project_id,
	COALESCE(draft_dag, dag) AS draft_dag,
	COALESCE(published_dag, 'null'::jsonb) AS published_dag,
	COALESCE(branch_name, 'main') AS branch_name,
	draft_updated_at, published_at, active_version_id,
	COALESCE(proposal_state, 'none') AS proposal_state,
	proposal_title, proposal_description,
	created_at, updated_at`

func (r *Repository) ListPipelines(ctx context.Context, query models.ListPipelinesQuery) (models.ListPipelinesResponse, error) {
	page := int64(1)
	if query.Page != nil && *query.Page > 0 {
		page = *query.Page
	}
	perPage := int64(50)
	if query.PerPage != nil && *query.PerPage > 0 {
		perPage = *query.PerPage
	}
	if perPage > 200 {
		perPage = 200
	}
	search, status := "", ""
	if query.Search != nil {
		search = strings.TrimSpace(*query.Search)
	}
	if query.Status != nil {
		status = strings.TrimSpace(*query.Status)
	}
	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM pipelines WHERE ($1='' OR name ILIKE '%' || $1 || '%' OR description ILIKE '%' || $1 || '%') AND ($2='' OR status=$2)`, search, status).Scan(&total); err != nil {
		return models.ListPipelinesResponse{}, err
	}
	rows, err := r.db.Query(ctx, `SELECT `+pipelineSelectColumns+`
FROM pipelines
WHERE ($1='' OR name ILIKE '%' || $1 || '%' OR description ILIKE '%' || $1 || '%') AND ($2='' OR status=$2)
ORDER BY updated_at DESC, created_at DESC
LIMIT $3 OFFSET $4`, search, status, perPage, (page-1)*perPage)
	if err != nil {
		return models.ListPipelinesResponse{}, err
	}
	defer rows.Close()
	items := []models.Pipeline{}
	for rows.Next() {
		p, err := scanPipeline(rows)
		if err != nil {
			return models.ListPipelinesResponse{}, err
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return models.ListPipelinesResponse{}, err
	}
	return models.ListPipelinesResponse{Data: items, Total: total, Page: page, PerPage: perPage}, nil
}

func (r *Repository) CreatePipeline(ctx context.Context, req models.CreatePipelineRequest, ownerID uuid.UUID) (*models.Pipeline, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	status := "draft"
	if req.Status != nil && strings.TrimSpace(*req.Status) != "" {
		status = strings.TrimSpace(*req.Status)
	}
	pipelineType := models.PipelineTypeBatch
	if req.PipelineType != nil {
		pipelineType = models.NormalizePipelineType(*req.PipelineType)
	}
	if err := models.ValidatePipelineType(pipelineType); err != nil {
		return nil, err
	}
	lifecycle := models.PipelineLifecycleDraft
	if req.Lifecycle != nil {
		lifecycle = models.NormalizePipelineLifecycle(*req.Lifecycle)
	}
	if err := models.ValidatePipelineLifecycle(lifecycle); err != nil {
		return nil, err
	}
	externalConfig := nullableJSON(req.External)
	incrementalConfig := nullableJSON(req.Incremental)
	streamingConfig := nullableJSON(req.Streaming)
	distributedConfig := nullableJSON(req.Distributed)
	if err := validatePipelineTypeConfig(pipelineType, externalConfig, streamingConfig); err != nil {
		return nil, err
	}
	if err := validateDistributedPipelineConfig(pipelineType, distributedConfig); err != nil {
		return nil, err
	}
	branchName := "main"
	if req.BranchName != nil && strings.TrimSpace(*req.BranchName) != "" {
		branchName = strings.TrimSpace(*req.BranchName)
	}
	dag, err := req.CanonicalDAG()
	if err != nil {
		return nil, fmt.Errorf("encode pipeline graph: %w", err)
	}
	scheduleConfig := json.RawMessage(`{}`)
	if req.ScheduleConfig != nil {
		scheduleConfig, err = json.Marshal(req.ScheduleConfig)
		if err != nil {
			return nil, fmt.Errorf("encode schedule_config: %w", err)
		}
	}
	retryPolicy := models.DefaultPipelineRetryPolicy()
	if req.RetryPolicy != nil {
		retryPolicy = *req.RetryPolicy
	}
	retryPolicyRaw, err := json.Marshal(retryPolicy)
	if err != nil {
		return nil, fmt.Errorf("encode retry_policy: %w", err)
	}
	id := uuid.New()
	pipeline, err := r.insertPipeline(ctx, id, req.Name, description, ownerID, dag, status, pipelineType, lifecycle, branchName, scheduleConfig, retryPolicyRaw, externalConfig, incrementalConfig, streamingConfig, distributedConfig, req.ComputeProfile, req.ProjectID)
	if err != nil {
		return nil, err
	}
	if _, err := r.insertPipelineVersion(ctx, pipeline, "draft", "Initial draft", &ownerID, nil); err != nil {
		return nil, err
	}
	return pipeline, nil
}

func (r *Repository) GetPipeline(ctx context.Context, id uuid.UUID) (*models.Pipeline, error) {
	return r.LoadPipeline(ctx, id)
}

func (r *Repository) UpdatePipeline(ctx context.Context, id uuid.UUID, req models.UpdatePipelineRequest) (*models.Pipeline, error) {
	current, err := r.LoadPipeline(ctx, id)
	if err != nil || current == nil {
		return current, err
	}
	name := current.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	description := current.Description
	if req.Description != nil {
		description = *req.Description
	}
	status := current.Status
	if req.Status != nil && strings.TrimSpace(*req.Status) != "" {
		status = strings.TrimSpace(*req.Status)
	}
	pipelineType := current.PipelineType
	if pipelineType == "" {
		pipelineType = models.PipelineTypeBatch
	}
	if req.PipelineType != nil {
		pipelineType = models.NormalizePipelineType(*req.PipelineType)
	}
	if err := models.ValidatePipelineType(pipelineType); err != nil {
		return nil, err
	}
	lifecycle := current.Lifecycle
	if lifecycle == "" {
		lifecycle = models.PipelineLifecycleDraft
	}
	if req.Lifecycle != nil {
		lifecycle = models.NormalizePipelineLifecycle(*req.Lifecycle)
	}
	if err := models.ValidatePipelineLifecycle(lifecycle); err != nil {
		return nil, err
	}
	externalConfig := nullableJSON(current.ExternalConfig)
	if rawJSONSpecified(req.External) {
		externalConfig = nullableJSON(req.External)
	}
	incrementalConfig := nullableJSON(current.IncrementalConfig)
	if rawJSONSpecified(req.Incremental) {
		incrementalConfig = nullableJSON(req.Incremental)
	}
	streamingConfig := nullableJSON(current.StreamingConfig)
	if rawJSONSpecified(req.Streaming) {
		streamingConfig = nullableJSON(req.Streaming)
	}
	distributedConfig := nullableJSON(current.DistributedConfig)
	if rawJSONSpecified(req.Distributed) {
		distributedConfig = nullableJSON(req.Distributed)
	}
	if err := validatePipelineTypeConfig(pipelineType, externalConfig, streamingConfig); err != nil {
		return nil, err
	}
	if err := validateDistributedPipelineConfig(pipelineType, distributedConfig); err != nil {
		return nil, err
	}
	computeProfileID := current.ComputeProfileID
	if req.ComputeProfile != nil {
		computeProfileID = req.ComputeProfile
	}
	projectID := current.ProjectID
	if req.ProjectID != nil {
		projectID = req.ProjectID
	}
	branchName := current.BranchName
	if branchName == "" {
		branchName = "main"
	}
	if req.BranchName != nil && strings.TrimSpace(*req.BranchName) != "" {
		branchName = strings.TrimSpace(*req.BranchName)
	}
	dag := current.DAG
	graphUpdated := req.HasGraphUpdate()
	if graphUpdated {
		dag, err = req.CanonicalDAG()
		if err != nil {
			return nil, fmt.Errorf("encode pipeline graph: %w", err)
		}
	}
	scheduleConfig := current.ScheduleConfig
	if req.ScheduleConfig != nil {
		scheduleConfig, err = json.Marshal(req.ScheduleConfig)
		if err != nil {
			return nil, fmt.Errorf("encode schedule_config: %w", err)
		}
	}
	retryPolicy := current.RetryPolicy
	if req.RetryPolicy != nil {
		retryPolicy, err = json.Marshal(req.RetryPolicy)
		if err != nil {
			return nil, fmt.Errorf("encode retry_policy: %w", err)
		}
	}
	var p models.Pipeline
	err = r.db.QueryRow(ctx, `UPDATE pipelines
SET name=$2, description=$3, dag=$4, draft_dag=$4, status=$5, schedule_config=$6, retry_policy=$7, branch_name=$8,
    pipeline_type=$9, lifecycle=$10, external_config=$11, incremental_config=$12, streaming_config=$13, distributed_config=$14, compute_profile_id=$15, project_id=$16,
    draft_updated_at=NOW(), updated_at=NOW()
WHERE id=$1
RETURNING `+pipelineSelectColumns, id, name, description, dag, status, scheduleConfig, retryPolicy, branchName, pipelineType, lifecycle, externalConfig, incrementalConfig, streamingConfig, distributedConfig, computeProfileID, projectID).Scan(pipelineScanDest(&p)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if graphUpdated {
		if _, err := r.insertPipelineVersion(ctx, &p, "draft", "Draft saved", nil, nil); err != nil {
			return nil, err
		}
	}
	return &p, nil
}

func (r *Repository) DeletePipeline(ctx context.Context, id uuid.UUID) (bool, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM pipelines WHERE id=$1`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repository) ListPipelineVersions(ctx context.Context, pipelineID uuid.UUID) ([]models.PipelineVersion, error) {
	rows, err := r.db.Query(ctx, `SELECT id, pipeline_id, version_number, branch_name, version_kind, dag, name, description, schedule_config, retry_policy, created_by, created_at, message, restored_from_version_id
FROM pipeline_versions
WHERE pipeline_id=$1
ORDER BY version_number DESC`, pipelineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.PipelineVersion{}
	for rows.Next() {
		version, err := scanPipelineVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, version)
	}
	return out, rows.Err()
}

func (r *Repository) PublishPipeline(ctx context.Context, id uuid.UUID, req models.PublishPipelineRequest, actorID *uuid.UUID) (*models.PipelinePublishResponse, error) {
	current, err := r.LoadPipeline(ctx, id)
	if err != nil || current == nil {
		return nil, err
	}
	branchName := current.BranchName
	if req.BranchName != nil && strings.TrimSpace(*req.BranchName) != "" {
		branchName = strings.TrimSpace(*req.BranchName)
	}
	if branchName == "" {
		branchName = "main"
	}
	current.BranchName = branchName
	version, err := r.insertPipelineVersion(ctx, current, "published", firstNonEmptyString(req.Message, "Published draft"), actorID, nil)
	if err != nil {
		return nil, err
	}
	title := req.ProposalTitle
	description := req.ProposalDescription
	var pipeline models.Pipeline
	err = r.db.QueryRow(ctx, `UPDATE pipelines
SET published_dag=$2, active_version_id=$3, published_at=NOW(), branch_name=$4, status='active',
    proposal_state='merged', proposal_title=$5, proposal_description=$6, updated_at=NOW()
WHERE id=$1
RETURNING `+pipelineSelectColumns, id, current.DAG, version.ID, branchName, title, description).Scan(pipelineScanDest(&pipeline)...)
	if err != nil {
		return nil, err
	}
	return &models.PipelinePublishResponse{Pipeline: pipeline, Version: version}, nil
}

func (r *Repository) CreatePipelineProposal(ctx context.Context, id uuid.UUID, req models.CreatePipelineProposalRequest, actorID *uuid.UUID) (*models.PipelinePublishResponse, error) {
	current, err := r.LoadPipeline(ctx, id)
	if err != nil || current == nil {
		return nil, err
	}
	branchName := current.BranchName
	if req.BranchName != nil && strings.TrimSpace(*req.BranchName) != "" {
		branchName = strings.TrimSpace(*req.BranchName)
	}
	if branchName == "" {
		branchName = "main"
	}
	current.BranchName = branchName
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Draft proposal"
	}
	version, err := r.insertPipelineVersion(ctx, current, "proposal", title, actorID, nil)
	if err != nil {
		return nil, err
	}
	var pipeline models.Pipeline
	err = r.db.QueryRow(ctx, `UPDATE pipelines
SET branch_name=$2, proposal_state='open', proposal_title=$3, proposal_description=$4, updated_at=NOW()
WHERE id=$1
RETURNING `+pipelineSelectColumns, id, branchName, title, req.Description).Scan(pipelineScanDest(&pipeline)...)
	if err != nil {
		return nil, err
	}
	return &models.PipelinePublishResponse{Pipeline: pipeline, Version: version}, nil
}

func (r *Repository) RestorePipelineVersion(ctx context.Context, pipelineID, versionID uuid.UUID, req models.RestorePipelineVersionRequest, actorID *uuid.UUID) (*models.PipelinePublishResponse, error) {
	version, err := r.getPipelineVersion(ctx, pipelineID, versionID)
	if err != nil || version == nil {
		return nil, err
	}
	current, err := r.LoadPipeline(ctx, pipelineID)
	if err != nil || current == nil {
		return nil, err
	}
	message := firstNonEmptyString(req.Message, fmt.Sprintf("Restored version %d", version.VersionNumber))
	restoredSnapshot := *current
	restoredSnapshot.DAG = version.DAG
	restoredSnapshot.DraftDAG = version.DAG
	restoredSnapshot.Name = version.Name
	restoredSnapshot.Description = version.Description
	restoredSnapshot.ScheduleConfig = version.ScheduleConfig
	restoredSnapshot.RetryPolicy = version.RetryPolicy
	restoredSnapshot.BranchName = version.BranchName
	restoredVersion, err := r.insertPipelineVersion(ctx, &restoredSnapshot, "restored", message, actorID, &version.ID)
	if err != nil {
		return nil, err
	}
	var pipeline models.Pipeline
	if req.AsDraft {
		err = r.db.QueryRow(ctx, `UPDATE pipelines
SET name=$2, description=$3, dag=$4, draft_dag=$4, schedule_config=$5, retry_policy=$6,
    branch_name=$7, draft_updated_at=NOW(), proposal_state='none', proposal_title=NULL, proposal_description=NULL, updated_at=NOW()
WHERE id=$1
RETURNING `+pipelineSelectColumns, pipelineID, version.Name, version.Description, version.DAG, version.ScheduleConfig, version.RetryPolicy, version.BranchName).Scan(pipelineScanDest(&pipeline)...)
	} else {
		err = r.db.QueryRow(ctx, `UPDATE pipelines
SET name=$2, description=$3, dag=$4, draft_dag=$4, published_dag=$4, schedule_config=$5, retry_policy=$6,
    branch_name=$7, active_version_id=$8, published_at=NOW(), status='active',
    proposal_state='none', proposal_title=NULL, proposal_description=NULL, draft_updated_at=NOW(), updated_at=NOW()
WHERE id=$1
RETURNING `+pipelineSelectColumns, pipelineID, version.Name, version.Description, version.DAG, version.ScheduleConfig, version.RetryPolicy, version.BranchName, restoredVersion.ID).Scan(pipelineScanDest(&pipeline)...)
	}
	if err != nil {
		return nil, err
	}
	return &models.PipelinePublishResponse{Pipeline: pipeline, Version: restoredVersion}, nil
}

func (r *Repository) insertPipeline(ctx context.Context, id uuid.UUID, name, description string, ownerID uuid.UUID, dag json.RawMessage, status string, pipelineType string, lifecycle string, branchName string, scheduleConfig json.RawMessage, retryPolicy json.RawMessage, externalConfig any, incrementalConfig any, streamingConfig any, distributedConfig any, computeProfileID *string, projectID *uuid.UUID) (*models.Pipeline, error) {
	var p models.Pipeline
	err := r.db.QueryRow(ctx, `INSERT INTO pipelines
	(id, name, description, owner_id, dag, draft_dag, status, pipeline_type, lifecycle, branch_name, schedule_config, retry_policy, external_config, incremental_config, streaming_config, distributed_config, compute_profile_id, project_id, draft_updated_at)
VALUES ($1,$2,$3,$4,$5,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,NOW())
RETURNING `+pipelineSelectColumns, id, name, description, ownerID, dag, status, pipelineType, lifecycle, branchName, scheduleConfig, retryPolicy, externalConfig, incrementalConfig, streamingConfig, distributedConfig, computeProfileID, projectID).Scan(pipelineScanDest(&p)...)
	return &p, err
}

func (r *Repository) insertPipelineVersion(ctx context.Context, pipeline *models.Pipeline, kind string, message string, actorID *uuid.UUID, restoredFrom *uuid.UUID) (models.PipelineVersion, error) {
	number, err := r.nextPipelineVersionNumber(ctx, pipeline.ID)
	if err != nil {
		return models.PipelineVersion{}, err
	}
	id := uuid.New()
	branchName := pipeline.BranchName
	if branchName == "" {
		branchName = "main"
	}
	var version models.PipelineVersion
	err = r.db.QueryRow(ctx, `INSERT INTO pipeline_versions
	(id, pipeline_id, version_number, branch_name, version_kind, dag, name, description, schedule_config, retry_policy, created_by, message, restored_from_version_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
RETURNING id, pipeline_id, version_number, branch_name, version_kind, dag, name, description, schedule_config, retry_policy, created_by, created_at, message, restored_from_version_id`,
		id, pipeline.ID, number, branchName, kind, pipeline.DAG, pipeline.Name, pipeline.Description, pipeline.ScheduleConfig, pipeline.RetryPolicy, actorID, message, restoredFrom).Scan(pipelineVersionScanDest(&version)...)
	return version, err
}

func (r *Repository) nextPipelineVersionNumber(ctx context.Context, pipelineID uuid.UUID) (int64, error) {
	var number int64
	err := r.db.QueryRow(ctx, `SELECT COALESCE(MAX(version_number), 0) + 1 FROM pipeline_versions WHERE pipeline_id=$1`, pipelineID).Scan(&number)
	return number, err
}

func (r *Repository) getPipelineVersion(ctx context.Context, pipelineID, versionID uuid.UUID) (*models.PipelineVersion, error) {
	var version models.PipelineVersion
	err := r.db.QueryRow(ctx, `SELECT id, pipeline_id, version_number, branch_name, version_kind, dag, name, description, schedule_config, retry_policy, created_by, created_at, message, restored_from_version_id
FROM pipeline_versions
WHERE pipeline_id=$1 AND id=$2`, pipelineID, versionID).Scan(pipelineVersionScanDest(&version)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &version, err
}

type pipelineScanner interface {
	Scan(dest ...any) error
}

func scanPipeline(row pipelineScanner) (models.Pipeline, error) {
	var p models.Pipeline
	err := row.Scan(pipelineScanDest(&p)...)
	return p, err
}

func scanPipelineVersion(row pipelineScanner) (models.PipelineVersion, error) {
	var version models.PipelineVersion
	err := row.Scan(pipelineVersionScanDest(&version)...)
	return version, err
}

func pipelineScanDest(p *models.Pipeline) []any {
	return []any{
		&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.DAG, &p.Status,
		&p.PipelineType, &p.Lifecycle,
		&p.ScheduleConfig, &p.RetryPolicy, &p.NextRunAt,
		&p.ExternalConfig, &p.IncrementalConfig, &p.StreamingConfig, &p.DistributedConfig, &p.ComputeProfileID, &p.ProjectID,
		&p.DraftDAG, &p.PublishedDAG, &p.BranchName,
		&p.DraftUpdatedAt, &p.PublishedAt, &p.ActiveVersionID,
		&p.ProposalState, &p.ProposalTitle, &p.ProposalDescription,
		&p.CreatedAt, &p.UpdatedAt,
	}
}

func pipelineVersionScanDest(v *models.PipelineVersion) []any {
	return []any{
		&v.ID, &v.PipelineID, &v.VersionNumber, &v.BranchName, &v.VersionKind,
		&v.DAG, &v.Name, &v.Description, &v.ScheduleConfig, &v.RetryPolicy,
		&v.CreatedBy, &v.CreatedAt, &v.Message, &v.RestoredFromVersionID,
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func rawJSONSpecified(raw json.RawMessage) bool {
	return len(bytes.TrimSpace(raw)) > 0
}

func nullableJSON(raw json.RawMessage) any {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	return json.RawMessage(append([]byte(nil), trimmed...))
}

func validatePipelineTypeConfig(pipelineType string, externalConfig any, streamingConfig any) error {
	switch pipelineType {
	case models.PipelineTypeExternal:
		if !jsonFieldHasString(externalConfig, "source_system") {
			return errors.New("external.source_system is required for EXTERNAL pipelines")
		}
	case models.PipelineTypeStreaming:
		if !jsonFieldHasString(streamingConfig, "input_stream_id") {
			return errors.New("streaming.input_stream_id is required for STREAMING pipelines")
		}
	}
	return nil
}

func validateDistributedPipelineConfig(pipelineType string, distributedConfig any) error {
	if pipelineType != models.PipelineTypeDistributed || distributedConfig == nil {
		return nil
	}
	engine := strings.TrimSpace(jsonFieldString(distributedConfig, "engine"))
	if engine == "" {
		return nil
	}
	switch strings.ToLower(engine) {
	case "spark", "pyspark", "flink":
		return nil
	default:
		return fmt.Errorf("distributed.engine must be one of spark, pyspark, or flink")
	}
}

func jsonFieldHasString(raw any, field string) bool {
	return strings.TrimSpace(jsonFieldString(raw, field)) != ""
}

func jsonFieldString(raw any, field string) string {
	if raw == nil {
		return ""
	}
	var bytesRaw []byte
	switch value := raw.(type) {
	case json.RawMessage:
		bytesRaw = value
	case []byte:
		bytesRaw = value
	default:
		return ""
	}
	var holder map[string]any
	if err := json.Unmarshal(bytesRaw, &holder); err != nil {
		return ""
	}
	value, ok := holder[field].(string)
	if !ok {
		return ""
	}
	return value
}
