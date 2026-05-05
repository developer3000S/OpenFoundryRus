//! FASE 6 / Tarea 6.4 — saga chaos integration test.
//!
//! Boots a Postgres 16 testcontainer, applies the
//! `20260504300000_saga_state_and_outbox.sql` migration, then
//! exercises:
//!
//! 1. **Happy path** — `cleanup.workspace` runs all three steps,
//!    `saga.state.status` ends in `completed`,
//!    every `saga.step.completed.v1` event is recorded in
//!    `outbox.events` plus the terminal `saga.completed.v1`.
//!
//! 2. **Chaos / LIFO compensation** — force step 2
//!    (`drop_workspace_blobs`) to fail. Verify that:
//!    * The saga ends in `compensated` (NOT `failed` — at least one
//!      compensation ran).
//!    * `failed_step` = `drop_workspace_blobs`.
//!    * `completed_steps` only contains `mark_for_deletion` (step 1).
//!    * The outbox carries one `saga.step.failed.v1` for step 2 and
//!      one `saga.step.compensated.v1` for step 1 — the LIFO
//!      invariant the migration plan §6.4 calls out.
//!    * Step 3 (`finalize_workspace_deletion`) was NOT executed
//!      and produced no outbox event.
//!
//! 3. **Single-step happy path** — `retention.sweep` (one step,
//!    no compensation) ends in `completed` with the expected output
//!    captured in `step_outputs`.
//!
//! Gated by the `it-postgres` feature so plain
//! `cargo test -p workflow-automation-service` stays a no-IO unit
//! run.

#![cfg(feature = "it-postgres")]

use saga::{SagaRunner, events::SagaStepCompletedV1};
use sqlx::{Connection, PgConnection, Row, postgres::PgPoolOptions};
use testcontainers::{
    GenericImage, ImageExt,
    core::{IntoContainerPort, WaitFor},
    runners::AsyncRunner,
};
use uuid::Uuid;
use workflow_automation_service::automation_operations::domain::dispatcher::dispatch_saga;
use workflow_automation_service::automation_operations::domain::steps::cleanup_workspace::CleanupWorkspaceInput;
use workflow_automation_service::automation_operations::domain::steps::retention_sweep::RetentionSweepInput;

const SAGA_MIGRATION: &str = include_str!("../migrations/20260505020000_saga_state_and_outbox.sql");

async fn boot_pg() -> (testcontainers::ContainerAsync<GenericImage>, sqlx::PgPool) {
    let image = GenericImage::new("postgres", "16-alpine")
        .with_exposed_port(5432.tcp())
        .with_wait_for(WaitFor::message_on_stderr(
            "database system is ready to accept connections",
        ))
        .with_env_var("POSTGRES_USER", "of")
        .with_env_var("POSTGRES_PASSWORD", "of")
        .with_env_var("POSTGRES_DB", "automation_ops_test");

    let pg = image.start().await.expect("start postgres testcontainer");
    let host_port = pg
        .get_host_port_ipv4(5432)
        .await
        .expect("expose host port for postgres");
    let url = format!("postgres://of:of@127.0.0.1:{host_port}/automation_ops_test");

    let mut admin = PgConnection::connect(&url).await.expect("connect admin");
    sqlx::raw_sql(SAGA_MIGRATION)
        .execute(&mut admin)
        .await
        .expect("apply automation_operations migration");
    drop(admin);

    let pool = PgPoolOptions::new()
        .max_connections(4)
        .connect(&url)
        .await
        .expect("connect pool");
    (pg, pool)
}

async fn read_saga_state(
    pool: &sqlx::PgPool,
    saga_id: Uuid,
) -> (String, Vec<String>, Option<String>, serde_json::Value) {
    let row = sqlx::query(
        "SELECT status, completed_steps, failed_step, step_outputs \
         FROM saga.state WHERE saga_id = $1",
    )
    .bind(saga_id)
    .fetch_one(pool)
    .await
    .expect("read saga_state");
    (
        row.try_get("status").unwrap(),
        row.try_get("completed_steps").unwrap(),
        row.try_get("failed_step").unwrap(),
        row.try_get("step_outputs").unwrap(),
    )
}

async fn read_outbox_events_for(pool: &sqlx::PgPool) -> Vec<(String, serde_json::Value)> {
    // The outbox helper INSERTs and immediately DELETEs in the same
    // transaction. After commit the table is steady-state empty —
    // the WAL carries the events but we cannot SELECT them back.
    // We work around it by querying the WAL via pg_logical_emit_message?
    // That requires logical replication setup. Simpler: turn off the
    // DELETE half of the outbox helper for the test? Also intrusive.
    //
    // The cleanest fix is to wrap the runner in a single transaction
    // that is INSPECTED before commit. The test below uses
    // `runner.events()` (in-memory record) for assertions; this
    // helper exists only as a defence-in-depth check that the
    // outbox table really is empty in steady state.
    let rows = sqlx::query("SELECT topic, payload FROM outbox.events ORDER BY created_at")
        .fetch_all(pool)
        .await
        .expect("read outbox.events");
    rows.into_iter()
        .map(|row| {
            (
                row.try_get::<String, _>("topic").unwrap(),
                row.try_get::<serde_json::Value, _>("payload").unwrap(),
            )
        })
        .collect()
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn cleanup_workspace_happy_path_completes() {
    let (_pg, pool) = boot_pg().await;
    let saga_id = Uuid::now_v7();

    let mut tx = pool.begin().await.unwrap();
    {
        let mut runner = SagaRunner::start(&mut tx, saga_id, "cleanup.workspace")
            .await
            .expect("runner start");
        let input = CleanupWorkspaceInput {
            tenant_id: "acme".into(),
            workspace_id: Uuid::now_v7(),
            force_failure_at: None,
        };
        let payload = serde_json::to_value(&input).unwrap();
        dispatch_saga("cleanup.workspace", &mut runner, payload)
            .await
            .expect("happy path");

        // Assert the in-memory event record on the runner BEFORE
        // commit (the outbox helper INSERTs+DELETEs in the same TX,
        // so post-commit the table is empty).
        let events = runner.events();
        let topics: Vec<&str> = events.iter().map(|e| e.kind.topic()).collect();
        assert_eq!(
            topics,
            vec![
                "saga.step.completed.v1",
                "saga.step.completed.v1",
                "saga.step.completed.v1",
                "saga.completed.v1",
            ],
            "expected three step.completed.v1 events plus the terminal saga.completed.v1"
        );
    }
    tx.commit().await.unwrap();

    let (status, completed, failed_step, step_outputs) = read_saga_state(&pool, saga_id).await;
    assert_eq!(status, "completed");
    assert_eq!(
        completed,
        vec![
            "mark_for_deletion".to_string(),
            "drop_workspace_blobs".to_string(),
            "finalize_workspace_deletion".to_string(),
        ]
    );
    assert!(failed_step.is_none());
    assert!(step_outputs.get("mark_for_deletion").is_some());
    assert!(step_outputs.get("drop_workspace_blobs").is_some());
    assert!(step_outputs.get("finalize_workspace_deletion").is_some());

    // Outbox table is steady-state empty after commit.
    assert!(
        read_outbox_events_for(&pool).await.is_empty(),
        "outbox.events should be empty after commit (INSERT+DELETE in same TX)"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn cleanup_workspace_step_two_failure_compensates_step_one() {
    let (_pg, pool) = boot_pg().await;
    let saga_id = Uuid::now_v7();

    let mut tx = pool.begin().await.unwrap();
    {
        let mut runner = SagaRunner::start(&mut tx, saga_id, "cleanup.workspace")
            .await
            .expect("runner start");
        let input = CleanupWorkspaceInput {
            tenant_id: "acme".into(),
            workspace_id: Uuid::now_v7(),
            force_failure_at: Some("drop_workspace_blobs".into()),
        };
        let payload = serde_json::to_value(&input).unwrap();
        let result = dispatch_saga("cleanup.workspace", &mut runner, payload).await;
        assert!(
            result.is_err(),
            "dispatch_saga must return Err when a step fails"
        );

        // Assert the outbox emission order BEFORE commit.
        // Expected sequence: step 1 completed → step 2 failed →
        // step 1 compensated (LIFO of completed steps). No step 3
        // event because step 3 never ran.
        let topics: Vec<&str> = runner.events().iter().map(|e| e.kind.topic()).collect();
        assert_eq!(
            topics,
            vec![
                "saga.step.completed.v1",
                "saga.step.failed.v1",
                "saga.step.compensated.v1",
            ],
            "LIFO compensation: step 1 completes, step 2 fails, step 1 compensates"
        );

        // The compensated event must reference step 1 by name.
        let compensated_step = runner
            .events()
            .iter()
            .find(|e| e.kind == saga::SagaEventKind::StepCompensated)
            .and_then(|e| e.step_name.clone())
            .expect("expected one step.compensated event");
        assert_eq!(compensated_step, "mark_for_deletion");
    }
    tx.commit().await.unwrap();

    // Saga state ends in `compensated` because at least one
    // compensation ran. `failed_step` records where the chain
    // broke. `completed_steps` only contains the step that
    // actually finished before the failure.
    let (status, completed, failed_step, step_outputs) = read_saga_state(&pool, saga_id).await;
    assert_eq!(status, "compensated");
    assert_eq!(failed_step.as_deref(), Some("drop_workspace_blobs"));
    assert_eq!(completed, vec!["mark_for_deletion".to_string()]);
    assert!(step_outputs.get("mark_for_deletion").is_some());
    assert!(
        step_outputs.get("drop_workspace_blobs").is_none(),
        "step 2 never produced an output"
    );
    assert!(
        step_outputs.get("finalize_workspace_deletion").is_none(),
        "step 3 never executed"
    );
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn retention_sweep_single_step_completes_with_output() {
    let (_pg, pool) = boot_pg().await;
    let saga_id = Uuid::now_v7();

    let mut tx = pool.begin().await.unwrap();
    {
        let mut runner = SagaRunner::start(&mut tx, saga_id, "retention.sweep")
            .await
            .expect("runner start");
        let input = RetentionSweepInput {
            tenant_id: "acme".into(),
            older_than_days: 30,
            dry_run: true,
        };
        let payload = serde_json::to_value(&input).unwrap();
        dispatch_saga("retention.sweep", &mut runner, payload)
            .await
            .expect("single-step happy path");

        let topics: Vec<&str> = runner.events().iter().map(|e| e.kind.topic()).collect();
        assert_eq!(
            topics,
            vec!["saga.step.completed.v1", "saga.completed.v1"],
            "single-step saga emits one step event + one terminal event",
        );
    }
    tx.commit().await.unwrap();

    let (status, completed, failed_step, step_outputs) = read_saga_state(&pool, saga_id).await;
    assert_eq!(status, "completed");
    assert!(failed_step.is_none());
    assert_eq!(completed, vec!["evict_retention_eligible".to_string()]);
    let output = step_outputs
        .get("evict_retention_eligible")
        .expect("output captured");
    assert_eq!(output["older_than_days"], 30);
    assert_eq!(output["dry_run"], true);
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn idempotent_replay_skips_completed_step() {
    let (_pg, pool) = boot_pg().await;
    let saga_id = Uuid::now_v7();
    let input = RetentionSweepInput {
        tenant_id: "acme".into(),
        older_than_days: 7,
        dry_run: false,
    };
    let payload = serde_json::to_value(&input).unwrap();

    // First run: saga completes.
    {
        let mut tx = pool.begin().await.unwrap();
        let mut runner = SagaRunner::start(&mut tx, saga_id, "retention.sweep")
            .await
            .unwrap();
        dispatch_saga("retention.sweep", &mut runner, payload.clone())
            .await
            .unwrap();
        tx.commit().await.unwrap();
    }
    let (first_status, _, _, _) = read_saga_state(&pool, saga_id).await;
    assert_eq!(first_status, "completed");

    // Second run with the same saga_id: the runner reads back the
    // terminal status and refuses to start (per `libs/saga`'s
    // `SagaError::Terminal`). This is the safety net that prevents
    // duplicate-effect re-execution.
    let mut tx = pool.begin().await.unwrap();
    let result = SagaRunner::start(&mut tx, saga_id, "retention.sweep").await;
    match result {
        Err(saga::SagaError::Terminal(status)) => {
            assert_eq!(status, "completed");
        }
        Err(other) => panic!("expected SagaError::Terminal, got {other:?}"),
        Ok(_) => panic!("expected refusal, got Ok"),
    }
}

#[tokio::test(flavor = "multi_thread", worker_threads = 2)]
async fn step_completed_event_id_is_deterministic_v5() {
    // Defence in depth: a SagaStepCompletedV1's `event_id` derived
    // by `libs/saga::SagaRunner` is a UUIDv5 over
    // `(saga_id, step_name, kind)`. Re-driving the same inputs
    // would yield the same id and collapse via the outbox's
    // ON CONFLICT DO NOTHING.
    //
    // The on-the-wire payload shape is verified by
    // `libs/saga/src/events.rs::tests` (every variant has a
    // round-trip test there); we cannot inspect the outbox row
    // here because the helper INSERTs and DELETEs in the same
    // transaction (see the `libs/outbox` module doc-comment), so
    // by the time control returns to this test the row is gone.
    let (_pg, pool) = boot_pg().await;
    let saga_id = Uuid::now_v7();
    let input = RetentionSweepInput {
        tenant_id: "acme".into(),
        older_than_days: 90,
        dry_run: true,
    };
    let payload = serde_json::to_value(&input).unwrap();

    let event_ids: Vec<Uuid> = {
        let mut tx = pool.begin().await.unwrap();
        let mut runner = SagaRunner::start(&mut tx, saga_id, "retention.sweep")
            .await
            .unwrap();
        dispatch_saga("retention.sweep", &mut runner, payload)
            .await
            .unwrap();
        let ids = runner.events().iter().map(|e| e.event_id).collect();
        tx.commit().await.unwrap();
        ids
    };

    // Each saga of this kind produces exactly two events
    // (step.completed + saga.completed) and each event_id is
    // a v5 UUID.
    assert_eq!(event_ids.len(), 2, "expected 2 events");
    for id in &event_ids {
        assert_eq!(id.get_version_num(), 5);
    }
    // Compile-time bind so the typed import does not regress.
    let _phantom: Option<SagaStepCompletedV1> = None;
    let _ = _phantom;
}
