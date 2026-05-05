// Package coremodels groups the canonical primitives shared by every
// OpenFoundry service. Subpackages are organised by domain:
//
//   - ids        — typed UUID v7 identifiers
//   - errs       — shared error taxonomy
//   - health     — /healthz response payload
//   - pagination — cursor-based PageRequest / PageResponse
//   - timestamp  — created_at / updated_at envelope
//   - dataset    — dataset RID, branch, transaction state, schema
//   - security   — markings / classifications
//   - media      — Foundry-compatible MediaReference
//
// All wire formats (JSON tag names, enum casing) match the Rust
// implementation in libs/core-models verbatim so a service written in
// either language can exchange payloads with the other.
package coremodels
