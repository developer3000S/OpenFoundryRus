package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openfoundry/openfoundry-go/libs/ml-kernel-go/models"
)

// asset_lineage.go ports fn get_experiment_asset_lineage from
// libs/ml-kernel/src/handlers/experiments.rs verbatim. Builds a
// 6-tier graph (experiment → runs → training jobs → model versions
// → models → deployments) with edges between every neighbour pair.

// lineageDeploymentRow holds the columns the lineage builder
// actually reads — narrower than the full ml_deployments row used
// by handlers/deployments.go.
type lineageDeploymentRow struct {
	ID                uuid.UUID
	ModelID           uuid.UUID
	Name              string
	Status            string
	StrategyType      string
	EndpointPath      string
	TrafficSplit      json.RawMessage
	MonitoringWindow  string
	BaselineDatasetID *uuid.UUID
	DriftReport       json.RawMessage
}

// GetExperimentAssetLineage handles `GET /api/v1/experiments/{id}/lineage`.
func (h *ExperimentsHandlers) GetExperimentAssetLineage(w http.ResponseWriter, r *http.Request, experimentID uuid.UUID) {
	ctx := r.Context()

	experiment, err := h.loadExperiment(ctx, experimentID)
	if err != nil {
		dbError(w, err)
		return
	}
	if experiment == nil {
		writeError(w, http.StatusNotFound, "experiment not found")
		return
	}

	runs, err := h.loadRunsForExperiment(ctx, experimentID)
	if err != nil {
		dbError(w, err)
		return
	}

	trainingJobs, err := h.loadTrainingJobsForExperiment(ctx, experimentID)
	if err != nil {
		dbError(w, err)
		return
	}

	datasetIDSet := newUUIDSet()
	modelIDSet := newUUIDSet()
	for _, id := range experiment.ObjectiveSpec.LinkedModelIDs {
		modelIDSet.add(id)
	}
	versionIDSet := newUUIDSet()
	frameworkSet := newStringSet()

	for _, id := range experiment.ObjectiveSpec.LinkedDatasetIDs {
		datasetIDSet.add(id)
	}
	for _, run := range runs {
		for _, id := range run.SourceDatasetIDs {
			datasetIDSet.add(id)
		}
		if run.ExternalTracking != nil && run.ExternalTracking.Framework != "" {
			frameworkSet.add(run.ExternalTracking.Framework)
		}
		if run.ModelVersionID != nil {
			versionIDSet.add(*run.ModelVersionID)
		}
	}
	for _, job := range trainingJobs {
		for _, id := range job.DatasetIDs {
			datasetIDSet.add(id)
		}
		if job.ModelID != nil {
			modelIDSet.add(*job.ModelID)
		}
		if job.BestModelVersionID != nil {
			versionIDSet.add(*job.BestModelVersionID)
		}
		if job.ExternalTraining != nil && job.ExternalTraining.Framework != "" {
			frameworkSet.add(job.ExternalTraining.Framework)
		}
		if engine := stringFieldFromJSON(job.TrainingConfig, "engine"); engine != "" {
			frameworkSet.add(engine)
		}
	}

	modelVersions, err := h.loadModelVersionsByIDs(ctx, versionIDSet.sorted())
	if err != nil {
		dbError(w, err)
		return
	}
	for _, v := range modelVersions {
		modelIDSet.add(v.ModelID)
		if engine := stringFieldFromJSON(v.Schema, "engine"); engine != "" {
			frameworkSet.add(engine)
		}
		if framework := nestedStringFieldFromJSON(v.Schema, "model_adapter", "framework"); framework != "" {
			frameworkSet.add(framework)
		}
	}

	registeredModels, err := h.loadModelsByIDs(ctx, modelIDSet.sorted())
	if err != nil {
		dbError(w, err)
		return
	}

	deployments, err := h.loadDeploymentsForModels(ctx, modelIDSet.sorted())
	if err != nil {
		dbError(w, err)
		return
	}

	nodes := make([]models.ModelAssetNode, 0)
	edges := make([]models.ModelAssetEdge, 0)

	experimentNodeID := assetNodeID("experiment", experiment.ID.String())
	expMeta, _ := json.Marshal(map[string]any{
		"objective":           experiment.Objective,
		"primary_metric":      experiment.PrimaryMetric,
		"deployment_target":   experiment.ObjectiveSpec.DeploymentTarget,
		"stakeholders":        experiment.ObjectiveSpec.Stakeholders,
		"success_criteria":    experiment.ObjectiveSpec.SuccessCriteria,
		"documentation_uri":   experiment.ObjectiveSpec.DocumentationURI,
		"collaboration_notes": experiment.ObjectiveSpec.CollaborationNotes,
	})
	nodes = append(nodes, models.ModelAssetNode{
		ID:       experimentNodeID,
		Kind:     "experiment",
		Label:    experiment.Name,
		Status:   experiment.ObjectiveSpec.Status,
		Metadata: expMeta,
	})

	for _, datasetID := range datasetIDSet.sorted() {
		emptyMeta, _ := json.Marshal(map[string]any{})
		nodes = append(nodes, models.ModelAssetNode{
			ID:       assetNodeID("dataset", datasetID.String()),
			Kind:     "dataset",
			Label:    datasetID.String(),
			Status:   "referenced",
			Metadata: emptyMeta,
		})
	}

	for _, datasetID := range experiment.ObjectiveSpec.LinkedDatasetIDs {
		edges = append(edges, models.ModelAssetEdge{
			Source:   experimentNodeID,
			Target:   assetNodeID("dataset", datasetID.String()),
			Relation: "targets_dataset",
		})
	}

	for _, run := range runs {
		nodeID := assetNodeID("run", run.ID.String())
		runMeta, _ := json.Marshal(map[string]any{
			"metrics":            run.Metrics,
			"params":             rawOrNullValue(run.Params),
			"artifacts":          run.Artifacts,
			"notes":              run.Notes,
			"source_dataset_ids": run.SourceDatasetIDs,
			"external_tracking":  run.ExternalTracking,
		})
		nodes = append(nodes, models.ModelAssetNode{
			ID:       nodeID,
			Kind:     "run",
			Label:    run.Name,
			Status:   run.Status,
			Metadata: runMeta,
		})
		edges = append(edges, models.ModelAssetEdge{
			Source:   experimentNodeID,
			Target:   nodeID,
			Relation: "tracks_run",
		})
		for _, datasetID := range run.SourceDatasetIDs {
			edges = append(edges, models.ModelAssetEdge{
				Source:   nodeID,
				Target:   assetNodeID("dataset", datasetID.String()),
				Relation: "consumes_dataset",
			})
		}
		if run.ModelVersionID != nil {
			edges = append(edges, models.ModelAssetEdge{
				Source:   nodeID,
				Target:   assetNodeID("version", run.ModelVersionID.String()),
				Relation: "logged_model_version",
			})
		}
	}

	for _, job := range trainingJobs {
		nodeID := assetNodeID("training_job", job.ID.String())
		jobMeta, _ := json.Marshal(map[string]any{
			"objective_metric_name": job.ObjectiveMetricName,
			"training_config":       rawOrNullValue(job.TrainingConfig),
			"hyperparameter_search": rawOrNullValue(job.HyperparameterSearch),
			"dataset_ids":           job.DatasetIDs,
			"trial_count":           len(job.Trials),
			"external_training":     job.ExternalTraining,
		})
		nodes = append(nodes, models.ModelAssetNode{
			ID:       nodeID,
			Kind:     "training_job",
			Label:    job.Name,
			Status:   job.Status,
			Metadata: jobMeta,
		})
		edges = append(edges, models.ModelAssetEdge{
			Source:   experimentNodeID,
			Target:   nodeID,
			Relation: "orchestrates_training",
		})
		for _, datasetID := range job.DatasetIDs {
			edges = append(edges, models.ModelAssetEdge{
				Source:   nodeID,
				Target:   assetNodeID("dataset", datasetID.String()),
				Relation: "trains_on",
			})
		}
		if job.ModelID != nil {
			edges = append(edges, models.ModelAssetEdge{
				Source:   nodeID,
				Target:   assetNodeID("model", job.ModelID.String()),
				Relation: "produces_for_model",
			})
		}
		if job.BestModelVersionID != nil {
			edges = append(edges, models.ModelAssetEdge{
				Source:   nodeID,
				Target:   assetNodeID("version", job.BestModelVersionID.String()),
				Relation: "best_candidate",
			})
		}
	}

	for _, modelID := range experiment.ObjectiveSpec.LinkedModelIDs {
		edges = append(edges, models.ModelAssetEdge{
			Source:   experimentNodeID,
			Target:   assetNodeID("model", modelID.String()),
			Relation: "targets_model",
		})
	}

	for _, m := range registeredModels {
		mMeta, _ := json.Marshal(map[string]any{
			"problem_type":          m.ProblemType,
			"tags":                  m.Tags,
			"latest_version_number": m.LatestVersionNumber,
		})
		nodes = append(nodes, models.ModelAssetNode{
			ID:       assetNodeID("model", m.ID.String()),
			Kind:     "model",
			Label:    m.Name,
			Status:   m.CurrentStage,
			Metadata: mMeta,
		})
	}

	for _, v := range modelVersions {
		vMeta, _ := json.Marshal(map[string]any{
			"version_number":    v.VersionNumber,
			"artifact_uri":      v.ArtifactURI,
			"metrics":           v.Metrics,
			"hyperparameters":   rawOrNullValue(v.Hyperparameters),
			"model_adapter":     v.ModelAdapter,
			"registry_source":   v.RegistrySource,
			"external_tracking": v.ExternalTracking,
			"schema":            rawOrNullValue(v.Schema),
		})
		nodes = append(nodes, models.ModelAssetNode{
			ID:       assetNodeID("version", v.ID.String()),
			Kind:     "model_version",
			Label:    v.VersionLabel,
			Status:   v.Stage,
			Metadata: vMeta,
		})
		edges = append(edges, models.ModelAssetEdge{
			Source:   assetNodeID("version", v.ID.String()),
			Target:   assetNodeID("model", v.ModelID.String()),
			Relation: "belongs_to_model",
		})
	}

	for _, d := range deployments {
		var trafficSplit any
		_ = json.Unmarshal(d.TrafficSplit, &trafficSplit)
		var driftReport any
		if len(d.DriftReport) > 0 && string(d.DriftReport) != "null" {
			_ = json.Unmarshal(d.DriftReport, &driftReport)
		}
		dMeta, _ := json.Marshal(map[string]any{
			"strategy_type":       d.StrategyType,
			"endpoint_path":       d.EndpointPath,
			"monitoring_window":   d.MonitoringWindow,
			"traffic_split":       trafficSplit,
			"baseline_dataset_id": d.BaselineDatasetID,
			"drift_report":        driftReport,
		})
		nodes = append(nodes, models.ModelAssetNode{
			ID:       assetNodeID("deployment", d.ID.String()),
			Kind:     "deployment",
			Label:    d.Name,
			Status:   d.Status,
			Metadata: dMeta,
		})
		edges = append(edges, models.ModelAssetEdge{
			Source:   assetNodeID("deployment", d.ID.String()),
			Target:   assetNodeID("model", d.ModelID.String()),
			Relation: "serves_model",
		})
		if d.BaselineDatasetID != nil {
			edges = append(edges, models.ModelAssetEdge{
				Source:   assetNodeID("deployment", d.ID.String()),
				Target:   assetNodeID("dataset", d.BaselineDatasetID.String()),
				Relation: "monitors_against_dataset",
			})
		}
	}

	writeJSON(w, http.StatusOK, models.ExperimentAssetLineageResponse{
		ExperimentID:    experiment.ID,
		ObjectiveStatus: experiment.ObjectiveSpec.Status,
		Nodes:           nodes,
		Edges:           edges,
		Summary: models.ModelAssetLineageSummary{
			DatasetCount:     datasetIDSet.size(),
			RunCount:         len(runs),
			TrainingJobCount: len(trainingJobs),
			ModelCount:       len(registeredModels),
			VersionCount:     len(modelVersions),
			DeploymentCount:  len(deployments),
			Frameworks:       frameworkSet.sorted(),
		},
	})
}

// --- DB loaders specific to the lineage builder ----------------------

func (h *ExperimentsHandlers) loadRunsForExperiment(ctx context.Context, experimentID uuid.UUID) ([]models.ExperimentRun, error) {
	rows, err := h.Pool.Query(ctx,
		`SELECT `+runColumns+` FROM ml_runs
          WHERE experiment_id = $1
          ORDER BY created_at DESC`, experimentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.ExperimentRun, 0)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, nil
}

func (h *ExperimentsHandlers) loadTrainingJobsForExperiment(ctx context.Context, experimentID uuid.UUID) ([]models.TrainingJob, error) {
	rows, err := h.Pool.Query(ctx,
		`SELECT `+trainingJobColumns+` FROM ml_training_jobs
          WHERE experiment_id = $1
          ORDER BY submitted_at DESC, created_at DESC`, experimentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.TrainingJob, 0)
	for rows.Next() {
		j, err := scanTrainingJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

func (h *ExperimentsHandlers) loadModelVersionsByIDs(ctx context.Context, ids []uuid.UUID) ([]models.ModelVersion, error) {
	if len(ids) == 0 {
		return []models.ModelVersion{}, nil
	}
	rows, err := h.Pool.Query(ctx,
		`SELECT `+modelVersionColumns+` FROM ml_model_versions
          WHERE id = ANY($1)
          ORDER BY version_number DESC, created_at DESC`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.ModelVersion, 0)
	for rows.Next() {
		v, err := scanModelVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (h *ExperimentsHandlers) loadModelsByIDs(ctx context.Context, ids []uuid.UUID) ([]models.RegisteredModel, error) {
	if len(ids) == 0 {
		return []models.RegisteredModel{}, nil
	}
	rows, err := h.Pool.Query(ctx,
		`SELECT `+modelColumns+` FROM ml_models
          WHERE id = ANY($1)
          ORDER BY updated_at DESC, created_at DESC`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.RegisteredModel, 0)
	for rows.Next() {
		m, err := scanModel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (h *ExperimentsHandlers) loadDeploymentsForModels(ctx context.Context, ids []uuid.UUID) ([]lineageDeploymentRow, error) {
	if len(ids) == 0 {
		return []lineageDeploymentRow{}, nil
	}
	rows, err := h.Pool.Query(ctx,
		`SELECT id, model_id, name, status, strategy_type, endpoint_path,
                traffic_split, monitoring_window, baseline_dataset_id, drift_report
          FROM ml_deployments
          WHERE model_id = ANY($1)
          ORDER BY updated_at DESC, created_at DESC`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]lineageDeploymentRow, 0)
	for rows.Next() {
		var d lineageDeploymentRow
		var baselineDatasetID *uuid.UUID
		if err := rows.Scan(
			&d.ID, &d.ModelID, &d.Name, &d.Status, &d.StrategyType,
			&d.EndpointPath, &d.TrafficSplit, &d.MonitoringWindow,
			&baselineDatasetID, &d.DriftReport,
		); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return nil, err
		}
		d.BaselineDatasetID = baselineDatasetID
		out = append(out, d)
	}
	return out, nil
}

// --- helpers ---------------------------------------------------------

func assetNodeID(kind, id string) string {
	return fmt.Sprintf("%s:%s", kind, id)
}

func stringFieldFromJSON(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	if s, ok := obj[key].(string); ok {
		return s
	}
	return ""
}

func nestedStringFieldFromJSON(raw json.RawMessage, parent, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	parentObj, ok := obj[parent].(map[string]any)
	if !ok {
		return ""
	}
	if s, ok := parentObj[key].(string); ok {
		return s
	}
	return ""
}

func rawOrNullValue(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

// uuidSet keeps insertion-order iteration via a slice + a presence
// map. Sorted iteration mirrors the Rust BTreeSet ordering used by
// the lineage builder for stable test output.
type uuidSet struct {
	seen map[uuid.UUID]struct{}
}

func newUUIDSet() *uuidSet { return &uuidSet{seen: map[uuid.UUID]struct{}{}} }
func (s *uuidSet) add(id uuid.UUID) {
	s.seen[id] = struct{}{}
}
func (s *uuidSet) size() int { return len(s.seen) }
func (s *uuidSet) sorted() []uuid.UUID {
	out := make([]uuid.UUID, 0, len(s.seen))
	for id := range s.seen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})
	return out
}

type stringSet struct {
	seen map[string]struct{}
}

func newStringSet() *stringSet { return &stringSet{seen: map[string]struct{}{}} }
func (s *stringSet) add(v string) {
	s.seen[v] = struct{}{}
}
func (s *stringSet) sorted() []string {
	out := make([]string, 0, len(s.seen))
	for v := range s.seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
