// Tests for the Phase 5C inline-edit helpers — input-name resolution
// (single mapping, ambiguous, configured), build_inline_edit_parameters
// back-fill from current target properties.
package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

func ptr[T any](v T) *T { return &v }

func TestResolveInlineEditInputNameSingleMapping(t *testing.T) {
	t.Parallel()
	cfg := models.UpdateObjectActionConfig{
		PropertyMappings: []models.ActionPropertyMapping{
			{PropertyName: "amount", InputName: ptr("amount_input")},
		},
	}
	configBytes, _ := json.Marshal(cfg)
	action := models.ActionType{Config: configBytes, OperationKind: "update_object"}

	got, err := resolveInlineEditInputName(action, "amount", models.PropertyInlineEditConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "amount_input" {
		t.Errorf("got %q want amount_input", got)
	}
}

func TestResolveInlineEditInputNameAmbiguousRequiresExplicit(t *testing.T) {
	t.Parallel()
	cfg := models.UpdateObjectActionConfig{
		PropertyMappings: []models.ActionPropertyMapping{
			{PropertyName: "amount", InputName: ptr("input_a")},
			{PropertyName: "amount", InputName: ptr("input_b")},
		},
	}
	configBytes, _ := json.Marshal(cfg)
	action := models.ActionType{Config: configBytes}

	if _, err := resolveInlineEditInputName(action, "amount", models.PropertyInlineEditConfig{}); err == nil {
		t.Fatal("expected ambiguity error")
	}

	got, err := resolveInlineEditInputName(action, "amount",
		models.PropertyInlineEditConfig{InputName: ptr("input_b")})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if got != "input_b" {
		t.Errorf("explicit selection drift: %q", got)
	}
}

func TestResolveInlineEditInputNameNoMapping(t *testing.T) {
	t.Parallel()
	cfg := models.UpdateObjectActionConfig{}
	configBytes, _ := json.Marshal(cfg)
	action := models.ActionType{Config: configBytes}
	if _, err := resolveInlineEditInputName(action, "amount", models.PropertyInlineEditConfig{}); err == nil {
		t.Fatal("expected error when property is not mapped")
	}
}

func TestBuildInlineEditParametersBackfillsOtherInputs(t *testing.T) {
	t.Parallel()
	cfg := models.UpdateObjectActionConfig{
		PropertyMappings: []models.ActionPropertyMapping{
			{PropertyName: "amount", InputName: ptr("amount_input")},
			{PropertyName: "currency", InputName: ptr("currency_input")},
		},
	}
	configBytes, _ := json.Marshal(cfg)
	action := models.ActionType{Config: configBytes}
	property := models.Property{
		ID:           uuid.New(),
		ObjectTypeID: uuid.New(),
		Name:         "amount",
		PropertyType: "number",
	}
	target := &domain.ObjectInstance{
		Properties: json.RawMessage(`{"amount":100,"currency":"USD"}`),
	}
	got, err := buildInlineEditParameters(action, property, target,
		models.PropertyInlineEditConfig{InputName: ptr("amount_input")},
		json.RawMessage(`200`))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(got, &asMap); err != nil {
		t.Fatalf("output not JSON object: %v", err)
	}
	if string(asMap["amount_input"]) != "200" {
		t.Errorf("amount_input drift: %s", asMap["amount_input"])
	}
	if string(asMap["currency_input"]) != `"USD"` {
		t.Errorf("currency_input back-fill drift: %s", asMap["currency_input"])
	}
}

func TestExecuteInlineEditUpdatesPropertyAndPersistsAttempt(t *testing.T) {
	ctx := context.Background()
	state := newTestState(t)
	objectTypeID := uuid.New()
	objectID := uuid.New()
	propertyID := seedInlineEditProperty(t, state, objectTypeID, "status", nil)
	action := seedInlineEditAction(t, state, objectTypeID, "status", "status_input", nil)
	attachInlineEditConfig(t, state, objectTypeID, propertyID, action.ID, nil)
	seedObjectTypeDefinition(t, state, objectTypeID)
	seedPropertyDefinition(t, state, objectTypeID, "status", "string")
	_, _ = state.Stores.Objects.Put(ctx, storage.Object{Tenant: "default", ID: storage.ObjectId(objectID.String()), TypeID: storage.TypeId(objectTypeID.String()), Payload: json.RawMessage(`{"status":"open"}`), UpdatedAtMs: time.Now().UnixMilli()}, nil)

	req := requestWithRoute(ctx, http.MethodPost, "/inline", []byte(`{"value":"closed"}`), map[string]string{
		"type_id": objectTypeID.String(), "property_id": propertyID.String(), "obj_id": objectID.String(),
	})
	rec := httptest.NewRecorder()
	ExecuteInlineEditHandler(state).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	stored, _ := state.Stores.Objects.Get(ctx, "default", storage.ObjectId(objectID.String()), storage.Strong())
	if string(stored.Payload) != `{"status":"closed"}` {
		t.Fatalf("payload drift: %s", stored.Payload)
	}
	entries, err := state.Stores.Actions.ListRecent(ctx, "default", storage.Page{Size: 20}, storage.Strong())
	if err != nil {
		t.Fatalf("list action log: %v", err)
	}
	if !hasActionAttempt(entries.Items, action.ID.String(), "success") {
		t.Fatalf("missing success action_attempt: %+v", entries.Items)
	}
}

func TestExecuteInlineEditBatchRejectsDuplicateTarget(t *testing.T) {
	state := newTestState(t)
	objectTypeID := uuid.New()
	propertyID := uuid.New()
	objectID := uuid.New()
	body := fmt.Sprintf(`{"edits":[{"property_id":"%s","object_id":"%s","value":"a"},{"property_id":"%s","object_id":"%s","value":"b"}]}`, propertyID, objectID, propertyID, objectID)
	req := requestWithRoute(context.Background(), http.MethodPost, "/inline-batch", []byte(body), map[string]string{"type_id": objectTypeID.String()})
	rec := httptest.NewRecorder()
	ExecuteInlineEditBatchHandler(state).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "same object") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExecuteInlineEditBatchPartialFailuresAndForbiddenItem(t *testing.T) {
	ctx := context.Background()
	state := newTestState(t)
	objectTypeID := uuid.New()
	okObjectID := uuid.New()
	forbiddenObjectID := uuid.New()
	missingObjectID := uuid.New()
	propertyID := seedInlineEditProperty(t, state, objectTypeID, "status", nil)
	action := seedInlineEditAction(t, state, objectTypeID, "status", "status_input", []string{"secret"})
	attachInlineEditConfig(t, state, objectTypeID, propertyID, action.ID, nil)
	seedObjectTypeDefinition(t, state, objectTypeID)
	seedPropertyDefinition(t, state, objectTypeID, "status", "string")
	_, _ = state.Stores.Objects.Put(ctx, storage.Object{Tenant: "default", ID: storage.ObjectId(okObjectID.String()), TypeID: storage.TypeId(objectTypeID.String()), Payload: json.RawMessage(`{"status":"old"}`), Markings: []storage.MarkingId{"secret"}, UpdatedAtMs: time.Now().UnixMilli()}, nil)
	_, _ = state.Stores.Objects.Put(ctx, storage.Object{Tenant: "default", ID: storage.ObjectId(forbiddenObjectID.String()), TypeID: storage.TypeId(objectTypeID.String()), Payload: json.RawMessage(`{"status":"old"}`), Markings: []storage.MarkingId{"public"}, UpdatedAtMs: time.Now().UnixMilli()}, nil)
	body := fmt.Sprintf(`{"edits":[{"property_id":"%s","object_id":"%s","value":"new"},{"property_id":"%s","object_id":"%s","value":"new"},{"property_id":"%s","object_id":"%s","value":"new"}]}`, propertyID, okObjectID, propertyID, forbiddenObjectID, propertyID, missingObjectID)
	req := requestWithRoute(ctx, http.MethodPost, "/inline-batch", []byte(body), map[string]string{"type_id": objectTypeID.String()})
	rec := httptest.NewRecorder()
	ExecuteInlineEditBatchHandler(state).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Succeeded int               `json:"succeeded"`
		Failed    int               `json:"failed"`
		Results   []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.Succeeded != 1 || resp.Failed != 2 {
		t.Fatalf("counts drift: %+v body=%s", resp, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "forbidden") || !strings.Contains(rec.Body.String(), "not found") {
		t.Fatalf("expected forbidden and not found failures: %s", rec.Body.String())
	}
}

func TestExecuteInlineEditBatchLimit(t *testing.T) {
	state := newTestState(t)
	objectTypeID := uuid.New()
	edits := make([]string, maxObjectsPerSubmission+1)
	for i := range edits {
		edits[i] = fmt.Sprintf(`{"property_id":"%s","object_id":"%s","value":%d}`, uuid.New(), uuid.New(), i)
	}
	body := []byte(`{"edits":[` + strings.Join(edits, ",") + `]}`)
	req := requestWithRoute(context.Background(), http.MethodPost, "/inline-batch", body, map[string]string{"type_id": objectTypeID.String()})
	rec := httptest.NewRecorder()
	ExecuteInlineEditBatchHandler(state).ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func requestWithRoute(ctx context.Context, method, path string, body []byte, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	claims := &authmw.Claims{Sub: uuid.New(), Email: "tester@example.com", Roles: []string{"admin"}}
	ctx = context.WithValue(authmw.ContextWithClaims(ctx, claims), chi.RouteCtxKey, rctx)
	return httptest.NewRequest(method, path, bytes.NewReader(body)).WithContext(ctx)
}

func seedInlineEditProperty(t *testing.T, state *ontologykernel.AppState, objectTypeID uuid.UUID, name string, inputName *string) uuid.UUID {
	t.Helper()
	propertyID := uuid.New()
	seedInlinePropertyRecord(t, state, objectTypeID, propertyID, name, uuid.Nil, inputName)
	return propertyID
}

func attachInlineEditConfig(t *testing.T, state *ontologykernel.AppState, objectTypeID, propertyID, actionID uuid.UUID, inputName *string) {
	t.Helper()
	seedInlinePropertyRecord(t, state, objectTypeID, propertyID, "status", actionID, inputName)
}

func seedInlinePropertyRecord(t *testing.T, state *ontologykernel.AppState, objectTypeID, propertyID uuid.UUID, name string, actionID uuid.UUID, inputName *string) {
	t.Helper()
	var cfg *models.PropertyInlineEditConfig
	if actionID != uuid.Nil {
		cfg = &models.PropertyInlineEditConfig{ActionTypeID: actionID, InputName: inputName}
	}
	now := time.Now().UTC()
	payload, _ := json.Marshal(models.Property{ID: propertyID, ObjectTypeID: objectTypeID, Name: name, DisplayName: name, PropertyType: "string", InlineEditConfig: cfg, CreatedAt: now, UpdatedAt: now})
	parent := storage.DefinitionId(objectTypeID.String())
	_, err := state.Stores.Definitions.Put(context.Background(), storage.DefinitionRecord{Kind: "property", ID: storage.DefinitionId(propertyID.String()), ParentID: &parent, Payload: payload}, nil)
	if err != nil {
		t.Fatalf("seed inline property: %v", err)
	}
}

func seedInlineEditAction(t *testing.T, state *ontologykernel.AppState, objectTypeID uuid.UUID, propertyName, inputName string, allowedMarkings []string) models.ActionType {
	t.Helper()
	cfg, _ := json.Marshal(models.UpdateObjectActionConfig{PropertyMappings: []models.ActionPropertyMapping{{PropertyName: propertyName, InputName: &inputName}}})
	now := time.Now().UTC()
	action := models.ActionType{ID: uuid.New(), Name: "inline_" + propertyName, DisplayName: "Inline", ObjectTypeID: objectTypeID, OperationKind: "update_object", Config: cfg, AuthorizationPolicy: models.ActionAuthorizationPolicy{AllowedMarkings: allowedMarkings}, OwnerID: uuid.New(), CreatedAt: now, UpdatedAt: now}
	rec, err := domain.ActionToRecord(action)
	if err != nil {
		t.Fatalf("action record: %v", err)
	}
	if _, err := state.Stores.Definitions.Put(context.Background(), rec, nil); err != nil {
		t.Fatalf("seed action: %v", err)
	}
	return action
}

func hasActionAttempt(entries []storage.ActionLogEntry, actionID string, status string) bool {
	for _, entry := range entries {
		if entry.Kind != "action_attempt" || entry.ActionID != actionID {
			continue
		}
		var payload map[string]any
		_ = json.Unmarshal(entry.Payload, &payload)
		if payload["status"] == status {
			return true
		}
	}
	return false
}
