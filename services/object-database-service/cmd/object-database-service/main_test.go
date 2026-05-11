package main

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/object-database-service/internal/config"
)

func TestBuildStoresDevModeAllowsInMemoryFallback(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{DevMode: true, Backend: config.BackendCassandra}
	objects, links, backend, session, cleanup, err := buildStores(context.Background(), cfg, slog.Default())
	require.NoError(t, err)
	assert.NotNil(t, objects)
	assert.NotNil(t, links)
	assert.Equal(t, config.BackendInMemory, backend)
	assert.Nil(t, session)
	assert.Nil(t, cleanup)
}

func TestBuildStoresRejectsImplicitProductionFallback(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{Backend: config.BackendCassandra}
	_, _, _, _, _, err := buildStores(context.Background(), cfg, slog.Default())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CASSANDRA_CONTACT_POINTS is required")
}

func TestBuildStoresRejectsInMemoryWithoutDevMode(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{Backend: config.BackendInMemory}
	_, _, _, _, _, err := buildStores(context.Background(), cfg, slog.Default())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OF_DEV_STUB_MODE=true")
}

func TestValidateKeyspace(t *testing.T) {
	t.Parallel()
	require.NoError(t, validateKeyspace("CASSANDRA_OBJECT_KEYSPACE", "ontology_objects"))
	err := validateKeyspace("CASSANDRA_OBJECT_KEYSPACE", "bad-name")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "not a valid CQL identifier"))
}
