// Package functions ports `libs/ontology-kernel/src/handlers/functions.rs`
// 1:1: the 10 endpoints that drive function-package CRUD, validation,
// simulation, run-ledger reads + metrics under
// `/api/v1/ontology/functions/*` and the public-by-default authoring
// surface at `GET /api/v1/ontology/functions/authoring-surface`.
//
// Wire-format parity is byte-identical: response envelopes, status
// codes and error message bodies match Rust verbatim. The simulate
// endpoint composes the same evaluator + run-ledger writer as the
// Rust source via `domain.ExecuteInlineFunction` +
// `domain.RecordFunctionPackageRun`.
package functions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/handlers/objects"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// FunctionPackageLoader is the injectable package lookup boundary used by
// simulation tests. Production delegates to PostgreSQL via AppState.DB.
type FunctionPackageLoader interface {
	LoadFunctionPackage(ctx context.Context, state *ontologykernel.AppState, id uuid.UUID) (*models.FunctionPackage, error)
}

type defaultFunctionPackageLoader struct{}

func (defaultFunctionPackageLoader) LoadFunctionPackage(ctx context.Context, state *ontologykernel.AppState, id uuid.UUID) (*models.FunctionPackage, error) {
	return loadPackageByID(ctx, state, id)
}

// FunctionPackageExecutor is the runtime boundary used by simulation.
// Production delegates to domain.ExecuteInlineFunction, which reaches the
// real Python sidecar through AppState.PythonRuntime for Python packages.
type FunctionPackageExecutor interface {
	ExecuteInlineFunction(ctx context.Context, state *ontologykernel.AppState, claims *authmw.Claims, action *models.ActionType, target *domain.ObjectInstance, parameters map[string]json.RawMessage, resolved *domain.ResolvedInlineFunction, justification *string) (json.RawMessage, error)
}

type defaultFunctionPackageExecutor struct{}

func (defaultFunctionPackageExecutor) ExecuteInlineFunction(ctx context.Context, state *ontologykernel.AppState, claims *authmw.Claims, action *models.ActionType, target *domain.ObjectInstance, parameters map[string]json.RawMessage, resolved *domain.ResolvedInlineFunction, justification *string) (json.RawMessage, error) {
	return domain.ExecuteInlineFunction(ctx, state, claims, action, target, parameters, resolved, justification)
}

// FunctionPackageRunRecorder captures simulation/action ledger writes.
// Production uses domain.RecordFunctionPackageRun; tests inject fakes to
// verify success/failure rows without a PostgreSQL server.
type FunctionPackageRunRecorder interface {
	RecordFunctionPackageRun(ctx context.Context, state *ontologykernel.AppState, pkg models.FunctionPackageSummary, runCtx domain.FunctionPackageRunContext, startedAt time.Time, completedAt time.Time, durationMs int64, status string, errorMessage *string) error
}

type defaultFunctionPackageRunRecorder struct{}

func (defaultFunctionPackageRunRecorder) RecordFunctionPackageRun(ctx context.Context, state *ontologykernel.AppState, pkg models.FunctionPackageSummary, runCtx domain.FunctionPackageRunContext, startedAt time.Time, completedAt time.Time, durationMs int64, status string, errorMessage *string) error {
	return recordFunctionPackageRun(ctx, state, pkg, runCtx, startedAt, completedAt, durationMs, status, errorMessage)
}

type functionPackageSimulationDeps struct {
	Loader   FunctionPackageLoader
	Executor FunctionPackageExecutor
	Recorder FunctionPackageRunRecorder
}

func normalizeSimulationDeps(deps functionPackageSimulationDeps) functionPackageSimulationDeps {
	if deps.Loader == nil {
		deps.Loader = defaultFunctionPackageLoader{}
	}
	if deps.Executor == nil {
		deps.Executor = defaultFunctionPackageExecutor{}
	}
	if deps.Recorder == nil {
		deps.Recorder = defaultFunctionPackageRunRecorder{}
	}
	return deps
}

// Mount registers every functions-handler endpoint on the chi router
// under the same path / verb shape as `lib.rs::build_router::functions_routes`.
func Mount(r chi.Router, state *ontologykernel.AppState) {
	r.Get("/functions", ListFunctionPackages(state))
	r.Post("/functions", CreateFunctionPackage(state))
	r.Get("/functions/authoring-surface", GetFunctionAuthoringSurface())
	r.Get("/functions/{id}", GetFunctionPackage(state))
	r.Patch("/functions/{id}", UpdateFunctionPackage(state))
	r.Delete("/functions/{id}", DeleteFunctionPackage(state))
	r.Post("/functions/{id}/validate", ValidateFunctionPackage(state))
	r.Post("/functions/{id}/simulate", SimulateFunctionPackage(state))
	r.Get("/functions/{id}/runs", ListFunctionPackageRuns(state))
	r.Get("/functions/{id}/metrics", GetFunctionPackageMetrics(state))
}

// ── Endpoints (1:1 with the Rust pub async fn set) ──────────────────

// GetFunctionAuthoringSurface mirrors `pub async fn get_function_authoring_surface`.
// Static catalog — no DB / state access.
func GetFunctionAuthoringSurface() http.HandlerFunc {
	body := models.FunctionAuthoringSurfaceResponse{
		Templates:   builtInFunctionAuthoringTemplates(),
		SDKPackages: functionSDKPackages(),
		CLICommands: functionAuthoringCLICommands(),
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, body)
	}
}

// ListFunctionPackages mirrors `pub async fn list_function_packages`.
// Pagination is post-load (matches Rust); the SQL filter is text-
// identical to the Rust source.
func ListFunctionPackages(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := parseListFunctionPackagesQuery(r)
		page := defaultPage(q.Page)
		perPage := defaultPerPage(q.PerPage)
		search := strDeref(q.Search)
		runtime := strDeref(q.Runtime)

		packages, err := listFunctionPackages(r.Context(), state, runtime, search)
		if err != nil {
			dbError(w, "failed to list function packages: "+err.Error())
			return
		}

		// Stable order: name ASC, then version DESC (semver ordering),
		// then created_at DESC. Mirrors the Rust `sort_by` cascade.
		sortPackages(packages)

		total := int64(len(packages))
		offset := int((page - 1) * perPage)
		end := offset + int(perPage)
		if offset > len(packages) {
			offset = len(packages)
		}
		if end > len(packages) {
			end = len(packages)
		}
		writeJSON(w, http.StatusOK, models.ListFunctionPackagesResponse{
			Data:    packages[offset:end],
			Total:   total,
			Page:    page,
			PerPage: perPage,
		})
	}
}

// CreateFunctionPackage mirrors `pub async fn create_function_package`.
func CreateFunctionPackage(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		var body models.CreateFunctionPackageRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			invalid(w, "invalid request body")
			return
		}
		if strings.TrimSpace(body.Name) == "" {
			invalid(w, "function package name is required")
			return
		}
		displayName := body.Name
		if body.DisplayName != nil {
			displayName = *body.DisplayName
		}
		description := ""
		if body.Description != nil {
			description = *body.Description
		}
		entrypoint := defaultEntrypoint
		if body.Entrypoint != nil {
			entrypoint = *body.Entrypoint
		}
		version := models.DefaultFunctionPackageVersion
		if body.Version != nil {
			version = *body.Version
		}
		capabilities := models.DefaultFunctionCapabilities()
		if body.Capabilities != nil {
			capabilities = *body.Capabilities
		}
		if _, err := models.ParseFunctionPackageVersion(version); err != nil {
			invalid(w, err.Error())
			return
		}
		if err := validatePackageSource(body.Runtime, body.Source, entrypoint, capabilities); err != nil {
			invalid(w, err.Error())
			return
		}

		// Rust uses Uuid::now_v7() so package IDs sort by time —
		// drives stable ordering on the listing endpoint.
		pkgID, err := uuid.NewV7()
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to allocate function package id: %s", err), http.StatusInternalServerError)
			return
		}
		pkg := models.FunctionPackage{ID: pkgID, Name: strings.TrimSpace(body.Name), Version: version, DisplayName: displayName, Description: description, Runtime: body.Runtime, Source: body.Source, Entrypoint: entrypoint, Capabilities: capabilities, OwnerID: claims.Sub}
		out, err := createFunctionPackage(r.Context(), state, pkg)
		if err != nil {
			dbError(w, "failed to create function package: "+err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

// GetFunctionPackage mirrors `pub async fn get_function_package`.
func GetFunctionPackage(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathUUID(r, "id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		pkg, err := loadPackage(r, state, id)
		if err != nil {
			dbError(w, err.Error())
			return
		}
		if pkg == nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		writeJSON(w, http.StatusOK, pkg)
	}
}

// UpdateFunctionPackage mirrors `pub async fn update_function_package`.
func UpdateFunctionPackage(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathUUID(r, "id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		var body models.UpdateFunctionPackageRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			invalid(w, "invalid request body")
			return
		}
		existing, err := loadPackage(r, state, id)
		if err != nil {
			dbError(w, err.Error())
			return
		}
		if existing == nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		runtime := existing.Runtime
		if body.Runtime != nil {
			runtime = *body.Runtime
		}
		source := existing.Source
		if body.Source != nil {
			source = *body.Source
		}
		entrypoint := existing.Entrypoint
		if body.Entrypoint != nil {
			entrypoint = *body.Entrypoint
		}
		capabilities := existing.Capabilities
		if body.Capabilities != nil {
			capabilities = *body.Capabilities
		}
		if err := validatePackageSource(runtime, source, entrypoint, capabilities); err != nil {
			invalid(w, err.Error())
			return
		}

		existing.Runtime = runtime
		existing.Source = source
		existing.Entrypoint = entrypoint
		existing.Capabilities = capabilities
		out, err := updateFunctionPackage(r.Context(), state, *existing, body)
		if err != nil {
			dbError(w, "failed to update function package: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// DeleteFunctionPackage mirrors `pub async fn delete_function_package`.
func DeleteFunctionPackage(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathUUID(r, "id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		deleted, err := deleteFunctionPackage(r.Context(), state, id)
		if err != nil {
			dbError(w, "failed to delete function package: "+err.Error())
			return
		}
		if !deleted {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ValidateFunctionPackage mirrors `pub async fn validate_function_package`.
func ValidateFunctionPackage(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathUUID(r, "id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		var body models.ValidateFunctionPackageRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			invalid(w, "invalid request body")
			return
		}
		pkg, err := loadPackage(r, state, id)
		if err != nil {
			dbError(w, err.Error())
			return
		}
		if pkg == nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		preview := buildPreview(pkg, &body)
		writeJSON(w, http.StatusOK, models.ValidateFunctionPackageResponse{
			Valid:   true,
			Package: pkg.Summary(),
			Preview: preview,
			Errors:  []string{},
		})
	}
}

// SimulateFunctionPackage mirrors `pub async fn simulate_function_package`.
func SimulateFunctionPackage(state *ontologykernel.AppState) http.HandlerFunc {
	return SimulateFunctionPackageWithDeps(state, functionPackageSimulationDeps{})
}

func SimulateFunctionPackageWithDeps(state *ontologykernel.AppState, deps functionPackageSimulationDeps) http.HandlerFunc {
	deps = normalizeSimulationDeps(deps)
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errBody("missing claims"))
			return
		}
		id, err := pathUUID(r, "id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		var body models.SimulateFunctionPackageRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			invalid(w, "invalid request body")
			return
		}
		ctx := r.Context()
		pkg, err := deps.Loader.LoadFunctionPackage(ctx, state, id)
		if err != nil {
			dbError(w, err.Error())
			return
		}
		if pkg == nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}

		var target *domain.ObjectInstance
		if body.TargetObjectID != nil {
			obj, err := objects.LoadObjectInstance(ctx, state, claims, *body.TargetObjectID, storage.Strong())
			if err != nil {
				dbError(w, "failed to load target object: "+err.Error())
				return
			}
			if obj == nil {
				writeJSON(w, http.StatusNotFound, nil)
				return
			}
			if err := domain.EnsureObjectAccess(claims, obj); err != nil {
				writeJSON(w, http.StatusForbidden, errBody(err.Error()))
				return
			}
			target = obj
		}

		parameters, err := parseParameters(body.Parameters)
		if err != nil {
			invalid(w, err.Error())
			return
		}
		resolved, err := buildPackageInvocation(pkg)
		if err != nil {
			invalid(w, err.Error())
			return
		}
		action := syntheticAction(pkg, body.ObjectTypeID)
		paramKeys := make([]string, 0, len(parameters))
		for k := range parameters {
			paramKeys = append(paramKeys, k)
		}
		var targetID *uuid.UUID
		if target != nil {
			t := target.ID
			targetID = &t
		}
		preview, _ := json.Marshal(map[string]any{
			"package":          pkg.Summary(),
			"target_object_id": targetID,
			"parameter_keys":   paramKeys,
			"capabilities":     resolved.Capabilities,
		})

		startedAt := time.Now().UTC()
		startTimer := time.Now()
		outcome, execErr := deps.Executor.ExecuteInlineFunction(ctx, state, claims, action, target, parameters, resolved, body.Justification)
		completedAt := time.Now().UTC()
		durationMs := time.Since(startTimer).Milliseconds()

		runCtx := domain.FunctionPackageRunContext{
			InvocationKind: "simulation",
			ObjectTypeID:   &body.ObjectTypeID,
			TargetObjectID: targetID,
			ActorID:        claims.Sub,
		}
		summary := pkg.Summary()
		if execErr == nil {
			_ = deps.Recorder.RecordFunctionPackageRun(ctx, state, summary, runCtx,
				startedAt, completedAt, durationMs, "success", nil)
			writeJSON(w, http.StatusOK, models.SimulateFunctionPackageResponse{
				Package: summary,
				Preview: preview,
				Result:  outcome,
			})
			return
		}
		errMessage := execErr.Error()
		_ = deps.Recorder.RecordFunctionPackageRun(ctx, state, summary, runCtx,
			startedAt, completedAt, durationMs, "failure", &errMessage)
		// Missing Python runtime is a config-gated dependency; everything else is 500.
		if errors.Is(execErr, domain.ErrPythonRuntimeNotWired) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"error":   "python_runtime_not_wired",
				"detail":  errMessage,
				"package": summary,
				"preview": preview,
			})
			return
		}
		dbError(w, errMessage)
	}
}

// ListFunctionPackageRuns mirrors `pub async fn list_function_package_runs`.
func ListFunctionPackageRuns(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathUUID(r, "id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		pkg, err := loadPackage(r, state, id)
		if err != nil {
			dbError(w, err.Error())
			return
		}
		if pkg == nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		query := parseListRunsQuery(r)
		if err := validateRunFilters(strDeref(query.Status), strDeref(query.InvocationKind)); err != nil {
			invalid(w, err.Error())
			return
		}
		page := defaultPage(query.Page)
		perPage := defaultPerPage(query.PerPage)
		data, total, err := listFunctionPackageRuns(r.Context(), state, id, strDeref(query.Status), strDeref(query.InvocationKind), page, perPage)
		if err != nil {
			dbError(w, "failed to load function package runs: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, models.ListFunctionPackageRunsResponse{Data: data, Total: total, Page: page, PerPage: perPage})
	}
}

// GetFunctionPackageMetrics mirrors `pub async fn get_function_package_metrics`.
func GetFunctionPackageMetrics(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathUUID(r, "id")
		if err != nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		pkg, err := loadPackage(r, state, id)
		if err != nil {
			dbError(w, err.Error())
			return
		}
		if pkg == nil {
			writeJSON(w, http.StatusNotFound, nil)
			return
		}
		row, err := functionPackageMetrics(r.Context(), state, id)
		if err != nil {
			dbError(w, "failed to load function package metrics: "+err.Error())
			return
		}
		successRate := 0.0
		if row.TotalRuns > 0 {
			successRate = float64(row.SuccessfulRuns) / float64(row.TotalRuns)
		}
		writeJSON(w, http.StatusOK, models.FunctionPackageMetricsResponse{Package: pkg.Summary(), TotalRuns: row.TotalRuns, SuccessfulRuns: row.SuccessfulRuns, FailedRuns: row.FailedRuns, SimulationRuns: row.SimulationRuns, ActionRuns: row.ActionRuns, SuccessRate: successRate, AvgDurationMs: row.AvgDurationMs, P95DurationMs: row.P95DurationMs, MaxDurationMs: row.MaxDurationMs, LastRunAt: row.LastRunAt, LastSuccessAt: row.LastSuccessAt, LastFailureAt: row.LastFailureAt})
	}
}

// ── Helpers (1:1 with the Rust private fns) ─────────────────────────

const defaultEntrypoint = "handler"

func ensureEntrypoint(entrypoint string) error {
	if entrypoint == "default" || entrypoint == "handler" {
		return nil
	}
	return errors.New("entrypoint must be 'default' or 'handler'")
}

func validatePackageSource(runtime, source, entrypoint string, capabilities models.FunctionCapabilities) error {
	if err := ensureEntrypoint(entrypoint); err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"runtime": runtime, "source": source})
	cfg, err := domain.ParseInlineFunctionConfig(body)
	if err != nil {
		return err
	}
	if cfg == nil {
		return errors.New("runtime/source must define a supported inline function")
	}
	return domain.ValidateFunctionCapabilities(*cfg, capabilities, nil)
}

func loadPackage(r *http.Request, state *ontologykernel.AppState, id uuid.UUID) (*models.FunctionPackage, error) {
	return loadPackageByID(r.Context(), state, id)
}

func loadPackageByIDPG(ctx context.Context, state *ontologykernel.AppState, id uuid.UUID) (*models.FunctionPackage, error) {
	var row models.FunctionPackageRow
	err := state.DB.QueryRow(ctx, `
		SELECT id, name, version, display_name, description, runtime, source, entrypoint,
		       capabilities, owner_id, created_at, updated_at
		FROM ontology_function_packages WHERE id = $1`, id,
	).Scan(
		&row.ID, &row.Name, &row.Version, &row.DisplayName, &row.Description,
		&row.Runtime, &row.Source, &row.Entrypoint, &row.Capabilities,
		&row.OwnerID, &row.CreatedAt, &row.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load function package: %w", err)
	}
	pkg := row.IntoPackage()
	return &pkg, nil
}

func buildPreview(pkg *models.FunctionPackage, body *models.ValidateFunctionPackageRequest) json.RawMessage {
	paramKeys := []string{}
	if len(body.Parameters) > 0 {
		var asMap map[string]json.RawMessage
		if err := json.Unmarshal(body.Parameters, &asMap); err == nil {
			for k := range asMap {
				paramKeys = append(paramKeys, k)
			}
		}
	}
	out, _ := json.Marshal(map[string]any{
		"kind":             "function_package",
		"package":          pkg.Summary(),
		"object_type_id":   body.ObjectTypeID,
		"target_object_id": body.TargetObjectID,
		"justification":    body.Justification,
		"parameter_keys":   paramKeys,
		"source_length":    len(pkg.Source),
	})
	return out
}

func parseParameters(parameters json.RawMessage) (map[string]json.RawMessage, error) {
	if len(parameters) == 0 || string(parameters) == "null" {
		return map[string]json.RawMessage{}, nil
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(parameters, &asMap); err != nil {
		return nil, errors.New("parameters must be a JSON object")
	}
	if asMap == nil {
		return map[string]json.RawMessage{}, nil
	}
	return asMap, nil
}

func buildPackageInvocation(pkg *models.FunctionPackage) (*domain.ResolvedInlineFunction, error) {
	body, _ := json.Marshal(map[string]string{
		"runtime": pkg.Runtime,
		"source":  pkg.Source,
	})
	cfg, err := domain.ParseInlineFunctionConfig(body)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, errors.New("function package runtime is not supported")
	}
	summary := pkg.Summary()
	return &domain.ResolvedInlineFunction{
		Config:       *cfg,
		Capabilities: pkg.Capabilities,
		Package:      &summary,
	}, nil
}

func validateRunFilters(status, invocationKind string) error {
	if status != "" && status != "success" && status != "failure" {
		return errors.New("status filter must be 'success' or 'failure'")
	}
	if invocationKind != "" && invocationKind != "simulation" && invocationKind != "action" {
		return errors.New("invocation_kind filter must be 'simulation' or 'action'")
	}
	return nil
}

func syntheticAction(pkg *models.FunctionPackage, objectTypeID uuid.UUID) *models.ActionType {
	cfg, _ := json.Marshal(map[string]any{"function_package_id": pkg.ID})
	return &models.ActionType{
		ID:                   pkg.ID,
		Name:                 pkg.Name,
		DisplayName:          pkg.DisplayName,
		Description:          pkg.Description,
		ObjectTypeID:         objectTypeID,
		OperationKind:        "invoke_function",
		InputSchema:          []models.ActionInputField{},
		FormSchema:           models.ActionFormSchema{},
		Config:               cfg,
		ConfirmationRequired: false,
		PermissionKey:        nil,
		AuthorizationPolicy:  models.ActionAuthorizationPolicy{},
		OwnerID:              pkg.OwnerID,
		CreatedAt:            pkg.CreatedAt,
		UpdatedAt:            pkg.UpdatedAt,
	}
}

// sortPackages mirrors the Rust `packages.sort_by` cascade: name ASC,
// then version DESC (semver-ordered when both parse as semver,
// lexicographic DESC otherwise), then created_at DESC.
func sortPackages(packages []models.FunctionPackage) {
	for i := 1; i < len(packages); i++ {
		for j := i; j > 0; j-- {
			if comparePackages(packages[j], packages[j-1]) < 0 {
				packages[j], packages[j-1] = packages[j-1], packages[j]
			} else {
				break
			}
		}
	}
}

func comparePackages(a, b models.FunctionPackage) int {
	if cmp := strings.Compare(a.Name, b.Name); cmp != 0 {
		return cmp
	}
	// Version DESC.
	switch {
	case versionGreater(a.Version, b.Version):
		return -1
	case versionGreater(b.Version, a.Version):
		return 1
	}
	// created_at DESC.
	if a.CreatedAt.After(b.CreatedAt) {
		return -1
	}
	if b.CreatedAt.After(a.CreatedAt) {
		return 1
	}
	return 0
}

func versionGreater(a, b string) bool {
	aParsed, aErr := models.ParseFunctionPackageVersion(a)
	bParsed, bErr := models.ParseFunctionPackageVersion(b)
	if aErr == nil && bErr == nil {
		// Both parse as semver — compare numerically using the same
		// helper the runtime port uses.
		aMaj, aMin, aPat, aPre, _ := splitSemverForSort(aParsed)
		bMaj, bMin, bPat, bPre, _ := splitSemverForSort(bParsed)
		if aMaj != bMaj {
			return aMaj > bMaj
		}
		if aMin != bMin {
			return aMin > bMin
		}
		if aPat != bPat {
			return aPat > bPat
		}
		// Empty pre > non-empty pre (release > pre-release).
		if aPre == "" && bPre != "" {
			return true
		}
		if bPre == "" && aPre != "" {
			return false
		}
		return aPre > bPre
	}
	return a > b
}

func splitSemverForSort(v string) (int, int, int, string, bool) {
	core := v
	pre := ""
	if idx := strings.Index(v, "-"); idx >= 0 {
		core = v[:idx]
		pre = v[idx+1:]
	}
	if idx := strings.Index(core, "+"); idx >= 0 {
		core = core[:idx]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return 0, 0, 0, "", false
	}
	maj, _ := strconv.Atoi(parts[0])
	min, _ := strconv.Atoi(parts[1])
	pat, _ := strconv.Atoi(parts[2])
	return maj, min, pat, pre, true
}

// ── HTTP plumbing ───────────────────────────────────────────────────

func parseListFunctionPackagesQuery(r *http.Request) models.ListFunctionPackagesQuery {
	q := r.URL.Query()
	out := models.ListFunctionPackagesQuery{}
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
	if raw := q.Get("search"); raw != "" {
		out.Search = &raw
	}
	if raw := q.Get("runtime"); raw != "" {
		out.Runtime = &raw
	}
	return out
}

func parseListRunsQuery(r *http.Request) models.ListFunctionPackageRunsQuery {
	q := r.URL.Query()
	out := models.ListFunctionPackageRunsQuery{}
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
	if raw := q.Get("status"); raw != "" {
		out.Status = &raw
	}
	if raw := q.Get("invocation_kind"); raw != "" {
		out.InvocationKind = &raw
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
