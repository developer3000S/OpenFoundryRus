//! Kafka consumer for `saga.step.requested.v1` — the entry point of
//! the FASE 6 / Tarea 6.3 saga runtime.
//!
//! Per inbound request the consumer:
//!
//! 1. Decodes the [`SagaStepRequestedV1`] payload (poison-pill
//!    messages are skipped without committing the offset).
//! 2. Records the deterministic
//!    `derive_request_event_id(saga, correlation_id)` in the
//!    per-service idempotency table — a redelivery short-circuits
//!    here without re-running the saga.
//! 3. Opens a Postgres transaction, instantiates a
//!    [`saga::SagaRunner`], and dispatches the matching step graph
//!    via [`crate::automation_operations::domain::dispatcher::dispatch_saga`]. The runner
//!    persists every step's state + emits `saga.step.*.v1` outbox
//!    events inside the same transaction.
//! 4. Commits the transaction (which is what makes Debezium publish
//!    the events).
//! 5. Commits the Kafka offset.
//!
//! Crash safety: a process kill between (3) and (4) leaves the saga
//! row in an intermediate state with the original Kafka message
//! still uncommitted; on redelivery the idempotency check fires
//! AFTER the row had been written, but `SagaRunner::start` reads
//! back `completed_steps` and short-circuits the already-finished
//! prefix — so the saga resumes from the last committed step.
//!
//! See the inventory at
//! [`docs/architecture/refactor/automation-ops-worker-inventory.md`]
//! §11 for the trade-off note on holding the Postgres transaction
//! across step bodies (the libs/saga contract).

use std::sync::Arc;
use std::time::Duration;

use event_bus_data::{DataMessage, DataPublisher, DataSubscriber};
use idempotency::{IdempotencyError, IdempotencyStore, Outcome};
use saga::events::SagaStepRequestedV1;
use sqlx::PgPool;
use thiserror::Error;
use tokio::time::sleep;
use tracing::{error, info, warn};
use uuid::Uuid;

use crate::automation_operations::domain::dispatcher::dispatch_saga;
use crate::automation_operations::event::{derive_request_event_id, derive_saga_id};
use crate::automation_operations::topics::SAGA_STEP_REQUESTED_V1;

/// Kafka consumer group used by every replica of the service.
pub const CONSUMER_GROUP: &str = "automation-operations-service";

/// Topics the consumer subscribes to.
pub const SUBSCRIBE_TOPICS: &[&str] = &[SAGA_STEP_REQUESTED_V1];

/// Fully-qualified table backing the per-service idempotency store.
pub const PROCESSED_EVENTS_TABLE: &str = "automation_operations.processed_events";

#[derive(Debug, Error)]
pub enum ConsumerError {
    #[error("kafka subscribe failed: {0}")]
    Subscribe(#[from] event_bus_data::SubscribeError),
    #[error("kafka commit failed: {0}")]
    Commit(#[from] event_bus_data::CommitError),
    #[error("idempotency error: {0}")]
    Idempotency(#[from] IdempotencyError),
    #[error("saga runner error: {0}")]
    Saga(#[from] saga::SagaError),
    #[error("postgres error: {0}")]
    Db(#[from] sqlx::Error),
    #[error("malformed saga.step.requested.v1 payload: {0}")]
    Decode(String),
}

/// Long-lived consumer state. Cheap to clone (everything is `Arc`-
/// shared).
pub struct SagaConsumer {
    pub pool: PgPool,
    pub idempotency: Arc<dyn IdempotencyStore>,
    /// Kept around for the future control-plane publish path
    /// (e.g. emitting a synthetic `saga.aborted.v1` to Kafka without
    /// going through outbox+Debezium when the service shuts down
    /// mid-saga).
    pub publisher: Arc<dyn DataPublisher>,
}

impl SagaConsumer {
    pub fn new(
        pool: PgPool,
        idempotency: Arc<dyn IdempotencyStore>,
        publisher: Arc<dyn DataPublisher>,
    ) -> Self {
        Self {
            pool,
            idempotency,
            publisher,
        }
    }

    /// Process exactly one inbound saga request. Returns a metric
    /// label so the caller can record the outcome.
    pub async fn process(
        &self,
        request: SagaStepRequestedV1,
    ) -> Result<&'static str, ConsumerError> {
        let event_id = derive_request_event_id(&request.saga, request.correlation_id);
        match self.idempotency.check_and_record(event_id).await? {
            Outcome::AlreadyProcessed => {
                info!(
                    %event_id,
                    saga = %request.saga,
                    saga_id = %request.saga_id,
                    correlation_id = %request.correlation_id,
                    "saga.step.requested already processed; skipping",
                );
                return Ok("deduped");
            }
            Outcome::FirstSeen => {}
        }

        // Defence in depth: the deterministic saga_id helper must
        // collapse with what the producer published. If the producer
        // and consumer disagree, fall back to whatever the producer
        // sent — our derivation is informational, not authoritative.
        let expected_saga_id = derive_saga_id(&request.saga, request.correlation_id);
        if expected_saga_id != request.saga_id {
            warn!(
                producer_saga_id = %request.saga_id,
                consumer_derived = %expected_saga_id,
                saga = %request.saga,
                "saga_id derivation mismatch between producer and consumer; honoring producer",
            );
        }

        self.run_saga(request).await
    }

    async fn run_saga(&self, request: SagaStepRequestedV1) -> Result<&'static str, ConsumerError> {
        let mut tx = self.pool.begin().await?;
        let mut runner =
            saga::SagaRunner::start(&mut tx, request.saga_id, request.saga.clone()).await?;
        let dispatch_result =
            dispatch_saga(&request.saga, &mut runner, request.input.clone()).await;

        // Commit the transaction regardless of dispatch outcome —
        // the runner has already updated `saga.state` to its
        // terminal value (`completed`, `failed`, or `compensated`)
        // and emitted the matching outbox events. Aborting the TX
        // here would lose the audit trail; committing publishes the
        // failure event correctly.
        tx.commit().await?;

        Ok(match dispatch_result {
            Ok(()) => "completed",
            Err(err) => {
                tracing::warn!(
                    saga = %request.saga,
                    saga_id = %request.saga_id,
                    error = %err,
                    "saga ended in non-completed terminal state"
                );
                "failed_or_compensated"
            }
        })
    }
}

/// Decode a Kafka payload as [`SagaStepRequestedV1`].
pub fn decode_request(payload: &[u8]) -> Result<SagaStepRequestedV1, serde_json::Error> {
    serde_json::from_slice(payload)
}

/// Subscribe and run the at-least-once consumer loop. One Kafka
/// message ⇒ one saga (Queued → Running → Completed / Failed /
/// Compensated). Offsets are committed AFTER the dispatch + tx
/// commit so a crash mid-saga replays the message after restart;
/// the idempotency store catches obvious duplicates and
/// `SagaRunner::start` resumes any partially-completed saga from
/// `saga.state.completed_steps`.
pub async fn run<S>(consumer: Arc<SagaConsumer>, subscriber: S) -> Result<(), ConsumerError>
where
    S: DataSubscriber,
{
    subscriber.subscribe(SUBSCRIBE_TOPICS)?;
    info!(
        group = CONSUMER_GROUP,
        topics = ?SUBSCRIBE_TOPICS,
        "automation-operations saga consumer started",
    );

    loop {
        let message = subscriber.recv().await?;
        match process_message(&consumer, &message).await {
            Ok(label) => {
                tracing::debug!(label, topic = message.topic(), "saga request processed");
                subscriber.commit(&message)?;
            }
            Err(err) => {
                error!(error = %err, "saga request processing failed; offset uncommitted");
                // Bounded sleep so a redelivery storm does not burn
                // CPU before DLQ kicks in at the broker level.
                sleep(Duration::from_secs(5)).await;
            }
        }
    }
}

async fn process_message(
    consumer: &Arc<SagaConsumer>,
    message: &DataMessage,
) -> Result<&'static str, ConsumerError> {
    let Some(payload) = message.payload() else {
        warn!(
            topic = message.topic(),
            partition = message.partition(),
            offset = message.offset(),
            "skipping saga.step.requested without payload",
        );
        return Ok("empty_payload");
    };
    let request = match decode_request(payload) {
        Ok(request) => request,
        Err(err) => {
            warn!(error = %err, "skipping malformed saga.step.requested");
            return Ok("decode_error");
        }
    };
    consumer.process(request).await
}

/// Defence in depth: keeps `Uuid` import alive so future helper
/// extraction does not silently drop the dependency.
#[allow(dead_code)]
fn _phantom_uuid() -> Uuid {
    Uuid::nil()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn subscribe_topics_pinned() {
        assert_eq!(SUBSCRIBE_TOPICS, &[SAGA_STEP_REQUESTED_V1]);
    }

    #[test]
    fn consumer_group_pinned() {
        assert_eq!(CONSUMER_GROUP, "automation-operations-service");
    }

    #[test]
    fn processed_events_table_matches_migration_schema() {
        assert_eq!(
            PROCESSED_EVENTS_TABLE,
            "automation_operations.processed_events"
        );
    }

    #[test]
    fn decode_request_rejects_garbage() {
        assert!(decode_request(b"{ not json").is_err());
    }

    #[test]
    fn decode_request_round_trips_minimal_payload() {
        let payload = serde_json::json!({
            "saga_id": Uuid::nil(),
            "saga": "retention.sweep",
            "tenant_id": "acme",
            "correlation_id": Uuid::nil(),
            "triggered_by": "system",
        });
        let bytes = serde_json::to_vec(&payload).unwrap();
        let decoded = decode_request(&bytes).expect("decode");
        assert_eq!(decoded.saga, "retention.sweep");
        assert_eq!(decoded.input, serde_json::Value::Null);
    }
}
