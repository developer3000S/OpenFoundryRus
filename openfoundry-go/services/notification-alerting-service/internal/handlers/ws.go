package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/core-models/ids"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/service"
)

const wsTicketTTL = 90 * time.Second

// WS wires the websocket-ticket issuer + the websocket upgrade endpoint.
type WS struct {
	JWT           *authmw.JWTConfig
	Notifications *repo.NotificationsRepo
	Bus           *service.NotificationBus // nil when NATS is unconfigured
}

// IssueTicket handles POST /api/v1/notifications/ws-ticket.
//
// Issues a short-lived (90 s) JWT with `token_use=ws_ticket` so the
// browser can upgrade to a websocket without juggling the long-lived
// access token in URL params. Mirrors the Rust impl.
func (h *WS) IssueTicket(w http.ResponseWriter, r *http.Request) {
	c := authmw.MustFromContext(r.Context())
	now := time.Now()

	ticketUse := "ws_ticket"
	ticketClaims := &authmw.Claims{
		Sub:           c.Sub,
		IAT:           now.Unix(),
		EXP:           now.Add(wsTicketTTL).Unix(),
		ISS:           c.ISS,
		AUD:           c.AUD,
		JTI:           ids.New(),
		Email:         c.Email,
		Name:          c.Name,
		Roles:         []string{},
		Permissions:   []string{},
		OrgID:         c.OrgID,
		Attributes:    json.RawMessage(`{}`),
		AuthMethods:   []string{"ws_ticket"},
		TokenUse:      &ticketUse,
		SessionKind:   c.SessionKind,
		SessionScope:  c.SessionScope,
	}

	tok, err := authmw.EncodeToken(h.JWT, ticketClaims)
	if err != nil {
		slog.Error("encode ws ticket failed", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":     tok,
		"expires_in": int64(wsTicketTTL.Seconds()),
	})
}

// Upgrade handles GET /api/v1/notifications/ws.
//
// Authenticates via the `ticket` query param (a short-lived JWT issued
// by IssueTicket). On success, writes a snapshot frame + streams every
// notification event from NATS to the client until disconnect.
func (h *WS) Upgrade(w http.ResponseWriter, r *http.Request) {
	ticket := r.URL.Query().Get("ticket")
	if ticket == "" {
		http.Error(w, "missing ticket", http.StatusUnauthorized)
		return
	}
	claims, err := authmw.DecodeToken(h.JWT, ticket)
	if err != nil || claims.TokenUse == nil || *claims.TokenUse != "ws_ticket" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// In production restrict by config.cors_origins; defaulting
		// to InsecureSkipVerify is fine while behind the gateway.
		InsecureSkipVerify: true,
	})
	if err != nil {
		slog.Warn("ws accept failed", slog.String("error", err.Error()))
		return
	}
	defer conn.Close(websocket.StatusInternalError, "internal error")

	ctx := r.Context()

	// Snapshot.
	notifications, _ := h.Notifications.Latest(ctx, claims.Sub, 20)
	unread, _ := h.Notifications.UnreadCount(ctx, &claims.Sub)
	if err := wsjson.Write(ctx, conn, map[string]any{
		"kind":         "snapshot",
		"data":         notifications,
		"unread_count": unread,
	}); err != nil {
		return
	}

	if h.Bus == nil {
		// Without NATS we have nothing else to stream.
		conn.Close(websocket.StatusNormalClosure, "")
		return
	}

	consumerName := "notifications-ws-user-" + claims.Sub.String() + "-" + ids.New().String()
	cons, err := h.Bus.Stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:       consumerName,
		FilterSubject: h.Bus.Subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		slog.Warn("ws consumer create failed", slog.String("error", err.Error()))
		return
	}
	defer func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = h.Bus.Stream.DeleteConsumer(cleanCtx, consumerName)
	}()

	consume, err := cons.Consume(func(msg jetstream.Msg) {
		_ = msg.Ack()
		// Decoded payload is the controlbus.Event envelope; we forward
		// the inner notification event payload directly to the client.
		var envelope struct {
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(msg.Data(), &envelope); err != nil {
			slog.Warn("ws decode envelope failed", slog.String("error", err.Error()))
			return
		}
		// Filter by user_id when present.
		if !targetsUser(envelope.Payload, claims.Sub) {
			return
		}
		writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := conn.Write(writeCtx, websocket.MessageText, envelope.Payload); err != nil {
			conn.Close(websocket.StatusGoingAway, "client gone")
			return
		}
	})
	if err != nil {
		slog.Warn("ws consume start failed", slog.String("error", err.Error()))
		return
	}
	defer consume.Stop()

	// Block on ctx — when the client disconnects, ctx is cancelled by Accept's read loop.
	<-ctx.Done()
	conn.Close(websocket.StatusNormalClosure, "")
}

// targetsUser inspects the payload (a controlbus NotificationEvent JSON)
// and returns true when user_id matches `userID` OR when user_id is null
// (broadcast). Mirrors the Rust `targets_user` logic.
func targetsUser(payload json.RawMessage, userID uuid.UUID) bool {
	var event struct {
		UserID *uuid.UUID `json:"user_id"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return false
	}
	if event.UserID == nil {
		return true
	}
	return *event.UserID == userID
}
