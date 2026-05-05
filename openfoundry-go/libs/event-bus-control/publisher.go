package controlbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Connect opens a NATS connection and returns a JetStream context.
//
// The caller owns the underlying *nats.Conn lifecycle through the
// returned closer. Match the Rust `connect()` helper's contract.
func Connect(ctx context.Context, natsURL string) (jetstream.JetStream, func(), error) {
	nc, err := nats.Connect(natsURL,
		nats.Name("openfoundry-control-bus"),
		nats.Timeout(5*time.Second),
		nats.MaxReconnects(-1),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("nats connect: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("jetstream context: %w", err)
	}
	closer := func() { nc.Drain(); nc.Close() }
	_ = ctx // ctx is reserved for future cancellation hooks
	return js, closer, nil
}

// Publisher wraps a JetStream context for typed event publishing.
//
// Source is the producing service name and lands in every Event.Source
// field — used by audit / lineage downstream.
type Publisher struct {
	JS     jetstream.JetStream
	Source string
}

// NewPublisher returns a Publisher.
func NewPublisher(js jetstream.JetStream, source string) *Publisher {
	return &Publisher{JS: js, Source: source}
}

// Publish marshals payload into an Event envelope and publishes to subject.
func (p *Publisher) Publish(ctx context.Context, subject, eventType string, payload any) error {
	evt, err := NewEvent(eventType, p.Source, payload)
	if err != nil {
		return fmt.Errorf("encode payload: %w", err)
	}
	body, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	if _, err := p.JS.Publish(ctx, subject, body); err != nil {
		return fmt.Errorf("jetstream publish %q: %w", subject, err)
	}
	slog.Debug("event published",
		slog.String("subject", subject),
		slog.String("event_type", eventType),
	)
	return nil
}
