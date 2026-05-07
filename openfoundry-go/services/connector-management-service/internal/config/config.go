// Package config resolves connector-management-service env config.
package config

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultHost                        = "0.0.0.0"
	DefaultPort                 uint16 = 50088
	DefaultDatasetServiceURL           = "http://localhost:50079"
	DefaultPipelineServiceURL          = "http://localhost:50080"
	DefaultOntologyServiceURL          = "http://localhost:50103"
	DefaultNetworkBoundaryURL          = "http://localhost:50119"
	DefaultSyncPollIntervalSecs        = 2
	DefaultAllowPrivateEgress          = true
	DefaultAgentStaleAfterSecs         = 120
	DefaultMediaSetsServiceURL         = "http://localhost:50156"
	DefaultVendedCredentialsTTL        = 900
)

type Config struct {
	Service struct{ Name, Version string }
	Server  struct {
		Host string
		Port uint16
	}
	DatabaseURL string
	JWTSecret   string
	MetricsAddr string

	DatasetServiceURL           string
	PipelineServiceURL          string
	OntologyServiceURL          string
	IngestionReplicationGRPCURL string
	NetworkBoundaryServiceURL   string
	SyncPollIntervalSecs        uint64
	AllowPrivateNetworkEgress   bool
	AllowedEgressHosts          []string
	AgentStaleAfterSecs         uint64
	MediaSetsServiceURL         string

	CredentialEncryptionKey      string
	CredentialKey                [32]byte
	SecretManagerURL             string
	OutboxEnabled                bool
	AutoRegistrationIntervalSecs uint64
	OpenFoundryDevAuth           bool
	VendedCredentialsTTLSeconds  int64
}

func FromEnv() (*Config, error) {
	cfg := &Config{}
	cfg.Service.Name = "connector-management-service"
	cfg.Service.Version = defaultStr(os.Getenv("SERVICE_VERSION"), "dev")
	cfg.Server.Host = defaultStr(os.Getenv("HOST"), DefaultHost)
	port, err := parseUint16("PORT", os.Getenv("PORT"), DefaultPort)
	if err != nil {
		return nil, err
	}
	cfg.Server.Port = port
	cfg.DatabaseURL = os.Getenv("DATABASE_URL")
	if cfg.DatabaseURL == "" {
		return nil, &MissingEnvError{Key: "DATABASE_URL"}
	}
	cfg.JWTSecret = defaultStr(os.Getenv("OPENFOUNDRY_JWT_SECRET"), os.Getenv("JWT_SECRET"))
	if cfg.JWTSecret == "" {
		return nil, &MissingEnvError{Key: "JWT_SECRET"}
	}
	cfg.MetricsAddr = defaultStr(os.Getenv("METRICS_ADDR"), "0.0.0.0:9090")

	cfg.DatasetServiceURL = defaultStr(os.Getenv("DATASET_SERVICE_URL"), DefaultDatasetServiceURL)
	cfg.PipelineServiceURL = defaultStr(os.Getenv("PIPELINE_SERVICE_URL"), DefaultPipelineServiceURL)
	cfg.OntologyServiceURL = defaultStr(os.Getenv("ONTOLOGY_SERVICE_URL"), DefaultOntologyServiceURL)
	cfg.IngestionReplicationGRPCURL = os.Getenv("INGESTION_REPLICATION_GRPC_URL")
	cfg.NetworkBoundaryServiceURL = defaultStr(os.Getenv("NETWORK_BOUNDARY_SERVICE_URL"), DefaultNetworkBoundaryURL)
	cfg.SyncPollIntervalSecs, err = parseUint64("SYNC_POLL_INTERVAL_SECS", os.Getenv("SYNC_POLL_INTERVAL_SECS"), DefaultSyncPollIntervalSecs)
	if err != nil {
		return nil, err
	}
	cfg.AllowPrivateNetworkEgress, err = parseBool("ALLOW_PRIVATE_NETWORK_EGRESS", os.Getenv("ALLOW_PRIVATE_NETWORK_EGRESS"), DefaultAllowPrivateEgress)
	if err != nil {
		return nil, err
	}
	cfg.AllowedEgressHosts = parseList(os.Getenv("ALLOWED_EGRESS_HOSTS"))
	cfg.AgentStaleAfterSecs, err = parseUint64("AGENT_STALE_AFTER_SECS", os.Getenv("AGENT_STALE_AFTER_SECS"), DefaultAgentStaleAfterSecs)
	if err != nil {
		return nil, err
	}
	cfg.MediaSetsServiceURL = defaultStr(os.Getenv("MEDIA_SETS_SERVICE_URL"), DefaultMediaSetsServiceURL)

	cfg.CredentialEncryptionKey = os.Getenv("CREDENTIAL_ENCRYPTION_KEY")
	cfg.CredentialKey, err = deriveCredentialKey(cfg.CredentialEncryptionKey, cfg.JWTSecret)
	if err != nil {
		return nil, err
	}
	cfg.SecretManagerURL = os.Getenv("SECRET_MANAGER_URL")
	cfg.OutboxEnabled, err = parseBool("OUTBOX_ENABLED", os.Getenv("OUTBOX_ENABLED"), true)
	if err != nil {
		return nil, err
	}
	cfg.AutoRegistrationIntervalSecs, err = parseUint64("OPENFOUNDRY_AUTO_REGISTRATION_INTERVAL_SECS", os.Getenv("OPENFOUNDRY_AUTO_REGISTRATION_INTERVAL_SECS"), 0)
	if err != nil {
		return nil, err
	}
	cfg.OpenFoundryDevAuth, err = parseBool("OPENFOUNDRY_DEV_AUTH", os.Getenv("OPENFOUNDRY_DEV_AUTH"), false)
	if err != nil {
		return nil, err
	}
	cfg.VendedCredentialsTTLSeconds, err = parseInt64("OPENFOUNDRY_VENDED_CREDENTIALS_TTL_SECS", os.Getenv("OPENFOUNDRY_VENDED_CREDENTIALS_TTL_SECS"), DefaultVendedCredentialsTTL)
	if err != nil {
		return nil, err
	}
	if cfg.VendedCredentialsTTLSeconds <= 0 {
		cfg.VendedCredentialsTTLSeconds = DefaultVendedCredentialsTTL
	}
	return cfg, nil
}

type MissingEnvError struct{ Key string }

func (e *MissingEnvError) Error() string {
	return fmt.Sprintf("required environment variable %s is not set", e.Key)
}

func IsMissingEnv(err error) bool { var me *MissingEnvError; return errors.As(err, &me) }

type InvalidEnvError struct {
	Key   string
	Value string
	Err   error
}

func (e *InvalidEnvError) Error() string {
	return fmt.Sprintf("invalid value for %s=%q: %v", e.Key, e.Value, e.Err)
}

func (e *InvalidEnvError) Unwrap() error { return e.Err }

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func parseUint16(key, v string, fallback uint16) (uint16, error) {
	if strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	n, err := strconv.ParseUint(v, 10, 16)
	if err != nil {
		return 0, &InvalidEnvError{Key: key, Value: v, Err: err}
	}
	return uint16(n), nil
}

func parseUint64(key, v string, fallback uint64) (uint64, error) {
	if strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, &InvalidEnvError{Key: key, Value: v, Err: err}
	}
	return n, nil
}

func parseInt64(key, v string, fallback int64) (int64, error) {
	if strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, &InvalidEnvError{Key: key, Value: v, Err: err}
	}
	return n, nil
}

func parseBool(key, v string, fallback bool) (bool, error) {
	if strings.TrimSpace(v) == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, &InvalidEnvError{Key: key, Value: v, Err: err}
	}
	return b, nil
}

func parseList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func deriveCredentialKey(envKeyB64, jwtSecret string) ([32]byte, error) {
	var out [32]byte
	if strings.TrimSpace(envKeyB64) == "" {
		digest := sha256.Sum256([]byte("openfoundry/credential-encryption/v1\x00" + jwtSecret))
		copy(out[:], digest[:])
		return out, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(envKeyB64))
	if err != nil {
		return out, &InvalidEnvError{Key: "CREDENTIAL_ENCRYPTION_KEY", Value: envKeyB64, Err: err}
	}
	if len(raw) != 32 {
		return out, &InvalidEnvError{Key: "CREDENTIAL_ENCRYPTION_KEY", Value: envKeyB64, Err: fmt.Errorf("must decode to 32 bytes (got %d)", len(raw))}
	}
	copy(out[:], raw)
	return out, nil
}
