package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExploratoryViewJSONShape(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	v := ExploratoryView{
		ID:         id,
		Slug:       "high-risk-flights",
		Name:       "High-Risk Flights",
		ObjectType: "flight",
		FilterSpec: json.RawMessage(`{"risk":"high"}`),
		Layout:     json.RawMessage(`{"columns":["id","tail"]}`),
		CreatedAt:  time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC),
	}
	b, err := json.Marshal(v)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, id.String(), got["id"])
	assert.Equal(t, "high-risk-flights", got["slug"])
	assert.Equal(t, "flight", got["object_type"])
	assert.Contains(t, got, "filter_spec")
	assert.Contains(t, got, "layout")
}

func TestWritebackProposalKeepsRustShape(t *testing.T) {
	t.Parallel()
	note := "manual override"
	p := WritebackProposal{
		ID:         uuid.New(),
		ObjectType: "aircraft",
		ObjectID:   "obj-1",
		Patch:      json.RawMessage(`{"tail":"N777OF"}`),
		Note:       &note,
		Status:     "pending",
		CreatedAt:  time.Now().UTC(),
	}
	b, err := json.Marshal(p)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, "aircraft", got["object_type"])
	assert.Equal(t, "obj-1", got["object_id"])
	assert.Equal(t, "manual override", got["note"])
	assert.Equal(t, "pending", got["status"])
}

func TestExploratoryMapViewIDOptional(t *testing.T) {
	t.Parallel()
	m := ExploratoryMap{
		ID:        uuid.New(),
		Name:      "global heatmap",
		MapKind:   "heatmap",
		Config:    json.RawMessage(`{}`),
		CreatedAt: time.Now().UTC(),
	}
	b, err := json.Marshal(m)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, "heatmap", got["map_kind"])
	// view_id is omitempty + nil pointer → null in the wire (Rust uses
	// Option<Uuid> with no skip_serializing_if so it's also null when
	// None — both shapes encode `null`).
}
