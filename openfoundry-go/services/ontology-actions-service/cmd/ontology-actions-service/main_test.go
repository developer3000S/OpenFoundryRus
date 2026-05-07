package main

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openfoundry/openfoundry-go/services/ontology-actions-service/internal/config"
)

func TestPythonSidecarConfigMapsServiceConfigToManager(t *testing.T) {
	cfg := &config.Config{
		PythonSidecarBinary:  "/opt/openfoundry-pyruntime",
		PythonSidecarArgs:    []string{"--debug"},
		PythonSidecarEnv:     []string{"PYRUNTIME_LOG=debug"},
		PythonSidecarTimeout: 11 * time.Second,
	}
	got := pythonSidecarConfig(cfg)
	if got.BinaryPath != cfg.PythonSidecarBinary {
		t.Fatalf("BinaryPath = %q", got.BinaryPath)
	}
	if !reflect.DeepEqual(got.Args, cfg.PythonSidecarArgs) {
		t.Fatalf("Args = %#v", got.Args)
	}
	if !reflect.DeepEqual(got.Env, cfg.PythonSidecarEnv) {
		t.Fatalf("Env = %#v", got.Env)
	}
	if got.StartupTimeout != cfg.PythonSidecarTimeout || got.HardCallTimeout != cfg.PythonSidecarTimeout {
		t.Fatalf("timeouts = startup %s hard %s", got.StartupTimeout, got.HardCallTimeout)
	}

	got.Args[0] = "mutated"
	got.Env[0] = "mutated=1"
	if cfg.PythonSidecarArgs[0] == "mutated" || cfg.PythonSidecarEnv[0] == "mutated=1" {
		t.Fatalf("pythonSidecarConfig must defensively copy slices")
	}
}

func TestBuildStateRequiresDatabaseURLUnlessDevMode(t *testing.T) {
	cfg := &config.Config{}
	cfg.JWTSecret = "test-secret"
	_, _, err := buildState(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("expected clear DATABASE_URL error, got %v", err)
	}
}

func TestBuildStateDevModeUsesExplicitInMemoryState(t *testing.T) {
	cfg := &config.Config{DevMode: true}
	cfg.JWTSecret = "test-secret"
	state, cleanup, err := buildState(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("buildState: %v", err)
	}
	if cleanup != nil {
		t.Fatal("dev in-memory state should not return production cleanup")
	}
	if state == nil || state.Stores.Objects == nil || state.DB != nil {
		t.Fatalf("unexpected dev state: %#v", state)
	}
}

func TestBuildStateWithDatabaseURLRequiresCassandraStores(t *testing.T) {
	cfg := &config.Config{DatabaseURL: "postgres://user:pass@localhost:5432/openfoundry"}
	cfg.JWTSecret = "test-secret"
	_, _, err := buildState(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || !strings.Contains(err.Error(), "CASSANDRA_CONTACT_POINTS is required") {
		t.Fatalf("expected clear Cassandra stores error, got %v", err)
	}
}

func TestBuildSearchBackendRequiresEndpointWhenConfigured(t *testing.T) {
	_, err := buildSearchBackend(&config.Config{SearchBackend: "vespa"})
	if err == nil || !strings.Contains(err.Error(), "SEARCH_ENDPOINT") {
		t.Fatalf("expected SEARCH_ENDPOINT error, got %v", err)
	}
}

func TestBuildSearchBackendReturnsNilWhenUnconfigured(t *testing.T) {
	backend, err := buildSearchBackend(&config.Config{})
	if err != nil {
		t.Fatalf("buildSearchBackend: %v", err)
	}
	if backend != nil {
		t.Fatalf("expected nil backend when search is unconfigured, got %#v", backend)
	}
}

func TestBuildStoresValidatesKeyspaceBeforeDial(t *testing.T) {
	cfg := &config.Config{
		CassandraContactPoints: "127.0.0.1:9042",
		CassandraKeyspace:      "bad-keyspace",
	}
	_, _, err := buildStores(context.Background(), cfg, nil)
	if err == nil || !strings.Contains(err.Error(), "not a valid CQL identifier") {
		t.Fatalf("expected keyspace validation error, got %v", err)
	}
}

func TestValidatePythonRuntimeConfigRequiresSidecarWhenProductionPythonEnabled(t *testing.T) {
	cfg := &config.Config{DatabaseURL: "postgres://user:pass@localhost:5432/openfoundry", PythonPackagesEnabled: true}
	err := validatePythonRuntimeConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "PYTHON_SIDECAR_BINARY is required") {
		t.Fatalf("expected production sidecar requirement, got %v", err)
	}

	cfg.PythonSidecarBinary = "/opt/openfoundry-pyruntime"
	if err := validatePythonRuntimeConfig(cfg); err != nil {
		t.Fatalf("sidecar configured should pass: %v", err)
	}
}

func TestValidatePythonRuntimeConfigAllowsDevModeMissingSidecar(t *testing.T) {
	cfg := &config.Config{DevMode: true, PythonPackagesEnabled: true}
	if err := validatePythonRuntimeConfig(cfg); err != nil {
		t.Fatalf("dev mode should preserve explicit python_runtime_not_wired behavior: %v", err)
	}
}
