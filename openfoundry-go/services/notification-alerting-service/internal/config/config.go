// Package config resolves notification-alerting-service configuration.
//
// Operator-facing env contract matches the Rust crate so a single Helm
// values.yaml drives both implementations.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

// Config bundles every knob the service consumes.
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
	NATSURL     string

	SMTP struct {
		Host        string
		Port        uint16
		Username    string
		Password    string
		FromAddress string
		FromName    string
	}

	MetricsAddr string
	OTLPEndpoint string
}

// FromEnv resolves Config from process env. Required vars: DATABASE_URL, JWT_SECRET.
func FromEnv() (*Config, error) {
	cfg := &Config{}
	cfg.Service.Name = "notification-alerting-service"
	cfg.Service.Version = defaultStr(os.Getenv("SERVICE_VERSION"), "dev")
	cfg.Server.Host = defaultStr(os.Getenv("HOST"), "0.0.0.0")
	cfg.Server.Port = parseUint16(os.Getenv("PORT"), 50114)

	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		return nil, &MissingEnvError{Key: "DATABASE_URL"}
	}
	cfg.JWTSecret = defaultStr(os.Getenv("OPENFOUNDRY_JWT_SECRET"), os.Getenv("JWT_SECRET"))
	if cfg.JWTSecret == "" {
		return nil, &MissingEnvError{Key: "JWT_SECRET"}
	}
	cfg.NATSURL = os.Getenv("NATS_URL")

	cfg.SMTP.Host = os.Getenv("SMTP_HOST")
	cfg.SMTP.Port = parseUint16(os.Getenv("SMTP_PORT"), 587)
	cfg.SMTP.Username = os.Getenv("SMTP_USERNAME")
	cfg.SMTP.Password = os.Getenv("SMTP_PASSWORD")
	cfg.SMTP.FromAddress = os.Getenv("SMTP_FROM_ADDRESS")
	cfg.SMTP.FromName = os.Getenv("SMTP_FROM_NAME")

	cfg.MetricsAddr = defaultStr(os.Getenv("METRICS_ADDR"), "0.0.0.0:9090")
	cfg.OTLPEndpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")

	return cfg, nil
}

// MissingEnvError signals a required env var was unset.
type MissingEnvError struct{ Key string }

func (e *MissingEnvError) Error() string {
	return fmt.Sprintf("required environment variable %s is not set", e.Key)
}

// IsMissingEnv reports whether err is a MissingEnvError.
func IsMissingEnv(err error) bool {
	var me *MissingEnvError
	return errors.As(err, &me)
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
