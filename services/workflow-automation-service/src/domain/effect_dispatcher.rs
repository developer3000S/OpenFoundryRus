//! HTTP effect dispatcher — calls `ontology-actions-service::POST
//! /api/v1/ontology/actions/{action_id}/execute` on behalf of an
//! AutomationRun.
//!
//! Mirrors the legacy Go activity contract from
//! `workers-go/workflow-automation/activities/activities.go::ExecuteOntologyAction`
//! byte-for-byte:
//!
//! * Same endpoint, same request body shape (`{parameters,
//!   target_object_id?, justification?}`), same headers
//!   (`authorization`, `content-type`, `x-audit-correlation-id`).
//! * Same trigger-payload extraction: root `action_id` first, falls
//!   back to `ontology_action.action_id`.
//! * Same retry envelope: 5 attempts, 30s initial → 10m max,
//!   exponential backoff (matches `automation_run.go::ao.RetryPolicy`).
//! * Same error classification: 4xx (except 429) is non-retryable
//!   ("invalid input" — the run lands in `Failed` immediately);
//!   5xx, 429 and transport errors retry within the envelope.
//!
//! Keeping the contract identical means the downstream `ontology-
//! actions-service` Rust handler does not need to change as part of
//! the FASE 5 cutover.

use std::time::Duration;

use reqwest::{Client, StatusCode};
use serde_json::{Value, json};
use thiserror::Error;
use tracing::warn;
use uuid::Uuid;

const HEADER_AUDIT_CORRELATION: &str = "x-audit-correlation-id";

/// Operator-tunable retry envelope. Defaults match the legacy Go
/// activity (`workers-go/workflow-automation/workflows/automation_run.go::ao`).
#[derive(Debug, Clone, Copy)]
pub struct RetryPolicy {
    /// Maximum number of attempts (including the first).
    pub max_attempts: u32,
    /// Initial backoff before the second attempt.
    pub initial_backoff: Duration,
    /// Cap on the backoff between attempts.
    pub max_backoff: Duration,
    /// Multiplier applied to the backoff after each failed attempt.
    pub backoff_multiplier: f32,
}

impl Default for RetryPolicy {
    fn default() -> Self {
        Self {
            max_attempts: 5,
            initial_backoff: Duration::from_secs(30),
            max_backoff: Duration::from_secs(600),
            backoff_multiplier: 2.0,
        }
    }
}

impl RetryPolicy {
    /// Compute the delay before attempt `attempt_number` (1-indexed,
    /// so `next_backoff(2)` is the delay between attempts 1 and 2).
    /// Returns `Duration::ZERO` for the very first attempt.
    pub fn next_backoff(&self, attempt_number: u32) -> Duration {
        if attempt_number <= 1 {
            return Duration::ZERO;
        }
        let exponent = attempt_number.saturating_sub(2);
        let raw =
            self.initial_backoff.as_secs_f32() * self.backoff_multiplier.powi(exponent as i32);
        let secs = raw.min(self.max_backoff.as_secs_f32()).max(0.0);
        Duration::from_secs_f32(secs)
    }
}

/// Effect dispatcher errors. The `Retryable` / `NonRetryable`
/// classification drives the consumer's retry envelope (mirror of
/// the Temporal `nonRetryable` markers in the legacy Go activity).
#[derive(Debug, Error)]
pub enum DispatchError {
    /// Trigger payload missing or malformed — the run cannot proceed.
    /// Consumer routes this to `state=Failed` immediately.
    #[error("invalid trigger payload: {0}")]
    InvalidPayload(String),

    /// Service URL / bearer token misconfigured. Same outcome as
    /// `InvalidPayload` from the consumer's perspective; flagged
    /// separately to make root-cause attribution easier in metrics.
    #[error("ontology-actions-service unconfigured: {0}")]
    Unconfigured(String),

    /// Upstream returned a 4xx (other than 429) — request was
    /// rejected on its merits, retrying makes no sense.
    #[error("upstream non-retryable {status}: {message}")]
    NonRetryable { status: u16, message: String },

    /// Upstream returned a 5xx, a 429, or the transport itself
    /// failed. Consumer applies the [`RetryPolicy`] backoff and
    /// re-attempts; after exhaustion this is reported via
    /// [`DispatchError::Exhausted`].
    #[error("upstream retryable error: {0}")]
    Retryable(String),

    /// Retry envelope exhausted. Consumer transitions the run to
    /// `state=Failed` with the underlying message attached.
    #[error("retry envelope exhausted after {attempts} attempts: {message}")]
    Exhausted { attempts: u32, message: String },
}

impl DispatchError {
    /// `true` for any error variant that should land the run in a
    /// terminal `Failed` state without further attempts.
    pub fn is_terminal(&self) -> bool {
        matches!(
            self,
            DispatchError::InvalidPayload(_)
                | DispatchError::Unconfigured(_)
                | DispatchError::NonRetryable { .. }
                | DispatchError::Exhausted { .. }
        )
    }
}

/// Pure extraction of the ontology-action invocation from a
/// `trigger_payload` JSON value. Mirrors the legacy Go function
/// `ontologyActionRequestFromInput` (root `action_id` first, falls
/// back to nested `ontology_action.action_id`).
pub fn extract_action_request(
    trigger_payload: &Value,
    correlation_id: Uuid,
) -> Result<OntologyActionRequest, DispatchError> {
    let scope = trigger_payload
        .get("ontology_action")
        .filter(|value| value.is_object())
        .unwrap_or(trigger_payload);
    let action_id = scope
        .get("action_id")
        .and_then(Value::as_str)
        .map(str::to_string)
        .filter(|value| !value.trim().is_empty())
        .ok_or_else(|| {
            DispatchError::InvalidPayload(
                "trigger_payload.action_id is required to dispatch an ontology action".into(),
            )
        })?;

    Ok(OntologyActionRequest {
        action_id,
        target_object_id: scope
            .get("target_object_id")
            .and_then(Value::as_str)
            .map(str::to_string)
            .filter(|value| !value.is_empty()),
        parameters: scope
            .get("parameters")
            .cloned()
            .filter(Value::is_object)
            .unwrap_or_else(|| json!({})),
        justification: scope
            .get("justification")
            .and_then(Value::as_str)
            .map(str::to_string)
            .filter(|value| !value.is_empty()),
        audit_correlation_id: correlation_id,
    })
}

/// Materialised request handed to the dispatcher. Kept separate from
/// the wire-format `AutomateConditionV1` so the dispatcher contract
/// is testable without going through serde.
#[derive(Debug, Clone)]
pub struct OntologyActionRequest {
    pub action_id: String,
    pub target_object_id: Option<String>,
    pub parameters: Value,
    pub justification: Option<String>,
    pub audit_correlation_id: Uuid,
}

impl OntologyActionRequest {
    fn body(&self) -> Value {
        let mut body = json!({ "parameters": self.parameters });
        if let Some(target) = &self.target_object_id {
            body["target_object_id"] = json!(target);
        }
        if let Some(justification) = &self.justification {
            body["justification"] = json!(justification);
        }
        body
    }
}

/// HTTP effect dispatcher. Cheap to clone (wraps an `Arc`-shared
/// `reqwest::Client`).
#[derive(Debug, Clone)]
pub struct EffectDispatcher {
    client: Client,
    base_url: String,
    bearer_token: String,
}

impl EffectDispatcher {
    /// Build a dispatcher from an in-memory `reqwest::Client` plus
    /// the upstream config. The base URL must include the scheme.
    pub fn new(
        client: Client,
        base_url: impl Into<String>,
        bearer_token: impl Into<String>,
    ) -> Self {
        Self {
            client,
            base_url: normalize_base_url(base_url.into()),
            bearer_token: normalize_bearer_token(bearer_token.into()),
        }
    }

    /// One-shot dispatch (single attempt, no retries). Returns the
    /// decoded JSON response on 2xx; classifies errors via
    /// [`DispatchError`] otherwise.
    ///
    /// The retry envelope lives in [`Self::dispatch_with_retries`]
    /// so this function stays trivially testable against `wiremock`.
    pub async fn dispatch_once(
        &self,
        request: &OntologyActionRequest,
    ) -> Result<Value, DispatchError> {
        if self.base_url.trim().is_empty() {
            return Err(DispatchError::Unconfigured(
                "OF_ONTOLOGY_ACTIONS_URL is empty".into(),
            ));
        }
        if self.bearer_token.trim().is_empty() {
            return Err(DispatchError::Unconfigured(
                "OF_ONTOLOGY_ACTIONS_BEARER_TOKEN is empty".into(),
            ));
        }

        let url = format!(
            "{}/api/v1/ontology/actions/{}/execute",
            self.base_url.trim_end_matches('/'),
            urlencoding::encode(&request.action_id)
        );
        let response = self
            .client
            .post(&url)
            .header(reqwest::header::AUTHORIZATION, &self.bearer_token)
            .header(reqwest::header::CONTENT_TYPE, "application/json")
            .header(
                HEADER_AUDIT_CORRELATION,
                request.audit_correlation_id.to_string(),
            )
            .json(&request.body())
            .send()
            .await
            .map_err(|err| {
                // Connection refused / DNS / timeout — retryable per
                // the legacy contract (5xx-like).
                DispatchError::Retryable(format!("transport error: {err}"))
            })?;

        let status = response.status();
        let body_bytes = response.bytes().await.map_err(|err| {
            DispatchError::Retryable(format!("failed to read response body: {err}"))
        })?;
        let payload = decode_json(&body_bytes);

        if status.is_success() {
            return Ok(payload);
        }

        let message = response_message(&payload, &body_bytes);
        if status.is_client_error() && status != StatusCode::TOO_MANY_REQUESTS {
            return Err(DispatchError::NonRetryable {
                status: status.as_u16(),
                message,
            });
        }
        Err(DispatchError::Retryable(format!(
            "ontology action returned {}: {message}",
            status.as_u16()
        )))
    }

    /// Dispatch with bounded retries. Sleeps between attempts per
    /// the supplied [`RetryPolicy`]; surfaces non-retryable errors
    /// immediately and wraps the final retryable failure as
    /// [`DispatchError::Exhausted`].
    pub async fn dispatch_with_retries(
        &self,
        request: &OntologyActionRequest,
        policy: RetryPolicy,
    ) -> Result<DispatchOutcome, DispatchError> {
        let mut attempt: u32 = 0;
        loop {
            attempt += 1;
            match self.dispatch_once(request).await {
                Ok(response) => {
                    return Ok(DispatchOutcome {
                        response,
                        attempts: attempt,
                    });
                }
                Err(err) if err.is_terminal() => return Err(err),
                Err(err) => {
                    if attempt >= policy.max_attempts {
                        return Err(DispatchError::Exhausted {
                            attempts: attempt,
                            message: err.to_string(),
                        });
                    }
                    let backoff = policy.next_backoff(attempt + 1);
                    warn!(
                        attempt,
                        next_attempt = attempt + 1,
                        backoff_ms = backoff.as_millis() as u64,
                        error = %err,
                        "ontology action dispatch failed; retrying after backoff"
                    );
                    tokio::time::sleep(backoff).await;
                }
            }
        }
    }
}

/// Successful dispatch result handed back to the consumer so it can
/// stamp the `attempts` count on the outbound `automate.outcome.v1`.
#[derive(Debug, Clone)]
pub struct DispatchOutcome {
    pub response: Value,
    pub attempts: u32,
}

fn normalize_base_url(raw: String) -> String {
    let trimmed = raw.trim().to_string();
    if trimmed.is_empty() || trimmed.contains("://") {
        trimmed
    } else {
        format!("http://{trimmed}")
    }
}

fn normalize_bearer_token(raw: String) -> String {
    let trimmed = raw.trim().to_string();
    if trimmed.is_empty() {
        return trimmed;
    }
    if trimmed.to_ascii_lowercase().starts_with("bearer ") {
        trimmed
    } else {
        format!("Bearer {trimmed}")
    }
}

fn decode_json(body: &[u8]) -> Value {
    if body.iter().all(u8::is_ascii_whitespace) {
        return json!({});
    }
    serde_json::from_slice::<Value>(body)
        .unwrap_or_else(|_| json!({ "raw": String::from_utf8_lossy(body).to_string() }))
}

fn response_message(payload: &Value, body: &[u8]) -> String {
    for key in ["error", "message", "details"] {
        if let Some(value) = payload.get(key) {
            if let Some(text) = value.as_str() {
                if !text.is_empty() {
                    return text.to_string();
                }
            }
            if !value.is_null() {
                return value.to_string();
            }
        }
    }
    if body.iter().all(u8::is_ascii_whitespace) {
        return "upstream error".to_string();
    }
    String::from_utf8_lossy(body).to_string()
}

mod urlencoding {
    /// Minimal percent-encoder for action ids — only the unreserved
    /// set per RFC 3986 (alphanum, `-`, `.`, `_`, `~`) passes
    /// through unchanged. Avoids pulling in a full URL crate just
    /// for this single field.
    pub fn encode(value: &str) -> String {
        let mut out = String::with_capacity(value.len());
        for byte in value.bytes() {
            let safe = byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'.' | b'_' | b'~');
            if safe {
                out.push(byte as char);
            } else {
                out.push_str(&format!("%{:02X}", byte));
            }
        }
        out
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn extract_root_action_request() {
        let payload = json!({
            "action_id": "promote",
            "target_object_id": "obj-1",
            "parameters": {"priority": "high"},
            "justification": "policy"
        });
        let req = extract_action_request(&payload, Uuid::nil()).expect("extract");
        assert_eq!(req.action_id, "promote");
        assert_eq!(req.target_object_id.as_deref(), Some("obj-1"));
        assert_eq!(req.parameters, json!({"priority": "high"}));
        assert_eq!(req.justification.as_deref(), Some("policy"));
    }

    #[test]
    fn extract_nested_action_request_falls_back_correctly() {
        let payload = json!({
            "ontology_action": {
                "action_id": "promote",
                "parameters": {"x": 1}
            },
            "extra": "noise"
        });
        let req = extract_action_request(&payload, Uuid::nil()).expect("extract");
        assert_eq!(req.action_id, "promote");
        assert_eq!(req.parameters, json!({"x": 1}));
        assert!(req.target_object_id.is_none());
    }

    #[test]
    fn extract_rejects_missing_action_id() {
        let payload = json!({"parameters": {"x": 1}});
        let err = extract_action_request(&payload, Uuid::nil()).expect_err("must reject");
        assert!(matches!(err, DispatchError::InvalidPayload(_)));
    }

    #[test]
    fn extract_rejects_empty_action_id() {
        let payload = json!({"action_id": "   "});
        let err = extract_action_request(&payload, Uuid::nil()).expect_err("must reject");
        assert!(matches!(err, DispatchError::InvalidPayload(_)));
    }

    #[test]
    fn dispatch_error_terminal_classification() {
        assert!(DispatchError::InvalidPayload("x".into()).is_terminal());
        assert!(DispatchError::Unconfigured("y".into()).is_terminal());
        assert!(
            DispatchError::NonRetryable {
                status: 400,
                message: "z".into()
            }
            .is_terminal()
        );
        assert!(
            DispatchError::Exhausted {
                attempts: 5,
                message: "w".into(),
            }
            .is_terminal()
        );
        assert!(!DispatchError::Retryable("ok".into()).is_terminal());
    }

    #[test]
    fn retry_policy_default_matches_legacy_go_activity() {
        let policy = RetryPolicy::default();
        assert_eq!(policy.max_attempts, 5);
        assert_eq!(policy.initial_backoff, Duration::from_secs(30));
        assert_eq!(policy.max_backoff, Duration::from_secs(600));
        assert_eq!(policy.backoff_multiplier, 2.0);
    }

    #[test]
    fn retry_backoff_grows_then_caps() {
        let policy = RetryPolicy::default();
        assert_eq!(policy.next_backoff(1), Duration::ZERO);
        assert_eq!(policy.next_backoff(2), Duration::from_secs(30));
        assert_eq!(policy.next_backoff(3), Duration::from_secs(60));
        assert_eq!(policy.next_backoff(4), Duration::from_secs(120));
        assert_eq!(policy.next_backoff(5), Duration::from_secs(240));
        // attempt 6 would cap at max_backoff (600s).
        assert_eq!(policy.next_backoff(6), Duration::from_secs(480));
        assert_eq!(policy.next_backoff(7), Duration::from_secs(600));
        assert_eq!(policy.next_backoff(20), Duration::from_secs(600));
    }

    #[test]
    fn body_includes_only_present_optional_fields() {
        let req = OntologyActionRequest {
            action_id: "promote".into(),
            target_object_id: None,
            parameters: json!({"k": "v"}),
            justification: None,
            audit_correlation_id: Uuid::nil(),
        };
        let body = req.body();
        assert!(body.get("target_object_id").is_none());
        assert!(body.get("justification").is_none());
        assert_eq!(body["parameters"], json!({"k": "v"}));
    }

    #[test]
    fn normalize_helpers() {
        assert_eq!(
            normalize_base_url("ontology-actions:50106".into()),
            "http://ontology-actions:50106"
        );
        assert_eq!(
            normalize_base_url("https://ontology-actions:50106/".into()),
            "https://ontology-actions:50106/"
        );
        assert_eq!(normalize_bearer_token("xyz".into()), "Bearer xyz");
        assert_eq!(normalize_bearer_token("Bearer xyz".into()), "Bearer xyz");
        assert_eq!(normalize_bearer_token("BEARER abc".into()), "BEARER abc");
    }

    #[test]
    fn url_encoder_passes_through_safe_chars() {
        assert_eq!(
            urlencoding::encode("promote-customer.v2"),
            "promote-customer.v2"
        );
    }

    #[test]
    fn url_encoder_escapes_unsafe_chars() {
        assert_eq!(urlencoding::encode("a/b c"), "a%2Fb%20c");
    }

    #[tokio::test]
    async fn dispatch_with_retries_returns_terminal_error_immediately() {
        // Hit a non-routable address so the very first attempt fails
        // with a transport error → retryable. Wrap in a tight policy
        // that exhausts after one retry to keep the test fast.
        let dispatcher = EffectDispatcher::new(
            Client::builder()
                .timeout(Duration::from_millis(50))
                .build()
                .unwrap(),
            "http://127.0.0.1:1",
            "test-token",
        );
        let request = OntologyActionRequest {
            action_id: "promote".into(),
            target_object_id: None,
            parameters: json!({}),
            justification: None,
            audit_correlation_id: Uuid::nil(),
        };
        let policy = RetryPolicy {
            max_attempts: 2,
            initial_backoff: Duration::from_millis(1),
            max_backoff: Duration::from_millis(1),
            backoff_multiplier: 1.0,
        };
        let err = dispatcher
            .dispatch_with_retries(&request, policy)
            .await
            .expect_err("transport must fail");
        match err {
            DispatchError::Exhausted { attempts, .. } => assert_eq!(attempts, 2),
            other => panic!("expected Exhausted, got {other:?}"),
        }
    }

    #[tokio::test]
    async fn dispatch_once_classifies_unconfigured_base_url() {
        let dispatcher = EffectDispatcher::new(Client::new(), "", "tok");
        let req = OntologyActionRequest {
            action_id: "x".into(),
            target_object_id: None,
            parameters: json!({}),
            justification: None,
            audit_correlation_id: Uuid::nil(),
        };
        let err = dispatcher.dispatch_once(&req).await.expect_err("unconfig");
        assert!(matches!(err, DispatchError::Unconfigured(_)));
    }

    #[tokio::test]
    async fn dispatch_once_classifies_unconfigured_bearer_token() {
        let dispatcher = EffectDispatcher::new(Client::new(), "http://localhost:1", "");
        let req = OntologyActionRequest {
            action_id: "x".into(),
            target_object_id: None,
            parameters: json!({}),
            justification: None,
            audit_correlation_id: Uuid::nil(),
        };
        let err = dispatcher.dispatch_once(&req).await.expect_err("unconfig");
        assert!(matches!(err, DispatchError::Unconfigured(_)));
    }
}
