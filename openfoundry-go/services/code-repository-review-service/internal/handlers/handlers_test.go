package handlers

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromoteEventTypeAndTopicConstants(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "foundry.global.branch.promote.requested.v1", PromoteTopic)
	assert.Equal(t, "global.branch.promote.requested.v1", PromoteEventType)
}

func TestBuildPromotePayloadShape(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	raw, err := buildPromotePayload(id, "release-2026Q2", "alice")
	require.NoError(t, err)

	var body map[string]any
	require.NoError(t, json.Unmarshal(raw, &body))
	assert.Equal(t, PromoteEventType, body["event_type"])
	assert.Equal(t, id.String(), body["global_branch_id"])
	assert.Equal(t, "release-2026Q2", body["global_branch_name"])
	assert.Equal(t, "alice", body["actor"])
	assert.Contains(t, body, "occurred_at")
}
