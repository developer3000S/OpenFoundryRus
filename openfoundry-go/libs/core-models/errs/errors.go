// Package errs is the shared error taxonomy for every service.
// Mirrors the Rust `core_models::error::AppError` enum.
package errs

import "fmt"

// Kind enumerates the canonical error categories.
type Kind int

const (
	KindUnknown Kind = iota
	KindNotFound
	KindUnauthorized
	KindForbidden
	KindBadRequest
	KindConflict
	KindInternal
)

// String returns the same lowercase token the Rust side emits in
// `Display` impls so log lines and error responses look identical.
func (k Kind) String() string {
	switch k {
	case KindNotFound:
		return "not found"
	case KindUnauthorized:
		return "unauthorized"
	case KindForbidden:
		return "forbidden"
	case KindBadRequest:
		return "bad request"
	case KindConflict:
		return "conflict"
	case KindInternal:
		return "internal error"
	default:
		return "unknown error"
	}
}

// AppError is the canonical service error carrying a category and a
// human-readable message. Implements `error` and supports `errors.Is`
// against a sentinel of the same Kind.
type AppError struct {
	Kind Kind
	Msg  string
}

func (e *AppError) Error() string { return fmt.Sprintf("%s: %s", e.Kind, e.Msg) }

// Is supports comparing by Kind only (errors.Is(err, &AppError{Kind: KindNotFound})).
func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	if !ok {
		return false
	}
	return t.Kind == e.Kind
}

func NotFound(msg string) *AppError     { return &AppError{Kind: KindNotFound, Msg: msg} }
func Unauthorized(msg string) *AppError { return &AppError{Kind: KindUnauthorized, Msg: msg} }
func Forbidden(msg string) *AppError    { return &AppError{Kind: KindForbidden, Msg: msg} }
func BadRequest(msg string) *AppError   { return &AppError{Kind: KindBadRequest, Msg: msg} }
func Conflict(msg string) *AppError     { return &AppError{Kind: KindConflict, Msg: msg} }
func Internal(msg string) *AppError     { return &AppError{Kind: KindInternal, Msg: msg} }
