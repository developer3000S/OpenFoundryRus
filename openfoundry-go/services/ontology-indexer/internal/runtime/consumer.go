package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
)

// RawIndexBackend is a small compatibility boundary used by Kafka
// integration tests that only assert offset semantics. The production
// runtime uses ProcessMessage with a SearchBackend.
type RawIndexBackend interface {
	Handle(ctx context.Context, topic string, event json.RawMessage) error
}

// Consumer reads ontology events through the package KafkaMessage
// abstraction, invokes Backend, and commits offsets only after durable
// backend processing. Malformed JSON is committed and skipped so one poison
// event cannot wedge a partition.
type Consumer struct {
	Reader  KafkaReader
	Backend RawIndexBackend
	Log     *slog.Logger
}

// Run consumes until ctx is canceled. Non-cancellation errors return so
// supervisors can restart and retry from the last committed offset.
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

// RunOnce fetches and processes one Kafka message.
func (c *Consumer) RunOnce(ctx context.Context) error {
	if c.Reader == nil {
		return errors.New("ontology indexer consumer requires a reader")
	}
	if c.Backend == nil {
		return errors.New("ontology indexer consumer requires a backend")
	}
	msg, err := c.Reader.FetchMessage(ctx)
	if err != nil {
		return err
	}
	if !json.Valid(msg.Value) {
		if c.Log != nil {
			c.Log.Warn("malformed ontology index event skipped", slog.String("topic", msg.Topic), slog.Int64("offset", msg.Offset))
		}
		return c.Reader.CommitMessages(ctx, msg)
	}
	if err := c.Backend.Handle(ctx, msg.Topic, json.RawMessage(msg.Value)); err != nil {
		return fmt.Errorf("handle ontology index event %s/%d@%d: %w", msg.Topic, msg.Partition, msg.Offset, err)
	}
	if err := c.Reader.CommitMessages(ctx, msg); err != nil {
		return fmt.Errorf("commit ontology index event %s/%d@%d: %w", msg.Topic, msg.Partition, msg.Offset, err)
	}
	return nil
}

func (c *Consumer) Close() error {
	if c == nil || c.Reader == nil {
		return nil
	}
	return c.Reader.Close()
}
