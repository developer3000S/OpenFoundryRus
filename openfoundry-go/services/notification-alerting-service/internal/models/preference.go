package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// NotificationPreference is the row in `notification_preferences` and
// the wire payload of GET/PUT /notifications/preferences.
type NotificationPreference struct {
	UserID            uuid.UUID       `json:"user_id"`
	InAppEnabled      bool            `json:"in_app_enabled"`
	EmailEnabled      bool            `json:"email_enabled"`
	EmailAddress      *string         `json:"email_address"`
	SlackWebhookURL   *string         `json:"slack_webhook_url"`
	TeamsWebhookURL   *string         `json:"teams_webhook_url"`
	DigestFrequency   string          `json:"digest_frequency"`
	QuietHours        json.RawMessage `json:"quiet_hours"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// UpdatePreferenceRequest is the body of PUT /notifications/preferences.
//
// All fields optional — unset fields keep their current value.
type UpdatePreferenceRequest struct {
	InAppEnabled    *bool           `json:"in_app_enabled,omitempty"`
	EmailEnabled    *bool           `json:"email_enabled,omitempty"`
	EmailAddress    *string         `json:"email_address,omitempty"`
	SlackWebhookURL *string         `json:"slack_webhook_url,omitempty"`
	TeamsWebhookURL *string         `json:"teams_webhook_url,omitempty"`
	DigestFrequency *string         `json:"digest_frequency,omitempty"`
	QuietHours      json.RawMessage `json:"quiet_hours,omitempty"`
}
