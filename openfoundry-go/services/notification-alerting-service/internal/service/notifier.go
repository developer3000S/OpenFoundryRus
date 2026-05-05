package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	controlbus "github.com/openfoundry/openfoundry-go/libs/event-bus-control"
	"github.com/openfoundry/openfoundry-go/libs/core-models/ids"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/repo"
)

// Notifier composes notification creation, channel dispatch and event
// publishing. The HTTP handlers + websocket hub call into Notifier.
type Notifier struct {
	Notifications *repo.NotificationsRepo
	Preferences   *repo.PreferencesRepo
	SMTP          *SMTPSender
	HTTP          *http.Client
	Bus           *NotificationBus // nil when NATS is unconfigured
}

// Create persists the notification, dispatches every channel, records
// the per-channel deliveries, and (best-effort) publishes a
// `notification.created` event on NATS.
//
// Mirrors `handlers::send::create_notification` in Rust.
func (n *Notifier) Create(ctx context.Context, body models.SendNotificationRequest) (*models.NotificationRecord, error) {
	channels := body.Channels
	if len(channels) == 0 {
		channels = []string{"in_app"}
	}
	metadata := body.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	severity := "info"
	if body.Severity != nil && *body.Severity != "" {
		severity = *body.Severity
	}
	category := "system"
	if body.Category != nil && *body.Category != "" {
		category = *body.Category
	}

	notification, err := n.Notifications.Insert(
		ctx,
		ids.New(),
		body.UserID,
		body.Title, body.Body, category, severity,
		channels, metadata,
	)
	if err != nil {
		return nil, fmt.Errorf("insert notification: %w", err)
	}

	var preference *models.NotificationPreference
	if body.UserID != nil {
		preference, err = n.Preferences.FindByUser(ctx, *body.UserID)
		if err != nil {
			slog.Warn("load preferences failed", slog.String("error", err.Error()))
			preference = nil
		}
	}

	for _, ch := range channels {
		result := n.dispatch(ctx, notification, preference, ch)
		response := result.Response
		var responsePtr *string
		if response != "" {
			responsePtr = &response
		}
		if _, err := n.Notifications.RecordDelivery(ctx, ids.New(), notification.ID, ch, result.Status, responsePtr); err != nil {
			slog.Warn("record delivery failed",
				slog.String("channel", ch),
				slog.String("error", err.Error()))
		}
	}

	unread, _ := n.Notifications.UnreadCount(ctx, notification.UserID)
	if n.Bus != nil {
		evt := controlbus.NotificationEvent{
			Kind:        "notification.created",
			UserID:      notification.UserID,
			UnreadCount: unread,
		}
		if rb, err := json.Marshal(notification); err == nil {
			evt.Notification = rb
		}
		if err := n.Bus.Publish(ctx, evt); err != nil {
			slog.Warn("publish notification.created failed", slog.String("error", err.Error()))
		}
	}

	return notification, nil
}

// PublishRead emits notification.read / notification.read_all events.
func (n *Notifier) PublishRead(ctx context.Context, kind string, userID *uuid.UUID, notification *models.NotificationRecord, unread int64) {
	if n.Bus == nil {
		return
	}
	evt := controlbus.NotificationEvent{
		Kind:        kind,
		UserID:      userID,
		UnreadCount: unread,
	}
	if notification != nil {
		if rb, err := json.Marshal(notification); err == nil {
			evt.Notification = rb
		}
	}
	if err := n.Bus.Publish(ctx, evt); err != nil {
		slog.Warn("publish notification event failed",
			slog.String("kind", kind),
			slog.String("error", err.Error()))
	}
}
