// Package writer abstracts the audit-sink output target.
//
// Two implementations:
//
//   - JSONLWriter — append batches as newline-delimited JSON to a
//     local file (or stdout when path is "-"). Production-suitable
//     for staging / observability while the Iceberg writer matures.
//   - IcebergWriter — stub. The Rust crate uses iceberg-rust's
//     `append_record_batches`. Apache iceberg-go's write API is still
//     unstable as of this commit, so the stub returns ErrNotImplemented
//     and a TODO points at the iceberg-go upstream issue. JSONLWriter
//     is the default until the upstream API stabilises.
//
// The Writer interface is intentionally narrow: append + close. Batch
// policy + Kafka offset commits live in `internal/runtime`.
package writer

import (
	"context"
	"errors"

	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/envelope"
)

// Writer is the audit-sink output target.
type Writer interface {
	// Append durably writes `batch` and returns when it is safe for
	// the caller to commit Kafka offsets.
	Append(ctx context.Context, batch []envelope.AuditEnvelope) error
	// Close flushes pending data and releases resources. Safe to
	// call multiple times.
	Close() error
}

// ErrNotImplemented signals that the Iceberg writer is not wired up
// in this build. Callers (runtime) translate this into a fatal-on-boot
// instead of a per-batch failure so the operator notices early.
var ErrNotImplemented = errors.New("writer not implemented in this build")
