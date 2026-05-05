// Package config resolves audit-sink configuration from the operator-
// facing environment contract.
//
// Variable names match the Rust crate verbatim so a single Helm
// values.yaml drives both implementations during cutover.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	databus "github.com/openfoundry/openfoundry-go/libs/event-bus-data"
)

// Iceberg target identifiers — pinned constants. A typo here is a
// compile error, not a silent topic-redirect.
const (
	IcebergCatalog          = "lakekeeper"
	IcebergNamespace        = "of_audit"
	IcebergTable            = "events"
	IcebergPartitionTransform = "day(at)"
	IcebergSortOrder        = "at ASC"
)

// Topic + consumer-group constants.
const (
	SourceTopic   = "audit.events.v1"
	ConsumerGroup = "audit-sink"
)

// BatchDefaultMaxRecords / BatchDefaultMaxWait — Rust constants.
const (
	BatchDefaultMaxRecords = 100_000
	BatchDefaultMaxWait    = 60 * time.Second
)

// Config is the resolved runtime configuration.
type Config struct {
	Service struct {
		Name    string
		Version string
	}
	DataBus     databus.Config
	CatalogURL  string
	Warehouse   string
	BatchPolicy BatchPolicy
	MetricsAddr string

	// JSONLWriterPath is non-empty when the writer should be the
	// JSONL-file fallback (development / staging). When empty, the
	// runtime uses the real Iceberg writer (still a stub — see
	// internal/writer/iceberg.go).
	JSONLWriterPath string
}

// BatchPolicy mirrors the Rust BatchPolicy.
type BatchPolicy struct {
	MaxRecords int
	MaxWait    time.Duration
}

// ShouldFlush mirrors `should_flush(records, elapsed)`.
func (p BatchPolicy) ShouldFlush(records int, elapsed time.Duration) bool {
	return records >= p.MaxRecords || elapsed >= p.MaxWait
}

// FromEnv resolves Config or returns a typed *MissingEnvError /
// *InvalidEnvError so the operator runbook can pinpoint the bad knob.
func FromEnv() (*Config, error) {
	bootstrap := os.Getenv("KAFKA_BOOTSTRAP_SERVERS")
	if bootstrap == "" {
		return nil, &MissingEnvError{Key: "KAFKA_BOOTSTRAP_SERVERS"}
	}
	catalogURL := os.Getenv("ICEBERG_CATALOG_URL")
	if catalogURL == "" {
		return nil, &MissingEnvError{Key: "ICEBERG_CATALOG_URL"}
	}

	principal := buildPrincipal()
	dbCfg := databus.NewConfig(splitCSV(bootstrap), principal)

	policy := BatchPolicy{
		MaxRecords: BatchDefaultMaxRecords,
		MaxWait:    BatchDefaultMaxWait,
	}
	if v, ok, err := lookupInt("AUDIT_SINK_BATCH_MAX_RECORDS"); err != nil {
		return nil, err
	} else if ok {
		policy.MaxRecords = v
	}
	if v, ok, err := lookupSecs("AUDIT_SINK_BATCH_MAX_WAIT_SECONDS"); err != nil {
		return nil, err
	} else if ok {
		policy.MaxWait = v
	}

	cfg := &Config{
		DataBus:         dbCfg,
		CatalogURL:      catalogURL,
		Warehouse:       os.Getenv("ICEBERG_WAREHOUSE"),
		BatchPolicy:     policy,
		MetricsAddr:     defaultStr(os.Getenv("METRICS_ADDR"), "0.0.0.0:9090"),
		JSONLWriterPath: os.Getenv("AUDIT_SINK_JSONL_PATH"),
	}
	cfg.Service.Name = "audit-sink"
	cfg.Service.Version = defaultStr(os.Getenv("SERVICE_VERSION"), "dev")
	return cfg, nil
}

func buildPrincipal() databus.ServicePrincipal {
	password := os.Getenv("KAFKA_SASL_PASSWORD")
	username := defaultStr(os.Getenv("KAFKA_SASL_USERNAME"),
		defaultStr(os.Getenv("KAFKA_CLIENT_ID"), "audit-sink"))

	if password == "" {
		// Dev mode — unauthenticated broker (testcontainers / docker-compose).
		return databus.InsecureDev(username)
	}
	mechanism := defaultStr(os.Getenv("KAFKA_SASL_MECHANISM"), databus.SASLMechanismScramSHA512)
	protocol := defaultStr(os.Getenv("KAFKA_SECURITY_PROTOCOL"), databus.SecurityProtocolSASLSSL)
	return databus.ServicePrincipal{
		Service:          username,
		Password:         password,
		Mechanism:        mechanism,
		SecurityProtocol: protocol,
	}
}

func splitCSV(v string) []string {
	out := []string{}
	current := ""
	for _, ch := range v {
		switch ch {
		case ',':
			if current != "" {
				out = append(out, current)
				current = ""
			}
		case ' ', '\t':
			// skip whitespace
		default:
			current += string(ch)
		}
	}
	if current != "" {
		out = append(out, current)
	}
	return out
}

func lookupInt(key string) (int, bool, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, false, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0, false, &InvalidEnvError{Key: key, Value: raw, Reason: "expected positive integer"}
	}
	return v, true, nil
}

func lookupSecs(key string) (time.Duration, bool, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, false, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0, false, &InvalidEnvError{Key: key, Value: raw, Reason: "expected positive integer (seconds)"}
	}
	return time.Duration(v) * time.Second, true, nil
}

func defaultStr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// MissingEnvError signals a required env var is unset.
type MissingEnvError struct{ Key string }

func (e *MissingEnvError) Error() string {
	return fmt.Sprintf("required environment variable %s is not set", e.Key)
}

// InvalidEnvError signals an env var is present but malformed.
type InvalidEnvError struct {
	Key, Value, Reason string
}

func (e *InvalidEnvError) Error() string {
	return fmt.Sprintf("invalid environment variable %s=%q: %s", e.Key, e.Value, e.Reason)
}

// IsMissingEnv reports whether err originates from FromEnv as a missing-required.
func IsMissingEnv(err error) bool {
	var me *MissingEnvError
	return errors.As(err, &me)
}
