// Command notification-alerting-service is the inbox + delivery + websocket service.
//
// 1:1 functional parity with services/notification-alerting-service in
// the Rust workspace.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/config"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/server"
	"github.com/openfoundry/openfoundry-go/services/notification-alerting-service/internal/service"
)

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

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("pgx pool failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()
	if err := repo.Migrate(ctx, pool); err != nil {
		log.Error("migrations failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	jwt := authmw.NewJWTConfig(cfg.JWTSecret)

	notifyRepo := &repo.NotificationsRepo{Pool: pool}
	prefsRepo := &repo.PreferencesRepo{Pool: pool}
	smtpSender := buildSMTP(cfg)

	var bus *service.NotificationBus
	var natsCloser func()
	if cfg.NATSURL != "" {
		bus, natsCloser, err = service.NewNotificationBus(ctx, cfg.NATSURL, cfg.Service.Name)
		if err != nil {
			log.Warn("NATS unavailable — websocket fan-out disabled",
				slog.String("error", err.Error()))
			bus = nil
		} else {
			defer natsCloser()
		}
	} else {
		log.Warn("NATS_URL not configured; distributed websocket notifications are disabled")
	}

	httpClient := &http.Client{Timeout: 10 * time.Second}
	notifier := &service.Notifier{
		Notifications: notifyRepo,
		Preferences:   prefsRepo,
		SMTP:          smtpSender,
		HTTP:          httpClient,
		Bus:           bus,
	}

	metrics := observability.NewMetrics()
	srv := server.New(server.Deps{
		Config:        cfg,
		JWT:           jwt,
		Notifications: notifyRepo,
		Preferences:   prefsRepo,
		Notifier:      notifier,
		Bus:           bus,
		Metrics:       metrics,
	})

	if err := server.Run(ctx, srv, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("server exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func buildSMTP(cfg *config.Config) *service.SMTPSender {
	if cfg.SMTP.Host == "" {
		return nil
	}
	return &service.SMTPSender{
		Host:        cfg.SMTP.Host,
		Port:        cfg.SMTP.Port,
		Username:    cfg.SMTP.Username,
		Password:    cfg.SMTP.Password,
		FromAddress: cfg.SMTP.FromAddress,
		FromName:    cfg.SMTP.FromName,
	}
}
