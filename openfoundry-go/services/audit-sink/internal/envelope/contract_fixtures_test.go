package envelope

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestAuditEnvelopeJSONContractFixture(t *testing.T) {
	raw, err := os.ReadFile("testdata/audit_envelope.json")
	if err != nil {
		t.Fatal(err)
	}
	env, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	encoded, err := json.Marshal(&env)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	assertSameJSON(t, raw, encoded)
}
func assertSameJSON(t *testing.T, wantRaw, gotRaw []byte) {
	t.Helper()
	var want, got any
	if err := json.Unmarshal(wantRaw, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(gotRaw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("JSON mismatch\nwant: %s\n got: %s", wantRaw, gotRaw)
	}
}
