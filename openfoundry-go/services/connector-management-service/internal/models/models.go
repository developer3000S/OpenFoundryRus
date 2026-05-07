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
