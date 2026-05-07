// Package funnel ports `libs/ontology-kernel/src/handlers/funnel.rs`
// 1:1: the 10 endpoints that drive ontology funnel CRUD, run-ledger
// reads and health metrics under `/api/v1/ontology/funnel/*`.
//
// The Go port now drives the full source lifecycle and run lifecycle:
// source CRUD/health, run ledger reads, pipeline trigger, dataset
// preview, row validation, object writeback/upsert, revisions, and
// storage insights all route through real repositories/stores.
package funnel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
)

// Mount registers every funnel endpoint on the chi router under the
// same path / verb shape as `lib.rs::build_router::funnel_routes`.
func Mount(r chi.Router, state *ontologykernel.AppState) {
	r.Get("/funnel/health", GetFunnelHealth(state))
	r.Get("/funnel/sources", ListFunnelSources(state))
	r.Post("/funnel/sources", CreateFunnelSource(state))
	r.Get("/funnel/sources/{id}", GetFunnelSource(state))
	r.Patch("/funnel/sources/{id}", UpdateFunnelSource(state))
	r.Delete("/funnel/sources/{id}", DeleteFunnelSource(state))
	r.Get("/funnel/sources/{id}/health", GetFunnelSourceHealth(state))
	r.Post("/funnel/sources/{id}/run", TriggerFunnelRun(state))
	r.Get("/funnel/sources/{id}/runs", ListFunnelRuns(state))
	r.Get("/funnel/sources/{source_id}/runs/{run_id}", GetFunnelRun(state))
}

// ── Endpoints (1:1 with the Rust pub async fn set) ──────────────────

// ListFunnelSources mirrors `pub async fn list_funnel_sources`.
func ListFunnelSources(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		query := parseListSourcesQuery(r)
		page := defaultPage(query.Page)
		perPage := defaultPerPage(query.PerPage)
		offset := (page - 1) * perPage
		statusFilter := strDeref(query.Status)
		isAdmin := claims.HasRole("admin")

		data, err := listFunnelSources(r.Context(), state, domain.ListSourcesParams{
			ObjectTypeID: query.ObjectTypeID,
			StatusFilter: statusFilter,
			IsAdmin:      isAdmin,
			ActorID:      claims.Sub,
			Offset:       offset,
			Limit:        perPage,
		})
		if err != nil {
			dbError(w, "failed to list ontology funnel sources: "+err.Error())
			return
		}
		total, err := countFunnelSources(r.Context(), state, query.ObjectTypeID, statusFilter, isAdmin, claims.Sub)
		if err != nil {
			dbError(w, "failed to count ontology funnel sources: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, models.ListOntologyFunnelSourcesResponse{
			Data: data, Total: total, Page: page, PerPage: perPage,
		})
	}
}

// GetFunnelHealth mirrors `pub async fn get_funnel_health`. Builds
// the per-source health record + the aggregate dashboard envelope.
func GetFunnelHealth(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		query := parseListHealthQuery(r)
		staleAfterHours := models.NormalizeStaleAfterHours(query.StaleAfterHours)

		sources, err := listFunnelSourcesForHealth(r.Context(), state, domain.HealthSourcesParams{
			ObjectTypeID: query.ObjectTypeID,
			IsAdmin:      claims.HasRole("admin"),
			ActorID:      claims.Sub,
		})
		if err != nil {
			dbError(w, "failed to list ontology funnel sources for health: "+err.Error())
			return
		}

		health := make([]models.OntologyFunnelSourceHealth, 0, len(sources))
		for _, src := range sources {
			metrics, err := domain.LoadHealthMetrics(r.Context(), state.Stores.Actions, domain.TenantFromClaims(claims), src.ID)
			if err != nil {
				dbError(w, "failed to load ontology funnel health metrics: "+err.Error())
				return
			}
			health = append(health, BuildSourceHealth(src, metrics, staleAfterHours))
		}

		sort.SliceStable(health, func(i, j int) bool {
			rl, rr := funnelHealthSortRank(health[i].HealthStatus), funnelHealthSortRank(health[j].HealthStatus)
			if rl != rr {
				return rl < rr
			}
			// last_run_at DESC (None last).
			li, lj := health[i].LastRunAt, health[j].LastRunAt
			switch {
			case li != nil && lj == nil:
				return true
			case li == nil && lj != nil:
				return false
			case li != nil && lj != nil:
				return li.After(*lj)
			}
			return false
		})

		response := aggregateHealth(health, staleAfterHours)
		writeJSON(w, http.StatusOK, response)
	}
}

// GetFunnelSourceHealth mirrors `pub async fn get_funnel_source_health`.
func GetFunnelSourceHealth(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		id, err := pathUUID(r, "id")
		if err != nil {
			notFound(w, "ontology funnel source not found")
			return
		}
		src, err := loadFunnelSource(r.Context(), state, id)
		if err != nil {
			dbError(w, err.Error())
			return
		}
		if src == nil {
			notFound(w, "ontology funnel source not found")
			return
		}
		if err := ensureOwnerOrAdmin(src.OwnerID, claims); err != nil {
			forbidden(w, err.Error())
			return
		}
		query := parseGetHealthQuery(r)
		staleAfterHours := models.NormalizeStaleAfterHours(query.StaleAfterHours)
		metrics, err := domain.LoadHealthMetrics(r.Context(), state.Stores.Actions, domain.TenantFromClaims(claims), src.ID)
		if err != nil {
			dbError(w, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, models.OntologyFunnelSourceHealthResponse{
			StaleAfterHours: staleAfterHours,
			SourceHealth:    BuildSourceHealth(*src, metrics, staleAfterHours),
		})
	}
}

// CreateFunnelSource mirrors `pub async fn create_funnel_source`.
func CreateFunnelSource(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		var body models.CreateOntologyFunnelSourceRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			invalid(w, "invalid request body")
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			invalid(w, "name is required")
			return
		}
		ctx := r.Context()
		exists, err := funnelObjectTypeExists(ctx, state, body.ObjectTypeID)
		if err != nil {
			invalid(w, "object_type_id does not exist")
			return
		}
		if !exists {
			invalid(w, "object_type_id does not exist")
			return
		}
		ok, err = funnelDatasetExists(ctx, state, body.DatasetID)
		if err != nil || !ok {
			invalid(w, "dataset_id does not exist")
			return
		}
		if body.PipelineID != nil {
			ok, err := funnelPipelineExists(ctx, state, *body.PipelineID)
			if err != nil || !ok {
				invalid(w, "pipeline_id does not exist")
				return
			}
		}

		previewLimit := models.NormalizePreviewLimit(body.PreviewLimit)
		status := models.NormalizeFunnelStatus(body.Status)
		if err := validateSourceStatus(status); err != nil {
			invalid(w, err.Error())
			return
		}
		marking := models.NormalizeDefaultMarking(body.DefaultMarking)
		if err := domain.ValidateMarking(marking); err != nil {
			invalid(w, err.Error())
			return
		}
		mappings, _ := json.Marshal(propertyMappingsOrDefault(body.PropertyMappings))
		ctxTrigger := body.TriggerContext
		if len(ctxTrigger) == 0 {
			ctxTrigger = json.RawMessage(`{}`)
		}
		desc := ""
		if body.Description != nil {
			desc = *body.Description
		}
		// Rust handlers/funnel.rs:127 uses Uuid::now_v7() for the
		// new source id so listings sort by creation time.
		sourceID, err := uuid.NewV7()
		if err != nil {
			dbError(w, "failed to allocate funnel source id: "+err.Error())
			return
		}
		out, err := createFunnelSource(ctx, state, domain.CreateSourceInput{
			ID:               sourceID,
			Name:             strings.TrimSpace(body.Name),
			Description:      desc,
			ObjectTypeID:     body.ObjectTypeID,
			DatasetID:        body.DatasetID,
			PipelineID:       body.PipelineID,
			DatasetBranch:    body.DatasetBranch,
			DatasetVersion:   body.DatasetVersion,
			PreviewLimit:     previewLimit,
			DefaultMarking:   marking,
			Status:           status,
			PropertyMappings: mappings,
			TriggerContext:   ctxTrigger,
			OwnerID:          claims.Sub,
		})
		if err != nil {
			dbError(w, "failed to create ontology funnel source: "+err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

// GetFunnelSource mirrors `pub async fn get_funnel_source`.
func GetFunnelSource(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		id, err := pathUUID(r, "id")
		if err != nil {
			notFound(w, "ontology funnel source not found")
			return
		}
		src, err := loadFunnelSource(r.Context(), state, id)
		if err != nil {
			dbError(w, err.Error())
			return
		}
		if src == nil {
			notFound(w, "ontology funnel source not found")
			return
		}
		if err := ensureOwnerOrAdmin(src.OwnerID, claims); err != nil {
			forbidden(w, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, src)
	}
}

// UpdateFunnelSource mirrors `pub async fn update_funnel_source`.
func UpdateFunnelSource(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		id, err := pathUUID(r, "id")
		if err != nil {
			notFound(w, "ontology funnel source not found")
			return
		}
		var body models.UpdateOntologyFunnelSourceRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			invalid(w, "invalid request body")
			return
		}
		ctx := r.Context()
		existing, err := loadFunnelSource(ctx, state, id)
		if err != nil {
			dbError(w, err.Error())
			return
		}
		if existing == nil {
			notFound(w, "ontology funnel source not found")
			return
		}
		if err := ensureOwnerOrAdmin(existing.OwnerID, claims); err != nil {
			forbidden(w, err.Error())
			return
		}

		// Apply update with three-way Option<Option<T>> semantics on
		// pipeline_id / branch / version: the raw model field's
		// presence flag is honoured by the carrier struct unmarshaller.
		pipelineID := existing.PipelineID
		if body.PipelineID != nil {
			pipelineID = body.PipelineID.Value
		}
		branch := existing.DatasetBranch
		if body.DatasetBranch != nil {
			branch = body.DatasetBranch.Value
		}
		version := existing.DatasetVersion
		if body.DatasetVersion != nil {
			version = body.DatasetVersion.Value
		}
		if pipelineID != nil {
			ok, err := funnelPipelineExists(ctx, state, *pipelineID)
			if err != nil || !ok {
				invalid(w, "pipeline_id does not exist")
				return
			}
		}
		previewLimit := existing.PreviewLimit
		if body.PreviewLimit != nil {
			previewLimit = clampPreviewLimit(*body.PreviewLimit)
		}
		status := existing.Status
		if body.Status != nil {
			status = *body.Status
		}
		if err := validateSourceStatus(status); err != nil {
			invalid(w, err.Error())
			return
		}
		marking := existing.DefaultMarking
		if body.DefaultMarking != nil {
			marking = *body.DefaultMarking
		}
		if err := domain.ValidateMarking(marking); err != nil {
			invalid(w, err.Error())
			return
		}
		mappings, _ := json.Marshal(propertyMappingsOrExisting(body.PropertyMappings, existing.PropertyMappings))
		triggerContext := existing.TriggerContext
		if len(body.TriggerContext) > 0 {
			triggerContext = body.TriggerContext
		}
		var trimmedName *string
		if body.Name != nil {
			t := strings.TrimSpace(*body.Name)
			trimmedName = &t
		}

		out, err := updateFunnelSource(ctx, state, domain.UpdateSourceInput{
			ID:               id,
			Name:             trimmedName,
			Description:      body.Description,
			PipelineID:       pipelineID,
			DatasetBranch:    branch,
			DatasetVersion:   version,
			PreviewLimit:     previewLimit,
			DefaultMarking:   marking,
			Status:           status,
			PropertyMappings: mappings,
			TriggerContext:   triggerContext,
		})
		if err != nil {
			dbError(w, "failed to update ontology funnel source: "+err.Error())
			return
		}
		if out == nil {
			notFound(w, "ontology funnel source not found")
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// DeleteFunnelSource mirrors `pub async fn delete_funnel_source`.
func DeleteFunnelSource(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		id, err := pathUUID(r, "id")
		if err != nil {
			notFound(w, "ontology funnel source not found")
			return
		}
		src, err := loadFunnelSource(r.Context(), state, id)
		if err != nil {
			dbError(w, err.Error())
			return
		}
		if src == nil {
			notFound(w, "ontology funnel source not found")
			return
		}
		if err := ensureOwnerOrAdmin(src.OwnerID, claims); err != nil {
			forbidden(w, err.Error())
			return
		}
		ok2, err := deleteFunnelSource(r.Context(), state, id)
		if err != nil {
			dbError(w, "failed to delete ontology funnel source: "+err.Error())
			return
		}
		if !ok2 {
			notFound(w, "ontology funnel source not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// TriggerFunnelRun mirrors `pub async fn trigger_funnel_run`.
//
// The full Rust path runs `execute_source_run`, which:
//  1. Triggers the upstream pipeline via HTTP into pipeline-build-service,
//  2. Fetches the dataset preview from dataset-versioning-service,
//  3. Validates each row + finds an existing object id by primary key,
//  4. Upserts the object via writeback (apply_object_write +
//     append_object_revision).
//
// The Go port executes the full path: it appends the start event,
// triggers optional pipeline work, reads the dataset preview, validates
// rows, upserts objects through the writeback substrate, appends the
// terminal run event, and updates source last_run_at.
func TriggerFunnelRun(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		id, err := pathUUID(r, "id")
		if err != nil {
			notFound(w, "ontology funnel source not found")
			return
		}
		var body models.TriggerOntologyFunnelRunRequest
		// Tolerate an empty body — Rust's TriggerOntologyFunnelRunRequest
		// derives Default and `axum::Json` accepts an empty payload
		// when every field is `#[serde(default)]`-annotated.
		if err := decodeOptionalJSON(r, &body); err != nil {
			invalid(w, "invalid request body")
			return
		}
		ctx := r.Context()
		src, err := loadFunnelSource(ctx, state, id)
		if err != nil {
			dbError(w, err.Error())
			return
		}
		if src == nil {
			notFound(w, "ontology funnel source not found")
			return
		}
		if err := ensureOwnerOrAdmin(src.OwnerID, claims); err != nil {
			forbidden(w, err.Error())
			return
		}
		if src.Status == "paused" {
			invalid(w, "ontology funnel source is paused")
			return
		}

		// Rust handlers/funnel.rs:1100 uses Uuid::now_v7() so run IDs
		// sort by trigger time inside the same tenant.
		runID, err := uuid.NewV7()
		if err != nil {
			dbError(w, "failed to allocate funnel run id: "+err.Error())
			return
		}
		tenant := domain.TenantFromClaims(claims)
		triggerType := "manual"
		if body.DryRun {
			triggerType = "manual_dry_run"
		}
		details := json.RawMessage(`{"started":true}`)
		if err := domain.CreateRun(ctx, state.Stores.Actions, tenant, domain.CreateRunInput{
			ID:           runID,
			SourceID:     src.ID,
			ObjectTypeID: src.ObjectTypeID,
			DatasetID:    src.DatasetID,
			PipelineID:   src.PipelineID,
			TriggerType:  triggerType,
			StartedBy:    claims.Sub,
			Details:      details,
		}); err != nil {
			dbError(w, "failed to create ontology funnel run: "+err.Error())
			return
		}

		// Real execute_source_run path: trigger pipeline, fetch
		// dataset preview, validate + upsert each row through the
		// writeback substrate, append the terminal event.
		outcome, runErr := executeSourceRun(ctx, state, claims, src, &body)
		if runErr != nil {
			_ = domain.FailRun(ctx, state.Stores.Actions, tenant, src.ID, runID, claims.Sub, runErr.Error())
			dbError(w, runErr.Error())
			return
		}
		finishedAt := time.Now().UTC()
		_ = domain.CompleteRun(ctx, state.Stores.Actions, tenant, claims.Sub, domain.CompleteRunInput{
			ID:            runID,
			SourceID:      src.ID,
			PipelineRunID: outcome.pipelineRunID,
			Status:        outcome.status,
			RowsRead:      outcome.rowsRead,
			InsertedCount: outcome.insertedCount,
			UpdatedCount:  outcome.updatedCount,
			SkippedCount:  outcome.skippedCount,
			ErrorCount:    outcome.errorCount,
			Details:       outcome.details,
			ErrorMessage:  outcome.errorMessage,
			FinishedAt:    finishedAt,
		})
		_ = markFunnelSourceRan(ctx, state, src.ID, finishedAt)

		// Reload the freshly-completed run so the response carries
		// the canonical ledger row (mirrors the Rust impl's tail).
		run, err := domain.LoadRunForSource(ctx, state.Stores.Actions, tenant, src.ID, runID)
		if err != nil {
			dbError(w, "failed to reload ontology funnel run: "+err.Error())
			return
		}
		if run == nil {
			dbError(w, "ontology funnel run completed but could not be reloaded")
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

// ListFunnelRuns mirrors `pub async fn list_funnel_runs`.
func ListFunnelRuns(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		id, err := pathUUID(r, "id")
		if err != nil {
			notFound(w, "ontology funnel source not found")
			return
		}
		ctx := r.Context()
		src, err := loadFunnelSource(ctx, state, id)
		if err != nil {
			dbError(w, err.Error())
			return
		}
		if src == nil {
			notFound(w, "ontology funnel source not found")
			return
		}
		if err := ensureOwnerOrAdmin(src.OwnerID, claims); err != nil {
			forbidden(w, err.Error())
			return
		}
		query := parseListRunsQuery(r)
		page := defaultPage(query.Page)
		perPage := defaultPerPage(query.PerPage)
		offset := (page - 1) * perPage
		tenant := domain.TenantFromClaims(claims)
		total, err := domain.CountRunsForSource(ctx, state.Stores.Actions, tenant, id)
		if err != nil {
			dbError(w, "failed to count ontology funnel runs: "+err.Error())
			return
		}
		data, err := domain.ListRunsForSource(ctx, state.Stores.Actions, tenant, id, offset, perPage)
		if err != nil {
			dbError(w, "failed to list ontology funnel runs: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, models.ListOntologyFunnelRunsResponse{
			Data: data, Total: total, Page: page, PerPage: perPage,
		})
	}
}

// GetFunnelRun mirrors `pub async fn get_funnel_run`.
func GetFunnelRun(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		sourceID, err := pathUUID(r, "source_id")
		if err != nil {
			notFound(w, "ontology funnel source not found")
			return
		}
		runID, err := pathUUID(r, "run_id")
		if err != nil {
			notFound(w, "ontology funnel run not found")
			return
		}
		ctx := r.Context()
		src, err := loadFunnelSource(ctx, state, sourceID)
		if err != nil {
			dbError(w, err.Error())
			return
		}
		if src == nil {
			notFound(w, "ontology funnel source not found")
			return
		}
		if err := ensureOwnerOrAdmin(src.OwnerID, claims); err != nil {
			forbidden(w, err.Error())
			return
		}
		run, err := domain.LoadRunForSource(ctx, state.Stores.Actions, domain.TenantFromClaims(claims), sourceID, runID)
		if err != nil {
			dbError(w, "failed to load ontology funnel run: "+err.Error())
			return
		}
		if run == nil {
			notFound(w, "ontology funnel run not found")
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

// ── Source health builder + sort + aggregator (1:1 with Rust private fns) ──

// BuildSourceHealth mirrors `fn build_source_health`. Exposed for
// the Rust unit-test parity tests.
func BuildSourceHealth(
	source models.OntologyFunnelSource,
	metrics models.OntologyFunnelHealthMetricsRow,
	staleAfterHours int64,
) models.OntologyFunnelSourceHealth {
	successRate := 0.0
	if metrics.TotalRuns > 0 {
		successRate = float64(metrics.SuccessfulRuns) / float64(metrics.TotalRuns)
	}
	hours := staleAfterHours
	if hours < 1 {
		hours = 1
	}
	staleCutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)

	healthStatus, healthReason := "", ""
	switch {
	case source.Status == "paused":
		healthStatus = "paused"
		healthReason = "source is paused and will not ingest new batch updates"
	case metrics.TotalRuns == 0:
		healthStatus = "never_run"
		healthReason = "source has not executed any funnel run yet"
	case metrics.LastRunAt != nil && metrics.LastRunAt.Before(staleCutoff):
		healthStatus = "stale"
		healthReason = fmt.Sprintf("no funnel run has completed within the last %d hour(s)", staleAfterHours)
	default:
		latest := ""
		if metrics.LatestRunStatus != nil {
			latest = *metrics.LatestRunStatus
		}
		switch latest {
		case "failed":
			healthStatus = "failing"
			healthReason = "latest funnel run failed before completing"
		case "completed_with_errors", "dry_run_with_errors":
			healthStatus = "degraded"
			healthReason = "latest funnel run completed with row-level or validation errors"
		case "running":
			healthStatus = "degraded"
			healthReason = "a funnel run is currently in progress"
		case "completed", "dry_run":
			healthStatus = "healthy"
			healthReason = "latest funnel run completed successfully"
		case "":
			healthStatus = "never_run"
			healthReason = "source has no observable run history"
		default:
			healthStatus = "degraded"
			healthReason = fmt.Sprintf("latest funnel run is in status '%s'", latest)
		}
	}

	return models.OntologyFunnelSourceHealth{
		Source:          source,
		HealthStatus:    healthStatus,
		HealthReason:    healthReason,
		TotalRuns:       metrics.TotalRuns,
		SuccessfulRuns:  metrics.SuccessfulRuns,
		FailedRuns:      metrics.FailedRuns,
		WarningRuns:     metrics.WarningRuns,
		SuccessRate:     successRate,
		AvgDurationMs:   metrics.AvgDurationMs,
		P95DurationMs:   metrics.P95DurationMs,
		MaxDurationMs:   metrics.MaxDurationMs,
		LatestRunStatus: metrics.LatestRunStatus,
		LastRunAt:       metrics.LastRunAt,
		LastSuccessAt:   metrics.LastSuccessAt,
		LastFailureAt:   metrics.LastFailureAt,
		LastWarningAt:   metrics.LastWarningAt,
		RowsRead:        metrics.RowsRead,
		InsertedCount:   metrics.InsertedCount,
		UpdatedCount:    metrics.UpdatedCount,
		SkippedCount:    metrics.SkippedCount,
		ErrorCount:      metrics.ErrorCount,
	}
}

func funnelHealthSortRank(status string) int {
	switch status {
	case "failing":
		return 0
	case "degraded":
		return 1
	case "stale":
		return 2
	case "never_run":
		return 3
	case "paused":
		return 4
	case "healthy":
		return 5
	default:
		return 6
	}
}

func aggregateHealth(sources []models.OntologyFunnelSourceHealth, staleAfterHours int64) models.OntologyFunnelHealthResponse {
	totalSources := int64(len(sources))
	out := models.OntologyFunnelHealthResponse{
		StaleAfterHours: staleAfterHours,
		TotalSources:    totalSources,
		Sources:         sources,
	}
	for _, s := range sources {
		if s.Source.Status == "active" {
			out.ActiveSources++
		}
		switch s.HealthStatus {
		case "paused":
			out.PausedSources++
		case "healthy":
			out.HealthySources++
		case "degraded":
			out.DegradedSources++
		case "failing":
			out.FailingSources++
		case "stale":
			out.StaleSources++
		case "never_run":
			out.NeverRunSources++
		}
		out.TotalRuns += s.TotalRuns
		out.SuccessfulRuns += s.SuccessfulRuns
		out.FailedRuns += s.FailedRuns
		out.WarningRuns += s.WarningRuns
		out.RowsRead += s.RowsRead
		out.InsertedCount += s.InsertedCount
		out.UpdatedCount += s.UpdatedCount
		out.SkippedCount += s.SkippedCount
		out.ErrorCount += s.ErrorCount
		if s.LastRunAt != nil && (out.LastRunAt == nil || s.LastRunAt.After(*out.LastRunAt)) {
			tt := *s.LastRunAt
			out.LastRunAt = &tt
		}
	}
	if out.TotalRuns > 0 {
		out.SuccessRate = float64(out.SuccessfulRuns) / float64(out.TotalRuns)
	}
	return out
}

// ── Helpers (1:1 with Rust private fns) ─────────────────────────────

func ensureOwnerOrAdmin(ownerID uuid.UUID, claims *authmw.Claims) error {
	if claims.HasRole("admin") || ownerID == claims.Sub {
		return nil
	}
	return errors.New("forbidden: only the owner can manage this ontology funnel source")
}

func validateSourceStatus(status string) error {
	switch strings.TrimSpace(status) {
	case "active", "paused":
		return nil
	default:
		return errors.New("status must be 'active' or 'paused'")
	}
}

func clampPreviewLimit(v int32) int32 {
	if v < 1 {
		return 1
	}
	if v > 1000 {
		return 1000
	}
	return v
}

func propertyMappingsOrDefault(p *[]models.OntologyFunnelPropertyMapping) []models.OntologyFunnelPropertyMapping {
	if p == nil {
		return []models.OntologyFunnelPropertyMapping{}
	}
	return *p
}

func propertyMappingsOrExisting(
	p *[]models.OntologyFunnelPropertyMapping,
	existing []models.OntologyFunnelPropertyMapping,
) []models.OntologyFunnelPropertyMapping {
	if p == nil {
		return existing
	}
	return *p
}

// ── HTTP plumbing ───────────────────────────────────────────────────

func parseListSourcesQuery(r *http.Request) models.ListOntologyFunnelSourcesQuery {
	q := r.URL.Query()
	out := models.ListOntologyFunnelSourcesQuery{}
	if raw := q.Get("object_type_id"); raw != "" {
		if id, err := uuid.Parse(raw); err == nil {
			out.ObjectTypeID = &id
		}
	}
	if raw := q.Get("status"); raw != "" {
		out.Status = &raw
	}
	if raw := q.Get("page"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			out.Page = &v
		}
	}
	if raw := q.Get("per_page"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			out.PerPage = &v
		}
	}
	return out
}

func parseListHealthQuery(r *http.Request) models.ListOntologyFunnelHealthQuery {
	q := r.URL.Query()
	out := models.ListOntologyFunnelHealthQuery{}
	if raw := q.Get("object_type_id"); raw != "" {
		if id, err := uuid.Parse(raw); err == nil {
			out.ObjectTypeID = &id
		}
	}
	if raw := q.Get("stale_after_hours"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			out.StaleAfterHours = &v
		}
	}
	return out
}

func parseGetHealthQuery(r *http.Request) models.GetOntologyFunnelSourceHealthQuery {
	q := r.URL.Query()
	out := models.GetOntologyFunnelSourceHealthQuery{}
	if raw := q.Get("stale_after_hours"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			out.StaleAfterHours = &v
		}
	}
	return out
}

func parseListRunsQuery(r *http.Request) models.ListOntologyFunnelRunsQuery {
	q := r.URL.Query()
	out := models.ListOntologyFunnelRunsQuery{}
	if raw := q.Get("page"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			out.Page = &v
		}
	}
	if raw := q.Get("per_page"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			out.PerPage = &v
		}
	}
	return out
}

func defaultPage(p *int64) int64 {
	if p == nil || *p < 1 {
		return 1
	}
	return *p
}

func defaultPerPage(p *int64) int64 {
	if p == nil {
		return 20
	}
	if *p < 1 {
		return 1
	}
	if *p > 100 {
		return 100
	}
	return *p
}

func strDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func pathUUID(r *http.Request, key string) (uuid.UUID, error) {
	raw := chi.URLParam(r, key)
	if raw == "" {
		return uuid.Nil, errors.New("missing path parameter " + key)
	}
	return uuid.Parse(strings.TrimSpace(raw))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

func errBody(msg string) map[string]string { return map[string]string{"error": msg} }

func invalid(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, errBody(msg))
}

func dbError(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusInternalServerError, errBody(msg))
}

func notFound(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusNotFound, errBody(msg))
}

func forbidden(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusForbidden, errBody(msg))
}

// decodeOptionalJSON tolerates an empty body — Rust's
// `axum::Json<TriggerOntologyFunnelRunRequest>` accepts an empty
// payload because every field is `#[serde(default)]`-annotated.
func decodeOptionalJSON(r *http.Request, dst any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

// keep io + context imports stable as the runtime grows; the
// trigger handler reaches into ctx via r.Context() already.
var _ context.Context = context.Background()
var _ = io.EOF
