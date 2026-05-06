package aievents

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopicPinned(t *testing.T) {
	t.Parallel()
	if Topic != "ai.events.v1" {
		t.Fatalf("Topic must remain ai.events.v1 (Kafka ACL wire-compat); got %q", Topic)
	}
}

func TestTxnIDPrefixPinned(t *testing.T) {
	t.Parallel()
	if TxnIDPrefix != "agent-runtime-" {
		t.Fatalf("TxnIDPrefix mismatch: %q", TxnIDPrefix)
	}
}

func TestTargetTablesMatchNamespaceLayout(t *testing.T) {
	t.Parallel()
	cases := map[AiEventKind]string{
		KindPrompt:     "prompts",
		KindResponse:   "responses",
		KindEvaluation: "evaluations",
		KindTrace:      "traces",
	}
	for k, want := range cases {
		if got := k.TargetTable(); got != want {
			t.Fatalf("%s.TargetTable() = %q want %q", k, got, want)
		}
	}
}

func TestAiEventKindLowercaseJSON(t *testing.T) {
	t.Parallel()
	// Rust serde rename_all="lowercase" — the JSON form is lowercase.
	for _, k := range []AiEventKind{KindPrompt, KindResponse, KindEvaluation, KindTrace} {
		b, err := json.Marshal(k)
		require.NoError(t, err)
		assert.Equal(t, `"`+string(k)+`"`, string(b))
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	t.Parallel()
	env := AiEventEnvelope{
		EventID:       uuid.Nil,
		At:            1_700_000_000_000_000,
		Kind:          KindPrompt,
		Producer:      "agent-runtime-service",
		SchemaVersion: 1,
		Payload:       json.RawMessage(`{"text":"hello"}`),
	}
	bytes, err := json.Marshal(env)
	require.NoError(t, err)
	var back AiEventEnvelope
	require.NoError(t, json.Unmarshal(bytes, &back))
	assert.Equal(t, KindPrompt, back.Kind)
	assert.Equal(t, uint32(1), back.SchemaVersion)
	assert.Equal(t, "agent-runtime-service", back.Producer)
}
