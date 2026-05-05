// Package handlers hosts the HTTP endpoints for notification-alerting-service.
package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/service"
)

// History wires the list / mark_read / mark_all_read endpoints.
type History struct {
	Notifications *repo.NotificationsRepo
	Notifier      *service.Notifier
}

// List handles GET /api/v1/notifications.
func (h *History) List(w http.ResponseWriter, r *http.Request) {
	c := authmw.MustFromContext(r.Context())

	limit := int64(20)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			limit = clamp(v, 1, 100)
		}
	}
	var status *string
	if v := r.URL.Query().Get("status"); v != "" {
		status = &v
	}

	notifications, err := h.Notifications.List(r.Context(), c.Sub, status, limit)
	if err != nil {
		slog.Error("list notifications failed", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	unread, _ := h.Notifications.UnreadCount(r.Context(), &c.Sub)

	writeJSON(w, http.StatusOK, map[string]any{
		"data":         notifications,
		"unread_count": unread,
	})
}

// MarkRead handles PATCH /api/v1/notifications/{id}/read.
func (h *History) MarkRead(w http.ResponseWriter, r *http.Request) {
	c := authmw.MustFromContext(r.Context())

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid notification id", http.StatusBadRequest)
		return
	}

	notification, err := h.Notifications.MarkRead(r.Context(), id, c.Sub)
	if err != nil {
		slog.Error("mark read failed", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if notification == nil {
		http.NotFound(w, r)
		return
	}

	unread, _ := h.Notifications.UnreadCount(r.Context(), &c.Sub)
	h.Notifier.PublishRead(r.Context(), "notification.read", &c.Sub, notification, unread)

	writeJSON(w, http.StatusOK, map[string]any{
		"notification": notification,
		"unread_count": unread,
	})
}

// MarkAllRead handles POST /api/v1/notifications/read-all.
func (h *History) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	c := authmw.MustFromContext(r.Context())

	if err := h.Notifications.MarkAllRead(r.Context(), c.Sub); err != nil {
		slog.Error("mark all read failed", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.Notifier.PublishRead(r.Context(), "notification.read_all", &c.Sub, nil, 0)
	writeJSON(w, http.StatusOK, map[string]any{"unread_count": 0})
}

func clamp(v, lo, hi int64) int64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
