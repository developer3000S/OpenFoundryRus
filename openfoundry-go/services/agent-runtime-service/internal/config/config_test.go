package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromEnvAllowFakeLLMProviderDefaultsFalse(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("JWT_SECRET", "secret")

	cfg, err := FromEnv()

	require.NoError(t, err)
	assert.False(t, cfg.AllowFakeLLMProvider)
}

func TestFromEnvAllowFakeLLMProviderCanBeEnabled(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("JWT_SECRET", "secret")
	t.Setenv("ALLOW_FAKE_LLM_PROVIDER", "true")

	cfg, err := FromEnv()

	require.NoError(t, err)
	assert.True(t, cfg.AllowFakeLLMProvider)
}
