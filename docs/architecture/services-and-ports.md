# Services and Ports

All backend services expose a health endpoint and bind to fixed default ports in local development. The edge gateway listens on `8080` and proxies public traffic to these internal services.

> Current-source note: this page describes runtime service names and default
> ports. It is not a filesystem map. The HTTP gateway source lives at
> `services/edge-gateway-service`; there is no current `services/gateway`
> directory. For authoritative route ownership, read
> `services/edge-gateway-service/internal/proxy/router_table.go`.

## Service Map

The **Plano objetivo** column maps each service onto one of the five
target planes from [Runtime Topology](./runtime-topology.md): *storage*,
*ingestion*, *compute*, *control* or *state* (relational). A small number
of services are dual-anchored (e.g. write-path services that govern
*state* but emit on the *control* plane).

| Service | Default Port | Plano objetivo | Primary Role |
| --- | --- | --- | --- |
| `edge-gateway-service` | `8080` | control | Public HTTP edge, route selection, request IDs, rate limiting, tenant/auth headers, audit fan-out |
| `identity-federation-service` | `50112` | control | Login, refresh, MFA, SAML/OIDC/OAuth flows, service account tokens, scoped/guest sessions |
| `authorization-policy-service` | `50093` | control | Roles, permissions, groups, policies, restricted views, and merged security-governance/cipher/network-boundary surfaces |
| `tenancy-organizations-service` | `50113` | control | Tenant resolution, organizations, enrollments, spaces, projects, and sharing boundaries |
| `connector-management-service` | `50088` | ingestion | Connector catalog, source/connection definitions, credentials metadata, connection testing, and discovery orchestration |
| `ingestion-replication-service` | `50090` / `50122` | ingestion | Ingest-job materialization, replication control plane, and CDC metadata endpoints |
| `dataset-versioning-service` | `50078` | state | Dataset metadata, branches, transactions, versions, files, and Iceberg-backed snapshot state |
| `media-sets-service` | `50156` / `50157` | state | Media set metadata, media item references, and media storage APIs |
| `iceberg-catalog-service` | `8197` | storage | Iceberg REST catalog compatibility surface |
| `sql-bi-gateway-service` | `50133` / `50134` | compute | Flight SQL / BI edge plus HTTP `/healthz` and saved-query style surfaces |
| `pipeline-build-service` | `50081` | compute | Pipeline definitions, validation, preview/build execution, run history, and scheduled/cron trigger ownership after consolidation |
| `lineage-service` | `50083` | compute | Dataset and column lineage APIs |
| `ontology-definition-service` | `50103` | control | Ontology schema/control plane: object types, properties, interfaces, link types, action definitions, and project governance |
| `object-database-service` | `50104` | state | Object instances, link instances, revision history, and transactional outbox |
| `ontology-query-service` | `50105` | compute | Search, graph traversal, object-set queries, KNN, read models, and projections |
| `ontology-actions-service` | `50106` | control | Controlled mutations, action validation/execution, funnel/functions/rules, and policy-aware filters |
| `workflow-automation-service` | `50137` | control | Workflow orchestration and execution runtime |
| `notebook-runtime-service` | `50134` | compute | Notebook kernels, cells, sessions, notepad/reporting-style surfaces after consolidation |
| `application-composition-service` | `50140` | control | Application composition, templates, publishing, and related widget/app surfaces |
| `code-repository-review-service` | `50155` | state | Code repository review and developer-platform repository flows |
| `federation-product-exchange-service` | `50120` | control | Federation, marketplace, product exchange, and Nexus-style collaboration surfaces |
| `notification-alerting-service` | `50114` | control | Notification transport, inbox APIs, delivery channels, alerting, and websocket fanout |
| `audit-compliance-service` | `50115` | control | Audit collection, retention, lineage deletion, SDS, GDPR, and compliance posture surfaces |
| `model-catalog-service` | `50085` | compute | ML experiments, runs, models, and model versions |
| `model-deployment-service` | `50086` | compute | Model deployments, predictions, drift, and batch prediction APIs |
| `ai-evaluation-service` | `50075` | compute | AI guardrail and evaluation APIs |
| `llm-catalog-service` | `50095` | compute | AI provider catalog APIs |
| `retrieval-context-service` | `50098` | compute | Knowledge-base retrieval and RAG context APIs |
| `agent-runtime-service` | `50127` | compute | Agent/AI runtime, tool execution, prompt workflow compatibility, and conversation surfaces |
| `entity-resolution-service` | `50058` | compute | Entity resolution and fusion-style APIs |
| `ontology-exploratory-analysis-service` | `50131` | compute | Exploratory ontology analysis and geospatial-style APIs after consolidation |
| `telemetry-governance-service` | `50153` | control | Monitoring views, monitor rules, and telemetry governance |

### Edge SQL surfaces — explicit positioning

Two surfaces sit at the **edge of the compute plane** and are easy to
confuse; their roles are intentionally disjoint:

| Component                      | Plano objetivo            | Role                                                                                                                                                                                                                            |
| ------------------------------ | ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `sql-bi-gateway-service`       | compute (edge BI gateway) | **Edge BI gateway**. The single Apache Arrow Flight SQL surface for external BI clients (Tableau, Superset, JDBC/ODBC). Backed by DataFusion, applies auth/quotas/audit/saved-queries, and routes per-statement to local DataFusion (Iceberg), Trino (Iceberg analytics), Vespa (hybrid retrieval) or Postgres (OLTP reference) — see [ADR-0014](./adr/ADR-0014-retire-trino-flight-sql-only.md), [ADR-0029](./adr/ADR-0029-reintroduce-trino-for-iceberg-analytics.md) and [ADR-0030](./adr/ADR-0030-service-consolidation-30-targets.md). After S8 also owns the warehousing (`/api/v1/warehouse/*`) and tabular-analysis (`/api/v1/tabular/*`) HTTP CRUD absorbed from the retired `sql-warehousing-service` and `tabular-analysis-service`; the analytical-expressions surface lives in the `libs/analytical-logic` internal crate (no duplicated routes). |

## Gateway Route Ownership

The gateway maps URL prefixes to backend services. Important examples
from `services/edge-gateway-service/internal/proxy/router_table.go`:

- `/api/v1/auth`, `/api/v1/users` -> `identity-federation-service`
- `/api/v1/roles`, `/api/v1/permissions`, `/api/v1/groups`, `/api/v1/policies` -> `authorization-policy-service`
- `/api/v1/tenancy/resolve`, `/api/v1/organizations`, `/api/v1/enrollments` -> `tenancy-organizations-service`
- `/api/v1/connectors/catalog`, `/api/v1/connections` -> `connector-management-service`
- `/api/v1/connector-agents`, connection sync jobs -> `ingestion-replication-service`
- `/api/v1/datasets`, `/api/v2/filesystem` -> `dataset-versioning-service`
- `/api/v1/pipelines`, pipeline runs, and pipeline cron triggers -> `pipeline-build-service`
- `/api/v1/workflows`, approvals, and workflow execution routes -> `workflow-automation-service`
- `/api/v1/lineage` -> `lineage-service`
- `/api/v1/ontology/projects` -> `tenancy-organizations-service`
- `/api/v1/ontology/actions`, `/api/v1/ontology/funnel`, `/api/v1/ontology/storage/insights`, `/api/v1/ontology/functions`, `/api/v1/ontology/rules`, `/api/v1/ontology/types/{id}/objects/{id}/inline-edit`, `/api/v1/ontology/types/{id}/rules`, `/api/v1/ontology/objects/{id}/rule-runs` -> `ontology-actions-service` (S8.1: sole runtime owner after absorbing funnel/functions/security)
- `/api/v1/ontology/search`, `/api/v1/ontology/graph`, `/api/v1/ontology/quiver`, `/api/v1/ontology/object-sets`, `/api/v1/ontology/types/{id}/objects/query`, `/api/v1/ontology/types/{id}/objects/knn` -> `ontology-query-service`
- `/api/v1/ontology/links/{id}/instances`, `/api/v1/ontology/types/{id}/objects` -> `object-database-service`
- `/api/v1/ontology/interfaces`, `/api/v1/ontology/shared-property-types`, `/api/v1/ontology/links`, `/api/v1/ontology/types` -> `ontology-definition-service`
- `/api/v1/ml/experiments`, `/api/v1/ml/models` -> `model-catalog-service`
- `/api/v1/ml/deployments`, `/api/v1/ml/batch-predictions` -> `model-deployment-service`
- `/api/v1/ai/evaluations` -> `ai-evaluation-service`
- `/api/v1/ai/providers` -> `llm-catalog-service`
- `/api/v1/ai/knowledge-bases/*/search` -> `retrieval-context-service`
- `/api/v1/entity-resolution`, `/api/v1/fusion` -> `entity-resolution-service`
- `/api/v1/code-repos` -> `code-repository-review-service` / global branch routes per router table
- `/api/v1/marketplace`, `/api/v1/federation-product-exchange`, `/api/v1/nexus` -> `federation-product-exchange-service`
- `/api/v1/nexus/spaces` -> `tenancy-organizations-service`
- `/api/v1/notifications` -> `notification-alerting-service`
- `/api/v1/audit` -> `audit-compliance-service`

## Cross-Service Dependencies

Configuration files show explicit service-to-service defaults for several domains:

- `connector-management-service` knows about dataset, pipeline, and ontology services
- `ingestion-replication-service` knows about dataset, pipeline, and ontology services
- connector discovery and virtual-table style routes are consolidated into `connector-management-service`
- `pipeline-build-service` depends on dataset, workflow, AI, and storage services
- `lineage-service` depends on dataset, workflow, and AI services
- `workflow-automation-service` depends on notification, ontology, and pipeline services
- `ontology-definition-service` depends on audit, AI, and notification services
- `object-database-service` depends on audit and notification services; all writes go through `object-database-service`
- `ontology-query-service` depends on `object-database-service` (fallback point lookups), `ontology-actions-service` (policy filters, S8.1), and AI services
- `ontology-actions-service` depends on `object-database-service` (mutations) and `ontology-definition-service` (action / function package definitions); owns the actions, funnel, function-runtime and rule (policy / marking) HTTP surfaces and the `actions_log` Cassandra column family (S8.1)
- reporting/notepad-style routes are consolidated into `notebook-runtime-service`
- `notebook-runtime-service` depends on query and AI services
- marketplace/product-exchange routes are consolidated into `federation-product-exchange-service`
- app-builder/application-curation/developer-console style routes are consolidated into `application-composition-service`

## Health Convention

Every current Go service exposes a `/healthz` route. Some services also keep
`/health` as a compatibility alias. This shared convention is used by:

- local runtime scripts
- GitHub Actions smoke jobs
- Helm health probes and operational checks

The `sql-bi-gateway-service` is gRPC-only on its primary Flight SQL port
(`50133`) and therefore exposes its HTTP `/healthz` probe (also aliased as
`/health`) plus the saved-queries / warehousing / tabular-analysis HTTP
CRUD on a companion port (`healthz_port`, default `50134`). The retired
`sql-warehousing-service` previously played the same gRPC-only role on
ports `50123`/`50124`; that surface is now folded into the gateway.
