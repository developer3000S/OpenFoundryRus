package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrimaryItemJSONShape(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	at := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	p := PrimaryItem{ID: id, Payload: json.RawMessage(`{"diagram":{}}`), CreatedAt: at}
	b, err := json.Marshal(p)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, id.String(), got["id"])
	assert.NotNil(t, got["payload"])
	assert.NotNil(t, got["created_at"])
}

func TestSecondaryItemJSONShape(t *testing.T) {
	t.Parallel()
	id, parent := uuid.New(), uuid.New()
	s := SecondaryItem{ID: id, ParentID: parent, Payload: json.RawMessage(`{"ref":"a"}`), CreatedAt: time.Now()}
	b, err := json.Marshal(s)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, id.String(), got["id"])
	assert.Equal(t, parent.String(), got["parent_id"])
}

func TestRequestEnvelopes(t *testing.T) {
	t.Parallel()
	body, err := json.Marshal(CreatePrimaryRequest{Payload: json.RawMessage(`{}`)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"payload":{}}`, string(body))

	body2, err := json.Marshal(CreateSecondaryRequest{Payload: json.RawMessage(`{"k":"v"}`)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"payload":{"k":"v"}}`, string(body2))
}
