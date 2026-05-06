// Package config resolves model-deployment-service env config.
//
// The Rust binary is `fn main(){}` and every public surface is a
// `#[path]` re-export from libs/ml-kernel:
//   - models/deployment from libs/ml-kernel/src/models/deployment.rs
//   - domain/drift     from libs/ml-kernel/src/domain/drift.rs
//   - handlers/deployments from libs/ml-kernel/src/handlers/deployments.rs
//
// Substrate-only port — handlers wire alongside libs/ml-kernel-go
// handlers slice in a follow-up. The Go port consumes
// libs/ml-kernel-go/models for its DTO surface as a proof point.
package config

import (
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
	cfg.Service.Name = "model-deployment-service"
	cfg.Service.Version = defaultStr(os.Getenv("SERVICE_VERSION"), "dev")
	cfg.Server.Host = defaultStr(os.Getenv("HOST"), "0.0.0.0")
	cfg.Server.Port = parseUint16(os.Getenv("PORT"), 50086)
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	cfg.JWTSecret = os.Getenv("JWT_SECRET")
	return cfg, nil
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
