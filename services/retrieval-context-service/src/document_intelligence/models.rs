use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::FromRow;
use uuid::Uuid;

#[derive(Debug, Clone, Serialize, FromRow)]
pub struct DocIntelJob {
    pub id: Uuid,
    pub source_uri: String,
    pub mime_type: Option<String>,
    pub pipeline: String,
    pub status: String,
    pub options: serde_json::Value,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct SubmitJobRequest {
    pub source_uri: String,
    pub mime_type: Option<String>,
    pub pipeline: String,
    pub options: Option<serde_json::Value>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct UpdateStatusRequest {
    pub status: String,
    pub message: Option<String>,
}

#[derive(Debug, Clone, Serialize, FromRow)]
pub struct DocIntelExtraction {
    pub id: Uuid,
    pub job_id: Uuid,
    pub extraction_kind: String,
    pub payload: serde_json::Value,
    pub confidence: Option<f32>,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct PublishExtractionRequest {
    pub extraction_kind: String,
    pub payload: serde_json::Value,
    pub confidence: Option<f32>,
}
