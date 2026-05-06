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

	"github.com/openfoundry/openfoundry-go/libs/ml-kernel-go/models"
)

// FeaturesHandlers ports libs/ml-kernel/src/handlers/features.rs:
//   - GET   list_features
//   - POST  create_feature
//   - PATCH update_feature
//   - POST  materialize_feature
//   - GET   get_online_feature_snapshot
type FeaturesHandlers struct {
	Pool *pgxpool.Pool
}

const featureColumns = `id, name, entity_name, data_type, description,
                        status, offline_source, transformation,
                        online_enabled, online_namespace, batch_schedule,
                        freshness_sla_minutes, tags, samples,
                        last_materialized_at, last_online_sync_at,
                        created_at, updated_at`

func scanFeature(s predictionsScanner) (models.FeatureDefinition, error) {
	var f models.FeatureDefinition
	var tagsRaw, samplesRaw []byte
	var lastMaterialized, lastOnlineSync *time.Time
	if err := s.Scan(
		&f.ID, &f.Name, &f.EntityName, &f.DataType, &f.Description,
		&f.Status, &f.OfflineSource, &f.Transformation,
		&f.OnlineEnabled, &f.OnlineNamespace, &f.BatchSchedule,
		&f.FreshnessSLAMinutes, &tagsRaw, &samplesRaw,
		&lastMaterialized, &lastOnlineSync,
		&f.CreatedAt, &f.UpdatedAt,
	); err != nil {
		return f, err
	}
	if len(tagsRaw) > 0 {
		_ = json.Unmarshal(tagsRaw, &f.Tags)
	}
	if f.Tags == nil {
		f.Tags = []string{}
	}
	if len(samplesRaw) > 0 {
		_ = json.Unmarshal(samplesRaw, &f.Samples)
	}
	if f.Samples == nil {
		f.Samples = []models.FeatureSample{}
	}
	f.LastMaterializedAt = lastMaterialized
	f.LastOnlineSyncAt = lastOnlineSync
	return f, nil
}

func (h *FeaturesHandlers) loadFeature(ctx context.Context, id uuid.UUID) (*models.FeatureDefinition, error) {
	row := h.Pool.QueryRow(ctx,
		`SELECT `+featureColumns+` FROM ml_features WHERE id = $1`, id)
	f, err := scanFeature(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

// ListFeatures handles `GET /api/v1/features`.
func (h *FeaturesHandlers) ListFeatures(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Pool.Query(r.Context(),
		`SELECT `+featureColumns+` FROM ml_features
          ORDER BY updated_at DESC, created_at DESC`)
	if err != nil {
		dbError(w, err)
		return
	}
	defer rows.Close()
	out := make([]models.FeatureDefinition, 0)
	for rows.Next() {
		f, err := scanFeature(rows)
		if err != nil {
			dbError(w, err)
			return
		}
		out = append(out, f)
	}
	writeJSON(w, http.StatusOK, models.ListFeaturesResponse{Data: out})
}

// CreateFeature handles `POST /api/v1/features`.
func (h *FeaturesHandlers) CreateFeature(w http.ResponseWriter, r *http.Request) {
	var body models.CreateFeatureRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.EntityName) == "" {
		writeError(w, http.StatusBadRequest, "feature name and entity name are required")
		return
	}

	tags := body.Tags
	if tags == nil {
		tags = []string{}
	}
	samples := body.Samples
	if samples == nil {
		samples = []models.FeatureSample{}
	}
	tagsJSON, _ := json.Marshal(tags)
	samplesJSON, _ := json.Marshal(samples)

	now := time.Now().UTC()
	var lastMaterialized, lastOnlineSync any
	if len(samples) > 0 {
		lastMaterialized = now
		if body.OnlineEnabled {
			lastOnlineSync = now
		}
	}

	row := h.Pool.QueryRow(r.Context(),
		`INSERT INTO ml_features
              (id, name, entity_name, data_type, description, status,
               offline_source, transformation, online_enabled,
               online_namespace, batch_schedule, freshness_sla_minutes,
               tags, samples, last_materialized_at, last_online_sync_at)
            VALUES ($1, $2, $3, $4, $5, 'active', $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
            RETURNING `+featureColumns,
		uuid.New(), strings.TrimSpace(body.Name),
		strings.TrimSpace(body.EntityName), body.DataType, body.Description,
		body.OfflineSource, body.Transformation, body.OnlineEnabled,
		body.OnlineNamespace, body.BatchSchedule, body.FreshnessSLAMinutes,
		tagsJSON, samplesJSON, lastMaterialized, lastOnlineSync)
	f, err := scanFeature(row)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// UpdateFeature handles `PATCH /api/v1/features/{id}`.
func (h *FeaturesHandlers) UpdateFeature(w http.ResponseWriter, r *http.Request, featureID uuid.UUID) {
	current, err := h.loadFeature(r.Context(), featureID)
	if err != nil {
		dbError(w, err)
		return
	}
	if current == nil {
		writeError(w, http.StatusNotFound, "feature not found")
		return
	}

	var body models.UpdateFeatureRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	name := derefString(body.Name, current.Name)
	entityName := derefString(body.EntityName, current.EntityName)
	dataType := derefString(body.DataType, current.DataType)
	description := derefString(body.Description, current.Description)
	status := derefString(body.Status, current.Status)
	offlineSource := derefString(body.OfflineSource, current.OfflineSource)
	transformation := derefString(body.Transformation, current.Transformation)
	onlineEnabled := current.OnlineEnabled
	if body.OnlineEnabled != nil {
		onlineEnabled = *body.OnlineEnabled
	}
	onlineNamespace := derefString(body.OnlineNamespace, current.OnlineNamespace)
	batchSchedule := derefString(body.BatchSchedule, current.BatchSchedule)
	freshnessSLA := current.FreshnessSLAMinutes
	if body.FreshnessSLAMinutes != nil {
		freshnessSLA = *body.FreshnessSLAMinutes
	}
	tags := current.Tags
	if body.Tags != nil {
		tags = *body.Tags
	}
	if tags == nil {
		tags = []string{}
	}
	tagsJSON, _ := json.Marshal(tags)

	row := h.Pool.QueryRow(r.Context(),
		`UPDATE ml_features SET
            name = $2, entity_name = $3, data_type = $4,
            description = $5, status = $6, offline_source = $7,
            transformation = $8, online_enabled = $9,
            online_namespace = $10, batch_schedule = $11,
            freshness_sla_minutes = $12, tags = $13, updated_at = NOW()
          WHERE id = $1
          RETURNING `+featureColumns,
		featureID, name, entityName, dataType, description, status,
		offlineSource, transformation, onlineEnabled, onlineNamespace,
		batchSchedule, freshnessSLA, tagsJSON)
	f, err := scanFeature(row)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// MaterializeFeature handles `POST /api/v1/features/{id}/materialize`.
func (h *FeaturesHandlers) MaterializeFeature(w http.ResponseWriter, r *http.Request, featureID uuid.UUID) {
	current, err := h.loadFeature(r.Context(), featureID)
	if err != nil {
		dbError(w, err)
		return
	}
	if current == nil {
		writeError(w, http.StatusNotFound, "feature not found")
		return
	}

	var body models.MaterializeFeatureRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	samples := body.Samples
	if len(samples) == 0 {
		samples = current.Samples
	}
	if samples == nil {
		samples = []models.FeatureSample{}
	}
	samplesJSON, _ := json.Marshal(samples)
	now := time.Now().UTC()
	var lastOnlineSync any
	if current.OnlineEnabled {
		lastOnlineSync = now
	} else {
		lastOnlineSync = current.LastOnlineSyncAt
	}

	row := h.Pool.QueryRow(r.Context(),
		`UPDATE ml_features SET
            samples = $2, last_materialized_at = $3,
            last_online_sync_at = $4, updated_at = NOW()
          WHERE id = $1
          RETURNING `+featureColumns,
		featureID, samplesJSON, now, lastOnlineSync)
	f, err := scanFeature(row)
	if err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// GetOnlineFeatureSnapshot handles
// `GET /api/v1/features/{id}/online-snapshot`.
func (h *FeaturesHandlers) GetOnlineFeatureSnapshot(w http.ResponseWriter, r *http.Request, featureID uuid.UUID) {
	current, err := h.loadFeature(r.Context(), featureID)
	if err != nil {
		dbError(w, err)
		return
	}
	if current == nil {
		writeError(w, http.StatusNotFound, "feature not found")
		return
	}
	source := "offline-materialization"
	if current.OnlineEnabled {
		source = "online-cache"
	}
	writeJSON(w, http.StatusOK, models.OnlineFeatureSnapshot{
		FeatureID: current.ID,
		Namespace: current.OnlineNamespace,
		Source:    source,
		Values:    current.Samples,
		FetchedAt: time.Now().UTC(),
	})
}

func derefString(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}
