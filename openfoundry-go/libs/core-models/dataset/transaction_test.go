package dataset_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/core-models/dataset"
)

func TestDatasetRIDRoundTrip(t *testing.T) {
	t.Parallel()
	rid := dataset.NewDatasetRID()
	parsed, err := dataset.ParseDatasetRID(rid.String())
	require.NoError(t, err)
	assert.Equal(t, rid, parsed)
}

func TestDatasetRIDRejectsBadInput(t *testing.T) {
	t.Parallel()
	_, err := dataset.ParseDatasetRID("ri.foundry.main.folder.123")
	assert.Error(t, err)
	_, err = dataset.ParseDatasetRID("ri.foundry.main.dataset.not-a-uuid")
	assert.Error(t, err)
}

func TestBranchNameValidation(t *testing.T) {
	t.Parallel()
	_, err := dataset.ParseBranchName("master")
	assert.NoError(t, err)
	_, err = dataset.ParseBranchName("feature/streaming-v2")
	assert.NoError(t, err)
	_, err = dataset.ParseBranchName("")
	assert.Error(t, err)
	_, err = dataset.ParseBranchName("bad name")
	assert.Error(t, err)
}

func TestTransactionTypeJSON(t *testing.T) {
	t.Parallel()
	out, err := json.Marshal(dataset.TxSnapshot)
	require.NoError(t, err)
	assert.JSONEq(t, `"snapshot"`, string(out))

	parsed, err := dataset.ParseTransactionType("APPEND")
	require.NoError(t, err)
	assert.Equal(t, dataset.TxAppend, parsed)

	_, err = dataset.ParseTransactionType("nope")
	assert.ErrorIs(t, err, dataset.ErrUnknownTransactionType)
}

func TestTransactionStateTerminal(t *testing.T) {
	t.Parallel()
	assert.False(t, dataset.TxOpen.IsTerminal())
	assert.True(t, dataset.TxCommitted.IsTerminal())
	assert.True(t, dataset.TxAborted.IsTerminal())
}

func TestTransactionStateParseError(t *testing.T) {
	t.Parallel()
	_, err := dataset.ParseTransactionState("nope")
	require.Error(t, err)
	assert.True(t, errors.Is(err, dataset.ErrUnknownTransactionState))
}
