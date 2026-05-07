// Package handler — kernel session CRUD. 1:1 port of
// `services/notebook-runtime-service/src/handlers/sessions.rs`.
//
// Python session lifecycle is wired to python-sidecar when configured.
// LLM sessions persist conversation ids in the LLM kernel adapter. SQL and R
// are stateless, matching Rust ensure_session behaviour.
package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openfoundry/openfoundry-go/services/notebook-runtime-service/internal/models"
)

// CreateSession mirrors `pub async fn create_session`. Persists a
// row at status `idle` so the session list endpoint can spot it; the
// kernel-manager spawn lands when the kernel runtime ports.
func (s *State) CreateSession(w http.ResponseWriter, r *http.Request) {
	claims := requireClaims(w, r)
	if claims == nil {
		return
	}
	notebookID, err := pathUUID(r, "notebook_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid notebook id"))
		return
	}
	var body models.CreateSessionRequest
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid body"))
		return
	}
	kernel := "python"
	if body.Kernel != nil && *body.Kernel != "" {
		kernel = *body.Kernel
	}
	id, _ := uuid.NewV7()
	if kernel == "python" && s.PythonKernel != nil {
		if err := s.PythonKernel.EnsureSession(r.Context(), id); err != nil {
			writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
			return
		}
	}
	if s.Pool == nil {
		if !s.smokeMode() {
			s.databaseRequired(w)
			return
		}
		now := time.Now().UTC()
		sess := models.Session{
			ID: id, NotebookID: notebookID, Kernel: kernel,
			Status: "idle", StartedBy: claims.Sub,
			CreatedAt: now, LastActivity: now,
		}
		s.memoryRepo().putSession(sess)
		writeJSON(w, http.StatusCreated, sess)
		return
	}
	row := s.Pool.QueryRow(r.Context(), `
        INSERT INTO sessions (id, notebook_id, kernel, status, started_by)
        VALUES ($1, $2, $3, 'idle', $4)
        RETURNING id, notebook_id, kernel, status, started_by, created_at, last_activity`,
		id, notebookID, kernel, claims.Sub)
	sess, err := scanSession(row)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusCreated, sess)
}

// ListSessions mirrors `pub async fn list_sessions`.
func (s *State) ListSessions(w http.ResponseWriter, r *http.Request) {
	notebookID, err := pathUUID(r, "notebook_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid notebook id"))
		return
	}
	repo := s.notebookListRepo()
	if repo == nil {
		s.databaseRequired(w)
		return
	}
	sessions, err := repo.ListSessions(r.Context(), notebookID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": sessions})
}

// StopSession mirrors `pub async fn stop_session`.
func (s *State) StopSession(w http.ResponseWriter, r *http.Request) {
	sessionID, err := pathUUID(r, "session_id")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody("invalid session id"))
		return
	}
	if s.Pool == nil {
		if !s.smokeMode() {
			s.databaseRequired(w)
			return
		}
		sess, ok := s.memoryRepo().stopSession(sessionID)
		if !ok {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		if sess.Kernel == "python" && s.PythonKernel != nil {
			_ = s.PythonKernel.DropSession(r.Context(), sessionID)
		}
		if sess.Kernel == "llm" && s.LLMKernel != nil {
			_ = s.LLMKernel.DropSession(r.Context(), sessionID)
		}
		writeJSON(w, http.StatusOK, sess)
		return
	}
	row := s.Pool.QueryRow(r.Context(), `
        UPDATE sessions SET status = 'dead', last_activity = NOW()
        WHERE id = $1
        RETURNING id, notebook_id, kernel, status, started_by, created_at, last_activity`,
		sessionID)
	sess, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err.Error()))
		return
	}
	switch sess.Kernel {
	case "python":
		if s.PythonKernel != nil {
			_ = s.PythonKernel.DropSession(r.Context(), sessionID)
		}
	case "llm":
		if s.LLMKernel != nil {
			_ = s.LLMKernel.DropSession(r.Context(), sessionID)
		}
	}
	writeJSON(w, http.StatusOK, sess)
}

func scanSession(s rowScanner) (models.Session, error) {
	var sess models.Session
	err := s.Scan(&sess.ID, &sess.NotebookID, &sess.Kernel, &sess.Status,
		&sess.StartedBy, &sess.CreatedAt, &sess.LastActivity)
	return sess, err
}
