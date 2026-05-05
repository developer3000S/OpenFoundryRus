// Package server hosts the small HTTP surface (healthz + metrics) the
// sinks expose for the platform's k8s probes and Prometheus scrape.
package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/openfoundry/openfoundry-go/libs/core-models/health"
	"github.com/openfoundry/openfoundry-go/services/audit-sink/internal/runtime"

	"encoding/json"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server bundles a tiny http.Server.
type Server struct {
	srv *http.Server
}

// New wires GET /healthz + GET /metrics on `addr`.
func New(addr, serviceName, version string, m *runtime.Metrics) *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(serviceName, version))
	})
	mux.Handle("/metrics", promhttp.HandlerFor(m.Registry,
		promhttp.HandlerOpts{Registry: m.Registry}))

	return &Server{srv: &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}}
}

// Run blocks until ctx is done or the listener returns.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
