//! Document-intelligence bounded context, absorbed from the legacy
//! `document-intelligence-service` crate (S8 / ADR-0030).
//!
//! Gated behind the `parsers` Cargo feature so the heavy parser
//! pipelines this module is destined to grow stay out of the default
//! CI compile path. The current files are the sketch handlers/models
//! that lived in the source crate before the merge; they reference
//! `crate::AppState` (a Postgres pool) which is not yet defined in
//! `retrieval-context-service`. Wiring `AppState` + an axum `Router`
//! is intentionally out of scope for this consolidation PR — the
//! source crate never wired them either.

#[cfg(feature = "parsers")]
pub mod handlers;

#[cfg(feature = "parsers")]
pub mod models;
