// Package config wires gateway configuration from YAML + OF_* env overrides.
//
// Field set + default ports MUST stay aligned with the Rust gateway's
// config.rs so a single Helm values.yaml can drive both implementations
// during the strangler-fig cutover.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Config is the top-level gateway configuration.
type Config struct {
	Service struct {
		Name    string `koanf:"name"`
		Version string `koanf:"version"`
	} `koanf:"service"`

	Server struct {
		Host            string `koanf:"host"`
		Port            uint16 `koanf:"port"`
		ShutdownTimeout string `koanf:"shutdown_timeout"`
	} `koanf:"server"`

	JWT struct {
		Secret   string `koanf:"secret"`
		Issuer   string `koanf:"issuer"`
		Audience string `koanf:"audience"`
	} `koanf:"jwt"`

	Telemetry struct {
		OTLPEndpoint string `koanf:"otlp_endpoint"`
		LogFormat    string `koanf:"log_format"`
	} `koanf:"telemetry"`

	NATSURL string `koanf:"nats_url"` // optional — enables audit publishing

	RedisURL string `koanf:"redis_url"` // optional — enables distributed rate limiting

	CORSOrigins []string `koanf:"cors_origins"`

	RateLimit RateLimitConfig `koanf:"rate_limit"`

	// Upstream service URLs — every name + default matches the Rust
	// gateway's config.rs verbatim so Helm values are language-agnostic.
	Upstream UpstreamURLs `koanf:"upstream"`
}

// RateLimitConfig matches the Rust [`RateLimitConfig`] struct.
type RateLimitConfig struct {
	AnonymousRequestsPerMinute uint32 `koanf:"anonymous_requests_per_minute"`
	BurstSize                  uint32 `koanf:"burst_size"`
	BucketTTLSecs              uint32 `koanf:"bucket_ttl_secs"`
	RedisKeyPrefix             string `koanf:"redis_key_prefix"`
}

// DefaultRateLimit returns the Rust crate's defaults.
func DefaultRateLimit() RateLimitConfig {
	return RateLimitConfig{
		AnonymousRequestsPerMinute: 120,
		BurstSize:                  30,
		BucketTTLSecs:              300,
		RedisKeyPrefix:             "openfoundry:ratelimit",
	}
}

// UpstreamURLs holds the per-bounded-context upstream hosts. Names
// match the Rust struct fields (snake_case) so a single values.yaml
// drives both gateways.
type UpstreamURLs struct {
	IdentityFederation             string `koanf:"identity_federation_service_url"`
	OauthIntegration               string `koanf:"oauth_integration_service_url"`
	SessionGovernance              string `koanf:"session_governance_service_url"`
	AuthorizationPolicy            string `koanf:"authorization_policy_service_url"`
	SecurityGovernance             string `koanf:"security_governance_service_url"`
	TenancyOrganizations           string `koanf:"tenancy_organizations_service_url"`
	Cipher                         string `koanf:"cipher_service_url"`
	DataConnector                  string `koanf:"data_connector_service_url"`
	ConnectorManagement            string `koanf:"connector_management_service_url"`
	VirtualTable                   string `koanf:"virtual_table_service_url"`
	IngestionReplication           string `koanf:"ingestion_replication_service_url"`
	DatasetVersioning              string `koanf:"dataset_versioning_service_url"`
	DataAssetCatalog               string `koanf:"data_asset_catalog_service_url"`
	DatasetQuality                 string `koanf:"dataset_quality_service_url"`
	IcebergCatalog                 string `koanf:"iceberg_catalog_service_url"`
	Query                          string `koanf:"query_service_url"`
	PipelineAuthoring              string `koanf:"pipeline_authoring_service_url"`
	PipelineBuild                  string `koanf:"pipeline_build_service_url"`
	PipelineSchedule               string `koanf:"pipeline_schedule_service_url"`
	Lineage                        string `koanf:"lineage_service_url"`
	OntologyDefinition             string `koanf:"ontology_definition_service_url"`
	ObjectDatabase                 string `koanf:"object_database_service_url"`
	OntologyQuery                  string `koanf:"ontology_query_service_url"`
	OntologyActions                string `koanf:"ontology_actions_service_url"`
	Ontology                       string `koanf:"ontology_service_url"`
	Workflow                       string `koanf:"workflow_service_url"`
	Approvals                      string `koanf:"approvals_service_url"`
	Notebook                       string `koanf:"notebook_service_url"`
	Notification                   string `koanf:"notification_service_url"`
	AppBuilder                     string `koanf:"app_builder_service_url"`
	ApplicationCuration            string `koanf:"application_curation_service_url"`
	ApplicationComposition         string `koanf:"application_composition_service_url"`
	ML                             string `koanf:"ml_service_url"`
	ModelCatalog                   string `koanf:"model_catalog_service_url"`
	ModelDeployment                string `koanf:"model_deployment_service_url"`
	ModelEvaluation                string `koanf:"model_evaluation_service_url"`
	ModelServing                   string `koanf:"model_serving_service_url"`
	ModelInferenceHistory          string `koanf:"model_inference_history_service_url"`
	AI                             string `koanf:"ai_service_url"`
	LLMCatalog                     string `koanf:"llm_catalog_service_url"`
	PromptWorkflow                 string `koanf:"prompt_workflow_service_url"`
	KnowledgeIndex                 string `koanf:"knowledge_index_service_url"`
	RetrievalContext               string `koanf:"retrieval_context_service_url"`
	ConversationState              string `koanf:"conversation_state_service_url"`
	AIEvaluation                   string `koanf:"ai_evaluation_service_url"`
	DocumentReporting              string `koanf:"document_reporting_service_url"`
	EntityResolution               string `koanf:"entity_resolution_service_url"`
	Streaming                      string `koanf:"streaming_service_url"`
	Report                         string `koanf:"report_service_url"`
	GeospatialIntelligence         string `koanf:"geospatial_intelligence_service_url"`
	CodeRepo                       string `koanf:"code_repo_service_url"`
	GlobalBranch                   string `koanf:"global_branch_service_url"`
	MarketplaceCatalog             string `koanf:"marketplace_catalog_service_url"`
	ProductDistribution            string `koanf:"product_distribution_service_url"`
	FederationProductExchange      string `koanf:"federation_product_exchange_service_url"`
	CheckpointsPurpose             string `koanf:"checkpoints_purpose_service_url"`
	NetworkBoundary                string `koanf:"network_boundary_service_url"`
	RetentionPolicy                string `koanf:"retention_policy_service_url"`
	LineageDeletion                string `koanf:"lineage_deletion_service_url"`
	AuditCompliance                string `koanf:"audit_compliance_service_url"`
	Audit                          string `koanf:"audit_service_url"`
	SDS                            string `koanf:"sds_service_url"`
	Nexus                          string `koanf:"nexus_service_url"`
}

// DefaultUpstreams returns the default localhost ports the Rust crate
// ships. Production deployments override every field via Helm values.
func DefaultUpstreams() UpstreamURLs {
	return UpstreamURLs{
		IdentityFederation:        "http://localhost:50112",
		OauthIntegration:          "http://localhost:50094",
		SessionGovernance:         "http://localhost:50074",
		AuthorizationPolicy:       "http://localhost:50093",
		SecurityGovernance:        "http://localhost:50114",
		TenancyOrganizations:      "http://localhost:50113",
		Cipher:                    "http://localhost:50073",
		DataConnector:             "http://localhost:50088",
		ConnectorManagement:       "http://localhost:50088",
		VirtualTable:              "http://localhost:50089",
		IngestionReplication:      "http://localhost:50090",
		DatasetVersioning:         "http://localhost:50078",
		DataAssetCatalog:          "http://localhost:50079",
		DatasetQuality:            "http://localhost:50072",
		IcebergCatalog:            "http://localhost:8197",
		Query:                     "http://localhost:50133",
		PipelineAuthoring:         "http://localhost:50080",
		PipelineBuild:             "http://localhost:50081",
		PipelineSchedule:          "http://localhost:50082",
		Lineage:                   "http://localhost:50083",
		OntologyDefinition:        "http://localhost:50103",
		ObjectDatabase:            "http://localhost:50104",
		OntologyQuery:             "http://localhost:50105",
		OntologyActions:           "http://localhost:50106",
		Ontology:                  "http://localhost:50103",
		Workflow:                  "http://localhost:50137",
		Approvals:                 "http://localhost:50071",
		Notebook:                  "http://localhost:50134",
		Notification:              "http://localhost:50114",
		AppBuilder:                "http://localhost:50063",
		ApplicationCuration:       "http://localhost:50101",
		ApplicationComposition:    "http://localhost:50140",
		ML:                        "http://localhost:50085",
		ModelCatalog:              "http://localhost:50085",
		ModelDeployment:           "http://localhost:50086",
		ModelEvaluation:           "http://localhost:50091",
		ModelServing:              "http://localhost:50087",
		ModelInferenceHistory:     "http://localhost:50092",
		AI:                        "http://localhost:50127",
		LLMCatalog:                "http://localhost:50095",
		PromptWorkflow:            "http://localhost:50096",
		KnowledgeIndex:            "http://localhost:50097",
		RetrievalContext:          "http://localhost:50098",
		ConversationState:         "http://localhost:50099",
		AIEvaluation:              "http://localhost:50075",
		DocumentReporting:         "http://localhost:50102",
		EntityResolution:          "http://localhost:50058",
		Streaming:                 "http://localhost:50121",
		Report:                    "http://localhost:50064",
		GeospatialIntelligence:    "http://localhost:50068",
		CodeRepo:                  "http://localhost:50065",
		GlobalBranch:              "http://localhost:50110",
		MarketplaceCatalog:        "http://localhost:50066",
		ProductDistribution:       "http://localhost:50111",
		FederationProductExchange: "http://localhost:50120",
		CheckpointsPurpose:        "http://localhost:50116",
		NetworkBoundary:           "http://localhost:50119",
		RetentionPolicy:           "http://localhost:50117",
		LineageDeletion:           "http://localhost:50118",
		AuditCompliance:           "http://localhost:50115",
		Audit:                     "http://localhost:50115",
		SDS:                       "http://localhost:50076",
		Nexus:                     "http://localhost:50067",
	}
}

// Defaults returns a Config pre-populated with the Rust gateway's
// defaults. Used when no config file is provided (dev shells, tests).
func Defaults() Config {
	c := Config{
		NATSURL:     "",
		CORSOrigins: nil,
		RateLimit:   DefaultRateLimit(),
		Upstream:    DefaultUpstreams(),
	}
	c.Service.Name = "edge-gateway-service"
	c.Service.Version = "0.1.0"
	c.Server.Host = "0.0.0.0"
	c.Server.Port = 8080
	c.Server.ShutdownTimeout = "15s"
	c.JWT.Issuer = ""
	c.JWT.Audience = ""
	c.Telemetry.OTLPEndpoint = "localhost:4317"
	c.Telemetry.LogFormat = "plain"
	return c
}

// Load resolves Config: defaults → defaultsPath (in-image) → envPath
// (from CONFIG_FILE) → OF_* env overrides (separator `__`).
func Load(defaultsPath, envPath string) (*Config, error) {
	cfg := Defaults()
	k := koanf.New(".")

	// Seed koanf with the Go-side defaults so YAML can override individual fields.
	if err := k.Load(structProvider{cfg}, nil); err != nil {
		return nil, fmt.Errorf("seed defaults: %w", err)
	}

	if defaultsPath != "" {
		if _, err := os.Stat(defaultsPath); err == nil {
			if err := k.Load(file.Provider(defaultsPath), yaml.Parser()); err != nil {
				return nil, fmt.Errorf("load defaults from %s: %w", defaultsPath, err)
			}
		}
	}
	if envPath != "" {
		if err := k.Load(file.Provider(envPath), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("load env config from %s: %w", envPath, err)
		}
	}
	envProvider := env.Provider("OF_", ".", func(s string) string {
		return strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(s, "OF_"), "__", "."))
	})
	if err := k.Load(envProvider, nil); err != nil {
		return nil, fmt.Errorf("load env overrides: %w", err)
	}

	out := Defaults()
	if err := k.UnmarshalWithConf("", &out, koanf.UnmarshalConf{Tag: "koanf"}); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &out, nil
}

// structProvider feeds a struct directly into koanf so defaults survive
// across the file/env merges. We don't reach for Confmap here because
// it walks `mapstructure` tags by default and we want our `koanf` tags.
type structProvider struct{ v Config }

func (s structProvider) ReadBytes() ([]byte, error) { return nil, nil }
func (s structProvider) Read() (map[string]any, error) {
	// koanf's struct provider expects mapstructure-style nesting; build it manually.
	out := map[string]any{
		"service": map[string]any{
			"name":    s.v.Service.Name,
			"version": s.v.Service.Version,
		},
		"server": map[string]any{
			"host":             s.v.Server.Host,
			"port":             s.v.Server.Port,
			"shutdown_timeout": s.v.Server.ShutdownTimeout,
		},
		"jwt": map[string]any{
			"secret":   s.v.JWT.Secret,
			"issuer":   s.v.JWT.Issuer,
			"audience": s.v.JWT.Audience,
		},
		"telemetry": map[string]any{
			"otlp_endpoint": s.v.Telemetry.OTLPEndpoint,
			"log_format":    s.v.Telemetry.LogFormat,
		},
		"nats_url":     s.v.NATSURL,
		"redis_url":    s.v.RedisURL,
		"cors_origins": s.v.CORSOrigins,
		"rate_limit": map[string]any{
			"anonymous_requests_per_minute": s.v.RateLimit.AnonymousRequestsPerMinute,
			"burst_size":                    s.v.RateLimit.BurstSize,
			"bucket_ttl_secs":               s.v.RateLimit.BucketTTLSecs,
			"redis_key_prefix":              s.v.RateLimit.RedisKeyPrefix,
		},
		"upstream": upstreamMap(s.v.Upstream),
	}
	return out, nil
}

func upstreamMap(u UpstreamURLs) map[string]any {
	return map[string]any{
		"identity_federation_service_url":         u.IdentityFederation,
		"oauth_integration_service_url":           u.OauthIntegration,
		"session_governance_service_url":          u.SessionGovernance,
		"authorization_policy_service_url":        u.AuthorizationPolicy,
		"security_governance_service_url":         u.SecurityGovernance,
		"tenancy_organizations_service_url":       u.TenancyOrganizations,
		"cipher_service_url":                      u.Cipher,
		"data_connector_service_url":              u.DataConnector,
		"connector_management_service_url":        u.ConnectorManagement,
		"virtual_table_service_url":               u.VirtualTable,
		"ingestion_replication_service_url":       u.IngestionReplication,
		"dataset_versioning_service_url":          u.DatasetVersioning,
		"data_asset_catalog_service_url":          u.DataAssetCatalog,
		"dataset_quality_service_url":             u.DatasetQuality,
		"iceberg_catalog_service_url":             u.IcebergCatalog,
		"query_service_url":                       u.Query,
		"pipeline_authoring_service_url":          u.PipelineAuthoring,
		"pipeline_build_service_url":              u.PipelineBuild,
		"pipeline_schedule_service_url":           u.PipelineSchedule,
		"lineage_service_url":                     u.Lineage,
		"ontology_definition_service_url":         u.OntologyDefinition,
		"object_database_service_url":             u.ObjectDatabase,
		"ontology_query_service_url":              u.OntologyQuery,
		"ontology_actions_service_url":            u.OntologyActions,
		"ontology_service_url":                    u.Ontology,
		"workflow_service_url":                    u.Workflow,
		"approvals_service_url":                   u.Approvals,
		"notebook_service_url":                    u.Notebook,
		"notification_service_url":                u.Notification,
		"app_builder_service_url":                 u.AppBuilder,
		"application_curation_service_url":        u.ApplicationCuration,
		"application_composition_service_url":     u.ApplicationComposition,
		"ml_service_url":                          u.ML,
		"model_catalog_service_url":               u.ModelCatalog,
		"model_deployment_service_url":            u.ModelDeployment,
		"model_evaluation_service_url":            u.ModelEvaluation,
		"model_serving_service_url":               u.ModelServing,
		"model_inference_history_service_url":     u.ModelInferenceHistory,
		"ai_service_url":                          u.AI,
		"llm_catalog_service_url":                 u.LLMCatalog,
		"prompt_workflow_service_url":             u.PromptWorkflow,
		"knowledge_index_service_url":             u.KnowledgeIndex,
		"retrieval_context_service_url":           u.RetrievalContext,
		"conversation_state_service_url":          u.ConversationState,
		"ai_evaluation_service_url":               u.AIEvaluation,
		"document_reporting_service_url":          u.DocumentReporting,
		"entity_resolution_service_url":           u.EntityResolution,
		"streaming_service_url":                   u.Streaming,
		"report_service_url":                      u.Report,
		"geospatial_intelligence_service_url":     u.GeospatialIntelligence,
		"code_repo_service_url":                   u.CodeRepo,
		"global_branch_service_url":               u.GlobalBranch,
		"marketplace_catalog_service_url":         u.MarketplaceCatalog,
		"product_distribution_service_url":        u.ProductDistribution,
		"federation_product_exchange_service_url": u.FederationProductExchange,
		"checkpoints_purpose_service_url":         u.CheckpointsPurpose,
		"network_boundary_service_url":            u.NetworkBoundary,
		"retention_policy_service_url":            u.RetentionPolicy,
		"lineage_deletion_service_url":            u.LineageDeletion,
		"audit_compliance_service_url":            u.AuditCompliance,
		"audit_service_url":                       u.Audit,
		"sds_service_url":                         u.SDS,
		"nexus_service_url":                       u.Nexus,
	}
}
