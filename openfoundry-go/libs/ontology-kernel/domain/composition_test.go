package domain

import (
	"testing"

	"github.com/google/uuid"

	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// StableLinkID must be deterministic across processes — Rust uses
// `Uuid::new_v5(NAMESPACE_OID, material)` and Go's `uuid.NewSHA1`
// over the same namespace must produce the same UUID.
func TestStableLinkIDIsDeterministic(t *testing.T) {
	t.Parallel()
	a := StableLinkID(storage.LinkTypeId("lt-1"), storage.ObjectId("a"), storage.ObjectId("b"))
	b := StableLinkID(storage.LinkTypeId("lt-1"), storage.ObjectId("a"), storage.ObjectId("b"))
	if a != b {
		t.Fatalf("expected identical UUIDs, got %s vs %s", a, b)
	}
}

func TestStableLinkIDDiffersOnInputs(t *testing.T) {
	t.Parallel()
	a := StableLinkID("lt-1", "a", "b")
	b := StableLinkID("lt-1", "a", "c")
	c := StableLinkID("lt-2", "a", "b")
	if a == b || a == c || b == c {
		t.Fatalf("expected distinct UUIDs, got %s %s %s", a, b, c)
	}
}

// Pin: the Rust impl is `Uuid::new_v5(NAMESPACE_OID, "openfoundry/ontology-link/<lt>/<from>/<to>")`.
// We pin the exact output for one fixture so a future refactor that
// silently switches namespaces or material concatenation would be
// caught here.
func TestStableLinkIDFixedFixture(t *testing.T) {
	t.Parallel()
	got := StableLinkID(storage.LinkTypeId("manages"), storage.ObjectId("u1"), storage.ObjectId("u2"))
	// Material: "openfoundry/ontology-link/manages/u1/u2"
	// uuidv5(NAMESPACE_OID, material) — must match the Rust output exactly.
	want := uuid.NewSHA1(uuid.NameSpaceOID, []byte("openfoundry/ontology-link/manages/u1/u2"))
	if got != want {
		t.Fatalf("mismatch with Rust v5 derivation: got %s, want %s", got, want)
	}
}
