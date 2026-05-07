package objectsets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

func sampleClaims() *authmw.Claims {
	return &authmw.Claims{Sub: uuid.Nil, Email: "test@example.com"}
}

func withClaims(claims *authmw.Claims, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := authmw.ContextWithClaims(r.Context(), claims)
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newStateWithDefinitions() (*ontologykernel.AppState, *stores.InMemoryDefinitionStore) {
	defs := stores.NewInMemoryDefinitionStore()
	bag := stores.NewInMemory()
	bag.Definitions = defs
	bag.Search = nil
	state := &ontologykernel.AppState{Stores: bag}
	return state, defs
}

func seedObjectType(t *testing.T, store *stores.InMemoryDefinitionStore, typeID uuid.UUID) {
	t.Helper()
	now := time.Now().UTC()
	createdMs := now.UnixMilli()
	updatedMs := now.UnixMilli()
	version := uint64(updatedMs)
	payload := map[string]any{
		"id":           typeID,
		"name":         "aircraft",
		"display_name": "Aircraft",
		"created_at":   now.Format(time.RFC3339),
		"updated_at":   now.Format(time.RFC3339),
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	_, err = store.Put(context.Background(), storage.DefinitionRecord{
		Kind:        storage.DefinitionKind("object_type"),
		ID:          storage.DefinitionId(typeID.String()),
		Version:     &version,
		Payload:     body,
		CreatedAtMs: &createdMs,
		UpdatedAtMs: &updatedMs,
	}, nil)
	require.NoError(t, err)
}

// libs/ontology-kernel/src/handlers/object_sets.rs — every endpoint
// returns 401 when the request carries no Claims.
func TestEndpointsRequireClaims(t *testing.T) {
	state, _ := newStateWithDefinitions()
	id := uuid.New().String()
	cases := []struct {
		method, path, body string
		fn                 http.HandlerFunc
	}{
		{http.MethodGet, "/ontology/object-sets", ``, ListObjectSets(state)},
		{http.MethodPost, "/ontology/object-sets", `{}`, CreateObjectSet(state)},
		{http.MethodGet, "/ontology/object-sets/" + id, ``, GetObjectSet(state)},
		{http.MethodPatch, "/ontology/object-sets/" + id, `{}`, UpdateObjectSet(state)},
		{http.MethodDelete, "/ontology/object-sets/" + id, ``, DeleteObjectSet(state)},
		{http.MethodPost, "/ontology/object-sets/" + id + "/evaluate", `{}`, EvaluateObjectSet(state)},
		{http.MethodPost, "/ontology/object-sets/" + id + "/materialize", `{}`, MaterializeObjectSet(state)},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		rec := httptest.NewRecorder()
		tc.fn.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "%s %s", tc.method, tc.path)
	}
}

// libs/ontology-kernel/src/handlers/object_sets.rs — Mount registers
// every documented route at the documented path / verb.
func TestMountRegistersEveryRoute(t *testing.T) {
	r := chi.NewRouter()
	state, _ := newStateWithDefinitions()
	Mount(r, state)

	got := map[string]bool{}
	_ = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		got[method+" "+route] = true
		return nil
	})
	want := []string{
		"GET /ontology/object-sets",
		"POST /ontology/object-sets",
		"GET /ontology/object-sets/{id}",
		"PATCH /ontology/object-sets/{id}",
		"DELETE /ontology/object-sets/{id}",
		"POST /ontology/object-sets/{id}/evaluate",
		"POST /ontology/object-sets/{id}/materialize",
	}
	for _, key := range want {
		assert.True(t, got[key], "missing route: %s", key)
	}
}

// libs/ontology-kernel/src/handlers/object_sets.rs `create_object_set`
// — invalid body → 400 verbatim.
func TestCreateObjectSetRejectsInvalidBody(t *testing.T) {
	state, _ := newStateWithDefinitions()
	req := httptest.NewRequest(http.MethodPost, "/ontology/object-sets", strings.NewReader(`{not json}`))
	rec := httptest.NewRecorder()
	withClaims(sampleClaims(), CreateObjectSet(state)).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid request body")
}

// libs/ontology-kernel/src/handlers/object_sets.rs `create_object_set`
// — missing name surfaces validation error verbatim.
func TestCreateObjectSetRejectsMissingName(t *testing.T) {
	state, defs := newStateWithDefinitions()
	typeID := uuid.New()
	seedObjectType(t, defs, typeID)

	body, err := json.Marshal(models.CreateObjectSetRequest{
		Name:             "  ",
		BaseObjectTypeID: typeID,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/ontology/object-sets", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	withClaims(sampleClaims(), CreateObjectSet(state)).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "name is required")
}

// libs/ontology-kernel/src/handlers/object_sets.rs `create_object_set`
// — references a non-existent base type → 400 verbatim.
func TestCreateObjectSetRejectsUnknownBaseType(t *testing.T) {
	state, _ := newStateWithDefinitions()
	body, err := json.Marshal(models.CreateObjectSetRequest{
		Name:             "active",
		BaseObjectTypeID: uuid.New(),
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/ontology/object-sets", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	withClaims(sampleClaims(), CreateObjectSet(state)).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "base_object_type_id does not exist")
}

// libs/ontology-kernel/src/handlers/object_sets.rs — full
// create→get→list→delete roundtrip via the in-memory definition
// store. Mirrors the Rust `create_list_paginate_evaluate_and_delete`
// happy path (sans evaluate, which requires function_runtime).
func TestCreateGetListDeleteRoundtrip(t *testing.T) {
	state, defs := newStateWithDefinitions()
	typeID := uuid.New()
	seedObjectType(t, defs, typeID)

	owner := uuid.New()
	claims := &authmw.Claims{Sub: owner, Email: "owner@example.com"}

	// Create.
	body, err := json.Marshal(models.CreateObjectSetRequest{
		Name:             "active",
		Description:      "saved set",
		BaseObjectTypeID: typeID,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/ontology/object-sets", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	withClaims(claims, CreateObjectSet(state)).ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created models.ObjectSetDefinition
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, "active", created.Name)
	assert.Equal(t, owner, created.OwnerID)

	// Get.
	r := chi.NewRouter()
	r.Get("/ontology/object-sets/{id}", GetObjectSet(state))
	getReq := httptest.NewRequest(http.MethodGet, "/ontology/object-sets/"+created.ID.String(), nil)
	getRec := httptest.NewRecorder()
	withClaims(claims, r).ServeHTTP(getRec, getReq)
	assert.Equal(t, http.StatusOK, getRec.Code)

	// List.
	listReq := httptest.NewRequest(http.MethodGet, "/ontology/object-sets", nil)
	listRec := httptest.NewRecorder()
	withClaims(claims, ListObjectSets(state)).ServeHTTP(listRec, listReq)
	require.Equal(t, http.StatusOK, listRec.Code)
	var listed models.ListObjectSetsResponse
	require.NoError(t, json.Unmarshal(listRec.Body.Bytes(), &listed))
	require.Len(t, listed.Data, 1)
	assert.Equal(t, created.ID, listed.Data[0].ID)

	// Delete.
	delRouter := chi.NewRouter()
	delRouter.Delete("/ontology/object-sets/{id}", DeleteObjectSet(state))
	delReq := httptest.NewRequest(http.MethodDelete, "/ontology/object-sets/"+created.ID.String(), nil)
	delRec := httptest.NewRecorder()
	withClaims(claims, delRouter).ServeHTTP(delRec, delReq)
	assert.Equal(t, http.StatusNoContent, delRec.Code)

	// Get after delete → 404.
	missReq := httptest.NewRequest(http.MethodGet, "/ontology/object-sets/"+created.ID.String(), nil)
	missRec := httptest.NewRecorder()
	withClaims(claims, r).ServeHTTP(missRec, missReq)
	assert.Equal(t, http.StatusNotFound, missRec.Code)
	assert.Contains(t, missRec.Body.String(), "object set not found")
}

// libs/ontology-kernel/src/handlers/object_sets.rs `delete_object_set`
// — non-owner without admin role is forbidden, even when the set
// exists.
func TestDeleteObjectSetRejectsNonOwner(t *testing.T) {
	state, defs := newStateWithDefinitions()
	typeID := uuid.New()
	seedObjectType(t, defs, typeID)

	owner := uuid.New()
	body, err := json.Marshal(models.CreateObjectSetRequest{
		Name:             "active",
		BaseObjectTypeID: typeID,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/ontology/object-sets", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	withClaims(&authmw.Claims{Sub: owner, Email: "o@e"}, CreateObjectSet(state)).ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created models.ObjectSetDefinition
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	// Different subject, no admin role.
	stranger := &authmw.Claims{Sub: uuid.New(), Email: "x@y"}

	r := chi.NewRouter()
	r.Delete("/ontology/object-sets/{id}", DeleteObjectSet(state))
	delReq := httptest.NewRequest(http.MethodDelete, "/ontology/object-sets/"+created.ID.String(), nil)
	delRec := httptest.NewRecorder()
	withClaims(stranger, r).ServeHTTP(delRec, delReq)
	assert.Equal(t, http.StatusForbidden, delRec.Code)
	assert.Contains(t, delRec.Body.String(), "forbidden: only the owner can delete this object set")
}

func seedStoredObject(t *testing.T, state *ontologykernel.AppState, tenant storage.TenantId, typeID uuid.UUID, id uuid.UUID, props string, markings ...storage.MarkingId) {
	t.Helper()
	if len(markings) == 0 {
		markings = []storage.MarkingId{"public"}
	}
	now := time.Now().UTC().UnixMilli()
	_, err := state.Stores.Objects.Put(context.Background(), storage.Object{
		Tenant:      tenant,
		ID:          storage.ObjectId(id.String()),
		TypeID:      storage.TypeId(typeID.String()),
		Payload:     json.RawMessage(props),
		UpdatedAtMs: now,
		Markings:    markings,
	}, nil)
	require.NoError(t, err)
}

func createObjectSetForEval(t *testing.T, state *ontologykernel.AppState, defs *stores.InMemoryDefinitionStore, typeID uuid.UUID, filters []models.ObjectSetFilter) models.ObjectSetDefinition {
	t.Helper()
	seedObjectType(t, defs, typeID)
	body, err := json.Marshal(models.CreateObjectSetRequest{
		Name:             "eval-set",
		BaseObjectTypeID: typeID,
		Filters:          filters,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/ontology/object-sets", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	withClaims(sampleClaims(), CreateObjectSet(state)).ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var def models.ObjectSetDefinition
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &def))
	return def
}

// libs/ontology-kernel/src/handlers/object_sets.rs `evaluate_object_set` —
// invalid filter input is validated by the evaluator and surfaces 400.
func TestEvaluateObjectSetRejectsInvalidInput(t *testing.T) {
	state, defs := newStateWithDefinitions()
	typeID := uuid.New()
	def := createObjectSetForEval(t, state, defs, typeID, nil)
	def.Filters = []models.ObjectSetFilter{{Field: "status", Operator: "bogus", Value: json.RawMessage(`"active"`)}}
	record, err := domain.ObjectSetDefinitionToRecord(def)
	require.NoError(t, err)
	_, err = defs.Put(context.Background(), record, nil)
	require.NoError(t, err)
	r := chi.NewRouter()
	r.Post("/ontology/object-sets/{id}/evaluate", EvaluateObjectSet(state))
	req := httptest.NewRequest(http.MethodPost, "/ontology/object-sets/"+def.ID.String()+"/evaluate", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	withClaims(sampleClaims(), r).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "unsupported filter operator")
}

func TestEvaluateObjectSetEmpty(t *testing.T) {
	state, defs := newStateWithDefinitions()
	typeID := uuid.New()
	def := createObjectSetForEval(t, state, defs, typeID, nil)
	r := chi.NewRouter()
	r.Post("/ontology/object-sets/{id}/evaluate", EvaluateObjectSet(state))
	req := httptest.NewRequest(http.MethodPost, "/ontology/object-sets/"+def.ID.String()+"/evaluate", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	withClaims(sampleClaims(), r).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got models.ObjectSetEvaluationResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, 0, got.TotalBaseMatches)
	assert.Equal(t, 0, got.TotalRows)
	assert.Empty(t, got.Rows)
	assert.False(t, got.Materialized)
}

func TestEvaluateObjectSetWithSimpleFilters(t *testing.T) {
	state, defs := newStateWithDefinitions()
	typeID := uuid.New()
	def := createObjectSetForEval(t, state, defs, typeID, []models.ObjectSetFilter{{Field: "status", Operator: "equals", Value: json.RawMessage(`"active"`)}})
	seedStoredObject(t, state, "default", typeID, uuid.New(), `{"status":"active","name":"A"}`)
	seedStoredObject(t, state, "default", typeID, uuid.New(), `{"status":"inactive","name":"B"}`)
	r := chi.NewRouter()
	r.Post("/ontology/object-sets/{id}/evaluate", EvaluateObjectSet(state))
	req := httptest.NewRequest(http.MethodPost, "/ontology/object-sets/"+def.ID.String()+"/evaluate", strings.NewReader(`{"limit":10}`))
	rec := httptest.NewRecorder()
	withClaims(sampleClaims(), r).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got models.ObjectSetEvaluationResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, 1, got.TotalBaseMatches)
	assert.Equal(t, 1, got.TotalRows)
	require.Len(t, got.Rows, 1)
	assert.JSONEq(t, `"active"`, string(domain.ResolveObjectSetPath(got.Rows[0], "base.properties.status")))
}

func TestMaterializeObjectSetPersistsMetadata(t *testing.T) {
	state, defs := newStateWithDefinitions()
	typeID := uuid.New()
	def := createObjectSetForEval(t, state, defs, typeID, nil)
	objectID := uuid.New()
	seedStoredObject(t, state, "default", typeID, objectID, `{"status":"active"}`)
	r := chi.NewRouter()
	r.Post("/ontology/object-sets/{id}/materialize", MaterializeObjectSet(state))
	req := httptest.NewRequest(http.MethodPost, "/ontology/object-sets/"+def.ID.String()+"/materialize", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	withClaims(sampleClaims(), r).ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got models.ObjectSetEvaluationResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.True(t, got.Materialized)
	require.NotNil(t, got.ObjectSet.MaterializedAt)
	assert.Equal(t, int32(1), got.ObjectSet.MaterializedRowCount)
	meta, err := state.Stores.ObjectSetMaterializations.GetMetadata(context.Background(), "default", storage.ObjectSetId(def.ID.String()), storage.Strong())
	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.Equal(t, uint64(1), meta.StoredRowCount)
}

func TestEvaluateObjectSetBackendErrors(t *testing.T) {
	state, defs := newStateWithDefinitions()
	typeID := uuid.New()
	def := createObjectSetForEval(t, state, defs, typeID, nil)
	mockObjects := stores.NewMockObjectStore()
	mockObjects.QueueListByType(storage.PagedResult[storage.Object]{}, storage.Backend("boom"))
	state.Stores.Objects = mockObjects
	r := chi.NewRouter()
	r.Post("/ontology/object-sets/{id}/evaluate", EvaluateObjectSet(state))
	req := httptest.NewRequest(http.MethodPost, "/ontology/object-sets/"+def.ID.String()+"/evaluate", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	withClaims(sampleClaims(), r).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "boom")
}

// libs/ontology-kernel/src/handlers/object_sets.rs — invalid path
// id surfaces 400 verbatim.
func TestInvalidPathIDRejected(t *testing.T) {
	state, _ := newStateWithDefinitions()
	r := chi.NewRouter()
	r.Get("/ontology/object-sets/{id}", GetObjectSet(state))
	req := httptest.NewRequest(http.MethodGet, "/ontology/object-sets/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	withClaims(sampleClaims(), r).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid path id")
}

// Wire shape: ListObjectSetsResponse marshals as the Rust shape.
func TestListObjectSetsResponseShape(t *testing.T) {
	resp := models.ListObjectSetsResponse{
		Data: []models.ObjectSetDefinition{},
	}
	b, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Equal(t, `{"data":[]}`, string(b))
}

// parseListQuery mirrors `ListObjectSetsQuery::default + size=...`.
// The handler then clamps to [1, 500]; here we pin only the
// parser shape (default 100, optional token).
func TestParseListQueryDefaults(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ontology/object-sets", nil)
	q := parseListQuery(r)
	assert.Equal(t, uint32(100), q.Size)
	assert.Nil(t, q.Token)

	r = httptest.NewRequest(http.MethodGet, "/ontology/object-sets?size=50&token=abc", nil)
	q = parseListQuery(r)
	assert.Equal(t, uint32(50), q.Size)
	if assert.NotNil(t, q.Token) {
		assert.Equal(t, "abc", *q.Token)
	}
}
