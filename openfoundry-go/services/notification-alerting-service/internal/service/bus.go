package service

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"

	controlbus "github.com/openfoundry/openfoundry-go/libs/event-bus-control"
)

// NotificationBus is the per-service NATS publisher + stream handle.
//
// Subject: `of.notifications.notification-alerting-service`. Stream:
// OF_NOTIFICATIONS. The websocket hub subscribes to the same subject
// to fan out events to connected clients.
type NotificationBus struct {
	JS        jetstream.JetStream
	Stream    jetstream.Stream
	Publisher *controlbus.Publisher
	Subject   string
}

// NewNotificationBus dials NATS, ensures the stream and returns a bus.
//
// Caller owns the closer (returned by controlbus.Connect).
func NewNotificationBus(ctx context.Context, natsURL, sourceService string) (*NotificationBus, func(), error) {
	js, closer, err := controlbus.Connect(ctx, natsURL)
	if err != nil {
		return nil, nil, err
	}
	stream, err := controlbus.EnsureStream(ctx, js, controlbus.StreamNotifications,
		[]string{controlbus.SubjectNotifications})
	if err != nil {
		closer()
		return nil, nil, err
	}
	subject := controlbus.SubjectNotifications + "." + sourceService
	return &NotificationBus{
		JS:        js,
		Stream:    stream,
		Publisher: controlbus.NewPublisher(js, sourceService),
		Subject:   subject,
	}, closer, nil
}

// Publish emits a notification event on the bus.
func (b *NotificationBus) Publish(ctx context.Context, evt controlbus.NotificationEvent) error {
	return b.Publisher.Publish(ctx, b.Subject, evt.Kind, evt)
}
