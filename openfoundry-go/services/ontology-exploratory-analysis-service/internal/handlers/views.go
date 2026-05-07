// Package handlers ports the saved-view / saved-map / writeback-proposal
// handlers from `services/ontology-exploratory-analysis-service/src/handlers.rs`.
//
// Like the Rust module the handlers persist saved views and maps as
// declarative `DefinitionStore` rows (kinds `exploratory_view`,
// `exploratory_map`) and writeback proposals as `ActionLogStore`
// appends. The Rust binary keeps these handlers as `#[allow(dead_code)]`
// until the four service-consolidation merges land — the Go binary
// mirrors that and only mounts them when the caller threads a
// `*Handlers` through `server.BuildRouterWithHandlers`.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
	"github.com/openfoundry/openfoundry-go/services/ontology-exploratory-analysis-service/internal/models"
)

// Kind discriminators stored in DefinitionStore. Values are byte-exact
// to the Rust constants in src/handlers.rs.
const (
	viewKind      = "exploratory_view"
	mapKind       = "exploratory_map"
	writebackKind = "exploratory.writeback_proposed"
	pageLimit     = uint32(200)
)

// Handlers carries the dependencies the saved-view / saved-map /
// writeback handlers consume. Mirrors `pub(crate) struct AppState` in
// Rust src/main.rs (the `actions` field lands with OEA-2 — kept here
// so the same struct serves both slices).
type Handlers struct {
	Definitions storageabstraction.DefinitionStore
	Actions     storageabstraction.ActionLogStore
	Tenant      storageabstraction.TenantId
	Subject     string
	// Now returns the current wall clock in milliseconds. Defaults to
	// time.Now when nil — handlers under test inject a deterministic
	// clock.
	Now func() int64
}

// New builds a handler set with the given dependencies. `actions` may
// be nil when the caller only mounts the views/maps subset (OEA-1
// alone); it MUST be non-nil before mounting writeback (OEA-2).
func New(definitions storageabstraction.DefinitionStore, tenant storageabstraction.TenantId, subject string) *Handlers {
	return &Handlers{Definitions: definitions, Tenant: tenant, Subject: subject}
}

// MountViews attaches the saved-view routes to r. Mirrors the Rust
// `list_views`, `create_view`, `get_view` handler trio.
func (h *Handlers) MountViews(r chi.Router) {
	r.Get("/api/v1/views", h.ListViews)
	r.Post("/api/v1/views", h.CreateView)
	r.Get("/api/v1/views/{id}", h.GetView)
}

// MountMaps attaches the saved-map routes. Mirrors the Rust
// `list_maps`, `create_map` handler pair.
func (h *Handlers) MountMaps(r chi.Router) {
	r.Get("/api/v1/maps", h.ListMaps)
	r.Post("/api/v1/maps", h.CreateMap)
}

// ListViews mirrors Rust `pub async fn list_views`.
func (h *Handlers) ListViews(w http.ResponseWriter, r *http.Request) {
	rows, err := h.Definitions.List(r.Context(), h.viewQuery(nil), storageabstraction.Eventual())
	if err != nil {
		writeRepoError(w, err)
		return
	}
	out := make([]models.ExploratoryView, 0, len(rows.Items))
	for _, rec := range rows.Items {
		v, conv := viewFromRecord(rec)
		if conv != nil {
			plainText(w, http.StatusInternalServerError, conv.Error())
			return
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

// CreateView mirrors Rust `pub async fn create_view`.
func (h *Handlers) CreateView(w http.ResponseWriter, r *http.Request) {
	var body models.CreateViewRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		plainText(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Slug) == "" {
		plainText(w, http.StatusBadRequest, "slug is required")
		return
	}
	if status, msg, err := h.ensureSlugAvailable(r.Context(), body.Slug); err != nil {
		writeRepoError(w, err)
		return
	} else if status != 0 {
		plainText(w, status, msg)
		return
	}

	id, err := uuid.NewV7()
	if err != nil {
		plainText(w, http.StatusInternalServerError, err.Error())
		return
	}
	layout := body.Layout
	if len(layout) == 0 {
		layout = json.RawMessage(`{}`)
	}
	now := h.nowMs()
	ts := datetimeFromMs(now)
	view := models.ExploratoryView{
		ID:         id,
		Slug:       body.Slug,
		Name:       body.Name,
		ObjectType: body.ObjectType,
		FilterSpec: body.FilterSpec,
		Layout:     layout,
		CreatedAt:  ts,
		UpdatedAt:  ts,
	}

	rec, err := h.viewToRecord(view, now)
	if err != nil {
		plainText(w, http.StatusInternalServerError, err.Error())
		return
	}
	outcome, err := h.Definitions.Put(r.Context(), rec, nil)
	if err != nil {
		writeRepoError(w, err)
		return
	}
	if outcome.Kind == storageabstraction.PutVersionConflict {
		plainText(w, http.StatusConflict, "view already exists")
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

// GetView mirrors Rust `pub async fn get_view`.
func (h *Handlers) GetView(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		plainText(w, http.StatusBadRequest, err.Error())
		return
	}
	row, err := h.Definitions.Get(
		r.Context(),
		storageabstraction.DefinitionKind(viewKind),
		storageabstraction.DefinitionId(id.String()),
		storageabstraction.Strong(),
	)
	if err != nil {
		writeRepoError(w, err)
		return
	}
	if row == nil {
		plainText(w, http.StatusNotFound, "view not found")
		return
	}
	view, conv := viewFromRecord(*row)
	if conv != nil {
		plainText(w, http.StatusInternalServerError, conv.Error())
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// viewQuery builds a DefinitionQuery scoped to the handler's tenant
// for the `exploratory_view` kind. `filters` may be nil.
func (h *Handlers) viewQuery(filters map[string]string) storageabstraction.DefinitionQuery {
	tenant := h.Tenant
	return storageabstraction.DefinitionQuery{
		Kind:    storageabstraction.DefinitionKind(viewKind),
		Tenant:  &tenant,
		Filters: filters,
		Page:    storageabstraction.Page{Size: pageLimit},
	}
}

// ensureSlugAvailable mirrors Rust `ensure_slug_available`. Returns
// (status, message, repoErr). Status == 0 ⇒ slug is free. repoErr is
// non-nil only on backend errors so the caller funnels them through
// writeRepoError.
func (h *Handlers) ensureSlugAvailable(ctx context.Context, slug string) (int, string, error) {
	q := h.viewQuery(map[string]string{"slug": slug})
	q.Page.Size = 1
	rows, err := h.Definitions.List(ctx, q, storageabstraction.Strong())
	if err != nil {
		return 0, "", err
	}
	for _, rec := range rows.Items {
		// The InMemoryDefinitionStore ignores Filters, so verify the
		// slug match in the payload — this preserves the Rust
		// production behaviour (Postgres adapter filters server-side)
		// independently of the in-memory fake's filter coverage.
		if recordSlug(rec) == slug {
			return http.StatusConflict, "slug already exists", nil
		}
	}
	return 0, "", nil
}

func (h *Handlers) viewToRecord(view models.ExploratoryView, now int64) (storageabstraction.DefinitionRecord, error) {
	tenant := h.Tenant
	version := uint64(1)
	payload, err := json.Marshal(struct {
		ID         uuid.UUID       `json:"id"`
		Slug       string          `json:"slug"`
		Name       string          `json:"name"`
		ObjectType string          `json:"object_type"`
		FilterSpec json.RawMessage `json:"filter_spec"`
		Layout     json.RawMessage `json:"layout"`
		CreatedAt  string          `json:"created_at"`
		UpdatedAt  string          `json:"updated_at"`
	}{
		ID:         view.ID,
		Slug:       view.Slug,
		Name:       view.Name,
		ObjectType: view.ObjectType,
		FilterSpec: nullableRaw(view.FilterSpec),
		Layout:     nullableRaw(view.Layout),
		CreatedAt:  view.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		UpdatedAt:  view.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
	})
	if err != nil {
		return storageabstraction.DefinitionRecord{}, err
	}
	created := now
	updated := now
	return storageabstraction.DefinitionRecord{
		Kind:        storageabstraction.DefinitionKind(viewKind),
		ID:          storageabstraction.DefinitionId(view.ID.String()),
		Tenant:      &tenant,
		Version:     &version,
		Payload:     payload,
		CreatedAtMs: &created,
		UpdatedAtMs: &updated,
	}, nil
}

func viewFromRecord(record storageabstraction.DefinitionRecord) (models.ExploratoryView, error) {
	id, err := uuid.Parse(string(record.ID))
	if err != nil {
		return models.ExploratoryView{}, fmt.Errorf("stored view id is not a UUID: %w", err)
	}
	slug, err := requiredString(record.Payload, "slug")
	if err != nil {
		return models.ExploratoryView{}, err
	}
	name, err := requiredString(record.Payload, "name")
	if err != nil {
		return models.ExploratoryView{}, err
	}
	objectType, err := requiredString(record.Payload, "object_type")
	if err != nil {
		return models.ExploratoryView{}, err
	}
	filterSpec := payloadField(record.Payload, "filter_spec")
	if len(filterSpec) == 0 {
		filterSpec = json.RawMessage(`{}`)
	}
	layout := payloadField(record.Payload, "layout")
	if len(layout) == 0 {
		layout = json.RawMessage(`{}`)
	}
	createdMs := pickTimestamp(record.CreatedAtMs, record.UpdatedAtMs)
	updatedMs := pickTimestamp(record.UpdatedAtMs, record.CreatedAtMs)
	return models.ExploratoryView{
		ID:         id,
		Slug:       slug,
		Name:       name,
		ObjectType: objectType,
		FilterSpec: filterSpec,
		Layout:     layout,
		CreatedAt:  datetimeFromMs(createdMs),
		UpdatedAt:  datetimeFromMs(updatedMs),
	}, nil
}

// recordSlug returns the `slug` field from a stored view payload, or
// "" when missing. Used by ensureSlugAvailable to compensate for fake
// stores that ignore DefinitionQuery.Filters.
func recordSlug(record storageabstraction.DefinitionRecord) string {
	if v, err := requiredString(record.Payload, "slug"); err == nil {
		return v
	}
	return ""
}

// writeRepoError translates a storage-abstraction error into the same
// (status, message) pair the Rust `repo_error` helper produces.
func writeRepoError(w http.ResponseWriter, err error) {
	var re *storageabstraction.RepoError
	status := http.StatusInternalServerError
	if errors.As(err, &re) {
		switch re.Kind {
		case storageabstraction.RepoInvalidArgument, storageabstraction.RepoTenantScope:
			status = http.StatusBadRequest
		case storageabstraction.RepoNotFound:
			status = http.StatusNotFound
		case storageabstraction.RepoBackend:
			status = http.StatusInternalServerError
		}
	}
	plainText(w, status, err.Error())
}
