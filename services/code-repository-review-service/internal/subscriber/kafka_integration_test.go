//go:build integration

package subscriber

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

type errorPort struct{ err error }

func (p errorPort) Handle(context.Context, json.RawMessage) error { return p.err }

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

func newTestKafkaReader(brokers []string, topic, group string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		GroupID:        group,
		Topic:          topic,
		CommitInterval: 0,
		MinBytes:       1,
		MaxBytes:       10e6,
		MaxWait:        100 * time.Millisecond,
	})
}

func fetchOneKafkaMessage(t *testing.T, brokers []string, topic, group string, timeout time.Duration) (kafka.Message, error) {
	t.Helper()
	r := newTestKafkaReader(brokers, topic, group)
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

	t.Run("valid event commits after port handling", func(t *testing.T) {
		topic := "of-code-review-valid-" + uuid.NewString()
		group := "of-code-review-" + uuid.NewString()
		createKafkaTopic(t, brokers, topic)
		body := []byte(`{"event_type":"dataset.branch.restored.v1","branch_rid":"ri.branch.valid"}`)
		produceKafkaMessage(t, brokers, topic, body)

		port := &recordingPort{}
		consumer := &Consumer{Reader: newTestKafkaReader(brokers, topic, group), Port: port}
		t.Cleanup(func() { _ = consumer.Close() })
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := consumer.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce valid: %v", err)
		}
		if port.called != 1 || len(port.events) != 1 {
			t.Fatalf("port calls drift: called=%d events=%d", port.called, len(port.events))
		}
		_ = consumer.Close()
		expectKafkaGroupCommitted(t, brokers, topic, group)
	})

	t.Run("malformed event commits without port handling", func(t *testing.T) {
		topic := "of-code-review-malformed-" + uuid.NewString()
		group := "of-code-review-" + uuid.NewString()
		createKafkaTopic(t, brokers, topic)
		body := []byte(`{"event_type":`)
		produceKafkaMessage(t, brokers, topic, body)

		port := &recordingPort{}
		consumer := &Consumer{Reader: newTestKafkaReader(brokers, topic, group), Port: port}
		t.Cleanup(func() { _ = consumer.Close() })
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := consumer.RunOnce(ctx); err != nil {
			t.Fatalf("RunOnce malformed: %v", err)
		}
		if port.called != 0 {
			t.Fatalf("malformed event called port %d times", port.called)
		}
		_ = consumer.Close()
		expectKafkaGroupCommitted(t, brokers, topic, group)
	})

	t.Run("backend error leaves offset uncommitted for retry", func(t *testing.T) {
		topic := "of-code-review-retry-" + uuid.NewString()
		group := "of-code-review-" + uuid.NewString()
		createKafkaTopic(t, brokers, topic)
		body := []byte(`{"event_type":"dataset.branch.restored.v1","branch_rid":"ri.branch.retry"}`)
		produceKafkaMessage(t, brokers, topic, body)

		consumer := &Consumer{Reader: newTestKafkaReader(brokers, topic, group), Port: errorPort{err: errors.New("backend down")}}
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
