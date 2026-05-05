package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/service"
)

// Send wires POST /api/v1/notifications/send and POST /internal/notifications.
type Send struct {
	Notifier *service.Notifier
}

// Authenticated handles POST /api/v1/notifications/send.
//
// If the body omits user_id, defaults to the caller's subject — same
// affordance as the Rust handler.
func (s *Send) Authenticated(w http.ResponseWriter, r *http.Request) {
	c := authmw.MustFromContext(r.Context())

	var body models.SendNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.UserID == nil {
		sub := c.Sub
		body.UserID = &sub
	}
	s.create(w, r, body)
}

// Internal handles POST /internal/notifications. No auth — restrict
// access at the network layer (gateway / NetworkPolicy).
func (s *Send) Internal(w http.ResponseWriter, r *http.Request) {
	var body models.SendNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	s.create(w, r, body)
}

func (s *Send) create(w http.ResponseWriter, r *http.Request, body models.SendNotificationRequest) {
	notification, err := s.Notifier.Create(r.Context(), body)
	if err != nil {
		slog.Error("create notification failed", slog.String("error", err.Error()))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, notification)
}
