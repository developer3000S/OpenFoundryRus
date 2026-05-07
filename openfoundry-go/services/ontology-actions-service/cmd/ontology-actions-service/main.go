// Command ontology-actions-service runs the consolidated ontology
// action / funnel / function / rule HTTP surface (per ADR-0030).
//
// Wiring status: the binary requires DATABASE_URL and Cassandra/Scylla
// runtime stores for normal startup. Local/test runs may opt into
// OF_DEV_STUB_MODE=true to create an explicit in-memory AppState without
// serving substrate handlers.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/gocql/gocql"
	"github.com/jackc/pgx/v5/pgxpool"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	cassandrakernel "github.com/openfoundry/openfoundry-go/libs/cassandra-kernel"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/stores"
	pythonsidecar "github.com/openfoundry/openfoundry-go/libs/python-sidecar"
	searchabstraction "github.com/openfoundry/openfoundry-go/libs/search-abstraction"
	"github.com/openfoundry/openfoundry-go/libs/search-abstraction/opensearch"
	"github.com/openfoundry/openfoundry-go/libs/search-abstraction/vespa"
	storageabstraction "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
	"github.com/openfoundry/openfoundry-go/services/ontology-actions-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/ontology-actions-service/internal/server"
)

// pythonRuntimeAdapter bridges *pythonsidecar.InlineFunctionResult to the
// ontologykernel.PythonInlineRuntime contract (different result type).
type pythonRuntimeAdapter struct{ mgr *pythonsidecar.Manager }

func (a pythonRuntimeAdapter) ExecuteInline(ctx context.Context, source string, inputJSON []byte, timeoutSeconds uint32) (*ontologykernel.InlineRuntimeResult, error) {
	out, err := a.mgr.ExecuteInline(ctx, source, inputJSON, timeoutSeconds)
	if err != nil {
		return nil, err
	}
	return &ontologykernel.InlineRuntimeResult{
		ResultJSON: out.ResultJSON,
		Stdout:     out.Stdout,
		Stderr:     out.Stderr,
	}, nil
}

var version = "dev"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.FromEnv()
	if err != nil {
		slog.Error("config load failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if cfg.Service.Version == "dev" {
		cfg.Service.Version = version
	}

	log := observability.InitLogging(cfg.Service.Name, cfg.Service.Version)
	shutdownTracing, err := observability.InitTracing(ctx, cfg.Service.Name, cfg.Service.Version)
	if err != nil {
		log.Error("tracing init failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() { _ = shutdownTracing(context.Background()) }()

	state, cleanupStores, err := buildState(ctx, cfg, log)
	if err != nil {
		log.Error("AppState build failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if cleanupStores != nil {
		defer cleanupStores()
	}
	if state != nil && state.DB != nil {
		defer state.DB.Close()
	}

	if err := validatePythonRuntimeConfig(cfg); err != nil {
		log.Error("python runtime config invalid", slog.String("error", err.Error()))
		os.Exit(1)
	}

	if state != nil && cfg.PythonSidecarBinary != "" {
		mgr, err := pythonsidecar.New(pythonSidecarConfig(cfg), log)
		if err != nil {
			log.Error("python-sidecar config invalid", slog.String("error", err.Error()))
			os.Exit(1)
		}
		startCtx, cancelStart := context.WithTimeout(ctx, cfg.PythonSidecarTimeout)
		if err := mgr.Start(startCtx); err != nil {
			cancelStart()
			log.Error("python-sidecar start failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		cancelStart()
		defer func() { _ = mgr.Stop(context.Background()) }()
		state.PythonRuntime = pythonRuntimeAdapter{mgr: mgr}
		log.Info("python sidecar wired", slog.String("binary", cfg.PythonSidecarBinary))
	} else if state != nil {
		log.Warn("PYTHON_SIDECAR_BINARY unset — inline Python functions will explicitly return ErrPythonRuntimeNotWired")
	}

	metrics := observability.NewMetrics()
	srv := server.New(cfg, state, metrics)
	if err := run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func validatePythonRuntimeConfig(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	if cfg.DevMode || cfg.DatabaseURL == "" || !cfg.PythonPackagesEnabled {
		return nil
	}
	if strings.TrimSpace(cfg.PythonSidecarBinary) == "" {
		return errors.New("PYTHON_SIDECAR_BINARY is required in production when PYTHON_PACKAGES_ENABLED=true; otherwise Python function packages would fail with python_runtime_not_wired")
	}
	return nil
}

func pythonSidecarConfig(cfg *config.Config) pythonsidecar.Config {
	return pythonsidecar.Config{
		BinaryPath:      cfg.PythonSidecarBinary,
		Args:            append([]string(nil), cfg.PythonSidecarArgs...),
		Env:             append([]string(nil), cfg.PythonSidecarEnv...),
		StartupTimeout:  cfg.PythonSidecarTimeout,
		HardCallTimeout: cfg.PythonSidecarTimeout,
	}
}

func buildState(ctx context.Context, cfg *config.Config, log *slog.Logger) (*ontologykernel.AppState, func(), error) {
	if cfg.DatabaseURL == "" {
		if !cfg.DevMode {
			return nil, nil, errors.New("DATABASE_URL is required for ontology-actions-service; set OF_DEV_STUB_MODE=true only for explicit local/test in-memory state")
		}
		log.Warn("OF_DEV_STUB_MODE enabled with DATABASE_URL unset — using explicit in-memory AppState for local/test execution")
		return newAppState(cfg, nil, stores.NewInMemory()), nil, nil
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, err
	}
	storeBag, cleanup, err := buildStores(ctx, cfg, pool)
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	return newAppState(cfg, pool, storeBag), cleanup, nil
}

func newAppState(cfg *config.Config, pool *pgxpool.Pool, storeBag stores.Stores) *ontologykernel.AppState {
	state := &ontologykernel.AppState{
		DB:                            pool,
		Stores:                        storeBag,
		HTTPClient:                    &http.Client{Timeout: 30 * time.Second},
		JWTConfig:                     authmw.NewJWTConfig(cfg.JWTSecret),
		AuditServiceURL:               cfg.AuditServiceURL,
		DatasetServiceURL:             cfg.DatasetServiceURL,
		OntologyServiceURL:            cfg.OntologyServiceURL,
		PipelineServiceURL:            cfg.PipelineServiceURL,
		AIServiceURL:                  cfg.AIServiceURL,
		NotificationServiceURL:        cfg.NotificationServiceURL,
		SearchEmbeddingProvider:       cfg.SearchEmbeddingProvider,
		NodeRuntimeCommand:            cfg.NodeRuntimeCommand,
		ConnectorManagementServiceURL: cfg.ConnectorManagementServiceURL,
	}
	return state
}

var keyspaceNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,47}$`)

func buildStores(ctx context.Context, cfg *config.Config, pool *pgxpool.Pool) (stores.Stores, func(), error) {
	if strings.TrimSpace(cfg.CassandraContactPoints) == "" {
		return stores.Stores{}, nil, errors.New("CASSANDRA_CONTACT_POINTS is required for ontology-actions-service production stores")
	}
	keyspace := strings.TrimSpace(cfg.CassandraKeyspace)
	if keyspace == "" {
		return stores.Stores{}, nil, errors.New("CASSANDRA_KEYSPACE is required for ontology-actions-service production stores")
	}
	if !keyspaceNameRe.MatchString(keyspace) {
		return stores.Stores{}, nil, fmt.Errorf("CASSANDRA_KEYSPACE %q is not a valid CQL identifier", keyspace)
	}
	cluster := &cassandrakernel.Cluster{
		Hosts:       splitCSV(cfg.CassandraContactPoints),
		Keyspace:    keyspace,
		Username:    cfg.CassandraUsername,
		Password:    cfg.CassandraPassword,
		Datacenter:  cfg.CassandraLocalDC,
		DialTimeout: 5 * time.Second,
		NumConns:    2,
		Consistency: gocql.LocalQuorum,
	}
	if len(cluster.Hosts) == 0 {
		return stores.Stores{}, nil, fmt.Errorf("CASSANDRA_CONTACT_POINTS resolved to no hosts: %q", cfg.CassandraContactPoints)
	}
	session, err := cluster.Connect()
	if err != nil {
		return stores.Stores{}, nil, fmt.Errorf("connect Cassandra/Scylla: %w", err)
	}
	cleanup := func() { session.Close() }
	if err := cassandrakernel.Apply(session, keyspace, cassandrakernel.OntologyRuntimeMigrations(keyspace)); err != nil {
		cleanup()
		return stores.Stores{}, nil, err
	}
	searchBackend, err := buildSearchBackend(cfg)
	if err != nil {
		cleanup()
		return stores.Stores{}, nil, err
	}
	select {
	case <-ctx.Done():
		cleanup()
		return stores.Stores{}, nil, ctx.Err()
	default:
	}
	return stores.Stores{
		Objects:     cassandrakernel.NewObjectStoreWithKeyspace(session, keyspace),
		Links:       cassandrakernel.NewLinkStoreWithKeyspace(session, keyspace),
		Actions:     cassandrakernel.NewActionLogStoreWithKeyspace(session, keyspace),
		Definitions: domain.NewPostgresDefinitionStore(pool),
		ReadModels:  cassandrakernel.NewReadModelStoreWithKeyspace(session, keyspace),
		Search:      searchBackend,
	}, cleanup, nil
}

func buildSearchBackend(cfg *config.Config) (storageabstraction.SearchBackend, error) {
	backend := strings.TrimSpace(cfg.SearchBackend)
	endpoint := strings.TrimSpace(cfg.SearchEndpoint)
	if backend == "" && endpoint == "" {
		return nil, nil
	}
	if backend == "" {
		backend = searchabstraction.BackendVespa.String()
	}
	if endpoint == "" {
		return nil, errors.New("SEARCH_ENDPOINT is required when SEARCH_BACKEND is configured")
	}
	choice, ok := searchabstraction.ParseBackendChoice(backend)
	if !ok {
		return nil, fmt.Errorf("unsupported SEARCH_BACKEND %q", backend)
	}
	switch choice {
	case searchabstraction.BackendVespa:
		return vespa.NewWithOptions(endpoint, vespa.WithAuthHeader(cfg.SearchAuthHeader)), nil
	case searchabstraction.BackendOpenSearch:
		return opensearch.NewWithOptions(endpoint, opensearch.WithAuthHeader(cfg.SearchAuthHeader)), nil
	default:
		return nil, fmt.Errorf("unsupported SEARCH_BACKEND %q", backend)
	}
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func run(ctx context.Context, srv *http.Server, log *slog.Logger) error {
	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		log.Info("shutting down")
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
