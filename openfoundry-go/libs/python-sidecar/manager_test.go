package pythonsidecar

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	pb "github.com/openfoundry/openfoundry-go/libs/proto-gen/runtime"
)

// TestSidecarEndToEnd starts a real openfoundry-pyruntime, drives the
// three RPCs, and shuts down. Skipped when the binary path is not set.
//
// Set PYTHON_SIDECAR_BINARY to the absolute path of openfoundry-pyruntime
// (e.g. the script installed in your dev venv) to enable.
func TestSidecarEndToEnd(t *testing.T) {
	bin := os.Getenv("PYTHON_SIDECAR_BINARY")
	if bin == "" {
		t.Skip("PYTHON_SIDECAR_BINARY not set — install openfoundry-pyruntime in a venv and re-run")
	}

	mgr, err := New(Config{
		BinaryPath:      bin,
		StartupTimeout:  5 * time.Second,
		HardCallTimeout: 10 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop(context.Background()) })

	t.Run("inline", func(t *testing.T) {
		out, err := mgr.ExecuteInline(ctx, `import sys`+"\n"+`print("hi")`+"\n"+`print("err", file=sys.stderr)`+"\n"+`result = {"k": 1}`, []byte("{}"), 0)
		if err != nil {
			t.Fatalf("ExecuteInline: %v", err)
		}
		if out.Stdout != "hi\n" {
			t.Fatalf("stdout = %q, want %q", out.Stdout, "hi\n")
		}
		if out.Stderr != "err\n" {
			t.Fatalf("stderr = %q, want %q", out.Stderr, "err\n")
		}
		var payload map[string]any
		if err := json.Unmarshal(out.ResultJSON, &payload); err != nil {
			t.Fatalf("ResultJSON not JSON: %v", err)
		}
		result, ok := payload["result"].(map[string]any)
		if !ok || result["k"] != float64(1) {
			t.Fatalf("unexpected result payload: %v", payload)
		}
	})

	t.Run("inline error", func(t *testing.T) {
		_, err := mgr.ExecuteInline(ctx, `raise RuntimeError("inline boom")`, []byte("{}"), 0)
		if err == nil || !strings.Contains(err.Error(), "inline boom") {
			t.Fatalf("expected inline error to contain inline boom, got %v", err)
		}
	})

	t.Run("pipeline", func(t *testing.T) {
		src := "rows_affected = 3\nresult_rows = [{\"a\": 1}, {\"a\": 2}, {\"a\": 3}]\nprint(\"rows\")"
		out, err := mgr.ExecutePipeline(ctx, src, []byte("{}"), []byte("[]"), nil, "", 0)
		if err != nil {
			t.Fatalf("ExecutePipeline: %v", err)
		}
		if out.Stdout != "rows\n" {
			t.Fatalf("pipeline stdout = %q, want %q", out.Stdout, "rows\n")
		}
		if !out.RowsAffectedSet || out.RowsAffected != 3 {
			t.Fatalf("rows_affected = %d (set=%v), want 3", out.RowsAffected, out.RowsAffectedSet)
		}
		var rows []map[string]any
		if err := json.Unmarshal(out.ResultRowsJSON, &rows); err != nil {
			t.Fatalf("result rows not JSON: %v", err)
		}
		if len(rows) != 3 {
			t.Fatalf("rows len = %d, want 3", len(rows))
		}
	})

	t.Run("pipeline error", func(t *testing.T) {
		_, err := mgr.ExecutePipeline(ctx, `raise RuntimeError("pipeline boom")`, []byte("{}"), []byte("[]"), nil, "", 0)
		if err == nil || !strings.Contains(err.Error(), "pipeline boom") {
			t.Fatalf("expected pipeline error to contain pipeline boom, got %v", err)
		}
	})

	t.Run("notebook session persists", func(t *testing.T) {
		sid := uuid.New()
		nbid := uuid.New()
		first, err := mgr.ExecuteNotebookCell(ctx, sid, nbid, "x = 21\nprint(x)", "", 0)
		if err != nil {
			t.Fatalf("first cell: %v", err)
		}
		if first.Stdout != "21\n" {
			t.Fatalf("first stdout = %q", first.Stdout)
		}
		second, err := mgr.ExecuteNotebookCell(ctx, sid, nbid, "print(x*2)", "", 0)
		if err != nil {
			t.Fatalf("second cell: %v", err)
		}
		if second.Stdout != "42\n" {
			t.Fatalf("second stdout = %q (session not persisted?)", second.Stdout)
		}
		if err := mgr.DropSession(ctx, sid); err != nil {
			t.Fatalf("DropSession: %v", err)
		}
	})

	t.Run("notebook error", func(t *testing.T) {
		_, err := mgr.ExecuteNotebookCell(ctx, uuid.New(), uuid.New(), `raise RuntimeError("notebook boom")`, "", 0)
		if err == nil || !strings.Contains(err.Error(), "notebook boom") {
			t.Fatalf("expected notebook error to contain notebook boom, got %v", err)
		}
	})
}

func TestSidecarBinaryUnsetIsSkipped(t *testing.T) {
	t.Setenv("PYTHON_SIDECAR_BINARY", "")
	if os.Getenv("PYTHON_SIDECAR_BINARY") != "" {
		t.Fatal("PYTHON_SIDECAR_BINARY should be unset in this test")
	}
	_, err := New(Config{BinaryPath: ""}, nil)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "binarypath") || !strings.Contains(strings.ToLower(err.Error()), "required") {
		t.Fatalf("expected missing binary path validation, got %v", err)
	}
}

func TestSidecarEndToEndWithFakeBinary(t *testing.T) {
	bin := fakeSidecarBinary(t)
	t.Setenv("PYTHON_SIDECAR_BINARY", bin)
	mgr, err := New(Config{BinaryPath: os.Getenv("PYTHON_SIDECAR_BINARY"), StartupTimeout: 2 * time.Second, HardCallTimeout: 2 * time.Second}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start fake sidecar: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop(context.Background()) })

	inline, err := mgr.ExecuteInline(ctx, "anything", []byte(`{"k":1}`), 1)
	if err != nil {
		t.Fatalf("ExecuteInline fake: %v", err)
	}
	if string(inline.ResultJSON) != `{"fake":true}` || inline.Stdout != "fake inline\n" || inline.Stderr != "fake stderr\n" {
		t.Fatalf("inline fake drift: %+v", inline)
	}

	cell, err := mgr.ExecuteNotebookCell(ctx, uuid.New(), uuid.New(), "print(42)", t.TempDir(), 1)
	if err != nil {
		t.Fatalf("ExecuteNotebookCell fake: %v", err)
	}
	if cell.OutputType != "text" || string(cell.ContentJSON) != `"fake notebook"` || cell.Stdout != "fake notebook\n" {
		t.Fatalf("notebook fake drift: %+v", cell)
	}

	if err := mgr.EnsureSession(ctx, uuid.New()); err != nil {
		t.Fatalf("EnsureSession fake: %v", err)
	}
	if err := mgr.DropSession(ctx, uuid.New()); err != nil {
		t.Fatalf("DropSession fake: %v", err)
	}
}

func fakeSidecarBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-sidecar.sh")
	script := "#!/bin/sh\nGO_WANT_HELPER_PROCESS=1 exec \"" + os.Args[0] + "\" -test.run=TestFakePythonSidecarProcess -- \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sidecar wrapper: %v", err)
	}
	return path
}

func TestFakePythonSidecarProcess(t *testing.T) {
	if len(os.Args) == 0 || os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") == "1" {
		runFakePythonSidecarProcess()
		return
	}
	os.Exit(m.Run())
}

func runFakePythonSidecarProcess() {
	bind := ""
	for i, arg := range os.Args {
		if arg == "--bind" && i+1 < len(os.Args) {
			bind = os.Args[i+1]
			break
		}
	}
	if bind == "" {
		os.Exit(2)
	}
	const unixPrefix = "unix:"
	if !strings.HasPrefix(bind, unixPrefix) {
		os.Exit(2)
	}
	socket := strings.TrimPrefix(bind, unixPrefix)
	_ = os.Remove(socket)
	lis, err := net.Listen("unix", socket)
	if err != nil {
		os.Exit(2)
	}
	grpcServer := grpc.NewServer()
	registerFakePythonRuntimeServiceServer(grpcServer, fakePythonRuntimeServer{})
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	if err := grpcServer.Serve(lis); err != nil {
		os.Exit(2)
	}
}

type fakePythonRuntimeServer struct{}

func (fakePythonRuntimeServer) ExecuteInlineFunction(context.Context, *pb.ExecuteInlineFunctionRequest) (*pb.ExecuteInlineFunctionResponse, error) {
	return &pb.ExecuteInlineFunctionResponse{ResultJson: `{"fake":true}`, Stdout: "fake inline\n", Stderr: "fake stderr\n"}, nil
}

func (fakePythonRuntimeServer) ExecutePipelineTransform(context.Context, *pb.ExecutePipelineTransformRequest) (*pb.ExecutePipelineTransformResponse, error) {
	return &pb.ExecutePipelineTransformResponse{OutputJson: `{"ok":true}`, ResultRowsJson: `[]`, RowsAffected: 1, RowsAffectedSet: true, Stdout: "fake pipeline\n"}, nil
}

func (fakePythonRuntimeServer) ExecuteNotebookCell(context.Context, *pb.ExecuteNotebookCellRequest) (*pb.ExecuteNotebookCellResponse, error) {
	return &pb.ExecuteNotebookCellResponse{OutputType: "text", ContentJson: `"fake notebook"`, Stdout: "fake notebook\n"}, nil
}

func (fakePythonRuntimeServer) EnsureSession(context.Context, *pb.EnsureSessionRequest) (*pb.EnsureSessionResponse, error) {
	return &pb.EnsureSessionResponse{}, nil
}

func (fakePythonRuntimeServer) DropSession(context.Context, *pb.DropSessionRequest) (*pb.DropSessionResponse, error) {
	return &pb.DropSessionResponse{}, nil
}

type fakePythonRuntimeService interface {
	ExecuteInlineFunction(context.Context, *pb.ExecuteInlineFunctionRequest) (*pb.ExecuteInlineFunctionResponse, error)
	ExecutePipelineTransform(context.Context, *pb.ExecutePipelineTransformRequest) (*pb.ExecutePipelineTransformResponse, error)
	ExecuteNotebookCell(context.Context, *pb.ExecuteNotebookCellRequest) (*pb.ExecuteNotebookCellResponse, error)
	EnsureSession(context.Context, *pb.EnsureSessionRequest) (*pb.EnsureSessionResponse, error)
	DropSession(context.Context, *pb.DropSessionRequest) (*pb.DropSessionResponse, error)
}

func registerFakePythonRuntimeServiceServer(s grpc.ServiceRegistrar, srv fakePythonRuntimeService) {
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "runtime.PythonRuntimeService",
		HandlerType: (*fakePythonRuntimeService)(nil),
		Methods: []grpc.MethodDesc{
			{MethodName: "ExecuteInlineFunction", Handler: fakeUnaryHandler("/runtime.PythonRuntimeService/ExecuteInlineFunction", func() any { return &pb.ExecuteInlineFunctionRequest{} }, func(ctx context.Context, req any) (any, error) {
				return srv.ExecuteInlineFunction(ctx, req.(*pb.ExecuteInlineFunctionRequest))
			})},
			{MethodName: "ExecutePipelineTransform", Handler: fakeUnaryHandler("/runtime.PythonRuntimeService/ExecutePipelineTransform", func() any { return &pb.ExecutePipelineTransformRequest{} }, func(ctx context.Context, req any) (any, error) {
				return srv.ExecutePipelineTransform(ctx, req.(*pb.ExecutePipelineTransformRequest))
			})},
			{MethodName: "ExecuteNotebookCell", Handler: fakeUnaryHandler("/runtime.PythonRuntimeService/ExecuteNotebookCell", func() any { return &pb.ExecuteNotebookCellRequest{} }, func(ctx context.Context, req any) (any, error) {
				return srv.ExecuteNotebookCell(ctx, req.(*pb.ExecuteNotebookCellRequest))
			})},
			{MethodName: "EnsureSession", Handler: fakeUnaryHandler("/runtime.PythonRuntimeService/EnsureSession", func() any { return &pb.EnsureSessionRequest{} }, func(ctx context.Context, req any) (any, error) {
				return srv.EnsureSession(ctx, req.(*pb.EnsureSessionRequest))
			})},
			{MethodName: "DropSession", Handler: fakeUnaryHandler("/runtime.PythonRuntimeService/DropSession", func() any { return &pb.DropSessionRequest{} }, func(ctx context.Context, req any) (any, error) {
				return srv.DropSession(ctx, req.(*pb.DropSessionRequest))
			})},
		},
	}, srv)
}

func fakeUnaryHandler(fullMethod string, newReq func() any, fn func(context.Context, any) (any, error)) grpc.MethodHandler {
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
