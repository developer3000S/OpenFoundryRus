// Saved-map handlers. Mirrors `pub async fn list_maps` and
// `pub async fn create_map` in src/handlers.rs.

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

// ListMaps mirrors Rust `pub async fn list_maps`.
func (h *Handlers) ListMaps(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Definitions.List(r.Context(), h.mapQuery(), storageabstraction.Eventual())
	if err != nil {
		writeRepoError(w, err)
		return
	}
	out := make([]models.ExploratoryMap, 0, len(rows.Items))
	for _, rec := range rows.Items {
		m, conv := mapFromRecord(rec)
		if conv != nil {
			plainText(w, http.StatusInternalServerError, conv.Error())
			return
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateMap mirrors Rust `pub async fn create_map`.
func (h *Handlers) CreateMap(w http.ResponseWriter, r *http.Request) {
	var body models.CreateMapRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		plainText(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.ViewID != nil {
		row, err := h.Definitions.Get(
			r.Context(),
			storageabstraction.DefinitionKind(viewKind),
			storageabstraction.DefinitionId(body.ViewID.String()),
			storageabstraction.Strong(),
		)
		if err != nil {
			writeRepoError(w, err)
			return
		}
		if row == nil {
			plainText(w, http.StatusNotFound, "view not found")
			return
		}
	}

	id, err := uuid.NewV7()
	if err != nil {
		plainText(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := h.nowMs()
	mp := models.ExploratoryMap{
		ID:        id,
		ViewID:    body.ViewID,
		Name:      body.Name,
		MapKind:   body.MapKind,
		Config:    body.Config,
		CreatedAt: datetimeFromMs(now),
	}

	rec, err := h.mapToRecord(mp, now)
	if err != nil {
		plainText(w, http.StatusInternalServerError, err.Error())
		return
	}
	outcome, err := h.Definitions.Put(r.Context(), rec, nil)
	if err != nil {
		writeRepoError(w, err)
		return
	}
	if outcome.Kind == storageabstraction.PutVersionConflict {
		plainText(w, http.StatusConflict, "map already exists")
		return
	}
	writeJSON(w, http.StatusCreated, mp)
}

func (h *Handlers) mapQuery() storageabstraction.DefinitionQuery {
	tenant := h.Tenant
	return storageabstraction.DefinitionQuery{
		Kind:   storageabstraction.DefinitionKind(mapKind),
		Tenant: &tenant,
		Page:   storageabstraction.Page{Size: pageLimit},
	}
}

func (h *Handlers) mapToRecord(m models.ExploratoryMap, now int64) (storageabstraction.DefinitionRecord, error) {
	tenant := h.Tenant
	version := uint64(1)

	type rawMap struct {
		ID        uuid.UUID       `json:"id"`
		ViewID    *uuid.UUID      `json:"view_id"`
		Name      string          `json:"name"`
		MapKind   string          `json:"map_kind"`
		Config    json.RawMessage `json:"config"`
		CreatedAt string          `json:"created_at"`
	}
	payload, err := json.Marshal(rawMap{
		ID:        m.ID,
		ViewID:    m.ViewID,
		Name:      m.Name,
		MapKind:   m.MapKind,
		Config:    nullableRaw(m.Config),
		CreatedAt: m.CreatedAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		return storageabstraction.DefinitionRecord{}, err
	}
	var parent *storageabstraction.DefinitionId
	if m.ViewID != nil {
		pid := storageabstraction.DefinitionId(m.ViewID.String())
		parent = &pid
	}
	created := now
	updated := now
	return storageabstraction.DefinitionRecord{
		Kind:        storageabstraction.DefinitionKind(mapKind),
		ID:          storageabstraction.DefinitionId(m.ID.String()),
		Tenant:      &tenant,
		ParentID:    parent,
		Version:     &version,
		Payload:     payload,
		CreatedAtMs: &created,
		UpdatedAtMs: &updated,
	}, nil
}

func mapFromRecord(record storageabstraction.DefinitionRecord) (models.ExploratoryMap, error) {
	id, err := uuid.Parse(string(record.ID))
	if err != nil {
		return models.ExploratoryMap{}, fmt.Errorf("stored map id is not a UUID: %w", err)
	}
	var viewID *uuid.UUID
	switch {
	case record.ParentID != nil:
		parsed, err := uuid.Parse(string(*record.ParentID))
		if err != nil {
			return models.ExploratoryMap{}, fmt.Errorf("stored view_id is not a UUID: %w", err)
		}
		viewID = &parsed
	default:
		// Fall back to payload `view_id` when ParentID is missing
		// (matches Rust's `.or_else(|| record.payload.get("view_id"))`).
		raw := payloadField(record.Payload, "view_id")
		if len(raw) > 0 && string(raw) != "null" {
			var s string
			if err := json.Unmarshal(raw, &s); err == nil && s != "" {
				parsed, err := uuid.Parse(s)
				if err != nil {
					return models.ExploratoryMap{}, fmt.Errorf("stored view_id is not a UUID: %w", err)
				}
				viewID = &parsed
			}
		}
	}
	name, err := requiredString(record.Payload, "name")
	if err != nil {
		return models.ExploratoryMap{}, err
	}
	kind, err := requiredString(record.Payload, "map_kind")
	if err != nil {
		return models.ExploratoryMap{}, err
	}
	config := payloadField(record.Payload, "config")
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	createdMs := pickTimestamp(record.CreatedAtMs, record.UpdatedAtMs)
	return models.ExploratoryMap{
		ID:        id,
		ViewID:    viewID,
		Name:      name,
		MapKind:   kind,
		Config:    config,
		CreatedAt: datetimeFromMs(createdMs),
	}, nil
}
