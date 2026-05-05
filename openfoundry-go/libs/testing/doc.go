// Package testingx hosts shared test utilities for OpenFoundry Go services.
//
// Mirrors the Rust `libs/testing` crate. Three families of helpers:
//
//   - containers — ephemeral Postgres via testcontainers-go.
//     Migration application is left to the caller; this package only
//     boots the container and returns a connected pgxpool.
//   - fixtures   — deterministic JWT issuance and SQL seed helpers
//     (datasets, branches, transactions, markings).
//   - mocks      — placeholder for wiremock equivalents (httptest +
//     hand-rolled stubs are usually enough in Go).
//
// All helpers are intentionally permissive (panic on misuse) — they
// are test-only.
//
// Build tag: integration tests live behind `//go:build integration`
// so unit-test runs do not require a Docker daemon.
package testingx
