package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	kafka "github.com/segmentio/kafka-go"
)

type recordingIndexBackend struct {
	err    error
	calls  int
	topics []string
	events []json.RawMessage
}

func (b *recordingIndexBackend) Handle(_ context.Context, topic string, event json.RawMessage) error {
	b.calls++
	b.topics = append(b.topics, topic)
	b.events = append(b.events, append(json.RawMessage(nil), event...))
	return b.err
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

func newSubscribedKafkaReader(t *testing.T, brokers []string, group string, topics []string) KafkaReader {
	t.Helper()
	r := NewKafkaReader(brokers, group, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.Subscribe(ctx, topics); err != nil {
		t.Fatalf("subscribe kafka reader: %v", err)
	}
	return r
}

func fetchOneKafkaMessage(t *testing.T, brokers []string, topic, group string, timeout time.Duration) (KafkaMessage, error) {
	t.Helper()
	r := newSubscribedKafkaReader(t, brokers, group, []string{topic})
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

func TestConsumerWithRealKafkaValidMalformedAndRetry(t *testing.T) {
	brokers := kafkaBrokersOrSkip(t)

	t.Run("valid event commits after backend handling", func(t *testing.T) {
		topic := "of-ontology-index-valid-" + uuid.NewString()
		group := "of-ontology-index-" + uuid.NewString()
		createKafkaTopic(t, brokers, topic)
		body := []byte(`{"event_type":"ontology.object.changed.v1","object_id":"obj-1","op":"upsert"}`)
		produceKafkaMessage(t, brokers, topic, body)

		backend := &recordingIndexBackend{}
		consumer := &Consumer{Reader: newSubscribedKafkaReader(t, brokers, group, []string{topic}), Backend: backend}
		t.Cleanup(func() { _ = consumer.Close() })
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := consumer.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce valid: %v", err)
		}
		if backend.calls != 1 || len(backend.events) != 1 || backend.topics[0] != topic {
			t.Fatalf("backend calls drift: calls=%d topics=%v events=%d", backend.calls, backend.topics, len(backend.events))
		}
		_ = consumer.Close()
		expectKafkaGroupCommitted(t, brokers, topic, group)
	})

	t.Run("malformed event commits without backend handling", func(t *testing.T) {
		topic := "of-ontology-index-malformed-" + uuid.NewString()
		group := "of-ontology-index-" + uuid.NewString()
		createKafkaTopic(t, brokers, topic)
		body := []byte(`{"event_type":`)
		produceKafkaMessage(t, brokers, topic, body)

		backend := &recordingIndexBackend{}
		consumer := &Consumer{Reader: newSubscribedKafkaReader(t, brokers, group, []string{topic}), Backend: backend}
		t.Cleanup(func() { _ = consumer.Close() })
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := consumer.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce malformed: %v", err)
		}
		if backend.calls != 0 {
			t.Fatalf("malformed event called backend %d times", backend.calls)
		}
		_ = consumer.Close()
		expectKafkaGroupCommitted(t, brokers, topic, group)
	})

	t.Run("backend error leaves offset uncommitted for retry", func(t *testing.T) {
		topic := "of-ontology-index-retry-" + uuid.NewString()
		group := "of-ontology-index-" + uuid.NewString()
		createKafkaTopic(t, brokers, topic)
		body := []byte(`{"event_type":"ontology.object.changed.v1","object_id":"obj-2","op":"upsert"}`)
		produceKafkaMessage(t, brokers, topic, body)

		consumer := &Consumer{Reader: newSubscribedKafkaReader(t, brokers, group, []string{topic}), Backend: &recordingIndexBackend{err: errors.New("backend down")}}
		t.Cleanup(func() { _ = consumer.Close() })
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := consumer.RunOnce(ctx)
		if err == nil || !strings.Contains(err.Error(), "backend down") {
			t.Fatalf("expected backend error, got %v", err)
		}
		_ = consumer.Close()
		expectKafkaGroupReplays(t, brokers, topic, group, body)
	})
}
