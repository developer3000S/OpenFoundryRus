use axum::{
    Json,
    extract::{Path, State},
    http::StatusCode,
    response::IntoResponse,
};
use serde_json::json;
use uuid::Uuid;

use crate::AppState;
use crate::document_intelligence::models::{
    DocIntelExtraction, DocIntelJob, PublishExtractionRequest, SubmitJobRequest,
    UpdateStatusRequest,
};

pub async fn list_jobs(State(state): State<AppState>) -> impl IntoResponse {
    match sqlx::query_as::<_, DocIntelJob>(
        "SELECT * FROM document_intelligence_jobs ORDER BY created_at DESC LIMIT 200",
    )
    .fetch_all(&state.db)
    .await
    {
        Ok(rows) => Json(rows).into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response(),
    }
}

pub async fn submit_job(
    State(state): State<AppState>,
    Json(body): Json<SubmitJobRequest>,
) -> impl IntoResponse {
    let id = Uuid::now_v7();
    let options = body.options.unwrap_or_else(|| json!({}));
    match sqlx::query_as::<_, DocIntelJob>(
        "INSERT INTO document_intelligence_jobs (id, source_uri, mime_type, pipeline, status, options) \
         VALUES ($1, $2, $3, $4, 'queued', $5) RETURNING *",
    )
    .bind(id)
    .bind(&body.source_uri)
    .bind(&body.mime_type)
    .bind(&body.pipeline)
    .bind(&options)
    .fetch_one(&state.db)
    .await
    {
        Ok(row) => (StatusCode::CREATED, Json(row)).into_response(),
        Err(e) => (StatusCode::BAD_REQUEST, e.to_string()).into_response(),
    }
}

pub async fn get_job(
    State(state): State<AppState>,
    Path(id): Path<Uuid>,
) -> impl IntoResponse {
    match sqlx::query_as::<_, DocIntelJob>(
        "SELECT * FROM document_intelligence_jobs WHERE id = $1",
    )
    .bind(id)
    .fetch_optional(&state.db)
    .await
    {
        Ok(Some(row)) => Json(row).into_response(),
        Ok(None) => (StatusCode::NOT_FOUND, "job not found").into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response(),
    }
}

pub async fn update_status(
    State(state): State<AppState>,
    Path(id): Path<Uuid>,
    Json(body): Json<UpdateStatusRequest>,
) -> impl IntoResponse {
    match sqlx::query_as::<_, DocIntelJob>(
        "UPDATE document_intelligence_jobs SET status = $2, updated_at = now() WHERE id = $1 RETURNING *",
    )
    .bind(id)
    .bind(&body.status)
    .fetch_optional(&state.db)
    .await
    {
        Ok(Some(row)) => {
            let _ = sqlx::query(
                "INSERT INTO document_intelligence_status_events (id, job_id, status, message) VALUES ($1, $2, $3, $4)",
            )
            .bind(Uuid::now_v7())
            .bind(id)
            .bind(&body.status)
            .bind(&body.message)
            .execute(&state.db)
            .await;
            Json(row).into_response()
        }
        Ok(None) => (StatusCode::NOT_FOUND, "job not found").into_response(),
        Err(e) => (StatusCode::BAD_REQUEST, e.to_string()).into_response(),
    }
}

pub async fn list_extractions(
    State(state): State<AppState>,
    Path(job_id): Path<Uuid>,
) -> impl IntoResponse {
    match sqlx::query_as::<_, DocIntelExtraction>(
        "SELECT * FROM document_intelligence_extractions WHERE job_id = $1 ORDER BY created_at DESC",
    )
    .bind(job_id)
    .fetch_all(&state.db)
    .await
    {
        Ok(rows) => Json(rows).into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR, e.to_string()).into_response(),
    }
}

pub async fn publish_extraction(
    State(state): State<AppState>,
    Path(job_id): Path<Uuid>,
    Json(body): Json<PublishExtractionRequest>,
) -> impl IntoResponse {
    let id = Uuid::now_v7();
    match sqlx::query_as::<_, DocIntelExtraction>(
        "INSERT INTO document_intelligence_extractions (id, job_id, extraction_kind, payload, confidence) \
         VALUES ($1, $2, $3, $4, $5) RETURNING *",
    )
    .bind(id)
    .bind(job_id)
    .bind(&body.extraction_kind)
    .bind(&body.payload)
    .bind(body.confidence)
    .fetch_one(&state.db)
    .await
    {
        Ok(row) => (StatusCode::CREATED, Json(row)).into_response(),
        Err(e) => (StatusCode::BAD_REQUEST, e.to_string()).into_response(),
    }
}
