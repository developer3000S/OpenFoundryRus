package functions

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
)

type fakeFunctionPackageLoader struct{ pkg *models.FunctionPackage }

func (f fakeFunctionPackageLoader) LoadFunctionPackage(context.Context, *ontologykernel.AppState, uuid.UUID) (*models.FunctionPackage, error) {
	return f.pkg, nil
}

type recordedFunctionRun struct {
	status       string
	errorMessage *string
	pkg          models.FunctionPackageSummary
	runCtx       domain.FunctionPackageRunContext
}

type fakeFunctionRunRecorder struct{ runs []recordedFunctionRun }

func (f *fakeFunctionRunRecorder) RecordFunctionPackageRun(_ context.Context, _ *ontologykernel.AppState, pkg models.FunctionPackageSummary, runCtx domain.FunctionPackageRunContext, _ time.Time, _ time.Time, _ int64, status string, errorMessage *string) error {
	f.runs = append(f.runs, recordedFunctionRun{status: status, errorMessage: errorMessage, pkg: pkg, runCtx: runCtx})
	return nil
}

type fakePythonInlineRuntime struct {
	result      []byte
	err         error
	seenSource  string
	seenInput   []byte
	seenTimeout uint32
}

func (f *fakePythonInlineRuntime) ExecuteInline(_ context.Context, source string, inputJSON []byte, timeoutSeconds uint32) (*ontologykernel.InlineRuntimeResult, error) {
	f.seenSource = source
	f.seenInput = append([]byte(nil), inputJSON...)
	f.seenTimeout = timeoutSeconds
	if f.err != nil {
		return nil, f.err
	}
	return &ontologykernel.InlineRuntimeResult{ResultJSON: f.result, Stdout: "stdout", Stderr: "stderr"}, nil
}

func testFunctionState(runtime ontologykernel.PythonInlineRuntime) *ontologykernel.AppState {
	return &ontologykernel.AppState{
		Stores:             stores.NewInMemory(),
		JWTConfig:          authmw.NewJWTConfig("test-secret"),
		OntologyServiceURL: "http://ontology.test",
		AIServiceURL:       "http://ai.test",
		PythonRuntime:      runtime,
	}
}

func testFunctionPackage(t *testing.T) *models.FunctionPackage {
	t.Helper()
	now := time.Now().UTC()
	return &models.FunctionPackage{
		ID:           uuid.New(),
		Name:         "classify_case",
		Version:      "1.0.0",
		DisplayName:  "Classify Case",
		Description:  "test package",
		Runtime:      "python",
		Source:       "result = {'ok': True}",
		Entrypoint:   "handler",
		Capabilities: models.FunctionCapabilities{AllowOntologyRead: true, TimeoutSeconds: 7, MaxSourceBytes: 1024},
		OwnerID:      uuid.New(),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func simulateRequest(t *testing.T, packageID, objectTypeID uuid.UUID) *http.Request {
	t.Helper()
	body, err := json.Marshal(models.SimulateFunctionPackageRequest{
		ObjectTypeID: objectTypeID,
		Parameters:   json.RawMessage(`{"x":1}`),
	})
	if err != nil {
		t.Fatalf("request body: %v", err)
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", packageID.String())
	claims := &authmw.Claims{Sub: uuid.New(), Email: "runner@example.com", Roles: []string{"admin"}}
	ctx := authmw.ContextWithClaims(context.Background(), claims)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	return httptest.NewRequest(http.MethodPost, "/functions/"+packageID.String()+"/simulate", bytes.NewReader(body)).WithContext(ctx)
}

func TestSimulateFunctionPackagePythonOK(t *testing.T) {
	t.Parallel()
	pkg := testFunctionPackage(t)
	objectTypeID := uuid.New()
	runtime := &fakePythonInlineRuntime{result: []byte(`{"value":1,"stdout":["hi"],"stderr":["warn"]}`)}
	state := testFunctionState(runtime)
	recorder := &fakeFunctionRunRecorder{}

	rec := httptest.NewRecorder()
	SimulateFunctionPackageWithDeps(state, functionPackageSimulationDeps{
		Loader:   fakeFunctionPackageLoader{pkg: pkg},
		Recorder: recorder,
	}).ServeHTTP(rec, simulateRequest(t, pkg.ID, objectTypeID))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var body models.SimulateFunctionPackageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response json: %v", err)
	}
	if body.Package.ID != pkg.ID || string(body.Result) != `{"value":1,"stdout":["hi"],"stderr":["warn"]}` {
		t.Fatalf("response drift: package=%+v result=%s", body.Package, body.Result)
	}
	if runtime.seenSource != pkg.Source || runtime.seenTimeout != 7 {
		t.Fatalf("runtime request drift: source=%q timeout=%d", runtime.seenSource, runtime.seenTimeout)
	}
	var input map[string]any
	if err := json.Unmarshal(runtime.seenInput, &input); err != nil {
		t.Fatalf("runtime input json: %v", err)
	}
	if input["functionPackage"] == nil || input["serviceToken"] == "" {
		t.Fatalf("runtime envelope missing package/token: %+v", input)
	}
	if len(recorder.runs) != 1 || recorder.runs[0].status != "success" || recorder.runs[0].errorMessage != nil {
		t.Fatalf("success run not recorded correctly: %+v", recorder.runs)
	}
	if recorder.runs[0].runCtx.InvocationKind != "simulation" || recorder.runs[0].runCtx.ObjectTypeID == nil || *recorder.runs[0].runCtx.ObjectTypeID != objectTypeID {
		t.Fatalf("run context drift: %+v", recorder.runs[0].runCtx)
	}
}

func TestSimulateFunctionPackagePythonExceptionRecordsFailure(t *testing.T) {
	t.Parallel()
	pkg := testFunctionPackage(t)
	state := testFunctionState(&fakePythonInlineRuntime{err: errors.New("Traceback: boom")})
	recorder := &fakeFunctionRunRecorder{}

	rec := httptest.NewRecorder()
	SimulateFunctionPackageWithDeps(state, functionPackageSimulationDeps{Loader: fakeFunctionPackageLoader{pkg: pkg}, Recorder: recorder}).ServeHTTP(rec, simulateRequest(t, pkg.ID, uuid.New()))

	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "Traceback: boom") {
		t.Fatalf("unexpected failure response: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(recorder.runs) != 1 || recorder.runs[0].status != "failure" || recorder.runs[0].errorMessage == nil || !strings.Contains(*recorder.runs[0].errorMessage, "Traceback: boom") {
		t.Fatalf("failure run not recorded correctly: %+v", recorder.runs)
	}
}

func TestSimulateFunctionPackageMalformedResultRecordsFailure(t *testing.T) {
	t.Parallel()
	pkg := testFunctionPackage(t)
	state := testFunctionState(&fakePythonInlineRuntime{result: []byte(`{"ok":`)})
	recorder := &fakeFunctionRunRecorder{}

	rec := httptest.NewRecorder()
	SimulateFunctionPackageWithDeps(state, functionPackageSimulationDeps{Loader: fakeFunctionPackageLoader{pkg: pkg}, Recorder: recorder}).ServeHTTP(rec, simulateRequest(t, pkg.ID, uuid.New()))

	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "failed to decode Python function response") {
		t.Fatalf("unexpected malformed response: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(recorder.runs) != 1 || recorder.runs[0].status != "failure" || recorder.runs[0].errorMessage == nil || !strings.Contains(*recorder.runs[0].errorMessage, "malformed result JSON") {
		t.Fatalf("malformed failure run not recorded correctly: %+v", recorder.runs)
	}
}

func TestSimulateFunctionPackageRuntimeMissingRecordsFailure(t *testing.T) {
	t.Parallel()
	pkg := testFunctionPackage(t)
	state := testFunctionState(nil)
	recorder := &fakeFunctionRunRecorder{}

	rec := httptest.NewRecorder()
	SimulateFunctionPackageWithDeps(state, functionPackageSimulationDeps{Loader: fakeFunctionPackageLoader{pkg: pkg}, Recorder: recorder}).ServeHTTP(rec, simulateRequest(t, pkg.ID, uuid.New()))

	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "python_runtime_not_wired") {
		t.Fatalf("unexpected missing-runtime response: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(recorder.runs) != 1 || recorder.runs[0].status != "failure" || recorder.runs[0].errorMessage == nil || !strings.Contains(*recorder.runs[0].errorMessage, "python runtime not wired") {
		t.Fatalf("missing-runtime failure run not recorded correctly: %+v", recorder.runs)
	}
}

func TestFunctionAuthoringSurfaceFixture(t *testing.T) {
	rec := httptest.NewRecorder()
	GetFunctionAuthoringSurface().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/functions/authoring-surface", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"templates", "sdk_packages", "cli_commands", "python"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("authoring surface missing %q: %s", want, rec.Body.String())
		}
	}
}

func TestFunctionPackageCRUDValidateRunsMetricsWithInMemoryStores(t *testing.T) {
	state := testFunctionState(&fakePythonInlineRuntime{result: []byte(`{"ok":true}`)})
	owner := uuid.New()
	createBody := `{"name":"score_case","runtime":"python","source":"result = {'ok': True}","capabilities":{"allow_ontology_read":true,"timeout_seconds":7,"max_source_bytes":1024}}`
	createRec := httptest.NewRecorder()
	CreateFunctionPackage(state).ServeHTTP(createRec, functionRequest(http.MethodPost, "/functions", createBody, nil, owner))
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}
	var pkg models.FunctionPackage
	if err := json.Unmarshal(createRec.Body.Bytes(), &pkg); err != nil {
		t.Fatalf("created package json: %v", err)
	}

	listRec := httptest.NewRecorder()
	ListFunctionPackages(state).ServeHTTP(listRec, functionRequest(http.MethodGet, "/functions?runtime=python", ``, nil, owner))
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), `"total":1`) {
		t.Fatalf("list status=%d body=%s", listRec.Code, listRec.Body.String())
	}

	validateRec := httptest.NewRecorder()
	ValidateFunctionPackage(state).ServeHTTP(validateRec, functionRequest(http.MethodPost, "/functions/"+pkg.ID.String()+"/validate", `{"parameters":{"x":1}}`, map[string]string{"id": pkg.ID.String()}, owner))
	if validateRec.Code != http.StatusOK || !strings.Contains(validateRec.Body.String(), `"valid":true`) {
		t.Fatalf("validate status=%d body=%s", validateRec.Code, validateRec.Body.String())
	}

	missingValidate := httptest.NewRecorder()
	ValidateFunctionPackage(state).ServeHTTP(missingValidate, functionRequest(http.MethodPost, "/functions/"+uuid.New().String()+"/validate", `{}`, map[string]string{"id": uuid.New().String()}, owner))
	if missingValidate.Code != http.StatusNotFound {
		t.Fatalf("missing validate status=%d body=%s", missingValidate.Code, missingValidate.Body.String())
	}

	simRec := httptest.NewRecorder()
	objectTypeID := uuid.New()
	SimulateFunctionPackage(state).ServeHTTP(simRec, functionRequest(http.MethodPost, "/functions/"+pkg.ID.String()+"/simulate", fmt.Sprintf(`{"object_type_id":"%s","parameters":{"x":1}}`, objectTypeID), map[string]string{"id": pkg.ID.String()}, owner))
	if simRec.Code != http.StatusOK || !strings.Contains(simRec.Body.String(), `"ok":true`) {
		t.Fatalf("simulate status=%d body=%s", simRec.Code, simRec.Body.String())
	}

	runsRec := httptest.NewRecorder()
	ListFunctionPackageRuns(state).ServeHTTP(runsRec, functionRequest(http.MethodGet, "/functions/"+pkg.ID.String()+"/runs", ``, map[string]string{"id": pkg.ID.String()}, owner))
	if runsRec.Code != http.StatusOK || !strings.Contains(runsRec.Body.String(), `"total":1`) {
		t.Fatalf("runs status=%d body=%s", runsRec.Code, runsRec.Body.String())
	}

	metricsRec := httptest.NewRecorder()
	GetFunctionPackageMetrics(state).ServeHTTP(metricsRec, functionRequest(http.MethodGet, "/functions/"+pkg.ID.String()+"/metrics", ``, map[string]string{"id": pkg.ID.String()}, owner))
	if metricsRec.Code != http.StatusOK || !strings.Contains(metricsRec.Body.String(), `"total_runs":1`) || !strings.Contains(metricsRec.Body.String(), `"successful_runs":1`) {
		t.Fatalf("metrics status=%d body=%s", metricsRec.Code, metricsRec.Body.String())
	}

	deleteRec := httptest.NewRecorder()
	DeleteFunctionPackage(state).ServeHTTP(deleteRec, functionRequest(http.MethodDelete, "/functions/"+pkg.ID.String(), ``, map[string]string{"id": pkg.ID.String()}, owner))
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
}

func TestCreateFunctionPackageValidationFailure(t *testing.T) {
	state := testFunctionState(nil)
	rec := httptest.NewRecorder()
	CreateFunctionPackage(state).ServeHTTP(rec, functionRequest(http.MethodPost, "/functions", `{"name":"bad","runtime":"python","source":""}`, nil, uuid.New()))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "requires a non-empty source") {
		t.Fatalf("validation failure status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func functionRequest(method, path, body string, params map[string]string, sub uuid.UUID) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	claims := &authmw.Claims{Sub: sub, Email: "fn@example.com", Roles: []string{"admin"}}
	ctx := authmw.ContextWithClaims(context.Background(), claims)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	return httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
}
