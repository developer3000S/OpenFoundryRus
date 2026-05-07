// Live Cassandra paginated scanner. Mirrors the
// `CassandraScanner` portion of `services/reindex-coordinator-service/
// src/scan.rs` so the two coordinators publish bit-for-bit identical
// `ontology.reindex.v1` records during the cut-over.
//
// Two-query pattern, identical to the legacy Go worker
// (`workers-go/reindex/activities/activities.go::scan`):
//
//  1. Index lookup against `objects_by_type` — per-type when a
//     `type_id` is supplied, otherwise an `ALLOW FILTERING` scan for
//     every type under the tenant.
//  2. Hydration via `objects_by_id` for each id returned in step 1;
//     soft-deleted rows (`deleted = true`) and rows whose primary
//     record has been purged are dropped before publish.
//
// Page state is the opaque base64 of the gocql paging-state bytes,
// matching the encoding used elsewhere in the service
// (`event.DeriveBatchEventID`) and `libs/cassandra-kernel/repos_shared.go::
// encodePagingState`.
package scan

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gocql/gocql"
)

// PageOutcome is the result of a single ScanPage call.
type PageOutcome struct {
	// Records hydrated from `objects_by_id` ready for publish.
	// Soft-deleted rows are already filtered out — `Deleted` will
	// always be false on the wire.
	Records []ReindexRecord
	// Scanned is the total number of ids returned by the index
	// lookup for this page, regardless of whether they survived the
	// deleted-row filter. Used for the `scanned` counter on the job
	// row.
	Scanned int
	// NextToken is the opaque base64 of the gocql paging-state.
	// nil ⇒ end of stream.
	NextToken *string
}

// ScanError mirrors the variants of the Rust `ScanError` enum so the
// error surface stays identical across implementations.
type ScanError struct {
	// Kind is one of "driver", "invalid_resume_token",
	// "invalid_object_payload".
	Kind     string
	Reason   string
	ObjectID string // populated for invalid_object_payload
}

func (e *ScanError) Error() string {
	switch e.Kind {
	case "driver":
		return "cassandra query failed: " + e.Reason
	case "invalid_resume_token":
		return "invalid resume token: " + e.Reason
	case "invalid_object_payload":
		return fmt.Sprintf("invalid object payload for id %s: %s", e.ObjectID, e.Reason)
	default:
		return "scan error: " + e.Reason
	}
}

// IsScanError tests for a ScanError of any kind.
func IsScanError(err error) bool {
	var se *ScanError
	return errors.As(err, &se)
}

// CassandraScanner runs the index → hydrate two-query pattern. One
// instance per process, shared by reference. The keyspace is fixed at
// construction time so the table-name slot in CQL is never user-
// controlled (CQL does not parameterise table names). gocql caches
// prepared statements per session, so re-issuing the same CQL string
// each call has effectively zero overhead.
type CassandraScanner struct {
	session  *gocql.Session
	keyspace string
}

// NewCassandraScanner builds a scanner bound to `keyspace`.
func NewCassandraScanner(session *gocql.Session, keyspace string) *CassandraScanner {
	return &CassandraScanner{session: session, keyspace: keyspace}
}

func (s *CassandraScanner) cqlIndexByType() string {
	return fmt.Sprintf(
		`SELECT object_id FROM %s.objects_by_type
		  WHERE tenant = ? AND type_id = ?`, s.keyspace)
}

func (s *CassandraScanner) cqlIndexAllTypes() string {
	return fmt.Sprintf(
		`SELECT type_id, object_id FROM %s.objects_by_type
		  WHERE tenant = ? ALLOW FILTERING`, s.keyspace)
}

func (s *CassandraScanner) cqlGetObject() string {
	return fmt.Sprintf(
		`SELECT type_id, properties, revision_number, deleted
		   FROM %s.objects_by_id WHERE tenant = ? AND object_id = ?`, s.keyspace)
}

// ScanPage fetches and hydrates one page. `resumeToken` is the
// previously-returned PageOutcome.NextToken, or nil for the first
// page. `pageSize` is clamped to [MinPageSize, MaxPageSize] — callers
// should already have done that via DecodeRequest, but we re-clamp
// defensively so a misuse can't blow past the SQL CHECK that the
// `reindex_jobs.page_size` column enforces.
func (s *CassandraScanner) ScanPage(
	ctx context.Context,
	tenantID string,
	typeID *string,
	pageSize int32,
	resumeToken *string,
) (*PageOutcome, error) {
	if pageSize < MinPageSize {
		pageSize = MinPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}

	pagingState, err := decodeOpaquePagingState(resumeToken)
	if err != nil {
		return nil, err
	}

	ids, nextState, err := s.runIndexLookup(ctx, tenantID, typeID, int(pageSize), pagingState)
	if err != nil {
		return nil, err
	}

	scanned := len(ids)
	records := make([]ReindexRecord, 0, len(ids))
	for _, ix := range ids {
		rec, err := s.fetchObject(ctx, tenantID, ix.objectID, ix.typeHint)
		if err != nil {
			return nil, err
		}
		if rec != nil {
			records = append(records, *rec)
		}
	}

	return &PageOutcome{
		Records:   records,
		Scanned:   scanned,
		NextToken: encodeOpaquePagingState(nextState),
	}, nil
}

// indexRow is the (object_id, effective type_id) pair returned by the
// index-lookup step.
type indexRow struct {
	objectID gocql.UUID
	typeHint string
}

func (s *CassandraScanner) runIndexLookup(
	ctx context.Context,
	tenantID string,
	typeID *string,
	pageSize int,
	pagingState []byte,
) ([]indexRow, []byte, error) {
	if typeID != nil {
		return s.indexByType(ctx, tenantID, *typeID, pageSize, pagingState)
	}
	return s.indexAllTypes(ctx, tenantID, pageSize, pagingState)
}

func (s *CassandraScanner) indexByType(
	ctx context.Context,
	tenantID, typeID string,
	pageSize int,
	pagingState []byte,
) ([]indexRow, []byte, error) {
	q := s.session.Query(s.cqlIndexByType(), tenantID, typeID).
		WithContext(ctx).
		PageSize(pageSize)
	if len(pagingState) > 0 {
		q = q.PageState(pagingState)
	}
	iter := q.Iter()
	out := make([]indexRow, 0, pageSize)
	var objectID gocql.UUID
	// gocql auto-paginates within a single Iter — to surface the
	// server's paging-state cursor we stop after pageSize rows so the
	// driver does not silently fetch page N+1. Same trick as the
	// Rust impl, which consumes exactly `result.rows` and then calls
	// `result.paging_state.clone()`.
	for i := 0; i < pageSize; i++ {
		if !iter.Scan(&objectID) {
			break
		}
		out = append(out, indexRow{objectID: objectID, typeHint: typeID})
	}
	nextState := append([]byte(nil), iter.PageState()...)
	if err := iter.Close(); err != nil {
		return nil, nil, &ScanError{Kind: "driver", Reason: err.Error()}
	}
	return out, nextState, nil
}

func (s *CassandraScanner) indexAllTypes(
	ctx context.Context,
	tenantID string,
	pageSize int,
	pagingState []byte,
) ([]indexRow, []byte, error) {
	q := s.session.Query(s.cqlIndexAllTypes(), tenantID).
		WithContext(ctx).
		PageSize(pageSize)
	if len(pagingState) > 0 {
		q = q.PageState(pagingState)
	}
	iter := q.Iter()
	out := make([]indexRow, 0, pageSize)
	var (
		rowType  string
		objectID gocql.UUID
	)
	for i := 0; i < pageSize; i++ {
		if !iter.Scan(&rowType, &objectID) {
			break
		}
		out = append(out, indexRow{objectID: objectID, typeHint: rowType})
	}
	nextState := append([]byte(nil), iter.PageState()...)
	if err := iter.Close(); err != nil {
		return nil, nil, &ScanError{Kind: "driver", Reason: err.Error()}
	}
	return out, nextState, nil
}

// fetchObject hydrates a single object. Returns (nil, nil) for soft-
// deleted rows and for ids whose primary record has been purged —
// matching the legacy Go worker's behaviour.
func (s *CassandraScanner) fetchObject(
	ctx context.Context,
	tenantID string,
	objectID gocql.UUID,
	typeHint string,
) (*ReindexRecord, error) {
	var (
		typeID     *string
		properties *string
		revision   *int64
		deleted    *bool
	)
	err := s.session.Query(s.cqlGetObject(), tenantID, objectID).
		WithContext(ctx).
		Scan(&typeID, &properties, &revision, &deleted)
	if errors.Is(err, gocql.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, &ScanError{Kind: "driver", Reason: err.Error()}
	}
	if deleted != nil && *deleted {
		return nil, nil
	}

	effType := typeHint
	if typeID != nil && *typeID != "" {
		effType = *typeID
	}
	rev := int64(0)
	if revision != nil {
		rev = *revision
	}
	propsText := "{}"
	if properties != nil && *properties != "" {
		propsText = *properties
	}
	propsRaw := json.RawMessage(propsText)
	if !json.Valid(propsRaw) {
		return nil, &ScanError{
			Kind:     "invalid_object_payload",
			ObjectID: objectID.String(),
			Reason:   "stored payload is not valid JSON",
		}
	}

	rec := EncodeBatchRecord(tenantID, objectID.String(), effType, rev, propsRaw)
	return &rec, nil
}

// encodeOpaquePagingState turns gocql paging-state bytes into the
// opaque base64 token surfaced to callers. Returns nil when there is
// no next page.
func encodeOpaquePagingState(b []byte) *string {
	if len(b) == 0 {
		return nil
	}
	s := base64.StdEncoding.EncodeToString(b)
	return &s
}

// decodeOpaquePagingState reverses encodeOpaquePagingState. Treats
// nil and the empty string as "no token" — matches the Rust impl
// which collapses `Some("")` to None.
func decodeOpaquePagingState(token *string) ([]byte, error) {
	if token == nil || *token == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(*token)
	if err != nil {
		return nil, &ScanError{Kind: "invalid_resume_token", Reason: err.Error()}
	}
	return raw, nil
}
