// Package handler hosts the HTTP handlers for notebook-runtime-service.
//
// Status:
//
//   - Notebook + Cell + Session CRUD: 1:1 ported against pgx (matches
//     Rust sqlx). When the DB pool is nil, explicit smoke mode uses an
//     in-memory repository; otherwise handlers return a database-required 503.
//   - Workspace file CRUD: filesystem-backed via `domain/environment`.
//   - Notepad export: HTML rendering via `domain/notepad`.
//   - Cell execute (`ExecuteCell` / `ExecuteAllCells`): Python cells run
//     through the python-sidecar gRPC boundary. SQL mirrors Rust by POSTing
//     to query-service, R shells out to Rscript, and LLM mirrors Rust by
//     POSTing to ai-service chat completions while tracking conversations.
//
// Notepad documents + presence are repository-backed. The no-DB test/smoke
// path uses the in-memory repository; production uses Postgres.
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/domain/environment"
	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/domain/notepad"
	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/models"
	nbrepo "github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/repo"
)

// State carries the deps every handler needs.
type State struct {
	Cfg          *config.Config
	Pool         *pgxpool.Pool
	PythonKernel NotebookPythonKernel
	SQLKernel    NotebookSQLKernel
	RKernel      NotebookRKernel
	LLMKernel    NotebookLLMKernel
	NotepadRepo  nbrepo.NotepadRepository
	ListRepo     NotebookListRepository
	MemoryRepo   *MemoryNotebookRepo
}

func (s *State) smokeMode() bool {
	return s.Cfg != nil && s.Cfg.SmokeMode
}

func (s *State) memoryRepo() *MemoryNotebookRepo {
	if s.MemoryRepo == nil {
		s.MemoryRepo = NewMemoryNotebookRepo()
	}
	return s.MemoryRepo
}

func (s *State) notebookListRepo() NotebookListRepository {
	if s.ListRepo != nil {
		return s.ListRepo
	}
	if s.Pool != nil {
		return PostgresNotebookListRepository{Pool: s.Pool}
	}
	if s.smokeMode() {
		return s.memoryRepo()
	}
	return nil
}

func (s *State) databaseRequired(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, errBody("DATABASE_URL is required unless NOTEBOOK_RUNTIME_SMOKE_MODE=true"))
}

// ── Workspace files (1:1 ported domain/environment) ──────────────────

func (s *State) ListWorkspaceFiles(w http.ResponseWriter, r *http.Request) {
	nb, err := pathUUID(r, "notebook_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid notebook id"))
		return
	}
	files, err := environment.ListWorkspaceFiles(s.Cfg.DataDir, nb)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": files})
}

func (s *State) UpsertWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	nb, err := pathUUID(r, "notebook_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid notebook id"))
		return
	}
	var body models.UpsertNotebookWorkspaceFileRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	file, err := environment.UpsertWorkspaceFile(s.Cfg.DataDir, nb, body.Path, body.Content)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, file)
}

func (s *State) DeleteWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	nb, err := pathUUID(r, "notebook_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid notebook id"))
		return
	}
	path := r.URL.Query().Get("path")
	ok, err := environment.DeleteWorkspaceFile(s.Cfg.DataDir, nb, path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err.Error()))
		return
	}
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Notepad documents + presence ───────────────────────────────────

func (s *State) notepadRepo() nbrepo.NotepadRepository {
	if s.NotepadRepo != nil {
		return s.NotepadRepo
	}
	if s.Pool != nil {
		s.NotepadRepo = nbrepo.NewPostgresNotepadRepository(s.Pool)
		return s.NotepadRepo
	}
	s.NotepadRepo = nbrepo.NewInMemoryNotepadRepository()
	return s.NotepadRepo
}

func (s *State) ListDocuments(w http.ResponseWriter, r *http.Request) {
	claims := requireClaims(w, r)
	if claims == nil {
		return
	}
	page := parseInt64Query(r, "page", 1)
	perPage := parseInt64Query(r, "per_page", 20)
	result, err := s.notepadRepo().ListDocuments(r.Context(), nbrepo.ListDocumentsParams{
		OwnerID: claims.Sub,
		Page:    page,
		PerPage: perPage,
		Search:  r.URL.Query().Get("search"),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *State) CreateDocument(w http.ResponseWriter, r *http.Request) {
	claims := requireClaims(w, r)
	if claims == nil {
		return
	}
	var body models.CreateNotepadDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		writeJSON(w, http.StatusBadRequest, errBody("title is required"))
		return
	}
	doc, err := s.notepadRepo().CreateDocument(r.Context(), nbrepo.CreateDocumentParams{
		Title:       title,
		Description: strPtrValue(body.Description),
		OwnerID:     claims.Sub,
		Content:     strPtrValue(body.Content),
		TemplateKey: nonEmptyPtr(body.TemplateKey),
		Widgets:     body.Widgets,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, doc)
}

func (s *State) GetDocument(w http.ResponseWriter, r *http.Request) {
	claims := requireClaims(w, r)
	if claims == nil {
		return
	}
	documentID, err := pathUUID(r, "document_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid document id"))
		return
	}
	doc, ok, err := s.notepadRepo().GetDocument(r.Context(), documentID, claims.Sub)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *State) UpdateDocument(w http.ResponseWriter, r *http.Request) {
	claims := requireClaims(w, r)
	if claims == nil {
		return
	}
	documentID, err := pathUUID(r, "document_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid document id"))
		return
	}
	var body models.UpdateNotepadDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	doc, ok, err := s.notepadRepo().UpdateDocument(r.Context(), nbrepo.UpdateDocumentParams{
		ID:            documentID,
		OwnerID:       claims.Sub,
		Title:         nonEmptyPtr(body.Title),
		Description:   body.Description,
		Content:       body.Content,
		TemplateKey:   nonEmptyPtr(body.TemplateKey),
		Widgets:       body.Widgets,
		LastIndexedAt: body.LastIndexedAt,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

func (s *State) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	claims := requireClaims(w, r)
	if claims == nil {
		return
	}
	documentID, err := pathUUID(r, "document_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid document id"))
		return
	}
	ok, err := s.notepadRepo().DeleteDocument(r.Context(), documentID, claims.Sub)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *State) ListPresence(w http.ResponseWriter, r *http.Request) {
	claims := requireClaims(w, r)
	if claims == nil {
		return
	}
	documentID, err := pathUUID(r, "document_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid document id"))
		return
	}
	presence, err := s.notepadRepo().ListPresence(r.Context(), documentID, claims.Sub)
	if errors.Is(err, nbrepo.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": presence})
}

func (s *State) UpsertPresence(w http.ResponseWriter, r *http.Request) {
	claims := requireClaims(w, r)
	if claims == nil {
		return
	}
	documentID, err := pathUUID(r, "document_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid document id"))
		return
	}
	var body models.UpsertNotepadPresenceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	sessionID := strings.TrimSpace(body.SessionID)
	displayName := strings.TrimSpace(body.DisplayName)
	if sessionID == "" || displayName == "" {
		writeJSON(w, http.StatusBadRequest, errBody("session_id and display_name are required"))
		return
	}
	presence, err := s.notepadRepo().UpsertPresence(r.Context(), nbrepo.UpsertPresenceParams{
		DocumentID:  documentID,
		OwnerID:     claims.Sub,
		UserID:      claims.Sub,
		SessionID:   sessionID,
		DisplayName: displayName,
		CursorLabel: strPtrValue(body.CursorLabel),
		Color:       defaultStr(strPtrValue(body.Color), "#0f766e"),
	})
	if errors.Is(err, nbrepo.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, presence)
}

func (s *State) ExportDocument(w http.ResponseWriter, r *http.Request) {
	claims := requireClaims(w, r)
	if claims == nil {
		return
	}
	var inline models.NotepadDocument
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&inline); err == nil && inline.ID != uuid.Nil {
			writeJSON(w, http.StatusOK, notepad.RenderExportPayload(&inline))
			return
		}
	}
	documentID, err := pathUUID(r, "document_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid document id"))
		return
	}
	doc, ok, err := s.notepadRepo().GetDocument(r.Context(), documentID, claims.Sub)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	writeJSON(w, http.StatusOK, notepad.RenderExportPayload(&doc))
}

func parseInt64Query(r *http.Request, key string, fallback int64) int64 {
	if raw := r.URL.Query().Get(key); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return v
		}
	}
	return fallback
}

func strPtrValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func nonEmptyPtr(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// ── Auth helper ──────────────────────────────────────────────────────

// requireClaims pulls the JWT claims attached by authmw. Returns nil
// + writes 401 when the upstream middleware has not been wired (or
// the JWT was absent / invalid).
func requireClaims(w http.ResponseWriter, r *http.Request) *authmw.Claims {
	c, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
		return nil
	}
	return c
}

// ── Shared utilities ─────────────────────────────────────────────────

func pathUUID(r *http.Request, key string) (uuid.UUID, error) {
	raw := chi.URLParam(r, key)
	if raw == "" {
		return uuid.Nil, errInvalid("missing path parameter " + key)
	}
	return uuid.Parse(strings.TrimSpace(raw))
}

func errBody(msg string) map[string]string { return map[string]string{"error": msg} }

func errInvalid(msg string) error { return errors.New(msg) }

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}
