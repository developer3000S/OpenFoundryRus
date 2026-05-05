// Package writer abstracts the ai-sink output target.
//
// Two implementations:
//
//   - JSONLWriter — opens one `<table>.jsonl` per Iceberg table inside
//     a directory. Production-suitable for staging and observability
//     until the Iceberg writer matures.
//   - IcebergWriter — stub. iceberg-go's write API is unstable as of
//     this commit; runtime callers default to JSONLWriter.
package writer

import (
	"context"
	"errors"

	"github.com/openfoundry/openfoundry-go/services/ai-sink/internal/envelope"
)

// Writer is the per-table batch sink. Implementations group the input
// batch by `table` and durably persist each group before returning.
type Writer interface {
	// Append durably writes `byTable`. Keys are Iceberg table names,
	// values are the per-table batches. Returns when it is safe for
	// the caller to commit Kafka offsets.
	Append(ctx context.Context, byTable map[string][]envelope.AiEventEnvelope) error
	// Close releases resources. Safe to call multiple times.
	Close() error
}

// ErrNotImplemented signals that the Iceberg writer is not wired up
// in this build. The runtime translates this into a fatal-on-boot.
var ErrNotImplemented = errors.New("writer not implemented in this build")
