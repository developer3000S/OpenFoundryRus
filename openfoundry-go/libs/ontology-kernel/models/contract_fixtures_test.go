package models

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type ontologyContractFixture struct {
	ObjectType              ObjectType              `json:"object_type"`
	CreateObjectTypeRequest CreateObjectTypeRequest `json:"create_object_type_request"`
	ActionFormSection       ActionFormSection       `json:"action_form_section"`
}

func TestOntologyKernelJSONContractFixture(t *testing.T) {
	assertFixtureRoundTrip(t, "testdata/ontology_contract.json", &ontologyContractFixture{})
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
