// Package models holds wire types for dataset-versioning-service.
package models

import (
	"bytes"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
)

func sortUUIDsAscending(ids []uuid.UUID) {
	sort.Slice(ids, func(i, j int) bool {
		return bytes.Compare(ids[i][:], ids[j][:]) < 0
	})
}

// ListResponse is the canonical envelope used by the legacy dataset surface.
type ListResponse[T any] struct {
	Items []T `json:"items"`
}

// Page is the Rust-compatible paginated envelope for dataset version lists.
type Page[T any] struct {
	Data       []T     `json:"data"`
	NextCursor *string `json:"next_cursor,omitempty"`
	HasMore    bool    `json:"has_more"`
}

// Dataset mirrors the `datasets` row, byte-compatible with Rust
// `data_asset_catalog::models::dataset::Dataset`.
type Dataset struct {
	ID             uuid.UUID       `json:"id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Format         string          `json:"format"`
	StoragePath    string          `json:"storage_path"`
	SizeBytes      int64           `json:"size_bytes"`
	RowCount       int64           `json:"row_count"`
	OwnerID        uuid.UUID       `json:"owner_id"`
	Tags           []string        `json:"tags"`
	CurrentVersion int32           `json:"current_version"`
	ActiveBranch   string          `json:"active_branch"`
	Metadata       json.RawMessage `json:"metadata"`
	HealthStatus   string          `json:"health_status"`
	CurrentViewID  *uuid.UUID      `json:"current_view_id"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// CreateDatasetRequest is the body of POST /v1/datasets, mirroring Rust
// `CreateDatasetRequest`.
type CreateDatasetRequest struct {
	Name         string          `json:"name"`
	Description  *string         `json:"description,omitempty"`
	Format       *string         `json:"format,omitempty"`
	Tags         []string        `json:"tags,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	HealthStatus *string         `json:"health_status,omitempty"`
}

// UpdateDatasetRequest mirrors Rust `UpdateDatasetRequest` PATCH semantics.
type UpdateDatasetRequest struct {
	Name          *string         `json:"name,omitempty"`
	Description   *string         `json:"description,omitempty"`
	OwnerID       *uuid.UUID      `json:"owner_id,omitempty"`
	Tags          []string        `json:"tags,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	HealthStatus  *string         `json:"health_status,omitempty"`
	CurrentViewID *uuid.UUID      `json:"current_view_id,omitempty"`
}

// DatasetVersion mirrors the Rust DatasetVersion model.
type DatasetVersion struct {
	ID            uuid.UUID  `json:"id"`
	DatasetID     uuid.UUID  `json:"dataset_id"`
	Version       int32      `json:"version"`
	Message       string     `json:"message"`
	SizeBytes     int64      `json:"size_bytes"`
	RowCount      int64      `json:"row_count"`
	StoragePath   string     `json:"storage_path"`
	TransactionID *uuid.UUID `json:"transaction_id"`
	CreatedAt     time.Time  `json:"created_at"`
}

// CreateDatasetVersionRequest is the body of POST /api/v1/datasets/{id}/versions.
type CreateDatasetVersionRequest struct {
	Version       *int32     `json:"version,omitempty"`
	Message       string     `json:"message,omitempty"`
	SizeBytes     int64      `json:"size_bytes,omitempty"`
	RowCount      int64      `json:"row_count,omitempty"`
	StoragePath   string     `json:"storage_path"`
	TransactionID *uuid.UUID `json:"transaction_id,omitempty"`
}

// DatasetBranch mirrors the Rust DatasetBranch wire model.
type DatasetBranch struct {
	ID                       uuid.UUID       `json:"id"`
	RID                      string          `json:"rid"`
	DatasetID                uuid.UUID       `json:"dataset_id"`
	DatasetRID               string          `json:"dataset_rid"`
	Name                     string          `json:"name"`
	ParentBranchID           *uuid.UUID      `json:"parent_branch_id"`
	HeadTransactionID        *uuid.UUID      `json:"head_transaction_id"`
	CreatedFromTransactionID *uuid.UUID      `json:"created_from_transaction_id"`
	LastActivityAt           time.Time       `json:"last_activity_at"`
	Labels                   json.RawMessage `json:"labels"`
	FallbackChain            []string        `json:"fallback_chain"`
	Version                  int32           `json:"version"`
	BaseVersion              int32           `json:"base_version"`
	Description              string          `json:"description"`
	IsDefault                bool            `json:"is_default"`
	CreatedAt                time.Time       `json:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at"`
}

// CreateDatasetBranchRequest mirrors Rust CreateDatasetBranchRequest.
type CreateDatasetBranchRequest struct {
	Name          string `json:"name"`
	SourceVersion *int32 `json:"source_version,omitempty"`
	Description   string `json:"description,omitempty"`
}

// MergeDatasetBranchRequest mirrors Rust MergeDatasetBranchRequest used by the
// legacy merge/promote endpoints.
type MergeDatasetBranchRequest struct {
	TargetBranch *string `json:"target_branch,omitempty"`
}

// IsRoot mirrors Rust DatasetBranch::is_root: a branch is root iff it has no parent.
func (b DatasetBranch) IsRoot() bool {
	return b.ParentBranchID == nil
}

// BranchRID returns the public branch RID, synthesising one when reading rows
// that pre-date the 20260504000010_branches_unify migration.
func (b DatasetBranch) BranchRID() string {
	if b.RID != "" {
		return b.RID
	}
	return "ri.foundry.main.branch." + b.ID.String()
}

// ParentBranchRID returns the parent branch RID, if any.
func (b DatasetBranch) ParentBranchRID() *string {
	if b.ParentBranchID == nil {
		return nil
	}
	rid := "ri.foundry.main.branch." + b.ParentBranchID.String()
	return &rid
}

// HeadTransactionRID returns the head transaction RID, if any.
func (b DatasetBranch) HeadTransactionRID() *string {
	return formatTransactionRIDOpt(b.HeadTransactionID)
}

// CreatedFromTransactionRID returns the source transaction RID for branches
// minted via source.from_transaction_rid.
func (b DatasetBranch) CreatedFromTransactionRID() *string {
	return formatTransactionRIDOpt(b.CreatedFromTransactionID)
}

func formatTransactionRIDOpt(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	rid := "ri.foundry.main.transaction." + id.String()
	return &rid
}

// DatasetFile exposes the persisted Foundry logical-to-physical backing file
// mapping for one dataset file.
type DatasetFile struct {
	ID            uuid.UUID  `json:"id"`
	DatasetID     uuid.UUID  `json:"dataset_id"`
	TransactionID uuid.UUID  `json:"transaction_id"`
	LogicalPath   string     `json:"logical_path"`
	PhysicalURI   string     `json:"physical_uri"`
	SizeBytes     int64      `json:"size_bytes"`
	SHA256        *string    `json:"sha256,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ModifiedAt    time.Time  `json:"modified_at"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
	Status        string     `json:"status"`
}

// ListDatasetFilesResponse is returned by GET /api/v1/datasets/{id}/files.
type ListDatasetFilesResponse struct {
	Branch string        `json:"branch"`
	Total  int           `json:"total"`
	Files  []DatasetFile `json:"files"`
}

// DownloadDatasetFileResponse contains a backend-specific presigned download
// URL for a single active dataset file.
type DownloadDatasetFileResponse struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
	Method    string    `json:"method"`
}

// CreateDatasetFileUploadURLRequest asks the service to presign an upload into
// a transaction-scoped logical path.
type CreateDatasetFileUploadURLRequest struct {
	LogicalPath string  `json:"logical_path"`
	ContentType *string `json:"content_type,omitempty"`
	SHA256      *string `json:"sha256,omitempty"`
}

// CreateDatasetFileUploadURLResponse tells the caller where to PUT bytes.
type CreateDatasetFileUploadURLResponse struct {
	URL         string    `json:"url"`
	PhysicalURI string    `json:"physical_uri"`
	ExpiresAt   time.Time `json:"expires_at"`
	Method      string    `json:"method"`
}

// Wire token types preserved from the Rust serde contracts.
type TransactionStatus string
type TransactionType string
type FileOperation string
type BranchStatus string
type MarkingSource string
type SchemaFieldType string
type DatasetFileStatus string
type RetentionPolicy string

const (
	TransactionStatusOpen      TransactionStatus = "OPEN"
	TransactionStatusCommitted TransactionStatus = "COMMITTED"
	TransactionStatusAborted   TransactionStatus = "ABORTED"

	TransactionTypeSnapshot TransactionType = "SNAPSHOT"
	TransactionTypeAppend   TransactionType = "APPEND"
	TransactionTypeUpdate   TransactionType = "UPDATE"
	TransactionTypeDelete   TransactionType = "DELETE"

	FileOperationAdd     FileOperation = "ADD"
	FileOperationReplace FileOperation = "REPLACE"
	FileOperationRemove  FileOperation = "REMOVE"

	BranchStatusActive   BranchStatus = "ACTIVE"
	BranchStatusArchived BranchStatus = "ARCHIVED"
	BranchStatusDeleted  BranchStatus = "DELETED"

	MarkingSourceParent   MarkingSource = "PARENT"
	MarkingSourceExplicit MarkingSource = "EXPLICIT"

	FileFormatParquet = "PARQUET"
	FileFormatAvro    = "AVRO"
	FileFormatText    = "TEXT"

	FieldTypeBoolean   SchemaFieldType = "BOOLEAN"
	FieldTypeByte      SchemaFieldType = "BYTE"
	FieldTypeShort     SchemaFieldType = "SHORT"
	FieldTypeInteger   SchemaFieldType = "INTEGER"
	FieldTypeLong      SchemaFieldType = "LONG"
	FieldTypeFloat     SchemaFieldType = "FLOAT"
	FieldTypeDouble    SchemaFieldType = "DOUBLE"
	FieldTypeString    SchemaFieldType = "STRING"
	FieldTypeBinary    SchemaFieldType = "BINARY"
	FieldTypeDate      SchemaFieldType = "DATE"
	FieldTypeTimestamp SchemaFieldType = "TIMESTAMP"
	FieldTypeDecimal   SchemaFieldType = "DECIMAL"
	FieldTypeArray     SchemaFieldType = "ARRAY"
	FieldTypeMap       SchemaFieldType = "MAP"
	FieldTypeStruct    SchemaFieldType = "STRUCT"

	DatasetFileStatusActive  DatasetFileStatus = "active"
	DatasetFileStatusDeleted DatasetFileStatus = "deleted"

	RetentionPolicyInherited RetentionPolicy = "INHERITED"
	RetentionPolicyForever   RetentionPolicy = "FOREVER"
	RetentionPolicyTTLDays   RetentionPolicy = "TTL_DAYS"
)

// JSONValue mirrors Rust serde_json::Value / JSONB fields.
type JSONValue = json.RawMessage

// Dataset model / metadata surfaces.

// Data asset catalog dataset shape includes metadata/view-health fields from Rust data_asset_catalog/models/dataset.rs.
type CatalogDataset struct {
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Format         string     `json:"format"`
	StoragePath    string     `json:"storage_path"`
	SizeBytes      int64      `json:"size_bytes"`
	RowCount       int64      `json:"row_count"`
	OwnerID        uuid.UUID  `json:"owner_id"`
	Tags           []string   `json:"tags"`
	CurrentVersion int32      `json:"current_version"`
	ActiveBranch   string     `json:"active_branch"`
	Metadata       JSONValue  `json:"metadata"`
	HealthStatus   string     `json:"health_status"`
	CurrentViewID  *uuid.UUID `json:"current_view_id"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type CatalogCreateDatasetRequest struct {
	Name         string    `json:"name"`
	Description  *string   `json:"description"`
	Format       *string   `json:"format"`
	Tags         []string  `json:"tags"`
	Metadata     JSONValue `json:"metadata"`
	HealthStatus *string   `json:"health_status"`
}

type CatalogUpdateDatasetRequest struct {
	Name          *string    `json:"name"`
	Description   *string    `json:"description"`
	OwnerID       *uuid.UUID `json:"owner_id"`
	Tags          []string   `json:"tags"`
	Metadata      JSONValue  `json:"metadata"`
	HealthStatus  *string    `json:"health_status"`
	CurrentViewID *uuid.UUID `json:"current_view_id"`
}

type ListDatasetsQuery struct {
	Page    *int64     `json:"page"`
	PerPage *int64     `json:"per_page"`
	Search  *string    `json:"search"`
	Tag     *string    `json:"tag"`
	OwnerID *uuid.UUID `json:"owner_id"`
}

type SnapshotRequest struct {
	Message string `json:"message"`
}

type MutationRequest struct {
	BranchName     *string   `json:"branch_name"`
	Message        string    `json:"message"`
	RowDelta       *int64    `json:"row_delta"`
	SizeDeltaBytes *int64    `json:"size_delta_bytes"`
	Metadata       JSONValue `json:"metadata"`
}

type CatalogTagFacet struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

type CatalogOwnerFacet struct {
	OwnerID uuid.UUID `json:"owner_id"`
	Count   int64     `json:"count"`
}

type CatalogFacets struct {
	Tags   []CatalogTagFacet   `json:"tags"`
	Owners []CatalogOwnerFacet `json:"owners"`
}

type SchemaField struct {
	Name      string `json:"name"`
	FieldType string `json:"field_type"`
	Nullable  bool   `json:"nullable"`
}

type LegacyDatasetSchema struct {
	ID        uuid.UUID `json:"id"`
	DatasetID uuid.UUID `json:"dataset_id"`
	Fields    JSONValue `json:"fields"`
	CreatedAt time.Time `json:"created_at"`
}

type DatasetPermissionEdge struct {
	ID            uuid.UUID `json:"id"`
	DatasetID     uuid.UUID `json:"dataset_id"`
	PrincipalKind string    `json:"principal_kind"`
	PrincipalID   string    `json:"principal_id"`
	Role          string    `json:"role"`
	Actions       []string  `json:"actions"`
	Source        string    `json:"source"`
	InheritedFrom *string   `json:"inherited_from"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type DatasetLineageLink struct {
	ID           uuid.UUID `json:"id"`
	DatasetID    uuid.UUID `json:"dataset_id"`
	Direction    string    `json:"direction"`
	TargetRID    string    `json:"target_rid"`
	TargetKind   string    `json:"target_kind"`
	RelationKind string    `json:"relation_kind"`
	PipelineID   *string   `json:"pipeline_id"`
	WorkflowID   *string   `json:"workflow_id"`
	Metadata     JSONValue `json:"metadata"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type DatasetFileIndexEntry struct {
	ID           uuid.UUID  `json:"id"`
	DatasetID    uuid.UUID  `json:"dataset_id"`
	Path         string     `json:"path"`
	StoragePath  string     `json:"storage_path"`
	EntryType    string     `json:"entry_type"`
	SizeBytes    int64      `json:"size_bytes"`
	ContentType  *string    `json:"content_type"`
	Metadata     JSONValue  `json:"metadata"`
	LastModified *time.Time `json:"last_modified"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type DatasetHealthSummary struct {
	Status             string   `json:"status"`
	QualityScore       *float64 `json:"quality_score"`
	ProfileGeneratedAt *string  `json:"profile_generated_at"`
	ActiveAlertCount   int64    `json:"active_alert_count"`
	LintPosture        *string  `json:"lint_posture"`
	LintFindingCount   int64    `json:"lint_finding_count"`
}

type MarkingReason struct {
	Kind        string  `json:"kind"`
	UpstreamRID *string `json:"upstream_rid,omitempty"`
}

type EffectiveMarking struct {
	ID     uuid.UUID     `json:"id"`
	Source MarkingReason `json:"source"`
}

type DatasetRichModel struct {
	Dataset
	Schema       *LegacyDatasetSchema    `json:"schema"`
	Files        []DatasetFileIndexEntry `json:"files"`
	Branches     []DatasetBranch         `json:"branches"`
	Versions     []DatasetVersion        `json:"versions"`
	CurrentView  *DatasetView            `json:"current_view"`
	Health       DatasetHealthSummary    `json:"health"`
	Markings     []EffectiveMarking      `json:"markings"`
	Permissions  []DatasetPermissionEdge `json:"permissions"`
	LineageLinks []DatasetLineageLink    `json:"lineage_links"`
}

type DatasetMetadataPatch struct {
	Name          *string    `json:"name"`
	Description   *string    `json:"description"`
	OwnerID       *uuid.UUID `json:"owner_id"`
	Tags          []string   `json:"tags"`
	Format        *string    `json:"format"`
	Metadata      JSONValue  `json:"metadata"`
	Schema        JSONValue  `json:"schema"`
	HealthStatus  *string    `json:"health_status"`
	CurrentViewID *uuid.UUID `json:"current_view_id"`
}

type PutDatasetMarkingsRequest struct {
	Markings []uuid.UUID `json:"markings"`
}

type PutDatasetPermissionsRequest struct {
	Permissions []PutDatasetPermissionEdge `json:"permissions"`
}

type PutDatasetPermissionEdge struct {
	PrincipalKind string   `json:"principal_kind"`
	PrincipalID   string   `json:"principal_id"`
	Role          string   `json:"role"`
	Actions       []string `json:"actions"`
	Source        *string  `json:"source"`
	InheritedFrom *string  `json:"inherited_from"`
}

type PutDatasetLineageLinksRequest struct {
	Links []PutDatasetLineageLink `json:"links"`
}

type PutDatasetLineageLink struct {
	Direction    string    `json:"direction"`
	TargetRID    string    `json:"target_rid"`
	TargetKind   *string   `json:"target_kind"`
	RelationKind *string   `json:"relation_kind"`
	PipelineID   *string   `json:"pipeline_id"`
	WorkflowID   *string   `json:"workflow_id"`
	Metadata     JSONValue `json:"metadata"`
}

type PutDatasetFilesRequest struct {
	Files []PutDatasetFileIndexEntry `json:"files"`
}

type PutDatasetFileIndexEntry struct {
	Path         string     `json:"path"`
	StoragePath  string     `json:"storage_path"`
	EntryType    *string    `json:"entry_type"`
	SizeBytes    *int64     `json:"size_bytes"`
	ContentType  *string    `json:"content_type"`
	Metadata     JSONValue  `json:"metadata"`
	LastModified *time.Time `json:"last_modified"`
}

type InternalDatasetMetadata struct {
	ID             uuid.UUID   `json:"id"`
	Name           string      `json:"name"`
	Format         string      `json:"format"`
	Markings       []uuid.UUID `json:"markings"`
	Tags           []string    `json:"tags"`
	CurrentVersion int32       `json:"current_version"`
	ActiveBranch   string      `json:"active_branch"`
	OwnerID        uuid.UUID   `json:"owner_id"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// Branches, ancestry, retention, markings, fallback, and compare.
type CreateBranchBody struct {
	Name            string        `json:"name"`
	ParentBranch    *string       `json:"parent_branch"`
	FromTransaction *uuid.UUID    `json:"from_transaction"`
	Description     *string       `json:"description"`
	Source          *BranchSource `json:"source"`
	FallbackChain   []string      `json:"fallback_chain"`
	Labels          JSONValue     `json:"labels"`
}

type BranchSource struct {
	FromBranch         *string `json:"from_branch"`
	FromTransactionRID *string `json:"from_transaction_rid"`
	AsRoot             *bool   `json:"as_root"`
}

type ReparentBody struct {
	NewParentBranch *string `json:"new_parent_branch"`
}

type RuntimeBranch struct {
	ID                       uuid.UUID  `json:"id"`
	RID                      string     `json:"rid"`
	DatasetID                uuid.UUID  `json:"dataset_id"`
	DatasetRID               string     `json:"dataset_rid"`
	Name                     string     `json:"name"`
	ParentBranchID           *uuid.UUID `json:"parent_branch_id"`
	HeadTransactionID        *uuid.UUID `json:"head_transaction_id"`
	CreatedFromTransactionID *uuid.UUID `json:"created_from_transaction_id"`
	LastActivityAt           time.Time  `json:"last_activity_at"`
	Labels                   JSONValue  `json:"labels"`
	FallbackChain            []string   `json:"fallback_chain"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

type BranchDeleteChildReparent struct {
	Branch         string  `json:"branch,omitempty"`
	BranchRID      string  `json:"branch_rid,omitempty"`
	ChildBranch    string  `json:"child_branch,omitempty"`
	ChildBranchRID string  `json:"child_branch_rid,omitempty"`
	NewParent      *string `json:"new_parent"`
	NewParentRID   *string `json:"new_parent_rid"`
}

type BranchDeletePreview struct {
	Branch                string                      `json:"branch"`
	BranchRID             string                      `json:"branch_rid"`
	CurrentParent         *string                     `json:"current_parent"`
	CurrentParentRID      *string                     `json:"current_parent_rid"`
	ChildrenToReparent    []BranchDeleteChildReparent `json:"children_to_reparent"`
	TransactionsPreserved bool                        `json:"transactions_preserved"`
	HeadTransaction       any                         `json:"head_transaction"`
}

type BranchDeleteResponse struct {
	Branch     string                      `json:"branch"`
	BranchRID  string                      `json:"branch_rid"`
	Reparented []BranchDeleteChildReparent `json:"reparented"`
}

type RollbackBody struct {
	TransactionID uuid.UUID `json:"transaction_id"`
	Summary       *string   `json:"summary"`
}

type BranchAncestryResponse struct {
	Branch   RuntimeBranch   `json:"branch"`
	Ancestry []RuntimeBranch `json:"ancestry"`
}

type UpdateRetentionBody struct {
	Policy  RetentionPolicy `json:"policy"`
	TTLDays *int32          `json:"ttl_days"`
}

type RetentionRow struct {
	ID                 uuid.UUID       `json:"id"`
	ParentBranchID     *uuid.UUID      `json:"parent_branch_id"`
	Policy             RetentionPolicy `json:"policy"`
	TTLDays            *int32          `json:"ttl_days"`
	LastActivityAt     time.Time       `json:"last_activity_at"`
	HasOpenTransaction bool            `json:"has_open_transaction"`
	IsRoot             bool            `json:"is_root"`
	ArchivedAt         *time.Time      `json:"archived_at"`
}

type EffectiveRetention struct {
	Policy         RetentionPolicy `json:"policy"`
	TTLDays        *int32          `json:"ttl_days"`
	SourceBranchID *uuid.UUID      `json:"source_branch_id"`
}

type BranchMarking struct {
	BranchID  uuid.UUID     `json:"branch_id"`
	MarkingID uuid.UUID     `json:"marking_id"`
	Source    MarkingSource `json:"source"`
}

// String returns the canonical Rust SCREAMING_SNAKE_CASE label.
func (s MarkingSource) String() string { return string(s) }

type BranchMarkingsView struct {
	Effective           []uuid.UUID `json:"effective"`
	Explicit            []uuid.UUID `json:"explicit"`
	InheritedFromParent []uuid.UUID `json:"inherited_from_parent"`
}

// BranchMarkingsViewFromRows projects a snapshot row set into the API
// response shape, mirroring Rust BranchMarkingsView::from_rows. Output
// slices are sorted ascending and the effective set deduplicates ids
// that appear under both PARENT and EXPLICIT sources.
func BranchMarkingsViewFromRows(rows []BranchMarking) BranchMarkingsView {
	explicit := make(map[uuid.UUID]struct{})
	inherited := make(map[uuid.UUID]struct{})
	for _, row := range rows {
		switch row.Source {
		case MarkingSourceParent:
			inherited[row.MarkingID] = struct{}{}
		case MarkingSourceExplicit:
			explicit[row.MarkingID] = struct{}{}
		}
	}
	effective := make(map[uuid.UUID]struct{}, len(explicit)+len(inherited))
	for k := range explicit {
		effective[k] = struct{}{}
	}
	for k := range inherited {
		effective[k] = struct{}{}
	}
	return BranchMarkingsView{
		Effective:           sortedUUIDs(effective),
		Explicit:            sortedUUIDs(explicit),
		InheritedFromParent: sortedUUIDs(inherited),
	}
}

func sortedUUIDs(set map[uuid.UUID]struct{}) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sortUUIDsAscending(out)
	return out
}

type RuntimeFallbackEntry struct {
	Position           int32  `json:"position"`
	FallbackBranchName string `json:"fallback_branch_name"`
}

type PutFallbacksRequest struct {
	Fallbacks []string `json:"fallbacks"`
	Chain     []string `json:"chain"`
}

func (p PutFallbacksRequest) Names() []string {
	if p.Chain != nil {
		return p.Chain
	}
	return p.Fallbacks
}

type TransactionSummary struct {
	TransactionRID string     `json:"transaction_rid"`
	TransactionID  uuid.UUID  `json:"transaction_id"`
	Branch         string     `json:"branch"`
	TxType         string     `json:"tx_type"`
	Status         string     `json:"status"`
	CommittedAt    *time.Time `json:"committed_at"`
	FilesChanged   int        `json:"files_changed"`
}

type ConflictingFile struct {
	LogicalPath     string  `json:"logical_path"`
	ATransactionRID string  `json:"a_transaction_rid"`
	BTransactionRID string  `json:"b_transaction_rid"`
	ContentHashA    *string `json:"content_hash_a"`
	ContentHashB    *string `json:"content_hash_b"`
}

type BranchCompareResponse struct {
	BaseBranch        string               `json:"base_branch"`
	CompareBranch     string               `json:"compare_branch"`
	LCABranchRID      *string              `json:"lca_branch_rid"`
	AOnlyTransactions []TransactionSummary `json:"a_only_transactions"`
	BOnlyTransactions []TransactionSummary `json:"b_only_transactions"`
	ConflictingFiles  []ConflictingFile    `json:"conflicting_files"`
}

type BranchEnvelope struct {
	EventType          string            `json:"event_type"`
	EventID            uuid.UUID         `json:"event_id"`
	OccurredAt         time.Time         `json:"occurred_at"`
	Actor              string            `json:"actor"`
	BranchRID          string            `json:"branch_rid"`
	DatasetRID         string            `json:"dataset_rid"`
	ParentRID          *string           `json:"parent_rid"`
	IsRoot             bool              `json:"is_root"`
	HeadTransactionRID *string           `json:"head_transaction_rid"`
	FallbackChain      []string          `json:"fallback_chain"`
	Labels             map[string]string `json:"labels"`
	Markings           []uuid.UUID       `json:"markings"`
	Extras             JSONValue         `json:"extras"`
}

// Topic and event_type strings for `foundry.branch.events.v1`.
// Mirrors Rust src/domain/branch_events.rs.
const (
	BranchEventsTopic       = "foundry.branch.events.v1"
	EventBranchCreated      = "dataset.branch.created.v1"
	EventBranchReparented   = "dataset.branch.reparented.v1"
	EventBranchDeleted      = "dataset.branch.deleted.v1"
	EventBranchArchived     = "dataset.branch.archived.v1"
	EventBranchRestored     = "dataset.branch.restored.v1"
	EventBranchMarkingsSet  = "dataset.branch.markings.updated.v1"
	EventBranchRetentionSet = "dataset.branch.retention.updated.v1"
)

// NewBranchEnvelope constructs a fresh envelope. The default constructor
// leaves the parent unset, so the envelope is born as a root event;
// WithParentRID(non-nil) flips IsRoot back to false.
func NewBranchEnvelope(eventType, branchRID, datasetRID, actor string) BranchEnvelope {
	return BranchEnvelope{
		EventType:     eventType,
		EventID:       uuid.New(),
		OccurredAt:    time.Now().UTC(),
		Actor:         actor,
		BranchRID:     branchRID,
		DatasetRID:    datasetRID,
		IsRoot:        true,
		FallbackChain: []string{},
		Labels:        map[string]string{},
		Markings:      []uuid.UUID{},
		Extras:        JSONValue([]byte("{}")),
	}
}

// WithParentRID sets the parent branch RID and recomputes IsRoot.
func (e BranchEnvelope) WithParentRID(parentRID *string) BranchEnvelope {
	e.IsRoot = parentRID == nil
	e.ParentRID = parentRID
	return e
}

// WithHead sets the head transaction RID.
func (e BranchEnvelope) WithHead(headTransactionRID *string) BranchEnvelope {
	e.HeadTransactionRID = headTransactionRID
	return e
}

// WithFallback sets the fallback chain.
func (e BranchEnvelope) WithFallback(chain []string) BranchEnvelope {
	if chain == nil {
		chain = []string{}
	}
	e.FallbackChain = chain
	return e
}

// WithLabels sets the free-form labels map.
func (e BranchEnvelope) WithLabels(labels map[string]string) BranchEnvelope {
	if labels == nil {
		labels = map[string]string{}
	}
	e.Labels = labels
	return e
}

// WithMarkings sets the marking ids attached to the event.
func (e BranchEnvelope) WithMarkings(markings []uuid.UUID) BranchEnvelope {
	if markings == nil {
		markings = []uuid.UUID{}
	}
	e.Markings = markings
	return e
}

// WithExtras attaches event-type-specific extras as a raw JSON object.
func (e BranchEnvelope) WithExtras(extras JSONValue) BranchEnvelope {
	if len(extras) == 0 {
		extras = JSONValue([]byte("{}"))
	}
	e.Extras = extras
	return e
}

// Payload renders the envelope as a JSON object (Rust into_payload).
// On any encoding error the function falls back to "{}" so that the
// outbox row never carries a malformed payload — same defensive choice
// as the Rust implementation.
func (e BranchEnvelope) Payload() json.RawMessage {
	raw, err := json.Marshal(e)
	if err != nil {
		return json.RawMessage([]byte("{}"))
	}
	return raw
}

// Transactions and 207 batch envelopes.
type StartTransactionBody struct {
	Type       TransactionType `json:"type"`
	Providence JSONValue       `json:"providence"`
	Summary    *string         `json:"summary"`
}

type ListTxQuery struct {
	Branch *string `json:"branch"`
	Before *string `json:"before"`
}

type RuntimeTransaction struct {
	ID          uuid.UUID         `json:"id"`
	DatasetID   uuid.UUID         `json:"dataset_id"`
	BranchID    uuid.UUID         `json:"branch_id"`
	BranchName  string            `json:"branch_name"`
	TxType      TransactionType   `json:"tx_type"`
	Status      TransactionStatus `json:"status"`
	Summary     string            `json:"summary"`
	Metadata    JSONValue         `json:"metadata"`
	Providence  JSONValue         `json:"providence"`
	StartedBy   *uuid.UUID        `json:"started_by"`
	StartedAt   time.Time         `json:"started_at"`
	CommittedAt *time.Time        `json:"committed_at"`
	AbortedAt   *time.Time        `json:"aborted_at"`
}

type DatasetTransaction struct {
	ID          uuid.UUID  `json:"id"`
	DatasetID   uuid.UUID  `json:"dataset_id"`
	ViewID      *uuid.UUID `json:"view_id"`
	Operation   string     `json:"operation"`
	BranchName  *string    `json:"branch_name"`
	Status      string     `json:"status"`
	Summary     string     `json:"summary"`
	Metadata    JSONValue  `json:"metadata"`
	CreatedAt   time.Time  `json:"created_at"`
	CommittedAt *time.Time `json:"committed_at"`
}

type TransactionRecord struct {
	ViewID     *uuid.UUID `json:"view_id"`
	Operation  string     `json:"operation"`
	BranchName *string    `json:"branch_name"`
	Summary    string     `json:"summary"`
	Metadata   JSONValue  `json:"metadata"`
}

type BatchGetTransactionsRequest struct {
	IDs []string `json:"ids"`
}

type BatchItemResult[T any] struct {
	ID     string  `json:"id"`
	Status int     `json:"status"`
	Data   *T      `json:"data,omitempty"`
	Error  *string `json:"error,omitempty"`
}

type ErrorEnvelope struct {
	Error      string  `json:"error"`
	Code       *string `json:"code,omitempty"`
	RetryAfter *int64  `json:"retry_after,omitempty"`
}

// Views, files, schema, preview, and data envelopes.
type DatasetView struct {
	ID                    uuid.UUID  `json:"id"`
	DatasetID             uuid.UUID  `json:"dataset_id"`
	Name                  string     `json:"name"`
	Description           string     `json:"description"`
	SQLText               string     `json:"sql_text"`
	SourceBranch          *string    `json:"source_branch"`
	SourceVersion         *int32     `json:"source_version"`
	Materialized          bool       `json:"materialized"`
	RefreshOnSourceUpdate bool       `json:"refresh_on_source_update"`
	Format                string     `json:"format"`
	CurrentVersion        int32      `json:"current_version"`
	StoragePath           *string    `json:"storage_path"`
	RowCount              int64      `json:"row_count"`
	SchemaFields          JSONValue  `json:"schema_fields"`
	LastRefreshedAt       *time.Time `json:"last_refreshed_at"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type CreateDatasetViewRequest struct {
	Name                  string  `json:"name"`
	Description           *string `json:"description"`
	SQL                   string  `json:"sql"`
	SourceBranch          *string `json:"source_branch"`
	SourceVersion         *int32  `json:"source_version"`
	Materialized          *bool   `json:"materialized"`
	RefreshOnSourceUpdate *bool   `json:"refresh_on_source_update"`
}

type ViewAtQuery struct {
	TS            *string    `json:"ts"`
	TransactionID *uuid.UUID `json:"transaction_id"`
	Branch        string     `json:"branch"`
}

type RuntimeViewFile struct {
	LogicalPath  string     `json:"logical_path"`
	PhysicalPath string     `json:"physical_path"`
	SizeBytes    int64      `json:"size_bytes"`
	IntroducedBy *uuid.UUID `json:"introduced_by"`
}

type ViewOut struct {
	ID                uuid.UUID         `json:"id"`
	DatasetID         uuid.UUID         `json:"dataset_id"`
	BranchID          uuid.UUID         `json:"branch_id"`
	HeadTransactionID uuid.UUID         `json:"head_transaction_id"`
	RequestedBranch   string            `json:"requested_branch"`
	ResolvedBranch    string            `json:"resolved_branch"`
	FallbackChain     []string          `json:"fallback_chain"`
	ComputedAt        time.Time         `json:"computed_at"`
	FileCount         int32             `json:"file_count"`
	SizeBytes         int64             `json:"size_bytes"`
	Files             []RuntimeViewFile `json:"files"`
}

type FileDiff struct {
	Added    []RuntimeViewFile `json:"added"`
	Removed  []RuntimeViewFile `json:"removed"`
	Modified []FileChange      `json:"modified"`
}

type FileChange struct {
	LogicalPath string          `json:"logical_path"`
	Before      RuntimeViewFile `json:"before"`
	After       RuntimeViewFile `json:"after"`
}

type CompareOut struct {
	Base   ViewOut  `json:"base"`
	Target ViewOut  `json:"target"`
	Files  FileDiff `json:"files"`
}

type ViewPreviewQuery struct {
	Limit  *int64 `json:"limit"`
	Offset *int64 `json:"offset"`
}

type PreviewQuery struct {
	Limit              *int    `json:"limit"`
	Offset             *int    `json:"offset"`
	Format             *string `json:"format"`
	CSVDelimiter       *string `json:"csv_delimiter"`
	CSVQuote           *string `json:"csv_quote"`
	CSVEscape          *string `json:"csv_escape"`
	CSVHeader          *bool   `json:"csv_header"`
	CSVNullValue       *string `json:"csv_null_value"`
	CSVCharset         *string `json:"csv_charset"`
	CSVDateFormat      *string `json:"csv_date_format"`
	CSVTimestampFormat *string `json:"csv_timestamp_format"`
	CSV                *bool   `json:"csv"`
}

type PreviewDataResponse struct {
	Columns []string      `json:"columns"`
	Rows    [][]JSONValue `json:"rows"`
	Format  string        `json:"format"`
	Limit   int           `json:"limit"`
	Offset  int           `json:"offset"`
}

// Backing filesystem / file upload and download surfaces.
type ListFilesQuery struct {
	Branch *string    `json:"branch"`
	ViewID *uuid.UUID `json:"view_id"`
	Prefix *string    `json:"prefix"`
}

type DatasetFileOut struct {
	ID            uuid.UUID         `json:"id"`
	DatasetID     uuid.UUID         `json:"dataset_id"`
	TransactionID uuid.UUID         `json:"transaction_id"`
	LogicalPath   string            `json:"logical_path"`
	PhysicalURI   string            `json:"physical_uri"`
	SizeBytes     int64             `json:"size_bytes"`
	SHA256        *string           `json:"sha256"`
	CreatedAt     time.Time         `json:"created_at"`
	ModifiedAt    time.Time         `json:"modified_at"`
	Status        DatasetFileStatus `json:"status"`
}

type ListFilesOut struct {
	ViewID *uuid.UUID       `json:"view_id"`
	Branch string           `json:"branch"`
	Total  int              `json:"total"`
	Files  []DatasetFileOut `json:"files"`
}

type UploadUrlBody struct {
	LogicalPath string  `json:"logical_path"`
	ContentType *string `json:"content_type"`
	SHA256      *string `json:"sha256"`
}

type UploadUrlOut struct {
	URL         string    `json:"url"`
	PhysicalURI string    `json:"physical_uri"`
	ExpiresAt   time.Time `json:"expires_at"`
	Method      string    `json:"method"`
}

type LocalProxyQuery struct {
	Expires int64  `json:"expires"`
	Sig     string `json:"sig"`
}

type StorageDetailsOut struct {
	FSID              string `json:"fs_id"`
	Driver            string `json:"driver"`
	BaseDirectory     string `json:"base_directory"`
	PresignTTLSeconds uint64 `json:"presign_ttl_seconds"`
	TotalActiveBytes  int64  `json:"total_active_bytes"`
	TotalActiveFiles  int64  `json:"total_active_files"`
	TotalDeletedBytes int64  `json:"total_deleted_bytes"`
	TotalDeletedFiles int64  `json:"total_deleted_files"`
}

// Foundry schema wire model. Field flattens the Rust tagged enum onto each field.
type Field struct {
	Name         string          `json:"name"`
	Type         SchemaFieldType `json:"type"`
	Precision    *uint8          `json:"precision,omitempty"`
	Scale        *uint8          `json:"scale,omitempty"`
	ArraySubType *Field          `json:"arraySubType,omitempty"`
	MapKeyType   *Field          `json:"mapKeyType,omitempty"`
	MapValueType *Field          `json:"mapValueType,omitempty"`
	SubSchemas   []Field         `json:"subSchemas,omitempty"`
	Nullable     bool            `json:"nullable"`
	Description  *string         `json:"description,omitempty"`
}

type CsvOptions struct {
	Delimiter       string  `json:"delimiter"`
	Quote           string  `json:"quote"`
	Escape          string  `json:"escape"`
	Header          bool    `json:"header"`
	NullValue       string  `json:"nullValue"`
	DateFormat      *string `json:"dateFormat,omitempty"`
	TimestampFormat *string `json:"timestampFormat,omitempty"`
	Charset         string  `json:"charset"`
}

type CustomMetadata struct {
	CSV *CsvOptions `json:"csv,omitempty"`
}

type DatasetSchema struct {
	Fields         []Field         `json:"fields"`
	FileFormat     string          `json:"file_format"`
	CustomMetadata *CustomMetadata `json:"custom_metadata,omitempty"`
}

type SchemaResponse struct {
	ViewID      uuid.UUID     `json:"view_id"`
	DatasetID   uuid.UUID     `json:"dataset_id"`
	Branch      *string       `json:"branch"`
	Schema      DatasetSchema `json:"schema"`
	ContentHash string        `json:"content_hash"`
	CreatedAt   time.Time     `json:"created_at"`
	Unchanged   bool          `json:"unchanged,omitempty"`
}

type PutSchemaBody struct {
	Schema DatasetSchema `json:"schema"`
}

type ValidateRequest struct {
	Schema DatasetSchema `json:"schema"`
}

type FileValidationReport struct {
	Path      string            `json:"path"`
	Format    string            `json:"format"`
	SizeBytes int64             `json:"size_bytes"`
	Conforms  bool              `json:"conforms"`
	Errors    []FileSchemaError `json:"errors"`
}

type FileSchemaError struct {
	Field   string `json:"field"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type ValidateResponse struct {
	Conforms     bool                   `json:"conforms"`
	Files        []FileValidationReport `json:"files"`
	SchemaErrors []string               `json:"schema_errors"`
}

// Quality, lint, and health models.
type DatasetValueCount struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

type DatasetColumnProfile struct {
	Name           string              `json:"name"`
	FieldType      string              `json:"field_type"`
	Nullable       bool                `json:"nullable"`
	NullCount      int64               `json:"null_count"`
	NullRate       float64             `json:"null_rate"`
	DistinctCount  int64               `json:"distinct_count"`
	UniquenessRate float64             `json:"uniqueness_rate"`
	SampleValues   []DatasetValueCount `json:"sample_values"`
	MinValue       *string             `json:"min_value"`
	MaxValue       *string             `json:"max_value"`
	AverageValue   *float64            `json:"average_value"`
}

type DatasetRuleResult struct {
	RuleID        uuid.UUID `json:"rule_id"`
	Name          string    `json:"name"`
	RuleType      string    `json:"rule_type"`
	Severity      string    `json:"severity"`
	Passed        bool      `json:"passed"`
	MeasuredValue *string   `json:"measured_value"`
	Message       string    `json:"message"`
}

type DatasetQualityProfile struct {
	RowCount          int64                  `json:"row_count"`
	ColumnCount       int64                  `json:"column_count"`
	DuplicateRows     int64                  `json:"duplicate_rows"`
	CompletenessRatio float64                `json:"completeness_ratio"`
	UniquenessRatio   float64                `json:"uniqueness_ratio"`
	GeneratedAt       time.Time              `json:"generated_at"`
	Columns           []DatasetColumnProfile `json:"columns"`
	RuleResults       []DatasetRuleResult    `json:"rule_results"`
}

type DatasetQualityRule struct {
	ID        uuid.UUID `json:"id"`
	DatasetID uuid.UUID `json:"dataset_id"`
	Name      string    `json:"name"`
	RuleType  string    `json:"rule_type"`
	Severity  string    `json:"severity"`
	Config    JSONValue `json:"config"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DatasetQualityHistoryEntry struct {
	ID          uuid.UUID `json:"id"`
	DatasetID   uuid.UUID `json:"dataset_id"`
	Score       float64   `json:"score"`
	PassedRules int32     `json:"passed_rules"`
	FailedRules int32     `json:"failed_rules"`
	AlertsCount int32     `json:"alerts_count"`
	CreatedAt   time.Time `json:"created_at"`
}

type DatasetQualityAlert struct {
	ID         uuid.UUID  `json:"id"`
	DatasetID  uuid.UUID  `json:"dataset_id"`
	Level      string     `json:"level"`
	Kind       string     `json:"kind"`
	Message    string     `json:"message"`
	Status     string     `json:"status"`
	Details    JSONValue  `json:"details"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at"`
}

type CreateQualityRuleRequest struct {
	Name     string    `json:"name"`
	RuleType string    `json:"rule_type"`
	Severity *string   `json:"severity"`
	Enabled  *bool     `json:"enabled"`
	Config   JSONValue `json:"config"`
}

type UpdateQualityRuleRequest struct {
	Name     *string   `json:"name"`
	Severity *string   `json:"severity"`
	Enabled  *bool     `json:"enabled"`
	Config   JSONValue `json:"config"`
}

type DatasetQualityResponse struct {
	Profile    *DatasetQualityProfile       `json:"profile"`
	Score      *float64                     `json:"score"`
	History    []DatasetQualityHistoryEntry `json:"history"`
	Alerts     []DatasetQualityAlert        `json:"alerts"`
	Rules      []DatasetQualityRule         `json:"rules"`
	ProfiledAt *time.Time                   `json:"profiled_at"`
}

type DatasetHealth struct {
	DatasetRID        string             `json:"dataset_rid"`
	DatasetID         *uuid.UUID         `json:"dataset_id"`
	RowCount          int64              `json:"row_count"`
	ColCount          int32              `json:"col_count"`
	NullPctByColumn   map[string]float64 `json:"null_pct_by_column"`
	FreshnessSeconds  int64              `json:"freshness_seconds"`
	LastCommitAt      *time.Time         `json:"last_commit_at"`
	TxnFailureRate24H float64            `json:"txn_failure_rate_24h"`
	LastBuildStatus   string             `json:"last_build_status"`
	SchemaDriftFlag   bool               `json:"schema_drift_flag"`
	Extras            JSONValue          `json:"extras"`
	LastComputedAt    time.Time          `json:"last_computed_at"`
}

type DatasetHealthPolicy struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	DatasetRID  *string   `json:"dataset_rid"`
	AllDatasets bool      `json:"all_datasets"`
	CheckKind   string    `json:"check_kind"`
	Threshold   JSONValue `json:"threshold"`
	Severity    string    `json:"severity"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateHealthPolicyRequest struct {
	Name        string    `json:"name"`
	DatasetRID  *string   `json:"dataset_rid"`
	AllDatasets bool      `json:"all_datasets"`
	CheckKind   string    `json:"check_kind"`
	Threshold   JSONValue `json:"threshold"`
	Severity    string    `json:"severity"`
	Active      bool      `json:"active"`
}

type DatasetLintSummary struct {
	ResourcePosture         string   `json:"resource_posture"`
	TotalFindings           int      `json:"total_findings"`
	HighSeverity            int      `json:"high_severity"`
	MediumSeverity          int      `json:"medium_severity"`
	LowSeverity             int      `json:"low_severity"`
	TrackedVersions         int      `json:"tracked_versions"`
	BranchCount             int      `json:"branch_count"`
	StaleBranchCount        int      `json:"stale_branch_count"`
	MaterializedViewCount   int      `json:"materialized_view_count"`
	AutoRefreshViewCount    int      `json:"auto_refresh_view_count"`
	TransactionCount        int      `json:"transaction_count"`
	FailedTransactionCount  int      `json:"failed_transaction_count"`
	PendingTransactionCount int      `json:"pending_transaction_count"`
	EnabledRuleCount        int      `json:"enabled_rule_count"`
	ActiveAlertCount        int      `json:"active_alert_count"`
	ObjectCount             int      `json:"object_count"`
	SmallFileCount          int      `json:"small_file_count"`
	LargestObjectBytes      int64    `json:"largest_object_bytes"`
	AverageObjectSizeBytes  int64    `json:"average_object_size_bytes"`
	QualityScore            *float64 `json:"quality_score"`
}

type DatasetLintFinding struct {
	Code           string   `json:"code"`
	Title          string   `json:"title"`
	Severity       string   `json:"severity"`
	Category       string   `json:"category"`
	Description    string   `json:"description"`
	Evidence       []string `json:"evidence"`
	Impact         string   `json:"impact"`
	Recommendation string   `json:"recommendation"`
}

type DatasetLintRecommendation struct {
	Code      string   `json:"code"`
	Priority  string   `json:"priority"`
	Title     string   `json:"title"`
	Rationale string   `json:"rationale"`
	Actions   []string `json:"actions"`
}

type DatasetLintResponse struct {
	DatasetID       uuid.UUID                   `json:"dataset_id"`
	DatasetName     string                      `json:"dataset_name"`
	AnalyzedAt      time.Time                   `json:"analyzed_at"`
	Summary         DatasetLintSummary          `json:"summary"`
	Findings        []DatasetLintFinding        `json:"findings"`
	Recommendations []DatasetLintRecommendation `json:"recommendations"`
}

// Retention worker, Iceberg/catalog, and write payloads.
type RetentionWorkerResult struct {
	ArchivedBranches int              `json:"archived_branches"`
	Archived         []BranchEnvelope `json:"archived"`
	RanAt            time.Time        `json:"ran_at"`
}

type UnionViewSpec struct {
	UnionViewDatasetRID string   `json:"union_view_dataset_rid"`
	OutputDatasetRIDs   []string `json:"output_dataset_rids"`
	DeploymentKeyParam  string   `json:"deployment_key_param"`
}

type PreparedDatasetQuery struct {
	Path string `json:"path"`
}

type ResolvedDatasetSource struct {
	Dataset     Dataset `json:"dataset"`
	Branch      *string `json:"branch"`
	Version     int32   `json:"version"`
	SizeBytes   int64   `json:"size_bytes"`
	StoragePath string  `json:"storage_path"`
}

type RuntimeTransactionFile struct {
	LogicalPath  string        `json:"logical_path"`
	PhysicalPath string        `json:"physical_path"`
	SizeBytes    int64         `json:"size_bytes"`
	Op           FileOperation `json:"op"`
}

type StagedFile struct {
	LogicalPath  string        `json:"logical_path"`
	PhysicalPath string        `json:"physical_path"`
	SizeBytes    int64         `json:"size_bytes"`
	Op           FileOperation `json:"op"`
}

type OpenTransaction struct {
	ID        uuid.UUID       `json:"id"`
	DatasetID uuid.UUID       `json:"dataset_id"`
	BranchID  uuid.UUID       `json:"branch_id"`
	Kind      TransactionType `json:"kind"`
}

type IcebergTableRef struct {
	Namespace string `json:"namespace"`
	Table     string `json:"table"`
	URI       string `json:"uri"`
}

type IcebergWritePayload struct {
	DatasetRID    string                   `json:"dataset_rid"`
	Branch        string                   `json:"branch"`
	TransactionID uuid.UUID                `json:"transaction_id"`
	Operation     TransactionType          `json:"operation"`
	Table         IcebergTableRef          `json:"table"`
	Files         []RuntimeTransactionFile `json:"files"`
	Metadata      JSONValue                `json:"metadata"`
}

func MarshalJSONValue(v any) (JSONValue, error) {
	return json.Marshal(v)
}

func UnmarshalJSONValue(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
