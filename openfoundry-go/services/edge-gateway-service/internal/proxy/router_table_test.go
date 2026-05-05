package proxy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/proxy"
)

// upstreamFor wires every URL slot to a unique probe so the test asserts
// which upstream the rule selected via simple string equality.
func upstreamFor(t *testing.T) config.UpstreamURLs {
	t.Helper()
	u := config.UpstreamURLs{}
	// Reflective string pointers would be tidier but invasive; the
	// fields below cover every code path the table-driven test exercises.
	u.IdentityFederation = "id-fed"
	u.Cipher = "cipher"
	u.AuthorizationPolicy = "authz"
	u.SecurityGovernance = "sec-gov"
	u.NetworkBoundary = "net-bound"
	u.ConnectorManagement = "conn-mgmt"
	u.IngestionReplication = "ingest"
	u.CheckpointsPurpose = "checkpoints"
	u.AuditCompliance = "audit"
	u.TenancyOrganizations = "tenancy"
	u.IcebergCatalog = "iceberg"
	u.DatasetVersioning = "dvs"
	u.Query = "query"
	u.PipelineBuild = "pipeline"
	u.Lineage = "lineage"
	u.OntologyActions = "ontology-actions"
	u.OntologyQuery = "ontology-query"
	u.ObjectDatabase = "object-db"
	u.OntologyDefinition = "ontology-def"
	u.Approvals = "approvals"
	u.Workflow = "workflow"
	u.Notebook = "notebook"
	u.Notification = "notif"
	u.ModelCatalog = "model-catalog"
	u.ModelDeployment = "model-deploy"
	u.ML = "ml"
	u.AIEvaluation = "ai-eval"
	u.LLMCatalog = "llm-catalog"
	u.PromptWorkflow = "prompts"
	u.RetrievalContext = "retrieval"
	u.KnowledgeIndex = "knowledge"
	u.ConversationState = "conv"
	u.AI = "ai"
	u.EntityResolution = "entity-res"
	u.Streaming = "streaming"
	u.Report = "report"
	u.GeospatialIntelligence = "geo"
	u.GlobalBranch = "branch"
	u.CodeRepo = "code-repo"
	u.FederationProductExchange = "marketplace"
	u.ApplicationComposition = "app-comp"
	u.AppBuilder = "app-builder"
	return u
}

func TestSelectUpstream(t *testing.T) {
	t.Parallel()
	u := upstreamFor(t)

	cases := []struct {
		path string
		want string
	}{
		// identity & auth
		{"/api/v1/auth/login", "id-fed"},
		{"/api/v1/auth/sso/google/callback", "id-fed"},
		{"/api/v1/api-keys", "id-fed"},
		{"/api/v1/auth/cipher", "cipher"},
		{"/api/v1/users/me", "id-fed"},
		{"/api/v1/users/abc/roles", "authz"},
		{"/api/v1/roles", "authz"},
		{"/api/v1/permissions/x", "authz"},
		// security/network
		{"/api/v1/security-governance/posture", "sec-gov"},
		{"/api/v1/network-boundaries", "net-bound"},
		{"/api/v1/data-connection/sources", "conn-mgmt"},
		{"/api/v1/data-connection/egress-policies", "net-bound"},
		// retention/audit
		{"/api/v1/retention/policies", "audit"},
		{"/api/v1/audit/gdpr/erase", "audit"},
		{"/api/v1/audit/events", "audit"},
		{"/api/v1/audit/sds/foo", "audit"},
		// tenancy
		{"/api/v1/organizations", "tenancy"},
		{"/api/v1/projects", "tenancy"},
		// iceberg
		{"/api/v1/iceberg-tables", "iceberg"},
		{"/iceberg/v1/namespaces", "iceberg"},
		{"/v1/iceberg-clients/api-tokens", "iceberg"},
		// datasets / catalog
		{"/api/v1/datasets/abc/quality", "dvs"},
		{"/api/v1/datasets/abc/transactions", "dvs"},
		{"/api/v1/datasets/abc/branches/main", "dvs"},
		{"/api/v1/datasets/abc/files", "dvs"},
		{"/api/v1/datasets/catalog/facets", "dvs"},
		{"/api/v1/datasets", "dvs"},
		// pipelines/queries/lineage
		{"/api/v1/queries/run", "query"},
		{"/api/v1/pipelines/123/run", "pipeline"},
		{"/api/v1/pipelines", "pipeline"},
		{"/api/v1/lineage/abc", "lineage"},
		// ontology
		{"/api/v1/ontology/actions/exec", "ontology-actions"},
		{"/api/v1/ontology/funnel/x", "ontology-actions"},
		{"/api/v1/ontology/types/X/objects/Y/inline-edit/foo", "ontology-actions"},
		{"/api/v1/ontology/search?q=x", "ontology-query"},
		{"/api/v1/ontology/types/X/objects/query", "ontology-query"},
		{"/api/v1/ontology/types/X/objects", "object-db"},
		{"/api/v1/ontology/links/X/instances/y", "object-db"},
		{"/api/v1/ontology/interfaces", "ontology-def"},
		{"/api/v1/ontology/types", "ontology-def"},
		{"/api/v1/ontology/anything-else", "ontology-def"},
		// workflows/notebooks
		{"/api/v1/workflows/approvals/x", "approvals"},
		{"/api/v1/approvals/x", "approvals"},
		{"/api/v1/workflows/instances", "workflow"},
		{"/api/v1/notebooks/abc", "notebook"},
		{"/api/v1/notepad/abc", "notebook"},
		// ML / AI
		{"/api/v1/ml/experiments", "model-catalog"},
		{"/api/v1/ml/deployments/abc/predict", "model-deploy"},
		{"/api/v1/ml/deployments/abc/drift", "model-deploy"},
		{"/api/v1/ml/widgets", "ml"},
		{"/api/v1/ai/evaluations/run", "ai-eval"},
		{"/api/v1/ai/providers", "llm-catalog"},
		{"/api/v1/ai/prompts", "prompts"},
		{"/api/v1/ai/knowledge-bases/abc/search", "retrieval"},
		{"/api/v1/ai/knowledge-bases", "knowledge"},
		{"/api/v1/ai/conversations", "conv"},
		{"/api/v1/ai/anything", "ai"},
		// streaming/reports/geo
		{"/api/v1/streaming/foo", "streaming"},
		{"/api/v1/reports", "report"},
		{"/api/v1/geospatial/x", "geo"},
		// code repo
		{"/api/v1/code-repos/repositories/abc/branches", "branch"},
		{"/api/v1/code-repos/repositories/abc/commits", "code-repo"},
		// marketplace
		{"/api/v1/marketplace/installs", "marketplace"},
		{"/api/v1/marketplace/devops/branches", "marketplace"},
		{"/api/v1/marketplace/anything", "marketplace"},
		// apps
		{"/api/v1/widgets/x", "app-comp"},
		{"/api/v1/apps/templates", "app-comp"},
		{"/api/v1/apps/abc/publish", "app-comp"},
		{"/api/v1/apps", "app-builder"},
		// no rule
		{"/healthz", ""},
		{"/api/v1/totally-unknown", ""},
	}
	for _, c := range cases {
		got := proxy.SelectUpstream(c.path, u)
		assert.Equal(t, c.want, got, "path=%s", c.path)
	}
}
