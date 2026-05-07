// Tests for the notebook + cell + session CRUD endpoints. The
// no-DB path is the fallback shape every smoke cluster + the unit
// tests use; the real DB path is exercised by integration tests
// against the migrations (out of scope here).
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/models"
)

func newState() *State {
	return &State{Cfg: &config.Config{SmokeMode: true}, Pool: nil, MemoryRepo: NewMemoryNotebookRepo()}
}

// mountTestRouter wires the same chi tree the server uses but stops
// before the auth middleware so tests can inject claims directly via
// authmw.WithContext.
func mountTestRouter(s *State) chi.Router {
	r := chi.NewRouter()
	r.Get("/api/v1/notebooks", s.ListNotebooks)
	r.Post("/api/v1/notebooks", s.CreateNotebook)
	r.Get("/api/v1/notebooks/{notebook_id}", s.GetNotebook)
	r.Put("/api/v1/notebooks/{notebook_id}", s.UpdateNotebook)
	r.Patch("/api/v1/notebooks/{notebook_id}", s.UpdateNotebook)
	r.Delete("/api/v1/notebooks/{notebook_id}", s.DeleteNotebook)
	r.Post("/api/v1/notebooks/{notebook_id}/cells", s.AddCell)
	r.Patch("/api/v1/notebooks/{notebook_id}/cells/{cell_id}", s.UpdateCell)
	r.Delete("/api/v1/notebooks/{notebook_id}/cells/{cell_id}", s.DeleteCell)
	r.Get("/api/v1/notebooks/{notebook_id}/sessions", s.ListSessions)
	r.Post("/api/v1/notebooks/{notebook_id}/sessions", s.CreateSession)
	r.Post("/api/v1/notebooks/{notebook_id}/sessions/{session_id}/stop", s.StopSession)
	return r
}

func withClaims(req *http.Request, sub uuid.UUID) *http.Request {
	ctx := authmw.ContextWithClaims(req.Context(), &authmw.Claims{Sub: sub})
	return req.WithContext(ctx)
}

// ── Notebook CRUD (no-DB path) ──────────────────────────────────────

func TestCreateNotebookNoDBSynthesisesRow(t *testing.T) {
	t.Parallel()
	r := mountTestRouter(newState())
	body, _ := json.Marshal(models.CreateNotebookRequest{Name: "demo"})
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks", bytes.NewReader(body)), uuid.New())
	req.ContentLength = int64(len(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	var got models.Notebook
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got.Name != "demo" || got.DefaultKernel != "python" {
		t.Errorf("body drift: %+v", got)
	}
}

func TestCreateNotebookRejectsWithoutClaims(t *testing.T) {
	t.Parallel()
	r := mountTestRouter(newState())
	body, _ := json.Marshal(models.CreateNotebookRequest{Name: "demo"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notebooks", bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: %d", w.Code)
	}
}

func TestListNotebooksSmokeModeReturnsPersistedEnvelope(t *testing.T) {
	t.Parallel()
	r := mountTestRouter(newState())
	createBody, _ := json.Marshal(models.CreateNotebookRequest{Name: "smoke-listed"})
	createReq := withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks", bytes.NewReader(createBody)), uuid.New())
	createReq.ContentLength = int64(len(createBody))
	r.ServeHTTP(httptest.NewRecorder(), createReq)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/notebooks?page=2&per_page=5", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d", w.Code)
	}
	var env map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	if env["page"].(float64) != 2 || env["per_page"].(float64) != 5 || env["total"].(float64) != 1 {
		t.Errorf("pagination drift: %+v", env)
	}
}

func TestListNotebooksRequiresDatabaseOutsideSmokeMode(t *testing.T) {
	t.Parallel()
	r := mountTestRouter(&State{Cfg: &config.Config{}, Pool: nil})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/notebooks", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListNotebooksUsesConfiguredRepository(t *testing.T) {
	t.Parallel()
	now := fixedTime()
	repo := &fakeNotebookListRepo{
		notebooks: []models.Notebook{{ID: uuid.New(), Name: "db-listed", DefaultKernel: "python", CreatedAt: now, UpdatedAt: now}},
		total:     7,
	}
	r := mountTestRouter(&State{Cfg: &config.Config{}, ListRepo: repo})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/notebooks?search=db&page=3&per_page=2", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if repo.notebookParams.Search != "db" || repo.notebookParams.Page != 3 || repo.notebookParams.PerPage != 2 {
		t.Fatalf("repo params drift: %+v", repo.notebookParams)
	}
	var env struct {
		Data    []models.Notebook `json:"data"`
		Total   int64             `json:"total"`
		Page    int64             `json:"page"`
		PerPage int64             `json:"per_page"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("json: %v", err)
	}
	if env.Total != 7 || len(env.Data) != 1 || env.Data[0].Name != "db-listed" {
		t.Fatalf("repository envelope drift: %+v", env)
	}
}

func TestListNotebooksRepositoryErrorReturns500(t *testing.T) {
	t.Parallel()
	r := mountTestRouter(&State{Cfg: &config.Config{}, ListRepo: &fakeNotebookListRepo{err: errors.New("db down")}})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/notebooks", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListNotebooksClampsPerPage(t *testing.T) {
	t.Parallel()
	r := mountTestRouter(newState())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/notebooks?per_page=999", nil))
	var env map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	if env["per_page"].(float64) != 100 {
		t.Errorf("per_page must clamp to 100, got %v", env["per_page"])
	}
}

func TestGetNotebookNoDBReturns404(t *testing.T) {
	t.Parallel()
	r := mountTestRouter(newState())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/notebooks/"+uuid.New().String(), nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestUpdateNotebookRoutedFromBothPatchAndPut(t *testing.T) {
	t.Parallel()
	r := mountTestRouter(newState())
	id := uuid.New().String()
	for _, method := range []string{http.MethodPatch, http.MethodPut} {
		body, _ := json.Marshal(models.UpdateNotebookRequest{})
		req := httptest.NewRequest(method, "/api/v1/notebooks/"+id, bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: expected 404 (no DB), got %d", method, w.Code)
		}
	}
}

func TestDeleteNotebookNoDBReturns404(t *testing.T) {
	t.Parallel()
	r := mountTestRouter(newState())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/notebooks/"+uuid.New().String(), nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status: %d", w.Code)
	}
}

func TestNotebookCellSessionSmokeCRUDRoundTrip(t *testing.T) {
	t.Parallel()
	owner := uuid.New()
	r := mountTestRouter(newState())

	nbBody := []byte(`{"name":"Roundtrip","description":"d","default_kernel":"python"}`)
	w := httptest.NewRecorder()
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks", bytes.NewReader(nbBody)), owner)
	req.ContentLength = int64(len(nbBody))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create notebook status=%d body=%s", w.Code, w.Body.String())
	}
	var nb models.Notebook
	_ = json.Unmarshal(w.Body.Bytes(), &nb)

	newName := "Renamed"
	patchBody, _ := json.Marshal(models.UpdateNotebookRequest{Name: &newName})
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/notebooks/"+nb.ID.String(), bytes.NewReader(patchBody))
	req.ContentLength = int64(len(patchBody))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update notebook status=%d body=%s", w.Code, w.Body.String())
	}

	source := "print(42)"
	cellBody, _ := json.Marshal(models.CreateCellRequest{Source: &source})
	w = httptest.NewRecorder()
	req = withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/"+nb.ID.String()+"/cells", bytes.NewReader(cellBody)), owner)
	req.ContentLength = int64(len(cellBody))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("add cell status=%d body=%s", w.Code, w.Body.String())
	}
	var cell models.Cell
	_ = json.Unmarshal(w.Body.Bytes(), &cell)

	updatedSource := "print(43)"
	cellPatch, _ := json.Marshal(models.UpdateCellRequest{Source: &updatedSource})
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/notebooks/"+nb.ID.String()+"/cells/"+cell.ID.String(), bytes.NewReader(cellPatch))
	req.ContentLength = int64(len(cellPatch))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update cell status=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/notebooks/"+nb.ID.String(), nil))
	if w.Code != http.StatusOK || bytes.Contains(w.Body.Bytes(), []byte(`"data":[]`)) {
		t.Fatalf("get notebook should return notebook+cells, status=%d body=%s", w.Code, w.Body.String())
	}
	var got struct {
		Notebook models.Notebook `json:"notebook"`
		Cells    []models.Cell   `json:"cells"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Notebook.Name != "Renamed" || len(got.Cells) != 1 || got.Cells[0].Source != updatedSource {
		t.Fatalf("get notebook drift: %+v", got)
	}

	sessionBody := []byte(`{}`)
	w = httptest.NewRecorder()
	req = withClaims(httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/"+nb.ID.String()+"/sessions", bytes.NewReader(sessionBody)), owner)
	req.ContentLength = int64(len(sessionBody))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create session status=%d body=%s", w.Code, w.Body.String())
	}
	var sess models.Session
	_ = json.Unmarshal(w.Body.Bytes(), &sess)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/notebooks/"+nb.ID.String()+"/sessions", nil))
	var sessions struct {
		Data []models.Session `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &sessions)
	if w.Code != http.StatusOK || len(sessions.Data) != 1 || sessions.Data[0].ID != sess.ID {
		t.Fatalf("list sessions drift status=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/notebooks/"+nb.ID.String()+"/sessions/"+sess.ID.String()+"/stop", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("stop session status=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/notebooks/"+nb.ID.String()+"/cells/"+cell.ID.String(), nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete cell status=%d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/v1/notebooks/"+nb.ID.String(), nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete notebook status=%d body=%s", w.Code, w.Body.String())
	}
}

// ── Cell CRUD ───────────────────────────────────────────────────────

func TestAddCellNoDBDefaultsKernelAndType(t *testing.T) {
	t.Parallel()
	r := mountTestRouter(newState())
	body, _ := json.Marshal(models.CreateCellRequest{})
	req := withClaims(httptest.NewRequest(http.MethodPost,
		"/api/v1/notebooks/"+uuid.New().String()+"/cells", bytes.NewReader(body)), uuid.New())
	req.ContentLength = int64(len(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	var got models.Cell
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.CellType != "code" || got.Kernel != "python" {
		t.Errorf("defaults drift: %+v", got)
	}
}

func TestUpdateCellNoDBReturns404(t *testing.T) {
	t.Parallel()
	r := mountTestRouter(newState())
	body, _ := json.Marshal(models.UpdateCellRequest{})
	req := httptest.NewRequest(http.MethodPatch,
		"/api/v1/notebooks/"+uuid.New().String()+"/cells/"+uuid.New().String(),
		bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status: %d", w.Code)
	}
}

// ── Sessions ────────────────────────────────────────────────────────

func TestCreateSessionNoDBReturnsIdleRow(t *testing.T) {
	t.Parallel()
	r := mountTestRouter(newState())
	notebookID := uuid.New()
	body, _ := json.Marshal(models.CreateSessionRequest{})
	req := withClaims(httptest.NewRequest(http.MethodPost,
		"/api/v1/notebooks/"+notebookID.String()+"/sessions", bytes.NewReader(body)), uuid.New())
	req.ContentLength = int64(len(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	var got models.Session
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Status != "idle" || got.Kernel != "python" {
		t.Errorf("session drift: %+v", got)
	}
	if got.NotebookID != notebookID {
		t.Errorf("notebook id drift: %s vs %s", got.NotebookID, notebookID)
	}
}

func TestListSessionsNoDB(t *testing.T) {
	t.Parallel()
	r := mountTestRouter(newState())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet,
		"/api/v1/notebooks/"+uuid.New().String()+"/sessions", nil))
	if w.Code != http.StatusOK {
		t.Errorf("status: %d", w.Code)
	}
}

func TestListSessionsRequiresDatabaseOutsideSmokeMode(t *testing.T) {
	t.Parallel()
	r := mountTestRouter(&State{Cfg: &config.Config{}, Pool: nil})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet,
		"/api/v1/notebooks/"+uuid.New().String()+"/sessions", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestListSessionsUsesConfiguredRepository(t *testing.T) {
	t.Parallel()
	notebookID := uuid.New()
	sess := models.Session{ID: uuid.New(), NotebookID: notebookID, Kernel: "python", Status: "idle", StartedBy: uuid.New(), CreatedAt: fixedTime(), LastActivity: fixedTime()}
	repo := &fakeNotebookListRepo{sessions: []models.Session{sess}}
	r := mountTestRouter(&State{Cfg: &config.Config{}, ListRepo: repo})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet,
		"/api/v1/notebooks/"+notebookID.String()+"/sessions", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if repo.sessionNotebookID != notebookID {
		t.Fatalf("repo notebook id drift: %s", repo.sessionNotebookID)
	}
	var env struct {
		Data []models.Session `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(env.Data) != 1 || env.Data[0].ID != sess.ID {
		t.Fatalf("repository session envelope drift: %+v", env)
	}
}

func TestStopSessionNoDBReturns404(t *testing.T) {
	t.Parallel()
	r := mountTestRouter(newState())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost,
		"/api/v1/notebooks/"+uuid.New().String()+"/sessions/"+uuid.New().String()+"/stop", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status: %d", w.Code)
	}
}

// ── Misc helpers ────────────────────────────────────────────────────

func TestParseInt64Defaults(t *testing.T) {
	t.Parallel()
	if got := parseInt64("", 7); got != 7 {
		t.Errorf("empty must fall back to default, got %d", got)
	}
	if got := parseInt64("42", 7); got != 42 {
		t.Errorf("parse drift: %d", got)
	}
	if got := parseInt64("nope", 7); got != 7 {
		t.Errorf("invalid must fall back, got %d", got)
	}
}

// ensures rowScanner works with a manually-constructed scanner —
// keeps the abstraction independent of pgx import internals.
type fakeScanner struct{ values []any }

func (f *fakeScanner) Scan(dest ...any) error {
	for i := range dest {
		switch d := dest[i].(type) {
		case *uuid.UUID:
			*d = f.values[i].(uuid.UUID)
		case *string:
			*d = f.values[i].(string)
		}
	}
	return nil
}

func TestRowScannerInterfaceWorks(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	f := &fakeScanner{values: []any{id, "s"}}
	var got1 uuid.UUID
	var got2 string
	if err := f.Scan(&got1, &got2); err != nil || got1 != id || got2 != "s" {
		t.Fatalf("scanner contract drift: %v %v %v", got1, got2, err)
	}
}

func fixedTime() time.Time { return time.Unix(1_700_000_000, 0).UTC() }

type fakeNotebookListRepo struct {
	notebooks         []models.Notebook
	total             int64
	sessions          []models.Session
	err               error
	notebookParams    ListNotebooksParams
	sessionNotebookID uuid.UUID
}

func (f *fakeNotebookListRepo) ListNotebooks(_ context.Context, params ListNotebooksParams) ([]models.Notebook, int64, error) {
	f.notebookParams = params
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.notebooks, f.total, nil
}

func (f *fakeNotebookListRepo) ListSessions(_ context.Context, notebookID uuid.UUID) ([]models.Session, error) {
	f.sessionNotebookID = notebookID
	if f.err != nil {
		return nil, f.err
	}
	return f.sessions, nil
}
