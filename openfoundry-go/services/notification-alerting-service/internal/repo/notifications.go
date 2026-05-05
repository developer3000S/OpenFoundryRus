package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/models"
)

// NotificationsRepo wraps the SQL surface for `notifications` /
// `notification_deliveries`. Goroutine-safe (pgxpool).
type NotificationsRepo struct{ Pool *pgxpool.Pool }

// Insert creates a new notification row.
func (r *NotificationsRepo) Insert(
	ctx context.Context,
	id uuid.UUID,
	userID *uuid.UUID,
	title, body, category, severity string,
	channels []string,
	metadata json.RawMessage,
) (*models.NotificationRecord, error) {
	if metadata == nil {
		metadata = json.RawMessage(`{}`)
	}
	chJSON, err := json.Marshal(channels)
	if err != nil {
		return nil, fmt.Errorf("encode channels: %w", err)
	}

	row := r.Pool.QueryRow(ctx,
		`INSERT INTO notifications (
			id, user_id, title, body, category, severity, status, channels, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, 'unread', $7, $8)
		RETURNING id, user_id, title, body, category, severity, status, channels, metadata, created_at, read_at`,
		id, userID, title, body, category, severity, chJSON, metadata,
	)
	return scanNotification(row)
}

// FindByID returns the notification (when visible to user).
func (r *NotificationsRepo) FindByID(ctx context.Context, id, userID uuid.UUID) (*models.NotificationRecord, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT id, user_id, title, body, category, severity, status, channels, metadata, created_at, read_at
		 FROM notifications
		 WHERE id = $1 AND (user_id = $2 OR user_id IS NULL)`,
		id, userID,
	)
	return scanNotification(row)
}

// List returns up to `limit` notifications for the caller.
//
// Filters by status when non-empty. Includes both user-targeted and
// global (user_id IS NULL) notifications, matching the Rust query.
func (r *NotificationsRepo) List(ctx context.Context, userID uuid.UUID, status *string, limit int64) ([]models.NotificationRecord, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, user_id, title, body, category, severity, status, channels, metadata, created_at, read_at
		 FROM notifications
		 WHERE (user_id = $1 OR user_id IS NULL)
		   AND ($2::TEXT IS NULL OR status = $2)
		 ORDER BY created_at DESC
		 LIMIT $3`,
		userID, status, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.NotificationRecord, 0)
	for rows.Next() {
		n, err := scanNotificationRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// MarkRead sets status='read' + read_at=NOW() and returns the updated row.
//
// Returns nil, nil when the notification was not found / not visible.
func (r *NotificationsRepo) MarkRead(ctx context.Context, id, userID uuid.UUID) (*models.NotificationRecord, error) {
	row := r.Pool.QueryRow(ctx,
		`UPDATE notifications
		 SET status = 'read', read_at = NOW()
		 WHERE id = $1 AND (user_id = $2 OR user_id IS NULL)
		 RETURNING id, user_id, title, body, category, severity, status, channels, metadata, created_at, read_at`,
		id, userID,
	)
	n, err := scanNotification(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return n, err
}

// MarkAllRead clears the unread flag for everything visible to the caller.
func (r *NotificationsRepo) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	_, err := r.Pool.Exec(ctx,
		`UPDATE notifications
		 SET status = 'read', read_at = NOW()
		 WHERE status = 'unread' AND (user_id = $1 OR user_id IS NULL)`,
		userID,
	)
	return err
}

// UnreadCount returns the count of unread notifications visible to the caller.
func (r *NotificationsRepo) UnreadCount(ctx context.Context, userID *uuid.UUID) (int64, error) {
	var n int64
	err := r.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications
		 WHERE status = 'unread'
		   AND (($1::UUID IS NULL AND user_id IS NULL) OR user_id = $1 OR user_id IS NULL)`,
		userID,
	).Scan(&n)
	return n, err
}

// Latest returns the most recent N notifications for the websocket snapshot.
func (r *NotificationsRepo) Latest(ctx context.Context, userID uuid.UUID, limit int64) ([]models.NotificationRecord, error) {
	rows, err := r.Pool.Query(ctx,
		`SELECT id, user_id, title, body, category, severity, status, channels, metadata, created_at, read_at
		 FROM notifications
		 WHERE user_id = $1 OR user_id IS NULL
		 ORDER BY created_at DESC
		 LIMIT $2`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.NotificationRecord, 0)
	for rows.Next() {
		n, err := scanNotificationRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// RecordDelivery inserts one row in `notification_deliveries`.
func (r *NotificationsRepo) RecordDelivery(
	ctx context.Context,
	id, notificationID uuid.UUID,
	channel, status string,
	response *string,
) (*models.NotificationDelivery, error) {
	row := r.Pool.QueryRow(ctx,
		`INSERT INTO notification_deliveries (id, notification_id, channel, status, response)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, notification_id, channel, status, response, created_at`,
		id, notificationID, channel, status, response,
	)
	d := &models.NotificationDelivery{}
	if err := row.Scan(&d.ID, &d.NotificationID, &d.Channel, &d.Status, &d.Response, &d.CreatedAt); err != nil {
		return nil, err
	}
	return d, nil
}

// row scanner used by both QueryRow and Query rows.
type rowLike interface {
	Scan(dest ...any) error
}

func scanNotification(row pgx.Row) (*models.NotificationRecord, error) {
	return scanNotificationRows(row)
}

func scanNotificationRows(r rowLike) (*models.NotificationRecord, error) {
	n := &models.NotificationRecord{}
	if err := r.Scan(
		&n.ID, &n.UserID, &n.Title, &n.Body, &n.Category, &n.Severity,
		&n.Status, &n.Channels, &n.Metadata, &n.CreatedAt, &n.ReadAt,
	); err != nil {
		return nil, err
	}
	return n, nil
}
