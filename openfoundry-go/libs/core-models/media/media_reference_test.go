package media_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/core-models/media"
)

func TestSchemaSerialisesScreamingSnake(t *testing.T) {
	t.Parallel()
	out, err := json.Marshal(media.SchemaImage)
	require.NoError(t, err)
	assert.JSONEq(t, `"IMAGE"`, string(out))

	parsed, err := media.ParseSetSchema("spreadsheet")
	require.NoError(t, err)
	assert.Equal(t, media.SchemaSpreadsheet, parsed)
}

func TestSchemaParseRejectsUnknown(t *testing.T) {
	t.Parallel()
	_, err := media.ParseSetSchema("nope")
	assert.ErrorIs(t, err, media.ErrUnknownSetSchema)
}

func TestReferenceCamelCaseRoundTrip(t *testing.T) {
	t.Parallel()
	original := media.NewReference(
		"ri.foundry.main.media_set.018f2f1c-aaaa-bbbb-cccc-000000000001",
		"ri.foundry.main.media_item.018f2f1c-aaaa-bbbb-cccc-000000000002",
		"master",
		media.SchemaImage,
	)
	encoded, err := original.ToFoundryJSON()
	require.NoError(t, err)
	parsed, err := media.FromFoundryJSON(encoded)
	require.NoError(t, err)
	assert.Equal(t, original, parsed)

	// Confirm camelCase keys.
	var view map[string]any
	require.NoError(t, json.Unmarshal([]byte(encoded), &view))
	assert.Contains(t, view, "mediaSetRid")
	assert.Contains(t, view, "mediaItemRid")
}
