package models

import (
	"encoding/json"
	"sort"

	"github.com/google/uuid"
)

// GalleryCatalog mirrors handlers/catalog.rs GalleryCatalog for the UI-facing
// connector gallery endpoint.
type GalleryCatalog struct {
	Connectors []GalleryConnector `json:"connectors"`
}

// GalleryConnector is the compact UI shape derived from a connector contract.
type GalleryConnector struct {
	Type         string   `json:"type"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
	Workers      []string `json:"workers"`
	Available    bool     `json:"available"`
}

type ConnectionEffectiveCapabilities struct {
	ConnectionID                uuid.UUID      `json:"connection_id"`
	ConnectorType               string         `json:"connector_type"`
	Status                      string         `json:"status"`
	Modes                       []string       `json:"modes"`
	Workers                     []string       `json:"workers"`
	SupportsConnectionTesting   bool           `json:"supports_connection_testing"`
	SupportsDiscovery           bool           `json:"supports_discovery"`
	SupportsSchemaIntrospection bool           `json:"supports_schema_introspection"`
	SupportsIncremental         bool           `json:"supports_incremental"`
	SupportsCDC                 bool           `json:"supports_cdc"`
	SupportsZeroCopy            bool           `json:"supports_zero_copy"`
	SupportsPrivateNetworkAgent bool           `json:"supports_private_network_agent"`
	RequiresPrivateNetworkAgent bool           `json:"requires_private_network_agent"`
	PrivateNetworkEgressAllowed bool           `json:"private_network_egress_allowed"`
	AllowedEgressHosts          []string       `json:"allowed_egress_hosts"`
	ConfigKeys                  []string       `json:"config_keys"`
	ConfigInferred              ConfigInferred `json:"config_inferred"`
	PolicyWarnings              []string       `json:"policy_warnings"`
	FoundryCompute              FoundryCompute `json:"foundry_compute"`
}

type ConfigInferred struct {
	HasCredentials       bool `json:"has_credentials"`
	HasOAuthToken        bool `json:"has_oauth_token"`
	HasPrivateKey        bool `json:"has_private_key"`
	HasCDCSelector       bool `json:"has_cdc_selector"`
	HasIncrementalCursor bool `json:"has_incremental_cursor"`
	RequestsZeroCopy     bool `json:"requests_zero_copy"`
}

var availableConnectorTypes = map[string]struct{}{
	"postgresql": {}, "mysql": {}, "s3": {}, "azure_blob": {}, "adls": {}, "onelake": {},
	"gcs": {}, "google_cloud_storage": {}, "generic": {}, "snowflake": {}, "databricks": {},
	"bigquery": {}, "parquet": {}, "rest_api": {}, "csv": {}, "json": {}, "kafka": {},
	"kinesis": {}, "iot": {}, "sap": {}, "streaming_kafka": {}, "streaming_kinesis": {},
	"streaming_sqs": {}, "streaming_pubsub": {}, "streaming_aveva_pi": {}, "streaming_external": {},
}

// ConnectorProfiles mirrors Rust connector_profiles() and also includes every
// connector module present in src/connectors so the gallery/contract catalogue
// is not missing specialized connectors that are still scaffolded in Rust.
func ConnectorProfiles() []ConnectorContractProfile {
	return []ConnectorContractProfile{
		sqlProfile("postgresql", "PostgreSQL", false),
		sqlProfile("postgres", "PostgreSQL", false),
		sqlProfile("mysql", "MySQL", false),
		sqlProfile("mssql", "Microsoft SQL Server", false),
		sqlProfile("oracle", "Oracle Database", false),
		driverProfile("odbc", "ODBC"),
		driverProfile("jdbc", "JDBC"),
		warehouseProfile("snowflake", "Snowflake"),
		warehouseProfile("bigquery", "BigQuery"),
		warehouseProfile("databricks", "Databricks"),
		objectStoreProfile("s3", "Amazon S3"),
		objectStoreProfile("azure_blob", "Azure Blob"),
		objectStoreProfile("adls", "Azure Data Lake Storage"),
		objectStoreProfile("onelake", "Microsoft OneLake"),
		objectStoreProfile("gcs", "Google Cloud Storage"),
		objectStoreProfile("google_cloud_storage", "Google Cloud Storage"),
		objectStoreProfile("parquet", "Parquet file"),
		objectStoreProfile("csv", "CSV file"),
		objectStoreProfile("json", "JSON file"),
		objectStoreProfile("excel", "Excel file"),
		objectStoreProfile("sftp", "SFTP"),
		objectStoreProfile("generic", "Generic open-table catalog"),
		objectStoreProfile("open_table_catalog", "Open Table Catalog"),
		eventBusProfile("kafka", "Kafka"),
		eventBusProfile("kinesis", "Amazon Kinesis"),
		eventBusProfile("iot", "IoT / IIoT feed"),
		saasProfile("salesforce", "Salesforce"),
		saasProfile("rest_api", "REST API"),
		saasProfile("graphql", "GraphQL"),
		saasProfile("ldap", "LDAP"),
		biProfile("tableau", "Tableau"),
		biProfile("power_bi", "Power BI"),
		erpProfile("sap", "SAP"),
	}
}

func ConnectorProfile(connectorType string) (ConnectorContractProfile, bool) {
	for _, profile := range ConnectorProfiles() {
		if profile.ConnectorType == connectorType {
			return profile, true
		}
	}
	return ConnectorContractProfile{}, false
}

func BuildConnectorContractCatalog() ConnectorContractCatalog {
	return ConnectorContractCatalog{Connectors: ConnectorProfiles(), CertificationSummary: CertificationSummary()}
}

func CertificationSummary() ConnectorCertificationSummary {
	profiles := ConnectorProfiles()
	families := make([]string, 0, len(profiles))
	certified := 0
	advanced := 0
	for _, profile := range profiles {
		switch profile.Certification.Level {
		case "certified":
			certified++
		case "advanced":
			advanced++
		}
		families = append(families, profile.TemplateFamily)
	}
	sort.Strings(families)
	families = compactStrings(families)
	return ConnectorCertificationSummary{CertifiedConnectors: certified, AdvancedConnectors: advanced, ConnectorsNeedingHardening: len(profiles) - certified - advanced, TemplateFamilies: families}
}

func BuildGalleryCatalog() GalleryCatalog {
	profiles := ConnectorProfiles()
	connectors := make([]GalleryConnector, 0, len(profiles))
	for _, profile := range profiles {
		connectors = append(connectors, ToGalleryConnector(profile))
	}
	return GalleryCatalog{Connectors: connectors}
}

func ToGalleryConnector(profile ConnectorContractProfile) GalleryConnector {
	capabilities := make([]string, 0, len(profile.Sync.Modes)+2)
	for _, mode := range profile.Sync.Modes {
		switch mode {
		case "batch", "incremental":
			capabilities = append(capabilities, "batch_sync")
		case "streaming":
			capabilities = append(capabilities, "streaming_sync")
		case "zero_copy":
			capabilities = append(capabilities, "virtual_table")
		default:
			capabilities = append(capabilities, mode)
		}
	}
	if profile.Sync.SupportsCDC {
		capabilities = append(capabilities, "cdc_sync")
	}
	if profile.Testing.SupportsDiscovery {
		capabilities = append(capabilities, "exploration")
	}
	sort.Strings(capabilities)
	capabilities = compactStrings(capabilities)

	workers := []string{"foundry"}
	if profile.Auth.SupportsPrivateNetworkAgent {
		workers = append(workers, "agent")
	}
	description := profile.TemplateFamily
	if len(profile.Notes) > 0 {
		description = profile.Notes[0]
	}
	_, available := availableConnectorTypes[profile.ConnectorType]
	return GalleryConnector{Type: profile.ConnectorType, Name: profile.DisplayName, Description: description, Capabilities: capabilities, Workers: workers, Available: available}
}

func StreamingSourceContracts() []StreamingSourceContract {
	return []StreamingSourceContract{
		streamingSource("streaming_kafka", "Apache Kafka", "Pull records from a Kafka topic via consumer-group offsets.", false, []StreamingSyncFieldDescriptor{{Name: "bootstrap_servers", Kind: "string", Required: true, Description: "Comma-separated host:port list."}, {Name: "topic", Kind: "string", Required: true, Description: "Topic the sync subscribes to."}, {Name: "consumer_group", Kind: "string", Required: true, Description: "Kafka consumer group id."}, {Name: "auto_offset_reset", Kind: "string", Required: false, Description: "earliest / latest."}}),
		streamingSource("streaming_kinesis", "Amazon Kinesis", "Pull records from a Kinesis stream shard.", false, []StreamingSyncFieldDescriptor{{Name: "stream_name", Kind: "string", Required: true, Description: "Kinesis stream name."}, {Name: "region", Kind: "string", Required: true, Description: "AWS region."}, {Name: "shard_iterator_type", Kind: "string", Required: false, Description: "LATEST / TRIM_HORIZON."}, {Name: "max_records_per_shard", Kind: "int", Required: false, Description: "Soft cap per pull."}}),
		streamingSource("streaming_sqs", "Amazon SQS", "Long-poll an SQS queue with explicit per-message ack.", false, []StreamingSyncFieldDescriptor{{Name: "queue_url", Kind: "string", Required: true, Description: "Full queue URL."}, {Name: "region", Kind: "string", Required: true, Description: "AWS region."}, {Name: "wait_time_seconds", Kind: "int", Required: false, Description: "Long-poll seconds (0..=20)."}, {Name: "visibility_timeout_seconds", Kind: "int", Required: false, Description: "Per-message visibility timeout."}}),
		streamingSource("streaming_pubsub", "Google Cloud Pub/Sub", "REST-based pull + ack against a subscription.", false, []StreamingSyncFieldDescriptor{{Name: "project_id", Kind: "string", Required: true, Description: "GCP project id."}, {Name: "subscription_id", Kind: "string", Required: true, Description: "Subscription id."}, {Name: "max_messages", Kind: "int", Required: false, Description: "Soft cap per pull."}, {Name: "ack_deadline_seconds", Kind: "int", Required: false, Description: "Per-pull ack-deadline override."}}),
		streamingSource("streaming_aveva_pi", "Aveva PI", "Poll the PI Web API for observation deltas.", false, []StreamingSyncFieldDescriptor{{Name: "base_url", Kind: "string", Required: true, Description: "PI Web API base URL."}, {Name: "event_stream_web_id", Kind: "string", Required: true, Description: "WebID of the event stream."}, {Name: "poll_interval_ms", Kind: "int", Required: false, Description: "Polling cadence."}, {Name: "auth_header", Kind: "secret", Required: false, Description: "Authorization header (Bearer / Basic)."}}),
		streamingSource("streaming_external", "External transform (Magritte)", "Generic webhook hook for sources without a dedicated connector (ActiveMQ, Amazon SNS, IBM MQ, RabbitMQ, MQTT, Solace …).", true, []StreamingSyncFieldDescriptor{{Name: "agent_label", Kind: "string", Required: true, Description: "Free-form label for the catalogue."}, {Name: "agent_token", Kind: "secret", Required: true, Description: "Bearer token the agent uses to push records."}, {Name: "protocol", Kind: "string", Required: true, Description: "activemq | rabbitmq | mqtt | sns | ibm_mq | solace."}}),
	}
}

func streamingSource(kind, displayName, description string, requiresAgent bool, fields []StreamingSyncFieldDescriptor) StreamingSourceContract {
	return StreamingSourceContract{Kind: kind, DisplayName: displayName, Description: description, RequiresAgent: requiresAgent, ConfigFields: fields}
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

func sqlProfile(connectorType, displayName string, supportsCDC bool) ConnectorContractProfile {
	level := "advanced"
	runtimeDepth := "batch_incremental_zero_copy"
	if supportsCDC {
		level = "certified"
		runtimeDepth = "batch_incremental_cdc_zero_copy"
	}
	return ConnectorContractProfile{ConnectorType: connectorType, DisplayName: displayName, TemplateFamily: "sql_tabular", Auth: ConnectorAuthProfile{Strategy: "username_password", SecretFields: []string{"user", "password"}, SupportsOAuth: false, SupportsPrivateNetworkAgent: false}, Testing: ConnectorTestingProfile{SupportsConnectionTesting: true, SupportsDiscovery: true, SupportsSchemaIntrospection: true}, Sync: ConnectorSyncProfile{Modes: []string{"batch", "incremental", "zero_copy"}, SupportsIncremental: true, SupportsCDC: supportsCDC, SupportsZeroCopy: true}, Observability: ConnectorObservabilityProfile{Retries: true, StatusTracking: true, SourceSignatures: true}, Builder: ConnectorBuilderProfile{ScaffoldKind: "sql_connector", ReusableComponents: []string{"connection_testing", "schema_introspection", "virtual_tables", "bulk_registration"}, ExampleTargets: []string{"postgresql", "mysql"}}, Certification: ConnectorCertificationProfile{Level: level, RuntimeDepth: runtimeDepth, Auth: "certified", Observability: "advanced", SchemaEvolution: "advanced", PerformancePosture: "advanced", FailureHandling: "advanced"}, Notes: []string{"Shared SQL connector contract with validation, discovery and virtual tables."}}
}

func warehouseProfile(connectorType, displayName string) ConnectorContractProfile {
	return ConnectorContractProfile{ConnectorType: connectorType, DisplayName: displayName, TemplateFamily: "warehouse_zero_copy", Auth: ConnectorAuthProfile{Strategy: "service_account_or_key_pair", SecretFields: []string{"credentials", "private_key"}, SupportsOAuth: false, SupportsPrivateNetworkAgent: true}, Testing: ConnectorTestingProfile{SupportsConnectionTesting: true, SupportsDiscovery: true, SupportsSchemaIntrospection: true}, Sync: ConnectorSyncProfile{Modes: []string{"batch", "incremental", "zero_copy"}, SupportsIncremental: true, SupportsCDC: false, SupportsZeroCopy: true}, Observability: ConnectorObservabilityProfile{Retries: true, StatusTracking: true, SourceSignatures: true}, Builder: ConnectorBuilderProfile{ScaffoldKind: "warehouse_connector", ReusableComponents: []string{"connection_testing", "discovery", "schema_introspection", "zero_copy_query"}, ExampleTargets: []string{"snowflake", "bigquery"}}, Certification: ConnectorCertificationProfile{Level: "advanced", RuntimeDepth: "batch_incremental_zero_copy", Auth: "advanced", Observability: "advanced", SchemaEvolution: "advanced", PerformancePosture: "advanced", FailureHandling: "advanced"}, Notes: []string{"Optimized for discovery plus virtual-table query flows."}}
}

func objectStoreProfile(connectorType, displayName string) ConnectorContractProfile {
	return ConnectorContractProfile{ConnectorType: connectorType, DisplayName: displayName, TemplateFamily: "object_storage", Auth: ConnectorAuthProfile{Strategy: "access_key", SecretFields: []string{"access_key", "secret_key"}, SupportsOAuth: false, SupportsPrivateNetworkAgent: true}, Testing: ConnectorTestingProfile{SupportsConnectionTesting: true, SupportsDiscovery: true, SupportsSchemaIntrospection: true}, Sync: ConnectorSyncProfile{Modes: []string{"batch", "incremental"}, SupportsIncremental: true, SupportsCDC: false, SupportsZeroCopy: false}, Observability: ConnectorObservabilityProfile{Retries: true, StatusTracking: true, SourceSignatures: true}, Builder: ConnectorBuilderProfile{ScaffoldKind: "object_storage_connector", ReusableComponents: []string{"path_discovery", "schema_sampling", "artifact_sync"}, ExampleTargets: []string{"s3", "parquet"}}, Certification: ConnectorCertificationProfile{Level: "advanced", RuntimeDepth: "batch_incremental", Auth: "advanced", Observability: "advanced", SchemaEvolution: "baseline", PerformancePosture: "advanced", FailureHandling: "advanced"}, Notes: []string{"Shared object storage template for path discovery and artifact sync."}}
}

func eventBusProfile(connectorType, displayName string) ConnectorContractProfile {
	return ConnectorContractProfile{ConnectorType: connectorType, DisplayName: displayName, TemplateFamily: "event_bus", Auth: ConnectorAuthProfile{Strategy: "brokers_and_credentials", SecretFields: []string{"username", "password"}, SupportsOAuth: false, SupportsPrivateNetworkAgent: true}, Testing: ConnectorTestingProfile{SupportsConnectionTesting: true, SupportsDiscovery: true, SupportsSchemaIntrospection: true}, Sync: ConnectorSyncProfile{Modes: []string{"streaming", "incremental", "zero_copy"}, SupportsIncremental: true, SupportsCDC: true, SupportsZeroCopy: true}, Observability: ConnectorObservabilityProfile{Retries: true, StatusTracking: true, SourceSignatures: true}, Builder: ConnectorBuilderProfile{ScaffoldKind: "event_bus_connector", ReusableComponents: []string{"topic_discovery", "schema_projection", "stream_materialization"}, ExampleTargets: []string{"kafka", "kinesis"}}, Certification: ConnectorCertificationProfile{Level: "certified", RuntimeDepth: "streaming_incremental_zero_copy", Auth: "advanced", Observability: "certified", SchemaEvolution: "advanced", PerformancePosture: "advanced", FailureHandling: "advanced"}, Notes: []string{"Shared event-bus template with topic discovery, zero-copy preview and sync."}}
}

func saasProfile(connectorType, displayName string) ConnectorContractProfile {
	return ConnectorContractProfile{ConnectorType: connectorType, DisplayName: displayName, TemplateFamily: "saas_api", Auth: ConnectorAuthProfile{Strategy: "oauth_or_api_key", SecretFields: []string{"client_id", "client_secret"}, SupportsOAuth: true, SupportsPrivateNetworkAgent: true}, Testing: ConnectorTestingProfile{SupportsConnectionTesting: true, SupportsDiscovery: true, SupportsSchemaIntrospection: true}, Sync: ConnectorSyncProfile{Modes: []string{"batch", "incremental"}, SupportsIncremental: true, SupportsCDC: false, SupportsZeroCopy: false}, Observability: ConnectorObservabilityProfile{Retries: true, StatusTracking: true, SourceSignatures: true}, Builder: ConnectorBuilderProfile{ScaffoldKind: "saas_connector", ReusableComponents: []string{"oauth_bootstrap", "discovery", "normalized_sync"}, ExampleTargets: []string{"salesforce", "servicenow"}}, Certification: ConnectorCertificationProfile{Level: "advanced", RuntimeDepth: "batch_incremental", Auth: "certified", Observability: "advanced", SchemaEvolution: "baseline", PerformancePosture: "baseline", FailureHandling: "advanced"}, Notes: []string{"Shared SaaS adapter contract with discovery and normalized sync flows."}}
}

func biProfile(connectorType, displayName string) ConnectorContractProfile {
	return ConnectorContractProfile{ConnectorType: connectorType, DisplayName: displayName, TemplateFamily: "bi_semantic", Auth: ConnectorAuthProfile{Strategy: "oauth_bearer_or_service_principal", SecretFields: []string{"client_id", "client_secret", "bearer_token"}, SupportsOAuth: true, SupportsPrivateNetworkAgent: true}, Testing: ConnectorTestingProfile{SupportsConnectionTesting: true, SupportsDiscovery: true, SupportsSchemaIntrospection: true}, Sync: ConnectorSyncProfile{Modes: []string{"batch", "incremental", "zero_copy"}, SupportsIncremental: true, SupportsCDC: false, SupportsZeroCopy: true}, Observability: ConnectorObservabilityProfile{Retries: true, StatusTracking: true, SourceSignatures: true}, Builder: ConnectorBuilderProfile{ScaffoldKind: "bi_connector", ReusableComponents: []string{"workspace_discovery", "semantic_model_projection", "dashboard_extracts"}, ExampleTargets: []string{"tableau", "power_bi"}}, Certification: ConnectorCertificationProfile{Level: "advanced", RuntimeDepth: "batch_incremental_zero_copy", Auth: "certified", Observability: "advanced", SchemaEvolution: "baseline", PerformancePosture: "baseline", FailureHandling: "advanced"}, Notes: []string{"Bridges BI semantic layers into discovery, virtual-table previews, and scheduled extracts."}}
}

func driverProfile(connectorType, displayName string) ConnectorContractProfile {
	return ConnectorContractProfile{ConnectorType: connectorType, DisplayName: displayName, TemplateFamily: "sql_driver_bridge", Auth: ConnectorAuthProfile{Strategy: "dsn_or_connection_string", SecretFields: []string{"username", "password", "connection_string"}, SupportsOAuth: false, SupportsPrivateNetworkAgent: true}, Testing: ConnectorTestingProfile{SupportsConnectionTesting: true, SupportsDiscovery: true, SupportsSchemaIntrospection: true}, Sync: ConnectorSyncProfile{Modes: []string{"batch", "incremental", "zero_copy"}, SupportsIncremental: true, SupportsCDC: false, SupportsZeroCopy: true}, Observability: ConnectorObservabilityProfile{Retries: true, StatusTracking: true, SourceSignatures: true}, Builder: ConnectorBuilderProfile{ScaffoldKind: "sql_driver_connector", ReusableComponents: []string{"connection_testing", "schema_introspection", "virtual_tables", "private_network_bridge"}, ExampleTargets: []string{"odbc", "jdbc"}}, Certification: ConnectorCertificationProfile{Level: "advanced", RuntimeDepth: "batch_incremental_zero_copy", Auth: "advanced", Observability: "advanced", SchemaEvolution: "advanced", PerformancePosture: "advanced", FailureHandling: "advanced"}, Notes: []string{"Standardizes private-network SQL driver connectivity through DSNs, JDBC URLs, and remote bridge catalogs."}}
}

func erpProfile(connectorType, displayName string) ConnectorContractProfile {
	return ConnectorContractProfile{ConnectorType: connectorType, DisplayName: displayName, TemplateFamily: "erp_api", Auth: ConnectorAuthProfile{Strategy: "service_account_or_basic", SecretFields: []string{"username", "password", "api_key"}, SupportsOAuth: false, SupportsPrivateNetworkAgent: true}, Testing: ConnectorTestingProfile{SupportsConnectionTesting: true, SupportsDiscovery: true, SupportsSchemaIntrospection: true}, Sync: ConnectorSyncProfile{Modes: []string{"batch", "incremental"}, SupportsIncremental: true, SupportsCDC: false, SupportsZeroCopy: false}, Observability: ConnectorObservabilityProfile{Retries: true, StatusTracking: true, SourceSignatures: true}, Builder: ConnectorBuilderProfile{ScaffoldKind: "erp_connector", ReusableComponents: []string{"entity_discovery", "odata_projection", "lineage_templates"}, ExampleTargets: []string{"sap"}}, Certification: ConnectorCertificationProfile{Level: "advanced", RuntimeDepth: "batch_incremental", Auth: "advanced", Observability: "advanced", SchemaEvolution: "baseline", PerformancePosture: "baseline", FailureHandling: "advanced"}, Notes: []string{"ERP connector contract for entity discovery and normalized operational data sync."}}
}

func ConfigKeys(raw json.RawMessage) ([]string, ConfigInferred) {
	var object map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &object) != nil || object == nil {
		return []string{}, ConfigInferred{}
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	inferred := ConfigInferred{}
	for _, key := range keys {
		switch key {
		case "access_key", "secret_key", "password", "credentials", "credential", "username", "user", "api_key", "client_secret", "connection_string", "dsn", "account_key", "auth_header":
			inferred.HasCredentials = true
		case "access_token", "oauth_token", "bearer_token", "refresh_token":
			inferred.HasCredentials = true
			inferred.HasOAuthToken = true
		case "private_key":
			inferred.HasCredentials = true
			inferred.HasPrivateKey = true
		case "cdc", "cdc_enabled", "replication_slot", "publication", "binlog", "change_stream":
			inferred.HasCDCSelector = true
		case "cursor", "cursor_field", "updated_at_column", "incremental_key", "watermark_column":
			inferred.HasIncrementalCursor = true
		case "zero_copy", "virtual_tables", "catalog_url", "table_include", "tables":
			inferred.RequestsZeroCopy = true
		}
	}
	return keys, inferred
}
