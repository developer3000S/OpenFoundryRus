//! HTTP handlers — every inbound producer (`POST /api/v1/automations`)
//! lands here and publishes a `saga.step.requested.v1` event via the
//! transactional outbox plus an `saga.state`
//! row in the same Postgres transaction. The saga consumer (see
//! [`crate::automation_operations::domain::saga_consumer`]) picks the event up and drives
//! the dispatch.

use axum::{
    Json,
    extract::{Path, State},
    http::StatusCode,
    response::IntoResponse,
};
use chrono::Utc;
use outbox::OutboxEvent;
use saga::events::SagaStepRequestedV1;
use serde_json::{Value, json};
use uuid::Uuid;

use crate::{
    AppState,
    automation_operations::{
        event::{derive_request_event_id, derive_saga_id},
        models::{CreatePrimaryRequest, CreateSecondaryRequest},
        topics::SAGA_STEP_REQUESTED_V1,
    },
};

pub async fn list_items(State(state): State<AppState>) -> impl IntoResponse {
    let rows = sqlx::query_as::<_, (Uuid, String, String, Option<String>, chrono::DateTime<Utc>)>(
        "SELECT saga_id, name, status, current_step, updated_at \
         FROM saga.state \
         ORDER BY updated_at DESC LIMIT 100",
    )
    .fetch_all(&state.db)
    .await;

    match rows {
        Ok(rows) => {
            let data: Vec<Value> = rows
                .into_iter()
                .map(|(saga_id, name, status, current_step, updated_at)| {
                    json!({
                        "id": saga_id,
                        "name": name,
                        "status": status,
                        "current_step": current_step,
                        "updated_at": updated_at,
                    })
                })
                .collect();
            Json(json!({ "data": data })).into_response()
        }
        Err(error) => {
            tracing::error!(?error, "list_items: saga_state query failed");
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(json!({ "error": error.to_string() })),
            )
                .into_response()
        }
    }
}

pub async fn create_item(
    State(state): State<AppState>,
    Json(body): Json<CreatePrimaryRequest>,
) -> impl IntoResponse {
    let request = match parse_payload(body.payload) {
        Ok(request) => request,
        Err(error) => {
            return (StatusCode::BAD_REQUEST, Json(json!({ "error": error }))).into_response();
        }
    };

    if !crate::automation_operations::domain::dispatcher::is_known(&request.saga) {
        return (
            StatusCode::BAD_REQUEST,
            Json(json!({
                "error": format!(
                    "unknown saga task_type {:?}; known: {:?}",
                    request.saga,
                    crate::automation_operations::domain::dispatcher::KNOWN_SAGA_TYPES,
                )
            })),
        )
            .into_response();
    }

    match dispatch_request(&state, &request).await {
        Ok(()) => (
            StatusCode::ACCEPTED,
            Json(json!({
                "id": request.saga_id,
                "saga": request.saga,
                "tenant_id": request.tenant_id,
                "correlation_id": request.correlation_id,
                "status": "running",
                "created_at": Utc::now(),
                "topic": SAGA_STEP_REQUESTED_V1,
            })),
        )
            .into_response(),
        Err(error) => (
            StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": error })),
        )
            .into_response(),
    }
}

pub async fn get_item(State(state): State<AppState>, Path(id): Path<Uuid>) -> impl IntoResponse {
    let row = sqlx::query_as::<
        _,
        (
            Uuid,
            String,
            String,
            Option<String>,
            Vec<String>,
            Value,
            Option<String>,
            chrono::DateTime<Utc>,
            chrono::DateTime<Utc>,
        ),
    >(
        "SELECT saga_id, name, status, current_step, completed_steps, step_outputs, \
                failed_step, created_at, updated_at \
         FROM saga.state \
         WHERE saga_id = $1",
    )
    .bind(id)
    .fetch_optional(&state.db)
    .await;

    match row {
        Ok(Some((
            saga_id,
            name,
            status,
            current_step,
            completed_steps,
            step_outputs,
            failed_step,
            created_at,
            updated_at,
        ))) => Json(json!({
            "id": saga_id,
            "name": name,
            "status": status,
            "current_step": current_step,
            "completed_steps": completed_steps,
            "step_outputs": step_outputs,
            "failed_step": failed_step,
            "created_at": created_at,
            "updated_at": updated_at,
        }))
        .into_response(),
        Ok(None) => StatusCode::NOT_FOUND.into_response(),
        Err(error) => {
            tracing::error!(?error, "get_item: saga_state query failed");
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(json!({ "error": error.to_string() })),
            )
                .into_response()
        }
    }
}

pub async fn list_secondary(
    State(_state): State<AppState>,
    Path(parent_id): Path<Uuid>,
) -> impl IntoResponse {
    // The saga aggregate rolls history into the `step_outputs` JSON
    // map of the parent row (per `libs/saga` contract). The
    // legacy `automation_queue_runs` projection is gone; operators
    // wanting the per-step history read it from `get_item`'s
    // `step_outputs`. This endpoint stays for API back-compat and
    // returns the parent's audit info if present.
    Json(json!({
        "data": [],
        "parent_id": parent_id,
        "note": "step history is rolled into the parent saga's step_outputs JSON; query GET /automations/{id} for the full timeline",
    }))
    .into_response()
}

pub async fn create_secondary(
    State(_state): State<AppState>,
    Path(parent_id): Path<Uuid>,
    Json(_body): Json<CreateSecondaryRequest>,
) -> impl IntoResponse {
    // Manual recording of an arbitrary step is not supported under
    // the FASE 6 saga model — every step transition is a
    // `SagaRunner` write. Returning 410 GONE makes the legacy
    // contract explicit; keep the route registered so a client
    // hitting it gets a useful error instead of a 404.
    (
        StatusCode::GONE,
        Json(json!({
            "error": "manual recording of automation runs is not supported; use POST /api/v1/automations to start a saga",
            "parent_id": parent_id,
        })),
    )
        .into_response()
}

/// Parse the inbound `CreatePrimaryRequest::payload` into a typed
/// `SagaStepRequestedV1`. Pure, no IO. Unit-testable.
fn parse_payload(payload: Value) -> Result<SagaStepRequestedV1, String> {
    let task_type = payload
        .get("task_type")
        .and_then(Value::as_str)
        .filter(|value| !value.trim().is_empty())
        .ok_or_else(|| "payload.task_type is required".to_string())?
        .to_string();

    let tenant_id = payload
        .get("tenant_id")
        .and_then(Value::as_str)
        .unwrap_or("default")
        .to_string();

    let correlation_id = payload
        .get("audit_correlation_id")
        .or_else(|| payload.get("correlation_id"))
        .and_then(Value::as_str)
        .map(Uuid::parse_str)
        .transpose()
        .map_err(|error| format!("invalid correlation_id: {error}"))?
        .unwrap_or_else(Uuid::now_v7);

    let triggered_by = payload
        .get("triggered_by")
        .and_then(Value::as_str)
        .unwrap_or("system")
        .to_string();

    let saga_id = payload
        .get("task_id")
        .and_then(Value::as_str)
        .map(Uuid::parse_str)
        .transpose()
        .map_err(|error| format!("invalid task_id: {error}"))?
        .unwrap_or_else(|| derive_saga_id(&task_type, correlation_id));

    let input = payload
        .get("input")
        .cloned()
        .or_else(|| payload.get("payload").cloned())
        .unwrap_or(Value::Null);

    Ok(SagaStepRequestedV1 {
        saga_id,
        saga: task_type,
        tenant_id,
        correlation_id,
        triggered_by,
        input,
    })
}

/// INSERTs the `saga_state` row + the outbox event in a single
/// transaction. The Tarea 6.3 invariant: the row exists ⇔ Debezium
/// has (or will have) the matching `saga.step.requested.v1` event.
async fn dispatch_request(state: &AppState, request: &SagaStepRequestedV1) -> Result<(), String> {
    let mut tx = state.db.begin().await.map_err(|err| err.to_string())?;

    sqlx::query(
        "INSERT INTO saga.state (saga_id, name) \
         VALUES ($1, $2) \
         ON CONFLICT (saga_id) DO NOTHING",
    )
    .bind(request.saga_id)
    .bind(&request.saga)
    .execute(&mut *tx)
    .await
    .map_err(|err| format!("saga_state insert failed: {err}"))?;

    let event_id = derive_request_event_id(&request.saga, request.correlation_id);
    let payload = serde_json::to_value(request).map_err(|err| err.to_string())?;
    let event = OutboxEvent::new(
        event_id,
        "saga",
        request.saga_id.to_string(),
        SAGA_STEP_REQUESTED_V1,
        payload,
    )
    .with_header("x-audit-correlation-id", request.correlation_id.to_string())
    .with_header("ol-job", format!("saga/{}", request.saga))
    .with_header("ol-run-id", request.saga_id.to_string())
    .with_header("ol-producer", "automation-operations-service");
    outbox::enqueue(&mut tx, event)
        .await
        .map_err(|err| err.to_string())?;

    tx.commit().await.map_err(|err| err.to_string())?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn parse_payload_requires_task_type() {
        let err = parse_payload(json!({"tenant_id": "acme"})).expect_err("missing task_type");
        assert_eq!(err, "payload.task_type is required");
    }

    #[test]
    fn parse_payload_derives_saga_id_when_absent() {
        let req = parse_payload(json!({
            "task_type": "retention.sweep",
            "tenant_id": "acme",
        }))
        .expect("payload");
        assert_eq!(req.saga, "retention.sweep");
        assert_eq!(req.tenant_id, "acme");
        assert_eq!(req.triggered_by, "system");
        assert_eq!(req.input, Value::Null);
        // Same correlation_id should derive the same saga_id.
        let req2 = parse_payload(json!({
            "task_type": "retention.sweep",
            "tenant_id": "acme",
            "audit_correlation_id": req.correlation_id,
        }))
        .unwrap();
        assert_eq!(req.saga_id, req2.saga_id);
    }

    #[test]
    fn parse_payload_honors_explicit_task_id() {
        let task_id = Uuid::now_v7();
        let req = parse_payload(json!({
            "task_id": task_id,
            "task_type": "cleanup.workspace",
            "tenant_id": "acme",
            "input": {"workspace_id": Uuid::nil()},
        }))
        .unwrap();
        assert_eq!(req.saga_id, task_id);
        assert_eq!(req.saga, "cleanup.workspace");
        assert_eq!(req.input["workspace_id"], Uuid::nil().to_string());
    }

    #[test]
    fn parse_payload_falls_back_from_payload_to_input_alias() {
        let req = parse_payload(json!({
            "task_type": "retention.sweep",
            "payload": {"older_than_days": 30},
        }))
        .unwrap();
        assert_eq!(req.input["older_than_days"], 30);
    }

    #[test]
    fn parse_payload_rejects_invalid_correlation_id() {
        let err = parse_payload(json!({
            "task_type": "retention.sweep",
            "audit_correlation_id": "not-a-uuid",
        }))
        .expect_err("must reject");
        assert!(err.starts_with("invalid correlation_id"));
    }
}
