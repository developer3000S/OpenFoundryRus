// Definition + read-model trait surfaces of the storage-abstraction
// crate. These mirror `pub trait DefinitionStore` and
// `pub trait ReadModelStore` (plus their supporting Kind / Id /
// Record / Query value objects) in
// `libs/storage-abstraction/src/repositories.rs`.
//
// Production wiring keeps declarative ontology definitions in
// PostgreSQL and warm runtime projections in a search/read-model
// plane; the kernel handlers depend on these traits rather than
// embedding raw SQL — see the `domain/*_repository.go` adapters in
// ontology-kernel.

package storageabstraction

import (
	"context"
	"encoding/json"
)

// ---------------------------------------------------------------------------
// Definitions
// ---------------------------------------------------------------------------

// DefinitionKind mirrors `struct DefinitionKind(pub String)`. Logical
// bucket for declarative ontology definitions (e.g. `object_type`,
// `property`, `action_type`).
type DefinitionKind string

// DefinitionId mirrors `struct DefinitionId(pub String)`. Stable
// identifier for a declarative definition row.
type DefinitionId string

// DefinitionRecord mirrors `struct DefinitionRecord`. Payload is
// JSON so storage-abstraction stays independent from
// ontology-kernel's HTTP model structs; callers deserialise it into
// `ObjectType`, `Property`, `ActionType`, etc. at the edge.
type DefinitionRecord struct {
	Kind        DefinitionKind  `json:"kind"`
	ID          DefinitionId    `json:"id"`
	Tenant      *TenantId       `json:"tenant,omitempty"`
	OwnerID     *string         `json:"owner_id,omitempty"`
	ParentID    *DefinitionId   `json:"parent_id,omitempty"`
	Version     *uint64         `json:"version,omitempty"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAtMs *int64          `json:"created_at_ms,omitempty"`
	UpdatedAtMs *int64          `json:"updated_at_ms,omitempty"`
}

// DefinitionQuery mirrors `struct DefinitionQuery`. Lightweight
// filters supported by every backend.
type DefinitionQuery struct {
	Kind     DefinitionKind    `json:"kind"`
	Tenant   *TenantId         `json:"tenant,omitempty"`
	OwnerID  *string           `json:"owner_id,omitempty"`
	ParentID *DefinitionId     `json:"parent_id,omitempty"`
	Filters  map[string]string `json:"filters,omitempty"`
	Search   *string           `json:"search,omitempty"`
	Page     Page              `json:"page"`
}

// DefinitionStore is the repository for declarative ontology
// definitions retained in PostgreSQL.
//
// Mirrors `pub trait DefinitionStore: Send + Sync` — every method
// takes ctx + the same arguments as the Rust signature. The Rust
// `count()` default impl is reproduced as a free function
// [DefinitionCount] so implementations can either override or take
// the default.
type DefinitionStore interface {
	// Get loads one definition. Returns (nil, nil) when missing.
	Get(ctx context.Context, kind DefinitionKind, id DefinitionId, consistency ReadConsistency) (*DefinitionRecord, error)

	// List enumerates definitions by kind and lightweight filters.
	List(ctx context.Context, query DefinitionQuery, consistency ReadConsistency) (PagedResult[DefinitionRecord], error)

	// Put inserts or updates one definition.
	// expectedVersion = nil → insert/upsert depending on the
	// backend's natural contract.
	Put(ctx context.Context, record DefinitionRecord, expectedVersion *uint64) (PutOutcome, error)

	// Delete removes one definition. Returns (false, nil) when
	// absent.
	Delete(ctx context.Context, kind DefinitionKind, id DefinitionId) (bool, error)

	// Count returns the matching count. Backends MAY override for
	// a cheaper `SELECT COUNT(*)`-style implementation; the default
	// helper [DefinitionCount] reproduces the Rust default-impl by
	// counting the items returned by List.
	Count(ctx context.Context, query DefinitionQuery, consistency ReadConsistency) (uint64, error)
}

// DefinitionCount mirrors the Rust `DefinitionStore::count` default
// implementation: list and count items. Backends that don't override
// Count can call this from their own Count method.
func DefinitionCount(ctx context.Context, store DefinitionStore, query DefinitionQuery, consistency ReadConsistency) (uint64, error) {
	page, err := store.List(ctx, query, consistency)
	if err != nil {
		return 0, err
	}
	return uint64(len(page.Items)), nil
}

// ---------------------------------------------------------------------------
// Read models
// ---------------------------------------------------------------------------

// ReadModelKind mirrors `struct ReadModelKind(pub String)`. Logical
// bucket for warm runtime projections (e.g. `function_run`,
// `project_working_state`).
type ReadModelKind string

// ReadModelId mirrors `struct ReadModelId(pub String)`. Stable
// identifier for a read-model row.
type ReadModelId string

// ReadModelRecord mirrors `struct ReadModelRecord`. Carries a JSON
// payload + a monotonic version + an updated_at_ms timestamp;
// implementations MUST discard a write whose version is older than
// the currently stored version.
type ReadModelRecord struct {
	Kind        ReadModelKind   `json:"kind"`
	Tenant      TenantId        `json:"tenant"`
	ID          ReadModelId     `json:"id"`
	ParentID    *ReadModelId    `json:"parent_id,omitempty"`
	Payload     json.RawMessage `json:"payload"`
	Version     uint64          `json:"version"`
	UpdatedAtMs int64           `json:"updated_at_ms"`
}

// ReadModelQuery mirrors `struct ReadModelQuery`.
type ReadModelQuery struct {
	Kind     ReadModelKind     `json:"kind"`
	Tenant   TenantId          `json:"tenant"`
	ParentID *ReadModelId      `json:"parent_id,omitempty"`
	Filters  map[string]string `json:"filters,omitempty"`
	Page     Page              `json:"page"`
}

// ReadModelStore is the generic read-model repository for warm
// runtime projections that do not deserve bespoke traits yet.
//
// Mirrors `pub trait ReadModelStore: Send + Sync`.
type ReadModelStore interface {
	// Get loads one read-model row. Returns (nil, nil) when
	// missing.
	Get(ctx context.Context, kind ReadModelKind, tenant TenantId, id ReadModelId, consistency ReadConsistency) (*ReadModelRecord, error)

	// List enumerates rows by kind/tenant + lightweight filters.
	List(ctx context.Context, query ReadModelQuery, consistency ReadConsistency) (PagedResult[ReadModelRecord], error)

	// Put upserts one row. Implementations MUST discard stale
	// writes whose version is older than the currently stored one.
	Put(ctx context.Context, record ReadModelRecord) (PutOutcome, error)

	// Delete removes one row. Returns (false, nil) when absent.
	Delete(ctx context.Context, kind ReadModelKind, tenant TenantId, id ReadModelId) (bool, error)
}
