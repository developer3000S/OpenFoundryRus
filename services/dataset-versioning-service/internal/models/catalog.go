// Package models — DV-12 catalog metadata, markings, lineage helpers ported
// from the Rust data_asset_catalog module. This file collects the public
// constants, paged envelopes, and pure helpers that surround the existing
// catalog wire types in models.go (CatalogDataset, EffectiveMarking,
// DatasetPermissionEdge, DatasetLineageLink, DatasetFileIndexEntry, etc.).
//
// Behaviour is byte-compatible with the Rust contracts so the Foundry UI and
// downstream services can consume either implementation without translation.
package models

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// Catalog enumerations
//
// Each block mirrors a serde-tagged Rust enum stored as plain text columns in
// the data-asset-catalog tables. Centralising the strings here keeps handlers
// and repo code free of magic literals.
// ---------------------------------------------------------------------------

// MarkingSourceKind values stored in dataset_markings.source. Mirrors Rust
// core_models::security::MarkingSource which serialises into one of "direct"
// or "inherited_from_upstream".
const (
	MarkingSourceKindDirect                = "direct"
	MarkingSourceKindInheritedFromUpstream = "inherited_from_upstream"
)

// PermissionSource values for dataset_permission_edges.source.
const (
	PermissionSourceDirect               = "direct"
	PermissionSourceInheritedFromProject = "inherited_from_project"
	PermissionSourceInheritedFromFolder  = "inherited_from_folder"
	PermissionSourceInheritedFromParent  = "inherited_from_parent"
)

// PrincipalKind values for dataset_permission_edges.principal_kind.
const (
	PrincipalKindUser         = "user"
	PrincipalKindGroup        = "group"
	PrincipalKindRole         = "role"
	PrincipalKindOrganization = "organization"
	PrincipalKindProject      = "project"
	PrincipalKindService      = "service"
)

// LineageDirection values for dataset_lineage_links.direction.
const (
	LineageDirectionUpstream   = "upstream"
	LineageDirectionDownstream = "downstream"
)

// LineageRelationKind defaults applied when a lineage link omits relation_kind.
const (
	LineageRelationDerivesFrom = "derives_from"
	LineageRelationConsumedBy  = "consumed_by"
	LineageRelationFeeds       = "feeds"
)

// LineageTargetKind defaults applied when a lineage link omits target_kind.
const (
	LineageTargetKindDataset  = "dataset"
	LineageTargetKindObject   = "object"
	LineageTargetKindModel    = "model"
	LineageTargetKindWorkflow = "workflow"
)

// FileEntryType values for dataset_file_index.entry_type.
const (
	FileEntryTypeFile      = "file"
	FileEntryTypeDirectory = "directory"
)

// HealthStatus values that can be assigned to datasets.health_status.
const (
	HealthStatusUnknown  = "unknown"
	HealthStatusHealthy  = "healthy"
	HealthStatusWarning  = "warning"
	HealthStatusDegraded = "degraded"
	HealthStatusCritical = "critical"
)

// CatalogDatasetFormat values that can be assigned to datasets.format.
const (
	DatasetFormatParquet = "parquet"
	DatasetFormatAvro    = "avro"
	DatasetFormatCSV     = "csv"
	DatasetFormatJSON    = "json"
	DatasetFormatText    = "text"
	DatasetFormatUnknown = "unknown"
)

// ResourceVisibility values stored on browseable dataset resources.
const (
	ResourceVisibilityPrivate      = "private"
	ResourceVisibilityShared       = "shared"
	ResourceVisibilityOrganization = "organization"
	ResourceVisibilityPublic       = "public"
)

// CatalogAuditAction string constants — match Rust emit_audit() invocations.
const (
	AuditActionDatasetMetadataUpdate     = "dataset.metadata.update"
	AuditActionDatasetMarkingsReplace    = "dataset.markings.replace"
	AuditActionDatasetPermissionsReplace = "dataset.permissions.replace"
	AuditActionDatasetLineageReplace     = "dataset.lineage_links.replace"
	AuditActionDatasetFilesReplace       = "dataset.files.replace"
)

// ---------------------------------------------------------------------------
// EffectiveMarking helpers
// ---------------------------------------------------------------------------

// NewDirectMarking returns an EffectiveMarking sourced from the dataset
// itself (matches Rust EffectiveMarking::direct).
func NewDirectMarking(id uuid.UUID) EffectiveMarking {
	return EffectiveMarking{ID: id, Source: MarkingReason{Kind: MarkingSourceKindDirect}}
}

// NewInheritedMarking returns an EffectiveMarking inherited from an upstream
// dataset's RID. The closest hop is what the UI displays, mirroring the
// Rust MarkingSource::InheritedFromUpstream variant.
func NewInheritedMarking(id uuid.UUID, upstreamRID string) EffectiveMarking {
	rid := upstreamRID
	return EffectiveMarking{ID: id, Source: MarkingReason{Kind: MarkingSourceKindInheritedFromUpstream, UpstreamRID: &rid}}
}

// IsDirect reports whether the marking originates on the dataset itself.
func (m EffectiveMarking) IsDirect() bool { return m.Source.Kind == MarkingSourceKindDirect }

// IsInherited reports whether the marking flows in from upstream lineage.
func (m EffectiveMarking) IsInherited() bool {
	return m.Source.Kind == MarkingSourceKindInheritedFromUpstream
}

// UpstreamRID returns the parent RID for an inherited marking, or the empty
// string for direct markings. Convenience wrapper around the optional field.
func (m EffectiveMarking) UpstreamRID() string {
	if m.Source.UpstreamRID == nil {
		return ""
	}
	return *m.Source.UpstreamRID
}

// DedupeMarkings collapses (id, source) duplicates while preserving the first
// occurrence so direct markings precede inherited ones — matches the Rust
// dedupe() helper in data_asset_catalog::domain::markings.
func DedupeMarkings(markings []EffectiveMarking) []EffectiveMarking {
	type key struct {
		id  uuid.UUID
		src string
		up  string
	}
	seen := make(map[key]struct{}, len(markings))
	out := make([]EffectiveMarking, 0, len(markings))
	for _, m := range markings {
		k := key{id: m.ID, src: m.Source.Kind}
		if m.Source.UpstreamRID != nil {
			k.up = *m.Source.UpstreamRID
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, m)
	}
	return out
}

// ReTagAsInheritedFrom returns a fresh slice where each input marking's
// source becomes "inherited_from_upstream" tagged with parentRID. Used by the
// resolver to project an upstream dataset's effective markings onto the child.
func ReTagAsInheritedFrom(parentRID string, markings []EffectiveMarking) []EffectiveMarking {
	out := make([]EffectiveMarking, len(markings))
	for i, m := range markings {
		out[i] = NewInheritedMarking(m.ID, parentRID)
	}
	return out
}

// ---------------------------------------------------------------------------
// Catalog list pagination envelope
// ---------------------------------------------------------------------------

// PagedDatasets mirrors the JSON envelope returned by Rust list_datasets:
// `{ data, page, per_page, total, total_pages }`. Used by the catalog
// pagination endpoint instead of the cursor-based Page[T] used by versions.
type PagedDatasets struct {
	Data       []CatalogDataset `json:"data"`
	Page       int64            `json:"page"`
	PerPage    int64            `json:"per_page"`
	Total      int64            `json:"total"`
	TotalPages int64            `json:"total_pages"`
}

// NormalisePageInputs applies the same bounds as Rust list_datasets:
// per_page is clamped to [1, 100], page is floored to 1. The defaults
// (page=1, per_page=20) match the Rust implementation byte-for-byte.
func NormalisePageInputs(page, perPage *int64) (int64, int64) {
	p := int64(1)
	if page != nil && *page > 1 {
		p = *page
	}
	pp := int64(20)
	if perPage != nil {
		pp = *perPage
	}
	if pp < 1 {
		pp = 1
	} else if pp > 100 {
		pp = 100
	}
	return p, pp
}

// TotalPages computes the page count for `total` rows at `perPage`.
// Matches the Rust ceil(total/per_page) projection.
func TotalPages(total, perPage int64) int64 {
	if perPage <= 0 {
		return 0
	}
	if total <= 0 {
		return 0
	}
	return (total + perPage - 1) / perPage
}

// ---------------------------------------------------------------------------
// MarkingResolveError — wire-stable error taxonomy for the resolver port
// ---------------------------------------------------------------------------

// MarkingResolveErrorKind classifies a MarkingResolveError. Mirrors the Rust
// thiserror enum so callers can branch on cause without parsing strings.
type MarkingResolveErrorKind string

const (
	MarkingResolveErrorKindDatabase MarkingResolveErrorKind = "database"
	MarkingResolveErrorKindLineage  MarkingResolveErrorKind = "lineage"
	MarkingResolveErrorKindCycle    MarkingResolveErrorKind = "cycle"
)

// MarkingResolveError reports failures while computing effective markings.
// The Kind field is the Rust enum discriminant; RID is set for lineage/cycle
// variants; Cause is the underlying error.
type MarkingResolveError struct {
	Kind  MarkingResolveErrorKind
	RID   string
	Cause error
}

// Error renders the human-readable message. Format matches Rust's
// `#[error]` output so log scrapers can be reused.
func (e *MarkingResolveError) Error() string {
	switch e.Kind {
	case MarkingResolveErrorKindCycle:
		return "lineage cycle detected at " + e.RID
	case MarkingResolveErrorKindLineage:
		return "lineage lookup failed for " + e.RID + ": " + safeCauseMessage(e.Cause)
	case MarkingResolveErrorKindDatabase:
		return "database error: " + safeCauseMessage(e.Cause)
	default:
		return "marking resolve error: " + safeCauseMessage(e.Cause)
	}
}

// Unwrap exposes the underlying error so errors.Is/As traversal works.
func (e *MarkingResolveError) Unwrap() error { return e.Cause }

// IsCycle is true for cycle-detection errors.
func (e *MarkingResolveError) IsCycle() bool { return e.Kind == MarkingResolveErrorKindCycle }

// NewMarkingDatabaseError wraps a DB error with the "database" classification.
func NewMarkingDatabaseError(cause error) *MarkingResolveError {
	return &MarkingResolveError{Kind: MarkingResolveErrorKindDatabase, Cause: cause}
}

// NewMarkingLineageError wraps a lineage-client error tagged with the RID
// the resolver was working on at the time.
func NewMarkingLineageError(rid string, cause error) *MarkingResolveError {
	return &MarkingResolveError{Kind: MarkingResolveErrorKindLineage, RID: rid, Cause: cause}
}

// NewMarkingCycleError flags a cycle in the lineage graph at `rid`.
func NewMarkingCycleError(rid string) *MarkingResolveError {
	return &MarkingResolveError{Kind: MarkingResolveErrorKindCycle, RID: rid}
}

func safeCauseMessage(cause error) string {
	if cause == nil {
		return "(no cause)"
	}
	return cause.Error()
}

// ErrMarkingResolverDisabled signals that the resolver was asked to run but
// no LineageClient was wired. Callers may fall back to direct-only markings.
var ErrMarkingResolverDisabled = errors.New("marking resolver: lineage client not configured")

// ---------------------------------------------------------------------------
// Validation enum membership helpers (lowercase comparisons)
// ---------------------------------------------------------------------------

// IsKnownMarkingSource reports whether `s` is one of the documented values.
func IsKnownMarkingSource(s string) bool {
	switch strings.ToLower(s) {
	case MarkingSourceKindDirect, MarkingSourceKindInheritedFromUpstream:
		return true
	default:
		return false
	}
}

// IsKnownPrincipalKind reports whether `s` is one of the documented values.
func IsKnownPrincipalKind(s string) bool {
	switch strings.ToLower(s) {
	case PrincipalKindUser, PrincipalKindGroup, PrincipalKindRole,
		PrincipalKindOrganization, PrincipalKindProject, PrincipalKindService:
		return true
	default:
		return false
	}
}

// IsKnownPermissionSource reports whether `s` is one of the documented values.
func IsKnownPermissionSource(s string) bool {
	switch strings.ToLower(s) {
	case PermissionSourceDirect,
		PermissionSourceInheritedFromProject,
		PermissionSourceInheritedFromFolder,
		PermissionSourceInheritedFromParent:
		return true
	default:
		return false
	}
}

// IsKnownLineageDirection reports whether `s` is "upstream" or "downstream".
func IsKnownLineageDirection(s string) bool {
	switch strings.ToLower(s) {
	case LineageDirectionUpstream, LineageDirectionDownstream:
		return true
	default:
		return false
	}
}

// IsKnownFileEntryType reports whether `s` is "file" or "directory".
func IsKnownFileEntryType(s string) bool {
	switch strings.ToLower(s) {
	case FileEntryTypeFile, FileEntryTypeDirectory:
		return true
	default:
		return false
	}
}

// IsKnownHealthStatus reports whether `s` matches one of the documented
// dataset health-state strings.
func IsKnownHealthStatus(s string) bool {
	switch strings.ToLower(s) {
	case HealthStatusUnknown, HealthStatusHealthy, HealthStatusWarning,
		HealthStatusDegraded, HealthStatusCritical:
		return true
	default:
		return false
	}
}

// IsKnownResourceVisibility reports whether `s` matches one of the
// browseable dataset visibility states.
func IsKnownResourceVisibility(s string) bool {
	switch strings.ToLower(s) {
	case ResourceVisibilityPrivate, ResourceVisibilityShared,
		ResourceVisibilityOrganization, ResourceVisibilityPublic:
		return true
	default:
		return false
	}
}

// IsKnownDatasetFormat reports whether `s` matches one of the documented
// dataset format identifiers.
func IsKnownDatasetFormat(s string) bool {
	switch strings.ToLower(s) {
	case DatasetFormatParquet, DatasetFormatAvro, DatasetFormatCSV,
		DatasetFormatJSON, DatasetFormatText, DatasetFormatUnknown:
		return true
	default:
		return false
	}
}
