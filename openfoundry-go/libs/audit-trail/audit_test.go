package audittrail_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	audittrail "github.com/openfoundry/openfoundry-go/libs/audit-trail"
)

func TestEventKindAndCategoriesMatchRustMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		evt     audittrail.AuditEvent
		want    audittrail.EventKind
		wantCat audittrail.AuditCategory
	}{
		{audittrail.NewMediaSetCreated("rid", "p", []string{"public"}, "n", "IMAGE", "open", false), audittrail.KindMediaSetCreated, audittrail.CategoryDataCreate},
		{audittrail.NewMediaSetDeleted("rid", "p", nil), audittrail.KindMediaSetDeleted, audittrail.CategoryDataDelete},
		{audittrail.NewMediaSetMarkingsChanged("rid", "p", []string{"a"}, []string{"b"}), audittrail.KindMediaSetMarkingsChanged, audittrail.CategoryManagementMarkings},
		{audittrail.NewMediaSetRetentionChanged("rid", "p", nil, 100, 200), audittrail.KindMediaSetRetentionChanged, audittrail.CategoryDataUpdate},
		{audittrail.NewMediaSetTransactionOpened("rid", "p", nil, "tx", "main"), audittrail.KindMediaSetTransactionOpened, audittrail.CategoryDataUpdate},
		{audittrail.NewMediaSetAccessPatternInvoked("rid", "p", nil, "thumbnail", "ephemeral"), audittrail.KindMediaSetAccessPatternInvoked, audittrail.CategoryDataLoad},
		{audittrail.NewMediaItemUploaded("itm", "ms", "p", nil, "/x", "image/png", 100, "deadbeef", ""), audittrail.KindMediaItemUploaded, audittrail.CategoryDataImport},
		{audittrail.NewMediaItemDownloaded("itm", "ms", "p", nil, 100, 60), audittrail.KindMediaItemDownloaded, audittrail.CategoryDataExport},
		{audittrail.NewVirtualMediaItemRegistered("itm", "ms", "p", nil, "s3://x", "/p"), audittrail.KindVirtualMediaItemRegistered, audittrail.CategoryDataCreate},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, c.evt.Kind)
		assert.Contains(t, c.evt.Categories(), c.wantCat)
	}
}

func TestEnvelopeJSONShape(t *testing.T) {
	t.Parallel()
	evt := audittrail.NewMediaSetCreated("ri.foundry.main.media_set.x", "ri.foundry.main.project.p",
		[]string{"public"}, "demo", "IMAGE", "open", false)
	ctx := audittrail.AuditContext{ActorID: "user-1", RequestID: "req-1"}
	env, err := audittrail.Build(evt, ctx, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	out, err := json.Marshal(env)
	require.NoError(t, err)
	var view map[string]any
	require.NoError(t, json.Unmarshal(out, &view))

	for _, k := range []string{
		"event_id", "at", "kind", "categories", "resource_rid", "project_rid",
		"markings_at_event", "actor_id", "request_id", "occurred_at", "payload",
	} {
		assert.Contains(t, view, k, "envelope must carry %q", k)
	}
	assert.Equal(t, "media_set.created", view["kind"])
	assert.Equal(t, []any{"dataCreate"}, view["categories"])
}

func TestEventIDIsDeterministic(t *testing.T) {
	t.Parallel()
	evt := audittrail.NewMediaSetCreated("rid", "p", nil, "n", "IMAGE", "open", false)
	ctx := audittrail.AuditContext{RequestID: "req-1"}
	env1, err := audittrail.Build(evt, ctx, time.Now())
	require.NoError(t, err)
	env2, err := audittrail.Build(evt, ctx, time.Now().Add(time.Hour))
	require.NoError(t, err)

	assert.Equal(t, env1.EventID, env2.EventID,
		"same (kind, resource_rid, identity_seed) MUST yield the same event_id")
}

func TestEventIDDifferentRequestIDDifferentID(t *testing.T) {
	t.Parallel()
	evt := audittrail.NewMediaSetCreated("rid", "p", nil, "n", "IMAGE", "open", false)
	id1, err := audittrail.Build(evt, audittrail.AuditContext{RequestID: "req-1"}, time.Now())
	require.NoError(t, err)
	id2, err := audittrail.Build(evt, audittrail.AuditContext{RequestID: "req-2"}, time.Now())
	require.NoError(t, err)
	assert.NotEqual(t, id1.EventID, id2.EventID)
}

// TestEventIDMatchesCrossLanguageGolden locks the v5 derivation against
// the same RFC 4122 SHA-1 namespace/name combination that the Rust
// crate uses. The expected UUIDs were computed with Python's
// uuid.uuid5 (RFC 4122 SHA-1) — the same algorithm as Rust uuid v5
// — so a regression here means Go and Rust services would compute
// different event_ids and the outbox idempotency would silently break.
func TestEventIDMatchesCrossLanguageGolden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind, rid, seed, want string
	}{
		{"media_set.created", "rid", "seed", "69a92f8b-8a2e-562e-b7e2-5011a4154773"},
		{"media_item.uploaded", "itm", "req-99", "7b0c0923-7fb3-5446-9b88-322af1e08bc9"},
	}
	for _, c := range cases {
		got := audittrail.DeriveEventID(c.kind, c.rid, c.seed).String()
		assert.Equal(t, c.want, got, "kind=%s rid=%s seed=%s", c.kind, c.rid, c.seed)
	}
}
