package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseDocsGenerateOpenAPIFlags(t *testing.T) {
	cfg, err := parseArgs([]string{"docs", "generate-openapi", "--proto-dir", "../proto", "--output", "openapi.json"}, bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if cfg.kind != cmdGenerateOpenAPI || cfg.protoDir != "../proto" || cfg.output != "openapi.json" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestParseScenarioAndMockProviderFlags(t *testing.T) {
	smokeCfg, err := parseArgs([]string{"smoke", "run", "--scenario", "smoke.json", "--output", "report.json"}, bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("smoke parseArgs returned error: %v", err)
	}
	if smokeCfg.kind != cmdSmokeRun || smokeCfg.scenario != "smoke.json" || smokeCfg.output != "report.json" {
		t.Fatalf("unexpected smoke config: %+v", smokeCfg)
	}

	mockCfg, err := parseArgs([]string{"mock-provider", "serve", "--host", "0.0.0.0", "--port", "19090"}, bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("mock parseArgs returned error: %v", err)
	}
	if mockCfg.kind != cmdMockProviderServe || mockCfg.host != "0.0.0.0" || mockCfg.port != 19090 {
		t.Fatalf("unexpected mock config: %+v", mockCfg)
	}
}

func TestParseRejectsMissingRequiredFlags(t *testing.T) {
	if _, err := parseArgs([]string{"docs", "generate-sdk-python", "--input", "openapi.json"}, bytes.NewBuffer(nil)); err == nil {
		t.Fatal("expected missing --output to fail")
	}
	if _, err := parseArgs([]string{"mock-provider", "serve", "--port", "70000"}, bytes.NewBuffer(nil)); err == nil {
		t.Fatal("expected invalid port to fail")
	}
}

func TestGenerateOpenAPIFromProto(t *testing.T) {
	dir := t.TempDir()
	protoDir := filepath.Join(dir, "proto")
	if err := os.Mkdir(protoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	proto := `syntax = "proto3";
package openfoundry.test.v1;
message GetWidgetRequest { string id = 1; }
message WidgetResponse { string id = 1; }
service WidgetService { rpc GetWidget(GetWidgetRequest) returns (WidgetResponse); }
`
	if err := os.WriteFile(filepath.Join(protoDir, "widget.proto"), []byte(proto), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "openapi.json")
	if err := generateOpenAPI(protoDir, out); err != nil {
		t.Fatalf("generateOpenAPI returned error: %v", err)
	}
	var spec openAPISpec
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	if _, ok := spec.Paths["/openfoundry/test/v1/widget-service/get-widget"]["get"]; !ok {
		t.Fatalf("generated spec missing expected GET path: %+v", spec.Paths)
	}
}

func TestSmokeRunCapturesAndAsserts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"abc","status":"ok"}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	scenario := filepath.Join(dir, "smoke.json")
	output := filepath.Join(dir, "report.json")
	raw := `{"base_url":` + quote(server.URL) + `,"steps":[{"name":"get","method":"GET","path":"/thing","expected_status":200,"capture":{"THING_ID":"id"},"expect":[{"path":"status","equals":"ok"}]}]}`
	if err := os.WriteFile(scenario, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runSmokeSuite(context.Background(), scenario, output); err != nil {
		t.Fatalf("runSmokeSuite returned error: %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"success": true`)) || !bytes.Contains(data, []byte(`"THING_ID": "abc"`)) {
		t.Fatalf("unexpected smoke report: %s", data)
	}
}

func TestBenchmarkRunWritesLatencyReport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusCreated) }))
	defer server.Close()
	dir := t.TempDir()
	scenario := filepath.Join(dir, "bench.json")
	output := filepath.Join(dir, "report.json")
	raw := `{"base_url":` + quote(server.URL) + `,"warmup_iterations":1,"measure_iterations":2,"scenarios":[{"name":"create","method":"POST","path":"/thing","expected_status":201,"tags":["unit"]}]}`
	if err := os.WriteFile(scenario, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runBenchmarkSuite(context.Background(), scenario, output); err != nil {
		t.Fatalf("runBenchmarkSuite returned error: %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"iterations": 2`)) || !bytes.Contains(data, []byte(`"statuses": [`)) {
		t.Fatalf("unexpected benchmark report: %s", data)
	}
}

func quote(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}
