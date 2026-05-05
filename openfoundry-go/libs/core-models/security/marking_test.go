package security_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/core-models/security"
)

func TestMarkingSourceJSON(t *testing.T) {
	t.Parallel()

	out, err := json.Marshal(security.Direct())
	require.NoError(t, err)
	assert.JSONEq(t, `{"kind":"direct"}`, string(out))

	out, err = json.Marshal(security.InheritedFrom("ri.foundry.main.dataset.abc"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"kind":"inherited_from_upstream","upstream_rid":"ri.foundry.main.dataset.abc"}`, string(out))
}

func TestMarkingIDRoundTrip(t *testing.T) {
	t.Parallel()
	id := security.NewMarkingID()
	out, err := json.Marshal(id)
	require.NoError(t, err)
	var back security.MarkingID
	require.NoError(t, json.Unmarshal(out, &back))
	assert.Equal(t, id, back)
}

func TestMarkingSourceHelpers(t *testing.T) {
	t.Parallel()
	d := security.Direct()
	assert.True(t, d.IsDirect())
	assert.Empty(t, d.UpstreamRID)

	i := security.InheritedFrom("ri.x")
	assert.False(t, i.IsDirect())
	assert.Equal(t, "ri.x", i.UpstreamRID)
}
