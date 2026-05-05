//! Dispatch from a `task_type` string (the saga type carried on
//! `saga.step.requested.v1`) to the matching `SagaStep` graph.
//!
//! The registry is **compile-time** — same convention as
//! `pipeline-build-service::domain::engine` and the
//! `workflow-automation-service` consumer dispatch. A new saga type
//! requires a new module under `super::steps` plus a new arm in
//! [`dispatch_saga`]; an unknown type is rejected so the saga lands
//! in `failed` immediately.

use saga::{SagaError, SagaRunner};
use serde_json::Value;

use super::steps::cleanup_workspace::{
    CleanupWorkspaceInput, DropWorkspaceBlobs, FinalizeWorkspaceDeletion, MarkForDeletion,
};
use super::steps::retention_sweep::{EvictRetentionEligible, RetentionSweepInput};

/// Pinned list of `task_type`s this service knows how to dispatch.
/// Used by the HTTP handler to reject unknown saga types up-front
/// (instead of rejecting at consumer time, which would still hit the
/// idempotency table and consume an outbox row before failing).
pub const KNOWN_SAGA_TYPES: &[&str] = &["retention.sweep", "cleanup.workspace"];

/// `true` iff `task_type` has a registered step graph.
pub fn is_known(task_type: &str) -> bool {
    KNOWN_SAGA_TYPES.contains(&task_type)
}

/// Drive `task_type`'s step graph to completion. Returns
/// `Ok(())` after `runner.finish()` for the happy path; returns
/// `Err(SagaError)` if any step (or the input parsing) failed
/// — by the time this returns the runner has already run LIFO
/// compensations and updated `saga.state` to its terminal value.
pub async fn dispatch_saga(
    task_type: &str,
    runner: &mut SagaRunner<'_, '_>,
    input: Value,
) -> Result<(), SagaError> {
    match task_type {
        "retention.sweep" => dispatch_retention_sweep(runner, input).await,
        "cleanup.workspace" => dispatch_cleanup_workspace(runner, input).await,
        unknown => Err(SagaError::step(
            "dispatch",
            format!("unknown saga type {unknown:?}; known: {KNOWN_SAGA_TYPES:?}",),
        )),
    }
}

async fn dispatch_retention_sweep(
    runner: &mut SagaRunner<'_, '_>,
    input: Value,
) -> Result<(), SagaError> {
    let input: RetentionSweepInput = serde_json::from_value(input)
        .map_err(|err| SagaError::step("retention.sweep", format!("invalid input: {err}")))?;
    runner.execute_step::<EvictRetentionEligible>(input).await?;
    runner.finish().await?;
    Ok(())
}

async fn dispatch_cleanup_workspace(
    runner: &mut SagaRunner<'_, '_>,
    input: Value,
) -> Result<(), SagaError> {
    let input: CleanupWorkspaceInput = serde_json::from_value(input)
        .map_err(|err| SagaError::step("cleanup.workspace", format!("invalid input: {err}")))?;
    runner
        .execute_step::<MarkForDeletion>(input.clone())
        .await?;
    runner
        .execute_step::<DropWorkspaceBlobs>(input.clone())
        .await?;
    runner
        .execute_step::<FinalizeWorkspaceDeletion>(input)
        .await?;
    runner.finish().await?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn known_saga_types_are_pinned() {
        assert!(is_known("retention.sweep"));
        assert!(is_known("cleanup.workspace"));
    }

    #[test]
    fn unknown_saga_types_are_rejected() {
        assert!(!is_known("does-not-exist"));
        assert!(!is_known(""));
    }
}
