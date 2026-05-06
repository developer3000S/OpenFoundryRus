// Package handlers exposes the HTTP surface of solution-design-service.
package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/solution-design-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/solution-design-service/internal/repo"
)

type Handlers struct {
	Repo *repo.Repo
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(msg))
}

func (h *Handlers) ListItems(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Repo.ListPrimary(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handlers) CreateItem(w http.ResponseWriter, r *http.Request) {
	var body models.CreatePrimaryRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	row, err := h.Repo.CreatePrimary(r.Context(), body.Payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (h *Handlers) GetItem(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	row, err := h.Repo.GetPrimary(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if row == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *Handlers) ListSecondary(w http.ResponseWriter, r *http.Request) {
	parent, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	rows, err := h.Repo.ListSecondary(r.Context(), parent)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *Handlers) CreateSecondary(w http.ResponseWriter, r *http.Request) {
	parent, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	var body models.CreateSecondaryRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	row, err := h.Repo.CreateSecondary(r.Context(), parent, body.Payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, row)
}
