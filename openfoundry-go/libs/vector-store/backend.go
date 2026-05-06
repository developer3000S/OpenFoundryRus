package vectorstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// Document is the logical record fed into a backend.
//
// Fields is an arbitrary JSON map so the same trait can serve simple
// `text + tenant_id` retrieval as well as richer catalog records.
// Embedding may be empty for backends that don't index vectors.
type Document struct {
	// ID is the stable identifier of the document inside its
	// logical collection.
	ID string `json:"id"`
	// Fields is a backend-agnostic field bag. Concrete backends
	// decide how to map each entry onto their own schema (column,
	// attribute, JSON field…).
	Fields map[string]json.RawMessage `json:"fields"`
	// Embedding is the dense embedding. Length must match the
	// embedding dimension the backend was configured for; empty
	// means "no vector signal".
	Embedding []float32 `json:"embedding"`
}

// Filter is a restrictive filter applied on top of the hybrid
// query.
//
// Kept deliberately small: backends translate this into their
// native filtering language (SQL `WHERE`, YQL clauses…). Exact
// equality is supported on every backend; ranges/sets can be added
// later in a backwards-compatible way.
type Filter struct {
	// Equals is an AND-combined set of exact-match constraints,
	// e.g. `{"tenant_id": "acme"}`.
	Equals map[string]json.RawMessage `json:"equals"`
}

// FilterEq is a convenience constructor for a single equality
// clause. The value is JSON-encoded; pass any json-marshalable
// scalar.
func FilterEq(field string, value any) Filter {
	raw, err := json.Marshal(value)
	if err != nil {
		// Fall back to an empty value rather than panicking;
		// callers who care about marshalability should pre-encode
		// the field themselves via Filter{Equals: ...}.
		raw = []byte("null")
	}
	return Filter{Equals: map[string]json.RawMessage{field: raw}}
}

// QueryHit is one result row returned by [VectorBackend.HybridQuery].
type QueryHit struct {
	// ID is the document id.
	ID string `json:"id"`
	// Score is the backend-assigned relevance score; semantics are
	// backend-specific but ordering is always "higher is more
	// relevant".
	Score float64 `json:"score"`
	// Fields are the optional summary fields returned by the
	// backend.
	Fields map[string]json.RawMessage `json:"fields,omitempty"`
}

// BackendErrorKind classifies a [BackendError]. Use the matching
// Err* sentinels with errors.Is.
type BackendErrorKind uint8

const (
	// BackendErrTransport — network / transport failure talking to
	// the backend.
	BackendErrTransport BackendErrorKind = iota + 1
	// BackendErrBackend — backend rejected the request (4xx/5xx,
	// SQL error, …).
	BackendErrBackend
	// BackendErrSerialization — failed to (de)serialize the wire
	// payload.
	BackendErrSerialization
	// BackendErrUnsupported — the backend doesn't (yet) support
	// the requested operation.
	BackendErrUnsupported
	// BackendErrUnimplemented — operation is defined in the
	// interface but the implementation is still pending. Used by
	// skeleton backends that haven't been wired yet.
	BackendErrUnimplemented
)

// BackendError is the error type returned by every backend.
// Wrap with %w via fmt.Errorf to preserve the kind classification
// when bubbling errors up.
type BackendError struct {
	Kind    BackendErrorKind
	Message string
}

func (e *BackendError) Error() string {
	switch e.Kind {
	case BackendErrTransport:
		return "transport error: " + e.Message
	case BackendErrBackend:
		return "backend error: " + e.Message
	case BackendErrSerialization:
		return "serialization error: " + e.Message
	case BackendErrUnsupported:
		return "operation not supported by backend: " + e.Message
	case BackendErrUnimplemented:
		return "operation not implemented for this backend: " + e.Message
	default:
		return "unknown backend error: " + e.Message
	}
}

// Sentinel errors for errors.Is.
var (
	// ErrTransport classifies transport-level failures.
	ErrTransport = errors.New("transport error")
	// ErrBackend classifies backend-rejected requests.
	ErrBackend = errors.New("backend error")
	// ErrSerialization classifies (de)serialization failures.
	ErrSerialization = errors.New("serialization error")
	// ErrUnsupported classifies operations a backend deliberately
	// does not support.
	ErrUnsupported = errors.New("operation not supported by backend")
	// ErrUnimplemented classifies operations whose implementation
	// is still pending in a skeleton backend.
	ErrUnimplemented = errors.New("operation not implemented for this backend")
)

// Is reports whether e matches the given target sentinel by Kind.
func (e *BackendError) Is(target error) bool {
	switch target {
	case ErrTransport:
		return e.Kind == BackendErrTransport
	case ErrBackend:
		return e.Kind == BackendErrBackend
	case ErrSerialization:
		return e.Kind == BackendErrSerialization
	case ErrUnsupported:
		return e.Kind == BackendErrUnsupported
	case ErrUnimplemented:
		return e.Kind == BackendErrUnimplemented
	}
	return false
}

// NewTransportError wraps msg as a transport-level [BackendError].
func NewTransportError(format string, args ...any) *BackendError {
	return &BackendError{Kind: BackendErrTransport, Message: fmt.Sprintf(format, args...)}
}

// NewBackendError wraps msg as a backend-rejection [BackendError].
func NewBackendError(format string, args ...any) *BackendError {
	return &BackendError{Kind: BackendErrBackend, Message: fmt.Sprintf(format, args...)}
}

// NewSerializationError wraps msg as a serialization [BackendError].
func NewSerializationError(format string, args ...any) *BackendError {
	return &BackendError{Kind: BackendErrSerialization, Message: fmt.Sprintf(format, args...)}
}

// NewUnsupportedError wraps op as an "unsupported" [BackendError].
func NewUnsupportedError(op string) *BackendError {
	return &BackendError{Kind: BackendErrUnsupported, Message: op}
}

// NewUnimplementedError wraps op as an "unimplemented" [BackendError].
func NewUnimplementedError(op string) *BackendError {
	return &BackendError{Kind: BackendErrUnimplemented, Message: op}
}

// VectorBackend is the common contract every search/vector backend
// implements.
//
// All methods take a [context.Context] for cancellation/deadlines
// and return errors typed via [BackendError] (use errors.Is with
// the Err* sentinels for classification).
type VectorBackend interface {
	// Upsert inserts or replaces a document. Implementations must
	// be idempotent on docID.
	Upsert(ctx context.Context, docID string, fields map[string]json.RawMessage, embedding []float32) error

	// Delete removes a document by id. Deleting a non-existent id
	// must succeed (no-op) so callers can run idempotent
	// reconciliation loops.
	Delete(ctx context.Context, docID string) error

	// HybridQuery runs a hybrid (lexical + dense) query.
	//
	//  - text: BM25 query string. Empty means "match all" (filter only).
	//  - embedding: dense vector for ANN search. Empty means "skip ANN".
	//  - filter: AND-combined restrictions.
	//  - topK: maximum number of hits to return.
	HybridQuery(ctx context.Context, text string, embedding []float32, filter Filter, topK int) ([]QueryHit, error)
}
