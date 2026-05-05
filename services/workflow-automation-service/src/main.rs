//! `workflow-automation-service` binary.
//!
//! Post-S8 ownership boundary: workflow definitions/runs +
//! Foundry-pattern condition consumer (legacy
//! `workflow-automation-service`), saga substrate + saga consumer
//! (legacy `automation-operations-service`), human-in-the-loop
//! approvals state machine + outbox (legacy `approvals-service`).
//! See ADR-0030 + `service-consolidation-map.md`.
//!
//! The companion `approvals-timeout-sweep` CronJob binary lives in
//! `src/bin/approvals_timeout_sweep.rs` and reuses
//! `workflow_automation_service::approvals`.

use std::{net::SocketAddr, sync::Arc, time::Duration};

use auth_middleware::jwt::JwtConfig;
use axum::{
    Router, middleware,
    routing::{get, post},
};
use event_bus_data::{
    DataBusConfig, DataPublisher, KafkaPublisher, KafkaSubscriber, ServicePrincipal,
};
use idempotency::{IdempotencyStore, postgres::PgIdempotencyStore};
use sqlx::{PgPool, postgres::PgPoolOptions};
use tracing_subscriber::EnvFilter;

use workflow_automation_service::{
    AppState, approvals,
    automation_operations::{
        self,
        domain::saga_consumer::{
            self, CONSUMER_GROUP as SAGA_CONSUMER_GROUP,
            PROCESSED_EVENTS_TABLE as SAGA_PROCESSED_EVENTS_TABLE, SagaConsumer,
        },
    },
    config::AppConfig,
    domain::{
        self,
        condition_consumer::{
            self, CONSUMER_GROUP as CONDITION_CONSUMER_GROUP, ConditionConsumer,
            PROCESSED_EVENTS_TABLE as CONDITION_PROCESSED_EVENTS_TABLE,
        },
        effect_dispatcher::EffectDispatcher,
    },
    handlers,
};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::try_from_default_env().unwrap_or_else(|_| {
                EnvFilter::new("workflow_automation_service=info,tower_http=info")
            }),
        )
        .init();

    let app_config = AppConfig::from_env()?;
    let db = PgPoolOptions::new()
        .max_connections(10)
        .acquire_timeout(Duration::from_secs(10))
        .connect(&app_config.database_url)
        .await?;
    let http_client = reqwest::Client::builder()
        .timeout(Duration::from_secs(30))
        .build()?;

    let jwt_config = JwtConfig::new(&app_config.jwt_secret).with_env_defaults();

    let state = AppState {
        db: db.clone(),
        http_client: http_client.clone(),
        jwt_config: jwt_config.clone(),
        nats_url: app_config.nats_url.clone(),
        pipeline_service_url: app_config.pipeline_service_url.clone(),
        ontology_service_url: app_config.ontology_service_url.clone(),
        audit_compliance_service_url: app_config.audit_compliance_service_url.clone(),
        audit_compliance_bearer_token: app_config.audit_compliance_bearer_token.clone(),
        approval_ttl_hours: app_config.approval_ttl_hours,
    };

    let app = build_router(state.clone(), jwt_config.clone());

    // Legacy NATS consumer for `of.workflows.run.requested`. Kept
    // until `pipeline-schedule-service` is migrated to publish
    // directly to `automate.condition.v1` (out of scope for FASE 5).
    {
        let nats_state = state.clone();
        tokio::spawn(async move {
            if let Err(error) = domain::workflow_run_requested::consume(nats_state).await {
                tracing::error!("workflow run requested consumer stopped: {error}");
            }
        });
    }

    // ── Foundry-pattern Kafka condition consumer (FASE 5 / Tarea 5.3) ──
    // Boot is best-effort: if Kafka is not configured (e.g. dev
    // running without the broker) we skip the consumer task and
    // log a warning so HTTP-only flows still come up.
    match build_condition_consumer(db.clone(), http_client.clone()) {
        Ok(consumer) => {
            let subscriber = KafkaSubscriber::new(
                &data_bus_config_from_env(CONDITION_CONSUMER_GROUP)?,
                CONDITION_CONSUMER_GROUP,
            )?;
            let consumer = Arc::new(consumer);
            tokio::spawn(async move {
                if let Err(error) = condition_consumer::run(consumer, subscriber).await {
                    tracing::error!("automate.condition.v1 consumer stopped: {error}");
                }
            });
        }
        Err(error) => {
            tracing::warn!(
                error = %error,
                "skipping automate.condition.v1 consumer; HTTP API still online",
            );
        }
    }

    // ── Foundry-pattern Kafka saga consumer (FASE 6 / Tarea 6.3, S8 merge) ──
    // Same best-effort boot semantics as the condition consumer.
    // The Kafka consumer group is preserved as the legacy string
    // `automation-operations-service` so changing the deployable
    // does NOT reprocess the topic log.
    match build_saga_consumer(db.clone()) {
        Ok(consumer) => {
            let subscriber = KafkaSubscriber::new(
                &data_bus_config_from_env(SAGA_CONSUMER_GROUP)?,
                SAGA_CONSUMER_GROUP,
            )?;
            let consumer = Arc::new(consumer);
            tokio::spawn(async move {
                if let Err(error) = saga_consumer::run(consumer, subscriber).await {
                    tracing::error!("saga.step.requested.v1 consumer stopped: {error}");
                }
            });
        }
        Err(error) => {
            tracing::warn!(
                error = %error,
                "skipping saga.step.requested.v1 consumer; HTTP API still online",
            );
        }
    }

    let addr: SocketAddr = format!("{}:{}", app_config.host, app_config.port).parse()?;
    let listener = tokio::net::TcpListener::bind(addr).await?;
    tracing::info!("workflow-automation-service listening on http://{}", addr);
    axum::serve(listener, app).await?;
    Ok(())
}

fn build_router(state: AppState, jwt_config: JwtConfig) -> Router {
    let workflow_authenticated = Router::new()
        .route(
            "/workflows",
            get(handlers::crud::list_workflows).post(handlers::crud::create_workflow),
        )
        .route(
            "/workflows/{id}",
            get(handlers::crud::get_workflow)
                .patch(handlers::crud::update_workflow)
                .delete(handlers::crud::delete_workflow),
        )
        .route(
            "/workflows/{id}/runs",
            get(handlers::runs::list_runs).post(handlers::execute::start_manual_run),
        )
        .route(
            "/workflows/approvals/{approval_id}/continue",
            post(handlers::approvals::continue_after_approval),
        )
        .layer(middleware::from_fn_with_state(
            jwt_config.clone(),
            auth_middleware::layer::auth_layer,
        ));

    let workflow_unauthenticated = Router::new()
        .route(
            "/workflows/{id}/webhook",
            post(handlers::execute::trigger_webhook),
        )
        .route(
            "/workflows/{id}/_internal/lineage",
            post(handlers::execute::start_internal_lineage_run),
        )
        .route(
            "/workflows/{id}/_internal/triggered",
            post(handlers::execute::start_internal_triggered_run),
        );

    // ── automations subdomain (legacy automation-operations-service) ──
    let automations = Router::new()
        .route(
            "/automations",
            get(automation_operations::handlers::list_items)
                .post(automation_operations::handlers::create_item),
        )
        .route(
            "/automations/{id}",
            get(automation_operations::handlers::get_item),
        )
        .route(
            "/automations/{parent_id}/runs",
            get(automation_operations::handlers::list_secondary)
                .post(automation_operations::handlers::create_secondary),
        )
        .layer(middleware::from_fn_with_state(
            jwt_config.clone(),
            auth_middleware::layer::auth_layer,
        ));

    // ── approvals subdomain (legacy approvals-service) ──
    let approvals_routes = Router::new()
        .route(
            "/approvals",
            get(approvals::handlers::approvals::list_approvals)
                .post(approvals::handlers::approvals::create_approval),
        )
        .route(
            "/approvals/{approval_id}/decide",
            post(approvals::handlers::approvals::decide_approval),
        )
        .layer(middleware::from_fn_with_state(
            jwt_config,
            auth_middleware::layer::auth_layer,
        ));

    Router::new()
        .nest(
            "/api/v1",
            workflow_authenticated
                .merge(workflow_unauthenticated)
                .merge(automations)
                .merge(approvals_routes),
        )
        .route("/healthz", get(|| async { "ok" }))
        .with_state(state)
}

#[derive(Debug, thiserror::Error)]
enum ConsumerBootError {
    #[error("KAFKA_BOOTSTRAP_SERVERS is not set")]
    KafkaUnconfigured,
    #[error("OF_ONTOLOGY_ACTIONS_URL is not set")]
    OntologyUrlUnconfigured,
    #[error("OF_ONTOLOGY_ACTIONS_BEARER_TOKEN is not set")]
    OntologyTokenUnconfigured,
    #[error("kafka publisher init failed: {0}")]
    KafkaPublisher(#[from] event_bus_data::PublishError),
}

fn build_condition_consumer(
    db: PgPool,
    http_client: reqwest::Client,
) -> Result<ConditionConsumer, ConsumerBootError> {
    if std::env::var("KAFKA_BOOTSTRAP_SERVERS").is_err() {
        return Err(ConsumerBootError::KafkaUnconfigured);
    }
    let ontology_url = first_non_empty_env(&[
        "OF_ONTOLOGY_ACTIONS_URL",
        "ONTOLOGY_ACTIONS_SERVICE_URL",
        "ONTOLOGY_SERVICE_URL",
        "OF_ONTOLOGY_ACTIONS_GRPC_ADDR",
    ])
    .ok_or(ConsumerBootError::OntologyUrlUnconfigured)?;
    let ontology_token = first_non_empty_env(&[
        "OF_ONTOLOGY_ACTIONS_BEARER_TOKEN",
        "ONTOLOGY_ACTIONS_BEARER_TOKEN",
    ])
    .ok_or(ConsumerBootError::OntologyTokenUnconfigured)?;

    let dispatcher = EffectDispatcher::new(http_client, ontology_url, ontology_token);
    let publisher: Arc<dyn DataPublisher> =
        Arc::new(KafkaPublisher::from_env(CONDITION_CONSUMER_GROUP)?);
    let idempotency: Arc<dyn IdempotencyStore> = Arc::new(PgIdempotencyStore::new(
        db.clone(),
        CONDITION_PROCESSED_EVENTS_TABLE,
    ));
    Ok(ConditionConsumer::new(
        db,
        idempotency,
        dispatcher,
        publisher,
    ))
}

fn build_saga_consumer(db: PgPool) -> Result<SagaConsumer, ConsumerBootError> {
    if std::env::var("KAFKA_BOOTSTRAP_SERVERS").is_err() {
        return Err(ConsumerBootError::KafkaUnconfigured);
    }
    let publisher: Arc<dyn DataPublisher> =
        Arc::new(KafkaPublisher::from_env(SAGA_CONSUMER_GROUP)?);
    let idempotency: Arc<dyn IdempotencyStore> = Arc::new(PgIdempotencyStore::new(
        db.clone(),
        SAGA_PROCESSED_EVENTS_TABLE,
    ));
    Ok(SagaConsumer::new(db, idempotency, publisher))
}

/// Build the `event-bus-data` config from the standard OpenFoundry
/// Kafka env vars. Same shape as `reindex-coordinator-service`.
fn data_bus_config_from_env(service_name: &str) -> Result<DataBusConfig, ConsumerBootError> {
    let brokers = std::env::var("KAFKA_BOOTSTRAP_SERVERS")
        .map_err(|_| ConsumerBootError::KafkaUnconfigured)?;
    let service = first_non_empty_env(&["KAFKA_SASL_USERNAME", "KAFKA_CLIENT_ID"])
        .unwrap_or_else(|| service_name.to_string());
    let mut principal = match first_non_empty_env(&["KAFKA_SASL_PASSWORD"]) {
        Some(password) => ServicePrincipal::scram_sha_512(service, password),
        None => ServicePrincipal::insecure_dev(service),
    };
    if let Some(mechanism) = first_non_empty_env(&["KAFKA_SASL_MECHANISM"]) {
        principal.mechanism = mechanism;
    }
    if let Some(protocol) = first_non_empty_env(&["KAFKA_SECURITY_PROTOCOL"]) {
        principal.security_protocol = protocol;
    }
    Ok(DataBusConfig::new(brokers, principal))
}

fn first_non_empty_env(keys: &[&'static str]) -> Option<String> {
    for key in keys {
        if let Ok(value) = std::env::var(key) {
            let trimmed = value.trim();
            if !trimmed.is_empty() {
                return Some(trimmed.to_string());
            }
        }
    }
    None
}
