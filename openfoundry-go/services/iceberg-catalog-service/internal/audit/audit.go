// Package audit emits structured slog events for iceberg-catalog-service.
//
// Mirrors the event vocabulary from the Rust `crate::audit` module. Each
// event carries a stable `audit_event=<name>` attribute that the Foundry
// audit collector drains into `audit-compliance-service`.
package audit

import (
	"log/slog"

	"github.com/google/uuid"
)

// NamespaceCreated logs `iceberg.namespace.created`.
func NamespaceCreated(actor uuid.UUID, projectRID, namespace string) {
	slog.Info("iceberg namespace created",
		slog.String("audit_event", "iceberg.namespace.created"),
		slog.String("actor", actor.String()),
		slog.String("project_rid", projectRID),
		slog.String("namespace", namespace),
	)
}

// NamespaceDeleted logs `iceberg.namespace.deleted`.
func NamespaceDeleted(actor uuid.UUID, projectRID, namespace string) {
	slog.Info("iceberg namespace deleted",
		slog.String("audit_event", "iceberg.namespace.deleted"),
		slog.String("actor", actor.String()),
		slog.String("project_rid", projectRID),
		slog.String("namespace", namespace),
	)
}

// TableCreated logs `iceberg.table.created`.
func TableCreated(actor uuid.UUID, tableRID, namespace, name string) {
	slog.Info("iceberg table created",
		slog.String("audit_event", "iceberg.table.created"),
		slog.String("actor", actor.String()),
		slog.String("table_rid", tableRID),
		slog.String("namespace", namespace),
		slog.String("name", name),
	)
}

// TableDropped logs `iceberg.table.dropped`.
func TableDropped(actor uuid.UUID, tableRID string, purge bool) {
	slog.Info("iceberg table dropped",
		slog.String("audit_event", "iceberg.table.dropped"),
		slog.String("actor", actor.String()),
		slog.String("table_rid", tableRID),
		slog.Bool("purge", purge),
	)
}

// TableMetadataUpdated logs `iceberg.table.metadata_updated`. `diff` is
// rendered with slog.Any so non-string payloads (maps, structs) survive
// round-trip through the JSON handler.
func TableMetadataUpdated(actor uuid.UUID, tableRID, metadataLocation string, diff any) {
	slog.Info("iceberg table metadata updated",
		slog.String("audit_event", "iceberg.table.metadata_updated"),
		slog.String("actor", actor.String()),
		slog.String("table_rid", tableRID),
		slog.String("metadata_location", metadataLocation),
		slog.Any("diff", diff),
	)
}

// OAuthTokenIssued logs an `iceberg.oauth_token.issued` event. `actor` is
// optional — for `client_credentials` grants the issued JWT identifies
// the service principal directly.
func OAuthTokenIssued(actor *uuid.UUID, grantType, scope string) {
	actorStr := ""
	if actor != nil {
		actorStr = actor.String()
	}
	slog.Info("iceberg oauth token issued",
		slog.String("audit_event", "iceberg.oauth_token.issued"),
		slog.String("actor", actorStr),
		slog.String("grant_type", grantType),
		slog.String("scope", scope),
	)
}

// APITokenCreated logs the issuance of a long-lived `ofty_*` API token.
func APITokenCreated(actor, tokenID uuid.UUID, scopes []string) {
	slog.Info("iceberg api token created",
		slog.String("audit_event", "iceberg.api_token.created"),
		slog.String("actor", actor.String()),
		slog.String("token_id", tokenID.String()),
		slog.Any("scopes", scopes),
	)
}

// TransactionBegin logs `iceberg.transaction.begin` (P2 Foundry txn).
func TransactionBegin(actor uuid.UUID, buildRID string) {
	slog.Info("foundry iceberg transaction begin",
		slog.String("audit_event", "iceberg.transaction.begin"),
		slog.String("actor", actor.String()),
		slog.String("build_rid", buildRID),
	)
}

// TransactionCommit logs `iceberg.transaction.commit`.
func TransactionCommit(actor uuid.UUID, buildRID string, tableCount int) {
	slog.Info("foundry iceberg transaction commit",
		slog.String("audit_event", "iceberg.transaction.commit"),
		slog.String("actor", actor.String()),
		slog.String("build_rid", buildRID),
		slog.Int("table_count", tableCount),
	)
}

// TransactionAbort logs `iceberg.transaction.abort`.
func TransactionAbort(actor uuid.UUID, buildRID, reason string) {
	slog.Info("foundry iceberg transaction abort",
		slog.String("audit_event", "iceberg.transaction.abort"),
		slog.String("actor", actor.String()),
		slog.String("build_rid", buildRID),
		slog.String("reason", reason),
	)
}

// TransactionConflict logs `iceberg.transaction.conflict`.
func TransactionConflict(actor uuid.UUID, buildRID, tableRID, conflictingWith string) {
	slog.Info("foundry iceberg transaction conflict",
		slog.String("audit_event", "iceberg.transaction.conflict"),
		slog.String("actor", actor.String()),
		slog.String("build_rid", buildRID),
		slog.String("table_rid", tableRID),
		slog.String("conflicting_with", conflictingWith),
	)
}

// TransactionRetry logs `iceberg.transaction.retry`.
func TransactionRetry(actor uuid.UUID, buildRID, tableRID string) {
	slog.Info("foundry iceberg transaction retry signal",
		slog.String("audit_event", "iceberg.transaction.retry"),
		slog.String("actor", actor.String()),
		slog.String("build_rid", buildRID),
		slog.String("table_rid", tableRID),
	)
}

// SchemaAltered logs `iceberg.schema.altered`.
func SchemaAltered(actor uuid.UUID, tableRID string, previousSchemaID, newSchemaID int64) {
	slog.Info("iceberg schema explicitly altered",
		slog.String("audit_event", "iceberg.schema.altered"),
		slog.String("actor", actor.String()),
		slog.String("table_rid", tableRID),
		slog.Int64("previous_schema_id", previousSchemaID),
		slog.Int64("new_schema_id", newSchemaID),
	)
}

// SchemaAttemptBlocked logs `iceberg.schema.attempt_blocked_by_strict_mode`.
func SchemaAttemptBlocked(actor uuid.UUID, tableRID, diff string) {
	slog.Warn("iceberg schema mutation blocked by strict-mode (call alter-schema first)",
		slog.String("audit_event", "iceberg.schema.attempt_blocked_by_strict_mode"),
		slog.String("actor", actor.String()),
		slog.String("table_rid", tableRID),
		slog.String("diff", diff),
	)
}

// BranchAliasApplied logs `iceberg.branch_alias.applied`. `actor` is
// optional — anonymous PyIceberg reads still record the rewrite.
func BranchAliasApplied(actor *uuid.UUID, requested, resolved string) {
	actorStr := ""
	if actor != nil {
		actorStr = actor.String()
	}
	slog.Info("iceberg branch alias applied",
		slog.String("audit_event", "iceberg.branch_alias.applied"),
		slog.String("actor", actorStr),
		slog.String("requested", requested),
		slog.String("resolved", resolved),
	)
}

// MarkingsUpdated logs a transition of effective markings on a namespace
// or a table. `scope` is "namespace" or "table"; `before`/`after` are the
// effective marking-name lists.
func MarkingsUpdated(actor uuid.UUID, targetRID, scope string, before, after []string) {
	slog.Info("iceberg markings updated",
		slog.String("audit_event", "iceberg.markings.updated"),
		slog.String("actor", actor.String()),
		slog.String("target_rid", targetRID),
		slog.String("scope", scope),
		slog.Any("before", before),
		slog.Any("after", after),
	)
}

// MarkingsOverrideCreated fires once per newly-introduced *explicit*
// marking on a table (i.e. one not present in the inherited set).
func MarkingsOverrideCreated(actor uuid.UUID, tableRID, marking string) {
	slog.Info("iceberg markings override created",
		slog.String("audit_event", "iceberg.markings.override_created"),
		slog.String("actor", actor.String()),
		slog.String("table_rid", tableRID),
		slog.String("marking", marking),
	)
}

// MarkingsInheritanceSnapshot logs `iceberg.markings.inheritance_snapshot`.
func MarkingsInheritanceSnapshot(actor uuid.UUID, tableRID, namespaceRID string, markings []string) {
	slog.Info("iceberg markings inherited at table creation",
		slog.String("audit_event", "iceberg.markings.inheritance_snapshot"),
		slog.String("actor", actor.String()),
		slog.String("table_rid", tableRID),
		slog.String("namespace_rid", namespaceRID),
		slog.Any("markings", markings),
	)
}

// AccessDenied logs a 403 emitted by the authz engine. Reason mirrors
// `authz.DenialReason.String()` so dashboards can split by cause.
func AccessDenied(actor uuid.UUID, targetRID, attemptedAction, reason string) {
	slog.Warn("iceberg access denied",
		slog.String("audit_event", "iceberg.access.denied"),
		slog.String("actor", actor.String()),
		slog.String("target_rid", targetRID),
		slog.String("attempted_action", attemptedAction),
		slog.String("reason", reason),
	)
}

// DiagnoseExecuted logs `iceberg.diagnose.executed`.
func DiagnoseExecuted(actor uuid.UUID, clientKind string, latencyMS int64, success bool) {
	slog.Info("iceberg diagnose executed",
		slog.String("audit_event", "iceberg.diagnose.executed"),
		slog.String("actor", actor.String()),
		slog.String("client_kind", clientKind),
		slog.Int64("latency_ms", latencyMS),
		slog.Bool("success", success),
	)
}
