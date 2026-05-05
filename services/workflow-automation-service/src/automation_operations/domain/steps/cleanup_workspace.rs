//! `cleanup.workspace` saga — three-step example with compensations,
//! used by the FASE 6 / Tarea 6.4 chaos test (LIFO compensation
//! verification).
//!
//! Step graph:
//!
//! ```text
//!   1. mark_for_deletion        — tombstone the workspace row.
//!      compensate ─►  unmark.
//!   2. drop_workspace_blobs     — delete object-store payload.
//!      compensate ─►  restore_from_soft_delete.
//!   3. finalize_workspace_deletion — emit lineage + audit events.
//!      compensate ─►  no-op (terminal step).
//! ```
//!
//! All three step bodies are **stubs** today (they return their
//! input unchanged). They become real HTTP calls into the owning
//! services in a future task; this module's job for Tarea 6.3 is to
//! wire the saga shape so Tarea 6.4 can validate the runner's
//! mechanics, and to give operators a concrete reference for what
//! a multi-step saga looks like in the new substrate.

use async_trait::async_trait;
use saga::{SagaError, SagaStep};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

/// Inbound payload for `cleanup.workspace`.
#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
pub struct CleanupWorkspaceInput {
    pub tenant_id: String,
    pub workspace_id: Uuid,
    /// When `Some`, the chaos-test fixture sets this to a step name
    /// to force that step's `execute` to fail. Production callers
    /// leave this `None`.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub force_failure_at: Option<String>,
}

impl CleanupWorkspaceInput {
    fn must_fail_here(&self, step: &str) -> bool {
        self.force_failure_at.as_deref() == Some(step)
    }
}

// ─────────────────────── Step 1: mark for deletion ────────────────────

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
pub struct MarkForDeletionOutput {
    pub workspace_id: Uuid,
    pub tombstoned_at: chrono::DateTime<chrono::Utc>,
}

pub struct MarkForDeletion;

#[async_trait]
impl SagaStep for MarkForDeletion {
    type Input = CleanupWorkspaceInput;
    type Output = MarkForDeletionOutput;

    fn step_name() -> &'static str {
        "mark_for_deletion"
    }

    async fn execute(input: Self::Input) -> Result<Self::Output, SagaError> {
        if input.must_fail_here(Self::step_name()) {
            return Err(SagaError::step(
                Self::step_name(),
                "forced failure (chaos test)",
            ));
        }
        Ok(MarkForDeletionOutput {
            workspace_id: input.workspace_id,
            tombstoned_at: chrono::Utc::now(),
        })
    }

    async fn compensate(_input: Self::Input) -> Result<(), SagaError> {
        // Real impl: clear the tombstone flag on the workspace row.
        Ok(())
    }
}

// ─────────────────────── Step 2: drop blobs ───────────────────────────

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
pub struct DropBlobsOutput {
    pub workspace_id: Uuid,
    /// Number of object-store keys soft-deleted in this step. The
    /// compensation can use this to size the `restore_from_soft_delete`
    /// fan-out.
    pub blob_count: u64,
}

pub struct DropWorkspaceBlobs;

#[async_trait]
impl SagaStep for DropWorkspaceBlobs {
    type Input = CleanupWorkspaceInput;
    type Output = DropBlobsOutput;

    fn step_name() -> &'static str {
        "drop_workspace_blobs"
    }

    async fn execute(input: Self::Input) -> Result<Self::Output, SagaError> {
        if input.must_fail_here(Self::step_name()) {
            return Err(SagaError::step(
                Self::step_name(),
                "forced failure (chaos test)",
            ));
        }
        Ok(DropBlobsOutput {
            workspace_id: input.workspace_id,
            blob_count: 0,
        })
    }

    async fn compensate(_input: Self::Input) -> Result<(), SagaError> {
        // Real impl: HTTP call into the storage service to restore
        // soft-deleted keys. The compensation does NOT have access
        // to the step's output (libs/saga contract); if the count
        // matters at restore time, the step should persist it via
        // a side-channel (e.g. workspace metadata) so the
        // compensation can read it back.
        Ok(())
    }
}

// ─────────────────────── Step 3: finalize ─────────────────────────────

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
pub struct FinalizeOutput {
    pub workspace_id: Uuid,
    pub finalized_at: chrono::DateTime<chrono::Utc>,
}

pub struct FinalizeWorkspaceDeletion;

#[async_trait]
impl SagaStep for FinalizeWorkspaceDeletion {
    type Input = CleanupWorkspaceInput;
    type Output = FinalizeOutput;

    fn step_name() -> &'static str {
        "finalize_workspace_deletion"
    }

    async fn execute(input: Self::Input) -> Result<Self::Output, SagaError> {
        if input.must_fail_here(Self::step_name()) {
            return Err(SagaError::step(
                Self::step_name(),
                "forced failure (chaos test)",
            ));
        }
        Ok(FinalizeOutput {
            workspace_id: input.workspace_id,
            finalized_at: chrono::Utc::now(),
        })
    }

    async fn compensate(_input: Self::Input) -> Result<(), SagaError> {
        // No compensation: by the time finalize runs, the previous
        // two steps have already produced terminal effects. If
        // finalize itself fails (e.g. lineage emission errored),
        // running compensations on the earlier steps still makes
        // sense — but finalize never compensates "itself", so this
        // is a no-op.
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn input(force_failure_at: Option<&str>) -> CleanupWorkspaceInput {
        CleanupWorkspaceInput {
            tenant_id: "acme".into(),
            workspace_id: Uuid::nil(),
            force_failure_at: force_failure_at.map(str::to_string),
        }
    }

    #[test]
    fn step_names_are_pinned() {
        assert_eq!(MarkForDeletion::step_name(), "mark_for_deletion");
        assert_eq!(DropWorkspaceBlobs::step_name(), "drop_workspace_blobs");
        assert_eq!(
            FinalizeWorkspaceDeletion::step_name(),
            "finalize_workspace_deletion"
        );
    }

    #[tokio::test]
    async fn happy_path_executes_each_step() {
        MarkForDeletion::execute(input(None)).await.unwrap();
        DropWorkspaceBlobs::execute(input(None)).await.unwrap();
        FinalizeWorkspaceDeletion::execute(input(None))
            .await
            .unwrap();
    }

    #[tokio::test]
    async fn forced_failure_only_triggers_for_named_step() {
        // Force step 2 to fail; steps 1 and 3 still succeed when
        // invoked directly.
        let i = input(Some("drop_workspace_blobs"));
        MarkForDeletion::execute(i.clone()).await.unwrap();
        let err = DropWorkspaceBlobs::execute(i.clone())
            .await
            .expect_err("must fail");
        assert!(err.to_string().contains("forced failure"));
        FinalizeWorkspaceDeletion::execute(i).await.unwrap();
    }

    #[tokio::test]
    async fn compensation_is_a_no_op_pure_function() {
        // The compensations don't depend on any state, so they
        // always succeed in the stub. The chaos test in Tarea 6.4
        // verifies they actually FIRE, in LIFO order, through the
        // SagaRunner.
        MarkForDeletion::compensate(input(None)).await.unwrap();
        DropWorkspaceBlobs::compensate(input(None)).await.unwrap();
        FinalizeWorkspaceDeletion::compensate(input(None))
            .await
            .unwrap();
    }
}
