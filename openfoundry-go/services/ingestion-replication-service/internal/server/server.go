// Package server wires the chi router for ingestion-replication-service.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/core-models/health"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/ingestion-replication-service/internal/handlers"
)

// StreamingMetadata is the optional bundle of streaming submodule
// handlers (schemas, profiles, topologies, …). Each field is wired
// independently — nil entries skip route registration so the foundation
// build stays lean for environments that haven't enabled a submodule.
//
// IRF-9 ships the Schemas slot; later slices add their own.
type StreamingMetadata struct {
	Schemas *handlers.SchemasHandler
}

func New(cfg *config.Config, jwt *authmw.JWTConfig, h *handlers.Handlers, m *observability.Metrics, sm StreamingMetadata) *http.Server {
	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Recoverer, chimw.Compress(5))
	r.Use(chimw.Timeout(30 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(cfg.Service.Name, cfg.Service.Version))
	})
	r.Method(http.MethodGet, "/metrics", m.Handler())

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(authmw.Middleware(jwt))

		api.Get("/ingest-jobs", h.ListIngestJobs)
		api.Post("/ingest-jobs", h.CreateIngestJob)
		api.Get("/ingest-jobs/{id}", h.GetIngestJob)
		api.Patch("/ingest-jobs/{id}", h.UpdateIngestJob)
		api.Delete("/ingest-jobs/{id}", h.DeleteIngestJob)

		api.Get("/streams", h.ListStreams)
		api.Post("/streams", h.CreateStream)
		api.Get("/streams/{id}", h.GetStream)
		api.Patch("/streams/{id}", h.UpdateStream)
		// Foundry "Reset stream" — rotates view RID, retires the
		// previous view, truncates the underlying topic + resets
		// consumer offsets. Path mirrors the Rust router exactly.
		api.Post("/streams/{id}:reset", h.ResetStream)

		if sm.Schemas != nil {
			// Confluent-style endpoints: the Rust router uses
			// `:validate` (a chi-friendly suffix). The Go side mirrors
			// the wire path exactly so clients compiled against the
			// Rust binary keep working.
			api.Post("/streams/{id}/schema:validate", sm.Schemas.ValidateSchema)
			api.Get("/streams/{id}/schema/history", sm.Schemas.ListSchemaHistory)
		}

		api.Get("/cdc/streams", h.ListCdcStreams)
		api.Post("/cdc/streams", h.RegisterCdcStream)
		api.Get("/cdc/streams/{id}", h.GetCdcStream)
		api.Get("/cdc/streams/{id}/checkpoint", h.GetCdcCheckpoint)
		api.Get("/cdc/streams/{id}/resolution", h.GetCdcResolution)
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	return &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
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
