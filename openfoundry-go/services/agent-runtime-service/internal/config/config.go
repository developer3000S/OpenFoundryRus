// Package config resolves agent-runtime-service env config.
//
// Post-S8.1.b (ADR-0030): this binary owns the agent runtime + the
// tool-registry surface absorbed from the retired tool-registry-service.
// The Rust binary is `fn main(){}` but agents handlers ship full CRUD;
// the Go port wires those canonically. Tools handlers re-export from
// libs/ai-kernel — the Go port mounts /api/v1/agent-runtime/tools in
// a follow-up slice once libs/ai-kernel-go/handlers/tools lands.
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
	DatabaseURL    string
	JWTSecret      string
	KafkaBootstrap string
}

func FromEnv() (*Config, error) {
	cfg := &Config{}
	cfg.Service.Name = "agent-runtime-service"
	cfg.Service.Version = defaultStr(os.Getenv("SERVICE_VERSION"), "dev")
	cfg.Server.Host = defaultStr(os.Getenv("HOST"), "0.0.0.0")
	cfg.Server.Port = parseUint16(os.Getenv("PORT"), 50127)
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	cfg.JWTSecret = os.Getenv("JWT_SECRET")
	cfg.KafkaBootstrap = os.Getenv("KAFKA_BOOTSTRAP_SERVERS")
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
