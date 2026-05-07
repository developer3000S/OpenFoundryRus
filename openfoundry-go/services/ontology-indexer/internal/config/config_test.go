package config

import (
	"testing"
	"time"

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
	t.Setenv("INDEXER_RETRY_MAX_ATTEMPTS", "")
	t.Setenv("INDEXER_RETRY_INITIAL_BACKOFF", "")
	t.Setenv("INDEXER_RETRY_MAX_BACKOFF", "")
	t.Setenv("INDEXER_DLQ_TOPIC", "")

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
	assert.Equal(t, 3, cfg.RetryMaxAttempts)
	assert.Equal(t, 100*time.Millisecond, cfg.RetryInitialBackoff)
	assert.Equal(t, 2*time.Second, cfg.RetryMaxBackoff)
	assert.Equal(t, "ontology-indexer.dlq.v1", cfg.DLQTopic)
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

func TestFromEnvRetryConfig(t *testing.T) {
	t.Setenv("INDEXER_RETRY_MAX_ATTEMPTS", "5")
	t.Setenv("INDEXER_RETRY_INITIAL_BACKOFF", "25ms")
	t.Setenv("INDEXER_RETRY_MAX_BACKOFF", "1s")
	t.Setenv("INDEXER_DLQ_TOPIC", "custom.dlq")

	cfg, err := FromEnv()
	assert.NoError(t, err)
	assert.Equal(t, 5, cfg.RetryMaxAttempts)
	assert.Equal(t, 25*time.Millisecond, cfg.RetryInitialBackoff)
	assert.Equal(t, time.Second, cfg.RetryMaxBackoff)
	assert.Equal(t, "custom.dlq", cfg.DLQTopic)
}

func TestFromEnvDisablesDLQ(t *testing.T) {
	t.Setenv("INDEXER_DLQ_TOPIC", "off")

	cfg, err := FromEnv()
	assert.NoError(t, err)
	assert.Empty(t, cfg.DLQTopic)
}
