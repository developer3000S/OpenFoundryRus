//go:build integration

package runtime_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	kafka "github.com/segmentio/kafka-go"

	databus "github.com/openfoundry/openfoundry-go/libs/event-bus-data"
	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/config"
	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/envelope"
	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/runtime"
)

type kafkaAuditWriter struct {
	mu      sync.Mutex
	batches [][]envelope.AuditEnvelope
	err     error
}

func (w *kafkaAuditWriter) Append(_ context.Context, batch []envelope.AuditEnvelope) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return w.err
	}
	cp := make([]envelope.AuditEnvelope, len(batch))
	copy(cp, batch)
	w.batches = append(w.batches, cp)
	return nil
}
func (w *kafkaAuditWriter) Close() error { return nil }
func (w *kafkaAuditWriter) calls() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.batches)
}

func kafkaBrokersOrSkip(t *testing.T) []string {
	t.Helper()
	bootstrap := strings.TrimSpace(os.Getenv("KAFKA_BOOTSTRAP_SERVERS"))
	if bootstrap == "" {
		t.Skip("KAFKA_BOOTSTRAP_SERVERS not set; skipping real Kafka consumer integration test")
	}
	return strings.Split(bootstrap, ",")
}

func createKafkaTopic(t *testing.T, brokers []string, topic string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := kafka.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		t.Fatalf("dial kafka: %v", err)
	}
	defer conn.Close()
	_ = conn.CreateTopics(kafka.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1})
}

func produceKafkaMessage(t *testing.T, brokers []string, topic string, value []byte) {
	t.Helper()
	w := &kafka.Writer{Addr: kafka.TCP(brokers...), Topic: topic, RequiredAcks: kafka.RequireAll, Balancer: &kafka.LeastBytes{}}
	defer w.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := w.WriteMessages(ctx, kafka.Message{Key: []byte(uuid.NewString()), Value: value}); err != nil {
		t.Fatalf("produce kafka message: %v", err)
	}
}

func newKafkaSubscriber(t *testing.T, brokers []string, topic, group string) *databus.KafkaSubscriber {
	t.Helper()
	cfg := databus.NewConfig(brokers, databus.InsecureDev("audit-sink-test"))
	sub, err := databus.NewKafkaSubscriber(cfg, group, []string{topic})
	if err != nil {
		t.Fatalf("new kafka subscriber: %v", err)
	}
	return sub
}

func fetchOneKafkaMessage(t *testing.T, brokers []string, topic, group string, timeout time.Duration) (kafka.Message, error) {
	t.Helper()
	r := kafka.NewReader(kafka.ReaderConfig{Brokers: brokers, GroupID: group, Topic: topic, CommitInterval: 0, MinBytes: 1, MaxBytes: 10e6, MaxWait: 100 * time.Millisecond})
	defer r.Close()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return r.FetchMessage(ctx)
}

func expectKafkaGroupCommitted(t *testing.T, brokers []string, topic, group string) {
	t.Helper()
	msg, err := fetchOneKafkaMessage(t, brokers, topic, group, 750*time.Millisecond)
	if err == nil {
		t.Fatalf("expected committed group offset for %s/%s, but re-read offset %d value=%s", topic, group, msg.Offset, msg.Value)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline while checking committed offset, got %v", err)
	}
}

func expectKafkaGroupReplays(t *testing.T, brokers []string, topic, group string, want []byte) {
	t.Helper()
	msg, err := fetchOneKafkaMessage(t, brokers, topic, group, 5*time.Second)
	if err != nil {
		t.Fatalf("expected uncommitted message replay, got %v", err)
	}
	if string(msg.Value) != string(want) {
		t.Fatalf("replayed value = %s, want %s", msg.Value, want)
	}
}

func runAuditSinkOnce(t *testing.T, brokers []string, topic, group string, w *kafkaAuditWriter) error {
	t.Helper()
	sub := newKafkaSubscriber(t, brokers, topic, group)
	defer sub.Close()
	cfg := &config.Config{BatchPolicy: config.BatchPolicy{MaxRecords: 1, MaxWait: 100 * time.Millisecond}}
	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	return runtime.Run(ctx, cfg, sub, w, runtime.NewMetrics(), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestRuntimeWithRealKafkaValidMalformedAndRetry(t *testing.T) {
	brokers := kafkaBrokersOrSkip(t)

	t.Run("valid event writes and commits", func(t *testing.T) {
		topic := "of-audit-sink-valid-" + uuid.NewString()
		group := "of-audit-sink-" + uuid.NewString()
		createKafkaTopic(t, brokers, topic)
		body := mkEnvelopeBytes(t, time.Now().Add(-time.Second).UnixMicro(), "audit.valid")
		produceKafkaMessage(t, brokers, topic, body)

		w := &kafkaAuditWriter{}
		if err := runAuditSinkOnce(t, brokers, topic, group, w); err != nil {
			t.Fatalf("Run valid: %v", err)
		}
		if w.calls() != 1 {
			t.Fatalf("writer calls = %d, want 1", w.calls())
		}
		expectKafkaGroupCommitted(t, brokers, topic, group)
	})

	t.Run("malformed event commits without writer", func(t *testing.T) {
		topic := "of-audit-sink-malformed-" + uuid.NewString()
		group := "of-audit-sink-" + uuid.NewString()
		createKafkaTopic(t, brokers, topic)
		body := []byte(`{"event_id":`)
		produceKafkaMessage(t, brokers, topic, body)

		w := &kafkaAuditWriter{}
		if err := runAuditSinkOnce(t, brokers, topic, group, w); err != nil {
			t.Fatalf("Run malformed: %v", err)
		}
		if w.calls() != 0 {
			t.Fatalf("malformed event wrote %d batches", w.calls())
		}
		expectKafkaGroupCommitted(t, brokers, topic, group)
	})

	t.Run("writer error leaves offset uncommitted for retry", func(t *testing.T) {
		topic := "of-audit-sink-retry-" + uuid.NewString()
		group := "of-audit-sink-" + uuid.NewString()
		createKafkaTopic(t, brokers, topic)
		body := mkEnvelopeBytes(t, time.Now().Add(-time.Second).UnixMicro(), "audit.retry")
		produceKafkaMessage(t, brokers, topic, body)

		err := runAuditSinkOnce(t, brokers, topic, group, &kafkaAuditWriter{err: errors.New("writer down")})
		if err == nil || !strings.Contains(err.Error(), "writer down") {
			t.Fatalf("expected writer error, got %v", err)
		}
		expectKafkaGroupReplays(t, brokers, topic, group, body)
	})
}
