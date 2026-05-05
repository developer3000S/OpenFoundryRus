// Package server wires the HTTP router, observability and graceful
// shutdown for the template service. Real services should keep this
// file's shape and only extend the routing tables in `mountAPIRoutes`.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/template/internal/config"
	"github.com/openfoundry/openfoundry-go/services/template/internal/handler"
)

// Server bundles the lifecycle of the HTTP listener.
type Server struct {
	httpServer *http.Server
	cfg        *config.Config
	log        *slog.Logger
}

// New builds a Server with all middleware and routes mounted.
func New(cfg *config.Config, metrics *observability.Metrics, log *slog.Logger) (*Server, error) {
	jwtCfg := authmw.NewJWTConfig(cfg.JWT.Secret).
		WithIssuer(cfg.JWT.Issuer).
		WithAudience(cfg.JWT.Audience)

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Compress(5))
	r.Use(chimw.Timeout(30 * time.Second))

	// Public endpoints (no auth).
	r.Get("/healthz", handler.Health(cfg.Service.Name, cfg.Service.Version))
	r.Method(http.MethodGet, "/metrics", metrics.Handler())

	// Authenticated API mount.
	r.Route("/api", func(api chi.Router) {
		api.Use(authmw.Middleware(jwtCfg))
		mountAPIRoutes(api)
	})

	shutdownTimeout := 15 * time.Second
	if d, err := time.ParseDuration(cfg.Server.ShutdownTimeout); err == nil {
		shutdownTimeout = d
	}

	s := &Server{
		cfg: cfg,
		log: log,
		httpServer: &http.Server{
			Addr:              cfg.Server.Addr,
			Handler:           r,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
	_ = shutdownTimeout // surfaced via Stop()
	return s, nil
}

// Run blocks until the listener returns or `ctx` is cancelled.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("listening", slog.String("addr", s.cfg.Server.Addr))
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		return s.shutdown()
	case err := <-errCh:
		return err
	}
}

func (s *Server) shutdown() error {
	timeout := 15 * time.Second
	if d, err := time.ParseDuration(s.cfg.Server.ShutdownTimeout); err == nil {
		timeout = d
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	s.log.Info("shutting down")
	return s.httpServer.Shutdown(ctx)
}

// mountAPIRoutes is the single hook real services extend.
func mountAPIRoutes(r chi.Router) {
	r.Get("/whoami", func(w http.ResponseWriter, req *http.Request) {
		c, _ := authmw.FromContext(req.Context())
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"email":"` + c.Email + `"}`))
	})
}
