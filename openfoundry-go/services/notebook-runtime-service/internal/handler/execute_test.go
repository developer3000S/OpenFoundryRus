package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	pythonsidecar "github.com/openfoundry/openfoundry-go/libs/python-sidecar"
	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/models"
)

type fakePythonKernel struct {
	results    map[string]*pythonsidecar.NotebookCellResult
	errors     map[string]error
	block      bool
	calls      []string
	ensureSeen []uuid.UUID
}

func (f *fakePythonKernel) EnsureSession(_ context.Context, sessionID uuid.UUID) error {
	f.ensureSeen = append(f.ensureSeen, sessionID)
	return nil
}

func (f *fakePythonKernel) ExecuteCell(ctx context.Context, _, _ uuid.UUID, source, _ string, _ uint32) (*pythonsidecar.NotebookCellResult, error) {
	f.calls = append(f.calls, source)
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if err := f.errors[source]; err != nil {
		return nil, err
	}
	if out := f.results[source]; out != nil {
		return out, nil
	}
	return &pythonsidecar.NotebookCellResult{OutputType: "text", ContentJSON: []byte(jsonString(""))}, nil
}

func (f *fakePythonKernel) DropSession(context.Context, uuid.UUID) error { return nil }

func executeTestState(t *testing.T, fk *fakePythonKernel) (*State, chi.Router, uuid.UUID) {
	t.Helper()
	nb := uuid.New()
	s := &State{Cfg: &config.Config{DataDir: t.TempDir()}, MemoryRepo: NewMemoryNotebookRepo(), PythonKernel: fk}
	r := chi.NewRouter()
	r.Post("/api/v1/notebooks/{notebook_id}/cells/{cell_id}/execute", s.ExecuteCell)
	r.Post("/api/v1/notebooks/{notebook_id}/cells/execute-all", s.ExecuteAllCells)
	return s, r, nb
}

func putTestCell(s *State, nb uuid.UUID, source string, position int32) models.Cell {
	id := uuid.New()
	now := time.Now().UTC()
	cell := models.Cell{ID: id, NotebookID: nb, CellType: "code", Kernel: "python", Source: source, Position: position, CreatedAt: now, UpdatedAt: now}
	s.MemoryRepo.putCell(cell)
	return cell
}

func TestExecuteCellSuccessfulPythonExecution(t *testing.T) {
	fk := &fakePythonKernel{results: map[string]*pythonsidecar.NotebookCellResult{
		"print('hi')": {OutputType: "text", ContentJSON: []byte(jsonString("hi\n")), Stdout: "hi\n"},
	}}
	s, r, nb := executeTestState(t, fk)
	cell := putTestCell(s, nb, "print('hi')", 1)

	w := httptest.NewRecorder()
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/"+nb.String()+"/cells/"+cell.ID.String()+"/execute", bytes.NewReader([]byte(`{}`))), uuid.New())
	req.ContentLength = 2
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	var got models.CellOutput
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got.OutputType != "text" || string(got.Content) != jsonString("hi\n") || got.ExecutionCount != 1 {
		t.Fatalf("output drift: %+v content=%s", got, got.Content)
	}
	persisted, _ := s.MemoryRepo.loadCell(cell.ID)
	if persisted.ExecutionCount == nil || *persisted.ExecutionCount != 1 || len(persisted.LastOutput) == 0 {
		t.Fatalf("output was not persisted: %+v", persisted)
	}
}

func TestExecuteCellPythonRequiresConfiguredSidecar(t *testing.T) {
	s, r, nb := executeTestState(t, nil)
	s.PythonKernel = nil
	cell := putTestCell(s, nb, "print('missing sidecar')", 1)

	w := httptest.NewRecorder()
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/"+nb.String()+"/cells/"+cell.ID.String()+"/execute", bytes.NewReader([]byte(`{}`))), uuid.New())
	req.ContentLength = 2
	r.ServeHTTP(w, req)

	var got models.CellOutput
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got.OutputType != "error" || !bytes.Contains(got.Content, []byte("python kernel sidecar is not configured")) {
		t.Fatalf("sidecar config error drift: %+v content=%s", got, got.Content)
	}
}

func TestExecuteCellStdoutStderrContract(t *testing.T) {
	fk := &fakePythonKernel{results: map[string]*pythonsidecar.NotebookCellResult{
		"logs": {OutputType: "text", ContentJSON: []byte(jsonString("out\n")), Stdout: "out\n", Stderr: "err\n"},
	}}
	s, r, nb := executeTestState(t, fk)
	cell := putTestCell(s, nb, "logs", 1)

	w := httptest.NewRecorder()
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/"+nb.String()+"/cells/"+cell.ID.String()+"/execute", bytes.NewReader([]byte(`{}`))), uuid.New())
	req.ContentLength = 2
	r.ServeHTTP(w, req)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("json: %v", err)
	}
	if string(raw["content"]) != jsonString("out\n") {
		t.Fatalf("stdout content drift: %s", raw["content"])
	}
	if _, ok := raw["stderr"]; ok {
		t.Fatalf("stderr must not leak into Rust CellOutput contract: %s", w.Body.String())
	}
}

func TestExecuteCellException(t *testing.T) {
	fk := &fakePythonKernel{errors: map[string]error{"boom": errors.New("Traceback: boom")}}
	s, r, nb := executeTestState(t, fk)
	cell := putTestCell(s, nb, "boom", 1)

	w := httptest.NewRecorder()
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/"+nb.String()+"/cells/"+cell.ID.String()+"/execute", bytes.NewReader([]byte(`{}`))), uuid.New())
	req.ContentLength = 2
	r.ServeHTTP(w, req)

	var got models.CellOutput
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.OutputType != "error" || !bytes.Contains(got.Content, []byte("Traceback: boom")) {
		t.Fatalf("error output drift: %+v content=%s", got, got.Content)
	}
}

func TestExecuteCellCancellation(t *testing.T) {
	fk := &fakePythonKernel{block: true}
	s, r, nb := executeTestState(t, fk)
	cell := putTestCell(s, nb, "wait", 1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/"+nb.String()+"/cells/"+cell.ID.String()+"/execute", bytes.NewReader([]byte(`{}`))).WithContext(ctx), uuid.New())
	req.ContentLength = 2
	r.ServeHTTP(w, req)

	var got models.CellOutput
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.OutputType != "error" || !bytes.Contains(got.Content, []byte("context canceled")) {
		t.Fatalf("cancel output drift: %+v content=%s", got, got.Content)
	}
}

func TestExecuteAllCellsOrdering(t *testing.T) {
	fk := &fakePythonKernel{results: map[string]*pythonsidecar.NotebookCellResult{
		"first":  {OutputType: "text", ContentJSON: []byte(jsonString("1"))},
		"second": {OutputType: "text", ContentJSON: []byte(jsonString("2"))},
	}}
	s, r, nb := executeTestState(t, fk)
	second := putTestCell(s, nb, "second", 2)
	first := putTestCell(s, nb, "first", 1)

	w := httptest.NewRecorder()
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/"+nb.String()+"/cells/execute-all", bytes.NewReader([]byte(`{}`))), uuid.New())
	req.ContentLength = 2
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	if len(fk.calls) != 2 || fk.calls[0] != "first" || fk.calls[1] != "second" {
		t.Fatalf("call order = %v", fk.calls)
	}
	var env struct {
		Results []struct {
			CellID uuid.UUID         `json:"cell_id"`
			Output models.CellOutput `json:"output"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(env.Results) != 2 || env.Results[0].CellID != first.ID || env.Results[1].CellID != second.ID {
		t.Fatalf("result order drift: %+v", env.Results)
	}
}

func TestExecuteAllCellsPythonRequiresConfiguredSidecar(t *testing.T) {
	s, r, nb := executeTestState(t, nil)
	s.PythonKernel = nil
	cell := putTestCell(s, nb, "print('missing sidecar')", 1)

	w := httptest.NewRecorder()
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/"+nb.String()+"/cells/execute-all", bytes.NewReader([]byte(`{}`))), uuid.New())
	req.ContentLength = 2
	r.ServeHTTP(w, req)

	var env struct {
		Results []struct {
			CellID uuid.UUID         `json:"cell_id"`
			Output models.CellOutput `json:"output"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(env.Results) != 1 || env.Results[0].CellID != cell.ID || env.Results[0].Output.OutputType != "error" || !bytes.Contains(env.Results[0].Output.Content, []byte("python kernel sidecar is not configured")) {
		t.Fatalf("execute-all sidecar config error drift: %+v", env.Results)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

type fakeSQLKernel struct {
	results map[string]*pythonsidecar.NotebookCellResult
	errors  map[string]error
	calls   []string
}

func (f *fakeSQLKernel) ExecuteSQL(_ context.Context, _ *authmw.Claims, source string) (*pythonsidecar.NotebookCellResult, error) {
	f.calls = append(f.calls, source)
	if err := f.errors[source]; err != nil {
		return nil, err
	}
	if out := f.results[source]; out != nil {
		return out, nil
	}
	return &pythonsidecar.NotebookCellResult{OutputType: "table", ContentJSON: []byte(`{"columns":[],"rows":[],"total_rows":0,"execution_time_ms":0}`)}, nil
}

type fakeRKernel struct {
	results map[string]*pythonsidecar.NotebookCellResult
	errors  map[string]error
	calls   []string
}

func (f *fakeRKernel) ExecuteR(_ context.Context, source, _ string) (*pythonsidecar.NotebookCellResult, error) {
	f.calls = append(f.calls, source)
	if err := f.errors[source]; err != nil {
		return nil, err
	}
	if out := f.results[source]; out != nil {
		return out, nil
	}
	return &pythonsidecar.NotebookCellResult{OutputType: "text", ContentJSON: []byte(jsonString(""))}, nil
}

type fakeLLMKernel struct {
	results map[string]*pythonsidecar.NotebookCellResult
	errors  map[string]error
	calls   []string
}

func (f *fakeLLMKernel) ExecuteLLM(_ context.Context, _ *uuid.UUID, _ uuid.UUID, source, _ string, _ []models.NotebookWorkspaceFile, _ *authmw.Claims) (*pythonsidecar.NotebookCellResult, error) {
	f.calls = append(f.calls, source)
	if err := f.errors[source]; err != nil {
		return nil, err
	}
	if out := f.results[source]; out != nil {
		return out, nil
	}
	return &pythonsidecar.NotebookCellResult{OutputType: "llm", ContentJSON: []byte(`{"reply":""}`)}, nil
}

func (f *fakeLLMKernel) DropSession(context.Context, uuid.UUID) error { return nil }

func putTestKernelCell(s *State, nb uuid.UUID, kernel, source string, position int32) models.Cell {
	id := uuid.New()
	now := time.Now().UTC()
	cell := models.Cell{ID: id, NotebookID: nb, CellType: "code", Kernel: kernel, Source: source, Position: position, CreatedAt: now, UpdatedAt: now}
	s.MemoryRepo.putCell(cell)
	return cell
}

func TestHTTPSQLKernelHappyAndFailure(t *testing.T) {
	t.Parallel()
	var seenAuth string
	var seenBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/queries/execute" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		seenAuth = r.Header.Get("authorization")
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("request json: %v", err)
		}
		if seenBody["sql"] == "bad" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"syntax error"}`))
			return
		}
		_, _ = w.Write([]byte(`{"columns":[{"name":"answer","data_type":"int"}],"rows":[["42"]],"total_rows":1,"execution_time_ms":5}`))
	}))
	defer srv.Close()

	k := &httpSQLKernel{Client: srv.Client(), QueryServiceURL: srv.URL + "/", JWTConfig: authmw.NewJWTConfig("test-secret")}
	out, err := k.ExecuteSQL(context.Background(), &authmw.Claims{Sub: uuid.New()}, "select 42")
	if err != nil {
		t.Fatalf("ExecuteSQL: %v", err)
	}
	if out.OutputType != "table" || !bytes.Contains(out.ContentJSON, []byte(`"answer"`)) || !strings.HasPrefix(seenAuth, "Bearer ") {
		t.Fatalf("SQL output/request drift: out=%+v auth=%q", out, seenAuth)
	}
	if seenBody["limit"] != float64(1000) {
		t.Fatalf("limit = %#v", seenBody["limit"])
	}
	_, err = k.ExecuteSQL(context.Background(), &authmw.Claims{Sub: uuid.New()}, "bad")
	if err == nil || !strings.Contains(err.Error(), "syntax error") {
		t.Fatalf("expected syntax error, got %v", err)
	}
}

func TestExecuteCellRHappyAndFailure(t *testing.T) {
	t.Parallel()
	rKernel := &fakeRKernel{results: map[string]*pythonsidecar.NotebookCellResult{
		"print(1)": {OutputType: "text", ContentJSON: []byte(jsonString("[1] 1\n"))},
	}, errors: map[string]error{"stop('boom')": errors.New("R boom")}}
	s, r, nb := executeTestState(t, &fakePythonKernel{})
	s.RKernel = rKernel
	okCell := putTestKernelCell(s, nb, "r", "print(1)", 1)
	badCell := putTestKernelCell(s, nb, "r", "stop('boom')", 2)

	w := httptest.NewRecorder()
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/"+nb.String()+"/cells/"+okCell.ID.String()+"/execute", bytes.NewReader([]byte(`{}`))), uuid.New())
	req.ContentLength = 2
	r.ServeHTTP(w, req)
	var okOut models.CellOutput
	_ = json.Unmarshal(w.Body.Bytes(), &okOut)
	if okOut.OutputType != "text" || string(okOut.Content) != jsonString("[1] 1\n") {
		t.Fatalf("R success drift: %+v content=%s", okOut, okOut.Content)
	}

	w = httptest.NewRecorder()
	req = withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/"+nb.String()+"/cells/"+badCell.ID.String()+"/execute", bytes.NewReader([]byte(`{}`))), uuid.New())
	req.ContentLength = 2
	r.ServeHTTP(w, req)
	var errOut models.CellOutput
	_ = json.Unmarshal(w.Body.Bytes(), &errOut)
	if errOut.OutputType != "error" || !bytes.Contains(errOut.Content, []byte("R boom")) {
		t.Fatalf("R failure drift: %+v content=%s", errOut, errOut.Content)
	}
}

func TestExecuteCellLLMHappyWithFakeRuntime(t *testing.T) {
	t.Parallel()
	llm := &fakeLLMKernel{results: map[string]*pythonsidecar.NotebookCellResult{
		"explain": {OutputType: "llm", ContentJSON: []byte(`{"reply":"ok","provider_name":"fake"}`)},
	}}
	s, r, nb := executeTestState(t, &fakePythonKernel{})
	s.LLMKernel = llm
	cell := putTestKernelCell(s, nb, "llm", "explain", 1)

	w := httptest.NewRecorder()
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/"+nb.String()+"/cells/"+cell.ID.String()+"/execute", bytes.NewReader([]byte(`{}`))), uuid.New())
	req.ContentLength = 2
	r.ServeHTTP(w, req)
	var out models.CellOutput
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.OutputType != "llm" || !bytes.Contains(out.Content, []byte(`"reply":"ok"`)) || len(llm.calls) != 1 {
		t.Fatalf("LLM output drift: %+v content=%s calls=%v", out, out.Content, llm.calls)
	}
}

func TestExecuteAllCellsMixedKernels(t *testing.T) {
	t.Parallel()
	fk := &fakePythonKernel{results: map[string]*pythonsidecar.NotebookCellResult{"py": {OutputType: "text", ContentJSON: []byte(jsonString("py"))}}}
	s, r, nb := executeTestState(t, fk)
	s.SQLKernel = &fakeSQLKernel{results: map[string]*pythonsidecar.NotebookCellResult{"sql": {OutputType: "table", ContentJSON: []byte(`{"rows":[["sql"]]}`)}}}
	s.RKernel = &fakeRKernel{results: map[string]*pythonsidecar.NotebookCellResult{"r": {OutputType: "text", ContentJSON: []byte(jsonString("r"))}}}
	s.LLMKernel = &fakeLLMKernel{results: map[string]*pythonsidecar.NotebookCellResult{"llm": {OutputType: "llm", ContentJSON: []byte(`{"reply":"llm"}`)}}}
	pyCell := putTestKernelCell(s, nb, "python", "py", 1)
	sqlCell := putTestKernelCell(s, nb, "sql", "sql", 2)
	rCell := putTestKernelCell(s, nb, "r", "r", 3)
	llmCell := putTestKernelCell(s, nb, "llm", "llm", 4)

	w := httptest.NewRecorder()
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/"+nb.String()+"/cells/execute-all", bytes.NewReader([]byte(`{}`))), uuid.New())
	req.ContentLength = 2
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Results []struct {
			CellID uuid.UUID         `json:"cell_id"`
			Output models.CellOutput `json:"output"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("json: %v", err)
	}
	wantIDs := []uuid.UUID{pyCell.ID, sqlCell.ID, rCell.ID, llmCell.ID}
	wantTypes := []string{"text", "table", "text", "llm"}
	if len(env.Results) != len(wantIDs) {
		t.Fatalf("result len=%d body=%s", len(env.Results), w.Body.String())
	}
	for i := range wantIDs {
		if env.Results[i].CellID != wantIDs[i] || env.Results[i].Output.OutputType != wantTypes[i] {
			t.Fatalf("mixed result[%d] drift: %+v", i, env.Results[i])
		}
	}
}

func TestBuildRScriptRustCompatibleWrapper(t *testing.T) {
	t.Parallel()
	got := buildRScript("print(getwd())", `/tmp/work'space`)
	want := "workspace_dir <- '/tmp/work\\'space'\nif (nzchar(workspace_dir)) { setwd(workspace_dir) }\nprint(getwd())\n"
	if got != want {
		t.Fatalf("R wrapper drift:\n got: %q\nwant: %q", got, want)
	}
}
