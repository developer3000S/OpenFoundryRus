# `libs/authz-cedar-go`

Go port of `libs/authz-cedar`, backed by
[`github.com/cedar-policy/cedar-go`](https://github.com/cedar-policy/cedar-go)
v1.6.0+ (post-1.0; AWS maintains it in lock-step with cedar-rust v4
against the same conformance test suite).

## Status — conformance-ready port

This package now mirrors the reusable Rust `libs/authz-cedar` surfaces:

- `lib.go` — `PolicyStore` (in-memory `*cedar.PolicySet` + bundled
  schema, behind a `sync.RWMutex`), `PolicyRecord` (mirrors the
  `pg-policy.cedar_policies` row shape), `ReplacePolicies` with strict
  schema validation + atomic swap.
- `engine.go` — `AuthzEngine` + `AuthorizeOutcome`, fire-and-forget
  audit emission via goroutine.
- `audit.go` — `AuthzAuditEvent` (wire-format pinned to Rust
  `audit.authz.v1`), `AuthzAuditSink` interface, `NoopAuditSink`,
  `SlogAuditSink`.
- `errors.go` — `PolicyParseError`, `ValidationError`, sentinel errors.
- `cedar_schema.cedarschema` — bundled schema, copied verbatim from
  `libs/authz-cedar/`.
- `pg.go` — Postgres reload adapter for the latest active policy version
  per id, with atomic replacement semantics.
- `nats.go` — hot-reload subscriber interface for `authz.policy.changed`.
- `audit_kafka.go` — Kafka audit sink publishing `audit.authz.v1` with
  the Rust-compatible OpenLineage header shape.
- `chi.go` — chi middleware replacement for the Rust `axum.rs` guard.
- `iceberg_policies.go` / `schedule_policies.go` — policy bundles kept
  in parity with the Rust helpers.
- Tests covering schema parsing, policy validation (strict mode,
  duplicate ids, schema-incompatible attribute), end-to-end Allow/Deny
  via the engine, diagnostics reasons/errors, Postgres reload,
  Kafka/NATS reload/audit adapters, policy-bundle validation, and audit
  JSON byte-compatible wire-format pinning.

## Remaining follow-up

- Keep extending the local conformance corpus when new service policy
  shapes land, and run it before cedar-go upgrades.

## Cedar-go API differences vs cedar-rust

Notable shape differences picked up during the port:

| cedar-rust v4 (used by Rust impl)                                | cedar-go v1.6.0                                          |
|------------------------------------------------------------------|----------------------------------------------------------|
| `cedar_policy::PolicySet::new()`                                 | `cedar.NewPolicySet()`                                   |
| `Policy::parse(Some(id), src)` returns `Result<Policy, _>`       | `var p cedar.Policy; p.UnmarshalCedar(b []byte) error`   |
| `Authorizer::is_authorized(req, set, ents)` returns `Response`   | `policySet.IsAuthorized(entities, req)` returns `(Decision, Diagnostic)` |
| `Schema::from_cedarschema_str(src)`                              | `var s schema.Schema; s.UnmarshalCedar(b)` (in `x/exp/schema`) |
| `Validator::new(schema).validate(set, ValidationMode::Strict)`   | `validate.New(resolved, validate.WithStrict()).Policy(id, ast)` per-policy (in `x/exp/schema/validate`) |
| `Decision::Allow / Decision::Deny`                               | `cedar.Allow / cedar.Deny`                               |
| `response.diagnostics().reason()`                                | `Diagnostic.Reasons` field (typed `[]DiagnosticReason`)  |
| `response.diagnostics().errors()`                                | `Diagnostic.Errors` field (typed `[]DiagnosticError`)    |

The validator is in an experimental namespace
(`x/exp/schema/validate`) but is the same code path the cedar-go
maintainers use to run the AWS Cedar conformance suite. Pinned to
v1.6.0; bumps require running the conformance mirror.

The validator consumes `*ast.Policy` from `cedar-go/x/exp/ast`, but the
top-level `cedar.Policy.AST()` returns `*ast.Policy` from
`cedar-go/ast`. Both packages share an identical memory layout; we use
the same direct pointer cast that cedar-go's own test suite uses (see
`internal/testvalidate/testvalidate.go RunPolicyChecks`).

## Wire-compat invariants (locked)

`AuthzAuditEvent` JSON shape pinned by `audit_test.go`:

- snake_case fields (`policy_ids`, not `policyIds`).
- `tenant`, `policy_ids`, `diagnostics` use `omitempty` — they MUST be
  absent from the wire when empty (matches Rust `skip_serializing_if`).
- `decision` is the lowercase string `"allow"` or `"deny"`.

## Usage

```go
store, err := cedarauthz.NewWithPolicies([]cedarauthz.PolicyRecord{
    {ID: "permit-read", Source: `permit(principal, action == Action::"read", resource is Dataset);`},
})
if err != nil { /* handle */ }

eng := cedarauthz.NewEngine(store, cedarauthz.SlogAuditSink{})

out, err := eng.Authorize(ctx, principal, action, resource, contextRecord, entities)
if err != nil { /* handle */ }
if !out.IsAllow() {
    // policy denied — diagnostics in out.Diagnostics
}
```
