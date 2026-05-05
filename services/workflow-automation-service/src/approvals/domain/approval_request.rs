//! `ApprovalRequest` aggregate — pure state machine for the
//! Foundry-pattern replacement of the Temporal
//! `ApprovalRequestWorkflow`.
//!
//! FASE 7 / Tarea 7.3 deliverable. Backs the
//! `audit_compliance.approval_requests` table created by
//! `migrations/20260504400000_approval_requests_state_machine.sql`.
//!
//! ## State table
//!
//! Allowed transitions (also enforced server-side by the SQL
//! `CHECK` constraint plus the application-level
//! `ApprovalRequestState::can_transition_to` guard):
//!
//! ```text
//!   Pending    → Approved      (POST /approvals/{id}/decide → approve)
//!   Pending    → Rejected      (POST /approvals/{id}/decide → reject)
//!   Pending    → Expired       (timeout sweep CronJob — Tarea 7.4)
//!   Pending    → Escalated     (RESERVED — no caller in-tree today)
//!   Escalated  → Approved      (RESERVED — escalation flow)
//!   Escalated  → Rejected      (RESERVED — escalation flow)
//!   Escalated  → Expired       (RESERVED — escalation timeout)
//!   <terminal> → <self>        (idempotent re-application — safe)
//! ```
//!
//! `Approved`, `Rejected`, `Expired` are terminal. `Escalated` is
//! the only re-enterable non-terminal state. The `Escalated` arm
//! exists so the schema accepts the future time-based-escalation
//! kind from the migration plan §7.1 taxonomy without another
//! schema migration when a real caller asks for it.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use state_machine::{StateMachine, TransitionError};
use std::fmt;
use uuid::Uuid;

/// Lifecycle of a row in `audit_compliance.approval_requests`.
///
/// Wire format (lowercase) matches the SQL `CHECK` constraint in
/// the migration; do not rename without coordinating the schema
/// migration in lockstep.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ApprovalRequestState {
    Pending,
    Approved,
    Rejected,
    Expired,
    /// RESERVED for future time-based escalation (migration plan
    /// §7.1). No code emits this state today; the column accepts
    /// it so a future caller does not need a schema migration.
    Escalated,
}

impl ApprovalRequestState {
    pub fn as_str(self) -> &'static str {
        match self {
            ApprovalRequestState::Pending => "pending",
            ApprovalRequestState::Approved => "approved",
            ApprovalRequestState::Rejected => "rejected",
            ApprovalRequestState::Expired => "expired",
            ApprovalRequestState::Escalated => "escalated",
        }
    }

    pub fn parse(value: &str) -> Result<Self, ApprovalRequestStateError> {
        match value {
            "pending" => Ok(ApprovalRequestState::Pending),
            "approved" => Ok(ApprovalRequestState::Approved),
            "rejected" => Ok(ApprovalRequestState::Rejected),
            "expired" => Ok(ApprovalRequestState::Expired),
            "escalated" => Ok(ApprovalRequestState::Escalated),
            other => Err(ApprovalRequestStateError::Unknown(other.to_string())),
        }
    }

    /// Terminal states: `Approved`, `Rejected`, `Expired`. A
    /// caller that holds a terminal row never needs to issue
    /// further transitions.
    pub fn is_terminal(self) -> bool {
        matches!(
            self,
            ApprovalRequestState::Approved
                | ApprovalRequestState::Rejected
                | ApprovalRequestState::Expired
        )
    }

    /// Validate a proposed transition `self → next` against the
    /// table in the module-level doc comment.
    pub fn can_transition_to(self, next: ApprovalRequestState) -> bool {
        if self == next {
            return true;
        }
        matches!(
            (self, next),
            (
                ApprovalRequestState::Pending,
                ApprovalRequestState::Approved
            ) | (
                ApprovalRequestState::Pending,
                ApprovalRequestState::Rejected
            ) | (ApprovalRequestState::Pending, ApprovalRequestState::Expired)
                | (
                    ApprovalRequestState::Pending,
                    ApprovalRequestState::Escalated
                )
                | (
                    ApprovalRequestState::Escalated,
                    ApprovalRequestState::Approved
                )
                | (
                    ApprovalRequestState::Escalated,
                    ApprovalRequestState::Rejected
                )
                | (
                    ApprovalRequestState::Escalated,
                    ApprovalRequestState::Expired
                )
        )
    }

    pub fn validate_transition(
        self,
        next: ApprovalRequestState,
    ) -> Result<(), ApprovalRequestStateError> {
        if self.can_transition_to(next) {
            Ok(())
        } else {
            Err(ApprovalRequestStateError::IllegalTransition {
                from: self,
                to: next,
            })
        }
    }
}

impl fmt::Display for ApprovalRequestState {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(self.as_str())
    }
}

#[derive(Debug, thiserror::Error)]
pub enum ApprovalRequestStateError {
    #[error("unknown approval request state {0:?}")]
    Unknown(String),
    #[error("illegal approval transition {from} → {to}")]
    IllegalTransition {
        from: ApprovalRequestState,
        to: ApprovalRequestState,
    },
}

/// Domain events the ApprovalRequest aggregate accepts. Each
/// variant triggers exactly one transition in the table above.
#[derive(Debug, Clone)]
pub enum ApprovalRequestEvent {
    /// Decider chose to approve.
    Approve {
        decided_by: String,
        comment: Option<String>,
    },
    /// Decider chose to reject.
    Reject {
        decided_by: String,
        comment: Option<String>,
    },
    /// Timeout sweep CronJob: expires_at <= now() and the row is
    /// still pending (or escalated).
    Expire { expired_at: DateTime<Utc> },
    /// RESERVED for the future escalation flow.
    Escalate { escalated_at: DateTime<Utc> },
}

/// Aggregate persisted in the `state_data` JSON column.
/// Operator-facing summaries (`tenant_id`, `subject`,
/// `correlation_id`, the rendered state, `expires_at`) are also
/// projected onto dedicated columns by the SQL migration so
/// dashboards do not have to crack open this JSON.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct ApprovalRequest {
    pub id: Uuid,
    pub tenant_id: String,
    pub subject: String,
    pub approver_set: Vec<String>,
    /// Free-form action payload the caller passed alongside the
    /// approval (mirrors the legacy `ApprovalRequestInput::action_payload`).
    #[serde(default)]
    pub action_payload: serde_json::Value,
    pub correlation_id: Uuid,
    pub state: ApprovalRequestState,
    /// Wall-clock deadline. `None` means "no deadline" — the
    /// timeout sweep skips these rows.
    #[serde(default)]
    pub expires_at: Option<DateTime<Utc>>,
    /// Stamped on the aggregate when `state` lands in
    /// Approved / Rejected. None on Expired / Pending / Escalated.
    #[serde(default)]
    pub decided_by: Option<String>,
    #[serde(default)]
    pub decided_at: Option<DateTime<Utc>>,
    #[serde(default)]
    pub comment: Option<String>,
}

impl ApprovalRequest {
    /// Build a fresh row in the `Pending` state.
    pub fn new(
        id: Uuid,
        tenant_id: impl Into<String>,
        subject: impl Into<String>,
        approver_set: Vec<String>,
        action_payload: serde_json::Value,
        correlation_id: Uuid,
        expires_at: Option<DateTime<Utc>>,
    ) -> Self {
        Self {
            id,
            tenant_id: tenant_id.into(),
            subject: subject.into(),
            approver_set,
            action_payload,
            correlation_id,
            state: ApprovalRequestState::Pending,
            expires_at,
            decided_by: None,
            decided_at: None,
            comment: None,
        }
    }
}

impl StateMachine for ApprovalRequest {
    type State = ApprovalRequestState;
    type Event = ApprovalRequestEvent;

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
        use ApprovalRequestEvent::*;
        use ApprovalRequestState::*;

        let next = match (self.state, &event) {
            (
                Pending,
                Approve {
                    decided_by,
                    comment,
                },
            )
            | (
                Escalated,
                Approve {
                    decided_by,
                    comment,
                },
            ) => {
                self.decided_by = Some(decided_by.clone());
                self.decided_at = Some(Utc::now());
                self.comment = comment.clone();
                self.expires_at = None;
                Approved
            }
            (
                Pending,
                Reject {
                    decided_by,
                    comment,
                },
            )
            | (
                Escalated,
                Reject {
                    decided_by,
                    comment,
                },
            ) => {
                self.decided_by = Some(decided_by.clone());
                self.decided_at = Some(Utc::now());
                self.comment = comment.clone();
                self.expires_at = None;
                Rejected
            }
            (Pending, Expire { expired_at }) | (Escalated, Expire { expired_at }) => {
                self.decided_at = Some(*expired_at);
                self.expires_at = None;
                Expired
            }
            (Pending, Escalate { escalated_at: _ }) => Escalated,
            (current, evt) => {
                return Err(TransitionError::invalid(format!(
                    "no ApprovalRequest transition from {current} for event {evt:?}",
                )));
            }
        };

        self.state.validate_transition(next).map_err(|err| {
            TransitionError::invalid(format!(
                "ApprovalRequest produced disallowed transition: {err}"
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

    fn fresh_request() -> ApprovalRequest {
        ApprovalRequest::new(
            Uuid::now_v7(),
            "acme",
            "Promote customer",
            vec!["alice".to_string()],
            json!({"customer_id": "c-1"}),
            Uuid::now_v7(),
            Some(Utc::now() + chrono::Duration::hours(24)),
        )
    }

    #[test]
    fn parse_round_trips_every_state() {
        for state in [
            ApprovalRequestState::Pending,
            ApprovalRequestState::Approved,
            ApprovalRequestState::Rejected,
            ApprovalRequestState::Expired,
            ApprovalRequestState::Escalated,
        ] {
            assert_eq!(ApprovalRequestState::parse(state.as_str()).unwrap(), state);
        }
    }

    #[test]
    fn parse_rejects_unknown() {
        assert!(matches!(
            ApprovalRequestState::parse("withdrawn"),
            Err(ApprovalRequestStateError::Unknown(_))
        ));
    }

    #[test]
    fn terminal_classification_excludes_pending_and_escalated() {
        assert!(ApprovalRequestState::Approved.is_terminal());
        assert!(ApprovalRequestState::Rejected.is_terminal());
        assert!(ApprovalRequestState::Expired.is_terminal());
        assert!(!ApprovalRequestState::Pending.is_terminal());
        assert!(!ApprovalRequestState::Escalated.is_terminal());
    }

    #[test]
    fn allowed_transitions_match_module_doc() {
        let allowed = [
            (
                ApprovalRequestState::Pending,
                ApprovalRequestState::Approved,
            ),
            (
                ApprovalRequestState::Pending,
                ApprovalRequestState::Rejected,
            ),
            (ApprovalRequestState::Pending, ApprovalRequestState::Expired),
            (
                ApprovalRequestState::Pending,
                ApprovalRequestState::Escalated,
            ),
            (
                ApprovalRequestState::Escalated,
                ApprovalRequestState::Approved,
            ),
            (
                ApprovalRequestState::Escalated,
                ApprovalRequestState::Rejected,
            ),
            (
                ApprovalRequestState::Escalated,
                ApprovalRequestState::Expired,
            ),
        ];
        for (from, to) in allowed {
            assert!(from.can_transition_to(to), "expected {from} → {to}");
        }
    }

    #[test]
    fn forbidden_transitions_are_rejected() {
        let forbidden = [
            // Terminal states cannot move
            (
                ApprovalRequestState::Approved,
                ApprovalRequestState::Pending,
            ),
            (
                ApprovalRequestState::Rejected,
                ApprovalRequestState::Pending,
            ),
            (ApprovalRequestState::Expired, ApprovalRequestState::Pending),
            (
                ApprovalRequestState::Approved,
                ApprovalRequestState::Rejected,
            ),
            (
                ApprovalRequestState::Approved,
                ApprovalRequestState::Expired,
            ),
            // Cannot escalate from terminal
            (
                ApprovalRequestState::Approved,
                ApprovalRequestState::Escalated,
            ),
            // Cannot escalate from escalated (no nested escalation today)
            (
                ApprovalRequestState::Escalated,
                ApprovalRequestState::Escalated,
            ),
        ];
        // Note: Escalated → Escalated is technically the self-loop,
        // which is actually allowed (idempotent). Drop it from the
        // forbidden list; the rest must be rejected.
        let forbidden: Vec<_> = forbidden
            .into_iter()
            .filter(|(from, to)| from != to)
            .collect();
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
            ApprovalRequestState::Pending,
            ApprovalRequestState::Approved,
            ApprovalRequestState::Rejected,
            ApprovalRequestState::Expired,
            ApprovalRequestState::Escalated,
        ] {
            assert!(state.can_transition_to(state));
        }
    }

    #[test]
    fn approve_records_decider_and_clears_deadline() {
        let req = fresh_request();
        assert!(req.expires_at.is_some());
        let approved = req
            .transition(ApprovalRequestEvent::Approve {
                decided_by: "alice".into(),
                comment: Some("LGTM".into()),
            })
            .expect("approve");
        assert_eq!(approved.state, ApprovalRequestState::Approved);
        assert_eq!(approved.decided_by.as_deref(), Some("alice"));
        assert_eq!(approved.comment.as_deref(), Some("LGTM"));
        assert!(approved.decided_at.is_some());
        assert!(approved.expires_at.is_none());
    }

    #[test]
    fn reject_records_decider_without_comment() {
        let req = fresh_request();
        let rejected = req
            .transition(ApprovalRequestEvent::Reject {
                decided_by: "bob".into(),
                comment: None,
            })
            .expect("reject");
        assert_eq!(rejected.state, ApprovalRequestState::Rejected);
        assert_eq!(rejected.decided_by.as_deref(), Some("bob"));
        assert!(rejected.comment.is_none());
    }

    #[test]
    fn expire_clears_deadline_but_does_not_set_decider() {
        let req = fresh_request();
        let when = Utc::now();
        let expired = req
            .transition(ApprovalRequestEvent::Expire { expired_at: when })
            .expect("expire");
        assert_eq!(expired.state, ApprovalRequestState::Expired);
        assert!(expired.decided_by.is_none());
        assert_eq!(expired.decided_at, Some(when));
        assert!(expired.expires_at.is_none());
    }

    #[test]
    fn escalate_keeps_pending_aggregate_alive() {
        let req = fresh_request();
        let when = Utc::now();
        let escalated = req
            .transition(ApprovalRequestEvent::Escalate { escalated_at: when })
            .expect("escalate");
        assert_eq!(escalated.state, ApprovalRequestState::Escalated);
        assert!(escalated.decided_by.is_none());
        // The deadline survives the escalation — a second timeout
        // sweep against the same row will still pick it up.
        assert!(escalated.expires_at.is_some());
    }

    #[test]
    fn escalated_can_still_be_decided() {
        let req = fresh_request()
            .transition(ApprovalRequestEvent::Escalate {
                escalated_at: Utc::now(),
            })
            .unwrap();
        let approved = req
            .transition(ApprovalRequestEvent::Approve {
                decided_by: "carol".into(),
                comment: None,
            })
            .expect("approve from escalated");
        assert_eq!(approved.state, ApprovalRequestState::Approved);
        assert_eq!(approved.decided_by.as_deref(), Some("carol"));
    }

    #[test]
    fn invalid_event_in_terminal_state_is_rejected() {
        let req = fresh_request();
        let approved = req
            .transition(ApprovalRequestEvent::Approve {
                decided_by: "alice".into(),
                comment: None,
            })
            .unwrap();
        let result = approved.transition(ApprovalRequestEvent::Approve {
            decided_by: "alice".into(),
            comment: None,
        });
        assert!(result.is_err());
    }

    #[test]
    fn state_data_round_trips_through_serde_json() {
        let req = fresh_request();
        let payload = serde_json::to_value(&req).expect("serialize");
        let decoded: ApprovalRequest = serde_json::from_value(payload).expect("deserialize");
        assert_eq!(req, decoded);
    }

    #[test]
    fn state_machine_state_str_matches_wire_format() {
        for state in [
            ApprovalRequestState::Pending,
            ApprovalRequestState::Approved,
            ApprovalRequestState::Rejected,
            ApprovalRequestState::Expired,
            ApprovalRequestState::Escalated,
        ] {
            assert_eq!(
                <ApprovalRequest as StateMachine>::state_str(state),
                state.as_str()
            );
        }
    }
}
