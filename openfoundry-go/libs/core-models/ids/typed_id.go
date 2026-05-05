// Package ids provides typed UUID v7 identifiers.
//
// Go's lack of a phantom-type generic equivalent means we cannot fully
// reproduce the Rust `TypedId<T>` newtype. Instead we expose a helper
// to mint v7 UUIDs and a thin wrapper services can embed to brand IDs
// per entity (see TypedID example).
package ids

import (
	"fmt"

	"github.com/google/uuid"
)

// New returns a fresh time-ordered UUID (v7).
//
// The Rust workspace standardises on UUID v7 for all entity IDs because
// it gives time-ordered B-tree-friendly inserts in Postgres.
func New() uuid.UUID {
	id, err := uuid.NewV7()
	if err != nil {
		// uuid.NewV7 can only fail if the system clock fails, which is
		// not recoverable.
		panic(fmt.Errorf("uuid v7 mint: %w", err))
	}
	return id
}

// TypedID is a thin wrapper services can alias per-entity to get
// compile-time separation between, say, UserID and SessionID:
//
//	type UserID    struct{ ids.TypedID }
//	type SessionID struct{ ids.TypedID }
//
// Both round-trip through JSON as the underlying UUID string, matching
// the Rust `#[serde(transparent)]` representation.
type TypedID struct {
	ID uuid.UUID
}

// NewTypedID mints a new TypedID backed by a v7 UUID.
func NewTypedID() TypedID { return TypedID{ID: New()} }

func (t TypedID) String() string             { return t.ID.String() }
func (t TypedID) MarshalText() ([]byte, error) { return []byte(t.ID.String()), nil }

func (t *TypedID) UnmarshalText(b []byte) error {
	parsed, err := uuid.Parse(string(b))
	if err != nil {
		return fmt.Errorf("typed id parse: %w", err)
	}
	t.ID = parsed
	return nil
}
