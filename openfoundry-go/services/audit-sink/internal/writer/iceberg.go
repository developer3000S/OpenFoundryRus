package writer

import (
	"context"

	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/envelope"
)

// IcebergWriter is the production target — appends batches to the
// Iceberg table `lakekeeper/of_audit/events` partitioned by `day(at)`.
//
// # TODO: real implementation
//
// The Rust sink uses `iceberg-rust`'s `IcebergTable::append_record_batches`.
// Apache iceberg-go's write API is still maturing — table identifier
// + namespace handling + REST snapshot commit landed in v0.x but the
// Parquet-write path is rough. Wire the real implementation when:
//
//   - github.com/apache/iceberg-go releases write-side stability docs.
//   - storage-abstraction (Go) gains an Iceberg facade equivalent to
//     the Rust crate.
//
// Until then, callers default to JSONLWriter (set
// AUDIT_SINK_JSONL_PATH=/var/log/audit.jsonl in dev / staging).
//
// IMPORTANT: this stub keeps the interface honest — it errors on
// every Append so the runtime fails loudly rather than silently
// dropping batches.
type IcebergWriter struct {
	CatalogURL string
	Warehouse  string
	Namespace  string
	Table      string
}

// NewIcebergWriter returns the stub.
func NewIcebergWriter(catalogURL, warehouse, namespace, table string) *IcebergWriter {
	return &IcebergWriter{
		CatalogURL: catalogURL,
		Warehouse:  warehouse,
		Namespace:  namespace,
		Table:      table,
	}
}

// Append implements Writer — currently always returns ErrNotImplemented.
func (i *IcebergWriter) Append(_ context.Context, _ []envelope.AuditEnvelope) error {
	return ErrNotImplemented
}

// Close implements Writer — no-op for the stub.
func (i *IcebergWriter) Close() error { return nil }
