// HTTP handlers for `/api/v1/apps` — the App Builder surface that the SPA's
// lib/api/apps.ts client calls. Scope here is the CRUD + publish path the
// dashboard fixture exercises; advanced editor endpoints (slate, preview,
// per-page CRUD, widget catalog) land in follow-up slices.
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/application-composition-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/application-composition-service/internal/repo"
)

func (h *Handlers) ListApps(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	rows, total, err := h.Repo.ListApps(r.Context(), repo.ListAppsFilter{
		Search: q.Get("search"), Status: q.Get("status"), Page: page, PerPage: perPage,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows, "total": total})
}

func (h *Handlers) GetApp(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	app, err := h.Repo.GetApp(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if app == nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (h *Handlers) CreateApp(w http.ResponseWriter, r *http.Request) {
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
	if strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	creator := claims.Sub
	app, err := h.Repo.CreateApp(r.Context(), &body, &creator)
	if err != nil {
		writeAppMutationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, app)
}

func (h *Handlers) UpdateApp(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	var body models.UpdateAppRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	app, err := h.Repo.UpdateApp(r.Context(), id, &body)
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

func (h *Handlers) DeleteApp(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	ok, err := h.Repo.DeleteApp(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) PublishApp(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	var body models.PublishAppRequest
	_ = json.NewDecoder(r.Body).Decode(&body)
	publisher := claims.Sub
	v, err := h.Repo.PublishApp(r.Context(), id, body.Notes, &publisher)
	if err != nil {
		writeAppMutationError(w, err)
		return
	}
	if v == nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *Handlers) ListAppVersions(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	rows, err := h.Repo.ListAppVersions(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": rows})
}

func (h *Handlers) PreviewApp(w http.ResponseWriter, r *http.Request) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be a uuid")
		return
	}
	app, err := h.Repo.GetApp(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if app == nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app":            app,
		"widget_catalog": []any{},
		"embed":          embedInfo(app.Slug),
	})
}

func (h *Handlers) GetAppEmbedInfo(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "slug is required")
		return
	}
	writeJSON(w, http.StatusOK, embedInfo(slug))
}

// GetPublishedApp serves the public-facing read endpoint used by embedded /
// runtime app surfaces. No auth required: published apps are designed to be
// consumed by anonymous portal visitors.
func (h *Handlers) GetPublishedApp(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "slug is required")
		return
	}
	app, err := h.Repo.GetAppBySlug(r.Context(), slug)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if app == nil || app.PublishedVersionID == nil {
		writeError(w, http.StatusNotFound, "no published version for slug")
		return
	}
	v, err := h.Repo.GetPublishedVersion(r.Context(), app.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if v == nil {
		writeError(w, http.StatusNotFound, "published version missing")
		return
	}
	publishedAt := v.CreatedAt
	if v.PublishedAt != nil {
		publishedAt = *v.PublishedAt
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"app":                      app,
		"embed":                    embedInfo(slug),
		"published_version_number": v.VersionNumber,
		"published_at":             publishedAt,
	})
}

func embedInfo(slug string) map[string]any {
	url := "/apps/runtime/" + slug
	return map[string]any{
		"url":         url,
		"iframe_html": `<iframe src="` + url + `" loading="lazy" style="width:100%;height:720px;border:0;"></iframe>`,
	}
}
