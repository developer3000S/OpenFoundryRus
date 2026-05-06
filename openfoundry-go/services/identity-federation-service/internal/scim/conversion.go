package scim

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

// UserToScim converts a UserRecord into its SCIM 2.0
// representation. Mirrors fn user_to_scim verbatim:
//
//   - Pulls a possible structured `name` out of attributes via the
//     `/scim/name` JSON pointer; falls back to a `formatted: <name>`
//     placeholder when absent.
//   - Promotes a `/scim/openfoundry` extension into the
//     openfoundry-user-extension URN; if absent but the user has
//     an organization_id, synthesises `{organizationId: <uuid>}` so
//     the wire shape always carries the org context.
//   - Emits the canonical single-email vector with primary=true,
//     type="work" using the user's email (which is the SCIM
//     userName on the wire).
//   - external_id reads off `/scim/externalId` in attributes (NOT
//     the dedicated scim_external_id column — the column is the
//     idempotency key, the attribute is the wire field). Mirrors
//     Rust verbatim.
//
// `baseURL` controls the meta.location URL (typically the service's
// public origin).
func UserToScim(record UserRecord, baseURL string) ScimUser {
	name := scimNameFromAttributes(record.Attributes)
	if name == nil {
		formatted := record.Name
		name = &ScimName{Formatted: &formatted}
	}

	extensions := map[string]json.RawMessage{}
	if v := jsonPointer(record.Attributes, "/scim/openfoundry"); v != nil {
		extensions[SchemaOpenfoundryUserExtension] = v
	} else if record.OrganizationID != nil {
		raw, _ := json.Marshal(map[string]string{"organizationId": record.OrganizationID.String()})
		extensions[SchemaOpenfoundryUserExtension] = raw
	}

	idStr := record.ID.String()
	primary := true
	emailType := "work"
	location := UserLocation(record.ID, baseURL)
	active := record.IsActive

	user := ScimUser{
		Schemas:  []string{SchemaUser},
		ID:       &idStr,
		UserName: record.Email,
		Name:     name,
		Emails: []ScimEmail{{
			Value:   record.Email,
			Primary: &primary,
			Type:    &emailType,
		}},
		Active: &active,
		Meta: &ScimMeta{
			ResourceType: "User",
			Location:     location,
		},
	}
	if v := jsonPointerString(record.Attributes, "/scim/externalId"); v != nil {
		user.ExternalID = v
	}
	if len(extensions) > 0 {
		user.Extensions = extensions
	}
	return user
}

// UserLocation builds the SCIM /Users/{id} URL for the given user
// id. Mirrors fn user_location.
func UserLocation(id uuid.UUID, baseURL string) string {
	return strings.TrimRight(baseURL, "/") + RouteUsers + "/" + id.String()
}

// GroupLocation builds the SCIM /Groups/{id} URL. Mirrors fn
// group_location.
func GroupLocation(id uuid.UUID, baseURL string) string {
	return strings.TrimRight(baseURL, "/") + RouteGroups + "/" + id.String()
}

// scimNameFromAttributes mirrors fn scim_name_from_attributes —
// pulls a ScimName off `attributes."/scim/name"` when one is
// present + parseable.
func scimNameFromAttributes(attrs json.RawMessage) *ScimName {
	v := jsonPointer(attrs, "/scim/name")
	if v == nil {
		return nil
	}
	var name ScimName
	if err := json.Unmarshal(v, &name); err != nil {
		return nil
	}
	return &name
}

// jsonPointer is a tiny RFC 6901-ish helper that walks the
// attributes blob via a slash-delimited path. Returns nil when the
// pointer doesn't resolve. Only supports the limited subset the
// SCIM port needs.
func jsonPointer(raw json.RawMessage, path string) json.RawMessage {
	if len(raw) == 0 || path == "" || path == "/" {
		return nil
	}
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	cur := raw
	for _, seg := range segments {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(cur, &obj); err != nil {
			return nil
		}
		next, ok := obj[seg]
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

// jsonPointerString returns the string at `path`, or nil when the
// pointer doesn't resolve to a string.
func jsonPointerString(raw json.RawMessage, path string) *string {
	v := jsonPointer(raw, path)
	if v == nil {
		return nil
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return nil
	}
	return &s
}
