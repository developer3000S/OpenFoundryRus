package vectorstore

import (
	"context"
	"encoding/json"
)

// PgVectorBackend is the placeholder pgvector-backed
// [VectorBackend] handle. It exists to anchor the contract
// between [VectorBackend] and the historical pgvector implementation
// pending in libs/vector-store/src/pgvector.rs.
//
// Construction parameters (pgx pool, table name, embedding
// dimension, …) will be added when the implementation lands. For
// now the type is empty so the interface impl below compiles, and
// every method returns [BackendErrUnimplemented].
//
// The public surface is intentionally minimal so adding the real
// implementation later is a non-breaking change.
type PgVectorBackend struct{}

// NewPgVectorBackend creates a new (currently inert) pgvector
// backend handle.
func NewPgVectorBackend() *PgVectorBackend {
	return &PgVectorBackend{}
}

// Upsert always returns [ErrUnimplemented].
func (b *PgVectorBackend) Upsert(_ context.Context, _ string, _ map[string]json.RawMessage, _ []float32) error {
	return NewUnimplementedError("pgvector::upsert")
}

// Delete always returns [ErrUnimplemented].
func (b *PgVectorBackend) Delete(_ context.Context, _ string) error {
	return NewUnimplementedError("pgvector::delete")
}

// HybridQuery always returns [ErrUnimplemented].
func (b *PgVectorBackend) HybridQuery(_ context.Context, _ string, _ []float32, _ Filter, _ int) ([]QueryHit, error) {
	return nil, NewUnimplementedError("pgvector::hybrid_query")
}

// Compile-time assertion that the skeleton satisfies the
// interface.
var _ VectorBackend = (*PgVectorBackend)(nil)
