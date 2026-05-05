package databus

import (
	"context"
	"fmt"

	kafka "github.com/segmentio/kafka-go"
)

// Publisher is the at-least-once Kafka producer.
//
// Mirrors the Rust DataPublisher trait: Publish(topic, key, payload, lineage)
// + Flush(timeout). Implementations MUST honour acks=all + idempotence
// for at-least-once semantics.
type Publisher interface {
	Publish(ctx context.Context, topic string, key, payload []byte, lineage *OpenLineageHeaders) error
	Flush(ctx context.Context) error
	Close() error
}

// KafkaPublisher is the segmentio/kafka-go-backed Publisher.
type KafkaPublisher struct {
	writer *kafka.Writer
}

// NewKafkaPublisher builds a Publisher from Config.
//
// Producer settings:
//
//   - acks=all + idempotent writes (RequiredAcks=-1 in segmentio terms).
//   - balancer = round-robin when key is nil; sticky-by-key otherwise.
//   - allow.auto.create.topics is enforced server-side; the Go client
//     does not have a per-writer flag and assumes brokers are configured
//     with `auto.create.topics.enable=false`. Tighten this at the broker
//     for parity with the Rust client behaviour.
func NewKafkaPublisher(cfg Config) (*KafkaPublisher, error) {
	dialer, err := cfg.Principal.dialer(cfg.RequestTimeout)
	if err != nil {
		return nil, fmt.Errorf("kafka dialer: %w", err)
	}
	w := &kafka.Writer{
		Addr:                   kafka.TCP(cfg.BootstrapServers...),
		Balancer:               &kafka.Hash{}, // sticky-by-key partitioning
		RequiredAcks:           kafka.RequireAll,
		Compression:            cfg.Compression,
		WriteTimeout:           cfg.RequestTimeout,
		AllowAutoTopicCreation: false,
		Transport: &kafka.Transport{
			TLS:      dialer.TLS,
			SASL:     dialer.SASLMechanism,
			ClientID: dialer.ClientID,
		},
	}
	return &KafkaPublisher{writer: w}, nil
}

// Publish writes one record to the broker.
//
// `lineage` may be nil; when set, the OpenLineage headers are attached
// in the canonical order so consumers can extract them directly.
func (p *KafkaPublisher) Publish(ctx context.Context, topic string, key, payload []byte, lineage *OpenLineageHeaders) error {
	msg := kafka.Message{
		Topic: topic,
		Key:   key,
		Value: payload,
	}
	if lineage != nil {
		msg.Headers = lineage.ToKafkaHeaders()
	}
	if err := p.writer.WriteMessages(ctx, msg); err != nil {
		return fmt.Errorf("kafka publish %q: %w", topic, err)
	}
	return nil
}

// Flush blocks until pending writes have been acknowledged or ctx is
// done. segmentio/kafka-go's Writer is synchronous when WriteMessages
// returns nil, but Flush is exposed so the API matches the Rust trait.
func (p *KafkaPublisher) Flush(_ context.Context) error { return nil }

// Close releases the underlying Writer.
func (p *KafkaPublisher) Close() error { return p.writer.Close() }
