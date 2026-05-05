package databus

import (
	"crypto/tls"
	"fmt"
	"time"

	kafka "github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

// SASL mechanism + security protocol tokens — match the Rust constants
// so Helm values and operator runbooks are language-agnostic.
const (
	SASLMechanismScramSHA512 = "SCRAM-SHA-512"
	SASLMechanismScramSHA256 = "SCRAM-SHA-256"
	SASLMechanismPlain       = "PLAIN"

	SecurityProtocolSASLSSL  = "SASL_SSL"
	SecurityProtocolPlain    = "PLAINTEXT"
)

// ServicePrincipal is the SASL identity a service uses against the broker.
//
// The platform provisions one principal per service so Kafka ACLs can be
// granted by service identity rather than by IP or shared credentials.
type ServicePrincipal struct {
	Service          string // SASL username + client.id prefix
	Password         string
	Mechanism        string // SCRAM-SHA-512 (default) / SCRAM-SHA-256 / PLAIN
	SecurityProtocol string // SASL_SSL (default) / PLAINTEXT
}

// ScramSHA512 builds a production-style principal.
func ScramSHA512(service, password string) ServicePrincipal {
	return ServicePrincipal{
		Service:          service,
		Password:         password,
		Mechanism:        SASLMechanismScramSHA512,
		SecurityProtocol: SecurityProtocolSASLSSL,
	}
}

// InsecureDev builds a dev-only principal that talks to an
// unauthenticated broker. Use for testcontainers / local docker-compose only.
func InsecureDev(service string) ServicePrincipal {
	return ServicePrincipal{
		Service:          service,
		SecurityProtocol: SecurityProtocolPlain,
	}
}

// dialer constructs a kafka.Dialer wired with the principal's credentials.
func (p ServicePrincipal) dialer(timeout time.Duration) (*kafka.Dialer, error) {
	d := &kafka.Dialer{
		Timeout:   timeout,
		ClientID:  p.Service,
		DualStack: true,
	}
	if p.SecurityProtocol == SecurityProtocolSASLSSL {
		d.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	if p.Mechanism == "" {
		return d, nil
	}
	mech, err := buildSASL(p)
	if err != nil {
		return nil, err
	}
	d.SASLMechanism = mech
	return d, nil
}

func buildSASL(p ServicePrincipal) (sasl.Mechanism, error) {
	switch p.Mechanism {
	case SASLMechanismScramSHA512:
		return scram.Mechanism(scram.SHA512, p.Service, p.Password)
	case SASLMechanismScramSHA256:
		return scram.Mechanism(scram.SHA256, p.Service, p.Password)
	case SASLMechanismPlain:
		return plain.Mechanism{Username: p.Service, Password: p.Password}, nil
	default:
		return nil, fmt.Errorf("unsupported SASL mechanism %q", p.Mechanism)
	}
}

// Config is the top-level data-bus configuration.
//
// Defaults match the Rust DataBusConfig: 30s request timeout, zstd
// compression, idempotent acks=all on producers, no auto-create, no
// auto-commit on consumers.
type Config struct {
	BootstrapServers []string
	Principal        ServicePrincipal
	RequestTimeout   time.Duration
	Compression      kafka.Compression
}

// NewConfig builds a Config with the Rust crate's defaults.
func NewConfig(bootstrap []string, principal ServicePrincipal) Config {
	return Config{
		BootstrapServers: bootstrap,
		Principal:        principal,
		RequestTimeout:   30 * time.Second,
		Compression:      kafka.Zstd,
	}
}

// WithCompression overrides the producer compression codec.
//
// Pass kafka.Compression(0) to disable compression entirely.
func (c Config) WithCompression(codec kafka.Compression) Config {
	c.Compression = codec
	return c
}
