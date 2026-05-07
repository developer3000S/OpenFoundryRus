package cedarauthz_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cedarauthz "github.com/openfoundry/openfoundry-go/libs/authz-cedar-go"
)

// Wire-format pinning: Rust serializes AuthzAuditEvent with snake_case
// fields and `omitempty` on tenant / policy_ids / diagnostics. The Go
// struct must match byte-for-byte so audit-trail consumers can decode
// events from either runtime indistinguishably.
func TestAuthzAuditEventJSONShape(t *testing.T) {
	t.Parallel()
	tenant := "acme"
	ev := cedarauthz.AuthzAuditEvent{
		Timestamp:   time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
		Principal:   `User::"alice"`,
		Action:      `Action::"read"`,
		Resource:    `Dataset::"ds-1"`,
		Decision:    "allow",
		Tenant:      &tenant,
		PolicyIDs:   []string{"permit-cleared-readers"},
		Diagnostics: []string{},
	}
	out, err := json.Marshal(ev)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	for _, k := range []string{
		"timestamp", "principal", "action", "resource", "decision",
		"tenant", "policy_ids",
	} {
		assert.Contains(t, view, k)
	}
	assert.Equal(t, "allow", view["decision"])
	assert.Equal(t, "acme", view["tenant"])
	// Diagnostics empty → omitted entirely (Rust `skip_serializing_if = "Vec::is_empty"`).
	assert.NotContains(t, view, "diagnostics", "empty Diagnostics must be omitted on the wire")
}

// tenant + policy_ids omitted when zero values, matching Rust `omitempty`.
func TestAuthzAuditEventOmitsZeroFields(t *testing.T) {
	t.Parallel()
	ev := cedarauthz.AuthzAuditEvent{
		Timestamp: time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
		Principal: `User::"alice"`,
		Action:    `Action::"read"`,
		Resource:  `Dataset::"ds-1"`,
		Decision:  "deny",
	}
	out, err := json.Marshal(ev)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))
	assert.NotContains(t, view, "tenant")
	assert.NotContains(t, view, "policy_ids")
	assert.NotContains(t, view, "diagnostics")
}

func TestAuthzAuditEventJSONByteCompatibleWithRust(t *testing.T) {
	t.Parallel()
	tenant := "acme"
	ev := cedarauthz.AuthzAuditEvent{
		Timestamp:   time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC),
		Principal:   `User::"alice"`,
		Action:      `Action::"read"`,
		Resource:    `Dataset::"ds-1"`,
		Decision:    "allow",
		Tenant:      &tenant,
		PolicyIDs:   []string{"permit-cleared-readers"},
		Diagnostics: []string{"while evaluating policy `p`: missing attribute"},
	}
	out, err := json.Marshal(ev)
	require.NoError(t, err)
	const expected = "{\"timestamp\":\"2026-05-06T00:00:00Z\",\"principal\":\"User::\\\"alice\\\"\",\"action\":\"Action::\\\"read\\\"\",\"resource\":\"Dataset::\\\"ds-1\\\"\",\"decision\":\"allow\",\"tenant\":\"acme\",\"policy_ids\":[\"permit-cleared-readers\"],\"diagnostics\":[\"while evaluating policy `p`: missing attribute\"]}"
	assert.JSONEq(t, expected, string(out))
	assert.Equal(t, expected, string(out),
		"encoding/json field order is part of the pinned Rust-compatible wire fixture")
}

// NoopAuditSink is a no-op (covered to ensure the contract).
func TestNoopAuditSink(t *testing.T) {
	t.Parallel()
	cedarauthz.NoopAuditSink{}.Emit(context.Background(), cedarauthz.AuthzAuditEvent{})
}

// SlogAuditSink emits a single INFO record with the expected fields.
// We don't assert on the structured log output here (Go's slog testing
// is verbose); just confirm Emit returns without panic when Logger is
// both nil and supplied.
func TestSlogAuditSinkAcceptsNilLogger(t *testing.T) {
	t.Parallel()
	cedarauthz.SlogAuditSink{}.Emit(context.Background(), cedarauthz.AuthzAuditEvent{
		Principal: "U", Action: "A", Resource: "R", Decision: "deny",
	})
}
