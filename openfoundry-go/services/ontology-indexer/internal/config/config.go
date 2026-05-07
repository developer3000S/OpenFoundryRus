// Package config resolves ontology-indexer env config.
//
// ontology-indexer is a worker: Kafka in, SearchBackend out. The HTTP
// surface only exposes /healthz + /metrics for ops.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// BackendKind mirrors the Rust enum.
type BackendKind string

const (
	BackendVespa      BackendKind = "vespa"
	BackendOpenSearch BackendKind = "opensearch"
)

// BackendKindFromEnv defaults to Vespa when unset / empty (matches Rust).
func BackendKindFromEnv(v string) BackendKind {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "opensearch":
		return BackendOpenSearch
	default:
		return BackendVespa
	}
}

type Config struct {
	Service struct{ Name, Version string }
	Server  struct {
		Host string
		Port uint16
	}
	BackendKind         BackendKind
	SearchEndpoint      string
	SearchUsername      string
	SearchPassword      string
	SearchBearerToken   string
	SearchAPIKey        string
	KafkaBootstrap      string
	ConsumerGroup       string
	RetryMaxAttempts    int
	RetryInitialBackoff time.Duration
	RetryMaxBackoff     time.Duration
	DLQTopic            string
	MetricsAddr         string
}

func FromEnv() (*Config, error) {
	cfg := &Config{}
	cfg.Service.Name = "ontology-indexer"
	cfg.Service.Version = defaultStr(os.Getenv("SERVICE_VERSION"), "dev")
	cfg.Server.Host = defaultStr(os.Getenv("HOST"), "0.0.0.0")
	cfg.Server.Port = parseUint16(os.Getenv("PORT"), 50124)
	cfg.BackendKind = BackendKindFromEnv(os.Getenv("SEARCH_BACKEND"))
	cfg.SearchEndpoint = os.Getenv("SEARCH_ENDPOINT")
	cfg.SearchUsername = os.Getenv("SEARCH_USERNAME")
	cfg.SearchPassword = os.Getenv("SEARCH_PASSWORD")
	cfg.SearchBearerToken = os.Getenv("SEARCH_BEARER_TOKEN")
	cfg.SearchAPIKey = os.Getenv("SEARCH_API_KEY")
	cfg.KafkaBootstrap = os.Getenv("KAFKA_BOOTSTRAP_SERVERS")
	cfg.ConsumerGroup = defaultStr(os.Getenv("KAFKA_CONSUMER_GROUP"), "ontology-indexer")
	cfg.RetryMaxAttempts = parseInt(os.Getenv("INDEXER_RETRY_MAX_ATTEMPTS"), 3)
	cfg.RetryInitialBackoff = parseDuration(os.Getenv("INDEXER_RETRY_INITIAL_BACKOFF"), 100*time.Millisecond)
	cfg.RetryMaxBackoff = parseDuration(os.Getenv("INDEXER_RETRY_MAX_BACKOFF"), 2*time.Second)
	cfg.DLQTopic = parseDLQTopic(os.Getenv("INDEXER_DLQ_TOPIC"), "ontology-indexer.dlq.v1")
	cfg.MetricsAddr = defaultStr(os.Getenv("METRICS_ADDR"), "0.0.0.0:9090")
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

func parseInt(v string, fallback int) int {
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

func parseDuration(v string, fallback time.Duration) time.Duration {
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func parseDLQTopic(v string, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "off", "none", "disabled":
		return ""
	case "":
		return fallback
	default:
		return strings.TrimSpace(v)
	}
}
