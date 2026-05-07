// Package models holds wire types for connector-management-service.
//
// Wire types for connections, sync definitions, and virtual tables.
package models

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ListResponse[T any] struct {
	Items []T `json:"items"`
}

// Connection mirrors the `connections` row.
type Connection struct {
	ID            uuid.UUID       `json:"id"`
	Name          string          `json:"name"`
	ConnectorType string          `json:"connector_type"`
	Config        json.RawMessage `json:"config"`
	Status        string          `json:"status"`
	OwnerID       uuid.UUID       `json:"owner_id"`
	LastSyncAt    *time.Time      `json:"last_sync_at"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// CreateConnectionRequest is POST /api/v1/connections.
type CreateConnectionRequest struct {
	Name          string          `json:"name"`
	ConnectorType string          `json:"connector_type"`
	Config        json.RawMessage `json:"config,omitempty"`
}

// UpdateConnectionRequest mirrors PATCH semantics.
type UpdateConnectionRequest struct {
	Name   *string         `json:"name,omitempty"`
	Config json.RawMessage `json:"config,omitempty"`
	Status *string         `json:"status,omitempty"`
}

// SyncJob mirrors the current Rust data-connection sync definition surface
// backed by batch_sync_defs after sync runtime state moved out of this service.
type SyncJob struct {
	ID              uuid.UUID `json:"id"`
	SourceID        uuid.UUID `json:"source_id"`
	OutputDatasetID uuid.UUID `json:"output_dataset_id"`
	FileGlob        *string   `json:"file_glob"`
	ScheduleCron    *string   `json:"schedule_cron"`
	CreatedAt       time.Time `json:"created_at"`
}

type CreateSyncJobRequest struct {
	SourceID        uuid.UUID `json:"source_id"`
	OutputDatasetID uuid.UUID `json:"output_dataset_id"`
	FileGlob        *string   `json:"file_glob,omitempty"`
	ScheduleCron    *string   `json:"schedule_cron,omitempty"`
}

type UpdateSyncJobRequest struct {
	OutputDatasetID *uuid.UUID `json:"output_dataset_id,omitempty"`
	FileGlob        *string    `json:"file_glob,omitempty"`
	ScheduleCron    *string    `json:"schedule_cron,omitempty"`
}

// MediaSetSyncKind identifies the Foundry media-set sync flavour.
type MediaSetSyncKind string

const (
	MediaSetSyncKindCopy    MediaSetSyncKind = "MEDIA_SET_SYNC"
	MediaSetSyncKindVirtual MediaSetSyncKind = "VIRTUAL_MEDIA_SET_SYNC"
)

type MediaSetSyncFilters struct {
	ExcludeAlreadySynced  bool    `json:"exclude_already_synced"`
	PathGlob              *string `json:"path_glob,omitempty"`
	FileSizeLimit         *uint64 `json:"file_size_limit,omitempty"`
	IgnoreUnmatchedSchema bool    `json:"ignore_unmatched_schema"`
}

type MediaSetSync struct {
	ID                uuid.UUID           `json:"id"`
	SourceID          uuid.UUID           `json:"source_id"`
	Kind              MediaSetSyncKind    `json:"kind"`
	TargetMediaSetRID string              `json:"target_media_set_rid"`
	Subfolder         string              `json:"subfolder"`
	Filters           MediaSetSyncFilters `json:"filters"`
	ScheduleCron      *string             `json:"schedule_cron,omitempty"`
	CreatedAt         time.Time           `json:"created_at"`
}

type CreateMediaSetSyncRequest struct {
	Kind              MediaSetSyncKind    `json:"kind"`
	TargetMediaSetRID string              `json:"target_media_set_rid"`
	Subfolder         string              `json:"subfolder,omitempty"`
	Filters           MediaSetSyncFilters `json:"filters,omitempty"`
	ScheduleCron      *string             `json:"schedule_cron,omitempty"`
}

type UpdateMediaSetSyncRequest struct {
	Kind              *MediaSetSyncKind    `json:"kind,omitempty"`
	TargetMediaSetRID *string              `json:"target_media_set_rid,omitempty"`
	Subfolder         *string              `json:"subfolder,omitempty"`
	Filters           *MediaSetSyncFilters `json:"filters,omitempty"`
	ScheduleCron      *string              `json:"schedule_cron,omitempty"`
}

type SourceFile struct {
	Path      string `json:"path"`
	SizeBytes uint64 `json:"size_bytes"`
	MimeType  string `json:"mime_type"`
}

type RunMediaSetSyncRequest struct {
	SourceFiles      []SourceFile `json:"source_files,omitempty"`
	AlreadySynced    []string     `json:"already_synced,omitempty"`
	AllowedMIMETypes []string     `json:"allowed_mime_types,omitempty"`
}

type SyncStats struct {
	Accepted         uint32 `json:"accepted"`
	Skipped          uint32 `json:"skipped"`
	SchemaMismatched uint32 `json:"schema_mismatched"`
}

type MediaSetSyncExecutionReport struct {
	Stats            SyncStats `json:"stats"`
	Dispatched       uint32    `json:"dispatched"`
	DispatchErrors   uint32    `json:"dispatch_errors"`
	SchemaMismatches []string  `json:"schema_mismatches"`
}

func (k MediaSetSyncKind) Valid() bool {
	return k == MediaSetSyncKindCopy || k == MediaSetSyncKindVirtual
}

func ValidateMediaSetSyncConfig(kind MediaSetSyncKind, targetRID string, filters MediaSetSyncFilters, schedule *string) []string {
	errs := []string{}
	if !kind.Valid() {
		errs = append(errs, "kind must be MEDIA_SET_SYNC or VIRTUAL_MEDIA_SET_SYNC")
	}
	if !strings.HasPrefix(strings.TrimSpace(targetRID), "ri.foundry.main.media_set.") {
		errs = append(errs, "target_media_set_rid must start with ri.foundry.main.media_set.")
	}
	if filters.PathGlob != nil {
		if _, err := filepath.Match(*filters.PathGlob, ""); err != nil {
			errs = append(errs, "invalid path_glob: "+err.Error())
		}
	}
	if filters.FileSizeLimit != nil && *filters.FileSizeLimit == 0 {
		errs = append(errs, "file_size_limit must be > 0")
	}
	if schedule != nil {
		fields := strings.Fields(strings.TrimSpace(*schedule))
		if len(fields) != 5 && len(fields) != 6 {
			errs = append(errs, "schedule_cron must have 5 or 6 fields")
		}
	}
	return errs
}

func (m MediaSetSync) Validate() []string {
	return ValidateMediaSetSyncConfig(m.Kind, m.TargetMediaSetRID, m.Filters, m.ScheduleCron)
}

func (r CreateMediaSetSyncRequest) Validate() []string {
	return ValidateMediaSetSyncConfig(r.Kind, r.TargetMediaSetRID, r.Filters, r.ScheduleCron)
}

type SyncRun struct {
	ID               uuid.UUID  `json:"id"`
	SyncDefID        uuid.UUID  `json:"sync_def_id"`
	Status           string     `json:"status"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at"`
	BytesWritten     int64      `json:"bytes_written"`
	FilesWritten     int64      `json:"files_written"`
	Error            *string    `json:"error"`
	IngestJobID      *string    `json:"ingest_job_id"`
	DatasetVersionID *uuid.UUID `json:"dataset_version_id"`
	ContentHash      *string    `json:"content_hash"`
}

type VirtualTableSourceLink struct {
	SourceRID                   string          `json:"source_rid"`
	Provider                    string          `json:"provider"`
	VirtualTablesEnabled        bool            `json:"virtual_tables_enabled"`
	CodeImportsEnabled          bool            `json:"code_imports_enabled"`
	ExportControls              json.RawMessage `json:"export_controls"`
	AutoRegisterProjectRID      *string         `json:"auto_register_project_rid"`
	AutoRegisterEnabled         bool            `json:"auto_register_enabled"`
	AutoRegisterIntervalSeconds *int32          `json:"auto_register_interval_seconds"`
	AutoRegisterTagFilters      json.RawMessage `json:"auto_register_tag_filters"`
	IcebergCatalogKind          *string         `json:"iceberg_catalog_kind"`
	IcebergCatalogConfig        json.RawMessage `json:"iceberg_catalog_config"`
	CreatedAt                   time.Time       `json:"created_at"`
	UpdatedAt                   time.Time       `json:"updated_at"`
}

type EnableVirtualTableSourceRequest struct {
	Provider             string          `json:"provider"`
	IcebergCatalogKind   *string         `json:"iceberg_catalog_kind,omitempty"`
	IcebergCatalogConfig json.RawMessage `json:"iceberg_catalog_config,omitempty"`
}

type VirtualTable struct {
	ID                                 uuid.UUID       `json:"id"`
	RID                                string          `json:"rid"`
	SourceRID                          string          `json:"source_rid"`
	ProjectRID                         string          `json:"project_rid"`
	Name                               string          `json:"name"`
	ParentFolderRID                    *string         `json:"parent_folder_rid"`
	Locator                            json.RawMessage `json:"locator"`
	TableType                          string          `json:"table_type"`
	SchemaInferred                     json.RawMessage `json:"schema_inferred"`
	Capabilities                       json.RawMessage `json:"capabilities"`
	UpdateDetectionEnabled             bool            `json:"update_detection_enabled"`
	UpdateDetectionIntervalSeconds     *int32          `json:"update_detection_interval_seconds"`
	LastObservedVersion                *string         `json:"last_observed_version"`
	LastPolledAt                       *time.Time      `json:"last_polled_at"`
	UpdateDetectionConsecutiveFailures int32           `json:"update_detection_consecutive_failures"`
	UpdateDetectionNextPollAt          *time.Time      `json:"update_detection_next_poll_at"`
	Markings                           []string        `json:"markings"`
	Properties                         json.RawMessage `json:"properties"`
	CreatedBy                          *string         `json:"created_by"`
	CreatedAt                          time.Time       `json:"created_at"`
	UpdatedAt                          time.Time       `json:"updated_at"`
}

type Locator struct {
	Kind      string `json:"kind"`
	Database  string `json:"database,omitempty"`
	Schema    string `json:"schema,omitempty"`
	Table     string `json:"table,omitempty"`
	Bucket    string `json:"bucket,omitempty"`
	Prefix    string `json:"prefix,omitempty"`
	Format    string `json:"format,omitempty"`
	Catalog   string `json:"catalog,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

type CreateVirtualTableRequest struct {
	ProjectRID      string   `json:"project_rid"`
	Name            *string  `json:"name,omitempty"`
	ParentFolderRID *string  `json:"parent_folder_rid,omitempty"`
	Locator         Locator  `json:"locator"`
	TableType       string   `json:"table_type"`
	Markings        []string `json:"markings,omitempty"`
}

type ListVirtualTablesResponse struct {
	Items      []VirtualTable `json:"items"`
	NextCursor *string        `json:"next_cursor"`
}

func (l Locator) CanonicalJSON() (json.RawMessage, error) {
	switch l.Kind {
	case "tabular":
		return json.Marshal(map[string]string{"kind": "tabular", "database": strings.TrimSpace(l.Database), "schema": strings.TrimSpace(l.Schema), "table": strings.TrimSpace(l.Table)})
	case "file":
		return json.Marshal(map[string]string{"kind": "file", "bucket": strings.TrimSpace(l.Bucket), "prefix": strings.TrimSpace(l.Prefix), "format": strings.ToLower(strings.TrimSpace(l.Format))})
	case "iceberg":
		return json.Marshal(map[string]string{"kind": "iceberg", "catalog": strings.TrimSpace(l.Catalog), "namespace": strings.TrimSpace(l.Namespace), "table": strings.TrimSpace(l.Table)})
	default:
		return nil, fmt.Errorf("invalid locator kind: %s", l.Kind)
	}
}

func (l Locator) DefaultDisplayName() string {
	switch l.Kind {
	case "tabular", "iceberg":
		return strings.TrimSpace(l.Table)
	case "file":
		bucket := strings.TrimSpace(l.Bucket)
		prefix := strings.TrimSpace(l.Prefix)
		if prefix == "" {
			return bucket
		}
		return bucket + "/" + prefix
	default:
		return ""
	}
}

// ConnectorAgent mirrors models/agent.rs.
type ConnectorAgent struct {
	ID              uuid.UUID       `json:"id"`
	Name            string          `json:"name"`
	AgentURL        string          `json:"agent_url"`
	OwnerID         uuid.UUID       `json:"owner_id"`
	Status          string          `json:"status"`
	Capabilities    json.RawMessage `json:"capabilities"`
	Metadata        json.RawMessage `json:"metadata"`
	LastHeartbeatAt *time.Time      `json:"last_heartbeat_at"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type RegisterAgentRequest struct {
	Name         string          `json:"name"`
	AgentURL     string          `json:"agent_url"`
	Capabilities json.RawMessage `json:"capabilities"`
	Metadata     json.RawMessage `json:"metadata"`
}

type AgentHeartbeatRequest struct {
	Capabilities json.RawMessage `json:"capabilities"`
	Metadata     json.RawMessage `json:"metadata"`
}

type ListConnectionsQuery struct {
	Page    *int64 `json:"page,omitempty"`
	PerPage *int64 `json:"per_page,omitempty"`
}

type ConnectorContractCatalog struct {
	Connectors           []ConnectorContractProfile    `json:"connectors"`
	CertificationSummary ConnectorCertificationSummary `json:"certification_summary"`
}

type ConnectorContractProfile struct {
	ConnectorType  string                        `json:"connector_type"`
	DisplayName    string                        `json:"display_name"`
	TemplateFamily string                        `json:"template_family"`
	Auth           ConnectorAuthProfile          `json:"auth"`
	Testing        ConnectorTestingProfile       `json:"testing"`
	Sync           ConnectorSyncProfile          `json:"sync"`
	Observability  ConnectorObservabilityProfile `json:"observability"`
	Builder        ConnectorBuilderProfile       `json:"builder"`
	Certification  ConnectorCertificationProfile `json:"certification"`
	Notes          []string                      `json:"notes"`
}

type ConnectorAuthProfile struct {
	Strategy                    string   `json:"strategy"`
	SecretFields                []string `json:"secret_fields"`
	SupportsOAuth               bool     `json:"supports_oauth"`
	SupportsPrivateNetworkAgent bool     `json:"supports_private_network_agent"`
}

type ConnectorTestingProfile struct {
	SupportsConnectionTesting   bool `json:"supports_connection_testing"`
	SupportsDiscovery           bool `json:"supports_discovery"`
	SupportsSchemaIntrospection bool `json:"supports_schema_introspection"`
}

type ConnectorSyncProfile struct {
	Modes               []string `json:"modes"`
	SupportsIncremental bool     `json:"supports_incremental"`
	SupportsCDC         bool     `json:"supports_cdc"`
	SupportsZeroCopy    bool     `json:"supports_zero_copy"`
}

type ConnectorObservabilityProfile struct {
	Retries          bool `json:"retries"`
	StatusTracking   bool `json:"status_tracking"`
	SourceSignatures bool `json:"source_signatures"`
}

type ConnectorBuilderProfile struct {
	ScaffoldKind       string   `json:"scaffold_kind"`
	ReusableComponents []string `json:"reusable_components"`
	ExampleTargets     []string `json:"example_targets"`
}

type ConnectorCertificationProfile struct {
	Level              string `json:"level"`
	RuntimeDepth       string `json:"runtime_depth"`
	Auth               string `json:"auth"`
	Observability      string `json:"observability"`
	SchemaEvolution    string `json:"schema_evolution"`
	PerformancePosture string `json:"performance_posture"`
	FailureHandling    string `json:"failure_handling"`
}

type ConnectorCertificationSummary struct {
	CertifiedConnectors        int      `json:"certified_connectors"`
	AdvancedConnectors         int      `json:"advanced_connectors"`
	ConnectorsNeedingHardening int      `json:"connectors_needing_hardening"`
	TemplateFamilies           []string `json:"template_families"`
}

type ConnectionCapabilityResponse struct {
	ConnectionID  uuid.UUID                       `json:"connection_id"`
	ConnectorType string                          `json:"connector_type"`
	Status        string                          `json:"status"`
	Contract      ConnectorContractProfile        `json:"contract"`
	Capabilities  ConnectionEffectiveCapabilities `json:"capabilities"`
}

type CredentialResponse struct {
	ID          uuid.UUID `json:"id"`
	SourceID    uuid.UUID `json:"source_id"`
	Kind        string    `json:"kind"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
}

type SetCredentialRequest struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type SourcePolicyBindingResponse struct {
	SourceID uuid.UUID `json:"source_id"`
	PolicyID uuid.UUID `json:"policy_id"`
	Kind     string    `json:"kind"`
}

type AttachPolicyRequest struct {
	PolicyID uuid.UUID `json:"policy_id"`
	Kind     string    `json:"kind"`
}

type SyncStatus string

const (
	SyncStatusPending   SyncStatus = "pending"
	SyncStatusRunning   SyncStatus = "running"
	SyncStatusRetrying  SyncStatus = "retrying"
	SyncStatusCompleted SyncStatus = "completed"
	SyncStatusFailed    SyncStatus = "failed"
)

type LegacySyncJob struct {
	ID                   uuid.UUID       `json:"id"`
	ConnectionID         uuid.UUID       `json:"connection_id"`
	TargetDatasetID      *uuid.UUID      `json:"target_dataset_id"`
	TableName            string          `json:"table_name"`
	Status               string          `json:"status"`
	RowsSynced           int64           `json:"rows_synced"`
	Error                *string         `json:"error"`
	Attempts             int32           `json:"attempts"`
	MaxAttempts          int32           `json:"max_attempts"`
	ScheduledAt          time.Time       `json:"scheduled_at"`
	NextRetryAt          *time.Time      `json:"next_retry_at"`
	ResultDatasetVersion *int32          `json:"result_dataset_version"`
	SyncMetadata         json.RawMessage `json:"sync_metadata"`
	StartedAt            *time.Time      `json:"started_at"`
	CompletedAt          *time.Time      `json:"completed_at"`
	CreatedAt            time.Time       `json:"created_at"`
}

type SyncRequest struct {
	TableName       string     `json:"table_name"`
	TargetDatasetID *uuid.UUID `json:"target_dataset_id"`
	ScheduleAt      *time.Time `json:"schedule_at"`
	MaxAttempts     *int32     `json:"max_attempts"`
}

type ConnectionRegistration struct {
	ID                  uuid.UUID       `json:"id"`
	ConnectionID        uuid.UUID       `json:"connection_id"`
	Selector            string          `json:"selector"`
	DisplayName         string          `json:"display_name"`
	SourceKind          string          `json:"source_kind"`
	RegistrationMode    string          `json:"registration_mode"`
	AutoSync            bool            `json:"auto_sync"`
	UpdateDetection     bool            `json:"update_detection"`
	TargetDatasetID     *uuid.UUID      `json:"target_dataset_id"`
	LastSourceSignature *string         `json:"last_source_signature"`
	LastDatasetVersion  *int32          `json:"last_dataset_version"`
	Metadata            json.RawMessage `json:"metadata"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

type DiscoveredSource struct {
	Selector         string          `json:"selector"`
	DisplayName      string          `json:"display_name"`
	SourceKind       string          `json:"source_kind"`
	SupportsSync     bool            `json:"supports_sync"`
	SupportsZeroCopy bool            `json:"supports_zero_copy"`
	SourceSignature  *string         `json:"source_signature,omitempty"`
	Metadata         json.RawMessage `json:"metadata"`
}

type AutoRegisterRequest struct {
	Selectors              []string   `json:"selectors"`
	RegistrationMode       *string    `json:"registration_mode"`
	AutoSync               *bool      `json:"auto_sync"`
	UpdateDetection        *bool      `json:"update_detection"`
	DefaultTargetDatasetID *uuid.UUID `json:"default_target_dataset_id"`
}

type RegistrationBulkRegisterRequest struct {
	Registrations []BulkRegistrationItem `json:"registrations"`
}

type BulkRegistrationItem struct {
	Selector         string          `json:"selector"`
	DisplayName      *string         `json:"display_name"`
	SourceKind       *string         `json:"source_kind"`
	RegistrationMode *string         `json:"registration_mode"`
	AutoSync         *bool           `json:"auto_sync"`
	UpdateDetection  *bool           `json:"update_detection"`
	TargetDatasetID  *uuid.UUID      `json:"target_dataset_id"`
	Metadata         json.RawMessage `json:"metadata"`
}

type VirtualTableQueryRequest struct {
	Selector string `json:"selector"`
	Limit    *int   `json:"limit"`
}

type VirtualTableQueryResponse struct {
	Selector        string            `json:"selector"`
	Mode            string            `json:"mode"`
	Columns         []string          `json:"columns"`
	RowCount        int               `json:"row_count"`
	Rows            []json.RawMessage `json:"rows"`
	SourceSignature *string           `json:"source_signature,omitempty"`
	Metadata        json.RawMessage   `json:"metadata"`
}

type QueryRegistrationBody struct {
	Limit *int `json:"limit"`
}

type UpdateAutoRegistrationBody struct {
	Enabled         bool     `json:"enabled"`
	IntervalSeconds *int32   `json:"interval_seconds"`
	TagFilters      []string `json:"tag_filters"`
}

type HyperAutoErpRequest struct {
	Selectors        []string `json:"selectors"`
	MaxEntities      *int     `json:"max_entities"`
	SampleLimit      *int     `json:"sample_limit"`
	ScheduleCron     *string  `json:"schedule_cron"`
	PipelineStatus   *string  `json:"pipeline_status"`
	QueueInitialSync *bool    `json:"queue_initial_sync"`
	CreateLinkTypes  *bool    `json:"create_link_types"`
}

type HyperAutoErpFieldPlan struct {
	SourceName       string `json:"source_name"`
	PropertyName     string `json:"property_name"`
	PropertyType     string `json:"property_type"`
	Nullable         bool   `json:"nullable"`
	UniqueConstraint bool   `json:"unique_constraint"`
	SemanticRole     string `json:"semantic_role"`
}

type HyperAutoErpEntityPlan struct {
	Selector           string                  `json:"selector"`
	DisplayName        string                  `json:"display_name"`
	SourceKind         string                  `json:"source_kind"`
	Module             string                  `json:"module"`
	SampleRowCount     int                     `json:"sample_row_count"`
	RawDatasetName     string                  `json:"raw_dataset_name"`
	CuratedDatasetName string                  `json:"curated_dataset_name"`
	PipelineName       string                  `json:"pipeline_name"`
	ObjectTypeName     string                  `json:"object_type_name"`
	ObjectDisplayName  string                  `json:"object_display_name"`
	PrimaryKeyProperty *string                 `json:"primary_key_property"`
	NormalizationSQL   string                  `json:"normalization_sql"`
	Fields             []HyperAutoErpFieldPlan `json:"fields"`
}

type HyperAutoErpLinkPlan struct {
	Name                     string  `json:"name"`
	DisplayName              string  `json:"display_name"`
	SourceObjectTypeName     string  `json:"source_object_type_name"`
	TargetObjectTypeName     string  `json:"target_object_type_name"`
	SourcePropertyName       string  `json:"source_property_name"`
	TargetPrimaryKeyProperty *string `json:"target_primary_key_property"`
	Cardinality              string  `json:"cardinality"`
	Rationale                string  `json:"rationale"`
}

type HyperAutoErpPreviewResponse struct {
	ConnectionID   uuid.UUID                `json:"connection_id"`
	ConnectionName string                   `json:"connection_name"`
	ConnectorType  string                   `json:"connector_type"`
	ErpSystem      string                   `json:"erp_system"`
	GeneratedAt    time.Time                `json:"generated_at"`
	EntityCount    int                      `json:"entity_count"`
	PipelineStatus string                   `json:"pipeline_status"`
	ScheduleCron   *string                  `json:"schedule_cron"`
	Entities       []HyperAutoErpEntityPlan `json:"entities"`
	Links          []HyperAutoErpLinkPlan   `json:"links"`
	Warnings       []string                 `json:"warnings"`
}

type HyperAutoGeneratedDataset struct {
	Selector    string    `json:"selector"`
	DatasetID   uuid.UUID `json:"dataset_id"`
	DatasetName string    `json:"dataset_name"`
	Stage       string    `json:"stage"`
	Reused      bool      `json:"reused"`
}

type HyperAutoGeneratedRegistration struct {
	Selector        string    `json:"selector"`
	RegistrationID  uuid.UUID `json:"registration_id"`
	TargetDatasetID uuid.UUID `json:"target_dataset_id"`
}

type HyperAutoGeneratedPipeline struct {
	Selector     string    `json:"selector"`
	PipelineID   uuid.UUID `json:"pipeline_id"`
	PipelineName string    `json:"pipeline_name"`
	Reused       bool      `json:"reused"`
}

type HyperAutoGeneratedObjectType struct {
	Selector          string    `json:"selector"`
	ObjectTypeID      uuid.UUID `json:"object_type_id"`
	ObjectTypeName    string    `json:"object_type_name"`
	Reused            bool      `json:"reused"`
	PropertiesCreated int       `json:"properties_created"`
}

type HyperAutoGeneratedLinkType struct {
	Name       string    `json:"name"`
	LinkTypeID uuid.UUID `json:"link_type_id"`
	Reused     bool      `json:"reused"`
}

type HyperAutoQueuedIngestJob struct {
	Selector        string    `json:"selector"`
	JobID           uuid.UUID `json:"job_id"`
	TargetDatasetID uuid.UUID `json:"target_dataset_id"`
	ScheduledAt     time.Time `json:"scheduled_at"`
}

type HyperAutoErpGenerateResponse struct {
	Preview         HyperAutoErpPreviewResponse      `json:"preview"`
	RawDatasets     []HyperAutoGeneratedDataset      `json:"raw_datasets"`
	CuratedDatasets []HyperAutoGeneratedDataset      `json:"curated_datasets"`
	Registrations   []HyperAutoGeneratedRegistration `json:"registrations"`
	Pipelines       []HyperAutoGeneratedPipeline     `json:"pipelines"`
	ObjectTypes     []HyperAutoGeneratedObjectType   `json:"object_types"`
	LinkTypes       []HyperAutoGeneratedLinkType     `json:"link_types"`
	IngestJobs      []HyperAutoQueuedIngestJob       `json:"ingest_jobs"`
}

type SourceProvider string

const (
	SourceProviderAmazonS3       SourceProvider = "AMAZON_S3"
	SourceProviderAzureABFS      SourceProvider = "AZURE_ABFS"
	SourceProviderBigQuery       SourceProvider = "BIGQUERY"
	SourceProviderDatabricks     SourceProvider = "DATABRICKS"
	SourceProviderFoundryIceberg SourceProvider = "FOUNDRY_ICEBERG"
	SourceProviderGCS            SourceProvider = "GCS"
	SourceProviderSnowflake      SourceProvider = "SNOWFLAKE"
)

type TableType string

const (
	TableTypeTable            TableType = "TABLE"
	TableTypeView             TableType = "VIEW"
	TableTypeMaterializedView TableType = "MATERIALIZED_VIEW"
	TableTypeExternalDelta    TableType = "EXTERNAL_DELTA"
	TableTypeManagedDelta     TableType = "MANAGED_DELTA"
	TableTypeManagedIceberg   TableType = "MANAGED_ICEBERG"
	TableTypeParquetFiles     TableType = "PARQUET_FILES"
	TableTypeAvroFiles        TableType = "AVRO_FILES"
	TableTypeCSVFiles         TableType = "CSV_FILES"
	TableTypeOther            TableType = "OTHER"
)

type ComputePushdownEngine string

const (
	ComputePushdownEngineIbis     ComputePushdownEngine = "ibis"
	ComputePushdownEnginePySpark  ComputePushdownEngine = "pyspark"
	ComputePushdownEngineSnowpark ComputePushdownEngine = "snowpark"
)

type FoundryCompute struct {
	PythonSingleNode          bool `json:"python_single_node"`
	PythonSpark               bool `json:"python_spark"`
	PipelineBuilderSingleNode bool `json:"pipeline_builder_single_node"`
	PipelineBuilderSpark      bool `json:"pipeline_builder_spark"`
}

type Capabilities struct {
	Read                bool                   `json:"read"`
	Write               bool                   `json:"write"`
	Incremental         bool                   `json:"incremental"`
	Versioning          bool                   `json:"versioning"`
	ComputePushdown     *ComputePushdownEngine `json:"compute_pushdown"`
	SnapshotSupported   bool                   `json:"snapshot_supported"`
	AppendOnlySupported bool                   `json:"append_only_supported"`
	FoundryCompute      FoundryCompute         `json:"foundry_compute"`
}

type VirtualTableBulkRegisterRequest struct {
	ProjectRID string                      `json:"project_rid"`
	Entries    []CreateVirtualTableRequest `json:"entries"`
}

type VirtualTableBulkRegisterResponse struct {
	Registered []VirtualTable          `json:"registered"`
	Errors     []VirtualTableBulkError `json:"errors"`
}

type VirtualTableBulkError struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

type UpdateMarkingsRequest struct {
	Markings []string `json:"markings"`
}

type DiscoverQuery struct {
	Path *string `json:"path"`
}

type DiscoveredEntry struct {
	DisplayName       string  `json:"display_name"`
	Path              string  `json:"path"`
	Kind              string  `json:"kind"`
	Registrable       bool    `json:"registrable"`
	InferredTableType *string `json:"inferred_table_type"`
}

type ListVirtualTablesQuery struct {
	Project   *string `json:"project"`
	Source    *string `json:"source"`
	Name      *string `json:"name"`
	TableType *string `json:"type"`
	Limit     *int64  `json:"limit"`
	Cursor    *string `json:"cursor"`
}

type FolderMirrorKind string

const (
	FolderMirrorKindFlat   FolderMirrorKind = "FLAT"
	FolderMirrorKindNested FolderMirrorKind = "NESTED"
)

type RemoteTable struct {
	Database        string    `json:"database"`
	Schema          string    `json:"schema"`
	Table           string    `json:"table"`
	TableType       TableType `json:"table_type"`
	SchemaSignature string    `json:"schema_signature"`
	Tags            []string  `json:"tags"`
}

type ExistingTable struct {
	RID             string `json:"rid"`
	Database        string `json:"database"`
	Schema          string `json:"schema"`
	Table           string `json:"table"`
	SchemaSignature string `json:"schema_signature"`
}

type DiffResult struct {
	Added    []RemoteTable   `json:"added"`
	Updated  []UpdatedTable  `json:"updated"`
	Orphaned []ExistingTable `json:"orphaned"`
}

type UpdatedTable struct {
	RID    string      `json:"rid"`
	Remote RemoteTable `json:"remote"`
}

type SourceAutoRegisterConfig struct {
	SourceRID           string           `json:"source_rid"`
	Provider            SourceProvider   `json:"provider"`
	ProjectRID          string           `json:"project_rid"`
	Layout              FolderMirrorKind `json:"layout"`
	TagFilters          []string         `json:"tag_filters"`
	PollIntervalSeconds uint64           `json:"poll_interval_seconds"`
}

type EnableAutoRegistrationRequest struct {
	ProjectName         string   `json:"project_name"`
	FolderMirrorKind    string   `json:"folder_mirror_kind"`
	TableTagFilters     []string `json:"table_tag_filters"`
	PollIntervalSeconds uint64   `json:"poll_interval_seconds"`
}

type AutoRegisterRun struct {
	ID         uuid.UUID       `json:"id"`
	SourceRID  string          `json:"source_rid"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at"`
	Status     string          `json:"status"`
	Added      int32           `json:"added"`
	Updated    int32           `json:"updated"`
	Orphaned   int32           `json:"orphaned"`
	Errors     json.RawMessage `json:"errors"`
}

type AutoRegistrationSettingsView struct {
	Enabled          bool     `json:"enabled"`
	RegistrationMode string   `json:"registration_mode"`
	AutoSync         bool     `json:"auto_sync"`
	UpdateDetection  bool     `json:"update_detection"`
	Selectors        []string `json:"selectors"`
}

type Version struct {
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
}

type PollOutcome string

const (
	PollOutcomeInitial         PollOutcome = "initial"
	PollOutcomeChanged         PollOutcome = "changed"
	PollOutcomeUnchanged       PollOutcome = "unchanged"
	PollOutcomePotentialUpdate PollOutcome = "potential_update"
	PollOutcomeFailed          PollOutcome = "failed"
)

type UpdateDetectionToggle struct {
	Enabled         bool   `json:"enabled"`
	IntervalSeconds uint64 `json:"interval_seconds"`
}

type PollResult struct {
	VirtualTableRID string      `json:"virtual_table_rid"`
	Outcome         PollOutcome `json:"outcome"`
	ObservedVersion *string     `json:"observed_version"`
	PreviousVersion *string     `json:"previous_version"`
	LatencyMS       int32       `json:"latency_ms"`
	ChangeDetected  bool        `json:"change_detected"`
	EventEmitted    bool        `json:"event_emitted"`
}

type PollHistoryRow struct {
	ID              uuid.UUID `json:"id"`
	VirtualTableID  uuid.UUID `json:"virtual_table_id"`
	PolledAt        time.Time `json:"polled_at"`
	ObservedVersion *string   `json:"observed_version"`
	ChangeDetected  bool      `json:"change_detected"`
	LatencyMS       int32     `json:"latency_ms"`
	ErrorMessage    *string   `json:"error_message"`
}

type ExportControls struct {
	AllowedMarkings      []string `json:"allowed_markings"`
	AllowedOrganizations []string `json:"allowed_organizations"`
}

type ToggleCodeImportsRequest struct {
	Enabled        bool           `json:"enabled"`
	ExportControls ExportControls `json:"export_controls"`
}

type ArrowType string

const (
	ArrowTypeBoolean   ArrowType = "boolean"
	ArrowTypeInt32     ArrowType = "int32"
	ArrowTypeInt64     ArrowType = "int64"
	ArrowTypeFloat32   ArrowType = "float32"
	ArrowTypeFloat64   ArrowType = "float64"
	ArrowTypeDecimal   ArrowType = "decimal"
	ArrowTypeUtf8      ArrowType = "utf8"
	ArrowTypeBinary    ArrowType = "binary"
	ArrowTypeDate32    ArrowType = "date32"
	ArrowTypeTimestamp ArrowType = "timestamp"
	ArrowTypeList      ArrowType = "list"
	ArrowTypeStruct    ArrowType = "struct"
)

type Mapping struct {
	Arrow   ArrowType `json:"arrow"`
	Warning *string   `json:"warning"`
}

type InferredColumn struct {
	Name         string `json:"name"`
	SourceType   string `json:"source_type"`
	InferredType string `json:"inferred_type"`
	Nullable     bool   `json:"nullable"`
}

type IcebergConfigResponse struct {
	Defaults  IcebergConfigValues `json:"defaults"`
	Overrides IcebergConfigValues `json:"overrides"`
}

type IcebergConfigValues struct {
	Warehouse string `json:"warehouse,omitempty"`
}

type IcebergListNamespacesResponse struct {
	Namespaces [][]string `json:"namespaces"`
}

type IcebergNamespaceResponse struct {
	Namespace  []string          `json:"namespace"`
	Properties map[string]string `json:"properties"`
}

type IcebergTableIdentifier struct {
	Namespace []string `json:"namespace"`
	Name      string   `json:"name"`
}

type IcebergListTablesResponse struct {
	Identifiers []IcebergTableIdentifier `json:"identifiers"`
}

type IcebergLoadTableResponse struct {
	MetadataLocation string          `json:"metadata-location"`
	Metadata         json.RawMessage `json:"metadata"`
	Config           json.RawMessage `json:"config"`
}

type WebhookDefinition struct {
	URL          string            `json:"url"`
	Method       string            `json:"method"`
	Headers      map[string]string `json:"headers"`
	InputSchema  json.RawMessage   `json:"input_schema"`
	OutputSchema json.RawMessage   `json:"output_schema"`
	AuthRef      *string           `json:"auth_ref"`
}

type InvokeWebhookRequest struct {
	Inputs json.RawMessage `json:"inputs"`
}

type InvokeWebhookResponse struct {
	Status           uint16          `json:"status"`
	Response         json.RawMessage `json:"response"`
	OutputParameters json.RawMessage `json:"output_parameters"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthenticatedResponse struct {
	Status       string `json:"status"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

type UserProfile struct {
	ID             uuid.UUID       `json:"id"`
	Email          string          `json:"email"`
	Name           string          `json:"name"`
	IsActive       bool            `json:"is_active"`
	Roles          []string        `json:"roles"`
	Groups         []string        `json:"groups"`
	Permissions    []string        `json:"permissions"`
	OrganizationID *uuid.UUID      `json:"organization_id"`
	Attributes     json.RawMessage `json:"attributes"`
	MFAEnabled     bool            `json:"mfa_enabled"`
	MFAEnforced    bool            `json:"mfa_enforced"`
	AuthSource     string          `json:"auth_source"`
	CreatedAt      string          `json:"created_at"`
}

type BootstrapStatusResponse struct {
	RequiresInitialAdmin bool `json:"requires_initial_admin"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

type StreamingSyncFieldDescriptor struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type StreamingSourceContract struct {
	Kind          string                         `json:"kind"`
	DisplayName   string                         `json:"display_name"`
	Description   string                         `json:"description"`
	RequiresAgent bool                           `json:"requires_agent"`
	ConfigFields  []StreamingSyncFieldDescriptor `json:"config_fields"`
}

type ConnectionChangedEvent struct {
	EventType     string          `json:"event_type"`
	Aggregate     string          `json:"aggregate"`
	AggregateID   string          `json:"aggregate_id"`
	Version       string          `json:"version"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Name          string          `json:"name"`
	ConnectorType string          `json:"connector_type"`
	Status        string          `json:"status"`
	Payload       json.RawMessage `json:"payload"`
}

type OutboxEvent struct {
	ID              uuid.UUID       `json:"id"`
	Aggregate       string          `json:"aggregate"`
	AggregateID     string          `json:"aggregate_id"`
	Topic           string          `json:"topic"`
	Payload         json.RawMessage `json:"payload"`
	OccurredAt      time.Time       `json:"occurred_at"`
	PublishedAt     *time.Time      `json:"published_at"`
	PublishAttempts int32           `json:"publish_attempts"`
	LastError       *string         `json:"last_error"`
}
