package audittrail

// EventKind is the wire token in the `kind` field of every audit
// event. Stable so SIEM filters key off it; never rename.
type EventKind string

const (
	KindMediaSetCreated              EventKind = "media_set.created"
	KindMediaSetDeleted              EventKind = "media_set.deleted"
	KindMediaSetMarkingsChanged      EventKind = "media_set.markings_changed"
	KindMediaSetRetentionChanged     EventKind = "media_set.retention_changed"
	KindMediaSetTransactionOpened    EventKind = "media_set.transaction_opened"
	KindMediaSetTransactionCommitted EventKind = "media_set.transaction_committed"
	KindMediaSetTransactionAborted   EventKind = "media_set.transaction_aborted"
	KindMediaSetAccessPatternInvoked EventKind = "media_set.access_pattern_invoked"
	KindMediaItemUploaded            EventKind = "media_item.uploaded"
	KindMediaItemDownloaded          EventKind = "media_item.downloaded"
	KindMediaItemDeleted             EventKind = "media_item.deleted"
	KindMediaItemMarkingOverridden   EventKind = "media_item.marking_overridden"
	KindVirtualMediaItemRegistered   EventKind = "virtual_media_item.registered"
)

// CategoriesFor returns the audit categories assigned to `kind`. Mirrors
// the Rust `AuditEvent::categories` mapping verbatim.
func CategoriesFor(kind EventKind) []AuditCategory {
	switch kind {
	case KindMediaSetCreated:
		return []AuditCategory{CategoryDataCreate}
	case KindMediaSetDeleted, KindMediaItemDeleted:
		return []AuditCategory{CategoryDataDelete}
	case KindMediaSetMarkingsChanged, KindMediaItemMarkingOverridden:
		return []AuditCategory{CategoryManagementMarkings}
	case KindMediaSetRetentionChanged,
		KindMediaSetTransactionOpened,
		KindMediaSetTransactionCommitted,
		KindMediaSetTransactionAborted:
		return []AuditCategory{CategoryDataUpdate}
	case KindMediaSetAccessPatternInvoked:
		return []AuditCategory{CategoryDataLoad}
	case KindMediaItemUploaded:
		return []AuditCategory{CategoryDataImport}
	case KindMediaItemDownloaded:
		return []AuditCategory{CategoryDataExport}
	case KindVirtualMediaItemRegistered:
		return []AuditCategory{CategoryDataCreate}
	default:
		return nil
	}
}

// AuditEvent is the discriminated union of every recordable mutation.
//
// One struct with all variant fields is the closest Go gets to
// Rust's sealed enum without resorting to interface boxing for every
// emit call. The trade-off is mild type laxity in code; the
// invariant — only the fields relevant to Kind are populated — is
// enforced by the per-variant constructors below
// (NewMediaSetCreated, NewMediaItemUploaded, …).
//
// JSON wire format is byte-identical to the Rust impl: `kind` is the
// discriminator (e.g. "media_set.created"), payload-specific fields
// land at the same level, and unset fields are omitted entirely.
type AuditEvent struct {
	Kind            EventKind `json:"kind"`
	ResourceRID     string    `json:"resource_rid"`
	ProjectRID      string    `json:"project_rid"`
	MarkingsAtEvent []string  `json:"markings_at_event"`

	// MediaSetCreated.
	Name              string `json:"name,omitempty"`
	Schema            string `json:"schema,omitempty"`
	TransactionPolicy string `json:"transaction_policy,omitempty"`
	Virtual           *bool  `json:"virtual,omitempty"`

	// Markings + retention changes.
	PreviousMarkings         []string `json:"previous_markings,omitempty"`
	PreviousRetentionSeconds *int64   `json:"previous_retention_seconds,omitempty"`
	NewRetentionSeconds      *int64   `json:"new_retention_seconds,omitempty"`

	// Transaction events.
	TransactionRID string `json:"transaction_rid,omitempty"`
	Branch         string `json:"branch,omitempty"`

	// Access patterns.
	AccessPattern string `json:"access_pattern,omitempty"`
	Persistence   string `json:"persistence,omitempty"`

	// Media item.
	MediaSetRID string  `json:"media_set_rid,omitempty"`
	Path        string  `json:"path,omitempty"`
	MimeType    string  `json:"mime_type,omitempty"`
	SizeBytes   *int64  `json:"size_bytes,omitempty"`
	SHA256      string  `json:"sha256,omitempty"`
	TTLSeconds  *uint64 `json:"ttl_seconds,omitempty"`

	// Virtual media item.
	PhysicalPath string `json:"physical_path,omitempty"`
	ItemPath     string `json:"item_path,omitempty"`
}

// Categories is shorthand for CategoriesFor(e.Kind).
func (e *AuditEvent) Categories() []AuditCategory { return CategoriesFor(e.Kind) }

// ─── Variant constructors ───────────────────────────────────────────────

func boolPtr(b bool) *bool       { return &b }
func i64Ptr(v int64) *int64      { return &v }
func u64Ptr(v uint64) *uint64    { return &v }

// NewMediaSetCreated builds an event for a freshly created media set.
func NewMediaSetCreated(rid, projectRID string, markings []string, name, schema, txPolicy string, virtual bool) AuditEvent {
	return AuditEvent{
		Kind:              KindMediaSetCreated,
		ResourceRID:       rid,
		ProjectRID:        projectRID,
		MarkingsAtEvent:   markings,
		Name:              name,
		Schema:            schema,
		TransactionPolicy: txPolicy,
		Virtual:           boolPtr(virtual),
	}
}

// NewMediaSetDeleted builds an event for a media-set deletion.
func NewMediaSetDeleted(rid, projectRID string, markings []string) AuditEvent {
	return AuditEvent{
		Kind:            KindMediaSetDeleted,
		ResourceRID:     rid,
		ProjectRID:      projectRID,
		MarkingsAtEvent: markings,
	}
}

// NewMediaSetMarkingsChanged builds the markings-changed audit event.
func NewMediaSetMarkingsChanged(rid, projectRID string, markings, previous []string) AuditEvent {
	return AuditEvent{
		Kind:             KindMediaSetMarkingsChanged,
		ResourceRID:      rid,
		ProjectRID:       projectRID,
		MarkingsAtEvent:  markings,
		PreviousMarkings: previous,
	}
}

// NewMediaSetRetentionChanged builds the retention-change audit event.
func NewMediaSetRetentionChanged(rid, projectRID string, markings []string, previousSecs, newSecs int64) AuditEvent {
	return AuditEvent{
		Kind:                     KindMediaSetRetentionChanged,
		ResourceRID:              rid,
		ProjectRID:               projectRID,
		MarkingsAtEvent:          markings,
		PreviousRetentionSeconds: i64Ptr(previousSecs),
		NewRetentionSeconds:      i64Ptr(newSecs),
	}
}

// NewMediaSetTransactionOpened/Committed/Aborted build the three
// dataset-transaction audit events.
func NewMediaSetTransactionOpened(rid, projectRID string, markings []string, txRID, branch string) AuditEvent {
	return AuditEvent{Kind: KindMediaSetTransactionOpened, ResourceRID: rid, ProjectRID: projectRID, MarkingsAtEvent: markings, TransactionRID: txRID, Branch: branch}
}
func NewMediaSetTransactionCommitted(rid, projectRID string, markings []string, txRID, branch string) AuditEvent {
	return AuditEvent{Kind: KindMediaSetTransactionCommitted, ResourceRID: rid, ProjectRID: projectRID, MarkingsAtEvent: markings, TransactionRID: txRID, Branch: branch}
}
func NewMediaSetTransactionAborted(rid, projectRID string, markings []string, txRID, branch string) AuditEvent {
	return AuditEvent{Kind: KindMediaSetTransactionAborted, ResourceRID: rid, ProjectRID: projectRID, MarkingsAtEvent: markings, TransactionRID: txRID, Branch: branch}
}

// NewMediaSetAccessPatternInvoked records server-side materialisation
// (image transform, OCR, transcription, …).
func NewMediaSetAccessPatternInvoked(rid, projectRID string, markings []string, pattern, persistence string) AuditEvent {
	return AuditEvent{
		Kind:            KindMediaSetAccessPatternInvoked,
		ResourceRID:     rid,
		ProjectRID:      projectRID,
		MarkingsAtEvent: markings,
		AccessPattern:   pattern,
		Persistence:     persistence,
	}
}

// NewMediaItemUploaded records a fresh media-item upload.
//
// `transactionRID` is optional — pass "" outside transactional contexts.
func NewMediaItemUploaded(itemRID, mediaSetRID, projectRID string, markings []string,
	path, mime string, size int64, sha256, transactionRID string) AuditEvent {
	return AuditEvent{
		Kind:            KindMediaItemUploaded,
		ResourceRID:     itemRID,
		MediaSetRID:     mediaSetRID,
		ProjectRID:      projectRID,
		MarkingsAtEvent: markings,
		Path:            path,
		MimeType:        mime,
		SizeBytes:       i64Ptr(size),
		SHA256:          sha256,
		TransactionRID:  transactionRID,
	}
}

// NewMediaItemDownloaded records a media-item download.
func NewMediaItemDownloaded(itemRID, mediaSetRID, projectRID string, markings []string, size int64, ttl uint64) AuditEvent {
	return AuditEvent{
		Kind:            KindMediaItemDownloaded,
		ResourceRID:     itemRID,
		MediaSetRID:     mediaSetRID,
		ProjectRID:      projectRID,
		MarkingsAtEvent: markings,
		SizeBytes:       i64Ptr(size),
		TTLSeconds:      u64Ptr(ttl),
	}
}

// NewMediaItemDeleted records a media-item deletion.
func NewMediaItemDeleted(itemRID, mediaSetRID, projectRID string, markings []string, size int64) AuditEvent {
	return AuditEvent{
		Kind:            KindMediaItemDeleted,
		ResourceRID:     itemRID,
		MediaSetRID:     mediaSetRID,
		ProjectRID:      projectRID,
		MarkingsAtEvent: markings,
		SizeBytes:       i64Ptr(size),
	}
}

// NewMediaItemMarkingOverridden records a per-item marking override.
func NewMediaItemMarkingOverridden(itemRID, mediaSetRID, projectRID string, markings, previous []string) AuditEvent {
	return AuditEvent{
		Kind:             KindMediaItemMarkingOverridden,
		ResourceRID:      itemRID,
		MediaSetRID:      mediaSetRID,
		ProjectRID:       projectRID,
		MarkingsAtEvent:  markings,
		PreviousMarkings: previous,
	}
}

// NewVirtualMediaItemRegistered records the registration of a virtual
// (pointer-only) media item in a media set.
func NewVirtualMediaItemRegistered(itemRID, mediaSetRID, projectRID string, markings []string, physicalPath, itemPath string) AuditEvent {
	return AuditEvent{
		Kind:            KindVirtualMediaItemRegistered,
		ResourceRID:     itemRID,
		MediaSetRID:     mediaSetRID,
		ProjectRID:      projectRID,
		MarkingsAtEvent: markings,
		PhysicalPath:    physicalPath,
		ItemPath:        itemPath,
	}
}
