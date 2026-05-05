package databus

import (
	"context"
	"errors"
	"fmt"

	kafka "github.com/segmentio/kafka-go"
)

// DataMessage is one Kafka record exposed to the consumer.
//
// Commit() must be called after the record has been durably processed.
// Auto-commit is off — same contract as the Rust trait.
type DataMessage struct {
	Topic     string
	Partition int
	Offset    int64
	Key       []byte
	Value     []byte
	Headers   []kafka.Header
	Lineage   *OpenLineageHeaders // populated when present in headers

	// internal
	reader *kafka.Reader
	raw    kafka.Message
}

// Commit acknowledges this record's offset to Kafka.
func (m *DataMessage) Commit(ctx context.Context) error {
	if m.reader == nil {
		return errors.New("databus: DataMessage has no associated reader (already committed?)")
	}
	if err := m.reader.CommitMessages(ctx, m.raw); err != nil {
		return fmt.Errorf("commit %s/%d@%d: %w", m.Topic, m.Partition, m.Offset, err)
	}
	return nil
}

// Subscriber is the at-least-once Kafka consumer.
//
// Mirrors the Rust DataSubscriber trait but with batch-commit semantics
// that match segmentio/kafka-go's model:
//
//	for {
//	    msg, err := sub.Poll(ctx)
//	    if err != nil { ... }
//	    batch = append(batch, msg)
//	    if shouldFlush() {
//	        process(batch)
//	        sub.CommitMessages(ctx, batch)
//	    }
//	}
type Subscriber interface {
	Poll(ctx context.Context) (*DataMessage, error)
	// CommitMessages commits the highest offset per topic+partition
	// represented in `msgs`. Pass the slice of every message that
	// has been durably processed since the last commit.
	CommitMessages(ctx context.Context, msgs []*DataMessage) error
	// CommitOffsets is a legacy single-call alternative that commits
	// every offset the underlying reader has seen. Kept for parity
	// with the Rust trait shape; new code should prefer CommitMessages.
	CommitOffsets(ctx context.Context) error
	Close() error
}

// KafkaSubscriber is the segmentio/kafka-go-backed Subscriber.
type KafkaSubscriber struct {
	reader *kafka.Reader
}

// NewKafkaSubscriber wires a Subscriber to one or more topics inside
// the named consumer group. Auto-commit is disabled.
func NewKafkaSubscriber(cfg Config, groupID string, topics []string) (*KafkaSubscriber, error) {
	if len(topics) == 0 {
		return nil, errors.New("databus: at least one topic is required")
	}
	dialer, err := cfg.Principal.dialer(cfg.RequestTimeout)
	if err != nil {
		return nil, fmt.Errorf("kafka dialer: %w", err)
	}
	rcfg := kafka.ReaderConfig{
		Brokers:        cfg.BootstrapServers,
		GroupID:        groupID,
		GroupTopics:    topics,
		Dialer:         dialer,
		IsolationLevel: kafka.ReadCommitted,
		// Auto-commit OFF — caller drives commits explicitly.
		CommitInterval: 0,
	}
	return &KafkaSubscriber{reader: kafka.NewReader(rcfg)}, nil
}

// Poll fetches the next record. Blocks until one arrives or ctx is done.
func (s *KafkaSubscriber) Poll(ctx context.Context) (*DataMessage, error) {
	raw, err := s.reader.FetchMessage(ctx)
	if err != nil {
		return nil, err
	}
	msg := &DataMessage{
		Topic:     raw.Topic,
		Partition: raw.Partition,
		Offset:    raw.Offset,
		Key:       raw.Key,
		Value:     raw.Value,
		Headers:   raw.Headers,
		reader:    s.reader,
		raw:       raw,
	}
	if lineage, ok := OpenLineageHeadersFromKafka(raw.Headers); ok {
		msg.Lineage = &lineage
	}
	return msg, nil
}

// CommitMessages implements Subscriber. Hands the raw messages to the
// underlying segmentio reader, which collapses them to one offset per
// topic+partition before sending the OffsetCommit RPC.
func (s *KafkaSubscriber) CommitMessages(ctx context.Context, msgs []*DataMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	raw := make([]kafka.Message, 0, len(msgs))
	for _, m := range msgs {
		if m == nil {
			continue
		}
		raw = append(raw, m.raw)
	}
	if err := s.reader.CommitMessages(ctx, raw...); err != nil {
		return fmt.Errorf("commit %d messages: %w", len(raw), err)
	}
	return nil
}

// CommitOffsets is a legacy alternative kept for parity with the Rust
// trait. segmentio/kafka-go has no per-reader bulk commit, so this is
// a no-op; new callers should prefer CommitMessages.
func (s *KafkaSubscriber) CommitOffsets(_ context.Context) error { return nil }

// Close releases the underlying Reader.
func (s *KafkaSubscriber) Close() error { return s.reader.Close() }
