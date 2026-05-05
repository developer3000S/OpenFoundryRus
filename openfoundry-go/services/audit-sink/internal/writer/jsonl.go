package writer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/envelope"
)

// JSONLWriter appends batches as newline-delimited JSON.
//
// Path "-" writes to stdout (useful for kubectl logs in dev). Any
// other value opens / appends to the file with mode 0640. The writer
// is goroutine-safe: a single batch flush holds the mutex.
type JSONLWriter struct {
	mu     sync.Mutex
	w      io.WriteCloser
	stdout bool
}

// NewJSONLWriter opens path for append. Pass "-" for stdout.
func NewJSONLWriter(path string) (*JSONLWriter, error) {
	if path == "-" {
		return &JSONLWriter{w: nopCloser{os.Stdout}, stdout: true}, nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, fmt.Errorf("open jsonl sink %s: %w", path, err)
	}
	return &JSONLWriter{w: f}, nil
}

// Append implements Writer.
func (w *JSONLWriter) Append(_ context.Context, batch []envelope.AuditEnvelope) error {
	if len(batch) == 0 {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	enc := json.NewEncoder(w.w)
	for i := range batch {
		if err := enc.Encode(&batch[i]); err != nil {
			return fmt.Errorf("encode audit envelope: %w", err)
		}
	}
	if f, ok := w.w.(*os.File); ok {
		return f.Sync()
	}
	return nil
}

// Close implements Writer.
func (w *JSONLWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stdout {
		return nil
	}
	return w.w.Close()
}

// nopCloser is for stdout — never close the std handle.
type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }
