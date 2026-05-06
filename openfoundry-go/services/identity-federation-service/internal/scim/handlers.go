package scim

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
)

// Handlers wires the read-side SCIM endpoints (this slice).
// CreateUser / PatchUser / DeleteUser + the Group surface land in
// follow-up slices (3.7b.3.2 / 3.7b.3.3).
//
// BaseURL is the public origin of the service (e.g.
// `https://id.example.com`); Handlers stitch that prefix into
// every Meta.Location, ResourceType and Schema URL so SCIM
// clients see a stable absolute URL.
type Handlers struct {
	BaseURL string
	Users   UserStore
}

// ─── Discovery endpoints ────────────────────────────────────────────

// ServiceProviderConfigHandler handles
// `GET /scim/v2/ServiceProviderConfig`. Mirrors fn
// service_provider_config_handler.
func (h *Handlers) ServiceProviderConfigHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, ServiceProviderConfigPayload(h.BaseURL))
}

// ListSchemas handles `GET /scim/v2/Schemas`. Returns the User +
// Group schema descriptors wrapped in the canonical SCIM list
// envelope.
func (h *Handlers) ListSchemas(w http.ResponseWriter, _ *http.Request) {
	resources := SchemaResources(h.BaseURL)
	writeJSON(w, http.StatusOK, NewScimList(resources, len(resources), 1))
}

// GetSchema handles `GET /scim/v2/Schemas/{id}`. Returns 404 when
// the URN doesn't match a known schema.
func (h *Handlers) GetSchema(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	for _, schema := range SchemaResources(h.BaseURL) {
		if schema.ID == id {
			writeJSON(w, http.StatusOK, schema)
			return
		}
	}
	writeScimError(w, http.StatusNotFound, "SCIM schema not found", nil)
}

// ListResourceTypes handles `GET /scim/v2/ResourceTypes`.
func (h *Handlers) ListResourceTypes(w http.ResponseWriter, _ *http.Request) {
	resources := ResourceTypes(h.BaseURL)
	writeJSON(w, http.StatusOK, NewScimList(resources, len(resources), 1))
}

// GetResourceType handles `GET /scim/v2/ResourceTypes/{id}`.
// Returns 404 when the id doesn't match `User` or `Group`.
func (h *Handlers) GetResourceType(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	for _, rt := range ResourceTypes(h.BaseURL) {
		if rt.ID == id {
			writeJSON(w, http.StatusOK, rt)
			return
		}
	}
	writeScimError(w, http.StatusNotFound, "SCIM resource type not found", nil)
}

// ─── User read endpoints ────────────────────────────────────────────

// GetUser handles `GET /scim/v2/Users/{id}`. Mirrors fn get_user.
//
// Requires SCIM-reader permission via requireScimReader; behind
// the SCIM router root the cedarauthz.AdminGuard for the
// scim_provision_user action gates the *write* surfaces, while
// the read path is permission-gated here so service-account
// readers can issue GET without holding the strict provisioning
// role.
func (h *Handlers) GetUser(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeScimError(w, http.StatusUnauthorized, "missing claims", nil)
		return
	}
	if !RequireScimReader(claims) {
		writeScimError(w, http.StatusForbidden, "missing scim:read permission", nil)
		return
	}
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		invalid := "invalidValue"
		writeScimError(w, http.StatusBadRequest, "user id is not a valid UUID", &invalid)
		return
	}
	rec, err := h.Users.Get(r.Context(), id)
	if err != nil {
		slog.Error("scim: load user failed", slog.String("error", err.Error()))
		writeScimError(w, http.StatusInternalServerError, "failed to load user", nil)
		return
	}
	if rec == nil {
		writeScimError(w, http.StatusNotFound, "SCIM user not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, UserToScim(*rec, h.BaseURL))
}

// ListUsers handles `GET /scim/v2/Users`. Mirrors fn list_users.
//
// Honours the SCIM pagination convention: startIndex is 1-based
// and defaults to 1; count defaults to 100 and is clamped to
// [1, 500] (matching the FilterFeature.MaxResults the service
// advertises). Filter supports `userName eq "..."` and
// `externalId eq "..."`.
func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeScimError(w, http.StatusUnauthorized, "missing claims", nil)
		return
	}
	if !RequireScimReader(claims) {
		writeScimError(w, http.StatusForbidden, "missing scim:read permission", nil)
		return
	}

	query := parseListQuery(r)
	filter, scimErr := ParseEqFilter(query.Filter, []string{"userName", "externalId"})
	if scimErr != nil {
		writeScimErrorPayload(w, http.StatusBadRequest, *scimErr)
		return
	}
	rows, total, err := h.Users.List(r.Context(), filter, query.StartIndex, query.Count)
	if err != nil {
		slog.Error("scim: list users failed", slog.String("error", err.Error()))
		writeScimError(w, http.StatusInternalServerError, "failed to list users", nil)
		return
	}
	resources := make([]ScimUser, 0, len(rows))
	for _, r := range rows {
		resources = append(resources, UserToScim(r, h.BaseURL))
	}
	writeJSON(w, http.StatusOK, NewScimList(resources, total, query.StartIndex))
}

// ─── Query + claim helpers ──────────────────────────────────────────

// ListQuery is the parsed form of the standard SCIM list query
// string: filter, startIndex (1-based, default 1), count
// (default 100, clamp [1, 500]).
type ListQuery struct {
	Filter     *string
	StartIndex int
	Count      int
}

// parseListQuery decodes the query-string parameters with the
// same defaults the Rust handler applies.
func parseListQuery(r *http.Request) ListQuery {
	q := ListQuery{StartIndex: 1, Count: 100}
	values := r.URL.Query()
	if v := values.Get("filter"); strings.TrimSpace(v) != "" {
		v := v
		q.Filter = &v
	}
	if v := values.Get("startIndex"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			q.StartIndex = n
		}
	}
	if v := values.Get("count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			if n < 1 {
				n = 1
			}
			if n > 500 {
				n = 500
			}
			q.Count = n
		}
	}
	return q
}

// RequireScimReader mirrors fn require_scim_reader — admin OR
// scim:read OR control_panel:write OR is_scim_service_account.
func RequireScimReader(claims *authmw.Claims) bool {
	return claims.HasRole("admin") ||
		claims.HasPermission("scim", "read") ||
		claims.HasPermission("control_panel", "write") ||
		IsScimServiceAccount(claims)
}

// RequireScimWriter mirrors fn require_scim_writer — admin OR
// scim:write OR is_scim_service_account.
//
// Exposed here even though this slice doesn't wire the write
// endpoints because the cedar-guarded write handlers in
// 3.7b.3.2 reuse the helper.
func RequireScimWriter(claims *authmw.Claims) bool {
	return claims.HasRole("admin") ||
		claims.HasPermission("scim", "write") ||
		IsScimServiceAccount(claims)
}

// IsScimServiceAccount reports whether the principal carries
// `kind: "service_account"` AND the `scim_writer` role. Mirrors
// fn is_scim_service_account.
func IsScimServiceAccount(claims *authmw.Claims) bool {
	if !claims.HasRole("scim_writer") {
		return false
	}
	if len(claims.Attributes) == 0 {
		return false
	}
	var attrs map[string]any
	if err := json.Unmarshal(claims.Attributes, &attrs); err != nil {
		return false
	}
	kind, ok := attrs["kind"].(string)
	return ok && kind == "service_account"
}

// ─── Wire helpers ───────────────────────────────────────────────────

// writeJSON writes `body` as JSON with Content-Type =
// application/scim+json (the SCIM-mandated MIME).
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/scim+json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeScimError writes a ScimError envelope with the given HTTP
// status. detail is the human-readable message; scimType is the
// optional SCIM error tag (invalidFilter, mutability, etc.).
func writeScimError(w http.ResponseWriter, status int, detail string, scimType *string) {
	writeJSON(w, status, NewScimError(status, detail, scimType))
}

// writeScimErrorPayload writes a pre-built ScimError envelope.
func writeScimErrorPayload(w http.ResponseWriter, status int, payload ScimError) {
	writeJSON(w, status, payload)
}

// errorWithDetail wraps a fmt-formatted error so the slog Errorf
// pattern stays compact at call sites.
func errorWithDetail(prefix string, err error) error {
	return fmt.Errorf("%s: %w", prefix, err)
}

var _ = errors.New
var _ = errorWithDetail
