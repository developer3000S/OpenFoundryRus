package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/dataset-versioning-service/internal/repo"
)

func (h *Handlers) resolveDatasetForCatalog(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	if _, ok := authmw.FromContext(r.Context()); !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return uuid.Nil, false
	}
	id, err := h.Repo.ResolveDatasetID(r.Context(), datasetIDParam(r))
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSONErr(w, http.StatusNotFound, "dataset not found")
		} else {
			writeJSONErr(w, http.StatusInternalServerError, "failed to resolve dataset")
		}
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handlers) requireDatasetWrite(w http.ResponseWriter, r *http.Request, datasetID uuid.UUID) (*authmw.Claims, bool) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}
	if canWriteDataset(claims) {
		return claims, true
	}
	writePermissionDenied(w, datasetWriteScope, datasetID.String())
	return nil, false
}

func (h *Handlers) requireDatasetAdmin(w http.ResponseWriter, r *http.Request, datasetID uuid.UUID) (*authmw.Claims, bool) {
	claims, ok := authmw.FromContext(r.Context())
	if !ok {
		writeJSONErr(w, http.StatusUnauthorized, "authentication required")
		return nil, false
	}
	if claims.HasRole("admin") || claims.HasPermissionKey("dataset.admin") || claims.HasPermission("dataset", "admin") {
		return claims, true
	}
	writePermissionDenied(w, "datasets:admin", datasetID.String())
	return nil, false
}

func (h *Handlers) GetDatasetModel(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	model, err := h.Repo.GetDatasetRichModel(r.Context(), datasetID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to load dataset model")
		return
	}
	if model == nil {
		writeJSONErr(w, http.StatusNotFound, "dataset not found")
		return
	}
	// When a resolver is wired, recompute markings via the lineage walk so
	// the rich model reflects upstream-inherited entries the repo cannot
	// discover on its own. We tolerate failures here: a partial model with
	// direct-only markings is still useful and matches the Rust fallback.
	if h.Resolver != nil {
		if effective, err := h.Resolver.Compute(r.Context(), datasetID.String()); err == nil {
			model.Markings = effective
		}
	}
	writeJSON(w, http.StatusOK, model)
}

func (h *Handlers) PatchDatasetMetadata(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	claims, ok := h.requireDatasetWrite(w, r, datasetID)
	if !ok {
		return
	}
	var body models.DatasetMetadataPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name != nil {
		if err := validateDatasetName(*body.Name); err != nil {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
	} else if body.DisplayName != nil {
		if err := validateDatasetName(*body.DisplayName); err != nil {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if body.Format != nil {
		normalized := strings.ToLower(*body.Format)
		if err := validateDatasetFormat(normalized); err != nil {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
		body.Format = &normalized
	}
	if body.HealthStatus != nil {
		if err := validateHealthStatus(*body.HealthStatus); err != nil {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if body.ResourceVisibility != nil {
		normalized := strings.ToLower(strings.TrimSpace(*body.ResourceVisibility))
		if err := validateResourceVisibility(normalized); err != nil {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
		body.ResourceVisibility = &normalized
	}
	if body.Visibility != nil && body.ResourceVisibility == nil {
		normalized := strings.ToLower(strings.TrimSpace(*body.Visibility))
		if err := validateResourceVisibility(normalized); err != nil {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
		body.ResourceVisibility = &normalized
	}
	if body.CurrentViewID != nil {
		belongs, err := h.Repo.DatasetViewBelongsToDataset(r.Context(), datasetID, *body.CurrentViewID)
		if err != nil {
			writeJSONErr(w, http.StatusInternalServerError, "failed to validate current_view_id")
			return
		}
		if !belongs {
			writeJSONErr(w, http.StatusBadRequest, "current_view_id must belong to the dataset")
			return
		}
	}
	updated, err := h.Repo.PatchDatasetMetadata(r.Context(), datasetID, &body)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			writeJSONErr(w, http.StatusNotFound, "dataset not found")
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, "failed to update dataset metadata")
		return
	}
	emitDatasetAudit(claims.Sub.String(), models.AuditActionDatasetMetadataUpdate, updated.StoragePath, map[string]any{
		"dataset_id":      updated.ID,
		"current_view_id": updated.CurrentViewID,
		"fields_changed":  metadataPatchChangedFields(&body),
	})
	if h.Resolver != nil {
		h.Resolver.Invalidate(updated.ID.String())
	}
	writeJSON(w, http.StatusOK, updated)
}

// metadataPatchChangedFields lists the columns the patch explicitly targeted
// so audit consumers can drive change-history queries. Order matches the
// Rust audit emit so payload diffs stay byte-for-byte stable.
func metadataPatchChangedFields(p *models.DatasetMetadataPatch) []string {
	out := make([]string, 0, 16)
	if p.Name != nil {
		out = append(out, "name")
	}
	if p.DisplayName != nil {
		out = append(out, "display_name")
	}
	if p.Description != nil {
		out = append(out, "description")
	}
	if p.OwnerID != nil {
		out = append(out, "owner_id")
	}
	if p.Tags != nil {
		out = append(out, "tags")
	}
	if p.Format != nil {
		out = append(out, "format")
	}
	if len(p.Metadata) > 0 {
		out = append(out, "metadata")
	}
	if len(p.Schema) > 0 {
		out = append(out, "schema")
	}
	if p.HealthStatus != nil {
		out = append(out, "health_status")
	}
	if p.CurrentViewID != nil {
		out = append(out, "current_view_id")
	}
	if p.ParentFolderRID != nil || p.ParentFolderRid != nil {
		out = append(out, "parent_folder_rid")
	}
	if p.FolderPath != nil {
		out = append(out, "folder_path")
	}
	if p.ProjectID != nil {
		out = append(out, "project_id")
	}
	if p.ProjectRID != nil {
		out = append(out, "project_rid")
	}
	if p.Path != nil {
		out = append(out, "path")
	}
	if p.ResourceVisibility != nil || p.Visibility != nil {
		out = append(out, "resource_visibility")
	}
	return out
}

func (h *Handlers) ListDatasetMarkings(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	items, err := h.effectiveMarkings(r.Context(), datasetID)
	if err != nil {
		if cycleErr, ok := asMarkingResolveError(err); ok && cycleErr.IsCycle() {
			writeJSON(w, http.StatusConflict, map[string]any{"error": cycleErr.Error(), "rid": cycleErr.RID})
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, "failed to list dataset markings")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// effectiveMarkings dispatches to the lineage-aware MarkingResolver when one
// is wired (production), otherwise falls back to the repo's per-row read so
// tests and small deployments keep working without a lineage-service. The
// result is the same JSON shape: []EffectiveMarking with direct entries
// preceding inherited ones, deduplicated by (id, source).
func (h *Handlers) effectiveMarkings(ctx context.Context, datasetID uuid.UUID) ([]models.EffectiveMarking, error) {
	if h.Resolver != nil {
		return h.Resolver.Compute(ctx, datasetID.String())
	}
	items, err := h.Repo.ListDatasetMarkings(ctx, datasetID)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func asMarkingResolveError(err error) (*models.MarkingResolveError, bool) {
	var target *models.MarkingResolveError
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

func (h *Handlers) PutDatasetMarkings(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	claims, ok := h.requireDatasetAdmin(w, r, datasetID)
	if !ok {
		return
	}
	var body models.PutDatasetMarkingsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.Repo.ReplaceDatasetMarkings(r.Context(), datasetID, body.Markings, claims.Sub); err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to replace dataset markings")
		return
	}
	emitDatasetAudit(claims.Sub.String(), models.AuditActionDatasetMarkingsReplace, datasetID.String(), map[string]any{
		"dataset_id":    datasetID,
		"marking_count": len(body.Markings),
	})
	if h.Resolver != nil {
		h.Resolver.Invalidate(datasetID.String())
	}
	h.ListDatasetMarkings(w, r)
}

func (h *Handlers) ListDatasetPermissions(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	items, err := h.Repo.ListDatasetPermissions(r.Context(), datasetID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list dataset permissions")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) PutDatasetPermissions(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	claims, ok := h.requireDatasetAdmin(w, r, datasetID)
	if !ok {
		return
	}
	var body models.PutDatasetPermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	for _, edge := range body.Permissions {
		source := models.PermissionSourceDirect
		if edge.Source != nil {
			source = *edge.Source
		}
		if err := validatePermissionEdge(edge.PrincipalKind, edge.PrincipalID, edge.Role, edge.Actions, source, edge.InheritedFrom); err != nil {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := h.Repo.ReplaceDatasetPermissions(r.Context(), datasetID, body.Permissions); err != nil {
		if repo.IsConflict(err) {
			writeJSONErr(w, http.StatusConflict, "dataset permission conflict")
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, "failed to replace dataset permissions")
		return
	}
	emitDatasetAudit(claims.Sub.String(), models.AuditActionDatasetPermissionsReplace, datasetID.String(), map[string]any{
		"dataset_id": datasetID,
		"edge_count": len(body.Permissions),
	})
	h.ListDatasetPermissions(w, r)
}

func (h *Handlers) ListDatasetLineageLinks(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	items, err := h.Repo.ListDatasetLineageLinks(r.Context(), datasetID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list dataset lineage links")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) PutDatasetLineageLinks(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	claims, ok := h.requireDatasetWrite(w, r, datasetID)
	if !ok {
		return
	}
	var body models.PutDatasetLineageLinksRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	for _, link := range body.Links {
		if err := validateLineageLink(link.Direction, link.TargetRID); err != nil {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := h.Repo.ReplaceDatasetLineageLinks(r.Context(), datasetID, body.Links); err != nil {
		if repo.IsConflict(err) {
			writeJSONErr(w, http.StatusConflict, "dataset lineage conflict")
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, "failed to replace dataset lineage links")
		return
	}
	emitDatasetAudit(claims.Sub.String(), models.AuditActionDatasetLineageReplace, datasetID.String(), map[string]any{
		"dataset_id": datasetID,
		"link_count": len(body.Links),
	})
	if h.Resolver != nil {
		// Lineage edges feed the marking-inheritance walk; invalidate so the
		// next read recomputes through the new graph shape.
		h.Resolver.Invalidate(datasetID.String())
	}
	h.ListDatasetLineageLinks(w, r)
}

func (h *Handlers) ListDatasetFileIndex(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	items, err := h.Repo.ListDatasetFileIndex(r.Context(), datasetID)
	if err != nil {
		writeJSONErr(w, http.StatusInternalServerError, "failed to list dataset file index")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) PutDatasetFileIndex(w http.ResponseWriter, r *http.Request) {
	datasetID, ok := h.resolveDatasetForCatalog(w, r)
	if !ok {
		return
	}
	claims, ok := h.requireDatasetWrite(w, r, datasetID)
	if !ok {
		return
	}
	var body models.PutDatasetFilesRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	for _, file := range body.Files {
		entryType := models.FileEntryTypeFile
		if file.EntryType != nil {
			entryType = *file.EntryType
		}
		size := int64(0)
		if file.SizeBytes != nil {
			size = *file.SizeBytes
		}
		if err := validateFileIndexEntry(file.Path, file.StoragePath, entryType, size); err != nil {
			writeJSONErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if err := h.Repo.ReplaceDatasetFileIndex(r.Context(), datasetID, body.Files); err != nil {
		if repo.IsConflict(err) {
			writeJSONErr(w, http.StatusConflict, "dataset file index conflict")
			return
		}
		writeJSONErr(w, http.StatusInternalServerError, "failed to replace dataset file index")
		return
	}
	emitDatasetAudit(claims.Sub.String(), models.AuditActionDatasetFilesReplace, datasetID.String(), map[string]any{
		"dataset_id": datasetID,
		"file_count": len(body.Files),
	})
	h.ListDatasetFileIndex(w, r)
}

// ===========================================================================
// Validators — shared by the handlers above. Constants live in models/catalog.go
// so handlers, repo and tests all reference the same enum literals.
// ===========================================================================

func validateDatasetName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("dataset name is required")
	}
	if len(trimmed) > 255 {
		return errors.New("dataset name must be 255 characters or fewer")
	}
	return nil
}

func validateDatasetFormat(format string) error {
	if models.IsKnownDatasetFormat(format) {
		return nil
	}
	return errors.New("unsupported dataset format: " + format)
}

func validateHealthStatus(status string) error {
	if models.IsKnownHealthStatus(status) {
		return nil
	}
	return errors.New("health_status must be one of: unknown, healthy, warning, degraded, critical")
}

func validateResourceVisibility(visibility string) error {
	if models.IsKnownResourceVisibility(visibility) {
		return nil
	}
	return errors.New("resource_visibility must be one of: private, shared, organization, public")
}

func validatePermissionEdge(principalKind, principalID, role string, actions []string, source string, inheritedFrom *string) error {
	if !models.IsKnownPrincipalKind(principalKind) {
		return errors.New("principal_kind must be one of: user, group, role, organization, project, service")
	}
	if !models.IsKnownPermissionSource(source) {
		return errors.New("source must be one of: direct, inherited_from_project, inherited_from_folder, inherited_from_parent")
	}
	if strings.TrimSpace(principalID) == "" {
		return errors.New("principal_id is required")
	}
	if strings.TrimSpace(role) == "" {
		return errors.New("role is required")
	}
	for _, action := range actions {
		if strings.TrimSpace(action) == "" {
			return errors.New("permission actions cannot be empty")
		}
	}
	if source == models.PermissionSourceDirect && inheritedFrom != nil {
		return errors.New("direct permissions cannot set inherited_from")
	}
	if source != models.PermissionSourceDirect && (inheritedFrom == nil || strings.TrimSpace(*inheritedFrom) == "") {
		return errors.New("inherited permissions require inherited_from")
	}
	return nil
}

func validateLineageLink(direction, targetRID string) error {
	if !models.IsKnownLineageDirection(direction) {
		return errors.New("direction must be one of: upstream, downstream")
	}
	if strings.TrimSpace(targetRID) == "" {
		return errors.New("target_rid is required")
	}
	return nil
}

func validateFileIndexEntry(path, storagePath, entryType string, sizeBytes int64) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("file path is required")
	}
	if strings.TrimSpace(storagePath) == "" {
		return errors.New("storage_path is required")
	}
	if !models.IsKnownFileEntryType(entryType) {
		return errors.New("entry_type must be one of: file, directory")
	}
	if sizeBytes < 0 {
		return errors.New("size_bytes must be non-negative")
	}
	return nil
}

// containsString remains for any callers under this package that still use
// the local helper. Newer code prefers models.IsKnown* membership checks.
func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

// ===========================================================================
// MarkingResolver — DV-12 port of data_asset_catalog::domain::markings.rs
//
// Effective markings = direct rows on the dataset ∪ for every immediate
// upstream parent, that parent's effective markings re-tagged with
// "inherited_from_upstream { upstream_rid }". The resolver caches results
// in-process with a TTL so repeated reads short-circuit the lineage walk; an
// invalidate hook is wired so any handler that mutates markings or lineage
// edges can drop the stale entry.
//
// The Rust implementation uses moka. Go has no equivalent in this repo, so
// we ship a tiny TTL map — the cache is bounded (max 10k entries, oldest
// purged on overflow) and protected by a single mutex; under the catalog
// load profile (read-mostly, low cardinality per process) this is fine.
// ===========================================================================

// LineageClient resolves the immediate upstream parents of a dataset RID.
// Production binds it to lineage-service's
// `GET /v1/lineage/{rid}/upstream` endpoint via [HTTPLineageClient]; tests
// inject [InMemoryLineageClient] so marking propagation can be exercised
// without a second HTTP server.
type LineageClient interface {
	Upstream(ctx context.Context, datasetRID string) ([]string, error)
}

// InMemoryLineageClient is the test-friendly implementation backed by a
// child-RID → parents map. Mutations to Edges are not safe across goroutines;
// fixtures should populate the map before sharing the client.
type InMemoryLineageClient struct {
	Edges map[string][]string
}

// Upstream implements [LineageClient]. Returns a copy so the caller cannot
// mutate the test fixture by accident.
func (c *InMemoryLineageClient) Upstream(_ context.Context, datasetRID string) ([]string, error) {
	if c == nil || len(c.Edges) == 0 {
		return nil, nil
	}
	parents := c.Edges[datasetRID]
	if len(parents) == 0 {
		return nil, nil
	}
	out := make([]string, len(parents))
	copy(out, parents)
	return out, nil
}

// HTTPLineageClient calls the lineage service over HTTP. The expected
// response body is `{"upstream": ["ri.foo", ...]}` — same shape produced by
// the Rust HttpLineageClient so either side of the migration boundary can
// answer the question.
type HTTPLineageClient struct {
	BaseURL string
	HTTP    *http.Client
}

// Upstream implements [LineageClient].
func (c *HTTPLineageClient) Upstream(ctx context.Context, datasetRID string) ([]string, error) {
	if c == nil || strings.TrimSpace(c.BaseURL) == "" {
		return nil, fmt.Errorf("lineage client: base URL not configured")
	}
	endpoint := strings.TrimRight(c.BaseURL, "/") + "/v1/lineage/" + url.PathEscape(datasetRID) + "/upstream"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("lineage GET %s: %w", endpoint, err)
	}
	req.Header.Set("Accept", "application/json")
	client := c.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lineage GET %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("lineage GET %s returned %d", endpoint, resp.StatusCode)
	}
	var body struct {
		Upstream []string `json:"upstream"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("lineage GET %s body: %w", endpoint, err)
	}
	return body.Upstream, nil
}

// DirectMarkingsLoader loads the rows where dataset_markings.source = 'direct'
// for the given dataset RID. Decoupled from the resolver so tests can inject
// any backing store and so we don't drag the full Store interface into the
// resolver constructor.
type DirectMarkingsLoader func(ctx context.Context, datasetRID string) ([]models.EffectiveMarking, error)

// MarkingResolverDefaultTTL matches the 60s recommended by the Rust spec.
const MarkingResolverDefaultTTL = 60 * time.Second

// MarkingResolverDefaultCapacity matches the moka 10k cap from Rust.
const MarkingResolverDefaultCapacity = 10_000

// MarkingResolver caches effective-marking computations and walks the
// lineage graph on miss. Construction happens via [NewMarkingResolver]; the
// zero value is unusable.
type MarkingResolver struct {
	loader   DirectMarkingsLoader
	lineage  LineageClient
	ttl      time.Duration
	capacity int
	now      func() time.Time

	mu    sync.Mutex
	cache map[string]markingCacheEntry
	order []string
}

type markingCacheEntry struct {
	items     []models.EffectiveMarking
	expiresAt time.Time
}

// NewMarkingResolver builds a resolver with sensible defaults. `loader`
// reads direct markings from persistence; `lineage` answers upstream
// queries. Both are required.
func NewMarkingResolver(loader DirectMarkingsLoader, lineage LineageClient) *MarkingResolver {
	return NewMarkingResolverWithTTL(loader, lineage, MarkingResolverDefaultTTL)
}

// NewMarkingResolverWithTTL builds a resolver with a caller-supplied cache
// TTL — useful for tests that want fast expiry to verify invalidation.
func NewMarkingResolverWithTTL(loader DirectMarkingsLoader, lineage LineageClient, ttl time.Duration) *MarkingResolver {
	if ttl <= 0 {
		ttl = MarkingResolverDefaultTTL
	}
	return &MarkingResolver{
		loader:   loader,
		lineage:  lineage,
		ttl:      ttl,
		capacity: MarkingResolverDefaultCapacity,
		now:      time.Now,
		cache:    make(map[string]markingCacheEntry, 64),
		order:    make([]string, 0, 64),
	}
}

// Compute returns the effective markings for `datasetRID`. Cached results
// are returned without touching either the loader or the lineage client.
func (r *MarkingResolver) Compute(ctx context.Context, datasetRID string) ([]models.EffectiveMarking, error) {
	if r == nil {
		return nil, models.ErrMarkingResolverDisabled
	}
	if cached, ok := r.lookup(datasetRID); ok {
		return cached, nil
	}
	visiting := make(map[string]struct{}, 8)
	items, err := r.computeInner(ctx, datasetRID, visiting)
	if err != nil {
		return nil, err
	}
	r.store(datasetRID, items)
	return items, nil
}

// Invalidate drops the cached entry for `datasetRID`. Idempotent.
func (r *MarkingResolver) Invalidate(datasetRID string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cache, datasetRID)
	for i, key := range r.order {
		if key == datasetRID {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
}

// InvalidateAll drops every cached entry. Useful on shutdown / tests.
func (r *MarkingResolver) InvalidateAll() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make(map[string]markingCacheEntry, 64)
	r.order = r.order[:0]
}

func (r *MarkingResolver) computeInner(ctx context.Context, rid string, visiting map[string]struct{}) ([]models.EffectiveMarking, error) {
	if _, present := visiting[rid]; present {
		return nil, models.NewMarkingCycleError(rid)
	}
	visiting[rid] = struct{}{}
	defer delete(visiting, rid)

	if r.loader == nil {
		return nil, models.ErrMarkingResolverDisabled
	}
	direct, err := r.loader(ctx, rid)
	if err != nil {
		return nil, models.NewMarkingDatabaseError(err)
	}

	var parents []string
	if r.lineage != nil {
		parents, err = r.lineage.Upstream(ctx, rid)
		if err != nil {
			return nil, models.NewMarkingLineageError(rid, err)
		}
	}

	out := make([]models.EffectiveMarking, 0, len(direct)+len(parents))
	out = append(out, direct...)
	for _, parent := range parents {
		parentMarks, err := r.computeInner(ctx, parent, visiting)
		if err != nil {
			return nil, err
		}
		out = append(out, models.ReTagAsInheritedFrom(parent, parentMarks)...)
	}
	return models.DedupeMarkings(out), nil
}

func (r *MarkingResolver) lookup(rid string) ([]models.EffectiveMarking, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.cache[rid]
	if !ok {
		return nil, false
	}
	if r.now().After(entry.expiresAt) {
		delete(r.cache, rid)
		for i, key := range r.order {
			if key == rid {
				r.order = append(r.order[:i], r.order[i+1:]...)
				break
			}
		}
		return nil, false
	}
	out := make([]models.EffectiveMarking, len(entry.items))
	copy(out, entry.items)
	return out, true
}

func (r *MarkingResolver) store(rid string, items []models.EffectiveMarking) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.cache[rid]; !exists {
		if len(r.order) >= r.capacity {
			oldest := r.order[0]
			r.order = r.order[1:]
			delete(r.cache, oldest)
		}
		r.order = append(r.order, rid)
	}
	stored := make([]models.EffectiveMarking, len(items))
	copy(stored, items)
	r.cache[rid] = markingCacheEntry{
		items:     stored,
		expiresAt: r.now().Add(r.ttl),
	}
}

// MarkingResolverFromRepo builds a resolver whose direct-loader pulls rows
// from the supplied Store via ListDatasetMarkings, filtering to the direct
// subset. Convenience constructor for production wiring; tests should build
// the resolver explicitly with custom loaders.
func MarkingResolverFromRepo(store Store, lineage LineageClient) *MarkingResolver {
	loader := func(ctx context.Context, datasetRID string) ([]models.EffectiveMarking, error) {
		id, err := uuid.Parse(datasetRID)
		if err != nil {
			return nil, fmt.Errorf("marking resolver: invalid dataset rid %q: %w", datasetRID, err)
		}
		all, err := store.ListDatasetMarkings(ctx, id)
		if err != nil {
			return nil, err
		}
		// Keep only direct rows — inherited rows in the table are stale
		// projections that the resolver recomputes on the fly.
		filtered := make([]models.EffectiveMarking, 0, len(all))
		for _, m := range all {
			if m.IsDirect() {
				filtered = append(filtered, m)
			}
		}
		return filtered, nil
	}
	return NewMarkingResolver(loader, lineage)
}
