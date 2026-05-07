package subscriber

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	kafka "github.com/segmentio/kafka-go"
)

const DefaultConsumerGroup = "code-repository-review-service.branch-events"

type KafkaReader interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

type Consumer struct {
	Reader KafkaReader
	Port   SubscriberPort
	Log    *slog.Logger
}

func NewKafkaReader(brokers []string, groupID string) *kafka.Reader {
	if groupID == "" {
		groupID = DefaultConsumerGroup
	}
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		GroupID:        groupID,
		Topic:          Topic,
		CommitInterval: 0,
		MinBytes:       1,
		MaxBytes:       10e6,
		MaxWait:        time.Second,
	})
}

func NewConsumer(brokers []string, groupID string, port SubscriberPort, log *slog.Logger) *Consumer {
	return &Consumer{Reader: NewKafkaReader(brokers, groupID), Port: port, Log: log}
}

// Run consumes branch events until ctx is canceled. Each successfully handled
// message is committed after the SubscriberPort returns. Malformed JSON is
// committed and skipped so one bad event cannot poison the consumer group.
func (c *Consumer) Run(ctx context.Context) error {
	for {
		if err := c.RunOnce(ctx); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return err
		}
	}
}

// RunOnce fetches and processes one Kafka message. It is intentionally public
// so tests can exercise the kafka-go loop without a Kafka broker.
func (c *Consumer) RunOnce(ctx context.Context) error {
	if c.Reader == nil {
		return errors.New("subscriber consumer requires a reader")
	}
	if c.Port == nil {
		return errors.New("subscriber consumer requires a subscriber port")
	}
	msg, err := c.Reader.FetchMessage(ctx)
	if err != nil {
		return err
	}
	if !json.Valid(msg.Value) {
		if c.Log != nil {
			c.Log.Warn("malformed branch event skipped", slog.String("topic", msg.Topic), slog.Int64("offset", msg.Offset))
		}
		return c.Reader.CommitMessages(ctx, msg)
	}
	if err := c.Port.Handle(ctx, json.RawMessage(msg.Value)); err != nil {
		return fmt.Errorf("handle branch event %s/%d@%d: %w", msg.Topic, msg.Partition, msg.Offset, err)
	}
	if err := c.Reader.CommitMessages(ctx, msg); err != nil {
		return fmt.Errorf("commit branch event %s/%d@%d: %w", msg.Topic, msg.Partition, msg.Offset, err)
	}
	return nil
}

func (c *Consumer) Close() error {
	if c == nil || c.Reader == nil {
		return nil
	}
	return c.Reader.Close()
}
