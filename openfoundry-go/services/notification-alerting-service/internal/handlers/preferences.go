package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/repo"
)

// Preferences wires GET / PUT /api/v1/notifications/preferences.
type Preferences struct {
	Repo *repo.PreferencesRepo
}

// Get handles GET /notifications/preferences.
func (p *Preferences) Get(w http.ResponseWriter, r *http.Request) {
	c := authmw.MustFromContext(r.Context())
	pref, err := p.loadOrDefault(r, c.Sub)
	if err != nil {
		slog.Error("get preferences failed", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, pref)
}

// Update handles PUT /notifications/preferences.
//
// Missing fields preserve current values, matching the Rust impl.
func (p *Preferences) Update(w http.ResponseWriter, r *http.Request) {
	c := authmw.MustFromContext(r.Context())

	var body models.UpdatePreferenceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	current, err := p.loadOrDefault(r, c.Sub)
	if err != nil {
		slog.Error("load current preferences failed", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	merged := mergePreferences(current, &body)
	updated, err := p.Repo.Upsert(r.Context(), merged)
	if err != nil {
		slog.Error("upsert preferences failed", slog.String("error", err.Error()))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (p *Preferences) loadOrDefault(r *http.Request, userID uuid.UUID) (*models.NotificationPreference, error) {
	existing, err := p.Repo.FindByUser(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	return repo.Default(userID), nil
}

// mergePreferences applies non-nil fields of `body` over `current`.
//
// Mirrors Rust `body.field.unwrap_or(current.field)` pattern field-by-field.
func mergePreferences(current *models.NotificationPreference, body *models.UpdatePreferenceRequest) *models.NotificationPreference {
	out := *current // copy
	if body.InAppEnabled != nil {
		out.InAppEnabled = *body.InAppEnabled
	}
	if body.EmailEnabled != nil {
		out.EmailEnabled = *body.EmailEnabled
	}
	if body.EmailAddress != nil {
		out.EmailAddress = body.EmailAddress
	}
	if body.SlackWebhookURL != nil {
		out.SlackWebhookURL = body.SlackWebhookURL
	}
	if body.TeamsWebhookURL != nil {
		out.TeamsWebhookURL = body.TeamsWebhookURL
	}
	if body.DigestFrequency != nil {
		out.DigestFrequency = *body.DigestFrequency
	}
	if len(body.QuietHours) > 0 {
		out.QuietHours = body.QuietHours
	}
	return &out
}
