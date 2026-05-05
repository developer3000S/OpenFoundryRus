package ratelimit_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/middleware/ratelimit"
)

func TestMemoryStoreFirstHitAllowedAndDecrements(t *testing.T) {
	t.Parallel()
	s := ratelimit.NewMemoryStore(0)
	out, err := s.Allow("k", 60, 5)
	require.NoError(t, err)
	assert.True(t, out.Allowed)
	assert.Equal(t, uint32(60), out.Limit)
	// Burst capacity is 5; we consumed 1 → remaining floor = 4.
	assert.Equal(t, uint32(4), out.Remaining)
}

func TestMemoryStoreExhaustsBurstThenDenies(t *testing.T) {
	t.Parallel()
	s := ratelimit.NewMemoryStore(0)
	for i := 0; i < 3; i++ {
		out, err := s.Allow("k", 60, 3)
		require.NoError(t, err)
		assert.True(t, out.Allowed, "burst hit %d should be allowed", i+1)
	}
	out, err := s.Allow("k", 60, 3)
	require.NoError(t, err)
	assert.False(t, out.Allowed, "fourth hit must be rate-limited")
	assert.Greater(t, out.ResetAfter, time.Duration(0))
}

func TestMemoryStoreSeparatesKeys(t *testing.T) {
	t.Parallel()
	s := ratelimit.NewMemoryStore(0)
	for i := 0; i < 2; i++ {
		out, _ := s.Allow("a", 60, 2)
		require.True(t, out.Allowed)
	}
	// Bucket A is now empty, but B is fresh.
	out, err := s.Allow("b", 60, 2)
	require.NoError(t, err)
	assert.True(t, out.Allowed)
}

func TestMemoryStoreLimitZeroDeniesAll(t *testing.T) {
	t.Parallel()
	s := ratelimit.NewMemoryStore(0)
	out, err := s.Allow("k", 0, 10)
	require.NoError(t, err)
	assert.False(t, out.Allowed)
}
