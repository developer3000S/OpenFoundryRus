package handlers

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/models"
)

func TestMergePreferencesRetainsCurrentWhenBodyOmitted(t *testing.T) {
	t.Parallel()
	uid := uuid.New()
	curEmail := "current@example.com"
	current := &models.NotificationPreference{
		UserID:          uid,
		InAppEnabled:    true,
		EmailEnabled:    true,
		EmailAddress:    &curEmail,
		DigestFrequency: "daily",
		QuietHours:      json.RawMessage(`{"start":"22:00"}`),
	}

	merged := mergePreferences(current, &models.UpdatePreferenceRequest{})

	assert.Equal(t, true, merged.InAppEnabled)
	assert.Equal(t, true, merged.EmailEnabled)
	assert.Equal(t, &curEmail, merged.EmailAddress)
	assert.Equal(t, "daily", merged.DigestFrequency)
	assert.JSONEq(t, `{"start":"22:00"}`, string(merged.QuietHours))
}

func TestMergePreferencesAppliesNonNilFields(t *testing.T) {
	t.Parallel()
	uid := uuid.New()
	current := &models.NotificationPreference{
		UserID:          uid,
		InAppEnabled:    true,
		EmailEnabled:    false,
		DigestFrequency: "instant",
		QuietHours:      json.RawMessage(`{}`),
	}

	enable := true
	freq := "weekly"
	body := &models.UpdatePreferenceRequest{
		EmailEnabled:    &enable,
		DigestFrequency: &freq,
		QuietHours:      json.RawMessage(`{"start":"00:00"}`),
	}

	merged := mergePreferences(current, body)
	assert.True(t, merged.EmailEnabled)
	assert.Equal(t, "weekly", merged.DigestFrequency)
	assert.JSONEq(t, `{"start":"00:00"}`, string(merged.QuietHours))
	assert.True(t, merged.InAppEnabled, "untouched field stays")
}
