package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
	pb "github.com/openfoundry/openfoundry-go/libs/proto-gen/runtime"
	pythonsidecar "github.com/openfoundry/openfoundry-go/libs/python-sidecar"
	"github.com/openfoundry/openfoundry-go/services/ontology-actions-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/ontology-actions-service/internal/server"
)

func TestPythonSidecarConfigMapsServiceConfigToManager(t *testing.T) {
	cfg := &config.Config{
		PythonSidecarBinary:  "/opt/openfoundry-pyruntime",
		PythonSidecarArgs:    []string{"--debug"},
		PythonSidecarEnv:     []string{"PYRUNTIME_LOG=debug"},
		PythonSidecarTimeout: 11 * time.Second,
	}
	got := pythonSidecarConfig(cfg)
	if got.BinaryPath != cfg.PythonSidecarBinary {
		t.Fatalf("BinaryPath = %q", got.BinaryPath)
	}
	if !reflect.DeepEqual(got.Args, cfg.PythonSidecarArgs) {
		t.Fatalf("Args = %#v", got.Args)
	}
	if !reflect.DeepEqual(got.Env, cfg.PythonSidecarEnv) {
		t.Fatalf("Env = %#v", got.Env)
	}
	if got.StartupTimeout != cfg.PythonSidecarTimeout || got.HardCallTimeout != cfg.PythonSidecarTimeout {
		t.Fatalf("timeouts = startup %s hard %s", got.StartupTimeout, got.HardCallTimeout)
	}

	got.Args[0] = "mutated"
	got.Env[0] = "mutated=1"
	if cfg.PythonSidecarArgs[0] == "mutated" || cfg.PythonSidecarEnv[0] == "mutated=1" {
		t.Fatalf("pythonSidecarConfig must defensively copy slices")
	}
}

func TestBuildStateRequiresDatabaseURLUnlessDevMode(t *testing.T) {
	cfg := &config.Config{}
	cfg.JWTSecret = "test-secret"
	_, _, _, err := buildState(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("expected clear DATABASE_URL error, got %v", err)
	}
}

func TestBuildStateDevModeUsesExplicitInMemoryState(t *testing.T) {
	cfg := &config.Config{DevMode: true}
	cfg.JWTSecret = "test-secret"
	state, cleanup, deps, err := buildState(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("buildState: %v", err)
	}
	if cleanup != nil {
		t.Fatal("dev in-memory state should not return production cleanup")
	}
	if deps != nil {
		t.Fatalf("dev in-memory state should not return dependency probes, got %#v", deps)
	}
	if state == nil || state.Stores.Objects == nil || state.DB != nil {
		t.Fatalf("unexpected dev state: %#v", state)
	}
}

func TestBuildStateWithDatabaseURLRequiresCassandraStores(t *testing.T) {
	cfg := &config.Config{DatabaseURL: "postgres://user:pass@localhost:5432/openfoundry"}
	cfg.JWTSecret = "test-secret"
	_, _, _, err := buildState(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "CASSANDRA_CONTACT_POINTS is required") {
		t.Fatalf("expected clear Cassandra stores error, got %v", err)
	}
}

func TestBuildSearchBackendRequiresEndpointWhenConfigured(t *testing.T) {
	_, err := buildSearchBackend(&config.Config{SearchBackend: "vespa"})
	if err == nil || !strings.Contains(err.Error(), "SEARCH_ENDPOINT") {
		t.Fatalf("expected SEARCH_ENDPOINT error, got %v", err)
	}
}

func TestBuildSearchBackendReturnsNilWhenUnconfigured(t *testing.T) {
	backend, err := buildSearchBackend(&config.Config{})
	if err != nil {
		t.Fatalf("buildSearchBackend: %v", err)
	}
	if backend != nil {
		t.Fatalf("expected nil backend when search is unconfigured, got %#v", backend)
	}
}

func TestBuildStoresValidatesKeyspaceBeforeDial(t *testing.T) {
	cfg := &config.Config{
		CassandraContactPoints: "127.0.0.1:9042",
		CassandraKeyspace:      "bad-keyspace",
	}
	_, _, _, err := buildStores(context.Background(), cfg, nil)
	if err == nil || !strings.Contains(err.Error(), "not a valid CQL identifier") {
		t.Fatalf("expected keyspace validation error, got %v", err)
	}
}

func TestValidatePythonRuntimeConfigRequiresSidecarWhenProductionPythonEnabled(t *testing.T) {
	cfg := &config.Config{DatabaseURL: "postgres://user:pass@localhost:5432/openfoundry", PythonPackagesEnabled: true}
	err := validatePythonRuntimeConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "PYTHON_SIDECAR_BINARY is required") {
		t.Fatalf("expected production sidecar requirement, got %v", err)
	}

	cfg.PythonSidecarBinary = "/opt/openfoundry-pyruntime"
	if err := validatePythonRuntimeConfig(cfg); err != nil {
		t.Fatalf("sidecar configured should pass: %v", err)
	}
}

func TestValidatePythonRuntimeConfigAllowsDevModeMissingSidecar(t *testing.T) {
	cfg := &config.Config{DevMode: true, PythonPackagesEnabled: true}
	if err := validatePythonRuntimeConfig(cfg); err != nil {
		t.Fatalf("dev mode should preserve explicit python_runtime_not_wired behavior: %v", err)
	}
}

func TestFunctionPackageSimulateUsesPythonSidecarManager(t *testing.T) {
	cfg := serviceTestConfig()
	state := newAppState(cfg, nil, stores.NewInMemory())

	mgr, err := pythonsidecar.New(pythonsidecar.Config{
		BinaryPath:      fakeSidecarBinary(t),
		StartupTimeout:  2 * time.Second,
		HardCallTimeout: 2 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("create python sidecar manager: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("start fake python sidecar manager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop(context.Background()) })
	state.PythonRuntime = pythonRuntimeAdapter{mgr: mgr}

	router := server.BuildRouter(cfg, state, nil)
	owner := uuid.New()
	created := postJSON(t, router, "/api/v1/ontology/functions", map[string]any{
		"name":    "score_case",
		"runtime": "python",
		"source":  "result = {'ignored_by_fake_sidecar': True}",
	}, owner)
	if created.Code != http.StatusCreated {
		t.Fatalf("create function status=%d body=%s", created.Code, created.Body.String())
	}
	var pkg struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &pkg); err != nil || pkg.ID == "" {
		t.Fatalf("created function package JSON id: id=%q err=%v body=%s", pkg.ID, err, created.Body.String())
	}

	simulated := postJSON(t, router, "/api/v1/ontology/functions/"+pkg.ID+"/simulate", map[string]any{
		"object_type_id": uuid.New().String(),
		"parameters": map[string]any{
			"x": 1,
		},
	}, owner)
	if simulated.Code != http.StatusOK {
		t.Fatalf("simulate function status=%d body=%s", simulated.Code, simulated.Body.String())
	}
	if !strings.Contains(simulated.Body.String(), `"fake":true`) || !strings.Contains(simulated.Body.String(), `"via":"python-sidecar-manager"`) {
		t.Fatalf("simulate response did not include fake sidecar result: %s", simulated.Body.String())
	}
}

func TestFunctionPackageSimulateWithoutSidecarReturnsMachineReadableError(t *testing.T) {
	cfg := serviceTestConfig()
	state := newAppState(cfg, nil, stores.NewInMemory())
	router := server.BuildRouter(cfg, state, nil)
	owner := uuid.New()

	created := postJSON(t, router, "/api/v1/ontology/functions", map[string]any{
		"name":    "score_case_missing_runtime",
		"runtime": "python",
		"source":  "result = {'would_need_python': True}",
	}, owner)
	if created.Code != http.StatusCreated {
		t.Fatalf("create function status=%d body=%s", created.Code, created.Body.String())
	}
	var pkg struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &pkg); err != nil || pkg.ID == "" {
		t.Fatalf("created function package JSON id: id=%q err=%v body=%s", pkg.ID, err, created.Body.String())
	}

	simulated := postJSON(t, router, "/api/v1/ontology/functions/"+pkg.ID+"/simulate", map[string]any{
		"object_type_id": uuid.New().String(),
		"parameters":     map[string]any{"x": 1},
	}, owner)
	if simulated.Code != http.StatusServiceUnavailable {
		t.Fatalf("simulate missing sidecar status=%d body=%s", simulated.Code, simulated.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(simulated.Body.Bytes(), &body); err != nil {
		t.Fatalf("missing-sidecar body not JSON: %v body=%s", err, simulated.Body.String())
	}
	if body["error"] != "python_runtime_not_wired" || !strings.Contains(body["detail"].(string), "python runtime not wired") {
		t.Fatalf("expected machine-readable python_runtime_not_wired body, got %v", body)
	}
	if _, ok := body["package"].(map[string]any); !ok {
		t.Fatalf("expected package summary in missing-sidecar body, got %v", body)
	}
	if _, ok := body["preview"].(string); !ok {
		t.Fatalf("expected preview JSON string in missing-sidecar body, got %v", body)
	}
}

func serviceTestConfig() *config.Config {
	cfg := &config.Config{DevMode: true}
	cfg.Service.Name = "ontology-actions-service"
	cfg.Service.Version = "test"
	cfg.JWTSecret = "ontology-actions-service-runtime-test-secret"
	return cfg
}

func postJSON(t *testing.T, router http.Handler, path string, payload map[string]any, owner uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+serviceTestToken(t, owner))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func serviceTestToken(t *testing.T, owner uuid.UUID) string {
	t.Helper()
	now := time.Now()
	tok, err := authmw.EncodeToken(authmw.NewJWTConfig(serviceTestConfig().JWTSecret), &authmw.Claims{
		Sub:   owner,
		IAT:   now.Unix(),
		EXP:   now.Add(time.Hour).Unix(),
		JTI:   uuid.New(),
		Email: "runtime-smoke@openfoundry.test",
		Name:  "Runtime Smoke",
		Roles: []string{"ontology.editor"},
	})
	if err != nil {
		t.Fatalf("encode test token: %v", err)
	}
	return tok
}

func fakeSidecarBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-ontology-actions-sidecar.sh")
	script := "#!/bin/sh\nGO_WANT_ONTOLOGY_ACTIONS_FAKE_SIDECAR=1 exec \"" + os.Args[0] + "\" -test.run=TestFakeOntologyActionsPythonSidecarProcess -- \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sidecar wrapper: %v", err)
	}
	return path
}

func TestFakeOntologyActionsPythonSidecarProcess(t *testing.T) {
	if os.Getenv("GO_WANT_ONTOLOGY_ACTIONS_FAKE_SIDECAR") != "1" {
		return
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("GO_WANT_ONTOLOGY_ACTIONS_FAKE_SIDECAR") == "1" {
		runFakeOntologyActionsPythonSidecarProcess()
		return
	}
	os.Exit(m.Run())
}

func runFakeOntologyActionsPythonSidecarProcess() {
	bind := ""
	for i, arg := range os.Args {
		if arg == "--bind" && i+1 < len(os.Args) {
			bind = os.Args[i+1]
			break
		}
	}
	if bind == "" || !strings.HasPrefix(bind, "unix:") {
		os.Exit(2)
	}
	socket := strings.TrimPrefix(bind, "unix:")
	_ = os.Remove(socket)
	lis, err := net.Listen("unix", socket)
	if err != nil {
		os.Exit(2)
	}
	grpcServer := grpc.NewServer()
	registerFakeOntologyActionsPythonRuntimeServiceServer(grpcServer, fakeOntologyActionsPythonRuntimeServer{})
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	if err := grpcServer.Serve(lis); err != nil {
		os.Exit(2)
	}
}

type fakeOntologyActionsPythonRuntimeServer struct{}

func (fakeOntologyActionsPythonRuntimeServer) ExecuteInlineFunction(context.Context, *pb.ExecuteInlineFunctionRequest) (*pb.ExecuteInlineFunctionResponse, error) {
	return &pb.ExecuteInlineFunctionResponse{ResultJson: `{"fake":true,"via":"python-sidecar-manager"}`, Stdout: "fake inline\n", Stderr: ""}, nil
}

func (fakeOntologyActionsPythonRuntimeServer) ExecutePipelineTransform(context.Context, *pb.ExecutePipelineTransformRequest) (*pb.ExecutePipelineTransformResponse, error) {
	return &pb.ExecutePipelineTransformResponse{}, nil
}

func (fakeOntologyActionsPythonRuntimeServer) ExecuteNotebookCell(context.Context, *pb.ExecuteNotebookCellRequest) (*pb.ExecuteNotebookCellResponse, error) {
	return &pb.ExecuteNotebookCellResponse{}, nil
}

func (fakeOntologyActionsPythonRuntimeServer) EnsureSession(context.Context, *pb.EnsureSessionRequest) (*pb.EnsureSessionResponse, error) {
	return &pb.EnsureSessionResponse{}, nil
}

func (fakeOntologyActionsPythonRuntimeServer) DropSession(context.Context, *pb.DropSessionRequest) (*pb.DropSessionResponse, error) {
	return &pb.DropSessionResponse{}, nil
}

type fakeOntologyActionsPythonRuntimeService interface {
	ExecuteInlineFunction(context.Context, *pb.ExecuteInlineFunctionRequest) (*pb.ExecuteInlineFunctionResponse, error)
	ExecutePipelineTransform(context.Context, *pb.ExecutePipelineTransformRequest) (*pb.ExecutePipelineTransformResponse, error)
	ExecuteNotebookCell(context.Context, *pb.ExecuteNotebookCellRequest) (*pb.ExecuteNotebookCellResponse, error)
	EnsureSession(context.Context, *pb.EnsureSessionRequest) (*pb.EnsureSessionResponse, error)
	DropSession(context.Context, *pb.DropSessionRequest) (*pb.DropSessionResponse, error)
}

func registerFakeOntologyActionsPythonRuntimeServiceServer(s grpc.ServiceRegistrar, srv fakeOntologyActionsPythonRuntimeService) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "runtime.PythonRuntimeService",
		HandlerType: (*fakeOntologyActionsPythonRuntimeService)(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: "ExecuteInlineFunction", Handler: fakeOntologyActionsUnaryHandler("/runtime.PythonRuntimeService/ExecuteInlineFunction", func() any { return &pb.ExecuteInlineFunctionRequest{} }, func(ctx context.Context, req any) (any, error) {
				return srv.ExecuteInlineFunction(ctx, req.(*pb.ExecuteInlineFunctionRequest))
			})},
			{MethodName: "ExecutePipelineTransform", Handler: fakeOntologyActionsUnaryHandler("/runtime.PythonRuntimeService/ExecutePipelineTransform", func() any { return &pb.ExecutePipelineTransformRequest{} }, func(ctx context.Context, req any) (any, error) {
				return srv.ExecutePipelineTransform(ctx, req.(*pb.ExecutePipelineTransformRequest))
			})},
			{MethodName: "ExecuteNotebookCell", Handler: fakeOntologyActionsUnaryHandler("/runtime.PythonRuntimeService/ExecuteNotebookCell", func() any { return &pb.ExecuteNotebookCellRequest{} }, func(ctx context.Context, req any) (any, error) {
				return srv.ExecuteNotebookCell(ctx, req.(*pb.ExecuteNotebookCellRequest))
			})},
			{MethodName: "EnsureSession", Handler: fakeOntologyActionsUnaryHandler("/runtime.PythonRuntimeService/EnsureSession", func() any { return &pb.EnsureSessionRequest{} }, func(ctx context.Context, req any) (any, error) {
				return srv.EnsureSession(ctx, req.(*pb.EnsureSessionRequest))
			})},
			{MethodName: "DropSession", Handler: fakeOntologyActionsUnaryHandler("/runtime.PythonRuntimeService/DropSession", func() any { return &pb.DropSessionRequest{} }, func(ctx context.Context, req any) (any, error) {
				return srv.DropSession(ctx, req.(*pb.DropSessionRequest))
			})},
		},
	}, srv)
}

func fakeOntologyActionsUnaryHandler(fullMethod string, newReq func() any, fn func(context.Context, any) (any, error)) grpc.MethodHandler {
	return func(_ any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		req := newReq()
		if err := dec(req); err != nil {
			return nil, err
		}
		if interceptor == nil {
			return fn(ctx, req)
		}
		info := &grpc.UnaryServerInfo{FullMethod: fullMethod}
		return interceptor(ctx, req, info, func(ctx context.Context, req any) (any, error) { return fn(ctx, req) })
	}
}

var _ ontologykernel.PythonInlineRuntime = pythonRuntimeAdapter{}
