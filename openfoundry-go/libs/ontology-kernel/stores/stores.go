// Package stores is the Go port of `libs/ontology-kernel/src/stores/*`.
//
// All persistence in ontology-kernel is being migrated from raw `pgx`
// call sites to the storage-abstraction interfaces so that the same
// handlers can be wired against:
//
//   - CassandraObjectStore / CassandraLinkStore / CassandraActionLogStore
//     (the production target — see ADR-0020),
//   - the legacy Postgres adapters in [pg.go] (only behind the
//     legacy-pg build tag, used while handlers are migrated one
//     service at a time per
//     docs/architecture/migration-plan-cassandra-foundry-parity.md
//     §S1.4–S1.7),
//   - hand-rolled fakes in [mock.go] for unit tests.
//
// The kernel's [github.com/openfoundry/openfoundry-go/libs/ontology-kernel.AppState]
// carries a single [Stores] handle so handlers stay infrastructure-
// agnostic.
//
// Coverage vs Rust: the Rust source declares seven trait fields —
// objects / links / actions / definitions / read_models / search /
// object_set_materializations. The Go port ships all seven, with
// object set materializations backed by the same search abstraction used by Rust.
package stores

import storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"

// Stores mirrors `pub struct Stores` in src/stores/mod.rs — the
// trait-object bag that ontology-kernel handlers route their I/O
// through.
type Stores struct {
	Objects     storageabstraction.ObjectStore
	Links       storageabstraction.LinkStore
	Actions     storageabstraction.ActionLogStore
	Definitions storageabstraction.DefinitionStore
	ReadModels  storageabstraction.ReadModelStore
	// Search is the optional lexical/vector backend. When nil,
	// `LoadAccessibleObjectSet` falls back to ObjectStore.ListByType
	// (matches the Rust feature-flag fallback when SearchBackend is
	// not configured).
	Search                    storageabstraction.SearchBackend
	ObjectSetMaterializations ObjectSetMaterializationStore
}

// NewInMemory mirrors `Stores::in_memory()`. Returns a Stores backed
// by hand-rolled in-process fakes (`mock.go` for the legacy three +
// `inmemory_definitions.go` for the read-side bag). Intended for
// unit tests and for smoke-testing handlers without spinning up
// infrastructure.
func NewInMemory() Stores {
	out := Stores{
		Objects:     NewInMemoryObjectStore(),
		Links:       NewInMemoryLinkStore(),
		Actions:     NewInMemoryActionLogStore(),
		Definitions: NewInMemoryDefinitionStore(),
		ReadModels:  NewInMemoryReadModelStore(),
		Search:      storageabstraction.NewInMemorySearchBackend(),
	}
	out.ObjectSetMaterializations = NewInMemoryObjectSetMaterializationStore()
	return out
}
