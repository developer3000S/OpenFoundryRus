// Package health exposes the canonical /healthz response payload.
package health

import "time"

// Status mirrors the Rust `core_models::health::HealthStatus` shape so
// /healthz responses are byte-identical between Rust and Go services.
type Status struct {
	Status    string    `json:"status"`
	Service   string    `json:"service"`
	Version   string    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
}

// OK builds a "status: ok" payload for the named service.
//
// version is read from the binary build metadata (set via -ldflags).
func OK(service, version string) Status {
	return Status{
		Status:    "ok",
		Service:   service,
		Version:   version,
		Timestamp: time.Now().UTC(),
	}
}
