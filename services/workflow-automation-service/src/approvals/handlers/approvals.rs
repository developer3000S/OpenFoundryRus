//! HTTP handlers — every inbound producer (`POST /api/v1/approvals`,
//! `POST /api/v1/approvals/{id}/decide`) lands here and persists the
//! state-machine row + outbox row in a single Postgres transaction.

use axum::{
    Json,
    extract::{Path, Query, State},
    http::StatusCode,
    response::IntoResponse,
};
use chrono::{DateTime, Duration, Utc};
use outbox::OutboxEvent;
use serde_json::{Value, json};
use sqlx::Row;
use state_machine::{Loaded, PgStore, StateMachine};
use uuid::Uuid;

use crate::{
    AppState,
    approvals::{
        domain::approval_request::{ApprovalRequest, ApprovalRequestEvent, ApprovalRequestState},
        event::{ApprovalCompletedV1, ApprovalRequestedV1, derive_outbox_event_id},
        models::approval::{
            ApprovalDecisionRequest, CreateApprovalRequest, CreateApprovalResponse,
            ListApprovalsQuery, WorkflowApproval,
        },
        topics::{APPROVAL_COMPLETED_V1, APPROVAL_REQUESTED_V1},
    },
};

const APPROVAL_REQUESTS_TABLE: &str = "audit_compliance.approval_requests";

pub async fn create_approval(
    State(state): State<AppState>,
    Json(body): Json<CreateApprovalRequest>,
) -> impl IntoResponse {
    let approval = approval_projection_from_request(&body);
    let aggregate = aggregate_from_projection(&approval, &body, state.approval_ttl_hours);

    match insert_state_machine_row_and_outbox(&state, &aggregate).await {
        Ok(()) => Json(CreateApprovalResponse {
            approval: with_state_metadata(approval, &aggregate),
            created: true,
        })
        .into_response(),
        Err(error) => {
            tracing::error!(?error, "approval state-machine insert failed");
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(json!({ "error": error })),
            )
                .into_response()
        }
    }
}

pub async fn list_approvals(
    State(state): State<AppState>,
    Query(params): Query<ListApprovalsQuery>,
) -> impl IntoResponse {
    let page = params.page.unwrap_or(1).max(1);
    let per_page = params.per_page.unwrap_or(20).clamp(1, 100);
    let offset = (page - 1) * per_page;

    let (query, status_filter): (String, Option<String>) = match &params.status {
        Some(status) => (
            "SELECT id, tenant_id, subject, assigned_to, decided_by, state, expires_at, \
                    correlation_id, created_at, updated_at \
             FROM audit_compliance.approval_requests \
             WHERE state = $1 \
             ORDER BY created_at DESC LIMIT $2 OFFSET $3"
                .to_string(),
            Some(status.clone()),
        ),
        None => (
            "SELECT id, tenant_id, subject, assigned_to, decided_by, state, expires_at, \
                    correlation_id, created_at, updated_at \
             FROM audit_compliance.approval_requests \
             ORDER BY created_at DESC LIMIT $1 OFFSET $2"
                .to_string(),
            None,
        ),
    };

    let mut q = sqlx::query(&query);
    if let Some(status) = status_filter.as_deref() {
        q = q.bind(status);
    }
    q = q.bind(per_page).bind(offset);

    let rows = q.fetch_all(&state.db).await;
    match rows {
        Ok(rows) => {
            let data: Vec<Value> = rows
                .into_iter()
                .map(|row| {
                    json!({
                        "id": row.get::<Uuid, _>("id"),
                        "tenant_id": row.get::<String, _>("tenant_id"),
                        "subject": row.get::<String, _>("subject"),
                        "assigned_to": row.get::<Option<Uuid>, _>("assigned_to"),
                        "decided_by": row.get::<Option<Uuid>, _>("decided_by"),
                        "state": row.get::<String, _>("state"),
                        "expires_at": row.get::<Option<DateTime<Utc>>, _>("expires_at"),
                        "correlation_id": row.get::<Uuid, _>("correlation_id"),
                        "created_at": row.get::<DateTime<Utc>, _>("created_at"),
                        "updated_at": row.get::<DateTime<Utc>, _>("updated_at"),
                    })
                })
                .collect();
            Json(json!({
                "data": data,
                "page": page,
                "per_page": per_page,
                "filters": {
                    "status": params.status,
                    "assigned_to": params.assigned_to,
                    "workflow_id": params.workflow_id,
                },
            }))
            .into_response()
        }
        Err(error) => {
            tracing::error!(?error, "list_approvals query failed");
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(json!({ "error": error.to_string() })),
            )
                .into_response()
        }
    }
}

pub async fn decide_approval(
    State(state): State<AppState>,
    Path(approval_id): Path<Uuid>,
    auth_middleware::layer::AuthUser(claims): auth_middleware::layer::AuthUser,
    Json(body): Json<ApprovalDecisionRequest>,
) -> impl IntoResponse {
    let event = match decision_event_from_body(&body, claims.sub) {
        Ok(event) => event,
        Err(error) => {
            return (StatusCode::BAD_REQUEST, Json(json!({ "error": error }))).into_response();
        }
    };

    match apply_decision_and_publish(&state, approval_id, event).await {
        Ok(()) => StatusCode::ACCEPTED.into_response(),
        Err(error) => {
            tracing::error!(%approval_id, ?error, "approval decide failed");
            // 409 Conflict if the row was not in pending — covers
            // the "already decided" + "already expired" cases.
            let status = if error.contains("transition") || error.contains("not found") {
                StatusCode::CONFLICT
            } else {
                StatusCode::INTERNAL_SERVER_ERROR
            };
            (status, Json(json!({ "error": error }))).into_response()
        }
    }
}

// ────────────────────────── Internal helpers ────────────────────────────

async fn insert_state_machine_row_and_outbox(
    state: &AppState,
    aggregate: &ApprovalRequest,
) -> Result<(), String> {
    let mut tx = state.db.begin().await.map_err(|err| err.to_string())?;

    let payload = serde_json::to_value(aggregate).map_err(|err| err.to_string())?;
    let state_str = ApprovalRequest::state_str(aggregate.current_state());
    let expires_at = StateMachine::expires_at(aggregate);
    let assigned_to = first_approver_uuid(&aggregate.approver_set);

    sqlx::query(&format!(
        "INSERT INTO {table} \
         (id, tenant_id, subject, assigned_to, decided_by, state, state_data, version, \
          expires_at, correlation_id, created_at, updated_at) \
         VALUES ($1, $2, $3, $4, NULL, $5, $6, 1, $7, $8, now(), now()) \
         ON CONFLICT (id) DO NOTHING",
        table = APPROVAL_REQUESTS_TABLE
    ))
    .bind(aggregate.id)
    .bind(&aggregate.tenant_id)
    .bind(&aggregate.subject)
    .bind(assigned_to)
    .bind(&state_str)
    .bind(&payload)
    .bind(expires_at)
    .bind(aggregate.correlation_id)
    .execute(&mut *tx)
    .await
    .map_err(|err| format!("approval_requests insert failed: {err}"))?;

    let event = ApprovalRequestedV1 {
        approval_id: aggregate.id,
        tenant_id: aggregate.tenant_id.clone(),
        subject: aggregate.subject.clone(),
        approver_set: aggregate.approver_set.clone(),
        action_payload: aggregate.action_payload.clone(),
        correlation_id: aggregate.correlation_id,
        triggered_by: "api".to_string(),
        expires_at: aggregate.expires_at,
    };
    enqueue_outbox(
        &mut tx,
        aggregate.id,
        "requested",
        APPROVAL_REQUESTED_V1,
        &event,
    )
    .await
    .map_err(|err| err.to_string())?;

    tx.commit().await.map_err(|err| err.to_string())?;
    Ok(())
}

pub(crate) async fn apply_decision_and_publish(
    state: &AppState,
    approval_id: Uuid,
    event: ApprovalRequestEvent,
) -> Result<(), String> {
    let pg_store = PgStore::<ApprovalRequest>::new(state.db.clone(), APPROVAL_REQUESTS_TABLE);
    let loaded = pg_store.load(approval_id).await.map_err(|err| match err {
        state_machine::StoreError::NotFound(_) => format!("approval {approval_id} not found"),
        other => other.to_string(),
    })?;
    let Loaded { machine, version } = loaded;
    let next_loaded = pg_store
        .apply(Loaded { machine, version }, event)
        .await
        .map_err(|err| err.to_string())?;

    let aggregate = next_loaded.machine;
    let mut tx = state.db.begin().await.map_err(|err| err.to_string())?;
    let event = ApprovalCompletedV1 {
        approval_id: aggregate.id,
        tenant_id: aggregate.tenant_id.clone(),
        correlation_id: aggregate.correlation_id,
        decision: match aggregate.state {
            ApprovalRequestState::Approved => "approved".to_string(),
            ApprovalRequestState::Rejected => "rejected".to_string(),
            other => {
                return Err(format!(
                    "approval landed in unexpected state {other:?} after decide"
                ));
            }
        },
        decided_by: aggregate
            .decided_by
            .clone()
            .unwrap_or_else(|| "system".to_string()),
        comment: aggregate.comment.clone(),
        decided_at: aggregate.decided_at.unwrap_or_else(Utc::now),
    };
    enqueue_outbox(
        &mut tx,
        aggregate.id,
        "completed",
        APPROVAL_COMPLETED_V1,
        &event,
    )
    .await
    .map_err(|err| err.to_string())?;

    // Mirror the decided_by column projection so list/get see the
    // decider without round-tripping through state_data.
    if let Some(decided_by_str) = aggregate.decided_by.as_deref() {
        if let Ok(decided_by_uuid) = Uuid::parse_str(decided_by_str) {
            sqlx::query(&format!(
                "UPDATE {table} SET decided_by = $1 WHERE id = $2",
                table = APPROVAL_REQUESTS_TABLE
            ))
            .bind(decided_by_uuid)
            .bind(aggregate.id)
            .execute(&mut *tx)
            .await
            .map_err(|err| err.to_string())?;
        }
    }

    tx.commit().await.map_err(|err| err.to_string())?;

    // FASE 7 / Tarea 7.3 interim — also POST to
    // audit-compliance-service /api/v1/audit/events synchronously
    // so the audit ledger keeps the same content as before. A
    // follow-up FASE 9 task collapses this into a Kafka consumer
    // of approval.completed.v1 inside audit-compliance-service.
    if let Err(err) = post_audit_event(state, &aggregate).await {
        tracing::warn!(
            approval_id = %aggregate.id,
            error = %err,
            "audit event POST failed; outbox event will eventually replay it"
        );
    }

    Ok(())
}

async fn enqueue_outbox<E: serde::Serialize>(
    tx: &mut sqlx::Transaction<'_, sqlx::Postgres>,
    approval_id: Uuid,
    kind: &str,
    topic: &str,
    payload: &E,
) -> Result<(), outbox::OutboxError> {
    let event_id = derive_outbox_event_id(approval_id, kind);
    let payload = serde_json::to_value(payload)?;
    let event = OutboxEvent::new(
        event_id,
        "approval_request",
        approval_id.to_string(),
        topic,
        payload,
    )
    .with_header("x-audit-correlation-id", approval_id.to_string())
    .with_header("ol-job", "approvals/decide".to_string())
    .with_header("ol-run-id", approval_id.to_string())
    .with_header("ol-producer", "approvals-service");
    outbox::enqueue(tx, event).await
}

async fn post_audit_event(state: &AppState, aggregate: &ApprovalRequest) -> Result<(), String> {
    let action = match aggregate.state {
        ApprovalRequestState::Approved => "approval.approved",
        ApprovalRequestState::Rejected => "approval.rejected",
        ApprovalRequestState::Expired => "approval.expired",
        other => return Err(format!("unsupported terminal state {other:?}")),
    };
    let actor = aggregate
        .decided_by
        .clone()
        .unwrap_or_else(|| "system".to_string());
    let payload = json!({
        "occurred_at": aggregate.decided_at.unwrap_or_else(Utc::now),
        "tenant_id": aggregate.tenant_id,
        "actor": actor,
        "action": action,
        "resource_type": "approval_request",
        "resource_id": aggregate.id,
        "audit_correlation_id": aggregate.correlation_id,
        "attributes": {
            "subject": aggregate.subject,
            "approver_set": aggregate.approver_set,
            "comment": aggregate.comment,
        }
    });

    let mut request = state
        .http_client
        .post(format!(
            "{}/api/v1/audit/events",
            state.audit_compliance_service_url.trim_end_matches('/')
        ))
        .header(
            "x-audit-correlation-id",
            aggregate.correlation_id.to_string(),
        )
        .json(&payload);
    if let Some(token) = state.audit_compliance_bearer_token.as_deref() {
        if !token.is_empty() {
            let header = if token.to_ascii_lowercase().starts_with("bearer ") {
                token.to_string()
            } else {
                format!("Bearer {token}")
            };
            request = request.header("authorization", header);
        }
    }

    let response = request
        .send()
        .await
        .map_err(|err| format!("audit POST failed: {err}"))?;
    if response.status().is_success() {
        Ok(())
    } else {
        let status = response.status();
        let body = response.text().await.unwrap_or_default();
        Err(format!("audit POST returned {status}: {body}"))
    }
}

fn approval_projection_from_request(body: &CreateApprovalRequest) -> WorkflowApproval {
    let id = Uuid::now_v7();
    let mut payload = body.payload.clone();
    ensure_object(&mut payload);
    let object = payload
        .as_object_mut()
        .expect("approval payload normalised to object");
    object.insert(
        "workflow_id".to_string(),
        serde_json::json!(body.workflow_id),
    );
    object.insert(
        "workflow_run_id".to_string(),
        serde_json::json!(body.workflow_run_id),
    );
    object.insert("step_id".to_string(), serde_json::json!(body.step_id));
    object.insert(
        "instructions".to_string(),
        serde_json::json!(body.instructions),
    );

    WorkflowApproval {
        id,
        workflow_id: body.workflow_id,
        workflow_run_id: body.workflow_run_id,
        step_id: body.step_id.clone(),
        title: body.title.clone(),
        instructions: body.instructions.clone(),
        assigned_to: body.assigned_to,
        status: "pending".to_string(),
        decision: None,
        payload,
        requested_at: Utc::now(),
        decided_at: None,
        decided_by: None,
    }
}

fn aggregate_from_projection(
    approval: &WorkflowApproval,
    body: &CreateApprovalRequest,
    ttl_hours: u32,
) -> ApprovalRequest {
    let tenant_id = approval
        .payload
        .get("tenant_id")
        .and_then(Value::as_str)
        .map(str::to_string)
        .unwrap_or_else(|| body.workflow_id.to_string());
    let approver_set = body
        .assigned_to
        .map(|id| vec![id.to_string()])
        .unwrap_or_default();
    let expires_at = body
        .payload
        .get("expires_at")
        .and_then(Value::as_str)
        .and_then(|raw| DateTime::parse_from_rfc3339(raw).ok())
        .map(|dt| dt.with_timezone(&Utc))
        .or_else(|| Some(Utc::now() + Duration::hours(ttl_hours.into())));
    ApprovalRequest::new(
        approval.id,
        tenant_id,
        approval.title.clone(),
        approver_set,
        approval.payload.clone(),
        approval.id,
        expires_at,
    )
}

fn with_state_metadata(
    mut approval: WorkflowApproval,
    aggregate: &ApprovalRequest,
) -> WorkflowApproval {
    ensure_object(&mut approval.payload);
    approval
        .payload
        .as_object_mut()
        .expect("approval payload normalised to object")
        .insert(
            "approval_state".to_string(),
            json!({
                "id": aggregate.id,
                "state": aggregate.state.as_str(),
                "expires_at": aggregate.expires_at,
                "correlation_id": aggregate.correlation_id,
                "topic_requested": APPROVAL_REQUESTED_V1,
                "topic_completed": APPROVAL_COMPLETED_V1,
                "authoritative": "audit_compliance.approval_requests",
            }),
        );
    approval
}

fn first_approver_uuid(approver_set: &[String]) -> Option<Uuid> {
    approver_set
        .first()
        .and_then(|raw| Uuid::parse_str(raw).ok())
}

fn decision_event_from_body(
    body: &ApprovalDecisionRequest,
    actor: Uuid,
) -> Result<ApprovalRequestEvent, String> {
    let comment = body.comment.clone();
    match body.decision.to_ascii_lowercase().as_str() {
        "approve" | "approved" => Ok(ApprovalRequestEvent::Approve {
            decided_by: actor.to_string(),
            comment,
        }),
        "reject" | "rejected" => Ok(ApprovalRequestEvent::Reject {
            decided_by: actor.to_string(),
            comment,
        }),
        other => Err(format!("unsupported approval decision '{other}'")),
    }
}

fn ensure_object(value: &mut Value) {
    if !value.is_object() {
        *value = Value::Object(serde_json::Map::new());
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn sample_create_request(assigned_to: Option<Uuid>) -> CreateApprovalRequest {
        CreateApprovalRequest {
            workflow_id: Uuid::now_v7(),
            workflow_run_id: Uuid::now_v7(),
            step_id: "review".into(),
            title: "Review change".into(),
            instructions: "Check the output".into(),
            assigned_to,
            payload: json!({"tenant_id": "acme"}),
        }
    }

    #[test]
    fn aggregate_uses_payload_tenant_id_when_present() {
        let request = sample_create_request(Some(Uuid::now_v7()));
        let projection = approval_projection_from_request(&request);
        let aggregate = aggregate_from_projection(&projection, &request, 24);
        assert_eq!(aggregate.tenant_id, "acme");
        assert_eq!(aggregate.subject, "Review change");
        assert_eq!(aggregate.approver_set.len(), 1);
        assert_eq!(aggregate.state, ApprovalRequestState::Pending);
        assert!(aggregate.expires_at.is_some());
    }

    #[test]
    fn aggregate_falls_back_to_workflow_id_for_tenant() {
        let request = CreateApprovalRequest {
            workflow_id: Uuid::now_v7(),
            workflow_run_id: Uuid::now_v7(),
            step_id: "x".into(),
            title: "x".into(),
            instructions: String::new(),
            assigned_to: None,
            payload: Value::Null,
        };
        let projection = approval_projection_from_request(&request);
        let aggregate = aggregate_from_projection(&projection, &request, 24);
        assert_eq!(aggregate.tenant_id, request.workflow_id.to_string());
        assert!(aggregate.approver_set.is_empty());
    }

    #[test]
    fn aggregate_honors_explicit_expires_at() {
        let when = Utc::now() + Duration::hours(48);
        let mut request = sample_create_request(None);
        request.payload = json!({
            "tenant_id": "acme",
            "expires_at": when.to_rfc3339(),
        });
        let projection = approval_projection_from_request(&request);
        let aggregate = aggregate_from_projection(&projection, &request, 1);
        // Tolerate sub-second drift from the rfc3339 round-trip.
        let drift = aggregate.expires_at.unwrap().signed_duration_since(when);
        assert!(drift.num_milliseconds().abs() <= 1);
    }

    #[test]
    fn decision_event_maps_approve_and_reject_strings() {
        let actor = Uuid::now_v7();
        let approve = decision_event_from_body(
            &ApprovalDecisionRequest {
                decision: "approved".into(),
                comment: Some("ok".into()),
                payload: Value::Null,
            },
            actor,
        )
        .unwrap();
        assert!(matches!(approve, ApprovalRequestEvent::Approve { .. }));
        let reject = decision_event_from_body(
            &ApprovalDecisionRequest {
                decision: "reject".into(),
                comment: None,
                payload: Value::Null,
            },
            actor,
        )
        .unwrap();
        assert!(matches!(reject, ApprovalRequestEvent::Reject { .. }));
    }

    #[test]
    fn decision_event_rejects_unknown_decision_string() {
        let err = decision_event_from_body(
            &ApprovalDecisionRequest {
                decision: "abstain".into(),
                comment: None,
                payload: Value::Null,
            },
            Uuid::now_v7(),
        )
        .expect_err("unknown decision");
        assert!(err.contains("unsupported"));
    }
}
