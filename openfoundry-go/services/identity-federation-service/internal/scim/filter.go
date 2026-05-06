package scim

import (
	"fmt"
	"strings"
)

// FilterAttribute discriminates the supported SCIM filter LHS
// operands. Mirrors enum ScimFilter (UserName / DisplayName /
// ExternalId).
type FilterAttribute uint8

const (
	// FilterUserName matches `userName eq "..."`. Used by the
	// User endpoints.
	FilterUserName FilterAttribute = iota + 1
	// FilterDisplayName matches `displayName eq "..."`. Used by
	// the Group endpoints.
	FilterDisplayName
	// FilterExternalID matches `externalId eq "..."`. Available
	// on both User and Group surfaces.
	FilterExternalID
)

// EqFilter is the parsed result of an `<attr> eq "<value>"`
// SCIM filter expression. The Attribute field tells the caller
// which column to push the predicate down to.
type EqFilter struct {
	Attribute FilterAttribute
	Value     string
}

// ParseEqFilter mirrors fn parse_eq_filter. Accepts a possibly-nil
// filter string and the list of LHS attributes the caller's
// surface is willing to honour. Returns:
//
//   - (nil, nil) when the filter is absent or whitespace-only;
//   - (*EqFilter, nil) when parsing succeeds;
//   - (nil, ScimError) when the filter is malformed or names an
//     unsupported attribute. The error carries scimType=
//     "invalidFilter" so callers can wrap it as a 400 response.
func ParseEqFilter(filter *string, supportedAttrs []string) (*EqFilter, *ScimError) {
	if filter == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*filter)
	if trimmed == "" {
		return nil, nil
	}
	lower := strings.ToLower(trimmed)
	var matchedAttr string
	for _, attr := range supportedAttrs {
		prefix := strings.ToLower(attr) + " eq "
		if strings.HasPrefix(lower, prefix) {
			matchedAttr = attr
			break
		}
	}
	if matchedAttr == "" {
		invalid := "invalidFilter"
		err := NewScimError(400, "unsupported SCIM filter: "+trimmed, &invalid)
		return nil, &err
	}

	prefixLen := len(matchedAttr) + len(" eq ")
	value := strings.TrimSpace(trimmed[prefixLen:])
	if len(value) < 2 || !strings.HasPrefix(value, `"`) || !strings.HasSuffix(value, `"`) {
		invalid := "invalidFilter"
		err := NewScimError(400, "SCIM filter value must be quoted", &invalid)
		return nil, &err
	}
	value = value[1 : len(value)-1]

	switch matchedAttr {
	case "userName":
		return &EqFilter{Attribute: FilterUserName, Value: value}, nil
	case "displayName":
		return &EqFilter{Attribute: FilterDisplayName, Value: value}, nil
	case "externalId":
		return &EqFilter{Attribute: FilterExternalID, Value: value}, nil
	default:
		invalid := "invalidFilter"
		err := NewScimError(400, fmt.Sprintf("unsupported SCIM filter: %s", trimmed), &invalid)
		return nil, &err
	}
}
