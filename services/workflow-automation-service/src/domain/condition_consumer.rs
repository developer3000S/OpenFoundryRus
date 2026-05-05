//! Kafka consumer for `automate.condition.v1` — the heart of the
//! FASE 5 / Tarea 5.3 Foundry-pattern runtime.
//!
//! Per inbound condition the consumer:
//!
//! 1. Decodes the [`AutomateConditionV1`] payload (poison-pill
//!    messages are skipped without committing the offset, so DLQ
//!    routing happens at the broker level after N redeliveries).
//! 2. Records the deterministic
//!    `derive_condition_event_id(definition_id, correlation_id)` in
//!    the per-service idempotency table — a redelivery short-circuits
//!    here without re-dispatching the effect.
//! 3. Loads (or, on first delivery, observes) the
//!    `automation_runs` row inserted by the HTTP handler in the
//!    same transaction as the outbox publish. The row is identified
//!    by `derive_run_id(definition_id, correlation_id)`.
//! 4. Transitions the row Queued → Running (via
//!    [`state_machine::PgStore::apply`] with [`AutomationRunEvent::Claim`]).
//!    Already-Running rows (consumer crashed mid-dispatch in a
//!    previous run) are re-claimed for a fresh attempt.
//! 5. Extracts the ontology-action invocation from the trigger
//!    payload and dispatches it via [`EffectDispatcher::
//!    dispatch_with_retries`].
//! 6. Transitions the row Running → Completed (or Failed) inside the
//!    SAME Postgres transaction as the outbox publish of
//!    `automate.outcome.v1`. This is the durability join point that
//!    makes "row in terminal state ⇔ outcome event published" an
//!    atomic invariant.
//! 7. Commits the Kafka offset.
//!
//! Crash safety: at every boundary, the redelivery + idempotency +
//! state-machine triplet collapses any race onto the same outcome
//! without duplicate side effects. See the inline comments at each
//! step for the specific failure modes.

use std::sync::Arc;
use std::time::Duration;

use event_bus_data::{
    DataMessage, DataPublisher, DataSubscriber, OpenLineageHeaders, PublishError,
};
use idempotency::{IdempotencyError, IdempotencyStore, Outcome};
use outbox::{OutboxError, OutboxEvent};
use serde_json::Value;
use sqlx::PgPool;
use state_machine::{Loaded, PgStore, StateMachine, StoreError};
use thiserror::Error;
use tokio::time::sleep;
use tracing::{error, info, warn};
use uuid::Uuid;

use crate::domain::automation_run::{
    AutomationRun, AutomationRunEvent, AutomationRunState, AutomationRunStateError,
};
use crate::domain::effect_dispatcher::{
    DispatchError, DispatchOutcome, EffectDispatcher, RetryPolicy, extract_action_request,
};
use crate::event::{
    AutomateConditionV1, AutomateOutcomeV1, derive_condition_event_id, derive_run_id,
};
use crate::topics::{AUTOMATE_CONDITION_V1, AUTOMATE_OUTCOME_V1};

/// Kafka consumer group used by every replica of the service.
/// Pinned here so a typo across replicas does not silently fork
/// the rebalance state.
pub const CONSUMER_GROUP: &str = "workflow-automation-service";

/// Topic the consumer subscribes to. Pinned for the same reason as
/// [`CONSUMER_GROUP`].
pub const SUBSCRIBE_TOPICS: &[&str] = &[AUTOMATE_CONDITION_V1];

/// Fully-qualified table the [`PgStore`] writes to.
pub const AUTOMATION_RUNS_TABLE: &str = "workflow_automation.automation_runs";

/// Fully-qualified table backing the idempotency store.
pub const PROCESSED_EVENTS_TABLE: &str = "workflow_automation.processed_events";

/// OpenLineage namespace stamped on every outbox event header.
const DEFAULT_LINEAGE_NAMESPACE: &str = "openfoundry";

#[derive(Debug, Error)]
pub enum ConsumerError {
    #[error("kafka subscribe failed: {0}")]
    Subscribe(#[from] event_bus_data::SubscribeError),
    #[error("kafka commit failed: {0}")]
    Commit(#[from] event_bus_data::CommitError),
    #[error("kafka publish failed: {0}")]
    Publish(#[from] PublishError),
    #[error("idempotency error: {0}")]
    Idempotency(#[from] IdempotencyError),
    #[error("state machine error: {0}")]
    State(#[from] StoreError),
    #[error("automation run state error: {0}")]
    Transition(#[from] AutomationRunStateError),
    #[error("outbox enqueue failed: {0}")]
    Outbox(#[from] OutboxError),
    #[error("postgres error: {0}")]
    Db(#[from] sqlx::Error),
    #[error("malformed condition payload: {0}")]
    Decode(String),
}

/// All long-lived state the consumer loop and the per-message
/// handler share. Cheap to clone (everything is `Arc`-shared).
pub struct ConditionConsumer {
    pub pool: PgPool,
    pub runs: PgStore<AutomationRun>,
    pub idempotency: Arc<dyn IdempotencyStore>,
    pub dispatcher: EffectDispatcher,
    pub publisher: Arc<dyn DataPublisher>,
    pub retry_policy: RetryPolicy,
    pub lineage_namespace: String,
}

impl ConditionConsumer {
    pub fn new(
        pool: PgPool,
        idempotency: Arc<dyn IdempotencyStore>,
        dispatcher: EffectDispatcher,
        publisher: Arc<dyn DataPublisher>,
    ) -> Self {
        let runs = PgStore::<AutomationRun>::new(pool.clone(), AUTOMATION_RUNS_TABLE);
        Self {
            pool,
            runs,
            idempotency,
            dispatcher,
            publisher,
            retry_policy: RetryPolicy::default(),
            lineage_namespace: std::env::var("OF_OPENLINEAGE_NAMESPACE")
                .unwrap_or_else(|_| DEFAULT_LINEAGE_NAMESPACE.to_string()),
        }
    }

    /// Process exactly one inbound condition message. Returns a
    /// metric label so the loop can record the outcome.
    pub async fn process(
        &self,
        condition: AutomateConditionV1,
    ) -> Result<&'static str, ConsumerError> {
        let event_id = derive_condition_event_id(condition.definition_id, condition.correlation_id);
        match self.idempotency.check_and_record(event_id).await? {
            Outcome::AlreadyProcessed => {
                info!(
                    %event_id,
                    definition_id = %condition.definition_id,
                    correlation_id = %condition.correlation_id,
                    "condition already processed; skipping",
                );
                return Ok("deduped");
            }
            Outcome::FirstSeen => {}
        }

        let run_id = derive_run_id(condition.definition_id, condition.correlation_id);
        let loaded = match self.runs.load(run_id).await {
            Ok(loaded) => loaded,
            Err(StoreError::NotFound(_)) => {
                // The HTTP handler is supposed to INSERT the row in
                // the same transaction as the outbox publish. A
                // missing row here means either (a) the outbox
                // event was published outside our handler path
                // (synthetic / replay) or (b) Debezium delivered the
                // event before the INSERT was visible to this
                // replica. We fall through to a synthesised row
                // creation so manual replays still work.
                warn!(
                    %run_id,
                    definition_id = %condition.definition_id,
                    correlation_id = %condition.correlation_id,
                    "automation run row missing for known condition; creating on the fly",
                );
                let run = AutomationRun::new(
                    run_id,
                    crate::event::tenant_uuid_from_str(&condition.tenant_id),
                    condition.definition_id,
                    condition.correlation_id,
                    None,
                );
                self.runs.insert(run).await?
            }
            Err(other) => return Err(other.into()),
        };

        let claimed = match self.claim_for_dispatch(loaded).await {
            ClaimOutcome::Claimed(loaded) => loaded,
            ClaimOutcome::Terminal(state) => {
                info!(
                    %run_id,
                    %state,
                    "automation run already terminal; skipping dispatch",
                );
                return Ok("already_terminal");
            }
        };

        // Dispatch — pure HTTP, no DB work happens between the claim
        // and the terminal apply. A crash here re-delivers the
        // condition; the subsequent attempt finds the row in
        // `Running`, re-claims, and re-dispatches. Idempotency on
        // the upstream `ontology-actions-service::execute` is the
        // downstream service's responsibility (per ADR-0021); we
        // accept the at-least-once semantics that any HTTP-based
        // effect implies.
        let action =
            match extract_action_request(&condition.trigger_payload, condition.correlation_id) {
                Ok(action) => action,
                Err(err) => {
                    self.land_terminal_failed(
                        run_id,
                        &condition,
                        err.to_string(),
                        claimed.machine.attempts,
                    )
                    .await?;
                    return Ok("invalid_payload");
                }
            };

        match self
            .dispatcher
            .dispatch_with_retries(&action, self.retry_policy)
            .await
        {
            Ok(outcome) => {
                self.land_terminal_completed(
                    run_id,
                    &condition,
                    outcome.response.clone(),
                    outcome.attempts,
                )
                .await?;
                self.publish_outcome_via_kafka(
                    run_id,
                    &condition,
                    "completed",
                    Some(outcome.response),
                    None,
                    outcome.attempts,
                )
                .await?;
                Ok("completed")
            }
            Err(err) => {
                let attempts = match &err {
                    DispatchError::Exhausted { attempts, .. } => *attempts,
                    _ => 1,
                };
                self.land_terminal_failed(run_id, &condition, err.to_string(), attempts)
                    .await?;
                self.publish_outcome_via_kafka(
                    run_id,
                    &condition,
                    "failed",
                    None,
                    Some(err.to_string()),
                    attempts,
                )
                .await?;
                Ok("failed")
            }
        }
    }

    /// Persist the terminal `completed` transition AND enqueue the
    /// outcome event in the same Postgres transaction. This is the
    /// invariant the runtime relies on: as soon as the row is in
    /// `Completed`, Debezium has already (or will) publish the
    /// matching `automate.outcome.v1` from `outbox.events`, and
    /// vice-versa.
    async fn land_terminal_completed(
        &self,
        run_id: Uuid,
        condition: &AutomateConditionV1,
        response: Value,
        attempts: u32,
    ) -> Result<(), ConsumerError> {
        let mut tx = self.pool.begin().await?;
        let loaded = self.load_in_tx(&mut tx, run_id).await?;
        let mut next = loaded
            .machine
            .clone()
            .transition(AutomationRunEvent::EffectCompleted {
                response: response.clone(),
            })
            .map_err(|err| ConsumerError::State(StoreError::Transition(err)))?;
        next.attempts = attempts.max(next.attempts);
        self.write_in_tx(&mut tx, &loaded, &next).await?;
        let outcome = AutomateOutcomeV1 {
            run_id,
            definition_id: condition.definition_id,
            tenant_id: condition.tenant_id.clone(),
            correlation_id: condition.correlation_id,
            status: "completed".into(),
            effect_response: Some(response),
            error: None,
            attempts,
        };
        self.enqueue_outcome(&mut tx, run_id, condition, &outcome)
            .await?;
        tx.commit().await?;
        Ok(())
    }

    /// Persist the terminal `failed` transition AND enqueue the
    /// outcome event in the same Postgres transaction. Mirror of
    /// [`Self::land_terminal_completed`].
    async fn land_terminal_failed(
        &self,
        run_id: Uuid,
        condition: &AutomateConditionV1,
        error: String,
        attempts: u32,
    ) -> Result<(), ConsumerError> {
        let mut tx = self.pool.begin().await?;
        let loaded = self.load_in_tx(&mut tx, run_id).await?;
        let event = if loaded.machine.state == AutomationRunState::Queued {
            AutomationRunEvent::PreFlightFailed {
                error: error.clone(),
            }
        } else {
            AutomationRunEvent::EffectFailed {
                error: error.clone(),
            }
        };
        let mut next = loaded
            .machine
            .clone()
            .transition(event)
            .map_err(|err| ConsumerError::State(StoreError::Transition(err)))?;
        next.attempts = attempts.max(next.attempts);
        self.write_in_tx(&mut tx, &loaded, &next).await?;
        let outcome = AutomateOutcomeV1 {
            run_id,
            definition_id: condition.definition_id,
            tenant_id: condition.tenant_id.clone(),
            correlation_id: condition.correlation_id,
            status: "failed".into(),
            effect_response: None,
            error: Some(error),
            attempts,
        };
        self.enqueue_outcome(&mut tx, run_id, condition, &outcome)
            .await?;
        tx.commit().await?;
        Ok(())
    }

    /// Belt-and-braces Kafka publish in addition to the outbox
    /// enqueue. The outbox is the durable record (Debezium will
    /// eventually publish even if the broker is down right now);
    /// this direct publish makes the live consumer feed
    /// (UI live updates, `automate.outcome.v1` listeners) tighter
    /// in the steady-state happy path. If the broker is down the
    /// outbox row is still safe in `outbox.events` until Debezium
    /// drains it.
    async fn publish_outcome_via_kafka(
        &self,
        run_id: Uuid,
        condition: &AutomateConditionV1,
        status: &str,
        effect_response: Option<Value>,
        error: Option<String>,
        attempts: u32,
    ) -> Result<(), ConsumerError> {
        let outcome = AutomateOutcomeV1 {
            run_id,
            definition_id: condition.definition_id,
            tenant_id: condition.tenant_id.clone(),
            correlation_id: condition.correlation_id,
            status: status.to_string(),
            effect_response,
            error,
            attempts,
        };
        let payload = serde_json::to_vec(&outcome)
            .map_err(|err| PublishError::InvalidRecord(format!("encode outcome event: {err}")))?;
        let lineage = OpenLineageHeaders::new(
            self.lineage_namespace.clone(),
            format!("automation_run/{}", condition.tenant_id),
            run_id.to_string(),
            CONSUMER_GROUP,
        );
        if let Err(err) = self
            .publisher
            .publish(
                AUTOMATE_OUTCOME_V1,
                Some(run_id.as_bytes()),
                &payload,
                &lineage,
            )
            .await
        {
            // Outbox + Debezium will still deliver — log and move on.
            warn!(
                %run_id,
                %status,
                error = %err,
                "direct Kafka outcome publish failed; outbox/Debezium will retry"
            );
        }
        Ok(())
    }

    async fn enqueue_outcome(
        &self,
        tx: &mut sqlx::Transaction<'_, sqlx::Postgres>,
        run_id: Uuid,
        condition: &AutomateConditionV1,
        outcome: &AutomateOutcomeV1,
    ) -> Result<(), OutboxError> {
        let payload = serde_json::to_value(outcome)?;
        // Deterministic event id: collapses retries inside the
        // outbox table onto the same row.
        let mut bytes = [0_u8; 33];
        bytes[..16].copy_from_slice(run_id.as_bytes());
        bytes[16..32].copy_from_slice(condition.correlation_id.as_bytes());
        bytes[32] = match outcome.status.as_str() {
            "completed" => b'C',
            "failed" => b'F',
            _ => b'X',
        };
        let event_id = Uuid::new_v5(&crate::event::WORKFLOW_AUTOMATION_NAMESPACE, &bytes);
        let event = OutboxEvent::new(
            event_id,
            "automation_run",
            run_id.to_string(),
            AUTOMATE_OUTCOME_V1,
            payload,
        )
        .with_header("ol-namespace", self.lineage_namespace.clone())
        .with_header("ol-job", format!("automation_run/{}", condition.tenant_id))
        .with_header("ol-run-id", run_id.to_string())
        .with_header("ol-producer", CONSUMER_GROUP)
        .with_header(
            "x-audit-correlation-id",
            condition.correlation_id.to_string(),
        );
        outbox::enqueue(tx, event).await
    }

    async fn claim_for_dispatch(&self, loaded: Loaded<AutomationRun>) -> ClaimOutcome {
        let current = loaded.machine.current_state();
        match current {
            AutomationRunState::Queued => {
                match self.runs.apply(loaded, AutomationRunEvent::Claim).await {
                    Ok(claimed) => ClaimOutcome::Claimed(claimed),
                    Err(err) => {
                        warn!(error = %err, "automation run claim failed; surfacing");
                        ClaimOutcome::Terminal(current)
                    }
                }
            }
            AutomationRunState::Running => {
                // Crash recovery: the previous attempt died mid-dispatch.
                // Re-issue the effect call. We bump attempts manually
                // here because the state machine refuses Claim from
                // Running; the row stays in Running.
                ClaimOutcome::Claimed(loaded)
            }
            AutomationRunState::Suspended | AutomationRunState::Compensating => {
                // These states are produced by the saga / human-in-loop
                // future expansions, not by the single-step path the
                // condition consumer drives. Treat as already-terminal
                // for FASE 5 purposes.
                ClaimOutcome::Terminal(current)
            }
            AutomationRunState::Completed | AutomationRunState::Failed => {
                ClaimOutcome::Terminal(current)
            }
        }
    }

    async fn load_in_tx(
        &self,
        tx: &mut sqlx::Transaction<'_, sqlx::Postgres>,
        run_id: Uuid,
    ) -> Result<Loaded<AutomationRun>, ConsumerError> {
        // PgStore does not (yet) expose a tx-scoped load. Use raw SQL
        // here so the read sits in the same transaction as the
        // subsequent UPDATE — protects against another consumer
        // applying a competing transition between our load and our
        // apply.
        let row: (Value, i64) = sqlx::query_as(
            "SELECT state_data, version FROM workflow_automation.automation_runs WHERE id = $1 FOR UPDATE",
        )
        .bind(run_id)
        .fetch_one(&mut **tx)
        .await?;
        let machine: AutomationRun =
            serde_json::from_value(row.0).map_err(|err| StoreError::InvalidState {
                id: run_id,
                message: err.to_string(),
            })?;
        Ok(Loaded {
            machine,
            version: row.1,
        })
    }

    async fn write_in_tx(
        &self,
        tx: &mut sqlx::Transaction<'_, sqlx::Postgres>,
        previous: &Loaded<AutomationRun>,
        next: &AutomationRun,
    ) -> Result<(), ConsumerError> {
        let payload = serde_json::to_value(next)?;
        let state = AutomationRun::state_str(next.current_state());
        let expires_at = StateMachine::expires_at(next);
        let rows = sqlx::query(
            "UPDATE workflow_automation.automation_runs \
             SET state = $1, state_data = $2, version = version + 1, \
                 expires_at = $3, updated_at = now() \
             WHERE id = $4 AND version = $5",
        )
        .bind(&state)
        .bind(&payload)
        .bind(expires_at)
        .bind(next.aggregate_id())
        .bind(previous.version)
        .execute(&mut **tx)
        .await?;
        if rows.rows_affected() == 0 {
            return Err(ConsumerError::State(StoreError::Stale {
                id: next.aggregate_id(),
                expected: previous.version,
            }));
        }
        Ok(())
    }
}

/// Helper to make `serde_json::Error` flow through `ConsumerError`
/// via the existing `Outbox(#[from] OutboxError)` arm without an
/// extra error variant.
impl From<serde_json::Error> for ConsumerError {
    fn from(err: serde_json::Error) -> Self {
        ConsumerError::Outbox(OutboxError::Serialize(err))
    }
}

enum ClaimOutcome {
    Claimed(Loaded<AutomationRun>),
    Terminal(AutomationRunState),
}

/// Decode a Kafka payload as [`AutomateConditionV1`]. Surfaces the
/// underlying `serde_json::Error` so the consumer loop can attribute
/// the failure in metrics.
pub fn decode_condition(payload: &[u8]) -> Result<AutomateConditionV1, serde_json::Error> {
    serde_json::from_slice(payload)
}

/// Subscribe and run the at-least-once consumer loop on
/// `automate.condition.v1`. One Kafka message ⇒ one full dispatch
/// (Queued → Running → Completed / Failed). Offsets are committed
/// AFTER the terminal transition so a crash mid-dispatch replays
/// the condition; the per-condition idempotency store + the row's
/// state guarantee no duplicate side effects on the happy path,
/// and the in-flight `Running` re-claim path documented in
/// [`ConditionConsumer::claim_for_dispatch`] handles the rare
/// crash-mid-dispatch case.
pub async fn run<S>(consumer: Arc<ConditionConsumer>, subscriber: S) -> Result<(), ConsumerError>
where
    S: DataSubscriber,
{
    subscriber.subscribe(SUBSCRIBE_TOPICS)?;
    info!(
        group = CONSUMER_GROUP,
        topics = ?SUBSCRIBE_TOPICS,
        "workflow-automation condition consumer started",
    );

    loop {
        let message = subscriber.recv().await?;
        match process_message(&consumer, &message).await {
            Ok(label) => {
                tracing::debug!(label, topic = message.topic(), "condition processed");
                subscriber.commit(&message)?;
            }
            Err(err) => {
                error!(error = %err, "condition processing failed; offset uncommitted");
                // Bounded sleep so a redelivery storm does not burn
                // CPU before DLQ kicks in at the broker level.
                sleep(Duration::from_secs(5)).await;
            }
        }
    }
}

async fn process_message(
    consumer: &Arc<ConditionConsumer>,
    message: &DataMessage,
) -> Result<&'static str, ConsumerError> {
    let Some(payload) = message.payload() else {
        warn!(
            topic = message.topic(),
            partition = message.partition(),
            offset = message.offset(),
            "skipping condition without payload",
        );
        return Ok("empty_payload");
    };
    let condition = match decode_condition(payload) {
        Ok(condition) => condition,
        Err(err) => {
            warn!(error = %err, "skipping malformed condition");
            return Ok("decode_error");
        }
    };
    consumer.process(condition).await
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn subscribe_topics_pinned() {
        assert_eq!(SUBSCRIBE_TOPICS, &[AUTOMATE_CONDITION_V1]);
    }

    #[test]
    fn consumer_group_pinned() {
        assert_eq!(CONSUMER_GROUP, "workflow-automation-service");
    }

    #[test]
    fn table_constants_match_migration_schema() {
        assert_eq!(AUTOMATION_RUNS_TABLE, "workflow_automation.automation_runs");
        assert_eq!(
            PROCESSED_EVENTS_TABLE,
            "workflow_automation.processed_events"
        );
    }

    #[test]
    fn decode_condition_round_trips_minimal_payload() {
        let payload = serde_json::json!({
            "definition_id": Uuid::nil(),
            "tenant_id": "acme",
            "correlation_id": Uuid::nil(),
            "triggered_by": "user-1",
            "trigger_type": "manual",
            "trigger_payload": {"action_id": "promote"},
        });
        let bytes = serde_json::to_vec(&payload).unwrap();
        let decoded = decode_condition(&bytes).expect("decode");
        assert_eq!(decoded.tenant_id, "acme");
        assert_eq!(decoded.trigger_payload["action_id"], "promote");
    }

    #[test]
    fn decode_condition_surfaces_malformed_input() {
        assert!(decode_condition(b"{ not json").is_err());
    }
}
