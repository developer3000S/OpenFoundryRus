//! `AutomationRun` aggregate — pure state machine for the
//! Foundry-pattern replacement of the Temporal
//! `WorkflowAutomationRun` workflow.
//!
//! This module is the Tarea 5.2 deliverable of the migration plan
//! (`docs/architecture/migration-plan-foundry-pattern-orchestration.md`):
//! the state enum, the allowed-transition table, and the
//! `state_machine::StateMachine` implementation that backs the
//! `workflow_automation.automation_runs` table created by
//! `migrations/20260504100000_automation_runs.sql`.
//!
//! **Scope of this file**: pure, no-IO, no consumer / dispatcher
//! wiring. Tarea 5.3 wires the condition consumer + effect
//! dispatcher and instantiates `state_machine::PgStore<AutomationRun>`.
//! Keeping the state machine pure here lets the trait impl be unit
//! tested without a database and lets the persistence boilerplate
//! live entirely inside `libs/state-machine`.
//!
//! ## State table
//!
//! Allowed transitions (also enforced server-side by the SQL
//! `CHECK` constraint plus the application-level
//! `AutomationRunState::can_transition_to` guard):
//!
//! ```text
//!   Queued       → Running       (consumer claims)
//!   Queued       → Failed        (definition broken / pre-flight reject)
//!   Running      → Completed     (effect succeeded)
//!   Running      → Failed        (effect failed terminally)
//!   Running      → Suspended     (multi-step waits on external signal)
//!   Running      → Compensating  (saga rollback triggered)
//!   Suspended    → Running       (resumed by signal)
//!   Suspended    → Failed        (timeout sweep / external cancel)
//!   Compensating → Failed        (compensation finished — terminal)
//!   <terminal>   → <self>        (idempotent re-application — safe)
//! ```
//!
//! `Completed`, `Failed` are terminal; `Compensating` is bound for
//! `Failed`; `Suspended` is the only re-enterable non-terminal state.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use state_machine::{StateMachine, TransitionError};
use std::fmt;
use uuid::Uuid;

/// Lifecycle of a row in `workflow_automation.automation_runs`.
///
/// Wire format (lowercase) matches the SQL `CHECK` constraint in the
/// migration; do not rename without coordinating the schema migration
/// in lockstep.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum AutomationRunState {
    /// Row created from a `automate.condition.v1` event (or a
    /// synchronous handler) before the consumer picks it up.
    Queued,
    /// Consumer is actively dispatching the effect (HTTP POST against
    /// `ontology-actions-service::POST /api/v1/ontology/actions/{id}/execute`).
    Running,
    /// Multi-step automation is waiting on an external signal
    /// (human-in-the-loop, cron, downstream event).
    Suspended,
    /// Saga rollback in progress — active reversal of completed
    /// sub-steps. Always terminal-bound (`Failed`).
    Compensating,
    /// Terminal success.
    Completed,
    /// Terminal failure (effect error after retry envelope, validation
    /// failure, or timeout sweep).
    Failed,
}

impl AutomationRunState {
    /// Wire-format string used in the SQL `CHECK` constraint, in
    /// `automate.outcome.v1` events, and in tracing.
    pub fn as_str(self) -> &'static str {
        match self {
            AutomationRunState::Queued => "queued",
            AutomationRunState::Running => "running",
            AutomationRunState::Suspended => "suspended",
            AutomationRunState::Compensating => "compensating",
            AutomationRunState::Completed => "completed",
            AutomationRunState::Failed => "failed",
        }
    }

    /// Parse the wire-format string. Inverse of [`Self::as_str`].
    pub fn parse(value: &str) -> Result<Self, AutomationRunStateError> {
        match value {
            "queued" => Ok(AutomationRunState::Queued),
            "running" => Ok(AutomationRunState::Running),
            "suspended" => Ok(AutomationRunState::Suspended),
            "compensating" => Ok(AutomationRunState::Compensating),
            "completed" => Ok(AutomationRunState::Completed),
            "failed" => Ok(AutomationRunState::Failed),
            other => Err(AutomationRunStateError::Unknown(other.to_string())),
        }
    }

    /// Terminal states: `Completed`, `Failed`. Operators that hold a
    /// terminal row never need to issue further transitions.
    pub fn is_terminal(self) -> bool {
        matches!(
            self,
            AutomationRunState::Completed | AutomationRunState::Failed
        )
    }

    /// Validate a proposed transition `self → next` against the table
    /// in the module-level doc comment. Self-transitions on terminal
    /// states are allowed so an `INSERT … ON CONFLICT DO UPDATE`
    /// writer that re-applies the same outcome event does not raise.
    pub fn can_transition_to(self, next: AutomationRunState) -> bool {
        if self == next {
            return true;
        }
        matches!(
            (self, next),
            (AutomationRunState::Queued, AutomationRunState::Running)
                | (AutomationRunState::Queued, AutomationRunState::Failed)
                | (AutomationRunState::Running, AutomationRunState::Completed)
                | (AutomationRunState::Running, AutomationRunState::Failed)
                | (AutomationRunState::Running, AutomationRunState::Suspended)
                | (
                    AutomationRunState::Running,
                    AutomationRunState::Compensating
                )
                | (AutomationRunState::Suspended, AutomationRunState::Running)
                | (AutomationRunState::Suspended, AutomationRunState::Failed)
                | (AutomationRunState::Compensating, AutomationRunState::Failed)
        )
    }

    /// Same as [`Self::can_transition_to`] but returns a typed error
    /// instead of a bool — for the boundary in `transition`.
    pub fn validate_transition(
        self,
        next: AutomationRunState,
    ) -> Result<(), AutomationRunStateError> {
        if self.can_transition_to(next) {
            Ok(())
        } else {
            Err(AutomationRunStateError::IllegalTransition {
                from: self,
                to: next,
            })
        }
    }
}

impl fmt::Display for AutomationRunState {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.as_str())
    }
}

#[derive(Debug, thiserror::Error)]
pub enum AutomationRunStateError {
    #[error("unknown automation run state {0:?}")]
    Unknown(String),
    #[error("illegal automation run transition {from} → {to}")]
    IllegalTransition {
        from: AutomationRunState,
        to: AutomationRunState,
    },
}

/// Domain events the AutomationRun aggregate accepts. Each variant is
/// the trigger for exactly one transition in the table above.
#[derive(Debug, Clone)]
pub enum AutomationRunEvent {
    /// Consumer claimed the row from `automate.condition.v1` and is
    /// about to call the effect endpoint.
    Claim,
    /// Effect call returned 2xx. Carries the upstream JSON response
    /// (verbatim) for the operator UI / outcome event.
    EffectCompleted { response: Value },
    /// Effect call exceeded the retry envelope or returned a
    /// non-retryable 4xx (mirrors the `nonRetryable` errors raised by
    /// the legacy Go activity).
    EffectFailed { error: String },
    /// Definition broke pre-flight (e.g., missing `action_id`,
    /// downstream service unconfigured). Direct transition from
    /// `Queued` to `Failed` without going through `Running`.
    PreFlightFailed { error: String },
    /// Multi-step automation requested a wait. `wait_until` becomes
    /// the row's new `expires_at` so the timeout sweep can resume the
    /// run automatically.
    Suspend { wait_until: Option<DateTime<Utc>> },
    /// External signal resumed a suspended run.
    Resume,
    /// Saga compensation triggered — the run is rolling back.
    StartCompensating,
    /// Compensation finished; the run lands in `Failed` with the
    /// original failure reason carried along.
    CompensationCompleted { error: String },
    /// Suspended run hit its `wait_until` (timeout sweep) or was
    /// cancelled by a control-plane HTTP route.
    SuspensionFailed { error: String },
}

/// The `AutomationRun` aggregate as persisted in the `state_data`
/// column. Operator-facing summaries (`tenant_id`, `definition_id`,
/// `correlation_id`, the rendered state, `expires_at`) are also
/// projected onto dedicated columns by the SQL migration so dashboards
/// do not have to crack open this JSON.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AutomationRun {
    pub id: Uuid,
    pub tenant_id: Uuid,
    pub definition_id: Uuid,
    pub correlation_id: Uuid,
    pub state: AutomationRunState,
    /// Aligned with the dedicated `expires_at` column.
    #[serde(default)]
    pub expires_at: Option<DateTime<Utc>>,
    /// Number of effect-dispatch attempts so far (the consumer's
    /// internal retry envelope, not the Postgres optimistic-lock
    /// `version`).
    #[serde(default)]
    pub attempts: u32,
    /// Last operator-facing error message. Set when `state` lands in
    /// `Failed` or `Compensating`.
    #[serde(default)]
    pub last_error: Option<String>,
    /// Decoded JSON body of the most recent successful effect call.
    /// `None` until the first `EffectCompleted` event.
    #[serde(default)]
    pub effect_response: Option<Value>,
    /// Multi-step progress payload. Tarea 5.3 (and FASE 6 saga work)
    /// will populate this with step-level breakdown; today single-step
    /// runs leave it as the empty object.
    #[serde(default)]
    pub progress: Value,
}

impl AutomationRun {
    /// Build a fresh row in the `Queued` state. The condition consumer
    /// (Tarea 5.3) will derive `id` as `uuid_v5(definition_id ||
    /// correlation_id)` per ADR-0038 so producer redeliveries are
    /// idempotent.
    pub fn new(
        id: Uuid,
        tenant_id: Uuid,
        definition_id: Uuid,
        correlation_id: Uuid,
        expires_at: Option<DateTime<Utc>>,
    ) -> Self {
        Self {
            id,
            tenant_id,
            definition_id,
            correlation_id,
            state: AutomationRunState::Queued,
            expires_at,
            attempts: 0,
            last_error: None,
            effect_response: None,
            progress: Value::Object(Default::default()),
        }
    }
}

impl StateMachine for AutomationRun {
    type State = AutomationRunState;
    type Event = AutomationRunEvent;

    fn aggregate_id(&self) -> Uuid {
        self.id
    }

    fn current_state(&self) -> Self::State {
        self.state
    }

    fn expires_at(&self) -> Option<DateTime<Utc>> {
        self.expires_at
    }

    fn state_str(state: Self::State) -> String {
        state.as_str().to_string()
    }

    fn transition(mut self, event: Self::Event) -> Result<Self, TransitionError> {
        use AutomationRunEvent::*;
        use AutomationRunState::*;

        let next = match (self.state, &event) {
            (Queued, Claim) => {
                self.attempts = self.attempts.saturating_add(1);
                Running
            }
            (Queued, PreFlightFailed { error }) => {
                self.last_error = Some(error.clone());
                Failed
            }
            (Running, EffectCompleted { response }) => {
                self.effect_response = Some(response.clone());
                self.expires_at = None;
                Completed
            }
            (Running, EffectFailed { error }) => {
                self.last_error = Some(error.clone());
                self.expires_at = None;
                Failed
            }
            (Running, Suspend { wait_until }) => {
                self.expires_at = *wait_until;
                Suspended
            }
            (Running, StartCompensating) => Compensating,
            (Suspended, Resume) => {
                self.attempts = self.attempts.saturating_add(1);
                self.expires_at = None;
                Running
            }
            (Suspended, SuspensionFailed { error }) => {
                self.last_error = Some(error.clone());
                self.expires_at = None;
                Failed
            }
            (Compensating, CompensationCompleted { error }) => {
                self.last_error = Some(error.clone());
                Failed
            }
            (current, evt) => {
                return Err(TransitionError::invalid(format!(
                    "no AutomationRun transition from {current} for event {evt:?}",
                )));
            }
        };

        // Defence in depth: even though every (state, event) arm
        // above writes a *valid* next state, the can_transition_to
        // guard catches accidental drift if a future arm is added
        // without updating the table.
        self.state.validate_transition(next).map_err(|err| {
            TransitionError::invalid(format!(
                "AutomationRun produced disallowed transition: {err}"
            ))
        })?;
        self.state = next;
        Ok(self)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn fresh_run() -> AutomationRun {
        AutomationRun::new(
            Uuid::now_v7(),
            Uuid::now_v7(),
            Uuid::now_v7(),
            Uuid::now_v7(),
            None,
        )
    }

    #[test]
    fn parse_round_trips_every_state() {
        for state in [
            AutomationRunState::Queued,
            AutomationRunState::Running,
            AutomationRunState::Suspended,
            AutomationRunState::Compensating,
            AutomationRunState::Completed,
            AutomationRunState::Failed,
        ] {
            assert_eq!(AutomationRunState::parse(state.as_str()).unwrap(), state);
        }
    }

    #[test]
    fn parse_rejects_unknown() {
        assert!(matches!(
            AutomationRunState::parse("escalated"),
            Err(AutomationRunStateError::Unknown(_))
        ));
    }

    #[test]
    fn terminal_classification_is_completed_and_failed_only() {
        assert!(AutomationRunState::Completed.is_terminal());
        assert!(AutomationRunState::Failed.is_terminal());
        for non_terminal in [
            AutomationRunState::Queued,
            AutomationRunState::Running,
            AutomationRunState::Suspended,
            AutomationRunState::Compensating,
        ] {
            assert!(
                !non_terminal.is_terminal(),
                "{non_terminal} must not be terminal"
            );
        }
    }

    #[test]
    fn allowed_transitions_match_module_doc() {
        let allowed = [
            (AutomationRunState::Queued, AutomationRunState::Running),
            (AutomationRunState::Queued, AutomationRunState::Failed),
            (AutomationRunState::Running, AutomationRunState::Completed),
            (AutomationRunState::Running, AutomationRunState::Failed),
            (AutomationRunState::Running, AutomationRunState::Suspended),
            (
                AutomationRunState::Running,
                AutomationRunState::Compensating,
            ),
            (AutomationRunState::Suspended, AutomationRunState::Running),
            (AutomationRunState::Suspended, AutomationRunState::Failed),
            (AutomationRunState::Compensating, AutomationRunState::Failed),
        ];
        for (from, to) in allowed {
            assert!(from.can_transition_to(to), "expected {from} → {to}");
        }
    }

    #[test]
    fn forbidden_transitions_are_rejected() {
        let forbidden = [
            // Cannot go back to Queued from anywhere
            (AutomationRunState::Running, AutomationRunState::Queued),
            (AutomationRunState::Suspended, AutomationRunState::Queued),
            // Terminal → anything else
            (AutomationRunState::Completed, AutomationRunState::Running),
            (AutomationRunState::Failed, AutomationRunState::Running),
            (AutomationRunState::Completed, AutomationRunState::Failed),
            // Compensating cannot go back to Running / Suspended
            (
                AutomationRunState::Compensating,
                AutomationRunState::Running,
            ),
            (
                AutomationRunState::Compensating,
                AutomationRunState::Completed,
            ),
            // Queued cannot jump straight to Suspended / Compensating /
            // Completed (must go via Running first)
            (AutomationRunState::Queued, AutomationRunState::Suspended),
            (AutomationRunState::Queued, AutomationRunState::Completed),
            (AutomationRunState::Queued, AutomationRunState::Compensating),
        ];
        for (from, to) in forbidden {
            assert!(
                !from.can_transition_to(to),
                "transition {from} → {to} should be rejected"
            );
        }
    }

    #[test]
    fn self_transitions_are_idempotent() {
        for state in [
            AutomationRunState::Queued,
            AutomationRunState::Running,
            AutomationRunState::Suspended,
            AutomationRunState::Compensating,
            AutomationRunState::Completed,
            AutomationRunState::Failed,
        ] {
            assert!(state.can_transition_to(state));
        }
    }

    #[test]
    fn happy_path_queued_running_completed() {
        let run = fresh_run();
        let claimed = run.transition(AutomationRunEvent::Claim).expect("claim");
        assert_eq!(claimed.state, AutomationRunState::Running);
        assert_eq!(claimed.attempts, 1);

        let completed = claimed
            .transition(AutomationRunEvent::EffectCompleted {
                response: json!({"ok": true}),
            })
            .expect("complete");
        assert_eq!(completed.state, AutomationRunState::Completed);
        assert_eq!(completed.effect_response, Some(json!({"ok": true})));
        assert!(completed.last_error.is_none());
        assert!(completed.expires_at.is_none());
    }

    #[test]
    fn pre_flight_failure_jumps_queued_to_failed_directly() {
        let run = fresh_run();
        let failed = run
            .transition(AutomationRunEvent::PreFlightFailed {
                error: "missing action_id".into(),
            })
            .expect("pre-flight reject");
        assert_eq!(failed.state, AutomationRunState::Failed);
        assert_eq!(failed.last_error.as_deref(), Some("missing action_id"));
        assert_eq!(failed.attempts, 0); // never claimed
    }

    #[test]
    fn effect_failure_records_error_and_clears_expires_at() {
        let mut run = fresh_run();
        run.expires_at = Some(Utc::now() + chrono::Duration::minutes(1));
        let claimed = run.transition(AutomationRunEvent::Claim).unwrap();
        let failed = claimed
            .transition(AutomationRunEvent::EffectFailed {
                error: "ontology action returned 503".into(),
            })
            .expect("effect failed");
        assert_eq!(failed.state, AutomationRunState::Failed);
        assert_eq!(
            failed.last_error.as_deref(),
            Some("ontology action returned 503")
        );
        assert!(failed.expires_at.is_none());
    }

    #[test]
    fn suspend_resume_cycle_advances_attempts_and_clears_deadline() {
        let run = fresh_run();
        let claimed = run.transition(AutomationRunEvent::Claim).unwrap();
        let wait_until = Utc::now() + chrono::Duration::hours(1);
        let suspended = claimed
            .transition(AutomationRunEvent::Suspend {
                wait_until: Some(wait_until),
            })
            .expect("suspend");
        assert_eq!(suspended.state, AutomationRunState::Suspended);
        assert_eq!(suspended.expires_at, Some(wait_until));

        let resumed = suspended
            .transition(AutomationRunEvent::Resume)
            .expect("resume");
        assert_eq!(resumed.state, AutomationRunState::Running);
        assert_eq!(resumed.attempts, 2);
        assert!(resumed.expires_at.is_none());
    }

    #[test]
    fn suspension_timeout_is_terminal_failure() {
        let run = fresh_run();
        let claimed = run.transition(AutomationRunEvent::Claim).unwrap();
        let suspended = claimed
            .transition(AutomationRunEvent::Suspend { wait_until: None })
            .unwrap();
        let failed = suspended
            .transition(AutomationRunEvent::SuspensionFailed {
                error: "wait deadline exceeded".into(),
            })
            .expect("suspension timeout");
        assert_eq!(failed.state, AutomationRunState::Failed);
        assert_eq!(failed.last_error.as_deref(), Some("wait deadline exceeded"));
    }

    #[test]
    fn compensating_chain_lands_in_failed() {
        let run = fresh_run();
        let claimed = run.transition(AutomationRunEvent::Claim).unwrap();
        let compensating = claimed
            .transition(AutomationRunEvent::StartCompensating)
            .expect("start compensating");
        assert_eq!(compensating.state, AutomationRunState::Compensating);

        let failed = compensating
            .transition(AutomationRunEvent::CompensationCompleted {
                error: "rolled back: dataset write reverted".into(),
            })
            .expect("compensation completed");
        assert_eq!(failed.state, AutomationRunState::Failed);
        assert_eq!(
            failed.last_error.as_deref(),
            Some("rolled back: dataset write reverted")
        );
    }

    #[test]
    fn invalid_event_in_terminal_state_is_rejected() {
        let run = fresh_run();
        let claimed = run.transition(AutomationRunEvent::Claim).unwrap();
        let completed = claimed
            .transition(AutomationRunEvent::EffectCompleted {
                response: json!({}),
            })
            .unwrap();
        // Re-claiming a completed run is nonsense.
        let result = completed.transition(AutomationRunEvent::Claim);
        assert!(result.is_err());
    }

    #[test]
    fn state_data_round_trips_through_serde_json() {
        let run = fresh_run();
        let claimed = run.transition(AutomationRunEvent::Claim).unwrap();
        let payload = serde_json::to_value(&claimed).expect("serialize");
        let decoded: AutomationRun = serde_json::from_value(payload).expect("deserialize");
        assert_eq!(decoded.state, AutomationRunState::Running);
        assert_eq!(decoded.attempts, 1);
        // Confirms the field-level snake_case rename for the state enum
        // matches the SQL `CHECK` constraint values.
        let serialized = serde_json::to_value(decoded.state).unwrap();
        assert_eq!(serialized, serde_json::Value::String("running".into()));
    }

    #[test]
    fn state_machine_state_str_matches_wire_format() {
        // Defence in depth: the PgStore writes `state_str(state)` into
        // the `state` column, so it must produce values the SQL CHECK
        // constraint accepts.
        for state in [
            AutomationRunState::Queued,
            AutomationRunState::Running,
            AutomationRunState::Suspended,
            AutomationRunState::Compensating,
            AutomationRunState::Completed,
            AutomationRunState::Failed,
        ] {
            assert_eq!(
                <AutomationRun as StateMachine>::state_str(state),
                state.as_str()
            );
        }
    }
}
