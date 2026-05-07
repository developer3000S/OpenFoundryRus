package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBackendKindFromEnv(t *testing.T) {
	t.Parallel()
	assert.Equal(t, BackendVespa, BackendKindFromEnv(""))
	assert.Equal(t, BackendVespa, BackendKindFromEnv("vespa"))
	assert.Equal(t, BackendVespa, BackendKindFromEnv("  VESPA  "))
	assert.Equal(t, BackendVespa, BackendKindFromEnv("unknown"))
	assert.Equal(t, BackendOpenSearch, BackendKindFromEnv("opensearch"))
	assert.Equal(t, BackendOpenSearch, BackendKindFromEnv("OpenSearch"))
}

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("HOST", "")
	t.Setenv("PORT", "")
	t.Setenv("SEARCH_BACKEND", "")
	t.Setenv("SEARCH_ENDPOINT", "")
	t.Setenv("SEARCH_USERNAME", "")
	t.Setenv("SEARCH_PASSWORD", "")
	t.Setenv("SEARCH_BEARER_TOKEN", "")
	t.Setenv("SEARCH_API_KEY", "")
	t.Setenv("KAFKA_BOOTSTRAP_SERVERS", "")
	t.Setenv("KAFKA_CONSUMER_GROUP", "")
	t.Setenv("METRICS_ADDR", "")

	cfg, err := FromEnv()
	assert.NoError(t, err)
	assert.Equal(t, "ontology-indexer", cfg.Service.Name)
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, uint16(50124), cfg.Server.Port)
	assert.Equal(t, BackendVespa, cfg.BackendKind)
	assert.Empty(t, cfg.SearchUsername)
	assert.Empty(t, cfg.SearchPassword)
	assert.Empty(t, cfg.SearchBearerToken)
	assert.Empty(t, cfg.SearchAPIKey)
	assert.Equal(t, "ontology-indexer", cfg.ConsumerGroup)
	assert.Equal(t, "0.0.0.0:9090", cfg.MetricsAddr)
}

func TestFromEnvSearchAuthConfig(t *testing.T) {
	t.Setenv("SEARCH_USERNAME", "user")
	t.Setenv("SEARCH_PASSWORD", "pass")
	t.Setenv("SEARCH_BEARER_TOKEN", "bearer")
	t.Setenv("SEARCH_API_KEY", "api")

	cfg, err := FromEnv()
	assert.NoError(t, err)
	assert.Equal(t, "user", cfg.SearchUsername)
	assert.Equal(t, "pass", cfg.SearchPassword)
	assert.Equal(t, "bearer", cfg.SearchBearerToken)
	assert.Equal(t, "api", cfg.SearchAPIKey)
}
