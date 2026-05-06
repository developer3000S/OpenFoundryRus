// Package-level access checks used by every kernel handler that
// returns object instances. Mirrors `libs/ontology-kernel/src/domain/access.rs`
// 1:1: same marking vocabulary, same clearance ranks, same admin
// short-circuit, same per-organization rejection.
package domain

import (
	"encoding/json"
	"errors"
	"fmt"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
)

// ValidMarkings mirrors `pub const VALID_MARKINGS`.
var ValidMarkings = []string{"public", "confidential", "pii"}

// ValidateMarking mirrors `pub fn validate_marking`. The error format
// matches Rust's `Debug` rendering of `&[&str]` (i.e.
// `["public", "confidential", "pii"]`) so consumers that match on the
// message body see the same byte sequence the Rust handler emits.
func ValidateMarking(marking string) error {
	for _, v := range ValidMarkings {
		if v == marking {
			return nil
		}
	}
	return fmt.Errorf("invalid marking '%s', valid markings: %s", marking, rustSliceDebug(ValidMarkings))
}

// ErrForbiddenOrg / ErrForbiddenMarking / ErrForbiddenClearance map
// onto the three Rust error strings the handler-side code matches on.
var (
	ErrForbiddenOrg        = errors.New("forbidden: object belongs to a different organization")
	ErrForbiddenClearance  = errors.New("forbidden: insufficient classification clearance")
)

// EnsureObjectAccess mirrors `pub fn ensure_object_access`.
//
// Decision cascade:
//  1. Subject carrying the `admin` role → always allowed.
//  2. Subject's `org_id` mismatching the object's `organization_id`
//     → ErrForbiddenOrg.
//  3. Object marking unknown → typed marking error.
//  4. Subject's classification clearance < object marking rank
//     → ErrForbiddenClearance.
func EnsureObjectAccess(claims *authmw.Claims, object *ObjectInstance) error {
	if claims.HasRole("admin") {
		return nil
	}

	if claims.OrgID != nil && object.OrganizationID != nil {
		if *claims.OrgID != *object.OrganizationID {
			return ErrForbiddenOrg
		}
	}

	required, ok := MarkingRank(object.Marking)
	if !ok {
		return fmt.Errorf("forbidden: unsupported object marking '%s'", object.Marking)
	}
	granted := ClearanceRank(claims)
	if granted < required {
		return ErrForbiddenClearance
	}
	return nil
}

// ClearanceRank mirrors `pub fn clearance_rank`. Reads
// `claims.attributes["classification_clearance"]` as a string, maps
// to a marking rank, defaults to 0 (public) on any failure path.
func ClearanceRank(claims *authmw.Claims) uint8 {
	if claims == nil || len(claims.Attributes) == 0 {
		return 0
	}
	var attrs map[string]any
	if err := json.Unmarshal(claims.Attributes, &attrs); err != nil {
		return 0
	}
	v, ok := attrs["classification_clearance"].(string)
	if !ok {
		return 0
	}
	rank, _ := MarkingRank(v)
	return rank
}

// MarkingRank mirrors `pub fn marking_rank`.
func MarkingRank(marking string) (uint8, bool) {
	switch marking {
	case "public":
		return 0, true
	case "confidential":
		return 1, true
	case "pii":
		return 2, true
	default:
		return 0, false
	}
}
