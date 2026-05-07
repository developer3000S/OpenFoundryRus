package scan

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPageSizeBounds(t *testing.T) {
	t.Parallel()
	assert.Equal(t, int32(1), MinPageSize)
	assert.Equal(t, int32(10000), MaxPageSize)
	assert.Equal(t, int32(1000), DefaultPageSize)
}

func TestDecodeRequestDefaultsAndClamps(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want DecodedRequest
	}{
		{"defaults", `{"tenant_id":"t-a"}`, DecodedRequest{TenantID: "t-a", PageSize: DefaultPageSize}},
		{"empty type collapses", `{"tenant_id":"t-a","type_id":""}`, DecodedRequest{TenantID: "t-a", PageSize: DefaultPageSize}},
		{"type with spaces trimmed", `{"tenant_id":"t-a","type_id":"  users "}`, DecodedRequest{TenantID: "t-a", TypeID: ptr("users"), PageSize: DefaultPageSize}},
		{"page_size 0 → default", `{"tenant_id":"t-a","page_size":0}`, DecodedRequest{TenantID: "t-a", PageSize: DefaultPageSize}},
		{"page_size clamped high", `{"tenant_id":"t-a","page_size":99999}`, DecodedRequest{TenantID: "t-a", PageSize: MaxPageSize}},
		{"page_size respected", `{"tenant_id":"t-a","page_size":250}`, DecodedRequest{TenantID: "t-a", PageSize: 250}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := DecodeRequest([]byte(c.in))
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestDecodeRequestRejectsEmptyTenant(t *testing.T) {
	t.Parallel()
	_, err := DecodeRequest([]byte(`{"tenant_id":""}`))
	require.Error(t, err)
	var de *DecodeError
	require.True(t, errors.As(err, &de))
	assert.Equal(t, "missing", de.Kind)
	assert.Equal(t, "tenant_id", de.Field)
}

func TestDecodeRequestRejectsNegativePageSize(t *testing.T) {
	t.Parallel()
	_, err := DecodeRequest([]byte(`{"tenant_id":"t","page_size":-1}`))
	require.Error(t, err)
	var de *DecodeError
	require.True(t, errors.As(err, &de))
	assert.Equal(t, "invalid", de.Kind)
	assert.Equal(t, "page_size", de.Field)
}

func TestReindexRecordPartitionKey(t *testing.T) {
	t.Parallel()
	r := ReindexRecord{Tenant: "tenant-a", ID: "obj-1"}
	assert.Equal(t, "tenant-a/obj-1", r.PartitionKey(),
		"partition-key composition must match the legacy Go worker for indexer hash co-location")
}

func TestEncodeBatchRecordExtractsEmbedding(t *testing.T) {
	t.Parallel()
	props := json.RawMessage(`{"name":"alice","embedding":[0.1,0.2,0.3]}`)
	r := EncodeBatchRecord("t-a", "obj-1", "user", 7, props)
	assert.Equal(t, "t-a", r.Tenant)
	assert.Equal(t, "obj-1", r.ID)
	assert.Equal(t, "user", r.TypeID)
	assert.Equal(t, int64(7), r.Version)
	assert.Equal(t, []float64{0.1, 0.2, 0.3}, r.Embedding)
	assert.False(t, r.Deleted, "deleted always false on publish path")
}

func TestEncodeBatchRecordOmitsEmbeddingWhenAbsent(t *testing.T) {
	t.Parallel()
	props := json.RawMessage(`{"name":"alice"}`)
	r := EncodeBatchRecord("t-a", "obj-1", "user", 1, props)
	assert.Nil(t, r.Embedding)

	b, err := json.Marshal(r)
	require.NoError(t, err)
	assert.NotContains(t, string(b), "embedding", "omitempty must elide nil embedding from wire")
}

// Mirrors the Rust `record_omits_embedding_when_absent_or_empty`
// test (the empty-array branch): `{"embedding":[]}` MUST collapse
// to nil so the `omitempty` json tag elides it from the wire.
func TestEncodeBatchRecordOmitsEmbeddingWhenEmpty(t *testing.T) {
	t.Parallel()
	r := EncodeBatchRecord("t", "id", "u", 1, json.RawMessage(`{"embedding":[]}`))
	assert.Nil(t, r.Embedding)
}

// Rust serde's `as_f64` accepts JSON integers as floats, so the Go
// port must too — the two implementations must agree on what counts
// as a numeric entry.
func TestEncodeBatchRecordAcceptsIntegerEmbedding(t *testing.T) {
	t.Parallel()
	r := EncodeBatchRecord("t", "id", "u", 1, json.RawMessage(`{"embedding":[1,2,3]}`))
	assert.Equal(t, []float64{1, 2, 3}, r.Embedding)
}

// Mirrors Rust's lenient `extract_embedding`: a heterogeneous array
// keeps the numeric entries and drops the rest, instead of failing
// the whole record. Strict `json.Unmarshal` of `[]float64` would
// fail on the first non-numeric entry; the Go port iterates and
// skips, matching the Rust loop.
func TestEncodeBatchRecordSkipsNonNumericEmbeddingEntries(t *testing.T) {
	t.Parallel()
	r := EncodeBatchRecord("t", "id", "u", 1,
		json.RawMessage(`{"embedding":[0.1,"oops",null,0.3]}`))
	assert.Equal(t, []float64{0.1, 0.3}, r.Embedding)
}

// Mirrors the Rust
// `record_round_trip_json_shape_matches_object_changed_v1` test:
// the wire shape MUST stay aligned with services/ontology-indexer
// `ObjectChangedV1` so the indexer keeps decoding both topics with
// the same code path. In particular `deleted` is always present on
// the wire — Rust serde emits it unconditionally, so Go must too.
func TestReindexRecordWireShapeMatchesObjectChangedV1(t *testing.T) {
	t.Parallel()
	r := EncodeBatchRecord(
		"tenant-a",
		"00000000-0000-0000-0000-000000000001",
		"users",
		7,
		json.RawMessage(`{"name":"alice"}`),
	)
	b, err := json.Marshal(r)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	assert.Equal(t, "tenant-a", got["tenant"])
	assert.Equal(t, "00000000-0000-0000-0000-000000000001", got["id"])
	assert.Equal(t, "users", got["type_id"])
	assert.Equal(t, float64(7), got["version"])
	assert.Equal(t, false, got["deleted"], "deleted must be present on wire (no omitempty)")
	payload, ok := got["payload"].(map[string]any)
	require.True(t, ok, "payload must round-trip as a JSON object")
	assert.Equal(t, "alice", payload["name"])
	_, hasEmbedding := got["embedding"]
	assert.False(t, hasEmbedding, "absent embedding must not appear on wire")
}

// Mirrors the Rust `record_partition_key_matches_legacy_format`
// test using a real UUID-shaped id, so a regression that breaks the
// `tenant/id` composition surfaces under the same assertion the
// Rust suite uses.
func TestReindexRecordPartitionKeyMatchesLegacyFormat(t *testing.T) {
	t.Parallel()
	r := EncodeBatchRecord(
		"tenant-a",
		"00000000-0000-0000-0000-000000000001",
		"users",
		7,
		json.RawMessage(`{}`),
	)
	assert.Equal(t,
		"tenant-a/00000000-0000-0000-0000-000000000001",
		r.PartitionKey())
}

func ptr[T any](v T) *T { return &v }
