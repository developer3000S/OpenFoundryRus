package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	databus "github.com/openfoundry/openfoundry-go/libs/event-bus-data"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/config"
	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/envelope"
	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/runtime"
)

// stubSubscriber yields a queue of pre-built messages and records every
// CommitMessages call so tests can assert on offset-commit batches.
type stubSubscriber struct {
	mu       sync.Mutex
	queue    [][]byte // each entry is the JSON body of one message
	commits  [][]int64 // outer slice = commit calls, inner = committed offsets
	closed   bool
}

func (s *stubSubscriber) Poll(ctx context.Context) (*databus.DataMessage, error) {
	s.mu.Lock()
	if len(s.queue) == 0 {
		s.mu.Unlock()
		<-ctx.Done()
		return nil, ctx.Err()
	}
	body := s.queue[0]
	s.queue = s.queue[1:]
	offset := int64(len(s.commits)*1000 + (1000 - len(s.queue)))
	s.mu.Unlock()
	return &databus.DataMessage{
		Topic:     config.SourceTopic,
		Partition: 0,
		Offset:    offset,
		Value:     body,
	}, nil
}

func (s *stubSubscriber) CommitMessages(_ context.Context, msgs []*databus.DataMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	offsets := make([]int64, len(msgs))
	for i, m := range msgs {
		offsets[i] = m.Offset
	}
	s.commits = append(s.commits, offsets)
	return nil
}

func (s *stubSubscriber) CommitOffsets(_ context.Context) error { return nil }
func (s *stubSubscriber) Close() error                          { s.closed = true; return nil }

// captureWriter records every Append call.
type captureWriter struct {
	mu      sync.Mutex
	batches [][]envelope.AuditEnvelope
	failOn  int // 1-indexed: fail the Nth Append call
	calls   int
}

func (c *captureWriter) Append(_ context.Context, batch []envelope.AuditEnvelope) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.failOn != 0 && c.calls == c.failOn {
		return errors.New("simulated writer failure")
	}
	cp := make([]envelope.AuditEnvelope, len(batch))
	copy(cp, batch)
	c.batches = append(c.batches, cp)
	return nil
}
func (c *captureWriter) Close() error { return nil }

func mkEnvelopeBytes(t *testing.T, at int64, kind string) []byte {
	t.Helper()
	body, err := json.Marshal(envelope.AuditEnvelope{
		EventID: uuid.New(),
		At:      at,
		Kind:    kind,
		Payload: json.RawMessage(`null`),
	})
	require.NoError(t, err)
	return body
}

func newConfig(maxRecords int, maxWait time.Duration) *config.Config {
	c := &config.Config{
		BatchPolicy: config.BatchPolicy{MaxRecords: maxRecords, MaxWait: maxWait},
	}
	c.Service.Name = "audit-sink"
	c.Service.Version = "test"
	return c
}

func TestBatchPolicyFlushesOnSize(t *testing.T) {
	t.Parallel()
	p := config.BatchPolicy{MaxRecords: 100_000, MaxWait: time.Minute}
	assert.True(t, p.ShouldFlush(100_000, time.Second))
	assert.False(t, p.ShouldFlush(99_999, time.Second))
}

func TestBatchPolicyFlushesOnTime(t *testing.T) {
	t.Parallel()
	p := config.BatchPolicy{MaxRecords: 100_000, MaxWait: time.Minute}
	assert.True(t, p.ShouldFlush(0, time.Minute))
	assert.False(t, p.ShouldFlush(0, 59*time.Second))
}

func TestEnvelopeDecodeAndPoison(t *testing.T) {
	t.Parallel()
	corr := "abc"
	want := envelope.AuditEnvelope{
		EventID:       uuid.New(),
		At:            1_700_000_000_000_000,
		CorrelationID: &corr,
		Kind:          "Login",
		Payload:       json.RawMessage(`{"outcome":"success"}`),
	}
	body, _ := json.Marshal(&want)
	got, err := envelope.Decode(body)
	require.NoError(t, err)
	assert.Equal(t, want.Kind, got.Kind)
	assert.Equal(t, want.At, got.At)

	_, err = envelope.Decode([]byte("not-json"))
	require.Error(t, err)
	var de *envelope.DecodeError
	assert.True(t, errors.As(err, &de))
}

func TestRuntimeFlushesOnSize(t *testing.T) {
	t.Parallel()
	cfg := newConfig(3, 5*time.Second)
	log := observability.InitLogging("audit-sink", "test")
	w := &captureWriter{}
	m := runtime.NewMetrics()

	sub := &stubSubscriber{
		queue: [][]byte{
			mkEnvelopeBytes(t, 1, "Login"),
			mkEnvelopeBytes(t, 2, "Login"),
			mkEnvelopeBytes(t, 3, "Logout"),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx, cfg, sub, w, m, log) }()

	require.Eventually(t, func() bool {
		w.mu.Lock(); defer w.mu.Unlock()
		return len(w.batches) == 1
	}, 3*time.Second, 20*time.Millisecond, "writer should receive the size-triggered batch")

	cancel()
	<-done

	w.mu.Lock(); defer w.mu.Unlock()
	require.Len(t, w.batches, 1)
	assert.Len(t, w.batches[0], 3)
	sub.mu.Lock(); defer sub.mu.Unlock()
	require.GreaterOrEqual(t, len(sub.commits), 1)
	assert.Len(t, sub.commits[0], 3, "all 3 offsets committed in one CommitMessages call")
}

func TestRuntimeFlushesOnTime(t *testing.T) {
	t.Parallel()
	cfg := newConfig(1000, 200*time.Millisecond)
	log := observability.InitLogging("audit-sink", "test")
	w := &captureWriter{}
	m := runtime.NewMetrics()

	sub := &stubSubscriber{queue: [][]byte{mkEnvelopeBytes(t, 1, "Login")}}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx, cfg, sub, w, m, log) }()

	require.Eventually(t, func() bool {
		w.mu.Lock(); defer w.mu.Unlock()
		return len(w.batches) >= 1
	}, 3*time.Second, 20*time.Millisecond, "writer should flush after MaxWait elapses")

	cancel()
	<-done

	w.mu.Lock(); defer w.mu.Unlock()
	require.GreaterOrEqual(t, len(w.batches), 1)
	assert.Len(t, w.batches[0], 1)
}

func TestRuntimeSkipsPoisonRecordsButCommitsOffset(t *testing.T) {
	t.Parallel()
	cfg := newConfig(3, 5*time.Second)
	log := observability.InitLogging("audit-sink", "test")
	w := &captureWriter{}
	m := runtime.NewMetrics()

	sub := &stubSubscriber{
		queue: [][]byte{
			mkEnvelopeBytes(t, 1, "Login"),
			[]byte("not-json"),               // poison
			mkEnvelopeBytes(t, 3, "Logout"),
		},
	}

	cfg.BatchPolicy.MaxRecords = 2 // 2 valid records flush after the second valid arrives
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx, cfg, sub, w, m, log) }()

	require.Eventually(t, func() bool {
		w.mu.Lock(); defer w.mu.Unlock()
		return len(w.batches) == 1
	}, 3*time.Second, 20*time.Millisecond)

	cancel()
	<-done

	w.mu.Lock(); defer w.mu.Unlock()
	require.Len(t, w.batches, 1)
	assert.Len(t, w.batches[0], 2, "writer sees only the 2 valid records")
	sub.mu.Lock(); defer sub.mu.Unlock()
	require.GreaterOrEqual(t, len(sub.commits), 1)
	assert.Len(t, sub.commits[0], 3, "commit advances past poison too (3 offsets)")
}

func TestRuntimeWriterFailureAborts(t *testing.T) {
	t.Parallel()
	cfg := newConfig(2, 5*time.Second)
	log := observability.InitLogging("audit-sink", "test")
	w := &captureWriter{failOn: 1}
	m := runtime.NewMetrics()

	sub := &stubSubscriber{
		queue: [][]byte{
			mkEnvelopeBytes(t, 1, "Login"),
			mkEnvelopeBytes(t, 2, "Login"),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx, cfg, sub, w, m, log) }()

	select {
	case err := <-done:
		require.Error(t, err, "Run must return when Writer.Append fails")
	case <-time.After(3 * time.Second):
		t.Fatal("runtime should have returned an error")
	}

	sub.mu.Lock(); defer sub.mu.Unlock()
	assert.Empty(t, sub.commits, "no commits when the writer rejects the batch")
}
