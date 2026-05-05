// Package server wires the chi router, middleware chain, and graceful
// shutdown for edge-gateway-service. The middleware order matches the
// Rust implementation:
//
//	request_id → CORS → audit → rate-limit → proxy
//
// Direct endpoints (`/healthz`, `/metrics`) sit OUTSIDE the proxy chain
// so they remain reachable when upstreams or rate-limit backends fail.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	controlbus "github.com/openfoundry/openfoundry-go/libs/event-bus-control"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/handler"
	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/middleware"
	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/middleware/ratelimit"
	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/proxy"
)

// Server bundles the lifecycle of the listener.
type Server struct {
	httpServer    *http.Server
	cfg           *config.Config
	log           *slog.Logger
	natsCloser    func()
	redisCloser   func()
}

// New builds a Server with all middleware mounted.
func New(ctx context.Context, cfg *config.Config, metrics *observability.Metrics, log *slog.Logger) (*Server, error) {
	jwtCfg := authmw.NewJWTConfig(cfg.JWT.Secret).
		WithIssuer(cfg.JWT.Issuer).
		WithAudience(cfg.JWT.Audience)

	rateStore, redisCloser := buildRateLimitStore(cfg, log)
	rateCfg := ratelimit.Config{
		AnonymousRequestsPerMinute: cfg.RateLimit.AnonymousRequestsPerMinute,
		BurstSize:                  cfg.RateLimit.BurstSize,
		BucketTTL:                  time.Duration(cfg.RateLimit.BucketTTLSecs) * time.Second,
		JWT:                        jwtCfg,
	}

	auditPub, natsCloser := buildAuditPublisher(ctx, cfg, log)

	r := chi.NewRouter()

	// Direct endpoints — outside the proxy chain.
	r.Use(chimw.Recoverer)
	r.Get("/healthz", handler.Health(cfg.Service.Name, cfg.Service.Version))
	r.Method(http.MethodGet, "/metrics", metrics.Handler())

	// Proxy chain — every other route is forwarded.
	proxyHandler := proxy.NewHandler(cfg, jwtCfg)
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequestID)
		r.Use(middleware.CORS(cfg.CORSOrigins))
		r.Use(middleware.Audit(middleware.AuditConfig{Publisher: auditPub}))
		r.Use(ratelimit.Middleware(rateCfg, rateStore))
		// Catch-all — chi's "/*" matches every method + path the
		// preceding direct routes did not consume.
		r.Handle("/*", proxyHandler)
	})

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	s := &Server{
		cfg: cfg,
		log: log,
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           r,
			ReadHeaderTimeout: 5 * time.Second,
		},
		natsCloser:  natsCloser,
		redisCloser: redisCloser,
	}
	return s, nil
}

// Run blocks until the listener returns or `ctx` is cancelled.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("listening", slog.String("addr", s.httpServer.Addr))
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
	err := s.httpServer.Shutdown(ctx)

	// Wait for in-flight audit publishes to finish (best-effort, bounded).
	middleware.AuditWait(ctx)

	if s.natsCloser != nil {
		s.natsCloser()
	}
	if s.redisCloser != nil {
		s.redisCloser()
	}
	return err
}

// buildRateLimitStore picks the Redis backend when RedisURL is set,
// falling back to the in-memory token bucket otherwise.
func buildRateLimitStore(cfg *config.Config, log *slog.Logger) (ratelimit.Store, func()) {
	if cfg.RedisURL == "" {
		log.Info("rate-limit using in-memory store (set redis_url for distributed mode)")
		return ratelimit.NewMemoryStore(time.Duration(cfg.RateLimit.BucketTTLSecs) * time.Second), nil
	}
	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Warn("failed to parse redis_url — falling back to in-memory rate-limit store",
			slog.String("error", err.Error()))
		return ratelimit.NewMemoryStore(time.Duration(cfg.RateLimit.BucketTTLSecs) * time.Second), nil
	}
	client := redis.NewClient(opts)
	store := ratelimit.NewRedisStore(client, cfg.RateLimit.RedisKeyPrefix,
		time.Duration(cfg.RateLimit.BucketTTLSecs)*time.Second)
	closer := func() { _ = client.Close() }
	return store, closer
}

// buildAuditPublisher dials NATS when configured. Failures fall back
// to a nil publisher (audit middleware becomes a no-op) so the gateway
// boots even when the message bus is down.
func buildAuditPublisher(ctx context.Context, cfg *config.Config, log *slog.Logger) (*controlbus.Publisher, func()) {
	if cfg.NATSURL == "" {
		return nil, nil
	}
	js, closer, err := controlbus.Connect(ctx, cfg.NATSURL)
	if err != nil {
		log.Warn("audit publishing disabled — NATS connect failed",
			slog.String("error", err.Error()))
		return nil, nil
	}
	if _, err := controlbus.EnsureStream(ctx, js, controlbus.StreamAudit,
		[]string{controlbus.SubjectAudit}); err != nil {
		log.Warn("audit publishing disabled — failed to ensure stream",
			slog.String("error", err.Error()))
		closer()
		return nil, nil
	}
	return controlbus.NewPublisher(js, "gateway"), closer
}
