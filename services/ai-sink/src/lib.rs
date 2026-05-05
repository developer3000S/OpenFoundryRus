//! `ai-sink` — `ai.events.v1` Kafka topic → Iceberg `of_ai.*`
//! (S5.3.b).
//!
//! ## Substrate scope
//!
//! Pure logic only:
//!
//! 1. Consumer group + source topic constants.
//! 2. The wire envelope (mirrors
//!    [`agent_runtime_service::ai_events::AiEventEnvelope`]; the
//!    legacy `prompt-workflow-service` mirror has been retired per
//!    ADR-0030 — both producers now live inside `agent-runtime-service`).
//! 3. Per-table Iceberg target identifiers (catalog / namespace /
//!    table / partition transform) for the four AI tables.
//! 4. Routing function `route(envelope) -> &str` returning the target
//!    table name; idempotent and unit-testable.
//! 5. Batch policy aligned with `audit-sink::BatchPolicy::PLAN_DEFAULT`
//!    (100k records or 60 s, whichever first).
//!
//! The runtime consumes Kafka, batches by target table and appends
//! Arrow `RecordBatch`es to Iceberg.

use serde::{Deserialize, Serialize};
use std::time::Duration;
use thiserror::Error;
use uuid::Uuid;

#[cfg(feature = "runtime")]
pub mod runtime;

pub const SOURCE_TOPIC: &str = "ai.events.v1";

pub const CONSUMER_GROUP: &str = "ai-sink";

/// Iceberg target — catalog + namespace shared across the four
/// tables; per-event routing picks the table.
pub mod iceberg_target {
    pub const CATALOG: &str = "lakekeeper";
    pub const NAMESPACE: &str = "of_ai";

    pub const TABLE_PROMPTS: &str = "prompts";
    pub const TABLE_RESPONSES: &str = "responses";
    pub const TABLE_EVALUATIONS: &str = "evaluations";
    pub const TABLE_TRACES: &str = "traces";

    /// All four tables partition on `day(at)` so file pruning works
    /// uniformly across the namespace.
    pub const PARTITION_TRANSFORM: &str = "day(at)";

    /// Sort by `at` ascending — events are near-monotonic per
    /// producer.
    pub const SORT_ORDER: &str = "at ASC";
}

/// Event kind — wire-equivalent to
/// `agent_runtime_service::ai_events::AiEventKind`. Replicated here so
/// the sink does not depend on either producer crate.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum AiEventKind {
    Prompt,
    Response,
    Evaluation,
    Trace,
}

impl AiEventKind {
    /// Iceberg target table for this kind.
    pub const fn target_table(self) -> &'static str {
        match self {
            AiEventKind::Prompt => iceberg_target::TABLE_PROMPTS,
            AiEventKind::Response => iceberg_target::TABLE_RESPONSES,
            AiEventKind::Evaluation => iceberg_target::TABLE_EVALUATIONS,
            AiEventKind::Trace => iceberg_target::TABLE_TRACES,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AiEventEnvelope {
    pub event_id: Uuid,
    pub at: i64,
    pub kind: AiEventKind,
    pub run_id: Option<Uuid>,
    pub trace_id: Option<String>,
    pub producer: String,
    pub schema_version: u32,
    pub payload: serde_json::Value,
}

#[derive(Debug, Error)]
pub enum DecodeError {
    #[error("invalid JSON: {0}")]
    Json(#[from] serde_json::Error),
}

/// Decode a Kafka payload into an [`AiEventEnvelope`]. Pure function
/// so the consumer loop stays testable without rdkafka.
pub fn decode(bytes: &[u8]) -> Result<AiEventEnvelope, DecodeError> {
    Ok(serde_json::from_slice(bytes)?)
}

/// Per-message routing — returns the Iceberg table name for a decoded
/// envelope.
pub const fn route(env: &AiEventEnvelope) -> &'static str {
    env.kind.target_table()
}

/// Batch policy — same shape as `audit_sink::BatchPolicy`; the sink
/// flushes whichever side trips first.
#[derive(Debug, Clone, Copy)]
pub struct BatchPolicy {
    pub max_records: usize,
    pub max_wait: Duration,
}

impl BatchPolicy {
    pub const PLAN_DEFAULT: BatchPolicy = BatchPolicy {
        max_records: 100_000,
        max_wait: Duration::from_secs(60),
    };

    pub fn should_flush(&self, current_records: usize, elapsed: Duration) -> bool {
        current_records >= self.max_records || elapsed >= self.max_wait
    }
}

/// Iceberg schema substrate — Arrow-equivalent column names + WORM
/// retention pins per table. The four AI tables share the same
/// minimum-viable column set so a single Arrow schema serves all
/// consumers (variant payload kept as `string`).
pub mod iceberg_schema {
    /// Common fields across all four tables. Per-kind extensions live
    /// inside `payload` (string column) until a specific kind earns
    /// its own typed columns.
    pub mod fields {
        pub const EVENT_ID: &str = "event_id";
        pub const AT: &str = "at"; // µs since epoch
        pub const KIND: &str = "kind";
        pub const RUN_ID: &str = "run_id";
        pub const TRACE_ID: &str = "trace_id";
        pub const PRODUCER: &str = "producer";
        pub const SCHEMA_VERSION: &str = "schema_version";
        pub const PAYLOAD: &str = "payload";
    }

    /// Stable Iceberg/Parquet field ids. They must never be reused or
    /// renumbered once data exists.
    pub mod field_ids {
        pub const EVENT_ID: i32 = 1;
        pub const AT: i32 = 2;
        pub const KIND: i32 = 3;
        pub const RUN_ID: i32 = 4;
        pub const TRACE_ID: i32 = 5;
        pub const PRODUCER: i32 = 6;
        pub const SCHEMA_VERSION: i32 = 7;
        pub const PAYLOAD: i32 = 8;
    }

    pub const PARTITION_TRANSFORM: &str = "day";
    pub const PARTITION_SOURCE_FIELD: &str = fields::AT;
    pub const SORT_FIELD: &str = fields::AT;
    pub const SORT_DIRECTION: &str = "asc";

    /// AI tables get a 1-year snapshot retention (model-evaluation
    /// queries rarely need older snapshots; raw rows themselves are
    /// kept forever via the partition data files). Contrast with
    /// `audit_sink::iceberg_schema::retention` which forbids expiry.
    pub const TABLE_PROPERTIES: &[(&str, &str)] = &[
        ("write.format.default", "parquet"),
        ("write.parquet.compression-codec", "zstd"),
        ("history.expire.max-snapshot-age-ms", "31536000000"), // 1y
        ("history.expire.min-snapshots-to-keep", "30"),
    ];
}

#[cfg(test)]
mod tests {
    use super::*;

    fn sample(kind: AiEventKind) -> AiEventEnvelope {
        AiEventEnvelope {
            event_id: Uuid::nil(),
            at: 1_700_000_000_000_000,
            kind,
            run_id: None,
            trace_id: None,
            producer: "agent-runtime-service".into(),
            schema_version: 1,
            payload: serde_json::json!({}),
        }
    }

    #[test]
    fn topic_and_group_pinned() {
        assert_eq!(SOURCE_TOPIC, "ai.events.v1");
        assert_eq!(CONSUMER_GROUP, "ai-sink");
    }

    #[test]
    fn route_dispatches_to_correct_table() {
        assert_eq!(route(&sample(AiEventKind::Prompt)), "prompts");
        assert_eq!(route(&sample(AiEventKind::Response)), "responses");
        assert_eq!(route(&sample(AiEventKind::Evaluation)), "evaluations");
        assert_eq!(route(&sample(AiEventKind::Trace)), "traces");
    }

    #[test]
    fn decode_round_trip() {
        let bytes = serde_json::to_vec(&sample(AiEventKind::Prompt)).unwrap();
        let back = decode(&bytes).unwrap();
        assert_eq!(back.kind, AiEventKind::Prompt);
    }

    #[test]
    fn decode_rejects_invalid_json() {
        assert!(decode(b"not json").is_err());
    }

    #[test]
    fn batch_policy_matches_audit_sink_defaults() {
        let p = BatchPolicy::PLAN_DEFAULT;
        assert_eq!(p.max_records, 100_000);
        assert_eq!(p.max_wait, Duration::from_secs(60));
        assert!(p.should_flush(100_000, Duration::from_secs(0)));
        assert!(p.should_flush(0, Duration::from_secs(60)));
        assert!(!p.should_flush(0, Duration::from_secs(0)));
    }

    #[test]
    fn schema_partition_consistent_with_target() {
        assert_eq!(
            format!(
                "{}({})",
                iceberg_schema::PARTITION_TRANSFORM,
                iceberg_schema::PARTITION_SOURCE_FIELD
            ),
            iceberg_target::PARTITION_TRANSFORM
        );
    }

    #[test]
    fn schema_field_ids_are_stable() {
        assert_eq!(iceberg_schema::field_ids::EVENT_ID, 1);
        assert_eq!(iceberg_schema::field_ids::PAYLOAD, 8);
    }
}
