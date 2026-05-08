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
	"github.com/openfoundry/openfoundry-go/services/ai-sink/internal/config"
	"github.com/openfoundry/openfoundry-go/services/ai-sink/internal/envelope"
	"github.com/openfoundry/openfoundry-go/services/ai-sink/internal/runtime"
)

type stubSubscriber struct {
	mu      sync.Mutex
	queue   [][]byte
	commits [][]int64
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
		Topic: config.SourceTopic, Partition: 0, Offset: offset, Value: body,
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
func (s *stubSubscriber) Close() error                          { return nil }

type captureWriter struct {
	mu      sync.Mutex
	batches []map[string][]envelope.AiEventEnvelope
	failOn  int
	calls   int
}

func (c *captureWriter) Append(_ context.Context, byTable map[string][]envelope.AiEventEnvelope) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if c.failOn != 0 && c.calls == c.failOn {
		return errors.New("simulated writer failure")
	}
	cp := make(map[string][]envelope.AiEventEnvelope, len(byTable))
	for k, v := range byTable {
		if len(v) == 0 {
			continue
		}
		dup := make([]envelope.AiEventEnvelope, len(v))
		copy(dup, v)
		cp[k] = dup
	}
	c.batches = append(c.batches, cp)
	return nil
}
func (c *captureWriter) Close() error { return nil }

// blockingWriter lets tests prove Kafka offsets are not committed until
// Writer.Append returns successfully.
type blockingWriter struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingWriter() *blockingWriter {
	return &blockingWriter{started: make(chan struct{}), release: make(chan struct{})}
}

func (b *blockingWriter) Append(ctx context.Context, byTable map[string][]envelope.AiEventEnvelope) error {
	b.once.Do(func() { close(b.started) })
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *blockingWriter) Close() error { return nil }

func mkBytes(t *testing.T, kind envelope.AiEventKind, at int64) []byte {
	t.Helper()
	body, err := json.Marshal(envelope.AiEventEnvelope{
		EventID: uuid.New(), At: at, Kind: kind,
		Producer: "agent-runtime-service", SchemaVersion: 1,
		Payload: json.RawMessage(`null`),
	})
	require.NoError(t, err)
	return body
}

func newConfig(maxRecords int, maxWait time.Duration) *config.Config {
	c := &config.Config{
		BatchPolicy: config.BatchPolicy{MaxRecords: maxRecords, MaxWait: maxWait},
	}
	c.Service.Name = "ai-sink"
	c.Service.Version = "test"
	return c
}

func TestRouteByKind(t *testing.T) {
	t.Parallel()
	cases := []struct {
		k    envelope.AiEventKind
		want string
	}{
		{envelope.KindPrompt, envelope.TablePrompts},
		{envelope.KindResponse, envelope.TableResponses},
		{envelope.KindEvaluation, envelope.TableEvaluations},
		{envelope.KindTrace, envelope.TableTraces},
	}
	for _, c := range cases {
		env := envelope.AiEventEnvelope{Kind: c.k}
		got, err := envelope.Route(&env)
		require.NoError(t, err)
		assert.Equal(t, c.want, got)
	}
}

func TestRouteUnknownKind(t *testing.T) {
	t.Parallel()
	env := envelope.AiEventEnvelope{Kind: "garbage"}
	_, err := envelope.Route(&env)
	assert.ErrorIs(t, err, envelope.ErrUnknownKind)
}

func TestBatchPolicyThresholds(t *testing.T) {
	t.Parallel()
	p := config.BatchPolicy{MaxRecords: 100_000, MaxWait: time.Minute}
	assert.True(t, p.ShouldFlush(100_000, time.Second))
	assert.False(t, p.ShouldFlush(99_999, time.Second))
	assert.True(t, p.ShouldFlush(0, time.Minute))
	assert.False(t, p.ShouldFlush(0, 59*time.Second))
}

func TestRuntimeRoutesAcrossFourTables(t *testing.T) {
	t.Parallel()
	cfg := newConfig(4, 5*time.Second)
	log := observability.InitLogging("ai-sink", "test")
	w := &captureWriter{}
	m := runtime.NewMetrics()

	sub := &stubSubscriber{
		queue: [][]byte{
			mkBytes(t, envelope.KindPrompt, 1),
			mkBytes(t, envelope.KindResponse, 2),
			mkBytes(t, envelope.KindEvaluation, 3),
			mkBytes(t, envelope.KindTrace, 4),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx, cfg, sub, w, m, log) }()

	require.Eventually(t, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return len(w.batches) == 1
	}, 3*time.Second, 20*time.Millisecond)

	cancel()
	<-done

	w.mu.Lock()
	defer w.mu.Unlock()
	require.Len(t, w.batches, 1)
	got := w.batches[0]
	assert.Len(t, got[envelope.TablePrompts], 1)
	assert.Len(t, got[envelope.TableResponses], 1)
	assert.Len(t, got[envelope.TableEvaluations], 1)
	assert.Len(t, got[envelope.TableTraces], 1)
	sub.mu.Lock()
	defer sub.mu.Unlock()
	require.GreaterOrEqual(t, len(sub.commits), 1)
	assert.Len(t, sub.commits[0], 4, "all 4 offsets committed in one CommitMessages call")
}

func TestRuntimeSkipsPoisonAndUnknownKindButCommits(t *testing.T) {
	t.Parallel()
	cfg := newConfig(2, 5*time.Second)
	log := observability.InitLogging("ai-sink", "test")
	w := &captureWriter{}
	m := runtime.NewMetrics()

	// Build one record with unknown kind (decodes but Route fails).
	unknownKindBody, _ := json.Marshal(envelope.AiEventEnvelope{
		EventID: uuid.New(), At: 100, Kind: "garbage",
		Producer: "p", SchemaVersion: 1, Payload: json.RawMessage(`null`),
	})

	sub := &stubSubscriber{
		queue: [][]byte{
			mkBytes(t, envelope.KindPrompt, 1),
			[]byte("not-json"), // poison
			unknownKindBody,    // unknown kind
			mkBytes(t, envelope.KindResponse, 4),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx, cfg, sub, w, m, log) }()

	require.Eventually(t, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return len(w.batches) == 1
	}, 3*time.Second, 20*time.Millisecond)

	cancel()
	<-done

	w.mu.Lock()
	defer w.mu.Unlock()
	got := w.batches[0]
	assert.Len(t, got[envelope.TablePrompts], 1)
	assert.Len(t, got[envelope.TableResponses], 1)
	sub.mu.Lock()
	defer sub.mu.Unlock()
	require.GreaterOrEqual(t, len(sub.commits), 1)
	assert.Len(t, sub.commits[0], 4, "commit advances past poison + unknown-kind too")
}

func TestRuntimeCommitsOffsetsOnlyAfterAllTableAppendsSucceed(t *testing.T) {
	t.Parallel()
	cfg := newConfig(1, 5*time.Second)
	log := observability.InitLogging("ai-sink", "test")
	w := newBlockingWriter()
	m := runtime.NewMetrics()
	sub := &stubSubscriber{queue: [][]byte{mkBytes(t, envelope.KindPrompt, 1)}}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx, cfg, sub, w, m, log) }()

	select {
	case <-w.started:
	case <-time.After(3 * time.Second):
		t.Fatal("writer append did not start")
	}

	sub.mu.Lock()
	commitsWhileAppendBlocked := len(sub.commits)
	sub.mu.Unlock()
	assert.Zero(t, commitsWhileAppendBlocked, "offsets must not be committable while table append is still in flight")

	close(w.release)
	require.Eventually(t, func() bool {
		sub.mu.Lock()
		defer sub.mu.Unlock()
		return len(sub.commits) == 1 && len(sub.commits[0]) == 1
	}, 3*time.Second, 20*time.Millisecond, "offset should be committed after append succeeds")

	cancel()
	<-done
}

func TestRuntimeWriterFailureDoesNotCommitOffsets(t *testing.T) {
	t.Parallel()
	cfg := newConfig(1, 5*time.Second)
	log := observability.InitLogging("ai-sink", "test")
	w := &captureWriter{failOn: 1}
	m := runtime.NewMetrics()
	sub := &stubSubscriber{queue: [][]byte{mkBytes(t, envelope.KindPrompt, 1)}}

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

	sub.mu.Lock()
	defer sub.mu.Unlock()
	assert.Empty(t, sub.commits, "no commits when any table append fails")
}
