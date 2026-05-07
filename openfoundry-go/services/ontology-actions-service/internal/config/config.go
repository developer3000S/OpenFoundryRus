// Package config mirrors `services/ontology-actions-service/src/config.rs`.
//
// Field-for-field 1:1 with the Rust struct so the existing Helm
// envSecrets templates and runbook overrides resolve identically.
// Defaults match ports allocated in `services/edge-gateway-service`.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Service struct {
		Name    string
		Version string
	}

	Server struct {
		Host string
		Port uint16
	}

	DatabaseURL string
	JWTSecret   string

	// DevMode permits explicit local/test startup without DATABASE_URL by
	// wiring an in-memory AppState. It is disabled by default and is enabled
	// only by OF_DEV_STUB_MODE=true (or the legacy ALLOW_SUBSTRATE_STUBS=true).
	DevMode bool

	AuditServiceURL               string
	DatasetServiceURL             string
	OntologyServiceURL            string
	PipelineServiceURL            string
	AIServiceURL                  string
	NotificationServiceURL        string
	SearchEmbeddingProvider       string
	NodeRuntimeCommand            string
	ConnectorManagementServiceURL string

	CassandraContactPoints string
	CassandraKeyspace      string
	CassandraUsername      string
	CassandraPassword      string
	CassandraLocalDC       string

	SearchBackend    string
	SearchEndpoint   string
	SearchAuthHeader string

	// PythonSidecarBinary is the absolute path to the openfoundry-pyruntime
	// executable. When empty, inline Python functions explicitly return
	// ErrPythonRuntimeNotWired rather than taking the normal production path.
	//
	// PYTHON_SIDECAR_BINARY is canonical; PYTHON_SIDECAR_BIN is accepted as a
	// legacy alias for existing deployments.
	PythonSidecarBinary string

	// PythonSidecarArgs are extra args appended after the manager-owned
	// `--bind <socket>` flags. Configure with PYTHON_SIDECAR_ARGS.
	PythonSidecarArgs []string

	// PythonSidecarEnv entries (KEY=VALUE) are appended to the inherited
	// process environment. Configure with PYTHON_SIDECAR_ENV as comma or
	// newline separated entries.
	PythonSidecarEnv []string

	// PythonSidecarTimeout caps sidecar startup and manager hard-call timeout.
	// Configure with PYTHON_SIDECAR_TIMEOUT (Go duration like 15s or seconds).
	PythonSidecarTimeout time.Duration

	// PythonPackagesEnabled gates production startup for inline Python packages.
	// When true outside OF_DEV_STUB_MODE, PYTHON_SIDECAR_BINARY is required so
	// python_runtime_not_wired is caught as a deployment config error.
	PythonPackagesEnabled bool
}

func FromEnv() (*Config, error) {
	c := &Config{}
	c.Service.Name = "ontology-actions-service"
	c.Service.Version = defaultStr(os.Getenv("SERVICE_VERSION"), "dev")
	c.Server.Host = defaultStr(os.Getenv("HOST"), "0.0.0.0")
	c.Server.Port = parseUint16(os.Getenv("PORT"), 50106)
	c.DatabaseURL = os.Getenv("DATABASE_URL")
	c.JWTSecret = os.Getenv("JWT_SECRET")
	c.DevMode = parseBool(os.Getenv("OF_DEV_STUB_MODE")) || parseBool(os.Getenv("ALLOW_SUBSTRATE_STUBS"))
	c.AuditServiceURL = defaultStr(os.Getenv("AUDIT_SERVICE_URL"), "http://localhost:50115")
	c.DatasetServiceURL = defaultStr(os.Getenv("DATASET_SERVICE_URL"), "http://localhost:50079")
	c.OntologyServiceURL = defaultStr(os.Getenv("ONTOLOGY_SERVICE_URL"), "http://localhost:50103")
	c.PipelineServiceURL = defaultStr(os.Getenv("PIPELINE_SERVICE_URL"), "http://localhost:50081")
	c.AIServiceURL = defaultStr(os.Getenv("AI_SERVICE_URL"), "http://localhost:50127")
	c.NotificationServiceURL = defaultStr(os.Getenv("NOTIFICATION_SERVICE_URL"), "http://localhost:50114")
	c.SearchEmbeddingProvider = defaultStr(os.Getenv("SEARCH_EMBEDDING_PROVIDER"), "deterministic-hash")
	c.NodeRuntimeCommand = defaultStr(os.Getenv("NODE_RUNTIME_COMMAND"), "node")
	c.ConnectorManagementServiceURL = defaultStr(os.Getenv("CONNECTOR_MANAGEMENT_SERVICE_URL"), "http://localhost:50130")
	c.CassandraContactPoints = os.Getenv("CASSANDRA_CONTACT_POINTS")
	c.CassandraKeyspace = os.Getenv("CASSANDRA_KEYSPACE")
	c.CassandraUsername = os.Getenv("CASSANDRA_USERNAME")
	c.CassandraPassword = os.Getenv("CASSANDRA_PASSWORD")
	c.CassandraLocalDC = defaultStr(os.Getenv("CASSANDRA_LOCAL_DC"), "dc1")
	c.SearchBackend = os.Getenv("SEARCH_BACKEND")
	c.SearchEndpoint = os.Getenv("SEARCH_ENDPOINT")
	c.SearchAuthHeader = defaultStr(os.Getenv("SEARCH_AUTH_HEADER"), bearerAuthHeader(os.Getenv("SEARCH_API_KEY")))
	c.PythonSidecarBinary = os.Getenv("PYTHON_SIDECAR_BINARY")
	c.PythonSidecarBinary = defaultStr(os.Getenv("PYTHON_SIDECAR_BINARY"), os.Getenv("PYTHON_SIDECAR_BIN"))
	c.PythonSidecarArgs = splitFields(os.Getenv("PYTHON_SIDECAR_ARGS"))
	c.PythonSidecarEnv = splitEnvList(os.Getenv("PYTHON_SIDECAR_ENV"))
	c.PythonSidecarTimeout = parseDuration(os.Getenv("PYTHON_SIDECAR_TIMEOUT"), 15*time.Second)
	c.PythonPackagesEnabled = parseBool(defaultStr(os.Getenv("PYTHON_PACKAGES_ENABLED"), os.Getenv("ENABLE_PYTHON_PACKAGES")))
	return c, nil
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func parseUint16(v string, fallback uint16) uint16 {
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseUint(v, 10, 16)
	if err != nil {
		return fallback
	}
	return uint16(n)
}

func splitFields(v string) []string {
	return strings.Fields(v)
}

func splitEnvList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '\n' })
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseDuration(v string, fallback time.Duration) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	seconds, err := strconv.ParseUint(v, 10, 32)
	if err != nil || seconds == 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	default:
		return false
	}
}

func bearerAuthHeader(apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return ""
	}
	return "Bearer " + apiKey
}
