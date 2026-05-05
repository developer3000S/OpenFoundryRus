# notification-alerting-service (Go)

Notification inbox + delivery + websocket fan-out. Functional 1:1 with
the Rust crate.

## Endpoints

| Method | Path                                              | Auth                | Purpose                          |
| ------ | ------------------------------------------------- | ------------------- | -------------------------------- |
| GET    | `/healthz`                                        | —                   | liveness                         |
| GET    | `/metrics`                                        | —                   | Prometheus scrape                |
| GET    | `/api/v1/notifications/ws`                        | ticket query param  | WebSocket upgrade (snapshot + live stream) |
| GET    | `/api/v1/notifications`                           | bearer JWT          | list (filter by `?status=`)      |
| PATCH  | `/api/v1/notifications/{id}/read`                 | bearer JWT          | mark one read                    |
| POST   | `/api/v1/notifications/read-all`                  | bearer JWT          | mark every visible unread read   |
| GET    | `/api/v1/notifications/preferences`               | bearer JWT          | per-user preferences             |
| PUT    | `/api/v1/notifications/preferences`               | bearer JWT          | update preferences               |
| POST   | `/api/v1/notifications/ws-ticket`                 | bearer JWT          | mint short-lived (90s) WS ticket |
| POST   | `/api/v1/notifications/send`                      | bearer JWT          | send (defaults user_id to caller)|
| POST   | `/internal/notifications`                         | none — restrict by NetworkPolicy | internal sender   |

## Channels

`in_app` (always), `email` (SMTP via stdlib), `slack` (webhook),
`teams` (webhook). The send handler dispatches each channel listed on
the request and records a row in `notification_deliveries` per attempt.

## NATS fan-out

When `NATS_URL` is set, every state change publishes a
`NotificationEvent` to `of.notifications.notification-alerting-service`
on stream `OF_NOTIFICATIONS`. The websocket handler subscribes per-user
and forwards events the client cares about (filtered by `user_id`).

## Configuration

Operator-facing env contract — names match the Rust crate:

| Variable                       | Required | Purpose                              |
| ------------------------------ | :------: | ------------------------------------ |
| `DATABASE_URL`                 | ✅       | Postgres connection string           |
| `JWT_SECRET` (or `OPENFOUNDRY_JWT_SECRET`) | ✅ | HS256 secret                |
| `NATS_URL`                     |          | enables websocket fan-out            |
| `HOST` / `PORT`                |          | default `0.0.0.0:50114`              |
| `SMTP_HOST` / `SMTP_PORT`      |          | enables email channel                |
| `SMTP_USERNAME` / `SMTP_PASSWORD` |        | SMTP auth                            |
| `SMTP_FROM_ADDRESS` / `SMTP_FROM_NAME` |    | sender identity                     |
| `METRICS_ADDR`                 |          | default `0.0.0.0:9090`               |
| `OTEL_TRACES_EXPORTER=none`    |          | disable tracing                      |

## Schema

Three tables under the configured Postgres database. Migrations are
embedded at `internal/repo/migrations/*.sql` and applied at startup
(idempotent `CREATE TABLE IF NOT EXISTS`):

- `notifications` (id, user_id, title, body, category, severity, status, channels, metadata, created_at, read_at)
- `notification_deliveries` (id, notification_id, channel, status, response, created_at)
- `notification_preferences` (user_id, in_app_enabled, email_enabled, email_address, slack_webhook_url, teams_webhook_url, digest_frequency, quiet_hours, updated_at)

## Build / run

```sh
make build-services   # produces ./bin/notification-alerting-service
DATABASE_URL=postgres://localhost/notif \
JWT_SECRET=$(openssl rand -hex 32) \
NATS_URL=nats://localhost:4222 \
OTEL_TRACES_EXPORTER=none \
./bin/notification-alerting-service
```

## Wire-compat invariants

- `NotificationRecord` JSON shape (snake_case).
- `NotificationPreference` JSON shape.
- `NotificationEvent` envelope (kind, user_id, notification, unread_count).
- `WebSocketTicketResponse` JSON.
- `/healthz` payload.
