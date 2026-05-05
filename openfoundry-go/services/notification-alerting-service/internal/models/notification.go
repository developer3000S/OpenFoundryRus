// Package models holds the wire types for notification-alerting-service.
//
// JSON tag set + field types match the Rust crate verbatim so the
// frontend, edge-gateway and event-bus consumers do not see a wire
// difference between the two implementations.
package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// NotificationRecord is the row in `notifications` and the wire payload
// returned by the list / mark_read endpoints.
type NotificationRecord struct {
	ID        uuid.UUID       `json:"id"`
	UserID    *uuid.UUID      `json:"user_id"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	Category  string          `json:"category"`
	Severity  string          `json:"severity"`
	Status    string          `json:"status"`
	Channels  json.RawMessage `json:"channels"`
	Metadata  json.RawMessage `json:"metadata"`
	CreatedAt time.Time       `json:"created_at"`
	ReadAt    *time.Time      `json:"read_at"`
}

// NotificationDelivery is the row in `notification_deliveries`.
type NotificationDelivery struct {
	ID             uuid.UUID `json:"id"`
	NotificationID uuid.UUID `json:"notification_id"`
	Channel        string    `json:"channel"`
	Status         string    `json:"status"`
	Response       *string   `json:"response,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// SendNotificationRequest is the body of POST /notifications/send and POST /internal/notifications.
type SendNotificationRequest struct {
	UserID   *uuid.UUID      `json:"user_id"`
	Title    string          `json:"title"`
	Body     string          `json:"body"`
	Severity *string         `json:"severity,omitempty"`
	Category *string         `json:"category,omitempty"`
	Channels []string        `json:"channels,omitempty"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// ListNotificationsQuery models the GET /notifications query string.
type ListNotificationsQuery struct {
	Status *string
	Limit  int64
}

// WebSocketTicketResponse is the response of POST /notifications/ws-ticket.
type WebSocketTicketResponse struct {
	Ticket    string `json:"ticket"`
	ExpiresIn int64  `json:"expires_in"`
}
