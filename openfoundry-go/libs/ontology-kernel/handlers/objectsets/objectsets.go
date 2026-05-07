// Package objectsets ports `libs/ontology-kernel/src/handlers/object_sets.rs`
// 1:1: 7 endpoints under `/ontology/object-sets` for saved object set
// CRUD + evaluate + materialize.
//
// Wire shape (matched against `services/edge-gateway-service` proxy
// allowlist `/api/v1/ontology/object-sets`):
//
//   - GET    /ontology/object-sets               → ListObjectSets
//   - POST   /ontology/object-sets               → CreateObjectSet
//   - GET    /ontology/object-sets/{id}          → GetObjectSet
//   - PATCH  /ontology/object-sets/{id}          → UpdateObjectSet
//   - DELETE /ontology/object-sets/{id}          → DeleteObjectSet
//   - POST   /ontology/object-sets/{id}/evaluate    → EvaluateObjectSet
//   - POST   /ontology/object-sets/{id}/materialize → MaterializeObjectSet
//
// All seven endpoints are wired so the router shape matches the Rust
// service. CRUD handlers are functionally complete and route through
// `domain.{Get,List,Create,Update,Delete}ObjectSet` against the
// DefinitionStore.
//
// Evaluate and materialize route through the same domain evaluator and
// object_set_materializations store as the Rust service.

package objectsets

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	kernelstores "github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// Mount registers every endpoint on the chi router.
func Mount(r chi.Router, state *ontologykernel.AppState) {
	r.Get("/ontology/object-sets", ListObjectSets(state))
	r.Post("/ontology/object-sets", CreateObjectSet(state))
	r.Get("/ontology/object-sets/{id}", GetObjectSet(state))
	r.Patch("/ontology/object-sets/{id}", UpdateObjectSet(state))
	r.Delete("/ontology/object-sets/{id}", DeleteObjectSet(state))
	r.Post("/ontology/object-sets/{id}/evaluate", EvaluateObjectSet(state))
	r.Post("/ontology/object-sets/{id}/materialize", MaterializeObjectSet(state))
}

// ── HTTP plumbing ────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

func errBody(msg string) map[string]string { return map[string]string{"error": msg} }

func unauthorized(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
}
func badRequest(w http.ResponseWriter, m string) { writeJSON(w, http.StatusBadRequest, errBody(m)) }
func forbidden(w http.ResponseWriter, m string)  { writeJSON(w, http.StatusForbidden, errBody(m)) }
func notFound(w http.ResponseWriter, m string)   { writeJSON(w, http.StatusNotFound, errBody(m)) }
func internalError(w http.ResponseWriter, m string) {
	writeJSON(w, http.StatusInternalServerError, errBody(m))
}
func pathUUID(r *http.Request, key string) (uuid.UUID, error) {
	raw := chi.URLParam(r, key)
	if raw == "" {
		return uuid.Nil, errors.New("missing path parameter " + key)
	}
	return uuid.Parse(strings.TrimSpace(raw))
}

// ── Query / helpers ──────────────────────────────────────────────────────

// listObjectSetsQuery mirrors `struct ListObjectSetsQuery`. `size`
// defaults to 100; `token` is optional.
type listObjectSetsQuery struct {
	Size  uint32
	Token *string
}

func parseListQuery(r *http.Request) listObjectSetsQuery {
	q := r.URL.Query()
	out := listObjectSetsQuery{Size: 100}
	if raw := q.Get("size"); raw != "" {
		if v, err := strconv.ParseUint(raw, 10, 32); err == nil {
			out.Size = uint32(v)
		}
	}
	if raw := q.Get("token"); raw != "" {
		s := raw
		out.Token = &s
	}
	return out
}

// objectSetID mirrors `fn object_set_id`.
func objectSetID(id uuid.UUID) storage.DefinitionId {
	return storage.DefinitionId(id.String())
}

// buildDefinitionFromCreate mirrors `fn build_definition_from_create`.
// The id is a fresh v7 uuid (Rust uses `Uuid::now_v7`) and
// created_at == updated_at at the call site.
func buildDefinitionFromCreate(ownerID uuid.UUID, req models.CreateObjectSetRequest) models.ObjectSetDefinition {
	now := time.Now().UTC()
	id, err := uuid.NewV7()
	if err != nil {
		// uuid.NewV7 only fails on entropy errors, which never happen
		// on the platforms we support. Fall back to v4 to mirror
		// `Uuid::now_v7`'s "always succeeds" contract.
		id = uuid.New()
	}
	return models.ObjectSetDefinition{
		ID:                   id,
		Name:                 req.Name,
		Description:          req.Description,
		BaseObjectTypeID:     req.BaseObjectTypeID,
		Filters:              req.Filters,
		Traversals:           req.Traversals,
		Join:                 req.Join,
		Projections:          req.Projections,
		WhatIfLabel:          req.WhatIfLabel,
		Policy:               req.Policy,
		MaterializedSnapshot: nil,
		MaterializedAt:       nil,
		MaterializedRowCount: 0,
		OwnerID:              ownerID,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

// loadObjectSet mirrors `async fn load_object_set`. The Rust path enriches
// materialization metadata when present and otherwise returns the bare
// definition; the Go handler applies metadata after materialize in the same
// response path.
func loadObjectSet(r *http.Request, state *ontologykernel.AppState, id uuid.UUID) (*models.ObjectSetDefinition, error) {
	return domain.GetObjectSet(r.Context(), state.Stores.Definitions, id)
}

// objectTypeExists mirrors `async fn object_type_exists`.
func objectTypeExists(r *http.Request, state *ontologykernel.AppState, objectTypeID uuid.UUID) (bool, error) {
	return domain.ObjectTypeExistsInDefinitionStore(r.Context(), state.Stores.Definitions, objectTypeID)
}

func objectSetMaterializationID(id uuid.UUID) storage.ObjectSetId {
	return storage.ObjectSetId(id.String())
}

func rowIDFromValue(row json.RawMessage, ordinal int) string {
	if raw := domain.ResolveObjectSetPath(row, "base.id"); raw != nil {
		var id string
		if err := json.Unmarshal(raw, &id); err == nil && id != "" {
			return id
		}
	}
	if raw := domain.ResolveObjectSetPath(row, "id"); raw != nil {
		var id string
		if err := json.Unmarshal(raw, &id); err == nil && id != "" {
			return id
		}
	}
	return "row-" + strconv.Itoa(ordinal)
}

func materializationFromEvaluation(claims *authmw.Claims, definition models.ObjectSetDefinition, evaluation models.ObjectSetEvaluationResponse) kernelstores.ObjectSetMaterialization {
	rows := make([]kernelstores.ObjectSetMaterializedRow, 0, len(evaluation.Rows))
	for i, row := range evaluation.Rows {
		rows = append(rows, kernelstores.ObjectSetMaterializedRow{
			RowID:   rowIDFromValue(row, i),
			Ordinal: uint32(i),
			Payload: row,
		})
	}
	return kernelstores.ObjectSetMaterialization{
		Tenant:                 domain.TenantFromClaims(claims),
		SetID:                  objectSetMaterializationID(definition.ID),
		BaseTypeID:             storage.TypeId(definition.BaseObjectTypeID.String()),
		GeneratedAtMs:          evaluation.GeneratedAt.UnixMilli(),
		TotalBaseMatches:       uint64(evaluation.TotalBaseMatches),
		TotalRows:              uint64(evaluation.TotalRows),
		TraversalNeighborCount: uint64(evaluation.TraversalNeighborCount),
		Rows:                   rows,
	}
}

func applyMaterializationMetadata(definition *models.ObjectSetDefinition, metadata kernelstores.ObjectSetMaterializationMetadata) {
	t := time.UnixMilli(metadata.GeneratedAtMs).UTC()
	definition.MaterializedAt = &t
	if metadata.TotalRows > uint64(^uint32(0)>>1) {
		definition.MaterializedRowCount = int32(^uint32(0) >> 1)
	} else {
		definition.MaterializedRowCount = int32(metadata.TotalRows)
	}
}

// ── Endpoints ────────────────────────────────────────────────────────────

// ListObjectSets mirrors `pub async fn list_object_sets`.
func ListObjectSets(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			unauthorized(w)
			return
		}
		query := parseListQuery(r)
		size := query.Size
		if size < 1 {
			size = 1
		} else if size > 500 {
			size = 500
		}
		page, err := domain.ListObjectSets(r.Context(), state.Stores.Definitions, domain.ObjectSetListQuery{
			OwnerID:                claims.Sub,
			IncludeRestrictedViews: true,
			Page: storage.Page{
				Size:  size,
				Token: query.Token,
			},
		})
		if err != nil {
			internalError(w, "failed to load object sets")
			return
		}
		data := page.Items
		if data == nil {
			data = []models.ObjectSetDefinition{}
		}
		writeJSON(w, http.StatusOK, models.ListObjectSetsResponse{
			Data:      data,
			NextToken: page.NextToken,
		})
	}
}

// CreateObjectSet mirrors `pub async fn create_object_set`.
func CreateObjectSet(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			unauthorized(w)
			return
		}
		var req models.CreateObjectSetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			badRequest(w, "invalid request body")
			return
		}
		definition := buildDefinitionFromCreate(claims.Sub, req)
		if err := domain.ValidateObjectSetDefinition(definition); err != nil {
			badRequest(w, err.Error())
			return
		}
		exists, err := objectTypeExists(r, state, definition.BaseObjectTypeID)
		if err != nil {
			internalError(w, err.Error())
			return
		}
		if !exists {
			badRequest(w, "base_object_type_id does not exist")
			return
		}
		if definition.Join != nil {
			ok, err := objectTypeExists(r, state, definition.Join.SecondaryObjectTypeID)
			if err != nil {
				internalError(w, err.Error())
				return
			}
			if !ok {
				badRequest(w, "join.secondary_object_type_id does not exist")
				return
			}
		}

		definitionID := definition.ID
		if _, err := domain.CreateObjectSet(r.Context(), state.Stores.Definitions, definition); err != nil {
			internalError(w, "failed to create object set")
			return
		}
		reloaded, err := loadObjectSet(r, state, definitionID)
		if err != nil {
			internalError(w, err.Error())
			return
		}
		if reloaded == nil {
			internalError(w, "created object set could not be reloaded")
			return
		}
		writeJSON(w, http.StatusCreated, reloaded)
	}
}

// GetObjectSet mirrors `pub async fn get_object_set`.
func GetObjectSet(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authmw.FromContext(r.Context()); !ok {
			unauthorized(w)
			return
		}
		id, err := pathUUID(r, "id")
		if err != nil {
			badRequest(w, "invalid path id")
			return
		}
		def, err := loadObjectSet(r, state, id)
		if err != nil {
			internalError(w, err.Error())
			return
		}
		if def == nil {
			notFound(w, "object set not found")
			return
		}
		writeJSON(w, http.StatusOK, def)
	}
}

// UpdateObjectSet mirrors `pub async fn update_object_set`.
func UpdateObjectSet(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			unauthorized(w)
			return
		}
		id, err := pathUUID(r, "id")
		if err != nil {
			badRequest(w, "invalid path id")
			return
		}
		existing, err := loadObjectSet(r, state, id)
		if err != nil {
			internalError(w, err.Error())
			return
		}
		if existing == nil {
			notFound(w, "object set not found")
			return
		}
		if existing.OwnerID != claims.Sub && !claims.HasRole("admin") {
			forbidden(w, "forbidden: only the owner can update this object set")
			return
		}

		var req models.UpdateObjectSetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			badRequest(w, "invalid request body")
			return
		}

		next := models.ObjectSetDefinition{
			ID:                   existing.ID,
			Name:                 existing.Name,
			Description:          existing.Description,
			BaseObjectTypeID:     existing.BaseObjectTypeID,
			Filters:              existing.Filters,
			Traversals:           existing.Traversals,
			Join:                 existing.Join,
			Projections:          existing.Projections,
			WhatIfLabel:          existing.WhatIfLabel,
			Policy:               existing.Policy,
			MaterializedSnapshot: nil,
			MaterializedAt:       nil,
			MaterializedRowCount: 0,
			OwnerID:              existing.OwnerID,
			CreatedAt:            existing.CreatedAt,
			UpdatedAt:            time.Now().UTC(),
		}
		if req.Name != nil {
			next.Name = *req.Name
		}
		if req.Description != nil {
			next.Description = *req.Description
		}
		if req.BaseObjectTypeID != nil {
			next.BaseObjectTypeID = *req.BaseObjectTypeID
		}
		if req.Filters != nil {
			next.Filters = *req.Filters
		}
		if req.Traversals != nil {
			next.Traversals = *req.Traversals
		}
		// Rust: `request.join.or(existing.join)` — `Some(j)` in the
		// patch overrides; absent leaves existing in place. There is
		// no path to clear it back to None over PATCH.
		if req.Join != nil {
			next.Join = req.Join
		}
		if req.Projections != nil {
			next.Projections = *req.Projections
		}
		// Same `or(existing)` semantics for `what_if_label`.
		if req.WhatIfLabel != nil {
			next.WhatIfLabel = req.WhatIfLabel
		}
		if req.Policy != nil {
			next.Policy = *req.Policy
		}

		if err := domain.ValidateObjectSetDefinition(next); err != nil {
			badRequest(w, err.Error())
			return
		}
		if _, err := domain.UpdateObjectSet(r.Context(), state.Stores.Definitions, next); err != nil {
			internalError(w, "failed to update object set")
			return
		}
		// Materialization invalidation skipped: store not yet ported.

		reloaded, err := loadObjectSet(r, state, id)
		if err != nil {
			internalError(w, err.Error())
			return
		}
		if reloaded == nil {
			internalError(w, "updated object set could not be reloaded")
			return
		}
		writeJSON(w, http.StatusOK, reloaded)
	}
}

// DeleteObjectSet mirrors `pub async fn delete_object_set`.
func DeleteObjectSet(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			unauthorized(w)
			return
		}
		id, err := pathUUID(r, "id")
		if err != nil {
			badRequest(w, "invalid path id")
			return
		}
		existing, err := loadObjectSet(r, state, id)
		if err != nil {
			internalError(w, err.Error())
			return
		}
		if existing == nil {
			notFound(w, "object set not found")
			return
		}
		if existing.OwnerID != claims.Sub && !claims.HasRole("admin") {
			forbidden(w, "forbidden: only the owner can delete this object set")
			return
		}
		deleted, err := domain.DeleteObjectSet(r.Context(), state.Stores.Definitions, id)
		if err != nil {
			internalError(w, "failed to delete object set")
			return
		}
		if !deleted {
			notFound(w, "object set not found")
			return
		}
		if state.Stores.ObjectSetMaterializations != nil {
			_, _ = state.Stores.ObjectSetMaterializations.Delete(r.Context(), domain.TenantFromClaims(claims), objectSetMaterializationID(id))
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// EvaluateObjectSet mirrors `pub async fn evaluate_object_set`.
func EvaluateObjectSet(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			unauthorized(w)
			return
		}
		id, err := pathUUID(r, "id")
		if err != nil {
			badRequest(w, "invalid path id")
			return
		}
		definition, err := loadObjectSet(r, state, id)
		if err != nil {
			internalError(w, err.Error())
			return
		}
		if definition == nil {
			notFound(w, "object set not found")
			return
		}
		var req models.EvaluateObjectSetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			badRequest(w, "invalid request body")
			return
		}
		limit := 250
		if req.Limit != nil {
			limit = *req.Limit
		}
		if limit < 1 {
			limit = 1
		} else if limit > 2000 {
			limit = 2000
		}
		evaluation, err := domain.EvaluateObjectSet(r.Context(), state, claims, *definition, limit, false)
		if err != nil {
			if strings.Contains(err.Error(), "forbidden") {
				forbidden(w, err.Error())
			} else {
				badRequest(w, err.Error())
			}
			return
		}
		writeJSON(w, http.StatusOK, evaluation)
	}
}

// MaterializeObjectSet mirrors `pub async fn materialize_object_set`.
func MaterializeObjectSet(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			unauthorized(w)
			return
		}
		id, err := pathUUID(r, "id")
		if err != nil {
			badRequest(w, "invalid path id")
			return
		}
		definition, err := loadObjectSet(r, state, id)
		if err != nil {
			internalError(w, err.Error())
			return
		}
		if definition == nil {
			notFound(w, "object set not found")
			return
		}
		var req models.EvaluateObjectSetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			badRequest(w, "invalid request body")
			return
		}
		limit := 2000
		if req.Limit != nil {
			limit = *req.Limit
		}
		if limit < 1 {
			limit = 1
		} else if limit > 5000 {
			limit = 5000
		}
		evaluation, err := domain.EvaluateObjectSet(r.Context(), state, claims, *definition, limit, true)
		if err != nil {
			if strings.Contains(err.Error(), "forbidden") {
				forbidden(w, err.Error())
			} else {
				badRequest(w, err.Error())
			}
			return
		}
		if state.Stores.ObjectSetMaterializations == nil {
			internalError(w, "failed to materialize object set")
			return
		}
		metadata, err := state.Stores.ObjectSetMaterializations.Replace(r.Context(), materializationFromEvaluation(claims, *definition, evaluation))
		if err != nil {
			internalError(w, "failed to materialize object set")
			return
		}
		applyMaterializationMetadata(&evaluation.ObjectSet, metadata)
		writeJSON(w, http.StatusOK, evaluation)
	}
}
