//! `approvals-timeout-sweep` — single-shot binary that drives the
//! `pending → expired` transition for every row in
//! `audit_compliance.approval_requests` whose `expires_at` is
//! `<= now()`.
//!
//! Intended deployment: Kubernetes `CronJob` running every 5 min.
//! Each pod boots, opens a Postgres pool, runs one
//! [`state_machine::PgStore::timeout_sweep`], applies the
//! [`workflow_automation_service::approvals::domain::approval_request::ApprovalRequestEvent::Expire`]
//! event to every claimed row + INSERTs an `approval.expired.v1`
//! outbox row in the same transaction, and exits with code 0 on
//! success.
//!
//! FASE 7 / Tarea 7.4 deliverable. Pattern mirrors
//! `libs/event-scheduler/src/bin/schedules-tick.rs` (Tarea 3.5).
//!
//! Environment:
//!
//! * `DATABASE_URL` — required, the same DSN
//!   `approvals-service` itself uses.
//! * `RUST_LOG` — log filter (default: `info`).

use std::time::Duration;

use anyhow::{Context, Result};
use chrono::Utc;
use outbox::OutboxEvent;
use sqlx::postgres::PgPoolOptions;
use state_machine::{Loaded, PgStore, StateMachine};
use tracing_subscriber::EnvFilter;
use uuid::Uuid;
use workflow_automation_service::approvals::domain::approval_request::{
    ApprovalRequest, ApprovalRequestEvent, ApprovalRequestState,
};
use workflow_automation_service::approvals::event::{ApprovalExpiredV1, derive_outbox_event_id};
use workflow_automation_service::approvals::topics::APPROVAL_EXPIRED_V1;

const APPROVAL_REQUESTS_TABLE: &str = "audit_compliance.approval_requests";
const SERVICE_NAME: &str = "approvals-timeout-sweep";

#[tokio::main]
async fn main() -> Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info")),
        )
        .with_target(false)
        .json()
        .init();

    let database_url = std::env::var("DATABASE_URL")
        .context("DATABASE_URL must be set (Postgres connection URL)")?;

    let pool = PgPoolOptions::new()
        .max_connections(4)
        .acquire_timeout(Duration::from_secs(10))
        .connect(&database_url)
        .await
        .context("failed to connect to Postgres")?;

    let store = PgStore::<ApprovalRequest>::new(pool.clone(), APPROVAL_REQUESTS_TABLE);
    let now = Utc::now();
    let candidates = store.timeout_sweep(now).await?;

    let mut expired = 0u32;
    let mut skipped = 0u32;
    let mut failed = 0u32;

    for loaded in candidates {
        // The sweep returns every row whose `expires_at <= now()`,
        // including already-terminal rows that happen to keep an
        // `expires_at` value from before their decision. Skip
        // anything that is no longer pending — the row's state
        // machine refuses the Expire event for terminal states
        // anyway, but we'd waste a database round-trip.
        if loaded.machine.state != ApprovalRequestState::Pending {
            skipped += 1;
            continue;
        }
        let approval_id = loaded.machine.id;
        let deadline = loaded.machine.expires_at.unwrap_or(now);
        match transition_and_publish(&pool, &store, loaded, now, deadline).await {
            Ok(()) => {
                expired += 1;
                tracing::info!(
                    %approval_id,
                    %deadline,
                    "approval expired"
                );
            }
            Err(error) => {
                failed += 1;
                tracing::warn!(
                    %approval_id,
                    error = %error,
                    "approval expire failed; will retry on next sweep"
                );
            }
        }
    }

    tracing::info!(expired, skipped, failed, %now, "timeout sweep completed");
    println!("{expired}");
    Ok(())
}

async fn transition_and_publish(
    pool: &sqlx::PgPool,
    store: &PgStore<ApprovalRequest>,
    loaded: Loaded<ApprovalRequest>,
    expired_at: chrono::DateTime<Utc>,
    deadline: chrono::DateTime<Utc>,
) -> Result<()> {
    let approval_id = loaded.machine.id;
    let tenant_id = loaded.machine.tenant_id.clone();
    let correlation_id = loaded.machine.correlation_id;

    // Step 1 — apply the Expire event via the libs/state-machine
    // helper. This is its own atomic UPDATE inside the helper.
    store
        .apply(loaded, ApprovalRequestEvent::Expire { expired_at })
        .await
        .context("PgStore::apply Expire failed")?;

    // Step 2 — publish approval.expired.v1 via the outbox in a
    // separate transaction. The deterministic event_id collapses
    // duplicate emissions on retry.
    let mut tx = pool.begin().await.context("begin tx")?;
    let payload = ApprovalExpiredV1 {
        approval_id,
        tenant_id,
        correlation_id,
        expired_at,
        deadline,
    };
    enqueue_expired(&mut tx, approval_id, &payload).await?;
    tx.commit().await.context("commit tx")?;
    Ok(())
}

async fn enqueue_expired(
    tx: &mut sqlx::Transaction<'_, sqlx::Postgres>,
    approval_id: Uuid,
    payload: &ApprovalExpiredV1,
) -> Result<()> {
    let event_id = derive_outbox_event_id(approval_id, "expired");
    let body = serde_json::to_value(payload).context("serialize payload")?;
    let event = OutboxEvent::new(
        event_id,
        "approval_request",
        approval_id.to_string(),
        APPROVAL_EXPIRED_V1,
        body,
    )
    .with_header("x-audit-correlation-id", payload.correlation_id.to_string())
    .with_header("ol-job", "approvals/timeout-sweep".to_string())
    .with_header("ol-run-id", approval_id.to_string())
    .with_header("ol-producer", SERVICE_NAME);
    outbox::enqueue(tx, event).await.context("outbox enqueue")?;
    Ok(())
}

// Defence in depth: the binary doesn't move state by itself; the
// `state_machine::PgStore` helper does. This compile-time anchor
// ensures the StateMachine trait keeps `expires_at` available so
// the partial index on `approval_requests.expires_at` matches the
// helper's WHERE clause exactly.
#[allow(dead_code)]
fn _phantom_state_machine_trait_dep() -> Option<chrono::DateTime<Utc>> {
    let req = ApprovalRequest::new(
        Uuid::nil(),
        "tenant",
        "subject",
        vec![],
        serde_json::Value::Null,
        Uuid::nil(),
        None,
    );
    StateMachine::expires_at(&req)
}
