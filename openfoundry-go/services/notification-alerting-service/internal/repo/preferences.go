package repo

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/models"
)

// PreferencesRepo wraps `notification_preferences`.
type PreferencesRepo struct{ Pool *pgxpool.Pool }

// FindByUser returns the row for `userID`. Returns nil, nil when absent.
func (r *PreferencesRepo) FindByUser(ctx context.Context, userID uuid.UUID) (*models.NotificationPreference, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT user_id, in_app_enabled, email_enabled, email_address,
		        slack_webhook_url, teams_webhook_url, digest_frequency, quiet_hours, updated_at
		 FROM notification_preferences WHERE user_id = $1`,
		userID,
	)
	p := &models.NotificationPreference{}
	if err := row.Scan(
		&p.UserID, &p.InAppEnabled, &p.EmailEnabled, &p.EmailAddress,
		&p.SlackWebhookURL, &p.TeamsWebhookURL, &p.DigestFrequency, &p.QuietHours, &p.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return p, nil
}

// Upsert applies the given preference values, returning the persisted row.
//
// Caller is expected to have merged optional update fields with current values.
func (r *PreferencesRepo) Upsert(ctx context.Context, p *models.NotificationPreference) (*models.NotificationPreference, error) {
	if p.QuietHours == nil {
		p.QuietHours = json.RawMessage(`{}`)
	}
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO notification_preferences (
			user_id, in_app_enabled, email_enabled, email_address,
			slack_webhook_url, teams_webhook_url, digest_frequency, quiet_hours
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id) DO UPDATE SET
			in_app_enabled    = EXCLUDED.in_app_enabled,
			email_enabled     = EXCLUDED.email_enabled,
			email_address     = EXCLUDED.email_address,
			slack_webhook_url = EXCLUDED.slack_webhook_url,
			teams_webhook_url = EXCLUDED.teams_webhook_url,
			digest_frequency  = EXCLUDED.digest_frequency,
			quiet_hours       = EXCLUDED.quiet_hours,
			updated_at        = NOW()
		RETURNING user_id, in_app_enabled, email_enabled, email_address,
		          slack_webhook_url, teams_webhook_url, digest_frequency, quiet_hours, updated_at`,
		p.UserID, p.InAppEnabled, p.EmailEnabled, p.EmailAddress,
		p.SlackWebhookURL, p.TeamsWebhookURL, p.DigestFrequency, p.QuietHours,
	)
	out := &models.NotificationPreference{}
	if err := row.Scan(
		&out.UserID, &out.InAppEnabled, &out.EmailEnabled, &out.EmailAddress,
		&out.SlackWebhookURL, &out.TeamsWebhookURL, &out.DigestFrequency, &out.QuietHours, &out.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return out, nil
}

// Default returns the in-memory default preferences for a user with no
// stored row. Mirrors the Rust `load_or_default_preferences` fallback.
func Default(userID uuid.UUID) *models.NotificationPreference {
	return &models.NotificationPreference{
		UserID:          userID,
		InAppEnabled:    true,
		EmailEnabled:    false,
		DigestFrequency: "instant",
		QuietHours:      json.RawMessage(`{}`),
	}
}
