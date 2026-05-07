package config

import (
	"reflect"
	"testing"
	"time"
)

func TestFromEnvPythonSidecarCanonicalConfig(t *testing.T) {
	t.Setenv("PYTHON_SIDECAR_BINARY", "/opt/openfoundry-pyruntime")
	t.Setenv("PYTHON_SIDECAR_BIN", "/legacy/ignored")
	t.Setenv("PYTHON_SIDECAR_ARGS", "--log-level debug --feature-x")
	t.Setenv("PYTHON_SIDECAR_ENV", "PYRUNTIME_LOG=debug,PATH=/venv/bin\nCUSTOM=1")
	t.Setenv("PYTHON_SIDECAR_TIMEOUT", "23s")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.PythonSidecarBinary != "/opt/openfoundry-pyruntime" {
		t.Fatalf("binary = %q", cfg.PythonSidecarBinary)
	}
	if !reflect.DeepEqual(cfg.PythonSidecarArgs, []string{"--log-level", "debug", "--feature-x"}) {
		t.Fatalf("args = %#v", cfg.PythonSidecarArgs)
	}
	if !reflect.DeepEqual(cfg.PythonSidecarEnv, []string{"PYRUNTIME_LOG=debug", "PATH=/venv/bin", "CUSTOM=1"}) {
		t.Fatalf("env = %#v", cfg.PythonSidecarEnv)
	}
	if cfg.PythonSidecarTimeout != 23*time.Second {
		t.Fatalf("timeout = %s", cfg.PythonSidecarTimeout)
	}
}

func TestFromEnvPythonSidecarLegacyBinaryAliasAndSecondsTimeout(t *testing.T) {
	t.Setenv("PYTHON_SIDECAR_BIN", "/legacy/openfoundry-pyruntime")
	t.Setenv("PYTHON_SIDECAR_TIMEOUT", "7")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.PythonSidecarBinary != "/legacy/openfoundry-pyruntime" {
		t.Fatalf("binary alias = %q", cfg.PythonSidecarBinary)
	}
	if cfg.PythonSidecarTimeout != 7*time.Second {
		t.Fatalf("timeout = %s", cfg.PythonSidecarTimeout)
	}
}

func TestFromEnvDevModeDefaultsFalseAndAcceptsExplicitEnv(t *testing.T) {
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.DevMode {
		t.Fatal("DevMode must default false")
	}

	t.Setenv("OF_DEV_STUB_MODE", "true")
	cfg, err = FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if !cfg.DevMode {
		t.Fatal("OF_DEV_STUB_MODE=true should enable DevMode")
	}
}

func TestFromEnvCassandraAndSearchConfig(t *testing.T) {
	t.Setenv("CASSANDRA_CONTACT_POINTS", "cass1:9042,cass2:9042")
	t.Setenv("CASSANDRA_KEYSPACE", "ontology_runtime")
	t.Setenv("CASSANDRA_USERNAME", "svc")
	t.Setenv("CASSANDRA_PASSWORD", "secret")
	t.Setenv("CASSANDRA_LOCAL_DC", "dc9")
	t.Setenv("SEARCH_BACKEND", "opensearch")
	t.Setenv("SEARCH_ENDPOINT", "https://search.example")
	t.Setenv("SEARCH_API_KEY", "search-token")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if cfg.CassandraContactPoints != "cass1:9042,cass2:9042" || cfg.CassandraKeyspace != "ontology_runtime" {
		t.Fatalf("unexpected Cassandra config: %#v", cfg)
	}
	if cfg.CassandraUsername != "svc" || cfg.CassandraPassword != "secret" || cfg.CassandraLocalDC != "dc9" {
		t.Fatalf("unexpected Cassandra auth/DC config: %#v", cfg)
	}
	if cfg.SearchBackend != "opensearch" || cfg.SearchEndpoint != "https://search.example" {
		t.Fatalf("unexpected search config: %#v", cfg)
	}
	if cfg.SearchAuthHeader != "Bearer search-token" {
		t.Fatalf("search auth header = %q", cfg.SearchAuthHeader)
	}
}

func TestFromEnvPythonPackagesEnabled(t *testing.T) {
	t.Setenv("PYTHON_PACKAGES_ENABLED", "true")
	cfg, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if !cfg.PythonPackagesEnabled {
		t.Fatalf("PythonPackagesEnabled = false")
	}
}
