// Package server wires the chi router for notification-alerting-service.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/core-models/health"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/handlers"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/service"
)

// Deps bundles everything the router needs.
type Deps struct {
	Config        *config.Config
	JWT           *authmw.JWTConfig
	Notifications *repo.NotificationsRepo
	Preferences   *repo.PreferencesRepo
	Notifier      *service.Notifier
	Bus           *service.NotificationBus // nil when NATS is unconfigured
	Metrics       *observability.Metrics
}

// New builds the http.Server with all middleware + routes mounted.
func New(d Deps) *http.Server {
	historyH := &handlers.History{Notifications: d.Notifications, Notifier: d.Notifier}
	prefsH := &handlers.Preferences{Repo: d.Preferences}
	sendH := &handlers.Send{Notifier: d.Notifier}
	wsH := &handlers.WS{JWT: d.JWT, Notifications: d.Notifications, Bus: d.Bus}

	r := chi.NewRouter()
	r.Use(chimw.RequestID, chimw.RealIP, chimw.Recoverer, chimw.Compress(5))
	r.Use(chimw.Timeout(30 * time.Second))

	// Direct endpoints — outside the auth chain.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(health.OK(d.Config.Service.Name, d.Config.Service.Version))
	})
	r.Method(http.MethodGet, "/metrics", d.Metrics.Handler())

	// /api/v1 — websocket upgrade is unauthenticated at the bearer layer
	// (it carries its own short-lived ticket as a query param).
	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/notifications/ws", wsH.Upgrade)

		api.Group(func(authed chi.Router) {
			authed.Use(authmw.Middleware(d.JWT))
			authed.Get("/notifications", historyH.List)
			authed.Patch("/notifications/{id}/read", historyH.MarkRead)
			authed.Post("/notifications/read-all", historyH.MarkAllRead)
			authed.Get("/notifications/preferences", prefsH.Get)
			authed.Put("/notifications/preferences", prefsH.Update)
			authed.Post("/notifications/ws-ticket", wsH.IssueTicket)
			authed.Post("/notifications/send", sendH.Authenticated)
		})
	})

	// /internal — no auth; restrict at network layer.
	r.Post("/internal/notifications", sendH.Internal)

	addr := d.Config.Server.Host + ":" + itoa(int(d.Config.Server.Port))
	return &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// Run blocks until ctx is done or the listener returns.
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

// itoa is a tiny strconv-free integer formatter to keep the package
// imports trim. The port is uint16 so 5 chars is enough.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [6]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
