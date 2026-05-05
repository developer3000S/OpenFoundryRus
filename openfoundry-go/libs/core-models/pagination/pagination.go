// Package pagination defines the cursor-based pagination envelope used
// across every list endpoint.
package pagination

// PageRequest is the inbound cursor + limit pair.
//
// `cursor` is opaque to the caller (services encode their own state).
// `limit` defaults to 50 when zero — the same default as the Rust side.
type PageRequest struct {
	Cursor *string `json:"cursor,omitempty"`
	Limit  uint32  `json:"limit"`
}

// DefaultLimit is the page size used when a request comes in with limit=0.
const DefaultLimit uint32 = 50

// Normalised returns a copy of the request with the default limit applied.
func (p PageRequest) Normalised() PageRequest {
	if p.Limit == 0 {
		p.Limit = DefaultLimit
	}
	return p
}

// PageResponse is the canonical list response envelope.
type PageResponse[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor,omitempty"`
	Total      *uint64 `json:"total,omitempty"`
}
