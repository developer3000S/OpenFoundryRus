package writer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/openfoundry/openfoundry-go/services/ai-sink/internal/envelope"
)

// JSONLWriter persists each Iceberg table to a separate
// `<table>.jsonl` file inside `dir`. Files are opened lazily on first
// Append and reused for the lifetime of the writer.
type JSONLWriter struct {
	dir   string
	mu    sync.Mutex
	files map[string]*os.File
}

// NewJSONLWriter creates `dir` if missing and returns a writer that
// owns the per-table files inside it.
func NewJSONLWriter(dir string) (*JSONLWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return &JSONLWriter{dir: dir, files: make(map[string]*os.File)}, nil
}

// Append implements Writer.
func (w *JSONLWriter) Append(_ context.Context, byTable map[string][]envelope.AiEventEnvelope) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for table, batch := range byTable {
		if len(batch) == 0 {
			continue
		}
		f, err := w.fileFor(table)
		if err != nil {
			return err
		}
		enc := json.NewEncoder(f)
		for i := range batch {
			if err := enc.Encode(&batch[i]); err != nil {
				return fmt.Errorf("encode ai envelope (%s): %w", table, err)
			}
		}
		if err := f.Sync(); err != nil {
			return fmt.Errorf("fsync %s: %w", f.Name(), err)
		}
	}
	return nil
}

func (w *JSONLWriter) fileFor(table string) (*os.File, error) {
	if f, ok := w.files[table]; ok {
		return f, nil
	}
	path := filepath.Join(w.dir, table+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	w.files[table] = f
	return f, nil
}

// Close implements Writer.
func (w *JSONLWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	var firstErr error
	for table, f := range w.files {
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("close %s: %w", table, err)
		}
		delete(w.files, table)
	}
	return firstErr
}
