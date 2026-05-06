package scim

import "strings"

// ServiceProviderConfigPayload mirrors fn service_provider_config —
// builds the canonical /scim/v2/ServiceProviderConfig response for
// a deployment hosted at `baseURL`.
func ServiceProviderConfigPayload(baseURL string) ServiceProviderConfig {
	base := strings.TrimRight(baseURL, "/")
	return ServiceProviderConfig{
		Schemas:          []string{SchemaServiceProviderConfig},
		DocumentationURI: "https://www.rfc-editor.org/rfc/rfc7644",
		Patch:            Feature{Supported: true},
		Bulk:             Feature{Supported: false},
		Filter:           FilterFeature{Supported: true, MaxResults: 500},
		ChangePassword:   Feature{Supported: false},
		Sort:             Feature{Supported: false},
		Etag:             Feature{Supported: false},
		AuthenticationSchemes: []AuthenticationScheme{{
			Name:        "OAuth Bearer Token",
			Description: "JWT bearer token issued by OpenFoundry identity-federation-service",
			Type:        "oauthbearertoken",
			Primary:     true,
		}},
		Meta: ScimMeta{
			ResourceType: "ServiceProviderConfig",
			Location:     base + RouteServiceProviderConfig,
		},
	}
}

// SchemaResources mirrors fn schema_resources — returns the
// User + Group schema descriptors for the /scim/v2/Schemas
// listing. Order matches the Rust impl (User first, then Group).
func SchemaResources(baseURL string) []ScimSchemaResource {
	return []ScimSchemaResource{
		userSchema(baseURL),
		groupSchema(baseURL),
	}
}

// ResourceTypes mirrors fn resource_types — returns the User +
// Group resource type descriptors for /scim/v2/ResourceTypes.
func ResourceTypes(baseURL string) []ScimResourceType {
	base := strings.TrimRight(baseURL, "/")
	return []ScimResourceType{
		{
			Schemas:  []string{SchemaResourceType},
			ID:       "User",
			Name:     "User",
			Endpoint: RouteUsers,
			Schema:   SchemaUser,
			Meta: ScimMeta{
				ResourceType: "ResourceType",
				Location:     base + RouteResourceTypes + "/User",
			},
		},
		{
			Schemas:  []string{SchemaResourceType},
			ID:       "Group",
			Name:     "Group",
			Endpoint: RouteGroups,
			Schema:   SchemaGroup,
			Meta: ScimMeta{
				ResourceType: "ResourceType",
				Location:     base + RouteResourceTypes + "/Group",
			},
		},
	}
}

// userSchema mirrors fn user_schema — describes the User resource's
// attributes (userName / name / emails / active / externalId +
// the openfoundry user-extension URN).
func userSchema(baseURL string) ScimSchemaResource {
	base := strings.TrimRight(baseURL, "/")
	return ScimSchemaResource{
		Schemas:     []string{SchemaSchema},
		ID:          SchemaUser,
		Name:        "User",
		Description: "OpenFoundry SCIM user resource",
		Attributes: []ScimSchemaAttribute{
			schemaAttr("userName", "string", false, true, "readWrite", "default", "server"),
			schemaAttr("name", "complex", false, false, "readWrite", "default", "none"),
			schemaAttr("emails", "complex", true, false, "readWrite", "default", "none"),
			schemaAttr("active", "boolean", false, false, "readWrite", "default", "none"),
			schemaAttr("externalId", "string", false, false, "readWrite", "default", "none"),
			schemaAttr(SchemaOpenfoundryUserExtension, "complex", false, false, "readWrite", "default", "none"),
		},
		Meta: ScimMeta{
			ResourceType: "Schema",
			Location:     base + RouteSchemas + "/" + SchemaUser,
		},
	}
}

// groupSchema mirrors fn group_schema — describes the Group
// resource's attributes (displayName / members / externalId).
func groupSchema(baseURL string) ScimSchemaResource {
	base := strings.TrimRight(baseURL, "/")
	return ScimSchemaResource{
		Schemas:     []string{SchemaSchema},
		ID:          SchemaGroup,
		Name:        "Group",
		Description: "OpenFoundry SCIM group resource",
		Attributes: []ScimSchemaAttribute{
			schemaAttr("displayName", "string", false, true, "readWrite", "default", "server"),
			schemaAttr("members", "complex", true, false, "readWrite", "default", "none"),
			schemaAttr("externalId", "string", false, false, "readWrite", "default", "none"),
		},
		Meta: ScimMeta{
			ResourceType: "Schema",
			Location:     base + RouteSchemas + "/" + SchemaGroup,
		},
	}
}

// schemaAttr mirrors fn attr — convenience constructor for
// ScimSchemaAttribute. The argument order matches the Rust impl
// for verbatim parity.
func schemaAttr(name, typ string, multiValued, required bool, mutability, returned, uniqueness string) ScimSchemaAttribute {
	return ScimSchemaAttribute{
		Name:        name,
		Type:        typ,
		MultiValued: multiValued,
		Required:    required,
		Mutability:  mutability,
		Returned:    returned,
		Uniqueness:  uniqueness,
	}
}
