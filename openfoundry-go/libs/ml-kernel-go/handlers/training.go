package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/libs/ml-kernel-go/domain/interop"
	"github.com/openfoundry/openfoundry-go/libs/ml-kernel-go/domain/training"
	"github.com/openfoundry/openfoundry-go/libs/ml-kernel-go/models"
)

// TrainingHandlers ports libs/ml-kernel/src/handlers/training.rs:
//   - GET  list_training_jobs
//   - POST create_training_job
//
// create_training_job now calls domain/interop and domain/training to
// resolve external tracking, execute the deterministic training branch,
// optionally auto-register the best model version, and persist the job.
type TrainingHandlers struct {
	Pool *pgxpool.Pool
}

const trainingJobColumns = `id, experiment_id, model_id, name, status,
                            dataset_ids, training_config,
                            hyperparameter_search, objective_metric_name,
                            trials, best_model_version_id,
                            submitted_at, started_at, completed_at,
                            created_at`

func scanTrainingJob(s predictionsScanner) (models.TrainingJob, error) {
	var j models.TrainingJob
	var datasetIDsRaw, trainingConfigRaw, hyperSearchRaw, trialsRaw []byte
	var experimentID, modelID, bestVersionID *uuid.UUID
	var startedAt, completedAt *time.Time
	if err := s.Scan(
		&j.ID, &experimentID, &modelID, &j.Name, &j.Status,
		&datasetIDsRaw, &trainingConfigRaw, &hyperSearchRaw,
		&j.ObjectiveMetricName, &trialsRaw, &bestVersionID,
		&j.SubmittedAt, &startedAt, &completedAt, &j.CreatedAt,
	); err != nil {
		return j, err
	}
	j.ExperimentID = experimentID
	j.ModelID = modelID
	j.BestModelVersionID = bestVersionID
	j.StartedAt = startedAt
	j.CompletedAt = completedAt
	if len(datasetIDsRaw) > 0 {
		_ = json.Unmarshal(datasetIDsRaw, &j.DatasetIDs)
	}
	if j.DatasetIDs == nil {
		j.DatasetIDs = []uuid.UUID{}
	}
	if len(trainingConfigRaw) > 0 {
		j.TrainingConfig = trainingConfigRaw
	} else {
		j.TrainingConfig = json.RawMessage("{}")
	}
	if len(hyperSearchRaw) > 0 {
		j.HyperparameterSearch = hyperSearchRaw
	} else {
		j.HyperparameterSearch = json.RawMessage("{}")
	}
	if len(trialsRaw) > 0 {
		_ = json.Unmarshal(trialsRaw, &j.Trials)
	}
	if j.Trials == nil {
		j.Trials = []models.TrainingTrial{}
	}
	j.ExternalTraining = interop.TrackingSourceFromTrainingConfig(j.TrainingConfig)
	return j, nil
}

// ListTrainingJobs handles `GET /api/v1/training-jobs`.
func (h *TrainingHandlers) ListTrainingJobs(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT `+trainingJobColumns+` FROM ml_training_jobs
          ORDER BY submitted_at DESC, created_at DESC`)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	out := make([]models.TrainingJob, 0)
	for rows.Next() {
		j, err := scanTrainingJob(rows)
		if err != nil {
			dbError(w, err)
			return
		}
		out = append(out, j)
	}
	writeJSON(w, http.StatusOK, models.ListTrainingJobsResponse{Data: out})
}

// CreateTrainingJob handles `POST /api/v1/training-jobs`. Mirrors fn
// create_training_job verbatim: merges external tracking into the
// training config, runs ExecuteTraining (which picks the
// external-import / synthetic-trials / inline-records branch
// automatically), optionally registers a new model_version when
// auto_register_model_version + model_id + best_hyperparameters all
// resolve, then inserts the ml_training_jobs row with status='completed'.
func (h *TrainingHandlers) CreateTrainingJob(w http.ResponseWriter, r *http.Request) {
	var body models.CreateTrainingJobRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "training job name is required")
		return
	}

	objectiveMetricName := derefString(body.ObjectiveMetricName, "accuracy")
	var search json.RawMessage
	if body.HyperparameterSearch != nil && len(*body.HyperparameterSearch) > 0 {
		search = *body.HyperparameterSearch
	} else {
		search = json.RawMessage("{}")
	}
	baseConfig := body.TrainingConfig
	if len(baseConfig) == 0 || string(baseConfig) == "null" {
		baseConfig = json.RawMessage(`{"engine":"tabular-logistic"}`)
	}
	resolvedConfig := interop.MergeTrainingConfigWithExternal(baseConfig, body.ExternalTraining)

	exec, err := training.ExecuteTraining(resolvedConfig, search, objectiveMetricName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().UTC()
	jobID := uuid.New()

	var bestModelVersionID *uuid.UUID
	if body.AutoRegisterModelVersion && body.ModelID != nil &&
		exec.BestHyperparameters != nil && len(exec.BestHyperparameters) > 0 {
		modelID := *body.ModelID
		var nextVersionNumber int32
		if err := h.Pool.QueryRow(r.Context(),
			`SELECT COALESCE(MAX(version_number), 0) + 1 FROM ml_model_versions WHERE model_id = $1`,
			modelID).Scan(&nextVersionNumber); err != nil {
			dbError(w, err)
			return
		}

		artifactURI := exec.BestArtifactURI
		if artifactURI == "" {
			artifactURI = "ml://models/" + modelID.String() + "/versions/" + itoaInt32(nextVersionNumber)
		}

		var schemaJSON json.RawMessage = exec.BestSchema
		if len(schemaJSON) == 0 {
			seed := map[string]any{
				"signature":        "tabular",
				"engine":           interop.EffectiveFramework(resolvedConfig),
				"objective_metric": objectiveMetricName,
				"generated_by":     "training-orchestrator",
				"reproducibility": map[string]any{
					"dataset_ids":           body.DatasetIDs,
					"training_config":       rawMessageOrNull(resolvedConfig),
					"hyperparameter_search": rawMessageOrNull(search),
				},
			}
			seedRaw, _ := json.Marshal(seed)
			schemaJSON = interop.NormalizeModelVersionSchema(
				seedRaw, exec.BestArtifactURI, resolvedConfig, nil, nil,
				interop.TrackingSourceFromTrainingConfig(resolvedConfig),
			)
		}

		metricsJSON, _ := json.Marshal(exec.BestMetrics)

		var versionID uuid.UUID
		if err := h.Pool.QueryRow(r.Context(),
			`INSERT INTO ml_model_versions
                  (id, model_id, version_number, version_label, stage,
                   source_run_id, training_job_id, hyperparameters,
                   metrics, artifact_uri, schema, promoted_at)
                VALUES ($1, $2, $3, $4, 'candidate', NULL, $5, $6, $7, $8, $9, NULL)
                RETURNING id`,
			uuid.New(), modelID, nextVersionNumber,
			"autotune-v"+itoaInt32(nextVersionNumber),
			jobID, exec.BestHyperparameters, metricsJSON, artifactURI, schemaJSON,
		).Scan(&versionID); err != nil {
			dbError(w, err)
			return
		}
		bestModelVersionID = &versionID

		if _, err := h.Pool.Exec(r.Context(),
			`UPDATE ml_models SET latest_version_number = $2, current_stage = 'candidate', updated_at = NOW()
              WHERE id = $1`, modelID, nextVersionNumber); err != nil {
			dbError(w, err)
			return
		}
	}

	datasetIDsJSON, _ := json.Marshal(body.DatasetIDs)
	trialsJSON, _ := json.Marshal(exec.Trials)

	row := h.Pool.QueryRow(r.Context(),
		`INSERT INTO ml_training_jobs
              (id, experiment_id, model_id, name, status, dataset_ids,
               training_config, hyperparameter_search,
               objective_metric_name, trials, best_model_version_id,
               submitted_at, started_at, completed_at, created_at)
            VALUES ($1, $2, $3, $4, 'completed', $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
            RETURNING `+trainingJobColumns,
		jobID, body.ExperimentID, body.ModelID, strings.TrimSpace(body.Name),
		datasetIDsJSON, resolvedConfig, search, objectiveMetricName,
		trialsJSON, bestModelVersionID, now, now, now, now)
	j, err := scanTrainingJob(row)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func itoaInt32(n int32) string {
	if n == 0 {
		return "0"
	}
	var b [11]byte
	i := len(b)
	negative := false
	x := n
	if x < 0 {
		negative = true
		x = -x
	}
	for x > 0 {
		i--
		b[i] = byte('0' + x%10)
		x /= 10
	}
	if negative {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func rawMessageOrNull(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

// loadTrainingJob is a small helper kept private but exported via
// the handler so future slices (eg. retry / cancel endpoints) can
// reuse it without re-implementing the column list.
func (h *TrainingHandlers) loadTrainingJob(ctx context.Context, id uuid.UUID) (*models.TrainingJob, error) {
	row := h.Pool.QueryRow(ctx,
		`SELECT `+trainingJobColumns+` FROM ml_training_jobs WHERE id = $1`, id)
	j, err := scanTrainingJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}
