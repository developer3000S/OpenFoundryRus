package handlers_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/sdk-generation-service/internal/models"
)

// Wire-format invariants: Job + Publication JSON shape preserved 1:1
// with the Rust crate's PrimaryItem/SecondaryItem.
func TestJobJSONShape(t *testing.T) {
	t.Parallel()
	j := models.Job{
		ID:        uuid.New(),
		Payload:   json.RawMessage(`{"k":"v"}`),
		CreatedAt: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
	}
	out, err := json.Marshal(j)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{"id", "payload", "created_at"} {
		assert.Contains(t, view, k)
	}
	assert.NotContains(t, view, "parent_id", "Job must not carry parent_id")
}

func TestPublicationJSONShape(t *testing.T) {
	t.Parallel()
	p := models.Publication{
		ID:        uuid.New(),
		ParentID:  uuid.New(),
		Payload:   json.RawMessage(`{"sdk":"typescript"}`),
		CreatedAt: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
	}
	out, err := json.Marshal(p)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{"id", "parent_id", "payload", "created_at"} {
		assert.Contains(t, view, k)
	}
}
