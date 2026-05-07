package functions

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

const (
	functionPackageDefinitionKind = storage.DefinitionKind("function_package")
	functionPackageRunKind        = storage.ReadModelKind("function_package_run")
)

func listFunctionPackages(ctx context.Context, state *ontologykernel.AppState, runtimeFilter, search string) ([]models.FunctionPackage, error) {
	if state.DB != nil {
		rows, err := state.DB.Query(ctx, `
			SELECT id, name, version, display_name, description, runtime, source, entrypoint,
			       capabilities, owner_id, created_at, updated_at
			FROM ontology_function_packages
			WHERE ($1 = '' OR runtime = $1)
			  AND ($2 = '' OR name ILIKE '%' || $2 || '%' OR display_name ILIKE '%' || $2 || '%')
			ORDER BY name ASC, created_at DESC`, runtimeFilter, search)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []models.FunctionPackage{}
		for rows.Next() {
			var row models.FunctionPackageRow
			if err := rows.Scan(&row.ID, &row.Name, &row.Version, &row.DisplayName, &row.Description, &row.Runtime, &row.Source, &row.Entrypoint, &row.Capabilities, &row.OwnerID, &row.CreatedAt, &row.UpdatedAt); err != nil {
				return nil, err
			}
			out = append(out, row.IntoPackage())
		}
		return out, rows.Err()
	}
	page, err := state.Stores.Definitions.List(ctx, storage.DefinitionQuery{Kind: functionPackageDefinitionKind, Page: storage.Page{Size: 10_000}}, storage.Strong())
	if err != nil {
		return nil, err
	}
	search = strings.ToLower(search)
	out := make([]models.FunctionPackage, 0, len(page.Items))
	for _, rec := range page.Items {
		pkg, err := functionPackageFromRecord(rec)
		if err != nil {
			return nil, err
		}
		if runtimeFilter != "" && pkg.Runtime != runtimeFilter {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(pkg.Name), search) && !strings.Contains(strings.ToLower(pkg.DisplayName), search) {
			continue
		}
		out = append(out, *pkg)
	}
	return out, nil
}

func createFunctionPackage(ctx context.Context, state *ontologykernel.AppState, pkg models.FunctionPackage) (*models.FunctionPackage, error) {
	if state.DB != nil {
		caps, _ := json.Marshal(pkg.Capabilities)
		var row models.FunctionPackageRow
		err := state.DB.QueryRow(ctx, `
			INSERT INTO ontology_function_packages (
				id, name, version, display_name, description, runtime, source, entrypoint, capabilities, owner_id
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10)
			RETURNING id, name, version, display_name, description, runtime, source, entrypoint,
			          capabilities, owner_id, created_at, updated_at`, pkg.ID, pkg.Name, pkg.Version, pkg.DisplayName, pkg.Description, pkg.Runtime, pkg.Source, pkg.Entrypoint, caps, pkg.OwnerID).Scan(&row.ID, &row.Name, &row.Version, &row.DisplayName, &row.Description, &row.Runtime, &row.Source, &row.Entrypoint, &row.Capabilities, &row.OwnerID, &row.CreatedAt, &row.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out := row.IntoPackage()
		return &out, nil
	}
	now := time.Now().UTC()
	pkg.CreatedAt = now
	pkg.UpdatedAt = now
	if err := putFunctionPackage(ctx, state, pkg); err != nil {
		return nil, err
	}
	return &pkg, nil
}

func updateFunctionPackage(ctx context.Context, state *ontologykernel.AppState, existing models.FunctionPackage, body models.UpdateFunctionPackageRequest) (*models.FunctionPackage, error) {
	if state.DB != nil {
		caps, _ := json.Marshal(existing.Capabilities)
		var row models.FunctionPackageRow
		err := state.DB.QueryRow(ctx, `
			UPDATE ontology_function_packages
			SET display_name = COALESCE($2, display_name),
			    description  = COALESCE($3, description),
			    runtime      = $4,
			    source       = $5,
			    entrypoint   = $6,
			    capabilities = $7::jsonb,
			    updated_at   = NOW()
			WHERE id = $1
			RETURNING id, name, version, display_name, description, runtime, source, entrypoint,
			          capabilities, owner_id, created_at, updated_at`, existing.ID, body.DisplayName, body.Description, existing.Runtime, existing.Source, existing.Entrypoint, caps).Scan(&row.ID, &row.Name, &row.Version, &row.DisplayName, &row.Description, &row.Runtime, &row.Source, &row.Entrypoint, &row.Capabilities, &row.OwnerID, &row.CreatedAt, &row.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out := row.IntoPackage()
		return &out, nil
	}
	if body.DisplayName != nil {
		existing.DisplayName = *body.DisplayName
	}
	if body.Description != nil {
		existing.Description = *body.Description
	}
	existing.UpdatedAt = time.Now().UTC()
	if err := putFunctionPackage(ctx, state, existing); err != nil {
		return nil, err
	}
	return &existing, nil
}

func deleteFunctionPackage(ctx context.Context, state *ontologykernel.AppState, id uuid.UUID) (bool, error) {
	if state.DB != nil {
		ct, err := state.DB.Exec(ctx, "DELETE FROM ontology_function_packages WHERE id = $1", id)
		if err != nil {
			return false, err
		}
		return ct.RowsAffected() > 0, nil
	}
	return state.Stores.Definitions.Delete(ctx, functionPackageDefinitionKind, storage.DefinitionId(id.String()))
}

func loadPackageByID(ctx context.Context, state *ontologykernel.AppState, id uuid.UUID) (*models.FunctionPackage, error) {
	if state.DB != nil {
		return loadPackageByIDPG(ctx, state, id)
	}
	rec, err := state.Stores.Definitions.Get(ctx, functionPackageDefinitionKind, storage.DefinitionId(id.String()), storage.Strong())
	if err != nil || rec == nil {
		return nil, err
	}
	return functionPackageFromRecord(*rec)
}

func putFunctionPackage(ctx context.Context, state *ontologykernel.AppState, pkg models.FunctionPackage) error {
	payload, err := json.Marshal(pkg)
	if err != nil {
		return err
	}
	owner := pkg.OwnerID.String()
	created := pkg.CreatedAt.UnixMilli()
	updated := pkg.UpdatedAt.UnixMilli()
	_, err = state.Stores.Definitions.Put(ctx, storage.DefinitionRecord{Kind: functionPackageDefinitionKind, ID: storage.DefinitionId(pkg.ID.String()), OwnerID: &owner, Payload: payload, CreatedAtMs: &created, UpdatedAtMs: &updated}, nil)
	return err
}

func functionPackageFromRecord(rec storage.DefinitionRecord) (*models.FunctionPackage, error) {
	var pkg models.FunctionPackage
	if err := json.Unmarshal(rec.Payload, &pkg); err != nil {
		return nil, err
	}
	return &pkg, nil
}

func recordFunctionPackageRun(ctx context.Context, state *ontologykernel.AppState, pkg models.FunctionPackageSummary, runCtx domain.FunctionPackageRunContext, startedAt time.Time, completedAt time.Time, durationMs int64, status string, errorMessage *string) error {
	if state.DB != nil {
		return domain.RecordFunctionPackageRun(ctx, state.DB, pkg, runCtx, startedAt, completedAt, durationMs, status, errorMessage)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	if durationMs < 0 {
		durationMs = 0
	}
	run := models.FunctionPackageRun{ID: id, FunctionPackageID: pkg.ID, FunctionPackageName: pkg.Name, FunctionPackageVersion: pkg.Version, Runtime: pkg.Runtime, Status: status, InvocationKind: runCtx.InvocationKind, ActionID: runCtx.ActionID, ActionName: runCtx.ActionName, ObjectTypeID: runCtx.ObjectTypeID, TargetObjectID: runCtx.TargetObjectID, ActorID: runCtx.ActorID, DurationMs: durationMs, ErrorMessage: errorMessage, StartedAt: startedAt, CompletedAt: completedAt}
	payload, err := json.Marshal(run)
	if err != nil {
		return err
	}
	parent := storage.ReadModelId(pkg.ID.String())
	_, err = state.Stores.ReadModels.Put(ctx, storage.ReadModelRecord{Kind: functionPackageRunKind, Tenant: storage.TenantId("default"), ID: storage.ReadModelId(id.String()), ParentID: &parent, Payload: payload, Version: uint64(completedAt.UnixMilli()), UpdatedAtMs: completedAt.UnixMilli()})
	return err
}

func listFunctionPackageRuns(ctx context.Context, state *ontologykernel.AppState, packageID uuid.UUID, status, invocationKind string, page, perPage int64) ([]models.FunctionPackageRun, int64, error) {
	if state.DB != nil {
		return listFunctionPackageRunsPG(ctx, state, packageID, status, invocationKind, page, perPage)
	}
	parent := storage.ReadModelId(packageID.String())
	result, err := state.Stores.ReadModels.List(ctx, storage.ReadModelQuery{Kind: functionPackageRunKind, Tenant: storage.TenantId("default"), ParentID: &parent, Page: storage.Page{Size: 10_000}}, storage.Strong())
	if err != nil {
		return nil, 0, err
	}
	runs := make([]models.FunctionPackageRun, 0, len(result.Items))
	for _, rec := range result.Items {
		var run models.FunctionPackageRun
		if err := json.Unmarshal(rec.Payload, &run); err != nil {
			return nil, 0, err
		}
		if status != "" && run.Status != status {
			continue
		}
		if invocationKind != "" && run.InvocationKind != invocationKind {
			continue
		}
		runs = append(runs, run)
	}
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].CompletedAt.After(runs[j].CompletedAt) })
	total := int64(len(runs))
	offset := (page - 1) * perPage
	if offset < 0 {
		offset = 0
	}
	if int(offset) > len(runs) {
		return []models.FunctionPackageRun{}, total, nil
	}
	end := int(offset + perPage)
	if end > len(runs) {
		end = len(runs)
	}
	return runs[int(offset):end], total, nil
}

func functionPackageMetrics(ctx context.Context, state *ontologykernel.AppState, packageID uuid.UUID) (models.FunctionPackageMetricsRow, error) {
	if state.DB != nil {
		return functionPackageMetricsPG(ctx, state, packageID)
	}
	runs, _, err := listFunctionPackageRuns(ctx, state, packageID, "", "", 1, 10_000)
	if err != nil {
		return models.FunctionPackageMetricsRow{}, err
	}
	row := models.FunctionPackageMetricsRow{TotalRuns: int64(len(runs))}
	durations := make([]float64, 0, len(runs))
	for _, run := range runs {
		durations = append(durations, float64(run.DurationMs))
		switch run.Status {
		case "success":
			row.SuccessfulRuns++
			if row.LastSuccessAt == nil || run.CompletedAt.After(*row.LastSuccessAt) {
				t := run.CompletedAt
				row.LastSuccessAt = &t
			}
		case "failure":
			row.FailedRuns++
			if row.LastFailureAt == nil || run.CompletedAt.After(*row.LastFailureAt) {
				t := run.CompletedAt
				row.LastFailureAt = &t
			}
		}
		if run.InvocationKind == "simulation" {
			row.SimulationRuns++
		}
		if run.InvocationKind == "action" {
			row.ActionRuns++
		}
		if row.LastRunAt == nil || run.CompletedAt.After(*row.LastRunAt) {
			t := run.CompletedAt
			row.LastRunAt = &t
		}
	}
	if len(durations) > 0 {
		sort.Float64s(durations)
		var sum float64
		for _, d := range durations {
			sum += d
		}
		avg := sum / float64(len(durations))
		row.AvgDurationMs = &avg
		max := int64(durations[len(durations)-1])
		row.MaxDurationMs = &max
		p95 := percentileCont(durations, 0.95)
		row.P95DurationMs = &p95
	}
	return row, nil
}

func percentileCont(sorted []float64, percentile float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := percentile * float64(len(sorted)-1)
	lower := int(rank)
	upper := lower
	if rank > float64(lower) {
		upper++
	}
	return sorted[lower] + (sorted[upper]-sorted[lower])*(rank-float64(lower))
}

func listFunctionPackageRunsPG(ctx context.Context, state *ontologykernel.AppState, packageID uuid.UUID, status, invocationKind string, page, perPage int64) ([]models.FunctionPackageRun, int64, error) {
	var total int64
	if err := state.DB.QueryRow(ctx, `
		SELECT COUNT(*) FROM ontology_function_package_runs
		WHERE function_package_id = $1
		  AND ($2 = '' OR status = $2)
		  AND ($3 = '' OR invocation_kind = $3)`, packageID, status, invocationKind).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * perPage
	rows, err := state.DB.Query(ctx, `
		SELECT id, function_package_id, function_package_name, function_package_version,
		       runtime, status, invocation_kind, action_id, action_name, object_type_id,
		       target_object_id, actor_id, duration_ms, error_message, started_at, completed_at
		FROM ontology_function_package_runs
		WHERE function_package_id = $1
		  AND ($2 = '' OR status = $2)
		  AND ($3 = '' OR invocation_kind = $3)
		ORDER BY completed_at DESC
		OFFSET $4 LIMIT $5`, packageID, status, invocationKind, offset, perPage)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []models.FunctionPackageRun{}
	for rows.Next() {
		var run models.FunctionPackageRun
		if err := rows.Scan(&run.ID, &run.FunctionPackageID, &run.FunctionPackageName, &run.FunctionPackageVersion, &run.Runtime, &run.Status, &run.InvocationKind, &run.ActionID, &run.ActionName, &run.ObjectTypeID, &run.TargetObjectID, &run.ActorID, &run.DurationMs, &run.ErrorMessage, &run.StartedAt, &run.CompletedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, run)
	}
	return out, total, rows.Err()
}

func functionPackageMetricsPG(ctx context.Context, state *ontologykernel.AppState, packageID uuid.UUID) (models.FunctionPackageMetricsRow, error) {
	var row models.FunctionPackageMetricsRow
	err := state.DB.QueryRow(ctx, `
		SELECT
		    COUNT(*)::bigint AS total_runs,
		    COUNT(*) FILTER (WHERE status = 'success')::bigint AS successful_runs,
		    COUNT(*) FILTER (WHERE status = 'failure')::bigint AS failed_runs,
		    COUNT(*) FILTER (WHERE invocation_kind = 'simulation')::bigint AS simulation_runs,
		    COUNT(*) FILTER (WHERE invocation_kind = 'action')::bigint AS action_runs,
		    AVG(duration_ms)::double precision AS avg_duration_ms,
		    percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms)::double precision AS p95_duration_ms,
		    MAX(duration_ms)::bigint AS max_duration_ms,
		    MAX(completed_at) AS last_run_at,
		    MAX(completed_at) FILTER (WHERE status = 'success') AS last_success_at,
		    MAX(completed_at) FILTER (WHERE status = 'failure') AS last_failure_at
		FROM ontology_function_package_runs
		WHERE function_package_id = $1`, packageID).Scan(&row.TotalRuns, &row.SuccessfulRuns, &row.FailedRuns, &row.SimulationRuns, &row.ActionRuns, &row.AvgDurationMs, &row.P95DurationMs, &row.MaxDurationMs, &row.LastRunAt, &row.LastSuccessAt, &row.LastFailureAt)
	return row, err
}
