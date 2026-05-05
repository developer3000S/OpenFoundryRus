package idempotency_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/idempotency"
)

func TestMemStoreFirstSeenThenDuplicate(t *testing.T) {
	t.Parallel()
	store := idempotency.NewMemStore()
	id := uuid.New()

	first, err := store.CheckAndRecord(context.Background(), id)
	require.NoError(t, err)
	assert.True(t, first.IsFirstSeen())

	dup, err := store.CheckAndRecord(context.Background(), id)
	require.NoError(t, err)
	assert.True(t, dup.IsAlreadyProcessed())
}

func TestIdempotentRunsClosureOnce(t *testing.T) {
	t.Parallel()
	store := idempotency.NewMemStore()
	id := uuid.New()
	calls := 0

	v, ran, err := idempotency.Idempotent(context.Background(), store, id,
		func(_ context.Context) (string, error) {
			calls++
			return "ok", nil
		})
	require.NoError(t, err)
	assert.True(t, ran)
	assert.Equal(t, "ok", v)

	v2, ran2, err := idempotency.Idempotent(context.Background(), store, id,
		func(_ context.Context) (string, error) {
			calls++
			return "should-not-run", nil
		})
	require.NoError(t, err)
	assert.False(t, ran2)
	assert.Equal(t, "", v2)
	assert.Equal(t, 1, calls, "closure must run exactly once per eventID")
}

func TestIdempotentClosureFailureLeavesEventRecorded(t *testing.T) {
	t.Parallel()
	store := idempotency.NewMemStore()
	id := uuid.New()
	boom := errors.New("boom")

	_, ran, err := idempotency.Idempotent(context.Background(), store, id,
		func(_ context.Context) (string, error) { return "", boom })
	assert.True(t, ran)
	assert.ErrorIs(t, err, boom)

	// Redelivery does NOT re-run — the eventID was recorded before f.
	_, ran2, err := idempotency.Idempotent(context.Background(), store, id,
		func(_ context.Context) (string, error) {
			t.Fatal("must not re-run after first delivery already recorded the event")
			return "", nil
		})
	require.NoError(t, err)
	assert.False(t, ran2)
}
