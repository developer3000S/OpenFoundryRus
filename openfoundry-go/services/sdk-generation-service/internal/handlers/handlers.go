// Package handlers wires the HTTP endpoints for sdk-generation-service.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/libs/core-models/ids"
	"github.com/openfoundry/openfoundry-go/services/sdk-generation-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/sdk-generation-service/internal/repo"
)

// Handlers bundles the dependencies the HTTP layer needs.
type Handlers struct {
	Repo *repo.Repo
}

// ListJobs handles GET /api/v1/sdk-generation-jobs.
func (h *Handlers) ListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.Repo.ListJobs(r.Context())
	if err != nil {
		slog.Error("list jobs failed", slog.String("error", err.Error()))
		writeText(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}

// CreateJob handles POST /api/v1/sdk-generation-jobs.
func (h *Handlers) CreateJob(w http.ResponseWriter, r *http.Request) {
	var body models.CreateJobRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeText(w, http.StatusBadRequest, "invalid body")
		return
	}
	job, err := h.Repo.CreateJob(r.Context(), ids.New(), body.Payload)
	if err != nil {
		slog.Error("create job failed", slog.String("error", err.Error()))
		writeText(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, job)
}

// GetJob handles GET /api/v1/sdk-generation-jobs/{id}.
func (h *Handlers) GetJob(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeText(w, http.StatusBadRequest, "invalid id")
		return
	}
	job, err := h.Repo.GetJob(r.Context(), id)
	if err != nil {
		slog.Error("get job failed", slog.String("error", err.Error()))
		writeText(w, http.StatusInternalServerError, err.Error())
		return
	}
	if job == nil {
		writeText(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// ListPublications handles GET /api/v1/sdk-generation-jobs/{id}/publications.
func (h *Handlers) ListPublications(w http.ResponseWriter, r *http.Request) {
	parent, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeText(w, http.StatusBadRequest, "invalid id")
		return
	}
	pubs, err := h.Repo.ListPublications(r.Context(), parent)
	if err != nil {
		slog.Error("list publications failed", slog.String("error", err.Error()))
		writeText(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, pubs)
}

// CreatePublication handles POST /api/v1/sdk-generation-jobs/{id}/publications.
func (h *Handlers) CreatePublication(w http.ResponseWriter, r *http.Request) {
	parent, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeText(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body models.CreatePublicationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeText(w, http.StatusBadRequest, "invalid body")
		return
	}
	pub, err := h.Repo.CreatePublication(r.Context(), ids.New(), parent, body.Payload)
	if err != nil {
		slog.Error("create publication failed", slog.String("error", err.Error()))
		writeText(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, pub)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeText(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(msg))
}
