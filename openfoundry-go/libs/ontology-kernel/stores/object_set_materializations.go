package stores

import (
	"context"
	"encoding/json"

	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// ObjectSetMaterializedRow mirrors storage-abstraction's Rust
// ObjectSetMaterializedRow for ontology-kernel materialization consumers.
type ObjectSetMaterializedRow struct {
	RowID   string          `json:"row_id"`
	Ordinal uint32          `json:"ordinal"`
	Payload json.RawMessage `json:"payload"`
}

// ObjectSetMaterializationMetadata describes the latest materialized snapshot.
type ObjectSetMaterializationMetadata struct {
	Tenant                 storageabstraction.TenantId    `json:"tenant"`
	SetID                  storageabstraction.ObjectSetId `json:"set_id"`
	BaseTypeID             storageabstraction.TypeId      `json:"base_type_id"`
	MaterializationID      string                         `json:"materialization_id"`
	GeneratedAtMs          int64                          `json:"generated_at_ms"`
	TotalBaseMatches       uint64                         `json:"total_base_matches"`
	TotalRows              uint64                         `json:"total_rows"`
	StoredRowCount         uint64                         `json:"stored_row_count"`
	TraversalNeighborCount uint64                         `json:"traversal_neighbor_count"`
}

// ObjectSetMaterialization is the full replacement payload for a saved set.
type ObjectSetMaterialization struct {
	Tenant                 storageabstraction.TenantId    `json:"tenant"`
	SetID                  storageabstraction.ObjectSetId `json:"set_id"`
	BaseTypeID             storageabstraction.TypeId      `json:"base_type_id"`
	GeneratedAtMs          int64                          `json:"generated_at_ms"`
	TotalBaseMatches       uint64                         `json:"total_base_matches"`
	TotalRows              uint64                         `json:"total_rows"`
	TraversalNeighborCount uint64                         `json:"traversal_neighbor_count"`
	Rows                   []ObjectSetMaterializedRow     `json:"rows"`
}

// ObjectSetMaterializationStore mirrors the Rust trait used by object-set
// materialize/get enrichment flows.
type ObjectSetMaterializationStore interface {
	Replace(ctx context.Context, materialization ObjectSetMaterialization) (ObjectSetMaterializationMetadata, error)
	GetMetadata(ctx context.Context, tenant storageabstraction.TenantId, setID storageabstraction.ObjectSetId, consistency storageabstraction.ReadConsistency) (*ObjectSetMaterializationMetadata, error)
	ListRows(ctx context.Context, tenant storageabstraction.TenantId, setID storageabstraction.ObjectSetId, page storageabstraction.Page, consistency storageabstraction.ReadConsistency) ([]ObjectSetMaterializedRow, *string, error)
	Delete(ctx context.Context, tenant storageabstraction.TenantId, setID storageabstraction.ObjectSetId) (bool, error)
}
