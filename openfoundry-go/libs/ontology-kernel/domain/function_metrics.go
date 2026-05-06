// Function-package run recorder. Mirrors
// `libs/ontology-kernel/src/domain/function_metrics.rs` 1:1 — same
// SQL shape (16 columns, same order, same parameter binding) and same
// error message format. The id is generated server-side via
// `uuid.NewV7` to match the Rust `Uuid::now_v7` call site.
//
// The Rust source consumes `&AppState` to reach the `sqlx` pool. The
// Go port takes `*pgxpool.Pool` directly so callers in any service can
// use the helper without a kernel-specific app state — handlers thread
// their own pool in.
package domain

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
)

// FunctionPackageRunContext mirrors `struct FunctionPackageRunContext`.
// Pointer fields carry `Option<T>`: nil ⇔ `None`.
type FunctionPackageRunContext struct {
	InvocationKind string
	ActionID       *uuid.UUID
	ActionName     *string
	ObjectTypeID   *uuid.UUID
	TargetObjectID *uuid.UUID
	ActorID        uuid.UUID
}

// recordFunctionPackageRunSQL is the verbatim SQL the Rust source
// issues. Kept as a package-level constant so callers + tests can
// pin the byte shape.
const recordFunctionPackageRunSQL = `INSERT INTO ontology_function_package_runs (
               id,
               function_package_id,
               function_package_name,
               function_package_version,
               runtime,
               status,
               invocation_kind,
               action_id,
               action_name,
               object_type_id,
               target_object_id,
               actor_id,
               duration_ms,
               error_message,
               started_at,
               completed_at
           )
           VALUES (
               $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
           )`

// RecordFunctionPackageRun mirrors `pub async fn record_function_package_run`.
//
// Inserts one ontology_function_package_runs row capturing every field
// the metrics dashboard reads. duration_ms is clamped to a minimum of
// 0 (Rust `duration_ms.max(0)`); error_message accepts nil for
// successful runs.
func RecordFunctionPackageRun(
	ctx context.Context,
	db *pgxpool.Pool,
	pkg models.FunctionPackageSummary,
	runCtx FunctionPackageRunContext,
	startedAt time.Time,
	completedAt time.Time,
	durationMs int64,
	status string,
	errorMessage *string,
) error {
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("failed to record function package run: %s", err)
	}
	clampedDuration := durationMs
	if clampedDuration < 0 {
		clampedDuration = 0
	}

	_, err = db.Exec(ctx, recordFunctionPackageRunSQL,
		id,
		pkg.ID,
		pkg.Name,
		pkg.Version,
		pkg.Runtime,
		status,
		runCtx.InvocationKind,
		runCtx.ActionID,
		runCtx.ActionName,
		runCtx.ObjectTypeID,
		runCtx.TargetObjectID,
		runCtx.ActorID,
		clampedDuration,
		errorMessage,
		startedAt,
		completedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to record function package run: %s", err)
	}
	return nil
}
