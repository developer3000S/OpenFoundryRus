package outbox_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/openfoundry/openfoundry-go/libs/outbox"
)

func TestOutboxEventBuilderAttachesHeaders(t *testing.T) {
	t.Parallel()
	evt := outbox.New(
		uuid.Nil,
		"ontology_object",
		"obj-1",
		"ontology.object.changed.v1",
		json.RawMessage(`{"version":1}`),
	).
		WithHeader("ol-run-id", "run-abc").
		WithHeader("ol-namespace", "of")

	assert.Equal(t, "run-abc", evt.Headers["ol-run-id"])
	assert.Equal(t, "of", evt.Headers["ol-namespace"])
	assert.Equal(t, "ontology.object.changed.v1", evt.Topic)
}
