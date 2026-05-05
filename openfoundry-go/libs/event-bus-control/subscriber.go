package controlbus

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// Default stream knobs — match the Rust crate's `ensure_stream` impl.
const (
	defaultStreamMaxMessages = 1_000_000
	defaultStreamMaxAge      = 7 * 24 * time.Hour
)

// EnsureStream creates the JetStream stream `name` if it does not
// exist, otherwise returns the existing one.
//
// Each subject in `subjects` is registered with a `.>` suffix so the
// stream captures the entire subtree (e.g. `of.datasets.>` matches
// `of.datasets.created`, `of.datasets.quality.refresh.requested`, …).
// Same convention as the Rust crate.
func EnsureStream(ctx context.Context, js jetstream.JetStream, name string, subjects []string) (jetstream.Stream, error) {
	wildcards := make([]string, len(subjects))
	for i, s := range subjects {
		wildcards[i] = s + ".>"
	}
	cfg := jetstream.StreamConfig{
		Name:      name,
		Subjects:  wildcards,
		Retention: jetstream.LimitsPolicy,
		MaxMsgs:   defaultStreamMaxMessages,
		MaxAge:    defaultStreamMaxAge,
	}
	stream, err := js.CreateOrUpdateStream(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("ensure stream %q: %w", name, err)
	}
	return stream, nil
}

// CreateConsumer creates (or returns the existing) durable pull
// consumer on `stream`. `filterSubject` is optional; when empty, the
// consumer receives every subject the stream covers.
func CreateConsumer(ctx context.Context, stream jetstream.Stream, name, filterSubject string) (jetstream.Consumer, error) {
	cfg := jetstream.ConsumerConfig{
		Durable:       name,
		FilterSubject: filterSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
	}
	cons, err := stream.CreateOrUpdateConsumer(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create consumer %q: %w", name, err)
	}
	return cons, nil
}

// ErrSubscribe wraps consumer-side failures with the operation name.
type ErrSubscribe struct {
	Op    string
	Cause error
}

func (e *ErrSubscribe) Error() string { return e.Op + ": " + e.Cause.Error() }
func (e *ErrSubscribe) Unwrap() error { return e.Cause }

// IsSubscribeError reports whether err originated from controlbus.
func IsSubscribeError(err error) bool {
	var se *ErrSubscribe
	return errors.As(err, &se)
}
