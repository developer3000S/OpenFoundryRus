// Package config resolves solution-design-service env config.
//
// The Rust binary is `fn main(){}` but handlers.rs already
// implements full CRUD on solution_diagrams + solution_references.
// The Go port is canonical and wires those handlers behind a
// chi router with JWT auth on /api/v1/solution-design.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Service struct{ Name, Version string }
	Server  struct {
		Host string
		Port uint16
	}
	DatabaseURL string
	JWTSecret   string
}

func FromEnv() (*Config, error) {
	cfg := &Config{}
	cfg.Service.Name = "solution-design-service"
	cfg.Service.Version = defaultStr(os.Getenv("SERVICE_VERSION"), "dev")
	cfg.Server.Host = defaultStr(os.Getenv("HOST"), "0.0.0.0")
	cfg.Server.Port = parseUint16(os.Getenv("PORT"), 50142)
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	cfg.JWTSecret = os.Getenv("JWT_SECRET")
	if cfg.DatabaseURL == "" {
		return nil, &MissingEnvError{Key: "DATABASE_URL"}
	}
	if cfg.JWTSecret == "" {
		return nil, &MissingEnvError{Key: "JWT_SECRET"}
	}
	return cfg, nil
}

type MissingEnvError struct{ Key string }

func (e *MissingEnvError) Error() string {
	return fmt.Sprintf("required environment variable %s is not set", e.Key)
}

func IsMissingEnv(err error) bool { var me *MissingEnvError; return errors.As(err, &me) }

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
