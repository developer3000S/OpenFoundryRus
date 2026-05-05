//! HTTP execution handlers — every inbound producer path
//! (`manual`, `webhook`, `lineage_build`, internal-triggered) lands
//! here and publishes an `automate.condition.v1` event via the
//! transactional outbox plus an `automation_runs` row in the same
//! Postgres transaction. The condition consumer (see
//! [`crate::domain::condition_consumer`]) picks the event up and
//! drives the dispatch.
//!
//! FASE 5 / Tarea 5.3 deliverable; the legacy Temporal adapter
//! was retired in FASE 8 / Tarea 8.2. The HTTP routes themselves
//! keep their old shapes so callers (UI, webhook senders,
//! `pipeline-service::trigger_lineage_builds`,
//! `pipeline-schedule-service::workflow_run_requested`) do not need
//! to change.

use axum::{
    Json,
    extract::{Path, State},
    http::{HeaderMap, StatusCode},
    response::IntoResponse,
};
use chrono::Utc;
use serde_json::{Value, json};
use uuid::Uuid;

use crate::{
    AppState,
    domain::automation_run::AutomationRun,
    domain::condition_consumer::AUTOMATION_RUNS_TABLE,
    event::{AutomateConditionV1, derive_run_id, tenant_uuid_from_str},
    handlers::crud::load_workflow,
    models::{
        execution::{
            InternalLineageRunRequest, InternalTriggeredRunRequest, StartRunRequest,
            TriggerEventRequest,
        },
        workflow::WorkflowDefinition,
    },
    topics::AUTOMATE_CONDITION_V1,
};
use outbox::OutboxEvent;
use state_machine::{PgStore, StateMachine};

pub async fn start_manual_run(
    State(state): State<AppState>,
    Path(workflow_id): Path<Uuid>,
    auth_middleware::layer::AuthUser(claims): auth_middleware::layer::AuthUser,
    Json(body): Json<StartRunRequest>,
) -> impl IntoResponse {
    let Some(workflow) = load_or_404(&state, workflow_id).await else {
        return StatusCode::NOT_FOUND.into_response();
    };

    match dispatch_run(&state, &workflow, "manual", Some(claims.sub), body.context).await {
        Ok(run) => (StatusCode::CREATED, Json(run)).into_response(),
        Err(error) => (StatusCode::BAD_REQUEST, Json(json!({ "error": error }))).into_response(),
    }
}

pub async fn trigger_webhook(
    State(state): State<AppState>,
    Path(workflow_id): Path<Uuid>,
    headers: HeaderMap,
    Json(body): Json<TriggerEventRequest>,
) -> impl IntoResponse {
    let Some(workflow) = load_or_404(&state, workflow_id).await else {
        return StatusCode::NOT_FOUND.into_response();
    };

    if workflow.trigger_type != "webhook" {
        return (
            StatusCode::BAD_REQUEST,
            Json(json!({ "error": "workflow is not configured for webhook triggers" })),
        )
            .into_response();
    }

    if let Some(expected_secret) = workflow.webhook_secret.as_deref() {
        let actual = headers
            .get("x-openfoundry-webhook-secret")
            .and_then(|value| value.to_str().ok())
            .unwrap_or_default();
        if actual != expected_secret {
            return StatusCode::UNAUTHORIZED.into_response();
        }
    }

    let context = json!({
        "trigger": {
            "type": "webhook",
            "workflow_id": workflow_id,
        },
        "payload": body.context,
    });
    match dispatch_run(&state, &workflow, "webhook", None, context).await {
        Ok(run) => (StatusCode::CREATED, Json(run)).into_response(),
        Err(error) => (StatusCode::BAD_REQUEST, Json(json!({ "error": error }))).into_response(),
    }
}

pub async fn start_internal_lineage_run(
    State(state): State<AppState>,
    Path(workflow_id): Path<Uuid>,
    Json(body): Json<InternalLineageRunRequest>,
) -> impl IntoResponse {
    let Some(workflow) = load_or_404(&state, workflow_id).await else {
        return StatusCode::NOT_FOUND.into_response();
    };

    if workflow.status != "active" {
        return (
            StatusCode::BAD_REQUEST,
            Json(json!({ "error": "workflow must be active to run from lineage" })),
        )
            .into_response();
    }

    match dispatch_run(&state, &workflow, "lineage_build", None, body.context).await {
        Ok(run) => (StatusCode::CREATED, Json(run)).into_response(),
        Err(error) => (StatusCode::BAD_REQUEST, Json(json!({ "error": error }))).into_response(),
    }
}

pub async fn start_internal_triggered_run(
    State(state): State<AppState>,
    Path(workflow_id): Path<Uuid>,
    Json(body): Json<InternalTriggeredRunRequest>,
) -> impl IntoResponse {
    match execute_internal_triggered_run(&state, workflow_id, body).await {
        Ok(run) => (StatusCode::CREATED, Json(run)).into_response(),
        Err(error) if error.contains("not found") => {
            (StatusCode::NOT_FOUND, Json(json!({ "error": error }))).into_response()
        }
        Err(error) => (StatusCode::BAD_REQUEST, Json(json!({ "error": error }))).into_response(),
    }
}

pub async fn execute_internal_triggered_run(
    state: &AppState,
    workflow_id: Uuid,
    body: InternalTriggeredRunRequest,
) -> Result<crate::models::execution::WorkflowRun, String> {
    let Some(workflow) = (match load_workflow(state, workflow_id).await {
        Ok(workflow) => workflow,
        Err(error) => {
            tracing::error!("internal triggered run lookup failed: {error}");
            return Err(error.to_string());
        }
    }) else {
        return Err(format!("workflow {workflow_id} not found"));
    };

    dispatch_run(
        state,
        &workflow,
        &body.trigger_type,
        body.started_by,
        body.context,
    )
    .await
}

/// Single funnel for every inbound producer. Inserts the
/// `automation_runs` row at `state=Queued` and enqueues the
/// `automate.condition.v1` outbox event in the same Postgres
/// transaction. Tarea 5.3 invariant: row exists ⇔ Debezium has
/// (or will have) the condition event.
async fn dispatch_run(
    state: &AppState,
    workflow: &WorkflowDefinition,
    trigger_type: &str,
    started_by: Option<Uuid>,
    context: Value,
) -> Result<crate::models::execution::WorkflowRun, String> {
    if workflow.status != "active" && trigger_type != "manual" {
        return Err("workflow must be active for automatic execution".to_string());
    }

    let correlation_id = Uuid::now_v7();
    let run_id = derive_run_id(workflow.id, correlation_id);
    let tenant_id_str = workflow_tenant_id(workflow);

    let condition = AutomateConditionV1 {
        definition_id: workflow.id,
        tenant_id: tenant_id_str.clone(),
        correlation_id,
        triggered_by: started_by
            .map(|value| value.to_string())
            .unwrap_or_else(|| "system".to_string()),
        trigger_type: trigger_type.to_string(),
        trigger_payload: context.clone(),
    };

    let run = AutomationRun::new(
        run_id,
        tenant_uuid_from_str(&tenant_id_str),
        workflow.id,
        correlation_id,
        None,
    );

    let mut tx = state.db.begin().await.map_err(|e| e.to_string())?;
    insert_automation_run_in_tx(&mut tx, &run)
        .await
        .map_err(|e| e.to_string())?;
    enqueue_condition_in_tx(&mut tx, run_id, &condition)
        .await
        .map_err(|e| e.to_string())?;
    sqlx::query(
        r#"UPDATE workflows
           SET last_triggered_at = NOW(), updated_at = NOW()
           WHERE id = $1"#,
    )
    .bind(workflow.id)
    .execute(&mut *tx)
    .await
    .map_err(|e| e.to_string())?;
    tx.commit().await.map_err(|e| e.to_string())?;

    Ok(accepted_run(
        workflow.id,
        run_id,
        trigger_type,
        started_by,
        context,
        correlation_id,
    ))
}

async fn insert_automation_run_in_tx(
    tx: &mut sqlx::Transaction<'_, sqlx::Postgres>,
    run: &AutomationRun,
) -> Result<(), sqlx::Error> {
    let payload = serde_json::to_value(run).map_err(|err| sqlx::Error::Decode(Box::new(err)))?;
    let state = AutomationRun::state_str(run.current_state());
    let expires_at = StateMachine::expires_at(run);
    sqlx::query(&format!(
        "INSERT INTO {table} \
         (id, state, state_data, version, expires_at, created_at, updated_at) \
         VALUES ($1, $2, $3, 1, $4, now(), now()) \
         ON CONFLICT (id) DO NOTHING",
        table = AUTOMATION_RUNS_TABLE
    ))
    .bind(run.aggregate_id())
    .bind(&state)
    .bind(&payload)
    .bind(expires_at)
    .execute(&mut **tx)
    .await?;
    Ok(())
}

async fn enqueue_condition_in_tx(
    tx: &mut sqlx::Transaction<'_, sqlx::Postgres>,
    run_id: Uuid,
    condition: &AutomateConditionV1,
) -> Result<(), outbox::OutboxError> {
    let payload = serde_json::to_value(condition)?;
    let event_id =
        crate::event::derive_condition_event_id(condition.definition_id, condition.correlation_id);
    let event = OutboxEvent::new(
        event_id,
        "automation_run",
        run_id.to_string(),
        AUTOMATE_CONDITION_V1,
        payload,
    )
    .with_header(
        "x-audit-correlation-id",
        condition.correlation_id.to_string(),
    )
    .with_header("ol-job", format!("automation_run/{}", condition.tenant_id))
    .with_header("ol-run-id", run_id.to_string())
    .with_header("ol-producer", "workflow-automation-service");
    outbox::enqueue(tx, event).await
}

async fn load_or_404(state: &AppState, workflow_id: Uuid) -> Option<WorkflowDefinition> {
    match load_workflow(state, workflow_id).await {
        Ok(workflow) => workflow,
        Err(error) => {
            tracing::error!("workflow lookup failed: {error}");
            None
        }
    }
}

fn workflow_tenant_id(workflow: &WorkflowDefinition) -> String {
    workflow
        .trigger_config
        .get("tenant_id")
        .and_then(serde_json::Value::as_str)
        .map(str::to_string)
        .unwrap_or_else(|| workflow.owner_id.to_string())
}

fn accepted_run(
    workflow_id: Uuid,
    run_id: Uuid,
    trigger_type: &str,
    started_by: Option<Uuid>,
    context: Value,
    correlation_id: Uuid,
) -> crate::models::execution::WorkflowRun {
    crate::models::execution::WorkflowRun {
        id: run_id,
        workflow_id,
        trigger_type: trigger_type.to_string(),
        // The row lands in `Queued` and the consumer flips to
        // `Running` within milliseconds — surface `running` here so
        // the UI does not have to special-case the brief Queued
        // window. Operators with deeper visibility hit
        // `automation_runs.state` directly for the precise value.
        status: "running".to_string(),
        started_by,
        current_step_id: None,
        context: json!({
            "input": context,
            "automate": {
                "run_id": run_id,
                "correlation_id": correlation_id,
                "topic": AUTOMATE_CONDITION_V1,
                // Compatibility shim: the legacy field name
                // `temporal.authoritative` is what the UI
                // currently pattern-matches on. We map it to
                // `false` so the front-end falls back to reading
                // the live row from `GET /workflows/{id}/runs`
                // (Postgres-backed, single source of truth in the
                // Foundry-pattern runtime).
                "authoritative": false,
            }
        }),
        error_message: None,
        started_at: Utc::now(),
        finished_at: None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn accepted_run_returns_api_compatible_state() {
        let workflow_id = Uuid::now_v7();
        let run_id = Uuid::now_v7();
        let started_by = Uuid::now_v7();
        let correlation_id = Uuid::now_v7();

        let run = accepted_run(
            workflow_id,
            run_id,
            "manual",
            Some(started_by),
            json!({"customer_id": "c-1"}),
            correlation_id,
        );

        assert_eq!(run.id, run_id);
        assert_eq!(run.workflow_id, workflow_id);
        assert_eq!(run.status, "running");
        assert_eq!(run.started_by, Some(started_by));
        assert_eq!(run.context["input"]["customer_id"], "c-1");
        assert_eq!(run.context["automate"]["run_id"], run_id.to_string());
        assert_eq!(
            run.context["automate"]["correlation_id"],
            correlation_id.to_string()
        );
        assert_eq!(run.context["automate"]["topic"], AUTOMATE_CONDITION_V1);
        assert_eq!(run.context["automate"]["authoritative"], false);
    }
}
