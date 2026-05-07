package runtime

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type transformContractFixture struct {
	Input  TransformInput  `json:"input"`
	Output TransformOutput `json:"output"`
}

func TestTransformJSONContractFixture(t *testing.T) {
	assertFixtureRoundTrip(t, "testdata/transform_contract.json", &transformContractFixture{})
}

func assertFixtureRoundTrip(t *testing.T, path string, dst any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	encoded, err := json.Marshal(dst)
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
