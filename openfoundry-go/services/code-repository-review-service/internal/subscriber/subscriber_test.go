package subscriber

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGlobalRIDAcceptsRIDOrBareUUID(t *testing.T) {
	t.Parallel()
	id := uuid.New()

	got, ok := ParseGlobalRID(GlobalRIDPrefix + id.String())
	require.True(t, ok)
	assert.Equal(t, id, got)

	got2, ok := ParseGlobalRID(id.String())
	require.True(t, ok)
	assert.Equal(t, id, got2)

	_, ok = ParseGlobalRID("ri.foundry.main.globalbranch.not-a-uuid")
	assert.False(t, ok)

	_, ok = ParseGlobalRID("garbage")
	assert.False(t, ok)
}

func TestReadLabelExtracts(t *testing.T) {
	t.Parallel()
	v, ok := readLabel([]byte(`{"labels":{"global_branch":"GB-123"}}`), "global_branch")
	require.True(t, ok)
	assert.Equal(t, "GB-123", v)

	_, ok = readLabel([]byte(`{}`), "global_branch")
	assert.False(t, ok)

	_, ok = readLabel([]byte(`{"labels":{}}`), "global_branch")
	assert.False(t, ok)
}

func TestReadRequiredFieldMissing(t *testing.T) {
	t.Parallel()
	_, _, err := readRequiredString([]byte(`{}`), "event_type", "branch_rid")
	require.Error(t, err)
	var mfe *MissingFieldError
	require.True(t, errors.As(err, &mfe))
	assert.Equal(t, "event_type", mfe.Field)
}

func TestErrorsIsViaUnwrap(t *testing.T) {
	t.Parallel()
	mfe := &MissingFieldError{Field: "branch_rid"}
	assert.True(t, errors.Is(mfe, ErrMissingField))

	maf := &MalformedFieldError{Field: "labels.global_branch"}
	assert.True(t, errors.Is(maf, ErrMalformed))
}

func TestTopicConstants(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "foundry.branch.events.v1", Topic)
	assert.Equal(t, "ri.foundry.main.globalbranch.", GlobalRIDPrefix)
}
