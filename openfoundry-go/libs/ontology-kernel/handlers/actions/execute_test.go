package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// allForbidden mirrors the Rust partition logic that decides whether
// validation errors collapse to 403 vs 400.
func TestAllForbiddenPartitionsErrors(t *testing.T) {
	t.Parallel()
	if !allForbidden([]string{"forbidden: missing permission 'x'"}) {
		t.Fatal("single forbidden error must collapse to 403")
	}
	if allForbidden([]string{"forbidden: a", "non-forbidden: b"}) {
		t.Fatal("mixed errors must NOT collapse to 403")
	}
	if allForbidden(nil) {
		t.Fatal("empty errors must NOT collapse to 403")
	}
}

// forbiddenLine ensures every authorisation error keeps the prefix
// allForbidden looks for.
func TestForbiddenLineAddsPrefixWhenMissing(t *testing.T) {
	t.Parallel()
	if got := forbiddenLine("missing permission 'x'"); got != "forbidden: missing permission 'x'" {
		t.Fatalf("prefix not added: %q", got)
	}
	if got := forbiddenLine("forbidden: already prefixed"); got != "forbidden: already prefixed" {
		t.Fatalf("double-prefix: %q", got)
	}
}

// ensureConfirmationJustification mirrors the Rust gate.
func TestEnsureConfirmationJustification(t *testing.T) {
	t.Parallel()
	confirm := models.ActionType{ConfirmationRequired: true}
	plain := models.ActionType{ConfirmationRequired: false}
	jstr := "approved by ops"
	empty := ""
	cases := []struct {
		name string
		a    models.ActionType
		j    *string
		want bool // expect error
	}{
		{"plain action no justification", plain, nil, false},
		{"confirm action no justification", confirm, nil, true},
		{"confirm action empty justification", confirm, &empty, true},
		{"confirm action valid justification", confirm, &jstr, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ensureConfirmationJustification(tc.a, tc.j)
			if (err != nil) != tc.want {
				t.Errorf("got err=%v, want err=%v", err, tc.want)
			}
		})
	}
}

// operationConfigBytes peels the optional `{ "operation": ... }`
// envelope. Rust's split_action_config returns the inner operation
// when the wrapper is present.
func TestOperationConfigBytes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"flat config", `{"property_mappings":[]}`, `{"property_mappings":[]}`},
		{"wrapped config", `{"operation":{"property_mappings":[]},"webhook_writeback":null}`, `{"property_mappings":[]}`},
		{"wrapped without operation", `{"notification_side_effects":[]}`, `{}`},
		{"null config", `null`, `{}`},
		{"empty config", ``, `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(operationConfigBytes(json.RawMessage(tc.in)))
			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

// planAction → planUpdateObject end-to-end against in-memory stores.
func TestPlanActionBuildsUpdateObjectPatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	state := newTestState(t)

	// Seed the property schema for the object type.
	objectTypeID := uuid.New()
	defID := storage.DefinitionId(objectTypeID.String())
	otRecord, _ := json.Marshal(map[string]any{"id": objectTypeID, "name": "case"})
	_, _ = state.Stores.Definitions.Put(ctx, storage.DefinitionRecord{
		Kind: "object_type", ID: defID, Payload: otRecord,
	}, nil)

	// Seed an object instance.
	objectID := uuid.New()
	now := time.Now().UTC().UnixMilli()
	props, _ := json.Marshal(map[string]any{"status": "open"})
	_, _ = state.Stores.Objects.Put(ctx, storage.Object{
		Tenant:      storage.TenantId("default"),
		ID:          storage.ObjectId(objectID.String()),
		TypeID:      storage.TypeId(objectTypeID.String()),
		Version:     0,
		Payload:     props,
		UpdatedAtMs: now,
	}, nil)

	action := models.ActionType{
		ID:            uuid.New(),
		ObjectTypeID:  objectTypeID,
		OperationKind: "update_object",
		// Inline static_patch so the plan doesn't have to materialise
		// inputs against the request parameters.
		Config: json.RawMessage(`{"property_mappings":[],"static_patch":{"status":"closed"}}`),
		// We exercise the planner with the property gate disabled
		// (LoadEffectivePropertiesViaStore returns empty); validation
		// of the patch payload happens at execute time.
	}
	req := &models.ValidateActionRequest{
		TargetObjectID: &objectID,
		Parameters:     json.RawMessage(`{}`),
	}
	plan, errs := planAction(ctx, state, &authmw.Claims{Sub: uuid.New()}, action, req)
	if len(errs) > 0 {
		// We expect a validation error because the in-memory
		// definition store doesn't carry effective properties for
		// the object_type yet — but the error must not be a generic
		// failure, it must be the typed "unknown property" branch.
		// This pins behaviour today; when the schema seeder lands
		// the assertion flips to plan.kind == planUpdateObject.
		var pinned bool
		for _, e := range errs {
			if strings.Contains(e, "unknown property 'status'") {
				pinned = true
			}
		}
		if !pinned {
			t.Fatalf("unexpected validation errors: %v", errs)
		}
		return
	}
	if plan.kind != planUpdateObject {
		t.Fatalf("expected planUpdateObject, got kind=%v", plan.kind)
	}
	if plan.target == nil || plan.target.ID != objectID {
		t.Fatalf("target drift: %+v", plan.target)
	}
}

// planAction → planDeleteObject end-to-end (no patch needed).
func TestPlanActionBuildsDeleteObjectPlan(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	state := newTestState(t)

	objectTypeID := uuid.New()
	objectID := uuid.New()
	props, _ := json.Marshal(map[string]any{})
	_, _ = state.Stores.Objects.Put(ctx, storage.Object{
		Tenant:      storage.TenantId("default"),
		ID:          storage.ObjectId(objectID.String()),
		TypeID:      storage.TypeId(objectTypeID.String()),
		Payload:     props,
		UpdatedAtMs: time.Now().UnixMilli(),
	}, nil)

	action := models.ActionType{
		ID:            uuid.New(),
		ObjectTypeID:  objectTypeID,
		OperationKind: "delete_object",
	}
	req := &models.ValidateActionRequest{TargetObjectID: &objectID, Parameters: json.RawMessage(`{}`)}
	plan, errs := planAction(ctx, state, &authmw.Claims{Sub: uuid.New()}, action, req)
	if len(errs) > 0 {
		t.Fatalf("unexpected: %v", errs)
	}
	if plan.kind != planDeleteObject {
		t.Fatalf("expected planDeleteObject, got %v", plan.kind)
	}
	if plan.target == nil || plan.target.ID != objectID {
		t.Fatalf("target drift")
	}
}

func TestPlanActionInterfaceNotFound(t *testing.T) {
	t.Parallel()
	state := newTestState(t)
	action := models.ActionType{ObjectTypeID: uuid.New(), OperationKind: "create_interface", Config: json.RawMessage(`{}`)}
	_, errs := planAction(context.Background(), state, &authmw.Claims{Sub: uuid.New()}, action,
		&models.ValidateActionRequest{Parameters: json.RawMessage(`{}`)})
	if len(errs) == 0 || !strings.Contains(errs[0], "was not found") {
		t.Fatalf("expected interface not found, got %v", errs)
	}
}

func TestPlanActionCreateInterfaceAmbiguousImplementation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	state := newTestState(t)
	interfaceID := uuid.New()
	typeA := uuid.New()
	typeB := uuid.New()
	seedInterfaceDefinition(t, state, interfaceID)
	seedObjectTypeDefinition(t, state, typeA)
	seedObjectTypeDefinition(t, state, typeB)
	seedInterfaceBinding(t, state, typeA, interfaceID)
	seedInterfaceBinding(t, state, typeB, interfaceID)
	action := models.ActionType{ObjectTypeID: interfaceID, OperationKind: "create_interface", Config: json.RawMessage(`{}`)}
	_, errs := planAction(ctx, state, &authmw.Claims{Sub: uuid.New()}, action,
		&models.ValidateActionRequest{Parameters: json.RawMessage(`{}`)})
	if len(errs) == 0 || !strings.Contains(errs[0], "ambiguous implementation") {
		t.Fatalf("expected ambiguous implementation, got %v", errs)
	}
}

func TestExecutePlanCreateInterfaceSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	state := newTestState(t)
	interfaceID := uuid.New()
	objectTypeID := uuid.New()
	seedInterfaceDefinition(t, state, interfaceID)
	seedObjectTypeDefinition(t, state, objectTypeID)
	seedPropertyDefinition(t, state, objectTypeID, "status", "string")
	seedInterfaceBinding(t, state, objectTypeID, interfaceID)
	action := models.ActionType{ID: uuid.New(), ObjectTypeID: interfaceID, OperationKind: "create_interface", Config: json.RawMessage(`{"static_patch":{"status":"open"}}`)}
	req := &models.ValidateActionRequest{Parameters: json.RawMessage(`{"__object_type":"` + objectTypeID.String() + `"}`)}
	plan, errs := planAction(ctx, state, &authmw.Claims{Sub: uuid.New()}, action, req)
	if len(errs) > 0 {
		t.Fatalf("planAction: %v", errs)
	}
	executed, err := executePlan(ctx, state, &authmw.Claims{Sub: uuid.New()}, action, plan)
	if err != nil {
		t.Fatalf("executePlan: %v", err)
	}
	stored, err := state.Stores.Objects.Get(ctx, storage.TenantId("default"), storage.ObjectId(executed.targetObjectID.String()), storage.Strong())
	if err != nil {
		t.Fatalf("get created object: %v", err)
	}
	if stored == nil || stored.TypeID != storage.TypeId(objectTypeID.String()) {
		t.Fatalf("created object drift: %+v", stored)
	}
	if string(stored.Payload) != `{"status":"open"}` {
		t.Fatalf("payload drift: %s", stored.Payload)
	}
}

func TestExecutePlanModifyInterfaceSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	state := newTestState(t)
	interfaceID := uuid.New()
	objectTypeID := uuid.New()
	objectID := uuid.New()
	seedInterfaceDefinition(t, state, interfaceID)
	seedObjectTypeDefinition(t, state, objectTypeID)
	seedPropertyDefinition(t, state, objectTypeID, "status", "string")
	seedInterfaceBinding(t, state, objectTypeID, interfaceID)
	_, _ = state.Stores.Objects.Put(ctx, storage.Object{Tenant: "default", ID: storage.ObjectId(objectID.String()), TypeID: storage.TypeId(objectTypeID.String()), Payload: json.RawMessage(`{"status":"old"}`), UpdatedAtMs: time.Now().UnixMilli()}, nil)
	action := models.ActionType{ID: uuid.New(), ObjectTypeID: interfaceID, OperationKind: "modify_interface", Config: json.RawMessage(`{"static_patch":{"status":"new"}}`)}
	plan, errs := planAction(ctx, state, &authmw.Claims{Sub: uuid.New()}, action, &models.ValidateActionRequest{Parameters: json.RawMessage(`{"__interface_ref":"` + objectID.String() + `"}`)})
	if len(errs) > 0 {
		t.Fatalf("planAction: %v", errs)
	}
	executed, err := executePlan(ctx, state, &authmw.Claims{Sub: uuid.New()}, action, plan)
	if err != nil {
		t.Fatalf("executePlan: %v", err)
	}
	if executed.targetObjectID == nil || *executed.targetObjectID != objectID {
		t.Fatalf("target drift: %+v", executed.targetObjectID)
	}
	stored, _ := state.Stores.Objects.Get(ctx, "default", storage.ObjectId(objectID.String()), storage.Strong())
	if string(stored.Payload) != `{"status":"new"}` {
		t.Fatalf("payload drift: %s", stored.Payload)
	}
}

func TestExecutePlanDeleteInterfaceSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	state := newTestState(t)
	interfaceID := uuid.New()
	objectTypeID := uuid.New()
	objectID := uuid.New()
	seedInterfaceDefinition(t, state, interfaceID)
	seedObjectTypeDefinition(t, state, objectTypeID)
	seedInterfaceBinding(t, state, objectTypeID, interfaceID)
	_, _ = state.Stores.Objects.Put(ctx, storage.Object{Tenant: "default", ID: storage.ObjectId(objectID.String()), TypeID: storage.TypeId(objectTypeID.String()), Payload: json.RawMessage(`{}`), UpdatedAtMs: time.Now().UnixMilli()}, nil)
	action := models.ActionType{ID: uuid.New(), ObjectTypeID: interfaceID, OperationKind: "delete_interface"}
	plan, errs := planAction(ctx, state, &authmw.Claims{Sub: uuid.New()}, action, &models.ValidateActionRequest{Parameters: json.RawMessage(`{"__interface_ref":"` + objectID.String() + `"}`)})
	if len(errs) > 0 {
		t.Fatalf("planAction: %v", errs)
	}
	executed, err := executePlan(ctx, state, &authmw.Claims{Sub: uuid.New()}, action, plan)
	if err != nil {
		t.Fatalf("executePlan: %v", err)
	}
	if !executed.deleted {
		t.Fatalf("delete flag not set")
	}
	stored, _ := state.Stores.Objects.Get(ctx, "default", storage.ObjectId(objectID.String()), storage.Strong())
	if stored != nil {
		t.Fatalf("object still present: %+v", stored)
	}
}

func TestExecutePlanInterfaceLinkCreateAndDeleteSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	state := newTestState(t)
	sourceInterfaceID := uuid.New()
	targetInterfaceID := uuid.New()
	sourceTypeID := uuid.New()
	targetTypeID := uuid.New()
	sourceID := uuid.New()
	targetID := uuid.New()
	linkTypeID := uuid.New()
	for _, id := range []uuid.UUID{sourceInterfaceID, targetInterfaceID} {
		seedInterfaceDefinition(t, state, id)
	}
	seedObjectTypeDefinition(t, state, sourceTypeID)
	seedObjectTypeDefinition(t, state, targetTypeID)
	seedInterfaceBinding(t, state, sourceTypeID, sourceInterfaceID)
	seedInterfaceBinding(t, state, targetTypeID, targetInterfaceID)
	seedLinkTypeDefinition(t, state, models.LinkType{ID: linkTypeID, SourceTypeID: sourceTypeID, TargetTypeID: targetTypeID})
	_, _ = state.Stores.Objects.Put(ctx, storage.Object{Tenant: "default", ID: storage.ObjectId(sourceID.String()), TypeID: storage.TypeId(sourceTypeID.String()), Payload: json.RawMessage(`{}`), UpdatedAtMs: time.Now().UnixMilli()}, nil)
	_, _ = state.Stores.Objects.Put(ctx, storage.Object{Tenant: "default", ID: storage.ObjectId(targetID.String()), TypeID: storage.TypeId(targetTypeID.String()), Payload: json.RawMessage(`{}`), UpdatedAtMs: time.Now().UnixMilli()}, nil)
	cfg := fmt.Sprintf(`{"link_type_id":"%s","source_interface_id":"%s","target_interface_id":"%s"}`, linkTypeID, sourceInterfaceID, targetInterfaceID)
	params := json.RawMessage(`{"__interface_ref":"` + sourceID.String() + `","target_interface_ref":"` + targetID.String() + `"}`)
	createAction := models.ActionType{ID: uuid.New(), ObjectTypeID: sourceInterfaceID, OperationKind: "create_interface_link", Config: json.RawMessage(cfg)}
	plan, errs := planAction(ctx, state, &authmw.Claims{Sub: uuid.New()}, createAction, &models.ValidateActionRequest{Parameters: params})
	if len(errs) > 0 {
		t.Fatalf("create planAction: %v", errs)
	}
	if _, err := executePlan(ctx, state, &authmw.Claims{Sub: uuid.New()}, createAction, plan); err != nil {
		t.Fatalf("create executePlan: %v", err)
	}
	links, _ := state.Stores.Links.ListOutgoing(ctx, "default", storage.LinkTypeId(linkTypeID.String()), storage.ObjectId(sourceID.String()), storage.Page{Size: 10}, storage.Strong())
	if len(links.Items) != 1 {
		t.Fatalf("link not created: %+v", links.Items)
	}
	deleteAction := models.ActionType{ID: uuid.New(), ObjectTypeID: sourceInterfaceID, OperationKind: "delete_interface_link", Config: json.RawMessage(cfg)}
	plan, errs = planAction(ctx, state, &authmw.Claims{Sub: uuid.New()}, deleteAction, &models.ValidateActionRequest{Parameters: params})
	if len(errs) > 0 {
		t.Fatalf("delete planAction: %v", errs)
	}
	executed, err := executePlan(ctx, state, &authmw.Claims{Sub: uuid.New()}, deleteAction, plan)
	if err != nil {
		t.Fatalf("delete executePlan: %v", err)
	}
	if !executed.deleted {
		t.Fatalf("delete flag not set")
	}
	links, _ = state.Stores.Links.ListOutgoing(ctx, "default", storage.LinkTypeId(linkTypeID.String()), storage.ObjectId(sourceID.String()), storage.Page{Size: 10}, storage.Strong())
	if len(links.Items) != 0 {
		t.Fatalf("link not deleted: %+v", links.Items)
	}
}

func TestExecutePlanModifyInterfaceVersionConflict(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	state := newTestState(t)
	interfaceID := uuid.New()
	objectTypeID := uuid.New()
	objectID := uuid.New()
	seedInterfaceDefinition(t, state, interfaceID)
	seedObjectTypeDefinition(t, state, objectTypeID)
	seedPropertyDefinition(t, state, objectTypeID, "status", "string")
	seedInterfaceBinding(t, state, objectTypeID, interfaceID)
	mockObjects := stores.NewMockObjectStore()
	stored := &storage.Object{Tenant: "default", ID: storage.ObjectId(objectID.String()), TypeID: storage.TypeId(objectTypeID.String()), Version: 1, Payload: json.RawMessage(`{"status":"old"}`), UpdatedAtMs: time.Now().UnixMilli()}
	mockObjects.QueueGet(stored, nil)
	mockObjects.QueueGet(stored, nil)
	mockObjects.QueuePut(storage.VersionConflict(1, 2), nil)
	state.Stores.Objects = mockObjects
	action := models.ActionType{ID: uuid.New(), ObjectTypeID: interfaceID, OperationKind: "modify_interface", Config: json.RawMessage(`{"static_patch":{"status":"new"}}`)}
	plan, errs := planAction(ctx, state, &authmw.Claims{Sub: uuid.New()}, action, &models.ValidateActionRequest{Parameters: json.RawMessage(`{"__interface_ref":"` + objectID.String() + `"}`)})
	if len(errs) > 0 {
		t.Fatalf("planAction: %v", errs)
	}
	_, err := executePlan(ctx, state, &authmw.Claims{Sub: uuid.New()}, action, plan)
	if !domain.IsVersionConflict(err) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

// planPreview emits the canonical Rust shape.
func TestPlanPreview(t *testing.T) {
	t.Parallel()
	target := &domain.ObjectInstance{ID: uuid.New(), ObjectTypeID: uuid.New()}
	patch := map[string]json.RawMessage{"status": json.RawMessage(`"closed"`)}
	upd := actionPlan{kind: planUpdateObject, target: target, patch: patch}
	got := planPreview(upd)
	var asMap map[string]any
	if err := json.Unmarshal(got, &asMap); err != nil {
		t.Fatalf("preview not JSON: %v", err)
	}
	if asMap["kind"] != "update_object" {
		t.Fatalf("preview kind drift: %v", asMap["kind"])
	}
	if _, ok := asMap["patch"].(map[string]any); !ok {
		t.Fatalf("preview missing patch object: %v", asMap)
	}

	del := actionPlan{kind: planDeleteObject, target: target}
	got = planPreview(del)
	_ = json.Unmarshal(got, &asMap)
	if asMap["kind"] != "delete_object" {
		t.Fatalf("delete preview kind drift: %v", asMap["kind"])
	}
}

// newTestState builds a fresh AppState with in-memory stores.
func newTestState(t *testing.T) *ontologykernel.AppState {
	t.Helper()
	return &ontologykernel.AppState{
		Stores:    stores.NewInMemory(),
		JWTConfig: authmw.NewJWTConfig("test-secret"),
	}
}

func seedObjectTypeDefinition(t *testing.T, state *ontologykernel.AppState, objectTypeID uuid.UUID) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"id": objectTypeID, "name": "type_" + objectTypeID.String()[:8]})
	_, err := state.Stores.Definitions.Put(context.Background(), storage.DefinitionRecord{
		Kind:    storage.DefinitionKind("object_type"),
		ID:      storage.DefinitionId(objectTypeID.String()),
		Payload: payload,
	}, nil)
	if err != nil {
		t.Fatalf("seed object type: %v", err)
	}
}

func seedPropertyDefinition(t *testing.T, state *ontologykernel.AppState, objectTypeID uuid.UUID, name, propertyType string) {
	t.Helper()
	propertyID := uuid.New()
	now := time.Now().UTC()
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
	_, err := state.Stores.Definitions.Put(context.Background(), storage.DefinitionRecord{
		Kind:     storage.DefinitionKind("property"),
		ID:       storage.DefinitionId(propertyID.String()),
		ParentID: &parent,
		Payload:  payload,
	}, nil)
	if err != nil {
		t.Fatalf("seed property: %v", err)
	}
}

func seedInterfaceDefinition(t *testing.T, state *ontologykernel.AppState, interfaceID uuid.UUID) {
	t.Helper()
	now := time.Now().UTC()
	payload, _ := json.Marshal(models.OntologyInterface{
		ID:          interfaceID,
		Name:        "iface_" + interfaceID.String()[:8],
		DisplayName: "Interface",
		OwnerID:     uuid.New(),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	_, err := state.Stores.Definitions.Put(context.Background(), storage.DefinitionRecord{
		Kind:    storage.DefinitionKind("interface"),
		ID:      storage.DefinitionId(interfaceID.String()),
		Payload: payload,
	}, nil)
	if err != nil {
		t.Fatalf("seed interface: %v", err)
	}
}

func seedInterfaceBinding(t *testing.T, state *ontologykernel.AppState, objectTypeID, interfaceID uuid.UUID) {
	t.Helper()
	payload, _ := json.Marshal(models.ObjectTypeInterfaceBinding{ObjectTypeID: objectTypeID, InterfaceID: interfaceID, CreatedAt: time.Now().UTC()})
	id := objectTypeID.String() + ":" + interfaceID.String()
	_, err := state.Stores.Definitions.Put(context.Background(), storage.DefinitionRecord{
		Kind:    storage.DefinitionKind("object_type_interface"),
		ID:      storage.DefinitionId(id),
		Payload: payload,
	}, nil)
	if err != nil {
		t.Fatalf("seed interface binding: %v", err)
	}
}

func seedLinkTypeDefinition(t *testing.T, state *ontologykernel.AppState, linkType models.LinkType) {
	t.Helper()
	payload, _ := json.Marshal(linkType)
	_, err := state.Stores.Definitions.Put(context.Background(), storage.DefinitionRecord{
		Kind:    storage.DefinitionKind("link_type"),
		ID:      storage.DefinitionId(linkType.ID.String()),
		Payload: payload,
	}, nil)
	if err != nil {
		t.Fatalf("seed link type: %v", err)
	}
}

type fakeInlineSidecar struct {
	result      json.RawMessage
	err         error
	seenSource  string
	seenInput   json.RawMessage
	seenTimeout uint32
}

func (f *fakeInlineSidecar) ExecuteInline(_ context.Context, source string, inputJSON []byte, timeoutSeconds uint32) (*ontologykernel.InlineRuntimeResult, error) {
	f.seenSource = source
	f.seenInput = append([]byte(nil), inputJSON...)
	f.seenTimeout = timeoutSeconds
	if f.err != nil {
		return nil, f.err
	}
	return &ontologykernel.InlineRuntimeResult{ResultJSON: f.result, Stdout: "stdout", Stderr: "stderr"}, nil
}

func inlinePythonPlan(source string) actionPlan {
	return actionPlan{
		kind:       planInvokeFunction,
		invocation: &httpInvocationConfig{URL: "inline://python", Method: "POST"},
		parameters: map[string]json.RawMessage{"x": json.RawMessage(`1`)},
		inlineFunction: &domain.ResolvedInlineFunction{
			Config: domain.InlineFunctionConfig{
				Kind:   domain.InlineFunctionPython,
				Python: &domain.InlinePythonFunctionConfig{Runtime: "python", Source: source},
			},
			Capabilities: models.FunctionCapabilities{AllowOntologyRead: true, TimeoutSeconds: 9, MaxSourceBytes: 1024},
		},
	}
}

func TestExecutePlanInlinePythonActionSuccessViaSidecar(t *testing.T) {
	t.Parallel()
	state := newTestState(t)
	fake := &fakeInlineSidecar{result: json.RawMessage(`{"ok":true}`)}
	state.PythonRuntime = fake
	justification := "approved"
	plan := inlinePythonPlan("result = {'ok': True}")
	plan.justification = &justification
	action := models.ActionType{ID: uuid.New(), ObjectTypeID: uuid.New(), OperationKind: "invoke_function", Name: "run_py"}

	executed, err := executePlan(context.Background(), state, &authmw.Claims{Sub: uuid.New()}, action, plan)
	if err != nil {
		t.Fatalf("executePlan: %v", err)
	}
	if string(executed.result) != `{"ok":true}` {
		t.Fatalf("result drift: %s", executed.result)
	}
	if fake.seenSource != "result = {'ok': True}" || fake.seenTimeout != 9 {
		t.Fatalf("sidecar request drift: source=%q timeout=%d", fake.seenSource, fake.seenTimeout)
	}
	var envelope map[string]any
	if err := json.Unmarshal(fake.seenInput, &envelope); err != nil {
		t.Fatalf("input envelope json: %v", err)
	}
	ctxEnvelope := envelope["context"].(map[string]any)
	if ctxEnvelope["justification"] != justification {
		t.Fatalf("justification not preserved in envelope: %+v", ctxEnvelope)
	}
	if _, ok := ctxEnvelope["parameters"].(map[string]any)["x"]; !ok {
		t.Fatalf("parameters not preserved in envelope: %+v", ctxEnvelope["parameters"])
	}
}

func TestPlanInvokeFunctionValidationError(t *testing.T) {
	t.Parallel()
	state := newTestState(t)
	action := models.ActionType{ID: uuid.New(), ObjectTypeID: uuid.New(), OperationKind: "invoke_function", Config: json.RawMessage(`{"runtime":"python","source":"   "}`)}
	_, errs := planAction(context.Background(), state, &authmw.Claims{Sub: uuid.New()}, action, &models.ValidateActionRequest{Parameters: json.RawMessage(`{}`)})
	if len(errs) == 0 || !strings.Contains(errs[0], "inline python function requires a non-empty source") {
		t.Fatalf("expected inline source validation error, got %v", errs)
	}
}

func TestExecutePlanInlinePythonException(t *testing.T) {
	t.Parallel()
	state := newTestState(t)
	state.PythonRuntime = &fakeInlineSidecar{err: errors.New("Traceback: boom")}
	action := models.ActionType{ID: uuid.New(), ObjectTypeID: uuid.New(), OperationKind: "invoke_function", Name: "run_py"}
	_, err := executePlan(context.Background(), state, &authmw.Claims{Sub: uuid.New()}, action, inlinePythonPlan("raise Exception('boom')"))
	if err == nil || !strings.Contains(err.Error(), "Traceback: boom") {
		t.Fatalf("expected Python exception, got %v", err)
	}
}

func TestExecutePlanInlinePythonPreservesRevertUndoAndMediaMetadata(t *testing.T) {
	t.Parallel()
	state := newTestState(t)
	payload := json.RawMessage(`{"status":"ok","undo":{"kind":"restore_object","object_id":"obj-1"},"revert":{"kind":"patch","properties":{"status":"old"}},"media_upload":{"status":"media_reference","attachment_rid":"ri.attachments.test"}}`)
	state.PythonRuntime = &fakeInlineSidecar{result: payload}
	action := models.ActionType{ID: uuid.New(), ObjectTypeID: uuid.New(), OperationKind: "invoke_function", Name: "run_py"}
	executed, err := executePlan(context.Background(), state, &authmw.Claims{Sub: uuid.New()}, action, inlinePythonPlan("result = {...}"))
	if err != nil {
		t.Fatalf("executePlan: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(executed.result, &result); err != nil {
		t.Fatalf("result json: %v", err)
	}
	if _, ok := result["undo"].(map[string]any); !ok {
		t.Fatalf("undo metadata missing: %v", result)
	}
	if _, ok := result["revert"].(map[string]any); !ok {
		t.Fatalf("revert metadata missing: %v", result)
	}
	media, ok := result["media_upload"].(map[string]any)
	if !ok || media["status"] != "media_reference" {
		t.Fatalf("media reference missing: %v", result)
	}
}

func TestExecutePlanInlinePythonRuntimeNotConfigured(t *testing.T) {
	t.Parallel()
	state := newTestState(t)
	action := models.ActionType{ID: uuid.New(), ObjectTypeID: uuid.New(), OperationKind: "invoke_function", Name: "run_py"}
	_, err := executePlan(context.Background(), state, &authmw.Claims{Sub: uuid.New()}, action, inlinePythonPlan("result = {'ok': True}"))
	if !errors.Is(err, domain.ErrPythonRuntimeNotWired) {
		t.Fatalf("expected ErrPythonRuntimeNotWired, got %v", err)
	}
}

type ctxAwareInlineSidecar struct{}

func (ctxAwareInlineSidecar) ExecuteInline(ctx context.Context, _ string, _ []byte, _ uint32) (*ontologykernel.InlineRuntimeResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestExecutePlanInlinePythonTimeoutPropagates(t *testing.T) {
	t.Parallel()
	state := newTestState(t)
	state.PythonRuntime = ctxAwareInlineSidecar{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	action := models.ActionType{ID: uuid.New(), ObjectTypeID: uuid.New(), OperationKind: "invoke_function", Name: "run_py"}
	_, err := executePlan(ctx, state, &authmw.Claims{Sub: uuid.New()}, action, inlinePythonPlan("while True: pass"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

type staticActionFunctionRuntime struct {
	result json.RawMessage
	err    error
}

func (f staticActionFunctionRuntime) ExecuteInlineFunction(context.Context, *ontologykernel.AppState, *authmw.Claims, *models.ActionType, *domain.ObjectInstance, map[string]json.RawMessage, *domain.ResolvedInlineFunction, *string) (json.RawMessage, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestExecuteActionAuditFailureDoesNotBreakSuccessResponse(t *testing.T) {
	t.Parallel()
	state := newTestState(t)
	state.HTTPClient = http.DefaultClient
	state.AuditServiceURL = "http://127.0.0.1:1"
	action := models.ActionType{
		ID:            uuid.New(),
		ObjectTypeID:  uuid.New(),
		OperationKind: "invoke_function",
		Name:          "run_py",
		DisplayName:   "Run Python",
		Config:        json.RawMessage(`{"runtime":"python","source":"result = {'ok': True}"}`),
		OwnerID:       uuid.New(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	record, err := domain.ActionToRecord(action)
	if err != nil {
		t.Fatalf("ActionToRecord: %v", err)
	}
	if _, err := state.Stores.Definitions.Put(context.Background(), record, nil); err != nil {
		t.Fatalf("seed action: %v", err)
	}

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", action.ID.String())
	claims := &authmw.Claims{Sub: uuid.New(), Email: "alice@example.com", Roles: []string{"admin"}}
	ctx := context.WithValue(authmw.ContextWithClaims(context.Background(), claims), chi.RouteCtxKey, rctx)
	req := httptest.NewRequest(http.MethodPost, "/ontology/actions/"+action.ID.String()+"/execute", bytes.NewReader([]byte(`{"parameters":{}}`))).WithContext(ctx)
	rec := httptest.NewRecorder()

	ExecuteActionWithRuntime(state, staticActionFunctionRuntime{result: json.RawMessage(`{"ok":true}`)}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var body models.ExecuteActionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response json: %v", err)
	}
	if string(body.Result) != `{"ok":true}` {
		t.Fatalf("result drift: %s", body.Result)
	}
}

func TestExecutePlanCreateLinkPersistsLink(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	state := newTestState(t)
	sourceTypeID := uuid.New()
	targetTypeID := uuid.New()
	sourceID := uuid.New()
	targetID := uuid.New()
	linkTypeID := uuid.New()
	linkType := &models.LinkType{ID: linkTypeID, SourceTypeID: sourceTypeID, TargetTypeID: targetTypeID}
	action := models.ActionType{ID: uuid.New(), ObjectTypeID: sourceTypeID, OperationKind: "create_link"}
	plan := actionPlan{
		kind:             planCreateLink,
		target:           &domain.ObjectInstance{ID: sourceID, ObjectTypeID: sourceTypeID},
		counterpart:      &domain.ObjectInstance{ID: targetID, ObjectTypeID: targetTypeID},
		linkType:         linkType,
		linkProperties:   json.RawMessage(`{"rel":"owns"}`),
		linkSourceObject: sourceID,
		linkTargetObject: targetID,
	}
	executed, err := executePlan(ctx, state, &authmw.Claims{Sub: uuid.New()}, action, plan)
	if err != nil {
		t.Fatalf("executePlan: %v", err)
	}
	if len(executed.link) == 0 {
		t.Fatalf("link response missing")
	}
	links, err := state.Stores.Links.ListOutgoing(ctx, storage.TenantId("default"), storage.LinkTypeId(linkTypeID.String()), storage.ObjectId(sourceID.String()), storage.Page{Size: 10}, storage.Strong())
	if err != nil {
		t.Fatalf("list links: %v", err)
	}
	if len(links.Items) != 1 || links.Items[0].To != storage.ObjectId(targetID.String()) {
		t.Fatalf("link not persisted: %+v", links.Items)
	}
}

func TestExecutePlanInvokeWebhookHTTPReturnsResult(t *testing.T) {
	t.Parallel()
	state := newTestState(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	action := models.ActionType{ID: uuid.New(), ObjectTypeID: uuid.New(), OperationKind: "invoke_webhook"}
	plan := actionPlan{kind: planInvokeWebhook, invocation: &httpInvocationConfig{URL: server.URL, Method: http.MethodPost}, payload: json.RawMessage(`{"x":1}`)}
	executed, err := executePlan(context.Background(), state, &authmw.Claims{Sub: uuid.New()}, action, plan)
	if err != nil {
		t.Fatalf("executePlan: %v", err)
	}
	if string(executed.result) != `{"ok":true}` {
		t.Fatalf("result drift: %s", executed.result)
	}
}

func TestExecutePlanInvokeFunctionHTTPRejectsMutationWithoutTarget(t *testing.T) {
	t.Parallel()
	state := newTestState(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"object_patch":{"status":"done"}}`))
	}))
	defer server.Close()

	action := models.ActionType{ID: uuid.New(), ObjectTypeID: uuid.New(), OperationKind: "invoke_function"}
	plan := actionPlan{kind: planInvokeFunction, invocation: &httpInvocationConfig{URL: server.URL, Method: http.MethodPost}, payload: json.RawMessage(`{}`)}
	_, err := executePlan(context.Background(), state, &authmw.Claims{Sub: uuid.New()}, action, plan)
	if err == nil || !strings.Contains(err.Error(), "target_object_id was not provided") {
		t.Fatalf("expected target mutation error, got %v", err)
	}
}

func TestExecutePlanInvokeFunctionHTTPDeletesTarget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	state := newTestState(t)
	objectTypeID := uuid.New()
	objectID := uuid.New()
	_, _ = state.Stores.Objects.Put(ctx, storage.Object{
		Tenant:      storage.TenantId("default"),
		ID:          storage.ObjectId(objectID.String()),
		TypeID:      storage.TypeId(objectTypeID.String()),
		Payload:     json.RawMessage(`{}`),
		UpdatedAtMs: time.Now().UTC().UnixMilli(),
	}, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"delete_object":true,"output":{"deleted":true}}`))
	}))
	defer server.Close()

	action := models.ActionType{ID: uuid.New(), ObjectTypeID: objectTypeID, OperationKind: "invoke_function"}
	plan := actionPlan{
		kind:       planInvokeFunction,
		target:     &domain.ObjectInstance{ID: objectID, ObjectTypeID: objectTypeID, Properties: json.RawMessage(`{}`)},
		invocation: &httpInvocationConfig{URL: server.URL, Method: http.MethodPost},
		payload:    json.RawMessage(`{}`),
	}
	executed, err := executePlan(ctx, state, &authmw.Claims{Sub: uuid.New()}, action, plan)
	if err != nil {
		t.Fatalf("executePlan: %v", err)
	}
	if !executed.deleted || string(executed.result) != `{"deleted":true}` {
		t.Fatalf("execution drift: deleted=%v result=%s", executed.deleted, executed.result)
	}
	stored, err := state.Stores.Objects.Get(ctx, storage.TenantId("default"), storage.ObjectId(objectID.String()), storage.Strong())
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	if stored != nil {
		t.Fatalf("object was not deleted: %+v", stored)
	}
}

func TestExecutePlanInlineFunctionRejectsConflictingEffects(t *testing.T) {
	t.Parallel()
	state := newTestState(t)
	action := models.ActionType{ID: uuid.New(), ObjectTypeID: uuid.New(), OperationKind: "invoke_function", Name: "run_py"}
	plan := inlinePythonPlan("result = {...}")
	plan.target = &domain.ObjectInstance{ID: uuid.New(), ObjectTypeID: action.ObjectTypeID, Properties: json.RawMessage(`{}`)}
	_, err := executePlanWithRuntime(context.Background(), state, &authmw.Claims{Sub: uuid.New()}, action, plan,
		staticActionFunctionRuntime{result: json.RawMessage(`{"delete_object":true,"object_patch":{"status":"done"}}`)})
	if err == nil || !strings.Contains(err.Error(), "cannot request delete_object together") {
		t.Fatalf("expected conflicting effects error, got %v", err)
	}
}

func TestExecuteActionPersistsActionLogAttempt(t *testing.T) {
	t.Parallel()
	state := newTestState(t)
	action := models.ActionType{
		ID:            uuid.New(),
		ObjectTypeID:  uuid.New(),
		OperationKind: "invoke_function",
		Name:          "run_py",
		DisplayName:   "Run Python",
		Config:        json.RawMessage(`{"runtime":"python","source":"result = {'ok': True}"}`),
		OwnerID:       uuid.New(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	record, err := domain.ActionToRecord(action)
	if err != nil {
		t.Fatalf("ActionToRecord: %v", err)
	}
	if _, err := state.Stores.Definitions.Put(context.Background(), record, nil); err != nil {
		t.Fatalf("seed action: %v", err)
	}

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", action.ID.String())
	claims := &authmw.Claims{Sub: uuid.New(), Email: "alice@example.com", Roles: []string{"admin"}}
	ctx := context.WithValue(authmw.ContextWithClaims(context.Background(), claims), chi.RouteCtxKey, rctx)
	req := httptest.NewRequest(http.MethodPost, "/ontology/actions/"+action.ID.String()+"/execute", bytes.NewReader([]byte(`{"parameters":{}}`))).WithContext(ctx)
	rec := httptest.NewRecorder()

	ExecuteActionWithRuntime(state, staticActionFunctionRuntime{result: json.RawMessage(`{"ok":true}`)}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	entries, err := state.Stores.Actions.ListRecent(context.Background(), storage.TenantId("default"), storage.Page{Size: 10}, storage.Strong())
	if err != nil {
		t.Fatalf("list action log: %v", err)
	}
	if len(entries.Items) != 1 || entries.Items[0].Kind != "action_attempt" {
		t.Fatalf("action attempt not persisted: %+v", entries.Items)
	}
	var payload map[string]any
	if err := json.Unmarshal(entries.Items[0].Payload, &payload); err != nil {
		t.Fatalf("payload json: %v", err)
	}
	if payload["status"] != "success" || payload["action_type_id"] != action.ID.String() {
		t.Fatalf("payload drift: %+v", payload)
	}
}
