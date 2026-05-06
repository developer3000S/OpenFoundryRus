package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/libs/ml-kernel-go/models"
)

// ExperimentsHandlers ports libs/ml-kernel/src/handlers/experiments.rs.
//
// This slice ships the experiment-CRUD surface verbatim:
//   - GET   list_experiments
//   - POST  create_experiment
//   - PATCH update_experiment            (refreshes run-rollup)
//
// The run + asset-lineage + compare endpoints stub at 501 until the
// runs row scaffolding (~260 LOC) and the asset-lineage builder
// (~459 LOC, full interop dependency) ship in their own slices.
type ExperimentsHandlers struct {
	Pool *pgxpool.Pool
}

const experimentColumns = `id, name, description, objective, objective_spec,
                           task_type, primary_metric, status, tags,
                           owner_id, run_count, best_metric,
                           created_at, updated_at`

func scanExperiment(s predictionsScanner) (models.Experiment, error) {
	var e models.Experiment
	var objectiveSpecRaw, tagsRaw, bestMetricRaw []byte
	var ownerID *uuid.UUID
	if err := s.Scan(
		&e.ID, &e.Name, &e.Description, &e.Objective, &objectiveSpecRaw,
		&e.TaskType, &e.PrimaryMetric, &e.Status, &tagsRaw,
		&ownerID, &e.RunCount, &bestMetricRaw,
		&e.CreatedAt, &e.UpdatedAt,
	); err != nil {
		return e, err
	}
	e.OwnerID = ownerID
	if len(objectiveSpecRaw) > 0 {
		_ = json.Unmarshal(objectiveSpecRaw, &e.ObjectiveSpec)
	}
	if len(tagsRaw) > 0 {
		_ = json.Unmarshal(tagsRaw, &e.Tags)
	}
	if e.Tags == nil {
		e.Tags = []string{}
	}
	if len(bestMetricRaw) > 0 && string(bestMetricRaw) != "null" {
		var mv models.MetricValue
		if err := json.Unmarshal(bestMetricRaw, &mv); err == nil {
			e.BestMetric = &mv
		}
	}
	return e, nil
}

func (h *ExperimentsHandlers) loadExperiment(ctx context.Context, id uuid.UUID) (*models.Experiment, error) {
	row := h.Pool.QueryRow(ctx,
		`SELECT `+experimentColumns+` FROM ml_experiments WHERE id = $1`, id)
	e, err := scanExperiment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// ListExperiments handles `GET /api/v1/experiments`.
func (h *ExperimentsHandlers) ListExperiments(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT `+experimentColumns+` FROM ml_experiments
          ORDER BY updated_at DESC, created_at DESC`)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	out := make([]models.Experiment, 0)
	for rows.Next() {
		e, err := scanExperiment(rows)
		if err != nil {
			dbError(w, err)
			return
		}
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, models.ListExperimentsResponse{Data: out})
}

// CreateExperiment handles `POST /api/v1/experiments`.
func (h *ExperimentsHandlers) CreateExperiment(w http.ResponseWriter, r *http.Request) {
	var body models.CreateExperimentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "experiment name is required")
		return
	}
	taskType := body.TaskType
	if taskType == "" {
		taskType = models.DefaultExperimentTaskType
	}
	primaryMetric := body.PrimaryMetric
	if primaryMetric == "" {
		primaryMetric = models.DefaultExperimentPrimaryMetric
	}
	tags := body.Tags
	if tags == nil {
		tags = []string{}
	}
	objectiveSpec := models.ModelingObjectiveSpec{Status: models.DefaultObjectiveStatus}
	if body.ObjectiveSpec != nil {
		objectiveSpec = *body.ObjectiveSpec
		if objectiveSpec.Status == "" {
			objectiveSpec.Status = models.DefaultObjectiveStatus
		}
	}

	tagsJSON, _ := json.Marshal(tags)
	objectiveSpecJSON, _ := json.Marshal(objectiveSpec)

	row := h.Pool.QueryRow(r.Context(),
		`INSERT INTO ml_experiments
              (id, name, description, objective, objective_spec,
               task_type, primary_metric, status, tags, run_count,
               best_metric)
            VALUES ($1, $2, $3, $4, $5, $6, $7, 'active', $8, 0, NULL)
            RETURNING `+experimentColumns,
		uuid.New(), strings.TrimSpace(body.Name), body.Description,
		body.Objective, objectiveSpecJSON, taskType, primaryMetric, tagsJSON)
	e, err := scanExperiment(row)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// UpdateExperiment handles `PATCH /api/v1/experiments/{id}`.
func (h *ExperimentsHandlers) UpdateExperiment(w http.ResponseWriter, r *http.Request, experimentID uuid.UUID) {
	current, err := h.loadExperiment(r.Context(), experimentID)
	if err != nil {
		dbError(w, err)
		return
	}
	if current == nil {
		writeError(w, http.StatusNotFound, "experiment not found")
		return
	}
	var body models.UpdateExperimentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	tags := current.Tags
	if body.Tags != nil {
		tags = *body.Tags
	}
	if tags == nil {
		tags = []string{}
	}
	objectiveSpec := current.ObjectiveSpec
	if body.ObjectiveSpec != nil {
		objectiveSpec = *body.ObjectiveSpec
	}

	name := derefString(body.Name, current.Name)
	desc := derefString(body.Description, current.Description)
	objective := derefString(body.Objective, current.Objective)
	taskType := derefString(body.TaskType, current.TaskType)
	primaryMetric := derefString(body.PrimaryMetric, current.PrimaryMetric)
	status := derefString(body.Status, current.Status)

	tagsJSON, _ := json.Marshal(tags)
	objectiveSpecJSON, _ := json.Marshal(objectiveSpec)

	row := h.Pool.QueryRow(r.Context(),
		`UPDATE ml_experiments SET
            name = $2, description = $3, objective = $4,
            objective_spec = $5, task_type = $6, primary_metric = $7,
            status = $8, tags = $9, updated_at = NOW()
          WHERE id = $1
          RETURNING `+experimentColumns,
		experimentID, name, desc, objective, objectiveSpecJSON,
		taskType, primaryMetric, status, tagsJSON)
	e, err := scanExperiment(row)
	if err != nil {
		dbError(w, err)
		return
	}
	if err := h.refreshExperimentRollup(r.Context(), experimentID); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

// refreshExperimentRollup recomputes (run_count, best_metric) for an
// experiment by re-scanning ml_runs.metrics. Mirrors fn
// refresh_experiment_rollup verbatim — best_metric is the highest
// numerical value across run metrics whose name matches the
// experiment's primary_metric (or the first metric in the run when
// the primary isn't found).
func (h *ExperimentsHandlers) refreshExperimentRollup(ctx context.Context, experimentID uuid.UUID) error {
	var primaryMetric *string
	if err := h.Pool.QueryRow(ctx,
		`SELECT primary_metric FROM ml_experiments WHERE id = $1`,
		experimentID).Scan(&primaryMetric); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if primaryMetric == nil {
		return nil
	}

	rows, err := h.Pool.Query(ctx,
		`SELECT metrics FROM ml_runs WHERE experiment_id = $1
          ORDER BY created_at DESC`, experimentID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var bestMetric *models.MetricValue
	count := int64(0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		count++
		var metrics []models.MetricValue
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &metrics)
		}
		var candidate *models.MetricValue
		for i := range metrics {
			if metrics[i].Name == *primaryMetric {
				m := metrics[i]
				candidate = &m
				break
			}
		}
		if candidate == nil && len(metrics) > 0 {
			m := metrics[0]
			candidate = &m
		}
		if candidate == nil {
			continue
		}
		if bestMetric == nil || candidate.Value > bestMetric.Value {
			bestMetric = candidate
		}
	}

	var bestMetricJSON []byte
	if bestMetric != nil {
		bestMetricJSON, _ = json.Marshal(bestMetric)
	}
	_, err = h.Pool.Exec(ctx,
		`UPDATE ml_experiments SET run_count = $2, best_metric = $3, updated_at = NOW()
          WHERE id = $1`, experimentID, count, bestMetricJSON)
	return err
}

// GetExperimentAssetLineage handles `GET /api/v1/experiments/{id}/lineage`.
// The full path builds a 6-tier lineage graph (experiment → runs →
// training jobs → model versions → models → deployments) with edges
// drawn between every neighbour pair. That builder is ~459 LOC of
// pure logic + interop helpers and lands in its own slice.
func (h *ExperimentsHandlers) GetExperimentAssetLineage(w http.ResponseWriter, r *http.Request, experimentID uuid.UUID) {
	if _, err := h.loadExperiment(r.Context(), experimentID); err != nil {
		dbError(w, err)
		return
	}
	writeError(w, http.StatusNotImplemented, "experiment asset lineage lands with libs/ml-kernel-go/domain/{interop,asset_lineage} port")
}

// ListRuns / CreateRun / UpdateRun / CompareRuns are the run-tier
// surface — they reuse the experiments connection but need the
// ml_runs row scaffolding (params/metrics/artifacts JSONB,
// external_tracking via interop, evaluation::quality_score). Stubs
// preserve the wire envelope until the run slice ships.

func (h *ExperimentsHandlers) ListRuns(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "experiment runs surface lands with libs/ml-kernel-go/handlers/runs slice")
}

func (h *ExperimentsHandlers) CreateRun(w http.ResponseWriter, r *http.Request) {
	var body models.CreateExperimentRunRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "run name is required")
		return
	}
	writeError(w, http.StatusNotImplemented, "experiment runs surface lands with libs/ml-kernel-go/handlers/runs slice")
}

func (h *ExperimentsHandlers) UpdateRun(w http.ResponseWriter, r *http.Request, runID uuid.UUID) {
	writeError(w, http.StatusNotImplemented, "experiment runs surface lands with libs/ml-kernel-go/handlers/runs slice")
}

func (h *ExperimentsHandlers) CompareRuns(w http.ResponseWriter, r *http.Request) {
	var body models.CompareRunsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.RunIDs) == 0 {
		writeError(w, http.StatusBadRequest, "at least one run id is required")
		return
	}
	writeError(w, http.StatusNotImplemented, "experiment runs surface lands with libs/ml-kernel-go/handlers/runs slice")
}
