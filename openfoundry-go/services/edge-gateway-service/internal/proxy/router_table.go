package proxy

import (
	"strings"

	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/config"
)

// SelectUpstream picks the upstream URL for a given request path.
//
// The decision tree mirrors Rust `proxy::service_router::proxy_handler`
// rule for rule. Order matters — earlier branches win, since some
// rules rely on a more specific suffix match before a broad prefix
// catches the path. Comments tag each rule with the Rust block it
// corresponds to so future edits can be cross-referenced.
//
// Returns "" when no rule matches; the caller should respond 404 with
// the `unknown_service_route` error code.
func SelectUpstream(path string, u config.UpstreamURLs) string {
	switch {
	// ── identity & auth (S8/B16: SSO + api-keys + oauth-clients merged in) ──
	case strings.HasPrefix(path, "/api/v1/auth/sso"),
		strings.HasPrefix(path, "/api/v1/api-keys"),
		strings.HasPrefix(path, "/api/v1/applications"),
		strings.HasPrefix(path, "/api/v1/oauth/clients"),
		strings.HasPrefix(path, "/api/v1/external-integrations"):
		return u.IdentityFederation

	case strings.HasPrefix(path, "/api/v1/auth/register"),
		strings.HasPrefix(path, "/api/v1/auth/login"),
		strings.HasPrefix(path, "/api/v1/auth/refresh"),
		strings.HasPrefix(path, "/api/v1/auth/mfa"),
		strings.HasPrefix(path, "/api/v1/auth/sessions"),
		path == "/api/v1/users/me",
		path == "/api/v2/admin/users/me":
		return u.IdentityFederation

	case strings.HasPrefix(path, "/api/v1/auth/cipher"):
		return u.Cipher

	case strings.HasPrefix(path, "/api/v1/control-panel"),
		strings.HasPrefix(path, "/api/v2/admin/control-panel"):
		return u.IdentityFederation

	case strings.HasPrefix(path, "/api/v1/users/") &&
		(strings.Contains(path, "/roles") || strings.Contains(path, "/groups")):
		return u.AuthorizationPolicy

	case strings.HasPrefix(path, "/api/v1/roles"),
		strings.HasPrefix(path, "/api/v1/permissions"),
		strings.HasPrefix(path, "/api/v1/groups"),
		strings.HasPrefix(path, "/api/v1/policies"),
		strings.HasPrefix(path, "/api/v1/restricted-views"),
		strings.HasPrefix(path, "/api/v2/admin/roles"),
		strings.HasPrefix(path, "/api/v2/admin/permissions"),
		strings.HasPrefix(path, "/api/v2/admin/groups"),
		strings.HasPrefix(path, "/api/v2/admin/policies"),
		strings.HasPrefix(path, "/api/v2/admin/restricted-views"):
		return u.AuthorizationPolicy

	case strings.HasPrefix(path, "/api/v1/security-governance"),
		path == "/api/v1/audit/classifications",
		strings.HasPrefix(path, "/api/v1/audit/governance/"),
		path == "/api/v1/audit/compliance/posture":
		return u.SecurityGovernance

	// ── network boundaries (precedes connector-management) ──
	case strings.HasPrefix(path, "/api/v1/network-boundaries"),
		strings.HasPrefix(path, "/api/v1/network-boundary"),
		strings.HasPrefix(path, "/api/v1/data-connection/egress-policies"):
		return u.NetworkBoundary

	case strings.HasPrefix(path, "/api/v1/data-connection"):
		return u.ConnectorManagement

	case strings.HasPrefix(path, "/api/v1/checkpoints-purpose"):
		return u.CheckpointsPurpose

	// ── retention + lineage deletion (S8/B15: merged into audit-compliance) ──
	case strings.HasPrefix(path, "/api/v1/retention"),
		strings.HasSuffix(path, "/retention"),
		(strings.Contains(path, "/transactions/") && strings.HasSuffix(path, "/retention")),
		strings.HasSuffix(path, "/applicable-policies"),
		strings.HasSuffix(path, "/retention-preview"):
		return u.AuditCompliance

	case strings.HasPrefix(path, "/api/v1/lineage-deletions"),
		path == "/api/v1/audit/gdpr/erase":
		return u.AuditCompliance

	case path == "/api/v1/audit/overview",
		path == "/api/v1/audit/events",
		strings.HasPrefix(path, "/api/v1/audit/events/"),
		path == "/api/v1/audit/collectors",
		path == "/api/v1/audit/anomalies",
		path == "/api/v1/audit/reports",
		path == "/api/v1/audit/reports/generate",
		path == "/api/v1/audit/policies",
		strings.HasPrefix(path, "/api/v1/audit/policies/"),
		path == "/api/v1/audit/gdpr/export":
		return u.AuditCompliance

	// ── tenancy ──
	case strings.HasPrefix(path, "/api/v1/tenancy"),
		strings.HasPrefix(path, "/api/v1/organizations"),
		strings.HasPrefix(path, "/api/v1/enrollments"),
		strings.HasPrefix(path, "/api/v1/spaces"),
		strings.HasPrefix(path, "/api/v1/projects"),
		strings.HasPrefix(path, "/api/v1/nexus/spaces"),
		strings.HasPrefix(path, "/api/v1/ontology/projects"):
		return u.TenancyOrganizations

	// ── catch-all auth/users (after the more specific rules above) ──
	case strings.HasPrefix(path, "/api/v1/auth"),
		strings.HasPrefix(path, "/api/v1/users"),
		strings.HasPrefix(path, "/api/v2/admin"):
		return u.IdentityFederation

	// ── connectors ──
	case strings.HasPrefix(path, "/api/v1/connector-agents"),
		(strings.HasPrefix(path, "/api/v1/connections/") &&
			(strings.HasSuffix(path, "/sync") || strings.HasSuffix(path, "/sync-jobs"))):
		return u.IngestionReplication

	case strings.HasPrefix(path, "/api/v1/connections/") &&
		(strings.HasSuffix(path, "/discover") ||
			strings.Contains(path, "/registrations") ||
			strings.HasSuffix(path, "/virtual-tables/query")):
		return u.ConnectorManagement

	case strings.HasPrefix(path, "/api/v1/connections/") && strings.Contains(path, "/hyperauto/"):
		return u.ConnectorManagement

	case strings.HasPrefix(path, "/api/v1/connectors/catalog"),
		strings.HasPrefix(path, "/api/v1/connections"):
		return u.ConnectorManagement

	// ── datasets quality (S8: merged into dataset-versioning) ──
	case strings.HasPrefix(path, "/api/v1/datasets/") &&
		(strings.HasSuffix(path, "/quality") ||
			strings.Contains(path, "/quality/") ||
			strings.HasSuffix(path, "/lint")):
		return u.DatasetVersioning

	// ── iceberg catalog (admin + spec endpoints) ──
	case strings.HasPrefix(path, "/api/v1/iceberg-tables"),
		strings.HasPrefix(path, "/iceberg/v1/"),
		path == "/iceberg/v1",
		strings.HasPrefix(path, "/v1/iceberg-clients"):
		return u.IcebergCatalog

	// ── dataset versioning (versions, transactions, branches, views, files) ──
	case strings.HasPrefix(path, "/api/v1/datasets/") && datasetVersioningPath(path):
		return u.DatasetVersioning

	// ── catalog/legacy filesystem (S8: merged into dataset-versioning) ──
	case strings.HasPrefix(path, "/api/v1/datasets"),
		strings.HasPrefix(path, "/api/v2/filesystem"):
		return u.DatasetVersioning

	case strings.HasPrefix(path, "/api/v1/queries"):
		return u.Query

	// ── pipelines (S8: schedule + authoring → pipeline-build) ──
	case strings.HasPrefix(path, "/api/v1/pipelines/triggers/cron/"):
		return u.PipelineBuild
	case strings.HasPrefix(path, "/api/v1/pipelines/") &&
		(strings.HasSuffix(path, "/run") || strings.Contains(path, "/runs/") || strings.HasSuffix(path, "/runs")):
		return u.PipelineBuild
	case strings.HasPrefix(path, "/api/v1/pipelines"):
		return u.PipelineBuild

	case strings.HasPrefix(path, "/api/v1/lineage"):
		return u.Lineage

	// ── ontology actions/funnel/functions/rules (S8.1) ──
	case strings.HasPrefix(path, "/api/v1/ontology/functions"),
		strings.HasPrefix(path, "/api/v1/ontology/funnel"),
		strings.HasPrefix(path, "/api/v1/ontology/storage/insights"),
		strings.HasPrefix(path, "/api/v1/ontology/actions"),
		strings.HasPrefix(path, "/api/v1/ontology/rules"),
		(strings.HasPrefix(path, "/api/v1/ontology/types/") &&
			strings.Contains(path, "/objects/") &&
			strings.Contains(path, "/inline-edit/")),
		(strings.HasPrefix(path, "/api/v1/ontology/types/") && strings.HasSuffix(path, "/rules")),
		(strings.HasPrefix(path, "/api/v1/ontology/objects/") && strings.HasSuffix(path, "/rule-runs")):
		return u.OntologyActions

	case strings.HasPrefix(path, "/api/v1/ontology/search"),
		strings.HasPrefix(path, "/api/v1/ontology/graph"),
		strings.HasPrefix(path, "/api/v1/ontology/quiver"),
		strings.HasPrefix(path, "/api/v1/ontology/object-sets"),
		(strings.HasPrefix(path, "/api/v1/ontology/types/") &&
			(strings.HasSuffix(path, "/objects/query") || strings.HasSuffix(path, "/objects/knn"))):
		return u.OntologyQuery

	case strings.HasPrefix(path, "/api/v1/ontology/links/") && strings.Contains(path, "/instances"):
		return u.ObjectDatabase
	case strings.HasPrefix(path, "/api/v1/ontology/types/") && strings.Contains(path, "/objects"):
		return u.ObjectDatabase

	case strings.HasPrefix(path, "/api/v1/ontology/interfaces"),
		strings.HasPrefix(path, "/api/v1/ontology/shared-property-types"),
		strings.HasPrefix(path, "/api/v1/ontology/links"),
		strings.HasPrefix(path, "/api/v1/ontology/types"):
		return u.OntologyDefinition
	case strings.HasPrefix(path, "/api/v1/ontology"):
		return u.OntologyDefinition

	case strings.HasPrefix(path, "/api/v1/workflows/approvals"),
		strings.HasPrefix(path, "/api/v1/approvals"):
		return u.Approvals
	case strings.HasPrefix(path, "/api/v1/workflows"):
		return u.Workflow

	case strings.HasPrefix(path, "/api/v1/notebooks"),
		strings.HasPrefix(path, "/api/v1/notepad"):
		return u.Notebook

	case strings.HasPrefix(path, "/api/v1/notifications"):
		return u.Notification

	// ── ML ──
	case strings.HasPrefix(path, "/api/v1/ml/experiments"),
		strings.HasPrefix(path, "/api/v1/ml/runs"):
		return u.ModelCatalog
	case strings.HasPrefix(path, "/api/v1/ml/models"),
		strings.HasPrefix(path, "/api/v1/ml/model-versions"):
		return u.ModelCatalog
	case strings.HasPrefix(path, "/api/v1/ml/deployments/") && strings.HasSuffix(path, "/drift"):
		return u.ModelDeployment
	case strings.HasPrefix(path, "/api/v1/ml/deployments/") && strings.HasSuffix(path, "/predict"):
		return u.ModelDeployment
	case strings.HasPrefix(path, "/api/v1/ml/batch-predictions"):
		return u.ModelDeployment
	case strings.HasPrefix(path, "/api/v1/ml/deployments"):
		return u.ModelDeployment
	case strings.HasPrefix(path, "/api/v1/ml"):
		return u.ML

	// ── AI ──
	case strings.HasPrefix(path, "/api/v1/ai/guardrails/evaluate"),
		strings.HasPrefix(path, "/api/v1/ai/evaluations"):
		return u.AIEvaluation
	case strings.HasPrefix(path, "/api/v1/ai/providers"):
		return u.LLMCatalog
	case strings.HasPrefix(path, "/api/v1/ai/prompts"):
		return u.PromptWorkflow
	case strings.HasPrefix(path, "/api/v1/ai/knowledge-bases/") && strings.HasSuffix(path, "/search"):
		return u.RetrievalContext
	case strings.HasPrefix(path, "/api/v1/ai/knowledge-bases"):
		return u.KnowledgeIndex
	case strings.HasPrefix(path, "/api/v1/ai/conversations"):
		return u.ConversationState
	case strings.HasPrefix(path, "/api/v1/ai/tools"):
		return u.AI
	case strings.HasPrefix(path, "/api/v1/ai"):
		return u.AI

	case strings.HasPrefix(path, "/api/v1/entity-resolution"),
		strings.HasPrefix(path, "/api/v1/fusion"):
		return u.EntityResolution

	case strings.HasPrefix(path, "/api/v1/streaming"):
		return u.Streaming
	case strings.HasPrefix(path, "/api/v1/reports"):
		return u.Report
	case strings.HasPrefix(path, "/api/v1/geospatial"):
		return u.GeospatialIntelligence

	case strings.HasPrefix(path, "/api/v1/code-repos/repositories/") &&
		strings.HasSuffix(path, "/branches"):
		return u.GlobalBranch
	case strings.HasPrefix(path, "/api/v1/code-repos"):
		return u.CodeRepo

	case strings.HasPrefix(path, "/api/v1/federation-product-exchange"),
		strings.HasPrefix(path, "/api/v1/nexus"),
		path == "/api/v1/marketplace/installs",
		path == "/api/v1/marketplace/devops/branches":
		return u.FederationProductExchange
	case strings.HasPrefix(path, "/api/v1/marketplace/devops"),
		strings.HasPrefix(path, "/api/v1/marketplace"):
		return u.FederationProductExchange

	case strings.HasPrefix(path, "/api/v1/audit/sds"):
		return u.AuditCompliance
	case strings.HasPrefix(path, "/api/v1/audit"):
		return u.AuditCompliance

	case strings.HasPrefix(path, "/api/v1/widgets"):
		return u.ApplicationComposition

	// ── apps (curation/composition split) ──
	case strings.HasPrefix(path, "/api/v1/apps/public/"),
		path == "/api/v1/apps/templates",
		path == "/api/v1/apps/from-template",
		(strings.HasPrefix(path, "/api/v1/apps/") && strings.HasSuffix(path, "/slate-package")),
		(strings.HasPrefix(path, "/api/v1/apps/") && strings.HasSuffix(path, "/versions")),
		(strings.HasPrefix(path, "/api/v1/apps/") && strings.HasSuffix(path, "/publish")):
		return u.ApplicationComposition
	case strings.HasPrefix(path, "/api/v1/apps"):
		return u.AppBuilder
	}
	return ""
}

// datasetVersioningPath captures the long suffix-or-contains rule the
// Rust gateway uses to keep dataset-versioning routes off the catalog
// fall-through. Pulled out for readability.
func datasetVersioningPath(path string) bool {
	switch {
	case strings.HasSuffix(path, "/versions"),
		strings.HasSuffix(path, "/transactions"),
		strings.HasSuffix(path, "/branches"),
		strings.Contains(path, "/branches/"),
		strings.HasSuffix(path, "/views/current"),
		strings.HasSuffix(path, "/views/at"),
		strings.HasSuffix(path, "/storage-details"):
		return true
	case strings.Contains(path, "/views/") &&
		(strings.HasSuffix(path, "/files") ||
			strings.HasSuffix(path, "/schema") ||
			strings.HasSuffix(path, "/data")):
		return true
	case strings.HasSuffix(path, "/files") && !strings.Contains(path, "/views/"):
		return true
	case strings.Contains(path, "/files/") && strings.HasSuffix(path, "/download"):
		return true
	case strings.Contains(path, "/transactions/") && strings.HasSuffix(path, "/files"):
		return true
	}
	return false
}
