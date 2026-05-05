package controlbus_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	controlbus "github.com/openfoundry/openfoundry-go/libs/event-bus-control"
)

func TestEventEnvelopeJSONShape(t *testing.T) {
	t.Parallel()
	evt, err := controlbus.NewEvent("dataset.quality.refresh.requested", "dataset-versioning-service",
		map[string]any{"dataset_id": "abc"})
	require.NoError(t, err)

	out, err := json.Marshal(evt)
	require.NoError(t, err)

	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))

	for _, k := range []string{"id", "timestamp", "event_type", "source", "payload"} {
		assert.Contains(t, view, k, "Event JSON must carry %q", k)
	}
	assert.Equal(t, "dataset-versioning-service", view["source"])
}

func TestDatasetQualityRefreshDefaults(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	req := controlbus.DatasetQualityRefreshForUpload(id)
	assert.Equal(t, "dataset_upload", req.Reason)
	assert.Equal(t, id, req.DatasetID)

	out, err := json.Marshal(req)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	trig := view["context"].(map[string]any)["trigger"].(map[string]any)
	assert.Equal(t, "dataset_upload", trig["type"])
}
