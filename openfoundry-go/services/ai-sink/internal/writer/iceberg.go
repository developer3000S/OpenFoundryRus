package writer

import (
	"context"

	"github.com/openfoundry/openfoundry-go/services/ai-sink/internal/envelope"
)

// IcebergWriter is the production target — appends batches to the four
// Iceberg tables under `lakekeeper/of_ai/{prompts,responses,evaluations,traces}`.
//
// # TODO: real implementation
//
// The Rust sink uses iceberg-rust's `IcebergTable::append_record_batches`
// once per affected table. Apache iceberg-go's write API is still
// maturing — wire this when the upstream stabilises (track the same
// issue as the audit-sink stub).
//
// Until then, `AI_SINK_JSONL_DIR=/var/log/ai-sink` selects the JSONL
// writer for dev/staging.
type IcebergWriter struct {
	CatalogURL string
	Warehouse  string
	Namespace  string
}

// NewIcebergWriter returns the stub.
func NewIcebergWriter(catalogURL, warehouse, namespace string) *IcebergWriter {
	return &IcebergWriter{CatalogURL: catalogURL, Warehouse: warehouse, Namespace: namespace}
}

// Append implements Writer — currently always returns ErrNotImplemented.
func (i *IcebergWriter) Append(_ context.Context, _ map[string][]envelope.AiEventEnvelope) error {
	return ErrNotImplemented
}

// Close implements Writer — no-op for the stub.
func (i *IcebergWriter) Close() error { return nil }
