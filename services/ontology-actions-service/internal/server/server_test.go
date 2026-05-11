package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	kernelactions "github.com/openfoundry/openfoundry-go/libs/ontology-kernel/handlers/actions"
	ontologymetrics "github.com/openfoundry/openfoundry-go/libs/ontology-kernel/metrics"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
	"github.com/openfoundry/openfoundry-go/services/ontology-actions-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/ontology-actions-service/internal/server"
)

const testJWTSecret = "ontology-actions-service-smoke-secret-do-not-use-in-prod"

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	cfg := &config.Config{}
	cfg.Service.Name = "ontology-actions-service"
	cfg.Service.Version = "test"
	cfg.JWTSecret = testJWTSecret
	state := &ontologykernel.AppState{Stores: stores.NewInMemory()}
	return server.BuildRouter(cfg, state, nil)
}

func TestBuildRouterRequiresAppState(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	cfg.Service.Name = "ontology-actions-service"
	cfg.Service.Version = "test"
	cfg.JWTSecret = testJWTSecret
	defer func() {
		if recover() == nil {
			t.Fatal("expected BuildRouter to panic without AppState")
		}
	}()
	_ = server.BuildRouter(cfg, nil, nil)
}

func devToken(t *testing.T) string {
	t.Helper()
	now := time.Now()
	cfg := authmw.NewJWTConfig(testJWTSecret)
	tok, err := authmw.EncodeToken(cfg, &authmw.Claims{
		Sub:   uuid.New(),
		IAT:   now.Unix(),
		EXP:   now.Add(time.Hour).Unix(),
		JTI:   uuid.New(),
		Email: "smoke@openfoundry.test",
		Name:  "Smoke Tester",
		Roles: []string{"ontology.editor"},
	})
	if err != nil {
		t.Fatalf("encode dev token: %v", err)
	}
	return tok
}

func TestListActionTypesRequiresBearerToken(t *testing.T) {
	t.Parallel()
	router := newTestRouter(t)

	// 1. No token → 401.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ontology/actions", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req.WithContext(context.Background()))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	// 2. Token → 200 + envelope.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/ontology/actions", nil)
	req.Header.Set("Authorization", "Bearer "+devToken(t))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	data, ok := body["data"].([]any)
	if !ok {
		t.Fatalf("expected `data` array, got %v", body)
	}
	if len(data) != 0 {
		t.Fatalf("expected empty data array, got %d entries", len(data))
	}
	if total, _ := body["total"].(float64); total != 0 {
		t.Fatalf("expected total=0, got %v", body["total"])
	}
}

// Mirrors `absorbed_routes_require_bearer_token`.
func TestAbsorbedRoutesRequireBearerToken(t *testing.T) {
	t.Parallel()
	router := newTestRouter(t)
	for _, path := range []string{
		"/api/v1/ontology/funnel/sources",
		"/api/v1/ontology/storage/insights",
		"/api/v1/ontology/functions",
		"/api/v1/ontology/rules",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401, got %d", path, rec.Code)
		}
	}
}

func TestHealthIsPublic(t *testing.T) {
	t.Parallel()
	router := newTestRouter(t)
	for _, path := range []string{"/health", "/healthz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", path, rec.Code)
		}
	}
}

func TestMetricsEndpointRegistersActionCollectors(t *testing.T) {
	cfg := &config.Config{}
	cfg.Service.Name = "ontology-actions-service"
	cfg.Service.Version = "test"
	cfg.JWTSecret = testJWTSecret
	state := &ontologykernel.AppState{Stores: stores.NewInMemory()}
	m := observability.NewMetrics()
	router := server.BuildRouter(cfg, state, m)
	if actionMetrics := ontologymetrics.ActionMetricsSingleton(); actionMetrics != nil {
		actionMetrics.RecordSuccess("metrics-smoke", 0.001)
		actionMetrics.RecordFailure("metrics-smoke", ontologymetrics.FailureTypeInvalidParameter, 0.002)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "action_executions_total") || !strings.Contains(rec.Body.String(), "action_execution_duration_seconds") || !strings.Contains(rec.Body.String(), "action_failures_total") {
		t.Fatalf("missing action metrics collectors in /metrics output: %s", rec.Body.String())
	}
}

func TestExecuteActionRouteAppliesUpdateObject(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.Service.Name = "ontology-actions-service"
	cfg.Service.Version = "test"
	cfg.JWTSecret = testJWTSecret
	state := &ontologykernel.AppState{Stores: stores.NewInMemory(), JWTConfig: authmw.NewJWTConfig(testJWTSecret)}
	router := server.BuildRouter(cfg, state, nil)

	objectTypeID := uuid.New()
	objectID := uuid.New()
	actionID := uuid.New()
	seedActionObjectType(t, state, objectTypeID)
	seedActionProperty(t, state, objectTypeID, "temperature", "float")
	seedActionObject(t, state, objectTypeID, objectID, map[string]any{"temperature": 72})
	seedUpdateAction(t, state, actionID, objectTypeID, "temperature")

	token := devToken(t)
	body := []byte(`{"target_object_id":"` + objectID.String() + `","parameters":{"temperature":84}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ontology/actions/"+actionID.String()+"/validate", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("validate expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/ontology/actions/"+actionID.String()+"/execute", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("execute expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var response struct {
		Object struct {
			Properties map[string]any `json:"properties"`
		} `json:"object"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode execute response: %v", err)
	}
	if got := response.Object.Properties["temperature"]; got != float64(84) {
		t.Fatalf("temperature drift: got %#v body=%s", got, rec.Body.String())
	}

	stored, err := state.Stores.Objects.Get(ctx, storage.TenantId("default"), storage.ObjectId(objectID.String()), storage.Strong())
	if err != nil {
		t.Fatalf("load stored object: %v", err)
	}
	var props map[string]any
	if err := json.Unmarshal(stored.Payload, &props); err != nil {
		t.Fatalf("decode stored object: %v", err)
	}
	if got := props["temperature"]; got != float64(84) {
		t.Fatalf("stored temperature drift: got %#v", got)
	}

	actionLogTypeID := storage.TypeId(kernelactions.ActionLogObjectTypeID.String())
	actionLogType, err := state.Stores.Definitions.Get(ctx, storage.DefinitionKind("object_type"), storage.DefinitionId(kernelactions.ActionLogObjectTypeID.String()), storage.Strong())
	if err != nil {
		t.Fatalf("load action log object type definition: %v", err)
	}
	if actionLogType == nil {
		t.Fatal("expected action log object type definition to be materialized")
	}
	parent := storage.DefinitionId(kernelactions.ActionLogObjectTypeID.String())
	actionLogProperties, err := state.Stores.Definitions.List(ctx, storage.DefinitionQuery{
		Kind:     storage.DefinitionKind("property"),
		ParentID: &parent,
		Page:     storage.Page{Size: 100},
	}, storage.Strong())
	if err != nil {
		t.Fatalf("list action log property definitions: %v", err)
	}
	if len(actionLogProperties.Items) < 10 {
		t.Fatalf("expected materialized action log properties, got %d", len(actionLogProperties.Items))
	}

	actionLogObjects, err := state.Stores.Objects.ListByType(ctx, storage.TenantId("default"), actionLogTypeID, storage.Page{Size: 10}, storage.Strong())
	if err != nil {
		t.Fatalf("list action log objects: %v", err)
	}
	if len(actionLogObjects.Items) != 1 {
		t.Fatalf("expected one materialized action log object, got %d", len(actionLogObjects.Items))
	}
	var actionLog map[string]any
	if err := json.Unmarshal(actionLogObjects.Items[0].Payload, &actionLog); err != nil {
		t.Fatalf("decode action log object: %v", err)
	}
	if got := actionLog["action_id"]; got != actionID.String() {
		t.Fatalf("action log action_id drift: got %#v", got)
	}
	if got := actionLog["action_name"]; got != "edit_weather" {
		t.Fatalf("action log action_name drift: got %#v", got)
	}
	if got := actionLog["operation_kind"]; got != models.ActionOperationKindUpdateObject.String() {
		t.Fatalf("action log operation_kind drift: got %#v", got)
	}
	if got := actionLog["status"]; got != "success" {
		t.Fatalf("action log status drift: got %#v", got)
	}
	if got := actionLog["target_object_id"]; got != objectID.String() {
		t.Fatalf("action log target_object_id drift: got %#v", got)
	}
	parameters, ok := actionLog["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("expected action log parameters object, got %#v", actionLog["parameters"])
	}
	if got := parameters["temperature"]; got != float64(84) {
		t.Fatalf("action log parameter drift: got %#v", got)
	}
	validation, ok := actionLog["validation"].(map[string]any)
	if !ok || validation["valid"] != true {
		t.Fatalf("expected successful validation payload, got %#v", actionLog["validation"])
	}
	edits, ok := actionLog["edits"].(map[string]any)
	if !ok || edits["object"] == nil {
		t.Fatalf("expected edits object payload, got %#v", actionLog["edits"])
	}
	if got := actionLog["applied_by_email"]; got != "smoke@openfoundry.test" {
		t.Fatalf("action log actor drift: got %#v", got)
	}
}

func seedActionObjectType(t *testing.T, state *ontologykernel.AppState, objectTypeID uuid.UUID) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"id": objectTypeID, "name": "weather", "display_name": "Weather"})
	if _, err := state.Stores.Definitions.Put(context.Background(), storage.DefinitionRecord{
		Kind:    storage.DefinitionKind("object_type"),
		ID:      storage.DefinitionId(objectTypeID.String()),
		Payload: payload,
	}, nil); err != nil {
		t.Fatalf("seed object type: %v", err)
	}
}

func seedActionProperty(t *testing.T, state *ontologykernel.AppState, objectTypeID uuid.UUID, name string, propertyType string) {
	t.Helper()
	now := time.Now().UTC()
	propertyID := uuid.New()
	payload, _ := json.Marshal(models.Property{
		ID:           propertyID,
		ObjectTypeID: objectTypeID,
		Name:         name,
		DisplayName:  name,
		PropertyType: propertyType,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	parent := storage.DefinitionId(objectTypeID.String())
	if _, err := state.Stores.Definitions.Put(context.Background(), storage.DefinitionRecord{
		Kind:     storage.DefinitionKind("property"),
		ID:       storage.DefinitionId(propertyID.String()),
		ParentID: &parent,
		Payload:  payload,
	}, nil); err != nil {
		t.Fatalf("seed property: %v", err)
	}
}

func seedActionObject(t *testing.T, state *ontologykernel.AppState, objectTypeID uuid.UUID, objectID uuid.UUID, properties map[string]any) {
	t.Helper()
	payload, _ := json.Marshal(properties)
	updated := time.Now().UTC().UnixMilli()
	if _, err := state.Stores.Objects.Put(context.Background(), storage.Object{
		Tenant:      storage.TenantId("default"),
		ID:          storage.ObjectId(objectID.String()),
		TypeID:      storage.TypeId(objectTypeID.String()),
		Version:     0,
		Payload:     payload,
		UpdatedAtMs: updated,
		Markings:    []storage.MarkingId{storage.MarkingId("public")},
	}, nil); err != nil {
		t.Fatalf("seed object: %v", err)
	}
}

func seedUpdateAction(t *testing.T, state *ontologykernel.AppState, actionID uuid.UUID, objectTypeID uuid.UUID, propertyName string) {
	t.Helper()
	inputName := propertyName
	config, _ := json.Marshal(models.UpdateObjectActionConfig{
		PropertyMappings: []models.ActionPropertyMapping{{PropertyName: propertyName, InputName: &inputName}},
		StaticPatch:      json.RawMessage(`null`),
	})
	now := time.Now().UTC()
	display := "Edit weather"
	action := models.ActionType{
		ID:            actionID,
		Name:          "edit_weather",
		DisplayName:   display,
		Description:   "Update weather action",
		ObjectTypeID:  objectTypeID,
		OperationKind: models.ActionOperationKindUpdateObject.String(),
		InputSchema: []models.ActionInputField{{
			Name:         propertyName,
			PropertyType: "float",
			Required:     true,
		}},
		Config:     config,
		OwnerID:    uuid.New(),
		CreatedAt:  now,
		UpdatedAt:  now,
		FormSchema: models.ActionFormSchema{},
	}
	record, err := domain.ActionToRecord(action)
	if err != nil {
		t.Fatalf("action record: %v", err)
	}
	if _, err := state.Stores.Definitions.Put(context.Background(), record, nil); err != nil {
		t.Fatalf("seed action: %v", err)
	}
}
