// Properties + LinkTypes HTTP handlers (P1 of the post-PoC plan). The web
// frontend already expects these routes; before this slice the only mount
// in this service was object_types, so the apps could not display property
// metadata or navigate link relationships.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/ontology-definition-service/internal/models"
)

func (h *Handlers) ListProperties(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	typeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid type id")
		return
	}
	items, err := h.Repo.ListProperties(r.Context(), typeID)
	if err != nil {
		slog.Error("list properties", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list properties")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handlers) CreateProperty(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	typeID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid type id")
		return
	}
	var body models.CreatePropertyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if body.Name == "" || body.PropertyType == "" {
		writeJSONErr(w, http.StatusBadRequest, "name + property_type are required")
		return
	}
	created, err := h.Repo.CreateProperty(r.Context(), typeID, &body)
	if err != nil {
		slog.Error("create property", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to create property")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handlers) ListLinkTypes(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var objectTypeID *uuid.UUID
	if raw := r.URL.Query().Get("object_type_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			writeJSONErr(w, http.StatusBadRequest, "invalid object_type_id")
			return
		}
		objectTypeID = &parsed
	}
	items, err := h.Repo.ListLinkTypes(r.Context(), objectTypeID)
	if err != nil {
		slog.Error("list link types", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to list link types")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": len(items)})
}

func (h *Handlers) GetLinkType(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	link, err := h.Repo.GetLinkType(r.Context(), id)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if link == nil {
		writeJSONErr(w, http.StatusNotFound, "link type not found")
		return
	}
	writeJSON(w, http.StatusOK, link)
}

func (h *Handlers) CreateLinkType(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var body models.CreateLinkTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if body.Name == "" || body.SourceTypeID == uuid.Nil || body.TargetTypeID == uuid.Nil {
		writeJSONErr(w, http.StatusBadRequest, "name + source_type_id + target_type_id are required")
		return
	}
	created, err := h.Repo.CreateLinkType(r.Context(), &body, claims.Sub)
	if err != nil {
		slog.Error("create link type", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to create link type")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *Handlers) UpdateLinkType(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var body models.UpdateLinkTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid json body")
		return
	}
	updated, err := h.Repo.UpdateLinkType(r.Context(), id, &body)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if updated == nil {
		writeJSONErr(w, http.StatusNotFound, "link type not found")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
