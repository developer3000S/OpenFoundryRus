// Package scim ports services/identity-federation-service/src/
// hardening/scim.rs + handlers/scim.rs verbatim. Mirrors the
// SCIM 2.0 (RFC 7643/7644) provisioning contract used by the
// identity-federation-service /scim/v2/* surface.
//
// This file (types.go) ports hardening/scim.rs in full: route
// constants, schema URNs, every wire-payload struct (ScimUser,
// ScimGroup, ScimMeta, ScimEmail, ScimName, ScimGroupMember,
// ScimListResponse, ScimPatchRequest, ScimPatchOperation,
// ScimError, ServiceProviderConfig, Feature, FilterFeature,
// AuthenticationScheme, ScimSchemaResource, ScimSchemaAttribute,
// ScimResourceType), and the metadata builders
// (ServiceProviderConfigPayload / SchemaResources /
// ResourceTypes).
//
// Slice ledger:
//   - 3.7b.3.1 — types + metadata builders + discovery endpoints
//     (this) + GetUser + ListUsers + filter parser + in-mem store.
//   - 3.7b.3.2 (next) — CreateUser + PatchUser + DeleteUser.
//   - 3.7b.3.3 — Group endpoints.
//   - 3.7b.3.4 — PostgresScimStore.
package scim

import "encoding/json"

// ─── Route constants ────────────────────────────────────────────────

const (
	RouteUsers                  = "/scim/v2/Users"
	RouteUserByID               = "/scim/v2/Users/{id}"
	RouteGroups                 = "/scim/v2/Groups"
	RouteGroupByID              = "/scim/v2/Groups/{id}"
	RouteServiceProviderConfig  = "/scim/v2/ServiceProviderConfig"
	RouteSchemas                = "/scim/v2/Schemas"
	RouteSchemaByID             = "/scim/v2/Schemas/{id}"
	RouteResourceTypes          = "/scim/v2/ResourceTypes"
	RouteResourceTypeByID       = "/scim/v2/ResourceTypes/{id}"
)

// ─── Schema URNs ────────────────────────────────────────────────────

const (
	SchemaUser                       = "urn:ietf:params:scim:schemas:core:2.0:User"
	SchemaGroup                      = "urn:ietf:params:scim:schemas:core:2.0:Group"
	SchemaListResponse               = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	SchemaPatchOp                    = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	SchemaError                      = "urn:ietf:params:scim:api:messages:2.0:Error"
	SchemaServiceProviderConfig      = "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"
	SchemaSchema                     = "urn:ietf:params:scim:schemas:core:2.0:Schema"
	SchemaResourceType               = "urn:ietf:params:scim:schemas:core:2.0:ResourceType"
	SchemaOpenfoundryUserExtension   = "urn:openfoundry:params:scim:schemas:extension:2.0:User"
)

// ─── Wire payloads ──────────────────────────────────────────────────

// ScimMeta is the SCIM `meta` sub-resource carried on every
// resource representation.
type ScimMeta struct {
	ResourceType string `json:"resourceType"`
	Location     string `json:"location"`
}

// ScimUser is the SCIM 2.0 User resource. Mirrors struct ScimUser.
//
// The `Extensions` field is a flat extension catch-all that
// captures top-level keys outside the canonical SCIM User fields
// (e.g. the openfoundry user-extension URN). It serialises with
// json.RawMessage values so the wire form preserves the caller's
// nested structure verbatim.
type ScimUser struct {
	Schemas    []string                   `json:"schemas"`
	ID         *string                    `json:"id,omitempty"`
	UserName   string                     `json:"userName"`
	Name       *ScimName                  `json:"name,omitempty"`
	Emails     []ScimEmail                `json:"emails,omitempty"`
	Active     *bool                      `json:"active,omitempty"`
	ExternalID *string                    `json:"externalId,omitempty"`
	Meta       *ScimMeta                  `json:"meta,omitempty"`
	Extensions map[string]json.RawMessage `json:"-"`
}

// MarshalJSON splices Extensions into the top-level object —
// equivalent to Rust's #[serde(flatten)] on the extensions map.
func (u ScimUser) MarshalJSON() ([]byte, error) {
	type alias ScimUser
	base, err := json.Marshal(alias(u))
	if err != nil {
		return nil, err
	}
	if len(u.Extensions) == 0 {
		return base, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(base, &obj); err != nil {
		return nil, err
	}
	for k, v := range u.Extensions {
		// Don't let extensions overwrite the canonical fields.
		if _, taken := obj[k]; !taken {
			obj[k] = v
		}
	}
	return json.Marshal(obj)
}

// UnmarshalJSON carries unrecognised top-level keys into Extensions.
func (u *ScimUser) UnmarshalJSON(data []byte) error {
	type alias ScimUser
	var base alias
	if err := json.Unmarshal(data, &base); err != nil {
		return err
	}
	*u = ScimUser(base)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, known := range scimUserKnownFields {
		delete(raw, known)
	}
	if len(raw) > 0 {
		u.Extensions = raw
	}
	return nil
}

var scimUserKnownFields = []string{
	"schemas", "id", "userName", "name", "emails", "active", "externalId", "meta",
}

// ScimName mirrors struct ScimName.
type ScimName struct {
	GivenName  *string `json:"givenName,omitempty"`
	FamilyName *string `json:"familyName,omitempty"`
	Formatted  *string `json:"formatted,omitempty"`
}

// ScimEmail mirrors struct ScimEmail.
type ScimEmail struct {
	Value   string  `json:"value"`
	Primary *bool   `json:"primary,omitempty"`
	Type    *string `json:"type,omitempty"`
}

// ScimGroup is the SCIM 2.0 Group resource.
type ScimGroup struct {
	Schemas     []string                   `json:"schemas"`
	ID          *string                    `json:"id,omitempty"`
	DisplayName string                     `json:"displayName"`
	Members     []ScimGroupMember          `json:"members,omitempty"`
	ExternalID  *string                    `json:"externalId,omitempty"`
	Meta        *ScimMeta                  `json:"meta,omitempty"`
	Extensions  map[string]json.RawMessage `json:"-"`
}

// MarshalJSON / UnmarshalJSON mirror the same flatten-extensions
// trick used on ScimUser.
func (g ScimGroup) MarshalJSON() ([]byte, error) {
	type alias ScimGroup
	base, err := json.Marshal(alias(g))
	if err != nil {
		return nil, err
	}
	if len(g.Extensions) == 0 {
		return base, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(base, &obj); err != nil {
		return nil, err
	}
	for k, v := range g.Extensions {
		if _, taken := obj[k]; !taken {
			obj[k] = v
		}
	}
	return json.Marshal(obj)
}

func (g *ScimGroup) UnmarshalJSON(data []byte) error {
	type alias ScimGroup
	var base alias
	if err := json.Unmarshal(data, &base); err != nil {
		return err
	}
	*g = ScimGroup(base)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for _, known := range scimGroupKnownFields {
		delete(raw, known)
	}
	if len(raw) > 0 {
		g.Extensions = raw
	}
	return nil
}

var scimGroupKnownFields = []string{
	"schemas", "id", "displayName", "members", "externalId", "meta",
}

// ScimGroupMember mirrors struct ScimGroupMember. The `$ref` JSON
// field name maps to Ref to keep the wire-compat invariant.
type ScimGroupMember struct {
	Value   string  `json:"value"`
	Ref     *string `json:"$ref,omitempty"`
	Type    *string `json:"type,omitempty"`
	Display *string `json:"display,omitempty"`
}

// ScimListResponse[T] is the paged-list envelope used by every
// list endpoint. ItemsPerPage is set automatically by NewScimList
// from len(Resources).
type ScimListResponse[T any] struct {
	Schemas      []string `json:"schemas"`
	TotalResults int      `json:"totalResults"`
	StartIndex   int      `json:"startIndex"`
	ItemsPerPage int      `json:"itemsPerPage"`
	Resources    []T      `json:"Resources"`
}

// NewScimList builds the canonical ListResponse for `resources`
// with the given total + start index.
func NewScimList[T any](resources []T, totalResults, startIndex int) ScimListResponse[T] {
	return ScimListResponse[T]{
		Schemas:      []string{SchemaListResponse},
		TotalResults: totalResults,
		StartIndex:   startIndex,
		ItemsPerPage: len(resources),
		Resources:    resources,
	}
}

// ScimPatchRequest mirrors struct ScimPatchRequest.
type ScimPatchRequest struct {
	Schemas    []string             `json:"schemas"`
	Operations []ScimPatchOperation `json:"Operations"`
}

// ScimPatchOperation mirrors struct ScimPatchOperation.
type ScimPatchOperation struct {
	Op    string          `json:"op"`
	Path  *string         `json:"path,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

// ─── ScimError ──────────────────────────────────────────────────────

// ScimError is the wire-shape of a SCIM error envelope. Use
// NewScimError or NewUnsupportedError to construct.
type ScimError struct {
	Schemas  []string `json:"schemas"`
	Status   string   `json:"status"`
	Detail   string   `json:"detail"`
	ScimType *string  `json:"scimType,omitempty"`
}

// NewScimError mirrors fn ScimError::new.
func NewScimError(status int, detail string, scimType *string) ScimError {
	return ScimError{
		Schemas:  []string{SchemaError},
		Status:   itoaStatus(status),
		Detail:   detail,
		ScimType: scimType,
	}
}

// NewUnsupportedError mirrors fn ScimError::unsupported — 400 +
// scim_type=mutability.
func NewUnsupportedError(detail string) ScimError {
	mutability := "mutability"
	return NewScimError(400, detail, &mutability)
}

// itoaStatus is `strconv.Itoa` inlined so the package doesn't pull
// strconv just for this.
func itoaStatus(n int) string {
	if n == 0 {
		return "0"
	}
	const digits = "0123456789"
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [11]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// ─── Service-provider config + schema/resource-type metadata ────────

// ServiceProviderConfig mirrors struct ServiceProviderConfig.
type ServiceProviderConfig struct {
	Schemas               []string                `json:"schemas"`
	DocumentationURI      string                  `json:"documentation_uri"`
	Patch                 Feature                 `json:"patch"`
	Bulk                  Feature                 `json:"bulk"`
	Filter                FilterFeature           `json:"filter"`
	ChangePassword        Feature                 `json:"change_password"`
	Sort                  Feature                 `json:"sort"`
	Etag                  Feature                 `json:"etag"`
	AuthenticationSchemes []AuthenticationScheme  `json:"authentication_schemes"`
	Meta                  ScimMeta                `json:"meta"`
}

// Feature is the canonical "bool only" capability descriptor.
type Feature struct {
	Supported bool `json:"supported"`
}

// FilterFeature mirrors struct FilterFeature.
type FilterFeature struct {
	Supported  bool `json:"supported"`
	MaxResults int  `json:"maxResults"`
}

// AuthenticationScheme mirrors struct AuthenticationScheme.
type AuthenticationScheme struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Primary     bool   `json:"primary"`
}

// ScimSchemaResource mirrors struct ScimSchemaResource.
type ScimSchemaResource struct {
	Schemas     []string               `json:"schemas"`
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Attributes  []ScimSchemaAttribute  `json:"attributes"`
	Meta        ScimMeta               `json:"meta"`
}

// ScimSchemaAttribute mirrors struct ScimSchemaAttribute.
type ScimSchemaAttribute struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	MultiValued bool   `json:"multiValued"`
	Required    bool   `json:"required"`
	Mutability  string `json:"mutability"`
	Returned    string `json:"returned"`
	Uniqueness  string `json:"uniqueness"`
}

// ScimResourceType mirrors struct ScimResourceType.
type ScimResourceType struct {
	Schemas  []string `json:"schemas"`
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Endpoint string   `json:"endpoint"`
	Schema   string   `json:"schema"`
	Meta     ScimMeta `json:"meta"`
}
