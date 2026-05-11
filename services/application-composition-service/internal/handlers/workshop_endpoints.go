package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/application-composition-service/internal/catalog"
	"github.com/openfoundry/openfoundry-go/services/application-composition-service/internal/models"
)

func (h *Handlers) ListAppTemplates(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	rows, err := h.Repo.ListAppTemplates(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

func (h *Handlers) CreateAppFromTemplate(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var body models.CreateAppRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	creator := claims.Sub
	app, err := h.Repo.CreateAppFromTemplate(r.Context(), &body, &creator)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "template not found")
		return
	}
	if err != nil {
		writeAppMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, app)
}

func (h *Handlers) AddPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	appID, ok := appIDFromRequest(w, r)
	if !ok {
		return
	}
	var page models.AppPage
	if err := json.NewDecoder(r.Body).Decode(&page); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	app, err := h.Repo.AddPage(r.Context(), appID, page)
	if err != nil {
		writeAppMutationError(w, err)
		return
	}
	if app == nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (h *Handlers) UpdatePage(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	appID, ok := appIDFromRequest(w, r)
	if !ok {
		return
	}
	pageID := strings.TrimSpace(chi.URLParam(r, "pageID"))
	if pageID == "" {
		writeError(w, http.StatusBadRequest, "page id is required")
		return
	}
	var page models.AppPage
	if err := json.NewDecoder(r.Body).Decode(&page); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	app, err := h.Repo.UpdatePage(r.Context(), appID, pageID, page)
	if err != nil {
		writeAppMutationError(w, err)
		return
	}
	if app == nil {
		writeError(w, http.StatusNotFound, "app or page not found")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (h *Handlers) DeletePage(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	appID, ok := appIDFromRequest(w, r)
	if !ok {
		return
	}
	pageID := strings.TrimSpace(chi.URLParam(r, "pageID"))
	if pageID == "" {
		writeError(w, http.StatusBadRequest, "page id is required")
		return
	}
	app, err := h.Repo.DeletePage(r.Context(), appID, pageID)
	if err != nil {
		writeAppMutationError(w, err)
		return
	}
	if app == nil {
		writeError(w, http.StatusNotFound, "app or page not found")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (h *Handlers) GetSlatePackage(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	appID, ok := appIDFromRequest(w, r)
	if !ok {
		return
	}
	pkg, err := h.Repo.BuildSlatePackage(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pkg == nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	writeJSON(w, http.StatusOK, pkg)
}

func (h *Handlers) ImportSlatePackage(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	appID, ok := appIDFromRequest(w, r)
	if !ok {
		return
	}
	var body models.ImportSlatePackageRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	response, err := h.Repo.ImportSlatePackage(r.Context(), appID, body)
	if err != nil {
		writeAppMutationError(w, err)
		return
	}
	if response == nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handlers) ListWidgetCatalog(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	doc, err := catalog.LoadWidgetCatalog()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("X-OpenFoundry-Widget-Catalog-Version", doc.CatalogVersion)
	w.Header().Set("X-OpenFoundry-Widget-Catalog-Schema", doc.SchemaVersion)
	writeJSON(w, http.StatusOK, doc.Items)
}

func appIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return uuid.UUID{}, false
	}
	return id, true
}

func writeAppMutationError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	if strings.Contains(msg, "is required") ||
		strings.Contains(msg, "must ") ||
		strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "unsupported") ||
		strings.Contains(msg, "invalid") {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	writeError(w, http.StatusInternalServerError, msg)
}
