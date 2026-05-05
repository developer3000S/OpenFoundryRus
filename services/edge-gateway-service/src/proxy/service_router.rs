use auth_middleware::{Claims, jwt, jwt::JwtConfig, tenant::TenantContext};
use axum::{
    body::Body,
    extract::{Request, State},
    http::{Method, StatusCode, Uri, header::AUTHORIZATION},
    response::{IntoResponse, Response},
};
use reqwest::Client;
use serde_json::json;

use crate::config::GatewayConfig;

/// Reverse-proxy handler: forwards requests to bounded-context owners based on URL prefix.
pub async fn proxy_handler(
    State((config, client, jwt_config)): State<(GatewayConfig, Client, JwtConfig)>,
    mut req: Request,
) -> Response {
    let path = req.uri().path();
    let claims = req
        .headers()
        .get(AUTHORIZATION)
        .and_then(|value| value.to_str().ok())
        .and_then(|value| value.strip_prefix("Bearer "))
        .and_then(|token| jwt::decode_token(&jwt_config, token).ok());
    if let Some(claims) = claims.as_ref() {
        if let Err(response) = enforce_zero_trust_scope(claims, req.method(), path) {
            return response;
        }
    }
    let tenant = claims.as_ref().map(TenantContext::from_claims);

    let upstream_base = if path.starts_with("/api/v1/auth/sso")
        || path.starts_with("/api/v1/api-keys")
        || path.starts_with("/api/v1/applications")
        || path.starts_with("/api/v1/oauth/clients")
        || path.starts_with("/api/v1/external-integrations")
    {
        &config.oauth_integration_service_url
    } else if path.starts_with("/api/v1/auth/register")
        || path.starts_with("/api/v1/auth/login")
        || path.starts_with("/api/v1/auth/refresh")
        || path.starts_with("/api/v1/auth/mfa")
        || path.starts_with("/api/v1/auth/sessions")
        || path == "/api/v1/users/me"
        || path == "/api/v2/admin/users/me"
    {
        &config.identity_federation_service_url
    } else if path.starts_with("/api/v1/auth/cipher") {
        &config.cipher_service_url
    } else if path.starts_with("/api/v1/control-panel")
        || path.starts_with("/api/v2/admin/control-panel")
    {
        &config.session_governance_service_url
    } else if path.starts_with("/api/v1/users/")
        && (path.contains("/roles") || path.contains("/groups"))
    {
        &config.authorization_policy_service_url
    } else if path.starts_with("/api/v1/roles")
        || path.starts_with("/api/v1/permissions")
        || path.starts_with("/api/v1/groups")
        || path.starts_with("/api/v1/policies")
        || path.starts_with("/api/v1/restricted-views")
        || path.starts_with("/api/v2/admin/roles")
        || path.starts_with("/api/v2/admin/permissions")
        || path.starts_with("/api/v2/admin/groups")
        || path.starts_with("/api/v2/admin/policies")
        || path.starts_with("/api/v2/admin/restricted-views")
    {
        &config.authorization_policy_service_url
    } else if path.starts_with("/api/v1/security-governance")
        || path == "/api/v1/audit/classifications"
        || path.starts_with("/api/v1/audit/governance/")
        || path == "/api/v1/audit/compliance/posture"
    {
        &config.security_governance_service_url
    } else if path.starts_with("/api/v1/network-boundaries")
        || path.starts_with("/api/v1/network-boundary")
        || path.starts_with("/api/v1/data-connection/egress-policies")
    {
        &config.network_boundary_service_url
    } else if path.starts_with("/api/v1/data-connection") {
        // Data Connection app surface (catalog, sources, capabilities, test, ...).
        // Egress policies are handled above by network-boundary-service.
        &config.connector_management_service_url
    } else if path.starts_with("/api/v1/checkpoints-purpose") {
        &config.checkpoints_purpose_service_url
    } else if path.starts_with("/api/v1/retention")
        || path.ends_with("/retention")
        || path.contains("/transactions/") && path.ends_with("/retention")
        // P4 — Foundry "View retention policies for a dataset [Beta]"
        // surface. The dataset-scoped resolution + preview live under
        // `/v1/datasets/{rid}/applicable-policies` and
        // `/retention-preview` and route to retention-policy-service.
        // The bare `/v1/policies` prefix is owned by
        // authorization-policy-service (RBAC); retention policy CRUD
        // is accessed via `/api/v1/retention/policies` instead, which
        // already matches the `/api/v1/retention` rule above.
        || path.ends_with("/applicable-policies")
        || path.ends_with("/retention-preview")
    {
        // S8 / ADR-0030 (B15): retention-policy-service merged → audit-compliance-service.
        &config.audit_compliance_service_url
    } else if path.starts_with("/api/v1/lineage-deletions") || path == "/api/v1/audit/gdpr/erase" {
        // S8 / ADR-0030 (B15): lineage-deletion-service merged → audit-compliance-service.
        &config.audit_compliance_service_url
    } else if path == "/api/v1/audit/overview"
        || path == "/api/v1/audit/events"
        || path.starts_with("/api/v1/audit/events/")
        || path == "/api/v1/audit/collectors"
        || path == "/api/v1/audit/anomalies"
        || path == "/api/v1/audit/reports"
        || path == "/api/v1/audit/reports/generate"
        || path == "/api/v1/audit/policies"
        || path.starts_with("/api/v1/audit/policies/")
        || path == "/api/v1/audit/gdpr/export"
    {
        &config.audit_compliance_service_url
    } else if path.starts_with("/api/v1/tenancy")
        || path.starts_with("/api/v1/organizations")
        || path.starts_with("/api/v1/enrollments")
        || path.starts_with("/api/v1/spaces")
        || path.starts_with("/api/v1/projects")
        || path.starts_with("/api/v1/nexus/spaces")
        || path.starts_with("/api/v1/ontology/projects")
    {
        &config.tenancy_organizations_service_url
    } else if path.starts_with("/api/v1/auth")
        || path.starts_with("/api/v1/users")
        || path.starts_with("/api/v2/admin")
    {
        &config.identity_federation_service_url
    } else if path.starts_with("/api/v1/connector-agents")
        || (path.starts_with("/api/v1/connections/")
            && (path.ends_with("/sync") || path.ends_with("/sync-jobs")))
    {
        &config.ingestion_replication_service_url
    } else if path.starts_with("/api/v1/connections/")
        && (path.ends_with("/discover")
            || path.contains("/registrations")
            || path.ends_with("/virtual-tables/query"))
    {
        &config.virtual_table_service_url
    } else if path.starts_with("/api/v1/connections/") && path.contains("/hyperauto/") {
        &config.connector_management_service_url
    } else if path.starts_with("/api/v1/connectors/catalog")
        || path.starts_with("/api/v1/connections")
    {
        &config.connector_management_service_url
    } else if path.starts_with("/api/v1/datasets/")
        && (path.ends_with("/quality") || path.contains("/quality/") || path.ends_with("/lint"))
    {
        // S8 / ADR-0030: dataset-quality-service merged → dataset-versioning-service.
        &config.dataset_versioning_service_url
    } else if path.starts_with("/api/v1/iceberg-tables")
        || path.starts_with("/iceberg/v1/")
        || path == "/iceberg/v1"
        || path.starts_with("/v1/iceberg-clients")
    {
        // ADR-0041 — both the Foundry admin surface
        // (`/api/v1/iceberg-tables/*` for the `/iceberg-tables` UI in
        // `apps/web`) and the Iceberg REST Catalog spec endpoints
        // (`/iceberg/v1/*` consumed by PyIceberg / Spark / Trino /
        // Snowflake) are owned by `iceberg-catalog-service`.
        // `/v1/iceberg-clients/api-tokens` is the API-token issuer the
        // UI calls when a user mints a long-lived `ofty_*` token.
        &config.iceberg_catalog_service_url
    } else if path.starts_with("/api/v1/datasets/")
        && (path.ends_with("/versions")
            || path.ends_with("/transactions")
            || path.ends_with("/branches")
            || path.contains("/branches/")
            // T6.x — Foundry "dataset views" (file-list views with their
            // own schema) live in dataset-versioning-service. The
            // catalog still owns the SQL-view routes (`/views/{view}`,
            // `/views/{view}/preview`); we route the DVS-specific
            // suffixes to the versioning service. P2's row preview
            // sits at `/views/{view}/data` for the same reason — to
            // avoid colliding with the catalog's SQL-view preview.
            || path.ends_with("/views/current")
            || path.ends_with("/views/at")
            || (path.contains("/views/")
                && (path.ends_with("/files")
                    || path.ends_with("/schema")
                    || path.ends_with("/data")))
            // P3 — top-level `/files` and per-file download / upload
            // routes for the Foundry "Backing filesystem" Files tab.
            // Catalog still owns `/filesystem` (legacy directory listing)
            // — different ending, so this rule doesn't shadow it.
            || (path.ends_with("/files")
                && !path.contains("/views/"))
            || (path.contains("/files/") && path.ends_with("/download"))
            || (path.contains("/transactions/") && path.ends_with("/files"))
            // P3 — Storage details panel (manage role).
            || path.ends_with("/storage-details"))
    {
        &config.dataset_versioning_service_url
    } else if path.starts_with("/api/v1/datasets") || path.starts_with("/api/v2/filesystem") {
        // S8 / ADR-0030: data-asset-catalog-service merged → dataset-versioning-service.
        &config.dataset_versioning_service_url
    } else if path.starts_with("/api/v1/queries") {
        &config.query_service_url
    } else if path.starts_with("/api/v1/pipelines/triggers/cron/") {
        // S8 / ADR-0030: pipeline-schedule-service merged → pipeline-build-service.
        &config.pipeline_build_service_url
    } else if path.starts_with("/api/v1/pipelines/")
        && (path.ends_with("/run") || path.contains("/runs/") || path.ends_with("/runs"))
    {
        &config.pipeline_build_service_url
    } else if path.starts_with("/api/v1/pipelines") {
        // S8 / ADR-0030: pipeline-authoring-service merged → pipeline-build-service.
        &config.pipeline_build_service_url
    } else if path.starts_with("/api/v1/lineage") {
        &config.lineage_service_url
    } else if path.starts_with("/api/v1/ontology/functions")
        || path.starts_with("/api/v1/ontology/funnel")
        || path.starts_with("/api/v1/ontology/storage/insights")
        || path.starts_with("/api/v1/ontology/actions")
        || path.starts_with("/api/v1/ontology/rules")
        || (path.starts_with("/api/v1/ontology/types/")
            && path.contains("/objects/")
            && path.contains("/inline-edit/"))
        || (path.starts_with("/api/v1/ontology/types/") && path.ends_with("/rules"))
        || (path.starts_with("/api/v1/ontology/objects/") && path.ends_with("/rule-runs"))
    {
        // S8.1 — `ontology-actions-service` is the sole runtime owner
        // of the ontology action / funnel / function / rule HTTP
        // surfaces (per ADR-0030 + service-consolidation-map.md).
        &config.ontology_actions_service_url
    } else if path.starts_with("/api/v1/ontology/search")
        || path.starts_with("/api/v1/ontology/graph")
        || path.starts_with("/api/v1/ontology/quiver")
        || path.starts_with("/api/v1/ontology/object-sets")
        || (path.starts_with("/api/v1/ontology/types/")
            && (path.ends_with("/objects/query") || path.ends_with("/objects/knn")))
    {
        &config.ontology_query_service_url
    } else if path.starts_with("/api/v1/ontology/links/") && path.contains("/instances") {
        &config.object_database_service_url
    } else if path.starts_with("/api/v1/ontology/types/") && path.contains("/objects") {
        &config.object_database_service_url
    } else if path.starts_with("/api/v1/ontology/interfaces")
        || path.starts_with("/api/v1/ontology/shared-property-types")
        || path.starts_with("/api/v1/ontology/links")
        || path.starts_with("/api/v1/ontology/types")
    {
        &config.ontology_definition_service_url
    } else if path.starts_with("/api/v1/ontology") {
        &config.ontology_definition_service_url
    } else if path.starts_with("/api/v1/workflows/approvals")
        || path.starts_with("/api/v1/approvals")
    {
        &config.approvals_service_url
    } else if path.starts_with("/api/v1/workflows") {
        &config.workflow_service_url
    } else if path.starts_with("/api/v1/notebooks") {
        &config.notebook_service_url
    } else if path.starts_with("/api/v1/notepad") {
        // S8 / ADR-0030: document-reporting-service merged →
        // notebook-runtime-service. Notepad routes now hit the notebook
        // runtime; the legacy `document_reporting_service_url` config is
        // kept for backwards compat with deployed values files.
        &config.notebook_service_url
    } else if path.starts_with("/api/v1/notifications") {
        &config.notification_service_url
    } else if path.starts_with("/api/v1/ml/experiments") || path.starts_with("/api/v1/ml/runs") {
        // S8: experiments/runs absorbed by `model-catalog-service`.
        &config.model_catalog_service_url
    } else if path.starts_with("/api/v1/ml/models") || path.starts_with("/api/v1/ml/model-versions")
    {
        &config.model_catalog_service_url
    } else if path.starts_with("/api/v1/ml/deployments/") && path.ends_with("/drift") {
        // S8 / ADR-0030: model-evaluation-service merged → model-deployment-service.
        &config.model_deployment_service_url
    } else if path.starts_with("/api/v1/ml/deployments/") && path.ends_with("/predict") {
        // S8 / ADR-0030: model-serving-service merged → model-deployment-service.
        &config.model_deployment_service_url
    } else if path.starts_with("/api/v1/ml/batch-predictions") {
        // S8 / ADR-0030: model-inference-history-service merged → model-deployment-service.
        &config.model_deployment_service_url
    } else if path.starts_with("/api/v1/ml/deployments") {
        &config.model_deployment_service_url
    } else if path.starts_with("/api/v1/ml") {
        &config.ml_service_url
    } else if path.starts_with("/api/v1/ai/guardrails/evaluate")
        || path.starts_with("/api/v1/ai/evaluations")
    {
        &config.ai_evaluation_service_url
    } else if path.starts_with("/api/v1/ai/providers") {
        &config.llm_catalog_service_url
    } else if path.starts_with("/api/v1/ai/prompts") {
        &config.prompt_workflow_service_url
    } else if path.starts_with("/api/v1/ai/knowledge-bases/") && path.ends_with("/search") {
        &config.retrieval_context_service_url
    } else if path.starts_with("/api/v1/ai/knowledge-bases") {
        &config.knowledge_index_service_url
    } else if path.starts_with("/api/v1/ai/conversations") {
        &config.conversation_state_service_url
    } else if path.starts_with("/api/v1/ai/tools") {
        &config.ai_service_url
    } else if path.starts_with("/api/v1/ai") {
        &config.ai_service_url
    } else if path.starts_with("/api/v1/entity-resolution") || path.starts_with("/api/v1/fusion") {
        &config.entity_resolution_service_url
    } else if path.starts_with("/api/v1/streaming") {
        &config.streaming_service_url
    } else if path.starts_with("/api/v1/reports") {
        &config.report_service_url
    } else if path.starts_with("/api/v1/geospatial") {
        &config.geospatial_intelligence_service_url
    } else if path.starts_with("/api/v1/code-repos/repositories/") && path.ends_with("/branches") {
        &config.global_branch_service_url
    } else if path.starts_with("/api/v1/code-repos") {
        &config.code_repo_service_url
    } else if path.starts_with("/api/v1/federation-product-exchange")
        || path.starts_with("/api/v1/nexus")
        || path == "/api/v1/marketplace/installs"
        || path == "/api/v1/marketplace/devops/branches"
    {
        &config.federation_product_exchange_service_url
    } else if path.starts_with("/api/v1/marketplace/devops") {
        &config.product_distribution_service_url
    } else if path.starts_with("/api/v1/marketplace") {
        &config.marketplace_catalog_service_url
    } else if path.starts_with("/api/v1/audit/sds") {
        // S8 / ADR-0030 (B15): sds-service merged → audit-compliance-service.
        &config.audit_compliance_service_url
    } else if path.starts_with("/api/v1/audit") {
        &config.audit_compliance_service_url
    } else if path.starts_with("/api/v1/widgets") {
        &config.application_composition_service_url
    } else if path.starts_with("/api/v1/apps/public/")
        || path == "/api/v1/apps/templates"
        || path == "/api/v1/apps/from-template"
        || (path.starts_with("/api/v1/apps/") && path.ends_with("/slate-package"))
        || (path.starts_with("/api/v1/apps/") && path.ends_with("/versions"))
        || (path.starts_with("/api/v1/apps/") && path.ends_with("/publish"))
    {
        &config.application_curation_service_url
    } else if path.starts_with("/api/v1/apps") {
        &config.app_builder_service_url
    } else {
        return gateway_error(
            StatusCode::NOT_FOUND,
            "unknown_service_route",
            "unknown service route",
        );
    };

    let upstream_path_and_query = upstream_path_and_query(req.uri());
    let uri = format!("{upstream_base}{upstream_path_and_query}");

    let Ok(uri) = uri.parse::<Uri>() else {
        return gateway_error(
            StatusCode::BAD_GATEWAY,
            "invalid_upstream_uri",
            "invalid upstream URI",
        );
    };
    *req.uri_mut() = uri;

    // Forward the request via reqwest while preserving HTTP-level compatibility.
    let method = req.method().clone();
    let url = req.uri().to_string();
    let headers = req.headers().clone();
    let body_limit = tenant
        .as_ref()
        .map(|tenant| tenant.clamp_request_body_bytes(10 * 1024 * 1024))
        .unwrap_or(10 * 1024 * 1024);

    let body_bytes = match axum::body::to_bytes(req.into_body(), body_limit).await {
        Ok(b) => b,
        Err(_) => {
            return gateway_error(
                StatusCode::PAYLOAD_TOO_LARGE,
                "body_too_large",
                "body too large",
            );
        }
    };

    let mut upstream_req = client.request(method, &url);
    for (key, value) in headers.iter() {
        if key != "host" {
            upstream_req = upstream_req.header(key, value);
        }
    }
    if let Some(tenant) = tenant {
        upstream_req = upstream_req
            .header("x-openfoundry-tenant-scope", tenant.scope_id)
            .header("x-openfoundry-tenant-tier", tenant.tier)
            .header(
                "x-openfoundry-quota-query-limit",
                tenant.quotas.max_query_limit.to_string(),
            )
            .header(
                "x-openfoundry-quota-pipeline-workers",
                tenant.quotas.max_pipeline_workers.to_string(),
            )
            .header(
                "x-openfoundry-quota-requests-per-minute",
                tenant.quotas.requests_per_minute.to_string(),
            );
    }
    if let Some(claims) = claims.as_ref() {
        upstream_req = apply_auth_context_headers(upstream_req, claims);
    }
    upstream_req = upstream_req.body(body_bytes);

    match upstream_req.send().await {
        Ok(resp) => {
            let status =
                StatusCode::from_u16(resp.status().as_u16()).unwrap_or(StatusCode::BAD_GATEWAY);
            let headers = resp.headers().clone();
            let body = resp.bytes().await.unwrap_or_default();

            let mut response = Response::builder().status(status);
            for (key, value) in headers.iter() {
                response = response.header(key, value);
            }
            response.body(Body::from(body)).unwrap_or_else(|_| {
                gateway_error(
                    StatusCode::INTERNAL_SERVER_ERROR,
                    "proxy_response_build_failed",
                    "proxy error",
                )
            })
        }
        Err(e) => {
            tracing::error!("upstream request failed: {e}");
            gateway_error(
                StatusCode::BAD_GATEWAY,
                "upstream_unavailable",
                "upstream unavailable",
            )
        }
    }
}

fn enforce_zero_trust_scope(claims: &Claims, method: &Method, path: &str) -> Result<(), Response> {
    if !claims.allows_http_method(method.as_str()) {
        return Err(gateway_error(
            StatusCode::FORBIDDEN,
            "scoped_session_method_denied",
            "session scope does not allow this HTTP method",
        ));
    }
    if !claims.allows_path(path) {
        return Err(gateway_error(
            StatusCode::FORBIDDEN,
            "scoped_session_path_denied",
            "session scope does not allow this path",
        ));
    }
    Ok(())
}

fn apply_auth_context_headers(
    mut request: reqwest::RequestBuilder,
    claims: &Claims,
) -> reqwest::RequestBuilder {
    request = request
        .header("x-openfoundry-auth-sub", claims.sub.to_string())
        .header("x-openfoundry-auth-email", claims.email.as_str())
        .header("x-openfoundry-auth-methods", claims.auth_methods.join(","))
        .header(
            "x-openfoundry-zero-trust",
            if claims.session_scope.is_some() {
                "scoped"
            } else {
                "standard"
            },
        );

    if let Some(org_id) = claims.org_id {
        request = request.header("x-openfoundry-org-id", org_id.to_string());
    }
    if let Some(session_kind) = claims.session_kind.as_deref() {
        request = request.header("x-openfoundry-session-kind", session_kind);
    }
    if let Some(clearance) = claims.classification_clearance() {
        request = request.header("x-openfoundry-classification-clearance", clearance);
    }
    if let Some(scope) = claims.session_scope.as_ref() {
        if let Some(workspace) = scope.workspace.as_deref() {
            request = request.header("x-openfoundry-scope-workspace", workspace);
        }
        if !scope.allowed_path_prefixes.is_empty() {
            request = request.header(
                "x-openfoundry-scope-path-prefixes",
                scope.allowed_path_prefixes.join(","),
            );
        }
        if !scope.allowed_org_ids.is_empty() {
            request = request.header(
                "x-openfoundry-allowed-org-ids",
                scope
                    .allowed_org_ids
                    .iter()
                    .map(uuid::Uuid::to_string)
                    .collect::<Vec<_>>()
                    .join(","),
            );
        }
        if !scope.allowed_markings.is_empty() {
            request = request.header(
                "x-openfoundry-allowed-markings",
                scope.allowed_markings.join(","),
            );
        }
        if !scope.restricted_view_ids.is_empty() {
            request = request.header(
                "x-openfoundry-restricted-view-ids",
                scope
                    .restricted_view_ids
                    .iter()
                    .map(uuid::Uuid::to_string)
                    .collect::<Vec<_>>()
                    .join(","),
            );
        }
        if scope.consumer_mode {
            request = request.header("x-openfoundry-consumer-mode", "true");
        }
        if let Some(guest_email) = scope.guest_email.as_deref() {
            request = request
                .header("x-openfoundry-guest-email", guest_email)
                .header("x-openfoundry-guest-access", "true");
        }
    }

    request
}

fn upstream_path_and_query(uri: &Uri) -> String {
    let path = rewrite_upstream_path(uri.path());
    match uri.query() {
        Some(query) => format!("{path}?{query}"),
        None => path,
    }
}

fn rewrite_upstream_path(path: &str) -> String {
    if path == "/api/v1/datasets/catalog/facets" {
        return "/v1/catalog/facets".to_string();
    }

    if let Some(rest) = path.strip_prefix("/api/v1/datasets") {
        let canonical = format!("/v1/datasets{rest}");
        if let Some(prefix) = canonical.strip_suffix("/filesystem") {
            return format!("{prefix}/files");
        }
        return canonical;
    }

    path.to_string()
}

fn gateway_error(status: StatusCode, code: &str, message: &str) -> Response {
    (
        status,
        axum::Json(json!({
            "error": {
                "code": code,
                "message": message,
            }
        })),
    )
        .into_response()
}

#[cfg(test)]
mod tests {
    use axum::{Router, body::Body, routing::any};
    use serde_json::json;
    use tower::ServiceExt;
    use uuid::Uuid;
    use wiremock::{
        Mock, MockServer, ResponseTemplate,
        matchers::{method, path, query_param},
    };

    use super::*;
    use auth_middleware::claims::SessionScope;

    fn scoped_claims() -> Claims {
        Claims {
            sub: Uuid::nil(),
            iat: 0,
            exp: i64::MAX,
            iss: None,
            aud: None,
            jti: Uuid::nil(),
            email: "guest@example.com".to_string(),
            name: "Guest".to_string(),
            roles: vec!["guest".to_string()],
            permissions: vec!["datasets:read".to_string()],
            org_id: Some(Uuid::nil()),
            attributes: json!({ "classification_clearance": "confidential" }),
            auth_methods: vec!["guest".to_string()],
            token_use: Some("access".to_string()),
            api_key_id: None,
            session_kind: Some("guest_session".to_string()),
            session_scope: Some(SessionScope {
                allowed_methods: vec!["GET".to_string()],
                allowed_path_prefixes: vec!["/api/v1/datasets".to_string()],
                allowed_subject_ids: vec![],
                allowed_org_ids: vec![Uuid::nil()],
                workspace: Some("shared".to_string()),
                classification_clearance: Some("public".to_string()),
                allowed_markings: vec!["public".to_string()],
                restricted_view_ids: vec![Uuid::nil()],
                consumer_mode: true,
                guest_email: Some("guest@example.com".to_string()),
                guest_display_name: Some("Guest".to_string()),
            }),
        }
    }

    #[test]
    fn zero_trust_scope_blocks_disallowed_requests() {
        let claims = scoped_claims();
        assert!(enforce_zero_trust_scope(&claims, &Method::GET, "/api/v1/datasets").is_ok());
        assert!(enforce_zero_trust_scope(&claims, &Method::POST, "/api/v1/datasets").is_err());
        assert!(enforce_zero_trust_scope(&claims, &Method::GET, "/api/v1/pipelines").is_err());
    }

    #[test]
    fn dataset_paths_rewrite_to_backend_v1_compatibility_surface() {
        let id = Uuid::nil();
        assert_eq!(rewrite_upstream_path("/api/v1/datasets"), "/v1/datasets");
        assert_eq!(
            rewrite_upstream_path(&format!("/api/v1/datasets/{id}/preview")),
            format!("/v1/datasets/{id}/preview")
        );
        assert_eq!(
            rewrite_upstream_path(&format!("/api/v1/datasets/{id}/schema")),
            format!("/v1/datasets/{id}/schema")
        );
        assert_eq!(
            rewrite_upstream_path(&format!("/api/v1/datasets/{id}/filesystem")),
            format!("/v1/datasets/{id}/files")
        );
        assert_eq!(
            rewrite_upstream_path(&format!("/api/v1/datasets/{id}/files")),
            format!("/v1/datasets/{id}/files")
        );
        assert_eq!(
            rewrite_upstream_path("/api/v1/datasets/catalog/facets"),
            "/v1/catalog/facets"
        );
        assert_eq!(
            rewrite_upstream_path("/api/v1/pipelines"),
            "/api/v1/pipelines"
        );
    }

    fn gateway_config(catalog_url: &str, versioning_url: &str) -> GatewayConfig {
        gateway_config_with_iceberg(catalog_url, versioning_url, catalog_url)
    }

    fn gateway_config_with_iceberg(
        catalog_url: &str,
        versioning_url: &str,
        iceberg_url: &str,
    ) -> GatewayConfig {
        let source = format!(
            r#"
                jwt_secret = "test-secret"
                data_asset_catalog_service_url = "{catalog_url}"
                dataset_versioning_service_url = "{versioning_url}"
                dataset_quality_service_url = "{catalog_url}"
                iceberg_catalog_service_url = "{iceberg_url}"
            "#
        );
        ::config::Config::builder()
            .add_source(::config::File::from_str(
                &source,
                ::config::FileFormat::Toml,
            ))
            .build()
            .expect("gateway test config")
            .try_deserialize::<GatewayConfig>()
            .expect("gateway config deserialize")
    }

    fn test_router(config: GatewayConfig) -> Router {
        Router::new()
            .route("/api/v1/{*rest}", any(proxy_handler))
            // ADR-0041 — gateway exposes the catalog spec endpoints
            // and the API-token issuer alongside `/api/v1/*`.
            .route("/iceberg/v1/{*rest}", any(proxy_handler))
            .route("/v1/iceberg-clients/{*rest}", any(proxy_handler))
            .with_state((config, Client::new(), JwtConfig::new("test-secret")))
    }

    async fn request(router: &Router, method: Method, uri: String) -> StatusCode {
        let req = Request::builder()
            .method(method)
            .uri(uri)
            .header("content-type", "application/json")
            .body(Body::from("{}"))
            .expect("request");
        router.clone().oneshot(req).await.expect("proxy").status()
    }

    async fn expect_mock(mock: &MockServer, method_name: &str, upstream_path: String) {
        Mock::given(method(method_name))
            .and(path(upstream_path))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({ "ok": true })))
            .mount(mock)
            .await;
    }

    #[tokio::test]
    async fn datasets_ui_routes_proxy_to_catalog_v1_endpoints() {
        let catalog = MockServer::start().await;
        let versioning = MockServer::start().await;
        let router = test_router(gateway_config(&catalog.uri(), &versioning.uri()));
        let id = Uuid::nil();

        expect_mock(&catalog, "GET", "/v1/datasets".to_string()).await;
        expect_mock(&catalog, "POST", "/v1/datasets".to_string()).await;
        expect_mock(&catalog, "GET", format!("/v1/datasets/{id}")).await;
        expect_mock(&catalog, "PATCH", format!("/v1/datasets/{id}")).await;
        expect_mock(&catalog, "DELETE", format!("/v1/datasets/{id}")).await;
        expect_mock(&catalog, "GET", format!("/v1/datasets/{id}/preview")).await;
        expect_mock(&catalog, "GET", format!("/v1/datasets/{id}/schema")).await;
        expect_mock(&catalog, "POST", format!("/v1/datasets/{id}/upload")).await;
        // P3 — `/files` moved to dataset-versioning-service. The
        // legacy `/filesystem` listing stays on the catalog and is
        // covered by `dataset_filesystem_alias_and_versioning_routes_keep_compatibility`.
        expect_mock(&catalog, "GET", "/v1/catalog/facets".to_string()).await;

        let checks = vec![
            (Method::GET, "/api/v1/datasets?page=1".to_string()),
            (Method::POST, "/api/v1/datasets".to_string()),
            (Method::GET, format!("/api/v1/datasets/{id}")),
            (Method::PATCH, format!("/api/v1/datasets/{id}")),
            (Method::DELETE, format!("/api/v1/datasets/{id}")),
            (
                Method::GET,
                format!("/api/v1/datasets/{id}/preview?limit=25"),
            ),
            (Method::GET, format!("/api/v1/datasets/{id}/schema")),
            (Method::POST, format!("/api/v1/datasets/{id}/upload")),
            (Method::GET, "/api/v1/datasets/catalog/facets".to_string()),
        ];

        for (method, uri) in checks {
            assert_eq!(
                request(&router, method, uri.clone()).await,
                StatusCode::OK,
                "{uri} should proxy successfully"
            );
        }
    }

    #[tokio::test]
    async fn iceberg_admin_and_spec_paths_proxy_to_iceberg_catalog_service() {
        // ADR-0041 — both the Foundry admin surface
        // (`/api/v1/iceberg-tables/*`) consumed by `apps/web` and the
        // Iceberg REST Catalog spec endpoints (`/iceberg/v1/*`)
        // consumed by external clients land on
        // `iceberg-catalog-service`. The spec endpoints are NOT under
        // `/api/v1` so they need a dedicated proxy mount.
        let other = MockServer::start().await;
        let iceberg = MockServer::start().await;
        let router = test_router(gateway_config_with_iceberg(
            &other.uri(),
            &other.uri(),
            &iceberg.uri(),
        ));
        let id = Uuid::nil();

        // Admin surface (UI). Path is forwarded verbatim to the
        // catalog because the proxy does not rewrite this prefix.
        expect_mock(&iceberg, "GET", "/api/v1/iceberg-tables".to_string()).await;
        expect_mock(&iceberg, "GET", format!("/api/v1/iceberg-tables/{id}")).await;
        expect_mock(
            &iceberg,
            "GET",
            format!("/api/v1/iceberg-tables/{id}/snapshots"),
        )
        .await;
        // Spec surface (PyIceberg / Spark / Trino).
        expect_mock(&iceberg, "GET", "/iceberg/v1/config".to_string()).await;
        expect_mock(
            &iceberg,
            "POST",
            "/iceberg/v1/transactions/commit".to_string(),
        )
        .await;
        // API-token issuer.
        expect_mock(
            &iceberg,
            "POST",
            "/v1/iceberg-clients/api-tokens".to_string(),
        )
        .await;

        let checks = vec![
            (Method::GET, "/api/v1/iceberg-tables".to_string()),
            (Method::GET, format!("/api/v1/iceberg-tables/{id}")),
            (
                Method::GET,
                format!("/api/v1/iceberg-tables/{id}/snapshots"),
            ),
            (Method::GET, "/iceberg/v1/config".to_string()),
            (Method::POST, "/iceberg/v1/transactions/commit".to_string()),
            (Method::POST, "/v1/iceberg-clients/api-tokens".to_string()),
        ];

        for (method, uri) in checks {
            assert_eq!(
                request(&router, method, uri.clone()).await,
                StatusCode::OK,
                "{uri} should proxy to iceberg-catalog-service"
            );
        }
    }

    #[tokio::test]
    async fn dataset_filesystem_alias_and_versioning_routes_keep_compatibility() {
        let catalog = MockServer::start().await;
        let versioning = MockServer::start().await;
        let router = test_router(gateway_config(&catalog.uri(), &versioning.uri()));
        let id = Uuid::nil();

        // Catalog still serves the legacy `/files` listing reached
        // through the `/filesystem` rewrite. The P3 top-level `/files`
        // endpoint lives in the versioning service and is mocked below.
        Mock::given(method("GET"))
            .and(path(format!("/v1/datasets/{id}/files")))
            .and(query_param("path", "current"))
            .respond_with(ResponseTemplate::new(200).set_body_json(json!({ "entries": [] })))
            .mount(&catalog)
            .await;
        expect_mock(&versioning, "GET", format!("/v1/datasets/{id}/versions")).await;
        expect_mock(
            &versioning,
            "GET",
            format!("/v1/datasets/{id}/transactions"),
        )
        .await;
        // P3 — top-level files listing on the versioning service.
        expect_mock(&versioning, "GET", format!("/v1/datasets/{id}/files")).await;

        assert_eq!(
            request(
                &router,
                Method::GET,
                format!("/api/v1/datasets/{id}/filesystem?path=current"),
            )
            .await,
            StatusCode::OK
        );
        assert_eq!(
            request(
                &router,
                Method::GET,
                format!("/api/v1/datasets/{id}/versions"),
            )
            .await,
            StatusCode::OK
        );
        assert_eq!(
            request(
                &router,
                Method::GET,
                format!("/api/v1/datasets/{id}/transactions"),
            )
            .await,
            StatusCode::OK
        );
        assert_eq!(
            request(
                &router,
                Method::GET,
                format!("/api/v1/datasets/{id}/files?branch=master"),
            )
            .await,
            StatusCode::OK
        );
    }
}
