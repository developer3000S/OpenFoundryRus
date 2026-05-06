package domain

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
)

func TestValidateMarkingAcceptsKnownLabels(t *testing.T) {
	t.Parallel()
	for _, m := range ValidMarkings {
		if err := ValidateMarking(m); err != nil {
			t.Errorf("expected %q to be valid: %v", m, err)
		}
	}
}

func TestValidateMarkingRejectsOthers(t *testing.T) {
	t.Parallel()
	err := ValidateMarking("top-secret")
	if err == nil {
		t.Fatal("top-secret should be rejected")
	}
	// Pin the Rust Debug-format rendering of the slice so consumers
	// that match on the message body see the same byte sequence.
	want := `invalid marking 'top-secret', valid markings: ["public", "confidential", "pii"]`
	if err.Error() != want {
		t.Fatalf("error format drift: got %q, want %q", err.Error(), want)
	}
}

func TestEnsureObjectAccessAdminBypass(t *testing.T) {
	t.Parallel()
	c := &authmw.Claims{Roles: []string{"admin"}}
	obj := &ObjectInstance{Marking: "pii"}
	if err := EnsureObjectAccess(c, obj); err != nil {
		t.Fatalf("admin must bypass: %v", err)
	}
}

func TestEnsureObjectAccessRejectsCrossOrg(t *testing.T) {
	t.Parallel()
	subjectOrg, objectOrg := uuid.New(), uuid.New()
	c := &authmw.Claims{Roles: []string{"member"}, OrgID: &subjectOrg}
	obj := &ObjectInstance{
		OrganizationID: &objectOrg,
		Marking:        "public",
	}
	err := EnsureObjectAccess(c, obj)
	if !errors.Is(err, ErrForbiddenOrg) {
		t.Fatalf("expected ErrForbiddenOrg, got %v", err)
	}
}

func TestEnsureObjectAccessClearanceRanksMatchRust(t *testing.T) {
	t.Parallel()
	cases := []struct {
		clearance string
		marking   string
		want      bool
	}{
		{"", "public", true},
		{"public", "public", true},
		{"public", "confidential", false},
		{"confidential", "confidential", true},
		{"confidential", "pii", false},
		{"pii", "pii", true},
	}
	for _, tc := range cases {
		attrs, _ := json.Marshal(map[string]string{"classification_clearance": tc.clearance})
		c := &authmw.Claims{
			Roles:      []string{"member"},
			Attributes: attrs,
		}
		err := EnsureObjectAccess(c, &ObjectInstance{Marking: tc.marking})
		got := err == nil
		if got != tc.want {
			t.Errorf("clearance=%s marking=%s: got allowed=%v, want %v (err=%v)",
				tc.clearance, tc.marking, got, tc.want, err)
		}
	}
}

func TestEnsureObjectAccessRejectsUnknownMarking(t *testing.T) {
	t.Parallel()
	c := &authmw.Claims{Roles: []string{"member"}}
	err := EnsureObjectAccess(c, &ObjectInstance{Marking: "purple"})
	if err == nil {
		t.Fatal("unknown marking must fail")
	}
}

func TestMarkingRankRoundTripsRust(t *testing.T) {
	t.Parallel()
	cases := map[string]uint8{"public": 0, "confidential": 1, "pii": 2}
	for marking, want := range cases {
		got, ok := MarkingRank(marking)
		if !ok || got != want {
			t.Errorf("MarkingRank(%s) = (%d,%v), want (%d,true)", marking, got, ok, want)
		}
	}
	if _, ok := MarkingRank("unknown"); ok {
		t.Fatal("unknown marking must return ok=false")
	}
}
