package domain

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	kernelstores "github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
)

// Mirrors the Rust unit `parses_typescript_runtime_config`.
func TestParseInlineFunctionConfig_TypeScript(t *testing.T) {
	t.Parallel()
	body := []byte(`{"runtime":"typescript","source":"export default async function handler() { return { ok: true }; }"}`)
	cfg, err := ParseInlineFunctionConfig(body)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if cfg == nil || cfg.Kind != InlineFunctionTypeScript {
		t.Fatalf("expected TypeScript variant, got %+v", cfg)
	}
	if cfg.RuntimeName() != "typescript" {
		t.Fatalf("runtime drift: %s", cfg.RuntimeName())
	}
}

func TestParseInlineFunctionConfig_Python(t *testing.T) {
	t.Parallel()
	body := []byte(`{"runtime":"python","source":"def handler(c): return {}"}`)
	cfg, err := ParseInlineFunctionConfig(body)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if cfg == nil || cfg.Kind != InlineFunctionPython {
		t.Fatalf("expected Python variant, got %+v", cfg)
	}
}

func TestParseInlineFunctionConfig_NoRuntime(t *testing.T) {
	t.Parallel()
	cfg, err := ParseInlineFunctionConfig([]byte(`{"source":"x"}`))
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected (nil, nil) when runtime field missing")
	}
}

func TestParseInlineFunctionConfig_RejectsEmptySource(t *testing.T) {
	t.Parallel()
	_, err := ParseInlineFunctionConfig([]byte(`{"runtime":"python","source":"   "}`))
	if err == nil {
		t.Fatal("expected error on empty python source")
	}
	_, err = ParseInlineFunctionConfig([]byte(`{"runtime":"typescript","source":""}`))
	if err == nil {
		t.Fatal("expected error on empty TypeScript source")
	}
}

func TestParseInlineFunctionConfig_RejectsUnknownRuntime(t *testing.T) {
	t.Parallel()
	_, err := ParseInlineFunctionConfig([]byte(`{"runtime":"ruby","source":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported function runtime") {
		t.Fatalf("expected unsupported runtime error, got %v", err)
	}
}

func TestValidateFunctionCapabilities_RejectsExceedingSource(t *testing.T) {
	t.Parallel()
	cfg := InlineFunctionConfig{
		Kind:       InlineFunctionTypeScript,
		TypeScript: &InlineTypeScriptFunctionConfig{Runtime: "typescript", Source: strings.Repeat("x", 100)},
	}
	caps := models.FunctionCapabilities{
		MaxSourceBytes: 50, TimeoutSeconds: 15,
	}
	if err := ValidateFunctionCapabilities(cfg, caps, nil); err == nil ||
		!strings.Contains(err.Error(), "exceeds max_source_bytes") {
		t.Fatalf("expected max_source_bytes failure, got %v", err)
	}
}

func TestValidateFunctionCapabilities_RejectsBadTimeout(t *testing.T) {
	t.Parallel()
	cfg := InlineFunctionConfig{
		Kind:       InlineFunctionTypeScript,
		TypeScript: &InlineTypeScriptFunctionConfig{Runtime: "typescript", Source: "x"},
	}
	caps := models.FunctionCapabilities{TimeoutSeconds: 0, MaxSourceBytes: 1024}
	if err := ValidateFunctionCapabilities(cfg, caps, nil); err == nil {
		t.Fatal("expected timeout=0 to fail")
	}
	caps.TimeoutSeconds = 301
	if err := ValidateFunctionCapabilities(cfg, caps, nil); err == nil {
		t.Fatal("expected timeout>300 to fail")
	}
}

func TestValidateFunctionCapabilities_RejectsBadEntrypoint(t *testing.T) {
	t.Parallel()
	cfg := InlineFunctionConfig{
		Kind:       InlineFunctionTypeScript,
		TypeScript: &InlineTypeScriptFunctionConfig{Runtime: "typescript", Source: "x"},
	}
	caps := models.FunctionCapabilities{TimeoutSeconds: 15, MaxSourceBytes: 1024}
	pkg := models.FunctionPackageSummary{Entrypoint: "main"}
	if err := ValidateFunctionCapabilities(cfg, caps, &pkg); err == nil ||
		!strings.Contains(err.Error(), "unsupported function package entrypoint") {
		t.Fatalf("expected entrypoint failure, got %v", err)
	}
	pkg.Entrypoint = "default"
	if err := ValidateFunctionCapabilities(cfg, caps, &pkg); err != nil {
		t.Fatalf("default entrypoint must be accepted: %v", err)
	}
}

// Mirrors the Rust unit `enriches_typescript_result_with_logs`.
func TestEnrichTypeScriptResultWithLogs(t *testing.T) {
	t.Parallel()
	got := enrichTypeScriptResult(
		json.RawMessage(`{"object_patch":{"status":"done"}}`),
		[]string{"hello"}, nil,
	)
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(got, &asMap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.Contains(string(asMap["stdout"]), "hello") {
		t.Fatalf("stdout drift: %s", asMap["stdout"])
	}
	var output map[string]any
	_ = json.Unmarshal(asMap["output"], &output)
	if stdout, ok := output["stdout"].([]any); !ok || len(stdout) != 1 || stdout[0] != "hello" {
		t.Fatalf("output.stdout drift: %v", output)
	}
}

func TestExecuteInlineTypeScriptFunctionCanInvokeDataConnectionWebhook(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node runtime not installed")
	}
	sourceID := uuid.New()
	type connectorRequest struct {
		method string
		path   string
		auth   string
		inputs map[string]any
	}
	requests := make(chan connectorRequest, 1)
	connector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Inputs map[string]any `json:"inputs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode connector request: %v", err)
		}
		requests <- connectorRequest{method: r.Method, path: r.URL.Path, auth: r.Header.Get("Authorization"), inputs: body.Inputs}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status": 200,
			"response": {"current": {"temperature_2m": 84}},
			"output_parameters": {"temperature": 84, "wind_speed": 4.8},
			"history": {"stored": true}
		}`))
	}))
	defer connector.Close()

	state := &ontologykernel.AppState{
		Stores:                        kernelstores.NewInMemory(),
		JWTConfig:                     authmw.NewJWTConfig("test-secret"),
		NodeRuntimeCommand:            node,
		OntologyServiceURL:            "http://ontology.local",
		AIServiceURL:                  "http://ai.local",
		ConnectorManagementServiceURL: connector.URL,
	}
	claims := &authmw.Claims{Sub: uuid.New(), Roles: []string{"admin"}}
	action := &models.ActionType{ID: uuid.New(), ObjectTypeID: uuid.New(), OperationKind: "invoke_function", Name: "weather_fn"}
	resolved := &ResolvedInlineFunction{
		Config: InlineFunctionConfig{Kind: InlineFunctionTypeScript, TypeScript: &InlineTypeScriptFunctionConfig{
			Runtime: "typescript",
			Source: `
export default async function handler(context) {
  const weather = await context.sdk.dataConnection.invokeWebhook({
    sourceId: context.parameters.sourceId,
    inputs: {
      latitude: context.parameters.latitude,
      longitude: context.parameters.longitude,
    },
  });
  return {
    output: {
      temperature: weather.output_parameters.temperature,
      windSpeed: weather.output_parameters.wind_speed,
      historyStored: weather.history.stored,
    },
  };
}`,
		}},
		Capabilities: models.FunctionCapabilities{
			AllowOntologyRead: true,
			AllowNetwork:      false,
			TimeoutSeconds:    15,
			MaxSourceBytes:    8192,
		},
	}
	params := map[string]json.RawMessage{
		"sourceId":  mustRawJSON(sourceID.String()),
		"latitude":  json.RawMessage(`40.016353`),
		"longitude": json.RawMessage(`-105.34458`),
	}

	result, err := ExecuteInlineFunction(context.Background(), state, claims, action, nil, params, resolved, nil)
	if err != nil {
		t.Fatalf("ExecuteInlineFunction: %v", err)
	}
	var got connectorRequest
	select {
	case got = <-requests:
	default:
		t.Fatal("connector webhook was not called")
	}
	if got.method != http.MethodPost {
		t.Fatalf("unexpected method: %s", got.method)
	}
	if got.path != "/api/v1/webhooks/"+sourceID.String()+"/invoke" {
		t.Fatalf("unexpected path: %s", got.path)
	}
	if !strings.HasPrefix(got.auth, "Bearer ") {
		t.Fatalf("expected service bearer token, got %q", got.auth)
	}
	if got.inputs["latitude"] != float64(40.016353) || got.inputs["longitude"] != float64(-105.34458) {
		t.Fatalf("webhook inputs drift: %#v", got.inputs)
	}
	var payload struct {
		Output struct {
			Temperature   float64 `json:"temperature"`
			WindSpeed     float64 `json:"windSpeed"`
			HistoryStored bool    `json:"historyStored"`
		} `json:"output"`
	}
	if err := json.Unmarshal(result, &payload); err != nil {
		t.Fatalf("result JSON: %v body=%s", err, result)
	}
	if payload.Output.Temperature != 84 || payload.Output.WindSpeed != 4.8 || !payload.Output.HistoryStored {
		t.Fatalf("unexpected external function output: %+v", payload.Output)
	}
}

func mustRawJSON(value any) json.RawMessage {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return body
}

// Mirrors `resolves_exact_versioned_package_reference`.
func TestSelectFunctionPackageVersion_ExactMatch(t *testing.T) {
	t.Parallel()
	packages := []models.FunctionPackage{
		{ID: uuid.Nil, Name: "triage", Version: "1.1.0"},
		{ID: uuid.Nil, Name: "triage", Version: "1.2.0"},
	}
	ref := versionedFunctionPackageReferenceConfig{Name: "triage", Version: "1.1.0"}
	pkg, err := selectFunctionPackageVersion(packages, ref)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if pkg == nil || pkg.Version != "1.1.0" {
		t.Fatalf("expected 1.1.0, got %+v", pkg)
	}
}

// Mirrors `resolves_latest_compatible_auto_upgrade_release`.
func TestSelectFunctionPackageVersion_AutoUpgradeLatest(t *testing.T) {
	t.Parallel()
	packages := []models.FunctionPackage{
		{Name: "triage", Version: "1.1.0"},
		{Name: "triage", Version: "1.3.2"},
		{Name: "triage", Version: "2.0.0"},
	}
	ref := versionedFunctionPackageReferenceConfig{
		Name: "triage", Version: "1.2.0", AutoUpgrade: true,
	}
	pkg, err := selectFunctionPackageVersion(packages, ref)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if pkg == nil {
		t.Fatal("expected a match")
	}
	if pkg.Version != "1.3.2" {
		t.Fatalf("expected latest 1.x.y >= 1.2.0 = 1.3.2, got %s", pkg.Version)
	}
}

// Mirrors `rejects_auto_upgrade_for_unstable_baseline`.
func TestSelectFunctionPackageVersion_RejectsUnstableAutoUpgrade(t *testing.T) {
	t.Parallel()
	packages := []models.FunctionPackage{{Name: "triage", Version: "0.3.0"}}
	ref := versionedFunctionPackageReferenceConfig{
		Name: "triage", Version: "0.3.0", AutoUpgrade: true,
	}
	_, err := selectFunctionPackageVersion(packages, ref)
	if err == nil || !strings.Contains(err.Error(), "stable baseline version 1.0.0 or above") {
		t.Fatalf("expected stable-baseline error, got %v", err)
	}
}

func TestExecuteInlinePythonFunction_ReturnsSentinel(t *testing.T) {
	t.Parallel()
	_, err := ExecuteInlinePythonFunction(nil, nil, nil, nil, nil, nil, nil, nil)
	if !errors.Is(err, ErrPythonRuntimeNotWired) {
		t.Fatalf("expected ErrPythonRuntimeNotWired, got %v", err)
	}
}

func TestObjectToJSON_RoundTrip(t *testing.T) {
	t.Parallel()
	props, _ := json.Marshal(map[string]any{"a": 1})
	obj := ObjectInstance{
		ID:           uuid.New(),
		ObjectTypeID: uuid.New(),
		Properties:   props,
		Marking:      "public",
		CreatedBy:    uuid.New(),
	}
	out := ObjectToJSON(obj)
	var asMap map[string]any
	if err := json.Unmarshal(out, &asMap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"id", "object_type_id", "marking", "properties", "created_by"} {
		if _, ok := asMap[key]; !ok {
			t.Errorf("missing key %q in object_to_json output", key)
		}
	}
}
