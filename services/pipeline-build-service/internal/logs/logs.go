// Package logs contains the pipeline-build-service live-log ports used by the
// HTTP SSE handler. The package is intentionally storage-agnostic: production
// can back LogStore with Postgres and LogSubscriber with an in-memory/pubsub
// broadcaster, while tests use the in-memory implementation below.
package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// LogLevel is the canonical live-log severity vocabulary. JSON shape mirrors
// the Rust enum's UPPERCASE representation.
type LogLevel string

const (
	LogTrace LogLevel = "TRACE"
	LogDebug LogLevel = "DEBUG"
	LogInfo  LogLevel = "INFO"
	LogWarn  LogLevel = "WARN"
	LogError LogLevel = "ERROR"
	LogFatal LogLevel = "FATAL"
)

// ParseLogLevel accepts the Rust aliases used by the REST/SSE query parser.
func ParseLogLevel(s string) (LogLevel, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "TRACE":
		return LogTrace, true
	case "DEBUG":
		return LogDebug, true
	case "INFO":
		return LogInfo, true
	case "WARN", "WARNING":
		return LogWarn, true
	case "ERROR":
		return LogError, true
	case "FATAL":
		return LogFatal, true
	default:
		return "", false
	}
}

// LogEntry is the Rust-compatible SSE/history payload. SSE emits this as the
// JSON data of `event: log` after omitting JobRID.
type LogEntry struct {
	Sequence int64           `json:"sequence"`
	JobRID   string          `json:"job_rid,omitempty"`
	TS       time.Time       `json:"ts"`
	Level    LogLevel        `json:"level"`
	Message  string          `json:"message"`
	Params   json.RawMessage `json:"params,omitempty"`
}

// RowDTO is the exact JSON shape sent by Rust for `event: log`.
type RowDTO struct {
	Sequence int64           `json:"sequence"`
	TS       time.Time       `json:"ts"`
	Level    string          `json:"level"`
	Message  string          `json:"message"`
	Params   json.RawMessage `json:"params,omitempty"`
}

func (e LogEntry) RowDTO() RowDTO {
	return RowDTO{Sequence: e.Sequence, TS: e.TS, Level: string(e.Level), Message: e.Message, Params: e.Params}
}

// Query contains the history/follow filters shared by REST and SSE.
type Query struct {
	FromSequence int64
	Since        *time.Time
	Until        *time.Time
	Levels       []LogLevel
	Limit        int64
	Follow       bool
}

func (q Query) allows(entry LogEntry) bool {
	if entry.Sequence < q.FromSequence {
		return false
	}
	if q.Since != nil && entry.TS.Before(*q.Since) {
		return false
	}
	if q.Until != nil && !entry.TS.Before(*q.Until) {
		return false
	}
	if len(q.Levels) > 0 {
		for _, level := range q.Levels {
			if entry.Level == level {
				return true
			}
		}
		return false
	}
	return true
}

// LogStore returns persisted catch-up history for a job RID.
type LogStore interface {
	History(ctx context.Context, jobRID string, query Query) ([]LogEntry, error)
}

// LogSubscriber tails live entries for a job RID. The returned cancel function
// releases subscription resources; it must be safe to call multiple times.
type LogSubscriber interface {
	Subscribe(ctx context.Context, jobRID string) (<-chan LogEntry, func(), error)
}

// Service bundles the two ports needed by StreamJobLogs.
type Service struct {
	Store      LogStore
	Subscriber LogSubscriber
}

// MemoryService is a deterministic fake suitable for httptest and local wiring.
type MemoryService struct {
	mu          sync.Mutex
	history     map[string][]LogEntry
	subscribers map[string]map[chan LogEntry]struct{}
	nextSeq     map[string]int64
}

func NewMemoryService() *MemoryService {
	return &MemoryService{history: map[string][]LogEntry{}, subscribers: map[string]map[chan LogEntry]struct{}{}, nextSeq: map[string]int64{}}
}

func (m *MemoryService) History(ctx context.Context, jobRID string, query Query) ([]LogEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entries := append([]LogEntry(nil), m.history[jobRID]...)
	out := make([]LogEntry, 0, len(entries))
	for _, entry := range entries {
		if query.allows(entry) {
			out = append(out, entry)
			if query.Limit > 0 && int64(len(out)) >= query.Limit {
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out, nil
}

func (m *MemoryService) Subscribe(ctx context.Context, jobRID string) (<-chan LogEntry, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	ch := make(chan LogEntry, 16)
	m.mu.Lock()
	if m.subscribers[jobRID] == nil {
		m.subscribers[jobRID] = map[chan LogEntry]struct{}{}
	}
	m.subscribers[jobRID][ch] = struct{}{}
	m.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			m.mu.Lock()
			delete(m.subscribers[jobRID], ch)
			m.mu.Unlock()
			close(ch)
		})
	}
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return ch, cancel, nil
}

func (m *MemoryService) Emit(jobRID string, level LogLevel, message string, params json.RawMessage) LogEntry {
	m.mu.Lock()
	m.nextSeq[jobRID]++
	entry := LogEntry{Sequence: m.nextSeq[jobRID], JobRID: jobRID, TS: time.Now().UTC(), Level: level, Message: message, Params: params}
	m.history[jobRID] = append(m.history[jobRID], entry)
	subs := make([]chan LogEntry, 0, len(m.subscribers[jobRID]))
	for ch := range m.subscribers[jobRID] {
		subs = append(subs, ch)
	}
	m.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- entry:
		default:
		}
	}
	return entry
}

func (m *MemoryService) AppendLogByRID(ctx context.Context, jobRID string, level LogLevel, message string, params json.RawMessage) (LogEntry, error) {
	if err := ctx.Err(); err != nil {
		return LogEntry{}, err
	}
	return m.Emit(jobRID, level, message, params), nil
}

// ErrUnavailable is a convenient sentinel for adapters/fakes.
type ErrUnavailable struct{ Cause error }

func (e *ErrUnavailable) Error() string { return fmt.Sprintf("log store unavailable: %v", e.Cause) }
func (e *ErrUnavailable) Unwrap() error { return e.Cause }
