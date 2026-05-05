package handlers

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestTargetsUserBroadcastWhenUserIDNull(t *testing.T) {
	t.Parallel()
	body := json.RawMessage(`{"kind":"notification.created","unread_count":1}`)
	assert.True(t, targetsUser(body, uuid.New()), "null user_id => broadcast to everyone")
}

func TestTargetsUserMatchesByUserID(t *testing.T) {
	t.Parallel()
	uid := uuid.New()
	other := uuid.New()
	body, err := json.Marshal(map[string]any{
		"kind":         "notification.created",
		"user_id":      uid,
		"unread_count": 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.True(t, targetsUser(body, uid))
	assert.False(t, targetsUser(body, other))
}
