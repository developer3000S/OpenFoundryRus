// Package server wires the chi router for iceberg-catalog-service.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/core-models/health"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/iceberg-catalog-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/iceberg-catalog-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/iceberg-catalog-service/internal/handlers/auth"
	icmetrics "github.com/openfoundry/openfoundry-go/services/iceberg-catalog-service/internal/metrics"
)

// Deps bundles every collaborator the chi router needs. Built once in
// `cmd/iceberg-catalog-service/main.go` so the server signature stays
// stable as new endpoints land.
type Deps struct {
	Handlers       *handlers.Handlers
	Markings       *handlers.MarkingsHandlers
	Bearer         *auth.Config
	BearerStore    auth.TokenStore
	IssueAPIStore  auth.IssueAPITokenStore
	OAuthValidator auth.OAuthClientValidator
	Metrics        *icmetrics.Metrics
}

func New(cfg *config.Config, jwt *authmw.JWTConfig, deps Deps, m *observability.Metrics) *http.Server {
	h := deps.Handlers
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Recoverer, chimw.Compress(5))
	r.Use(chimw.Timeout(30 * time.Second))
	if deps.Metrics != nil {
		r.Use(instrument(deps.Metrics))
	}

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(cfg.Service.Name, cfg.Service.Version))
	})
	r.Get("/version", versionHandler(cfg))
	r.Method(http.MethodGet, "/metrics", m.Handler())

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(authmw.Middleware(jwt))

		api.Get("/namespaces", h.ListNamespaces)
		api.Post("/namespaces", h.CreateNamespace)
		api.Get("/namespaces/{id}", h.GetNamespace)
		api.Patch("/namespaces/{id}", h.UpdateNamespace)
		api.Delete("/namespaces/{id}", h.DeleteNamespace)

		api.Get("/namespaces/{namespace}/tables", h.ListTables)
		api.Post("/namespaces/{namespace}/tables", h.CreateTable)
		api.Post("/tables/rename", h.RenameTable)
		api.Get("/namespaces/{namespace}/tables/{table}", h.LoadTable)
		api.Post("/namespaces/{namespace}/tables/{table}", h.CommitTable)
		api.Delete("/namespaces/{namespace}/tables/{table}", h.DropTable)
		api.Get("/namespaces/{namespace}/tables/{table}/refs", h.ListRefs)
		api.Get("/namespaces/{namespace}/tables/{table}/refs/{ref}", h.GetRef)
		api.Put("/namespaces/{namespace}/tables/{table}/refs/{ref}", h.UpsertRef)
		api.Delete("/namespaces/{namespace}/tables/{table}/refs/{ref}", h.DeleteRef)
		api.Get("/namespaces/{namespace}/tables/{table}/metadata", h.ListMetadataFiles)
		api.Get("/namespaces/{namespace}/tables/{table}/metadata/{version}", h.GetMetadataFile)
		api.Get("/namespaces/{namespace}/tables/{table}/snapshots", h.ListSnapshots)
		api.Get("/namespaces/{namespace}/tables/{table}/snapshots/{snapshot_id}", h.GetSnapshot)
	})

	r.Post("/openfoundry/iceberg/v1/append", h.AppendBatch)

	// OAuth2 token endpoint — public per the REST Catalog spec.
	if deps.Bearer != nil {
		r.Post("/iceberg/v1/oauth/tokens", auth.IssueTokenHandler(deps.Bearer, deps.OAuthValidator))
	}

	// API-token issuance is gated by the Foundry JWT middleware.
	if deps.IssueAPIStore != nil {
		r.Route("/v1/iceberg-clients", func(api chi.Router) {
			api.Use(authmw.Middleware(jwt))
			ttl := int64(0)
			if cfg != nil {
				ttl = cfg.LongLivedTokenTTLSec
			}
			api.Post("/api-tokens", auth.CreateAPITokenHandler(deps.IssueAPIStore, ttl))
		})
	}

	// Markings endpoints sit on a SEPARATE router so they can run the
	// new bearer middleware (with read/write scope enforcement) while
	// the rest of /iceberg/v1 keeps the Foundry JWT chain.
	if deps.Markings != nil && deps.Bearer != nil {
		r.Route("/iceberg/v1/namespaces/{namespace}/markings", func(api chi.Router) {
			api.Use(auth.Middleware(deps.Bearer, deps.BearerStore))
			api.Get("/", deps.Markings.GetNamespaceMarkings)
			api.Post("/", deps.Markings.UpdateNamespaceMarkings)
		})
		r.Route("/iceberg/v1/namespaces/{namespace}/tables/{table}/markings", func(api chi.Router) {
			api.Use(auth.Middleware(deps.Bearer, deps.BearerStore))
			api.Get("/", deps.Markings.GetTableMarkings)
			api.Patch("/", deps.Markings.UpdateTableMarkings)
		})
	}

	r.Route("/iceberg/v1", func(api chi.Router) {
		api.Use(authmw.Middleware(jwt))
		api.Get("/config", h.GetConfig)
		api.Post("/diagnose", h.RunDiagnose)
		api.Get("/namespaces/{namespace}/tables", h.ListTables)
		api.Post("/namespaces/{namespace}/tables", h.CreateTable)
		api.Post("/tables/rename", h.RenameTable)
		api.Get("/namespaces/{namespace}/tables/{table}", h.LoadTable)
		api.Post("/namespaces/{namespace}/tables/{table}", h.CommitTable)
		api.Delete("/namespaces/{namespace}/tables/{table}", h.DropTable)
		api.Get("/namespaces/{namespace}/tables/{table}/refs", h.ListRefs)
		api.Get("/namespaces/{namespace}/tables/{table}/refs/{ref}", h.GetRef)
		api.Put("/namespaces/{namespace}/tables/{table}/refs/{ref}", h.UpsertRef)
		api.Delete("/namespaces/{namespace}/tables/{table}/refs/{ref}", h.DeleteRef)
		api.Get("/namespaces/{namespace}/tables/{table}/metadata", h.ListMetadataFiles)
		api.Get("/namespaces/{namespace}/tables/{table}/metadata/{version}", h.GetMetadataFile)
		api.Get("/namespaces/{namespace}/tables/{table}/snapshots", h.ListSnapshots)
		api.Get("/namespaces/{namespace}/tables/{table}/snapshots/{snapshot_id}", h.GetSnapshot)
		api.Post("/transactions/commit", h.MultiTableCommit)
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	return &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// instrument records the per-route counter, histogram and in-flight
// gauge declared by the iceberg metrics package. The route label is
// resolved via chi's RouteContext after the handler chain runs so
// path-parameterised endpoints aggregate under a single label rather
// than blowing up cardinality (one series per concrete URL).
func instrument(m *icmetrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)

			// In-flight gauge is keyed on method + a coarse "all"
			// endpoint label rather than the request URL: chi has not
			// resolved the route pattern at this point and using the
			// raw path would let arbitrary client input expand the
			// label set.
			inFlight := m.RESTRequestsInFlight.WithLabelValues(r.Method, "all")
			inFlight.Inc()
			defer inFlight.Dec()

			next.ServeHTTP(ww, r)

			route := routeLabel(r)
			method := r.Method
			status := strconv.Itoa(ww.Status())
			if ww.Status() == 0 {
				status = "200"
			}

			m.RESTRequestsTotal.WithLabelValues(method, route, status).Inc()
			m.RESTRequestLatencySeconds.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
		})
	}
}

// routeLabel resolves the chi route pattern for a request, falling back
// to the URL path when chi has no template registered (e.g. for the
// root `/healthz` and `/metrics` endpoints).
func routeLabel(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if pattern := rctx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return r.URL.Path
}

// versionHandler renders BUILD_GIT_SHA and the configured semantic
// version as JSON. Mirrors the `/version` correlator the build pipeline
// uses to align deploys with the originating commit.
func versionHandler(cfg *config.Config) http.HandlerFunc {
	gitSHA := os.Getenv("BUILD_GIT_SHA")
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"service":       cfg.Service.Name,
			"version":       cfg.Service.Version,
			"build_git_sha": gitSHA,
		})
	}
}

func Run(ctx context.Context, srv *http.Server, log *slog.Logger) error {
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
