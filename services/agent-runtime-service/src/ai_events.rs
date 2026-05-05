//! AI-event publisher substrate (S5.3.a).
//!
//! Pins the topic, the producer transactional-id prefix and the
//! envelope shape used by `agent-runtime-service` when it emits
//! prompts, responses, evaluations and traces to Kafka. The matching
//! sink (`ai-sink`, S5.3.b) consumes this topic and materialises into
//! `of_ai.{prompts,responses,evaluations,traces}`.
//!
//! The envelope is producer-agnostic on purpose — agent-runtime
//! emits both prompt-workflow and conversation-state events after
//! ADR-0030's S8 consolidation absorbed those services here. The
//! downstream Iceberg writer has a single decoder.

use serde::{Deserialize, Serialize};
use uuid::Uuid;

/// Kafka topic — also pinned in the matching ACL CR
/// `infra/k8s/platform/manifests/strimzi/kafka-acls-domain-v1.yaml` (Write for this
/// service).
pub const TOPIC: &str = "ai.events.v1";

/// Transactional-id prefix declared in the ACL (`agent-runtime-`).
pub const TXN_ID_PREFIX: &str = "agent-runtime-";

/// Discrete event kinds that route to one of the four Iceberg target
/// tables. The `ai-sink` consumer dispatches on this enum.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum AiEventKind {
    /// User-facing prompt sent into the model. → `of_ai.prompts`.
    Prompt,
    /// Model response (or tool-call plan). → `of_ai.responses`.
    Response,
    /// Offline / online eval scores. → `of_ai.evaluations`.
    Evaluation,
    /// OpenTelemetry trace span attached to the agent run.
    /// → `of_ai.traces`.
    Trace,
}

impl AiEventKind {
    /// Iceberg target table for this kind. The sink uses this directly
    /// so the routing logic does not drift between producer and
    /// consumer.
    pub const fn target_table(self) -> &'static str {
        match self {
            AiEventKind::Prompt => "prompts",
            AiEventKind::Response => "responses",
            AiEventKind::Evaluation => "evaluations",
            AiEventKind::Trace => "traces",
        }
    }
}

/// Wire envelope — every record on `ai.events.v1` deserialises into
/// this. `payload` is opaque JSON sized to the table dictated by
/// `kind`. Schema evolution lives in the payload, not the envelope.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AiEventEnvelope {
    /// Deterministic v5 UUID — the dedup key for the sink.
    pub event_id: Uuid,
    /// Microseconds since unix epoch (partition source: `day(at)`).
    pub at: i64,
    /// Event kind → routes to one of the four target tables.
    pub kind: AiEventKind,
    /// Run id from the agent runtime (or workflow id from the prompt
    /// workflow service). Lets the sink JOIN events back to a run
    /// without reading `payload`.
    pub run_id: Option<Uuid>,
    /// OpenTelemetry trace id (hex 32) when the event sits inside a
    /// trace context.
    pub trace_id: Option<String>,
    /// Producer name — `"agent-runtime-service"` (post-ADR-0030 the
    /// only producer; legacy `"prompt-workflow-service"` retired).
    pub producer: String,
    /// Schema version of `payload` for the (kind, version) tuple.
    pub schema_version: u32,
    /// Opaque JSON; Iceberg writer stores as `string`.
    pub payload: serde_json::Value,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn topic_pinned() {
        assert_eq!(TOPIC, "ai.events.v1");
    }

    #[test]
    fn target_tables_match_namespace_layout() {
        assert_eq!(AiEventKind::Prompt.target_table(), "prompts");
        assert_eq!(AiEventKind::Response.target_table(), "responses");
        assert_eq!(AiEventKind::Evaluation.target_table(), "evaluations");
        assert_eq!(AiEventKind::Trace.target_table(), "traces");
    }

    #[test]
    fn envelope_round_trips() {
        let env = AiEventEnvelope {
            event_id: Uuid::nil(),
            at: 1_700_000_000_000_000,
            kind: AiEventKind::Prompt,
            run_id: None,
            trace_id: None,
            producer: "agent-runtime-service".into(),
            schema_version: 1,
            payload: serde_json::json!({"text": "hello"}),
        };
        let bytes = serde_json::to_vec(&env).unwrap();
        let back: AiEventEnvelope = serde_json::from_slice(&bytes).unwrap();
        assert_eq!(back.kind, AiEventKind::Prompt);
        assert_eq!(back.schema_version, 1);
    }
}
