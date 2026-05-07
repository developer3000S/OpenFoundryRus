// Package postgres contains production Postgres adapters for pipeline-build-service.
package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/executor"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/domain/resolver"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/handler"
	livellogs "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/logs"
	"github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/models"
	sparkpkg "github.com/openfoundry/openfoundry-go/services/pipeline-build-service/internal/spark"
)

// DB is the pgx/pgxmock surface used by the repository. *pgxpool.Pool and
// pgxmock.PgxPoolIface both satisfy it.
type DB interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type txBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Repository implements the production persistence ports used by the handlers.
type Repository struct{ db DB }

func NewRepository(db DB) *Repository { return &Repository{db: db} }

func NewRepositoryFromPool(pool *pgxpool.Pool) *Repository { return NewRepository(pool) }

// Lookup implements resolver.JobSpecRepository.
func (r *Repository) Lookup(ctx context.Context, pipelineRID, outputDatasetRID, buildBranch string, fallbackChain []string) (*models.JobSpec, error) {
	branches := append([]string{buildBranch}, fallbackChain...)
	for _, branch := range branches {
		branch = strings.TrimSpace(branch)
		if branch == "" {
			continue
		}
		spec, err := r.lookupJobSpecOnBranch(ctx, pipelineRID, outputDatasetRID, branch)
		if err != nil {
			return nil, err
		}
		if spec != nil {
			return spec, nil
		}
	}
	return nil, nil
}

func (r *Repository) lookupJobSpecOnBranch(ctx context.Context, pipelineRID, outputDatasetRID, branch string) (*models.JobSpec, error) {
	const q = `SELECT rid, pipeline_rid, branch_name, inputs, output_dataset_rid, job_spec_json, content_hash
FROM pipeline_job_specs
WHERE pipeline_rid=$1 AND output_dataset_rid=$2 AND branch_name=$3`
	var spec models.JobSpec
	var inputsRaw, jobSpecRaw []byte
	var singleOutput string
	err := r.db.QueryRow(ctx, q, pipelineRID, outputDatasetRID, branch).Scan(&spec.RID, &spec.PipelineRID, &spec.BranchName, &inputsRaw, &singleOutput, &jobSpecRaw, &spec.ContentHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(inputsRaw) > 0 {
		if err := json.Unmarshal(inputsRaw, &spec.Inputs); err != nil {
			return nil, fmt.Errorf("decode job spec inputs: %w", err)
		}
	}
	var body struct {
		LogicKind         string          `json:"logic_kind"`
		LogicPayload      json.RawMessage `json:"logic_payload"`
		OutputDatasetRIDs []string        `json:"output_dataset_rids"`
	}
	if len(jobSpecRaw) > 0 {
		if err := json.Unmarshal(jobSpecRaw, &body); err != nil {
			return nil, fmt.Errorf("decode job_spec_json: %w", err)
		}
	}
	spec.LogicKind = body.LogicKind
	spec.LogicPayload = body.LogicPayload
	spec.OutputDatasetRIDs = body.OutputDatasetRIDs
	if len(spec.OutputDatasetRIDs) == 0 {
		spec.OutputDatasetRIDs = []string{singleOutput}
	}
	return &spec, nil
}

// ListBranches implements resolver.DatasetVersioningRepository using the
// dataset-versioning schema when it is co-located in the same database.
func (r *Repository) ListBranches(ctx context.Context, datasetRID string) ([]models.BranchSnapshot, error) {
	const q = `SELECT b.name, COALESCE('ri.foundry.main.transaction.' || b.head_transaction_id::text, '')
FROM dataset_branches b
WHERE b.dataset_rid=$1 AND b.deleted_at IS NULL
ORDER BY b.name`
	rows, err := r.db.Query(ctx, q, datasetRID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.BranchSnapshot{}
	for rows.Next() {
		var item models.BranchSnapshot
		var head string
		if err := rows.Scan(&item.Name, &head); err != nil {
			return nil, err
		}
		if head != "" {
			item.HeadTransactionRID = &head
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) OpenTransaction(ctx context.Context, datasetRID, branch string) (string, error) {
	txID := uuid.New()
	const q = `INSERT INTO dataset_transactions (id, dataset_id, branch_id, branch_name, tx_type, status, operation)
SELECT $1, d.id, b.id, b.name, 'UPDATE', 'OPEN', 'pipeline-build'
FROM datasets d JOIN dataset_branches b ON b.dataset_id=d.id
WHERE d.rid=$2 AND b.name=$3 AND b.deleted_at IS NULL`
	tag, err := r.db.Exec(ctx, q, txID, datasetRID, branch)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() == 0 {
		return "", fmt.Errorf("dataset branch not found: %s@%s", datasetRID, branch)
	}
	return "ri.foundry.main.transaction." + txID.String(), nil
}

func (r *Repository) ViewSchema(ctx context.Context, datasetRID, branch string) (json.RawMessage, error) {
	const q = `SELECT COALESCE(v.metadata->'schema', '{}'::jsonb)
FROM datasets d
JOIN dataset_branches b ON b.dataset_id=d.id
LEFT JOIN dataset_views v ON v.dataset_id=d.id AND v.branch_id=b.id
WHERE d.rid=$1 AND b.name=$2 AND b.deleted_at IS NULL
ORDER BY v.computed_at DESC NULLS LAST
LIMIT 1`
	var raw []byte
	err := r.db.QueryRow(ctx, q, datasetRID, branch).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return json.RawMessage(`{}`), nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	return json.RawMessage(raw), nil
}

// TryAcquire implements resolver.BranchLockRepository.
func (r *Repository) TryAcquire(ctx context.Context, outputDatasetRID string, buildID uuid.UUID, transactionRID string) (uuid.UUID, bool, error) {
	const insert = `INSERT INTO build_input_locks (output_dataset_rid, build_id, transaction_rid) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`
	tag, err := r.db.Exec(ctx, insert, outputDatasetRID, buildID, transactionRID)
	if err != nil {
		return uuid.Nil, false, err
	}
	if tag.RowsAffected() == 1 {
		return uuid.Nil, true, nil
	}
	var holder uuid.UUID
	err = r.db.QueryRow(ctx, `SELECT build_id FROM build_input_locks WHERE output_dataset_rid=$1`, outputDatasetRID).Scan(&holder)
	if err != nil {
		return uuid.Nil, false, err
	}
	return holder, false, nil
}

func (r *Repository) HasUpstreamInProgress(ctx context.Context, inputDatasetRIDs []string, selfBuildID uuid.UUID) (bool, error) {
	if len(inputDatasetRIDs) == 0 {
		return false, nil
	}
	const q = `SELECT EXISTS (
  SELECT 1 FROM job_outputs jo
  JOIN jobs j ON j.id=jo.job_id
  JOIN builds b ON b.id=j.build_id
  WHERE jo.output_dataset_rid = ANY($1) AND b.id<>$2 AND b.state IN ('BUILD_RESOLUTION','BUILD_QUEUED','BUILD_RUNNING','BUILD_ABORTING')
)`
	var exists bool
	if err := r.db.QueryRow(ctx, q, inputDatasetRIDs, selfBuildID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// OpenBuild implements handler.BuildRepository.
func (r *Repository) OpenBuild(ctx context.Context, args resolver.ResolveBuildArgs, buildID uuid.UUID) error {
	if args.TriggerKind == "" {
		args.TriggerKind = "MANUAL"
	}
	if args.AbortPolicy == "" {
		args.AbortPolicy = string(models.AbortDependentOnly)
	}
	_, err := r.db.Exec(ctx, `INSERT INTO builds (id, pipeline_rid, build_branch, job_spec_fallback, state, trigger_kind, force_build, requested_by, abort_policy)
VALUES ($1,$2,$3,$4,'BUILD_RESOLUTION',$5,$6,$7,$8)
ON CONFLICT (id) DO NOTHING`, buildID, args.PipelineRID, args.BuildBranch, args.JobSpecFallback, args.TriggerKind, args.ForceBuild, args.RequestedBy, args.AbortPolicy)
	return err
}

func (r *Repository) PersistResolvedBuild(ctx context.Context, resolved *models.ResolvedBuild) error {
	if resolved == nil {
		return errors.New("resolved build is nil")
	}
	if beginner, ok := r.db.(txBeginner); ok {
		tx, err := beginner.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		if err := persistResolvedBuild(ctx, tx, resolved); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	return persistResolvedBuild(ctx, r.db, resolved)
}

func persistResolvedBuild(ctx context.Context, db DB, resolved *models.ResolvedBuild) error {
	state := string(resolved.State)
	if resolved.QueuedReason != nil {
		state = string(models.BuildQueued)
	}
	_, err := db.Exec(ctx, `UPDATE builds SET state=$2, queued_at=CASE WHEN $2='BUILD_QUEUED' THEN NOW() ELSE queued_at END WHERE id=$1`, resolved.BuildID, state)
	if err != nil {
		return err
	}
	jobBySpec := map[string]uuid.UUID{}
	for _, job := range resolved.Jobs {
		jobBySpec[job.JobSpecRID] = job.ID
		_, err := db.Exec(ctx, `INSERT INTO jobs (id, build_id, job_spec_rid, state, output_transaction_rids)
VALUES ($1,$2,$3,'WAITING',$4)
ON CONFLICT (id) DO UPDATE SET output_transaction_rids=EXCLUDED.output_transaction_rids`, job.ID, resolved.BuildID, job.JobSpecRID, job.OutputTransactionRIDs)
		if err != nil {
			return err
		}
		for _, txRID := range job.OutputTransactionRIDs {
			datasetRID := datasetForTransaction(resolved.OpenedTransactions, txRID)
			_, err := db.Exec(ctx, `INSERT INTO job_outputs (job_id, output_dataset_rid, transaction_rid)
VALUES ($1,$2,$3) ON CONFLICT (job_id, output_dataset_rid) DO UPDATE SET transaction_rid=EXCLUDED.transaction_rid`, job.ID, datasetRID, txRID)
			if err != nil {
				return err
			}
		}
	}
	for _, job := range resolved.Jobs {
		for _, depSpec := range job.DependsOnJobSpecRIDs {
			depID, ok := jobBySpec[depSpec]
			if !ok {
				continue
			}
			_, err := db.Exec(ctx, `INSERT INTO job_dependencies (job_id, depends_on_job_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, job.ID, depID)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func datasetForTransaction(items []models.OpenedTransaction, txRID string) string {
	for _, item := range items {
		if item.TransactionRID == txRID {
			return item.DatasetRID
		}
	}
	return txRID
}

func (r *Repository) MarkBuildFailed(ctx context.Context, buildID uuid.UUID, reason string) error {
	_, err := r.db.Exec(ctx, `UPDATE builds SET state='BUILD_FAILED', error_message=$2, finished_at=NOW() WHERE id=$1`, buildID, reason)
	return err
}

// ListBuilds and GetBuild are production query helpers used by route handlers
// and repository tests.
func (r *Repository) ListBuilds(ctx context.Context, q models.ListBuildsQuery) ([]models.BuildEnvelope, error) {
	limit := int64(50)
	if q.Limit != nil && *q.Limit > 0 && *q.Limit <= 500 {
		limit = *q.Limit
	}
	var cursor *time.Time
	if q.Cursor != "" {
		if parsed, err := time.Parse(time.RFC3339, q.Cursor); err == nil {
			cursor = &parsed
		} else if parsed, err := time.Parse(time.RFC3339Nano, q.Cursor); err == nil {
			cursor = &parsed
		}
	}
	rows, err := r.db.Query(ctx, `SELECT id, rid, pipeline_rid, build_branch, job_spec_fallback, state, trigger_kind, force_build, abort_policy, queued_at, started_at, finished_at, error_message, requested_by, created_at
FROM builds
WHERE ($1='' OR pipeline_rid=$1) AND ($2='' OR state=$2) AND ($3='' OR build_branch=$3)
  AND ($4::timestamptz IS NULL OR created_at >= $4)
  AND ($5::timestamptz IS NULL OR created_at < $5)
ORDER BY created_at DESC
LIMIT $6`, q.PipelineRID, q.Status, q.Branch, q.Since, cursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.BuildEnvelope{}
	for rows.Next() {
		b, err := scanBuild(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, models.BuildEnvelope{Build: b})
	}
	return out, rows.Err()
}

func (r *Repository) GetBuild(ctx context.Context, idOrRID string) (*models.BuildEnvelope, error) {
	var b models.Build
	err := r.db.QueryRow(ctx, `SELECT id, rid, pipeline_rid, build_branch, job_spec_fallback, state, trigger_kind, force_build, abort_policy, queued_at, started_at, finished_at, error_message, requested_by, created_at
FROM builds WHERE id::text=$1 OR rid=$1`, idOrRID).Scan(&b.ID, &b.RID, &b.PipelineRID, &b.BuildBranch, &b.JobSpecFallback, &b.State, &b.TriggerKind, &b.ForceBuild, &b.AbortPolicy, &b.QueuedAt, &b.StartedAt, &b.FinishedAt, &b.ErrorMessage, &b.RequestedBy, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	jobs, err := r.listJobsForBuild(ctx, b.ID)
	if err != nil {
		return nil, err
	}
	return &models.BuildEnvelope{Build: b, Jobs: jobs}, nil
}

func scanBuild(rows pgx.Rows) (models.Build, error) {
	var b models.Build
	err := rows.Scan(&b.ID, &b.RID, &b.PipelineRID, &b.BuildBranch, &b.JobSpecFallback, &b.State, &b.TriggerKind, &b.ForceBuild, &b.AbortPolicy, &b.QueuedAt, &b.StartedAt, &b.FinishedAt, &b.ErrorMessage, &b.RequestedBy, &b.CreatedAt)
	return b, err
}

func (r *Repository) ListJobsForBuildID(ctx context.Context, idOrRID string) ([]models.Job, error) {
	var buildID uuid.UUID
	err := r.db.QueryRow(ctx, `SELECT id FROM builds WHERE id::text=$1 OR rid=$1`, idOrRID).Scan(&buildID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r.listJobsForBuild(ctx, buildID)
}

func (r *Repository) GetJob(ctx context.Context, idOrRID string) (*models.Job, error) {
	var j models.Job
	err := r.db.QueryRow(ctx, `SELECT id, rid, build_id, job_spec_rid, state, output_transaction_rids, state_changed_at, attempt, stale_skipped, failure_reason, output_content_hash, created_at
FROM jobs WHERE id::text=$1 OR rid=$1`, idOrRID).Scan(&j.ID, &j.RID, &j.BuildID, &j.JobSpecRID, &j.State, &j.OutputTransactionRIDs, &j.StateChangedAt, &j.Attempt, &j.StaleSkipped, &j.FailureReason, &j.OutputContentHash, &j.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (r *Repository) listJobsForBuild(ctx context.Context, buildID uuid.UUID) ([]models.Job, error) {
	rows, err := r.db.Query(ctx, `SELECT id, rid, build_id, job_spec_rid, state, output_transaction_rids, state_changed_at, attempt, stale_skipped, failure_reason, output_content_hash, created_at FROM jobs WHERE build_id=$1 ORDER BY created_at`, buildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Job{}
	for rows.Next() {
		var j models.Job
		if err := rows.Scan(&j.ID, &j.RID, &j.BuildID, &j.JobSpecRID, &j.State, &j.OutputTransactionRIDs, &j.StateChangedAt, &j.Attempt, &j.StaleSkipped, &j.FailureReason, &j.OutputContentHash, &j.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (r *Repository) LoadPlan(ctx context.Context, buildID uuid.UUID) (executor.Plan, error) {
	var plan executor.Plan
	var abortPolicy string
	err := r.db.QueryRow(ctx, `SELECT id, build_branch, abort_policy FROM builds WHERE id=$1`, buildID).Scan(&plan.BuildID, &plan.BuildBranch, &abortPolicy)
	if err != nil {
		return executor.Plan{}, err
	}
	plan.AbortPolicy = executor.AbortPolicy(abortPolicy)
	plan.Parallelism = executor.DefaultParallelism
	plan.MaxAttempts = 1
	jobs, err := r.listJobsForBuild(ctx, buildID)
	if err != nil {
		return executor.Plan{}, err
	}
	deps, err := r.dependenciesForBuild(ctx, buildID)
	if err != nil {
		return executor.Plan{}, err
	}
	for _, job := range jobs {
		outputs, err := r.outputsForJob(ctx, job.ID)
		if err != nil {
			return executor.Plan{}, err
		}
		plan.Nodes = append(plan.Nodes, executor.Node{ID: job.JobSpecRID, JobID: job.ID, DependsOn: deps[job.ID], Outputs: outputs, MaxAttempts: 1, Metadata: map[string]any{"job_spec_rid": job.JobSpecRID}})
	}
	return plan, nil
}

func (r *Repository) dependenciesForBuild(ctx context.Context, buildID uuid.UUID) (map[uuid.UUID][]string, error) {
	rows, err := r.db.Query(ctx, `SELECT jd.job_id, dep.job_spec_rid FROM job_dependencies jd JOIN jobs dep ON dep.id=jd.depends_on_job_id JOIN jobs j ON j.id=jd.job_id WHERE j.build_id=$1`, buildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID][]string{}
	for rows.Next() {
		var jobID uuid.UUID
		var depSpec string
		if err := rows.Scan(&jobID, &depSpec); err != nil {
			return nil, err
		}
		out[jobID] = append(out[jobID], depSpec)
	}
	return out, rows.Err()
}

func (r *Repository) outputsForJob(ctx context.Context, jobID uuid.UUID) ([]executor.OutputTransaction, error) {
	rows, err := r.db.Query(ctx, `SELECT output_dataset_rid, transaction_rid FROM job_outputs WHERE job_id=$1 ORDER BY output_dataset_rid`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []executor.OutputTransaction{}
	for rows.Next() {
		var tx executor.OutputTransaction
		if err := rows.Scan(&tx.DatasetRID, &tx.TransactionRID); err != nil {
			return nil, err
		}
		out = append(out, tx)
	}
	return out, rows.Err()
}

func (r *Repository) LoadPipeline(ctx context.Context, pipelineID uuid.UUID) (*models.Pipeline, error) {
	var p models.Pipeline
	err := r.db.QueryRow(ctx, `SELECT id, name, description, owner_id, dag, status, schedule_config, retry_policy, next_run_at, created_at, updated_at FROM pipelines WHERE id=$1`, pipelineID).Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.DAG, &p.Status, &p.ScheduleConfig, &p.RetryPolicy, &p.NextRunAt, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}

func (r *Repository) OpenPipelineRun(ctx context.Context, pipeline *models.Pipeline, req models.TriggerPipelineRequest, startedBy *uuid.UUID, contextJSON json.RawMessage) (*models.PipelineRun, error) {
	return r.OpenPipelineRunWithOptions(ctx, pipeline, req, startedBy, "manual", req.FromNodeID, nil, 1, contextJSON)
}

func (r *Repository) OpenPipelineRunWithOptions(ctx context.Context, pipeline *models.Pipeline, req models.TriggerPipelineRequest, startedBy *uuid.UUID, triggerType string, fromNodeID *string, retryOfRunID *uuid.UUID, attemptNumber int32, contextJSON json.RawMessage) (*models.PipelineRun, error) {
	id := uuid.New()
	if triggerType == "" {
		triggerType = "manual"
	}
	if attemptNumber < 1 {
		attemptNumber = 1
	}
	var run models.PipelineRun
	err := r.db.QueryRow(ctx, `INSERT INTO pipeline_runs (id, pipeline_id, status, started_by, trigger_type, attempt_number, started_from_node_id, retry_of_run_id, execution_context)
VALUES ($1,$2,'running',$3,$4,$5,$6,$7,$8)
RETURNING id, pipeline_id, status, trigger_type, started_by, attempt_number, started_from_node_id, retry_of_run_id, execution_context, node_results, error_message, started_at, finished_at`, id, pipeline.ID, startedBy, triggerType, attemptNumber, fromNodeID, retryOfRunID, contextJSON).Scan(&run.ID, &run.PipelineID, &run.Status, &run.TriggerType, &run.StartedBy, &run.AttemptNumber, &run.StartedFromNodeID, &run.RetryOfRunID, &run.ExecutionContext, &run.NodeResults, &run.ErrorMessage, &run.StartedAt, &run.FinishedAt)
	if err != nil {
		return nil, err
	}
	_ = req
	return &run, nil
}

func (r *Repository) ListPipelineRuns(ctx context.Context, pipelineID uuid.UUID, page, perPage int64) ([]models.PipelineRun, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}
	return r.queryPipelineRuns(ctx, `SELECT id, pipeline_id, status, trigger_type, started_by, attempt_number, started_from_node_id, retry_of_run_id, execution_context, node_results, error_message, started_at, finished_at FROM pipeline_runs WHERE pipeline_id=$1 ORDER BY started_at DESC LIMIT $2 OFFSET $3`, pipelineID, perPage, (page-1)*perPage)
}

func (r *Repository) GetPipelineRun(ctx context.Context, pipelineID, runID uuid.UUID) (*models.PipelineRun, error) {
	var run models.PipelineRun
	err := r.db.QueryRow(ctx, `SELECT id, pipeline_id, status, trigger_type, started_by, attempt_number, started_from_node_id, retry_of_run_id, execution_context, node_results, error_message, started_at, finished_at FROM pipeline_runs WHERE id=$1 AND pipeline_id=$2`, runID, pipelineID).Scan(&run.ID, &run.PipelineID, &run.Status, &run.TriggerType, &run.StartedBy, &run.AttemptNumber, &run.StartedFromNodeID, &run.RetryOfRunID, &run.ExecutionContext, &run.NodeResults, &run.ErrorMessage, &run.StartedAt, &run.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &run, err
}

func (r *Repository) ListBuildQueue(ctx context.Context, query handler.BuildQueueQuery) ([]models.PipelineRun, error) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PerPage < 1 {
		query.PerPage = 50
	}
	if query.PerPage > 200 {
		query.PerPage = 200
	}
	return r.queryPipelineRuns(ctx, `SELECT id, pipeline_id, status, trigger_type, started_by, attempt_number, started_from_node_id, retry_of_run_id, execution_context, node_results, error_message, started_at, finished_at FROM pipeline_runs WHERE ($1='' OR status=$1) AND ($2='' OR trigger_type=$2) AND ($3::uuid IS NULL OR pipeline_id=$3) ORDER BY started_at DESC LIMIT $4 OFFSET $5`, query.Status, query.TriggerType, query.PipelineID, query.PerPage, (query.Page-1)*query.PerPage)
}

func (r *Repository) AbortPipelineRun(ctx context.Context, runID uuid.UUID) (*models.PipelineRun, bool, error) {
	var run models.PipelineRun
	err := r.db.QueryRow(ctx, `UPDATE pipeline_runs SET status='aborted', error_message=COALESCE(error_message, 'aborted by user'), finished_at=NOW() WHERE id=$1 AND status='running' RETURNING id, pipeline_id, status, trigger_type, started_by, attempt_number, started_from_node_id, retry_of_run_id, execution_context, node_results, error_message, started_at, finished_at`, runID).Scan(&run.ID, &run.PipelineID, &run.Status, &run.TriggerType, &run.StartedBy, &run.AttemptNumber, &run.StartedFromNodeID, &run.RetryOfRunID, &run.ExecutionContext, &run.NodeResults, &run.ErrorMessage, &run.StartedAt, &run.FinishedAt)
	if err == nil {
		return &run, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, err
	}
	var exists bool
	err = r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pipeline_runs WHERE id=$1)`, runID).Scan(&exists)
	return nil, exists, err
}

func (r *Repository) QueueSummary(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.Query(ctx, `SELECT status, COUNT(*)::bigint FROM pipeline_runs WHERE started_at > NOW() - INTERVAL '24 hours' GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		out[status] = count
	}
	return out, rows.Err()
}

func (r *Repository) ListDuePipelines(ctx context.Context) ([]models.Pipeline, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name, description, owner_id, dag, status, schedule_config, retry_policy, next_run_at, created_at, updated_at FROM pipelines WHERE status='active' AND next_run_at IS NOT NULL AND next_run_at <= NOW() ORDER BY next_run_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Pipeline{}
	for rows.Next() {
		var p models.Pipeline
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.DAG, &p.Status, &p.ScheduleConfig, &p.RetryPolicy, &p.NextRunAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) UpdatePipelineNextRun(ctx context.Context, pipelineID uuid.UUID, nextRunAt *time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE pipelines SET next_run_at=$2, updated_at=NOW() WHERE id=$1`, pipelineID, nextRunAt)
	return err
}

func (r *Repository) queryPipelineRuns(ctx context.Context, sql string, args ...any) ([]models.PipelineRun, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.PipelineRun{}
	for rows.Next() {
		var run models.PipelineRun
		if err := rows.Scan(&run.ID, &run.PipelineID, &run.Status, &run.TriggerType, &run.StartedBy, &run.AttemptNumber, &run.StartedFromNodeID, &run.RetryOfRunID, &run.ExecutionContext, &run.NodeResults, &run.ErrorMessage, &run.StartedAt, &run.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (r *Repository) FinishPipelineRun(ctx context.Context, runID uuid.UUID, status string, nodeResults json.RawMessage, errorMessage *string) error {
	_, err := r.db.Exec(ctx, `UPDATE pipeline_runs SET status=$2, node_results=$3, error_message=$4, finished_at=NOW() WHERE id=$1`, runID, status, nodeResults, errorMessage)
	return err
}

func (r *Repository) LoadBuildForAbort(ctx context.Context, id string) (*handler.AbortBuildSnapshot, error) {
	var snap handler.AbortBuildSnapshot
	err := r.db.QueryRow(ctx, `SELECT id, rid, state FROM builds WHERE id::text=$1 OR rid=$1`, id).Scan(&snap.ID, &snap.RID, &snap.State)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	jobs, err := r.abortJobs(ctx, snap.ID)
	if err != nil {
		return nil, err
	}
	snap.Jobs = jobs
	return &snap, nil
}

func (r *Repository) abortJobs(ctx context.Context, buildID uuid.UUID) ([]handler.AbortJobSnapshot, error) {
	rows, err := r.db.Query(ctx, `SELECT id, state FROM jobs WHERE build_id=$1`, buildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []handler.AbortJobSnapshot{}
	for rows.Next() {
		var job handler.AbortJobSnapshot
		if err := rows.Scan(&job.ID, &job.State); err != nil {
			return nil, err
		}
		job.Outputs, err = r.outputsForJob(ctx, job.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func (r *Repository) MarkBuildAborting(ctx context.Context, buildID uuid.UUID, reason string) error {
	_, err := r.db.Exec(ctx, `UPDATE builds SET state='BUILD_ABORTING', error_message=$2 WHERE id=$1`, buildID, reason)
	return err
}

func (r *Repository) TransitionJob(ctx context.Context, jobID uuid.UUID, from, to models.JobState, reason string) error {
	_, err := r.db.Exec(ctx, `UPDATE jobs SET state=$2, state_changed_at=NOW(), failure_reason=$3 WHERE id=$1 AND state=$4`, jobID, string(to), reason, string(from))
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `INSERT INTO job_state_transitions (job_id, from_state, to_state, reason) VALUES ($1,$2,$3,$4)`, jobID, string(from), string(to), reason)
	return err
}

func (r *Repository) MarkBuildAborted(ctx context.Context, buildID uuid.UUID, reason string) error {
	_, err := r.db.Exec(ctx, `UPDATE builds SET state='BUILD_ABORTED', error_message=$2, finished_at=NOW() WHERE id=$1`, buildID, reason)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `DELETE FROM build_input_locks WHERE build_id=$1`, buildID)
	return err
}

func (r *Repository) History(ctx context.Context, jobRID string, query livellogs.Query) ([]livellogs.LogEntry, error) {
	jobID, err := uuid.Parse(jobRID)
	if err != nil {
		err = r.db.QueryRow(ctx, `SELECT id FROM jobs WHERE rid=$1`, jobRID).Scan(&jobID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
	}
	limit := query.Limit
	if limit <= 0 || limit > 5000 {
		limit = 5000
	}
	rows, err := r.db.Query(ctx, `SELECT sequence, $1::text, ts, level, message, params FROM job_logs
WHERE job_id=$2 AND sequence >= $3 AND ($4::timestamptz IS NULL OR ts >= $4) AND ($5::timestamptz IS NULL OR ts < $5) AND (cardinality($6::text[]) = 0 OR level = ANY($6::text[]))
ORDER BY sequence ASC LIMIT $7`, jobRID, jobID, query.FromSequence, query.Since, query.Until, logLevels(query.Levels), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []livellogs.LogEntry{}
	for rows.Next() {
		var entry livellogs.LogEntry
		if err := rows.Scan(&entry.Sequence, &entry.JobRID, &entry.TS, &entry.Level, &entry.Message, &entry.Params); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (r *Repository) AppendLog(ctx context.Context, jobID uuid.UUID, level livellogs.LogLevel, message string, params json.RawMessage) (livellogs.LogEntry, error) {
	id := uuid.New()
	var entry livellogs.LogEntry
	err := r.db.QueryRow(ctx, `INSERT INTO job_logs (id, job_id, level, message, params) VALUES ($1,$2,$3,$4,$5)
RETURNING sequence, ts, level, message, params`, id, jobID, string(level), message, params).Scan(&entry.Sequence, &entry.TS, &entry.Level, &entry.Message, &entry.Params)
	entry.JobRID = jobID.String()
	return entry, err
}

func logLevels(levels []livellogs.LogLevel) []string {
	out := make([]string, len(levels))
	for i, level := range levels {
		out[i] = string(level)
	}
	return out
}

func sortedStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

var _ = sortedStrings

// Abort implements executor.TransactionManager for already-opened output rows.
func (r *Repository) Abort(ctx context.Context, tx executor.OutputTransaction) error {
	_, err := r.db.Exec(ctx, `UPDATE job_outputs SET aborted=TRUE WHERE output_dataset_rid=$1 AND transaction_rid=$2`, tx.DatasetRID, tx.TransactionRID)
	return err
}

// Commit implements executor.OutputCommitter for already-opened output rows.
func (r *Repository) Commit(ctx context.Context, tx executor.OutputTransaction) error {
	_, err := r.db.Exec(ctx, `UPDATE job_outputs SET committed=TRUE WHERE output_dataset_rid=$1 AND transaction_rid=$2`, tx.DatasetRID, tx.TransactionRID)
	return err
}

func (r *Repository) ListDatasetBuilds(ctx context.Context, datasetRID string, limit int64) ([]models.Build, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `SELECT DISTINCT b.id, b.rid, b.pipeline_rid, b.build_branch, b.job_spec_fallback, b.state, b.trigger_kind, b.force_build, b.abort_policy, b.queued_at, b.started_at, b.finished_at, b.error_message, b.requested_by, b.created_at
FROM builds b
JOIN jobs j ON j.build_id=b.id
JOIN job_outputs jo ON jo.job_id=j.id
WHERE jo.output_dataset_rid=$1
ORDER BY b.created_at DESC
LIMIT $2`, datasetRID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []models.Build{}
	for rows.Next() {
		b, err := scanBuild(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *Repository) GetJobOutputs(ctx context.Context, jobRID string) (*handler.JobOutputsResponse, error) {
	var jobID uuid.UUID
	var state string
	var stale bool
	err := r.db.QueryRow(ctx, `SELECT id, state, stale_skipped FROM jobs WHERE rid=$1 OR id::text=$1`, jobRID).Scan(&jobID, &state, &stale)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := r.db.Query(ctx, `SELECT output_dataset_rid, transaction_rid, committed, aborted FROM job_outputs WHERE job_id=$1 ORDER BY output_dataset_rid ASC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &handler.JobOutputsResponse{RID: jobRID, State: state, StaleSkipped: stale, Outputs: []handler.JobOutputRow{}}
	for rows.Next() {
		var row handler.JobOutputRow
		if err := rows.Scan(&row.OutputDatasetRID, &row.TransactionRID, &row.Committed, &row.Aborted); err != nil {
			return nil, err
		}
		out.Outputs = append(out.Outputs, row)
	}
	return out, rows.Err()
}

func (r *Repository) GetJobInputResolutions(ctx context.Context, jobRID string) (json.RawMessage, error) {
	var raw []byte
	err := r.db.QueryRow(ctx, `SELECT input_view_resolutions FROM jobs WHERE rid=$1 OR id::text=$1`, jobRID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		raw = []byte(`[]`)
	}
	return json.RawMessage(raw), nil
}

func (r *Repository) PublishJobSpec(ctx context.Context, kind string, req handler.CreateJobSpecRequest, createdBy string) (handler.PublishedJobSpec, error) {
	if req.PipelineRID == "" || req.BranchName == "" {
		return handler.PublishedJobSpec{}, errors.New("pipeline_rid and branch_name are required")
	}
	if kind == "" {
		return handler.PublishedJobSpec{}, errors.New("logic kind is required")
	}
	contentHash := ""
	if req.ContentHash != nil {
		contentHash = *req.ContentHash
	}
	if contentHash == "" {
		contentHash = deriveJobSpecHash(kind, req)
	}
	outputs := append([]string(nil), req.OutputDatasetRIDs...)
	if len(outputs) == 0 && kind != "EXPORT" {
		return handler.PublishedJobSpec{}, errors.New("output_dataset_rids must declare at least one dataset")
	}
	keyOutput := ""
	if len(outputs) > 0 {
		keyOutput = outputs[0]
	}
	var existing handler.PublishedJobSpec
	err := r.db.QueryRow(ctx, `SELECT rid, logic_kind, content_hash FROM job_specs WHERE pipeline_rid=$1 AND branch_name=$2 AND logic_kind=$3 AND output_dataset_rids=$4 AND content_hash=$5 ORDER BY created_at DESC LIMIT 1`, req.PipelineRID, req.BranchName, kind, outputs, contentHash).Scan(&existing.RID, &existing.LogicKind, &existing.ContentHash)
	if err == nil {
		return existing, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return handler.PublishedJobSpec{}, err
	}
	id := uuid.New()
	rid := "ri.foundry.main.job_spec." + id.String()
	payload := req.LogicPayload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	inputsRaw, _ := json.Marshal(req.Inputs)
	if len(inputsRaw) == 0 {
		inputsRaw = []byte(`[]`)
	}
	if createdBy == "" {
		createdBy = uuid.Nil.String()
	}
	_, err = r.db.Exec(ctx, `INSERT INTO job_specs (id, rid, pipeline_rid, branch_name, logic_kind, inputs, output_dataset_rids, logic_payload, content_hash, created_by)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, rid, req.PipelineRID, req.BranchName, kind, inputsRaw, outputs, payload, contentHash, createdBy)
	if err != nil {
		return handler.PublishedJobSpec{}, err
	}
	jobSpecJSON, _ := json.Marshal(map[string]any{"logic_kind": kind, "logic_payload": json.RawMessage(payload), "output_dataset_rids": outputs})
	publisher, _ := uuid.Parse(createdBy)
	if publisher == uuid.Nil {
		publisher = uuid.Nil
	}
	if keyOutput != "" {
		_, err = r.db.Exec(ctx, `INSERT INTO pipeline_job_specs (id, pipeline_rid, branch_name, output_dataset_rid, output_branch, job_spec_json, inputs, content_hash, published_by)
VALUES ($1,$2,$3,$4,$3,$5,$6,$7,$8)
ON CONFLICT (pipeline_rid, branch_name, output_dataset_rid) DO UPDATE SET
  job_spec_json=EXCLUDED.job_spec_json,
  inputs=EXCLUDED.inputs,
  content_hash=EXCLUDED.content_hash,
  version=CASE WHEN pipeline_job_specs.content_hash=EXCLUDED.content_hash THEN pipeline_job_specs.version ELSE pipeline_job_specs.version + 1 END,
  published_at=NOW()
`, id, req.PipelineRID, req.BranchName, keyOutput, jobSpecJSON, inputsRaw, contentHash, publisher)
		if err != nil {
			return handler.PublishedJobSpec{}, err
		}
	}
	return handler.PublishedJobSpec{RID: rid, LogicKind: kind, ContentHash: contentHash}, nil
}

func (r *Repository) AppendLogByRID(ctx context.Context, jobRID string, level livellogs.LogLevel, message string, params json.RawMessage) (livellogs.LogEntry, error) {
	var jobID uuid.UUID
	if parsed, err := uuid.Parse(jobRID); err == nil {
		jobID = parsed
	} else {
		err := r.db.QueryRow(ctx, `SELECT id FROM jobs WHERE rid=$1`, jobRID).Scan(&jobID)
		if err != nil {
			return livellogs.LogEntry{}, err
		}
	}
	return r.AppendLog(ctx, jobID, level, message, params)
}

func deriveJobSpecHash(kind string, req handler.CreateJobSpecRequest) string {
	h := sha256.New()
	h.Write([]byte(kind))
	h.Write([]byte("|"))
	h.Write([]byte(req.PipelineRID))
	h.Write([]byte("|"))
	h.Write([]byte(req.BranchName))
	h.Write([]byte("|"))
	h.Write(req.LogicPayload)
	for _, output := range req.OutputDatasetRIDs {
		h.Write([]byte("|"))
		h.Write([]byte(output))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// SaveSparkSubmission upserts the Rust-compatible SparkApplication submission row.
func (r *Repository) SaveSparkSubmission(ctx context.Context, submission handler.SparkSubmission) error {
	_, err := r.db.Exec(ctx, `INSERT INTO pipeline_run_submissions
    (pipeline_run_id, spark_app_name, namespace, status, error_message)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (pipeline_run_id) DO UPDATE
SET spark_app_name=EXCLUDED.spark_app_name,
    namespace=EXCLUDED.namespace,
    status=EXCLUDED.status,
    error_message=EXCLUDED.error_message,
    submitted_at=NOW(),
    last_observed_at=NOW()`, submission.PipelineRunID, submission.SparkAppName, submission.Namespace, string(submission.Status), submission.ErrorMessage)
	return err
}

// GetSparkSubmission loads the persisted SparkApplication control-plane mapping.
func (r *Repository) GetSparkSubmission(ctx context.Context, pipelineRunID uuid.UUID) (*handler.SparkSubmission, error) {
	var sub handler.SparkSubmission
	var status string
	err := r.db.QueryRow(ctx, `SELECT pipeline_run_id, namespace, spark_app_name, status, error_message, submitted_at, last_observed_at
FROM pipeline_run_submissions WHERE pipeline_run_id=$1`, pipelineRunID).Scan(&sub.PipelineRunID, &sub.Namespace, &sub.SparkAppName, &status, &sub.ErrorMessage, &sub.SubmittedAt, &sub.LastObservedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sub.Status = sparkStatus(status)
	return &sub, nil
}

// UpdateSparkSubmissionStatus refreshes the observed SparkApplication status.
func (r *Repository) UpdateSparkSubmissionStatus(ctx context.Context, pipelineRunID uuid.UUID, status sparkpkg.SparkRunStatus, errorMessage *string) error {
	_, err := r.db.Exec(ctx, `UPDATE pipeline_run_submissions
SET status=$2, error_message=$3, last_observed_at=NOW()
WHERE pipeline_run_id=$1`, pipelineRunID, string(status), errorMessage)
	return err
}

// ListSparkSubmissions returns recent SparkApplication submissions.
func (r *Repository) ListSparkSubmissions(ctx context.Context, limit int64) ([]handler.SparkSubmission, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `SELECT pipeline_run_id, namespace, spark_app_name, status, error_message, submitted_at, last_observed_at
FROM pipeline_run_submissions
ORDER BY submitted_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []handler.SparkSubmission{}
	for rows.Next() {
		var sub handler.SparkSubmission
		var status string
		if err := rows.Scan(&sub.PipelineRunID, &sub.Namespace, &sub.SparkAppName, &status, &sub.ErrorMessage, &sub.SubmittedAt, &sub.LastObservedAt); err != nil {
			return nil, err
		}
		sub.Status = sparkStatus(status)
		items = append(items, sub)
	}
	return items, rows.Err()
}

func sparkStatus(status string) sparkpkg.SparkRunStatus {
	switch sparkpkg.SparkRunStatus(status) {
	case sparkpkg.SparkRunSubmitted, sparkpkg.SparkRunRunning, sparkpkg.SparkRunSucceeded, sparkpkg.SparkRunFailed, sparkpkg.SparkRunUnknown:
		return sparkpkg.SparkRunStatus(status)
	default:
		return sparkpkg.SparkRunUnknown
	}
}
