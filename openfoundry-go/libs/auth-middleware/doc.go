// Package authmw provides JWT authentication primitives shared across
// every OpenFoundry Go service.
//
// What this package owns (Phase 0):
//   - Claims / SessionScope wire-compatible with the Rust workspace.
//   - JWTConfig with HS256 + RS256 support.
//   - EncodeToken / DecodeToken.
//   - HTTP middleware (chi-compatible) that extracts the bearer token,
//     validates it, and stuffs the resulting Claims into request context.
//
// Deferred (port together with `identity-federation-service` in Phase 2):
//   - Unattended secret resolution from env / disk.
//   - RBAC sub-package.
//   - Row-level-security predicates.
//   - Tenant resolution helpers.
//   - Markings sub-package.
package authmw
