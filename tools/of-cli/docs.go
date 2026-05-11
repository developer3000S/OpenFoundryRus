package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	openFoundryAPIVersion    = "0.1.0"
	openAPIPath              = "apps/web/public/generated/openapi/openfoundry.json"
	generateOpenAPICommand   = "go run ./tools/of-cli docs generate-openapi --proto-dir proto --output %s"
	generateTypeScriptSDKCmd = "go run ./tools/of-cli docs generate-sdk-typescript --input %s --output %s"
	generatePythonSDKCmd     = "go run ./tools/of-cli docs generate-sdk-python --input %s --output %s"
	generateJavaSDKCmd       = "go run ./tools/of-cli docs generate-sdk-java --input %s --output %s"
)

type openAPISpec struct {
	OpenAPI    string                                 `json:"openapi"`
	Info       openAPIInfo                            `json:"info"`
	Servers    []openAPIServer                        `json:"servers,omitempty"`
	Tags       []openAPITag                           `json:"tags,omitempty"`
	Security   []map[string][]string                  `json:"security,omitempty"`
	Paths      map[string]map[string]openAPIOperation `json:"paths"`
	Components openAPIComponents                      `json:"components"`
}

type openAPIInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

type openAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type openAPITag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type openAPIComponents struct {
	Schemas         map[string]openAPISchema         `json:"schemas"`
	SecuritySchemes map[string]openAPISecurityScheme `json:"securitySchemes,omitempty"`
}

type openAPISecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
	Description  string `json:"description,omitempty"`
}

type openAPISchema struct {
	Type                 string                    `json:"type,omitempty"`
	Format               string                    `json:"format,omitempty"`
	Description          string                    `json:"description,omitempty"`
	Properties           *map[string]openAPISchema `json:"properties,omitempty"`
	Required             []string                  `json:"required,omitempty"`
	Items                *openAPISchema            `json:"items,omitempty"`
	Ref                  string                    `json:"$ref,omitempty"`
	AdditionalProperties *openAPISchema            `json:"additionalProperties,omitempty"`
	Enum                 []string                  `json:"enum,omitempty"`
}

type openAPIOperation struct {
	Summary                  string                     `json:"summary"`
	Description              string                     `json:"description,omitempty"`
	OperationID              string                     `json:"operationId"`
	Tags                     []string                   `json:"tags"`
	Parameters               []openAPIParameter         `json:"parameters,omitempty"`
	RequestBody              *openAPIRequestBody        `json:"requestBody,omitempty"`
	Responses                map[string]openAPIResponse `json:"responses"`
	Deprecated               *bool                      `json:"deprecated,omitempty"`
	Security                 []map[string][]string      `json:"security,omitempty"`
	XOpenFoundrySDKNamespace string                     `json:"x-openfoundry-sdk-namespace,omitempty"`
	XOpenFoundryAPIVersion   string                     `json:"x-openfoundry-api-version,omitempty"`
	XOpenFoundryMCPTool      string                     `json:"x-openfoundry-mcp-tool,omitempty"`
	XOpenFoundryStability    string                     `json:"x-openfoundry-stability,omitempty"`
}

type openAPIParameter struct {
	Name        string        `json:"name"`
	In          string        `json:"in"`
	Required    bool          `json:"required"`
	Description string        `json:"description,omitempty"`
	Schema      openAPISchema `json:"schema"`
}

type openAPIRequestBody struct {
	Required bool                        `json:"required"`
	Content  map[string]openAPIMediaType `json:"content"`
}

type openAPIResponse struct {
	Description string                      `json:"description"`
	Content     map[string]openAPIMediaType `json:"content"`
}

type openAPIMediaType struct {
	Schema openAPISchema `json:"schema"`
}

type protoService struct {
	packageName string
	name        string
	rpcs        []protoRPC
}

type protoRPC struct {
	name     string
	request  string
	response string
}

func generateOpenAPI(protoDir, output string) error {
	spec, err := buildOpenAPI(protoDir)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFile(output, data)
}

func validateOpenAPI(protoDir, expected string) error {
	spec, err := buildOpenAPI(protoDir)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	current, err := os.ReadFile(expected)
	if err != nil {
		return err
	}
	if !bytes.Equal(bytes.TrimSpace(current), bytes.TrimSpace(data)) {
		return fmt.Errorf("OpenAPI drift detected in %s. Regenerate it with `%s`", expected, fmt.Sprintf(generateOpenAPICommand, expected))
	}
	return nil
}

func buildOpenAPI(protoDir string) (openAPISpec, error) {
	files, err := collectProtoFiles(protoDir)
	if err != nil {
		return openAPISpec{}, err
	}

	serviceRe := regexp.MustCompile(`service\s+(\w+)\s*\{([\s\S]*?)\}`)
	rpcRe := regexp.MustCompile(`rpc\s+(\w+)\s*\(([^)]+)\)\s+returns\s+\(([^)]+)\)`)
	packageRe := regexp.MustCompile(`package\s+([a-zA-Z0-9_\.]+)`)

	var services []protoService
	schemas := map[string]openAPISchema{}
	for _, file := range files {
		contentBytes, err := os.ReadFile(file)
		if err != nil {
			return openAPISpec{}, err
		}
		content := string(contentBytes)
		packageName := "open_foundry.unknown"
		if match := packageRe.FindStringSubmatch(content); len(match) > 1 {
			packageName = match[1]
		}
		for _, serviceMatch := range serviceRe.FindAllStringSubmatch(content, -1) {
			var rpcs []protoRPC
			for _, rpcMatch := range rpcRe.FindAllStringSubmatch(serviceMatch[2], -1) {
				rpcs = append(rpcs, protoRPC{
					name:     rpcMatch[1],
					request:  sanitizeTypeName(rpcMatch[2]),
					response: sanitizeTypeName(rpcMatch[3]),
				})
			}
			if len(rpcs) > 0 {
				services = append(services, protoService{packageName: packageName, name: serviceMatch[1], rpcs: rpcs})
			}
		}
		for name, schema := range parseMessageSchemas(content) {
			schemas[name] = schema
		}
	}

	paths := map[string]map[string]openAPIOperation{}
	for _, service := range services {
		basePath := packageToBasePath(service.packageName)
		for _, rpc := range service.rpcs {
			path := fmt.Sprintf("/api/v1/%s/%s", basePath, toKebabCase(rpc.name))
			method := httpMethodForRPC(rpc.name)
			parameters := []openAPIParameter(nil)
			if method == "get" || method == "delete" {
				parameters = queryParametersFromRequestSchema(rpc.request, schemas)
			}
			namespace := namespaceForOperation(service.packageName, rpc.name, path, []string{service.packageName})
			operation := openAPIOperation{
				Summary:                  fmt.Sprintf("%s %s", service.name, rpc.name),
				Description:              fmt.Sprintf("Generated from `%s` RPC `%s` in service `%s`.", service.packageName, rpc.name, service.name),
				OperationID:              fmt.Sprintf("%s.%s.%s", service.packageName, service.name, rpc.name),
				Tags:                     []string{service.packageName},
				Parameters:               parameters,
				Responses:                successAndErrorResponses(schemaRef(rpc.response)),
				Security:                 bearerAuthSecurity(),
				XOpenFoundrySDKNamespace: namespace,
				XOpenFoundryAPIVersion:   apiVersionFromPath(path),
				XOpenFoundryMCPTool:      mcpToolName(namespace, rpc.name),
				XOpenFoundryStability:    stabilityForPath(path),
			}
			if method != "get" && method != "delete" {
				operation.RequestBody = &openAPIRequestBody{
					Required: true,
					Content:  jsonContent(schemaRef(rpc.request)),
				}
			}
			if paths[path] == nil {
				paths[path] = map[string]openAPIOperation{}
			}
			paths[path][method] = operation
		}
	}

	spec := openAPISpec{
		OpenAPI: "3.1.0",
		Info: openAPIInfo{
			Title:       "OpenFoundry API",
			Version:     openFoundryAPIVersion,
			Description: "Versioned OpenFoundry JSON/HTTP contract generated from proto services and curated REST overlays.",
		},
		Servers:  []openAPIServer{{URL: "/", Description: "OpenFoundry API gateway root"}},
		Security: bearerAuthSecurity(),
		Paths:    paths,
		Components: openAPIComponents{
			Schemas: schemas,
			SecuritySchemes: map[string]openAPISecurityScheme{
				"bearerAuth": bearerSecurityScheme(),
			},
		},
	}
	augmentWithRESTOverlays(&spec)
	finalizeSpecMetadata(&spec)
	return spec, nil
}

func collectProtoFiles(dir string) ([]string, error) {
	var files []string
	if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".proto") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func parseMessageSchemas(content string) map[string]openAPISchema {
	messageRe := regexp.MustCompile(`message\s+(\w+)\s*\{([\s\S]*?)\}`)
	fieldRe := regexp.MustCompile(`(repeated\s+)?(map<[^>]+>|[a-zA-Z0-9_\.]+)\s+(\w+)\s*=\s*\d+`)
	schemas := map[string]openAPISchema{}
	for _, message := range messageRe.FindAllStringSubmatch(content, -1) {
		properties := map[string]openAPISchema{}
		for _, field := range fieldRe.FindAllStringSubmatch(message[2], -1) {
			properties[field[3]] = fieldSchema(field[2], strings.TrimSpace(field[1]) != "")
		}
		schemas[message[1]] = objectSchema(properties)
	}
	return schemas
}

func fieldSchema(fieldType string, repeated bool) openAPISchema {
	var schema openAPISchema
	if strings.HasPrefix(fieldType, "map<") {
		inner := strings.TrimSuffix(strings.TrimPrefix(fieldType, "map<"), ">")
		valueType := "string"
		parts := strings.SplitN(inner, ",", 2)
		if len(parts) == 2 {
			valueType = strings.TrimSpace(parts[1])
		}
		valueSchema := fieldSchema(valueType, false)
		schema = openAPISchema{Type: "object", AdditionalProperties: &valueSchema}
	} else {
		schema = primitiveOrRef(fieldType)
	}
	if repeated {
		return arraySchema(schema)
	}
	return schema
}

func primitiveOrRef(fieldType string) openAPISchema {
	switch fieldType {
	case "string", "bytes":
		return stringSchema("")
	case "bool":
		return booleanSchema()
	case "float", "double":
		return openAPISchema{Type: "number"}
	case "int32", "uint32":
		return openAPISchema{Type: "integer", Format: "int32"}
	case "int64", "uint64":
		return integerSchema()
	case "google.protobuf.Timestamp":
		return stringSchema("date-time")
	default:
		return schemaRef(fieldType)
	}
}

func queryParametersFromRequestSchema(requestType string, schemas map[string]openAPISchema) []openAPIParameter {
	schema, ok := schemas[requestType]
	if !ok || schema.Properties == nil {
		return nil
	}
	names := sortedSchemaKeys(*schema.Properties)
	parameters := make([]openAPIParameter, 0, len(names))
	for _, name := range names {
		parameters = append(parameters, openAPIParameter{
			Name:        name,
			In:          "query",
			Required:    false,
			Description: fmt.Sprintf("Query parameter derived from `%s.%s`.", requestType, name),
			Schema:      (*schema.Properties)[name],
		})
	}
	return parameters
}

func successAndErrorResponses(responseSchema openAPISchema) map[string]openAPIResponse {
	return map[string]openAPIResponse{
		"200": {
			Description: "Successful response",
			Content:     jsonContent(responseSchema),
		},
		"default": {
			Description: "Structured error response",
			Content:     jsonContent(schemaRef("ApiError")),
		},
	}
}

func bearerAuthSecurity() []map[string][]string {
	return []map[string][]string{{"bearerAuth": []string{}}}
}

func bearerSecurityScheme() openAPISecurityScheme {
	return openAPISecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: "JWT",
		Description:  "Bearer token accepted by the OpenFoundry gateway. Session and personal access tokens share this scheme.",
	}
}

func jsonContent(schema openAPISchema) map[string]openAPIMediaType {
	return map[string]openAPIMediaType{"application/json": {Schema: schema}}
}

func schemaRef(name string) openAPISchema {
	return openAPISchema{Ref: "#/components/schemas/" + sanitizeTypeName(name)}
}

func sanitizeTypeName(name string) string {
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), "stream")), ".")
	return parts[len(parts)-1]
}

func objectSchema(properties map[string]openAPISchema) openAPISchema {
	return openAPISchema{Type: "object", Properties: &properties}
}

func objectWithAdditionalProperties(valueType openAPISchema) openAPISchema {
	return openAPISchema{Type: "object", AdditionalProperties: &valueType}
}

func stringSchema(format string) openAPISchema {
	return openAPISchema{Type: "string", Format: format}
}

func integerSchema() openAPISchema {
	return openAPISchema{Type: "integer", Format: "int64"}
}

func booleanSchema() openAPISchema {
	return openAPISchema{Type: "boolean"}
}

func arraySchema(itemSchema openAPISchema) openAPISchema {
	return openAPISchema{Type: "array", Items: &itemSchema}
}

func anyValueSchema() openAPISchema {
	return objectWithAdditionalProperties(openAPISchema{})
}

func packageToBasePath(packageName string) string {
	parts := strings.Split(packageName, ".")
	segment := parts[len(parts)-1]
	switch segment {
	case "query":
		return "queries"
	case "dataset":
		return "datasets"
	case "pipeline":
		return "pipelines"
	case "workflow":
		return "workflows"
	case "notification":
		return "notifications"
	case "app_builder":
		return "apps"
	case "report":
		return "reports"
	case "code_repo":
		return "code-repos"
	default:
		return strings.ReplaceAll(segment, "_", "-")
	}
}

func httpMethodForRPC(name string) string {
	if strings.HasPrefix(name, "List") || strings.HasPrefix(name, "Get") {
		return "get"
	}
	if strings.HasPrefix(name, "Delete") {
		return "delete"
	}
	if strings.HasPrefix(name, "Update") {
		return "patch"
	}
	return "post"
}

func namespaceForOperation(packageName, rpcName, path string, tags []string) string {
	if len(tags) > 0 {
		trimmed := strings.TrimPrefix(strings.TrimPrefix(tags[0], "open_foundry."), "rest.")
		if trimmed != "" {
			return strings.ReplaceAll(trimmed, ".", "_")
		}
	}
	segments := strings.Split(path, "/")
	nonEmpty := make([]string, 0, len(segments))
	for _, segment := range segments {
		if segment != "" {
			nonEmpty = append(nonEmpty, segment)
		}
	}
	if len(nonEmpty) > 2 {
		return strings.ReplaceAll(nonEmpty[2], "-", "_")
	}
	parts := strings.Split(packageName, ".")
	leaf := strings.ReplaceAll(parts[len(parts)-1], "-", "_")
	if leaf == "" {
		return strings.ToLower(rpcName)
	}
	return leaf
}

func apiVersionFromPath(path string) string {
	for _, segment := range strings.Split(path, "/") {
		if isAPIVersionSegment(segment) {
			return segment
		}
	}
	return ""
}

func apiVersionFromIdentifier(value string) string {
	for _, segment := range regexp.MustCompile(`[\/\._]`).Split(value, -1) {
		if isAPIVersionSegment(segment) {
			return segment
		}
	}
	return ""
}

func isAPIVersionSegment(segment string) bool {
	if len(segment) < 2 || segment[0] != 'v' {
		return false
	}
	for _, ch := range segment[1:] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func stabilityForPath(path string) string {
	if apiVersionFromPath(path) == "v2" {
		return "stable"
	}
	return "beta"
}

func mcpToolName(namespace, operationName string) string {
	return fmt.Sprintf("openfoundry.%s.%s", toCamelCase(namespace), toCamelCase(operationName))
}

func finalizeSpecMetadata(spec *openAPISpec) {
	if _, ok := spec.Components.Schemas["ApiError"]; !ok {
		spec.Components.Schemas["ApiError"] = apiErrorSchema()
	}
	tagSet := map[string]bool{}
	for _, path := range spec.Paths {
		for _, operation := range path {
			for _, tag := range operation.Tags {
				tagSet[tag] = true
			}
		}
	}
	if len(tagSet) == 0 {
		tagSet["open_foundry"] = true
	}
	tagNames := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tagNames = append(tagNames, tag)
	}
	sort.Strings(tagNames)
	spec.Tags = make([]openAPITag, 0, len(tagNames))
	for _, tag := range tagNames {
		spec.Tags = append(spec.Tags, openAPITag{Name: tag, Description: fmt.Sprintf("Operations generated for the `%s` namespace.", tag)})
	}
}

func apiErrorSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"code":       stringSchema(""),
		"details":    anyValueSchema(),
		"message":    stringSchema(""),
		"request_id": stringSchema(""),
		"status":     integerSchema(),
	})
}

func augmentWithRESTOverlays(spec *openAPISpec) {
	schemas := spec.Components.Schemas
	schemas["UserResponse"] = userResponseSchema()
	schemas["UpdateUserRequest"] = updateUserRequestSchema()
	schemas["Permission"] = permissionSchema()
	schemas["CreatePermissionRequest"] = createPermissionRequestSchema()
	schemas["RoleResponse"] = roleResponseSchema()
	schemas["CreateRoleRequest"] = createRoleRequestSchema()
	schemas["UpdateRoleRequest"] = updateRoleRequestSchema()
	schemas["GroupResponse"] = groupResponseSchema()
	schemas["CreateGroupRequest"] = createGroupRequestSchema()
	schemas["UpdateGroupRequest"] = updateGroupRequestSchema()
	schemas["Policy"] = policySchema()
	schemas["UpsertPolicyRequest"] = upsertPolicyRequestSchema()
	schemas["PolicyEvaluationResult"] = policyEvaluationResultSchema()
	schemas["EvaluatePolicyRequest"] = evaluatePolicyRequestSchema()
	schemas["AppBrandingSettings"] = appBrandingSettingsSchema()
	schemas["ControlPanelSettings"] = controlPanelSettingsSchema()
	schemas["UpdateControlPanelRequest"] = updateControlPanelRequestSchema()
	schemas["AdminUsersListResponse"] = listResponseSchema("UserResponse")
	schemas["AdminRolesListResponse"] = listResponseSchema("RoleResponse")
	schemas["AdminGroupsListResponse"] = listResponseSchema("GroupResponse")
	schemas["AdminPermissionsListResponse"] = listResponseSchema("Permission")
	schemas["AdminPoliciesListResponse"] = listResponseSchema("Policy")
	schemas["FilesystemBreadcrumb"] = filesystemBreadcrumbSchema()
	schemas["FilesystemEntry"] = filesystemEntrySchema()
	schemas["FilesystemSections"] = filesystemSectionsSchema()
	schemas["FilesystemListResponse"] = filesystemListResponseSchema()

	insertOperation(spec, "/api/v2/admin/users", "get", manualOperation("List admin users (v2)", "rest.admin.v2.listUsers", []string{"rest.admin.v2"}, "", "AdminUsersListResponse", nil))
	insertOperation(spec, "/api/v2/admin/users/me", "get", manualOperation("Get current admin user (v2)", "rest.admin.v2.getCurrentUser", []string{"rest.admin.v2"}, "", "UserResponse", nil))
	insertOperation(spec, "/api/v2/admin/users/{id}", "patch", manualOperation("Update admin user (v2)", "rest.admin.v2.updateUser", []string{"rest.admin.v2"}, "UpdateUserRequest", "UserResponse", []openAPIParameter{pathParameter("id")}))
	insertOperation(spec, "/api/v2/admin/roles", "get", manualOperation("List roles (v2)", "rest.admin.v2.listRoles", []string{"rest.admin.v2"}, "", "AdminRolesListResponse", nil))
	insertOperation(spec, "/api/v2/admin/roles", "post", manualOperation("Create role (v2)", "rest.admin.v2.createRole", []string{"rest.admin.v2"}, "CreateRoleRequest", "RoleResponse", nil))
	insertOperation(spec, "/api/v2/admin/roles/{id}", "put", manualOperation("Update role (v2)", "rest.admin.v2.updateRole", []string{"rest.admin.v2"}, "UpdateRoleRequest", "RoleResponse", []openAPIParameter{pathParameter("id")}))
	insertOperation(spec, "/api/v2/admin/groups", "get", manualOperation("List groups (v2)", "rest.admin.v2.listGroups", []string{"rest.admin.v2"}, "", "AdminGroupsListResponse", nil))
	insertOperation(spec, "/api/v2/admin/groups", "post", manualOperation("Create group (v2)", "rest.admin.v2.createGroup", []string{"rest.admin.v2"}, "CreateGroupRequest", "GroupResponse", nil))
	insertOperation(spec, "/api/v2/admin/groups/{id}", "put", manualOperation("Update group (v2)", "rest.admin.v2.updateGroup", []string{"rest.admin.v2"}, "UpdateGroupRequest", "GroupResponse", []openAPIParameter{pathParameter("id")}))
	insertOperation(spec, "/api/v2/admin/permissions", "get", manualOperation("List permissions (v2)", "rest.admin.v2.listPermissions", []string{"rest.admin.v2"}, "", "AdminPermissionsListResponse", nil))
	insertOperation(spec, "/api/v2/admin/permissions", "post", manualOperation("Create permission (v2)", "rest.admin.v2.createPermission", []string{"rest.admin.v2"}, "CreatePermissionRequest", "Permission", nil))
	insertOperation(spec, "/api/v2/admin/policies", "get", manualOperation("List policies (v2)", "rest.admin.v2.listPolicies", []string{"rest.admin.v2"}, "", "AdminPoliciesListResponse", nil))
	insertOperation(spec, "/api/v2/admin/policies", "post", manualOperation("Create policy (v2)", "rest.admin.v2.createPolicy", []string{"rest.admin.v2"}, "UpsertPolicyRequest", "Policy", nil))
	insertOperation(spec, "/api/v2/admin/policies/{id}", "patch", manualOperation("Update policy (v2)", "rest.admin.v2.updatePolicy", []string{"rest.admin.v2"}, "UpsertPolicyRequest", "Policy", []openAPIParameter{pathParameter("id")}))
	insertOperation(spec, "/api/v2/admin/policies/evaluate", "post", manualOperation("Evaluate policy (v2)", "rest.admin.v2.evaluatePolicy", []string{"rest.admin.v2"}, "EvaluatePolicyRequest", "PolicyEvaluationResult", nil))
	insertOperation(spec, "/api/v2/admin/control-panel", "get", manualOperation("Get control panel (v2)", "rest.admin.v2.getControlPanel", []string{"rest.admin.v2"}, "", "ControlPanelSettings", nil))
	insertOperation(spec, "/api/v2/admin/control-panel", "put", manualOperation("Update control panel (v2)", "rest.admin.v2.updateControlPanel", []string{"rest.admin.v2"}, "UpdateControlPanelRequest", "ControlPanelSettings", nil))
	insertOperation(spec, "/api/v2/filesystem/datasets/{dataset_id}", "get", manualOperation("List dataset filesystem (v2)", "rest.filesystem.v2.getDatasetFilesystem", []string{"rest.filesystem.v2"}, "", "FilesystemListResponse", []openAPIParameter{pathParameter("dataset_id"), queryParameter("path")}))
}

func insertOperation(spec *openAPISpec, path, method string, operation openAPIOperation) {
	if spec.Paths[path] == nil {
		spec.Paths[path] = map[string]openAPIOperation{}
	}
	spec.Paths[path][method] = operation
}

func manualOperation(summary, operationID string, tags []string, requestSchema, responseSchema string, parameters []openAPIParameter) openAPIOperation {
	leaf := operationID
	if idx := strings.LastIndex(leaf, "."); idx >= 0 {
		leaf = leaf[idx+1:]
	}
	namespace := namespaceForOperation(operationID, leaf, operationID, tags)
	operation := openAPIOperation{
		Summary:                  summary,
		Description:              fmt.Sprintf("Curated REST overlay for `%s`.", operationID),
		OperationID:              operationID,
		Tags:                     tags,
		Parameters:               parameters,
		Responses:                successAndErrorResponses(schemaRef(responseSchema)),
		Security:                 bearerAuthSecurity(),
		XOpenFoundrySDKNamespace: namespace,
		XOpenFoundryAPIVersion:   apiVersionFromIdentifier(operationID),
		XOpenFoundryMCPTool:      mcpToolName(namespace, leaf),
		XOpenFoundryStability:    "stable",
	}
	if requestSchema != "" {
		operation.RequestBody = &openAPIRequestBody{Required: true, Content: jsonContent(schemaRef(requestSchema))}
	}
	return operation
}

func pathParameter(name string) openAPIParameter {
	return openAPIParameter{Name: name, In: "path", Required: true, Description: fmt.Sprintf("Path parameter `%s`.", name), Schema: stringSchema("")}
}

func queryParameter(name string) openAPIParameter {
	return openAPIParameter{Name: name, In: "query", Required: false, Description: fmt.Sprintf("Query parameter `%s`.", name), Schema: stringSchema("")}
}

func listResponseSchema(itemSchemaName string) openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"count": integerSchema(),
		"items": arraySchema(schemaRef(itemSchemaName)),
	})
}

func userResponseSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"attributes":      anyValueSchema(),
		"auth_source":     stringSchema(""),
		"created_at":      stringSchema("date-time"),
		"email":           stringSchema(""),
		"groups":          arraySchema(stringSchema("")),
		"id":              stringSchema("uuid"),
		"is_active":       booleanSchema(),
		"mfa_enabled":     booleanSchema(),
		"mfa_enforced":    booleanSchema(),
		"name":            stringSchema(""),
		"organization_id": stringSchema("uuid"),
		"permissions":     arraySchema(stringSchema("")),
		"roles":           arraySchema(stringSchema("")),
	})
}

func updateUserRequestSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"attributes":      anyValueSchema(),
		"is_active":       booleanSchema(),
		"mfa_enforced":    booleanSchema(),
		"name":            stringSchema(""),
		"organization_id": stringSchema("uuid"),
	})
}

func permissionSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"action":      stringSchema(""),
		"created_at":  stringSchema("date-time"),
		"description": stringSchema(""),
		"id":          stringSchema("uuid"),
		"resource":    stringSchema(""),
	})
}

func createPermissionRequestSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"action":      stringSchema(""),
		"description": stringSchema(""),
		"resource":    stringSchema(""),
	})
}

func roleResponseSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"created_at":     stringSchema("date-time"),
		"description":    stringSchema(""),
		"id":             stringSchema("uuid"),
		"name":           stringSchema(""),
		"permission_ids": arraySchema(stringSchema("uuid")),
		"permissions":    arraySchema(stringSchema("")),
	})
}

func createRoleRequestSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"description":    stringSchema(""),
		"name":           stringSchema(""),
		"permission_ids": arraySchema(stringSchema("uuid")),
	})
}

func updateRoleRequestSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"description":    stringSchema(""),
		"permission_ids": arraySchema(stringSchema("uuid")),
	})
}

func groupResponseSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"created_at":   stringSchema("date-time"),
		"description":  stringSchema(""),
		"id":           stringSchema("uuid"),
		"member_count": integerSchema(),
		"name":         stringSchema(""),
		"role_ids":     arraySchema(stringSchema("uuid")),
		"roles":        arraySchema(stringSchema("")),
	})
}

func createGroupRequestSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"description": stringSchema(""),
		"name":        stringSchema(""),
		"role_ids":    arraySchema(stringSchema("uuid")),
	})
}

func updateGroupRequestSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"description": stringSchema(""),
		"role_ids":    arraySchema(stringSchema("uuid")),
	})
}

func policySchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"action":      stringSchema(""),
		"conditions":  anyValueSchema(),
		"created_at":  stringSchema("date-time"),
		"created_by":  stringSchema("uuid"),
		"description": stringSchema(""),
		"effect":      stringSchema(""),
		"enabled":     booleanSchema(),
		"id":          stringSchema("uuid"),
		"name":        stringSchema(""),
		"resource":    stringSchema(""),
		"row_filter":  stringSchema(""),
		"updated_at":  stringSchema("date-time"),
	})
}

func upsertPolicyRequestSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"action":      stringSchema(""),
		"conditions":  anyValueSchema(),
		"description": stringSchema(""),
		"effect":      stringSchema(""),
		"enabled":     booleanSchema(),
		"name":        stringSchema(""),
		"resource":    stringSchema(""),
		"row_filter":  stringSchema(""),
	})
}

func policyEvaluationResultSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"allowed":            booleanSchema(),
		"deny_policy_ids":    arraySchema(stringSchema("uuid")),
		"matched_policy_ids": arraySchema(stringSchema("uuid")),
		"row_filter":         stringSchema(""),
	})
}

func evaluatePolicyRequestSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"action":              stringSchema(""),
		"resource":            stringSchema(""),
		"resource_attributes": anyValueSchema(),
	})
}

func appBrandingSettingsSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"accent_color":    stringSchema(""),
		"display_name":    stringSchema(""),
		"favicon_url":     stringSchema(""),
		"logo_url":        stringSchema(""),
		"primary_color":   stringSchema(""),
		"show_powered_by": booleanSchema(),
	})
}

func controlPanelSettingsSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"allow_self_signup":     booleanSchema(),
		"allowed_email_domains": arraySchema(stringSchema("")),
		"announcement_banner":   stringSchema(""),
		"default_app_branding":  schemaRef("AppBrandingSettings"),
		"default_region":        stringSchema(""),
		"deployment_mode":       stringSchema(""),
		"docs_url":              stringSchema(""),
		"maintenance_mode":      booleanSchema(),
		"platform_name":         stringSchema(""),
		"release_channel":       stringSchema(""),
		"restricted_operations": arraySchema(stringSchema("")),
		"status_page_url":       stringSchema(""),
		"support_email":         stringSchema(""),
		"updated_at":            stringSchema("date-time"),
		"updated_by":            stringSchema("uuid"),
	})
}

func updateControlPanelRequestSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"allow_self_signup":     booleanSchema(),
		"allowed_email_domains": arraySchema(stringSchema("")),
		"announcement_banner":   stringSchema(""),
		"default_app_branding":  schemaRef("AppBrandingSettings"),
		"default_region":        stringSchema(""),
		"deployment_mode":       stringSchema(""),
		"docs_url":              stringSchema(""),
		"maintenance_mode":      booleanSchema(),
		"platform_name":         stringSchema(""),
		"release_channel":       stringSchema(""),
		"restricted_operations": arraySchema(stringSchema("")),
		"status_page_url":       stringSchema(""),
		"support_email":         stringSchema(""),
	})
}

func filesystemBreadcrumbSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"name": stringSchema(""),
		"path": stringSchema(""),
	})
}

func filesystemEntrySchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"content_type":  stringSchema(""),
		"entry_type":    stringSchema(""),
		"last_modified": stringSchema("date-time"),
		"metadata":      anyValueSchema(),
		"name":          stringSchema(""),
		"path":          stringSchema(""),
		"size_bytes":    integerSchema(),
		"storage_path":  stringSchema(""),
	})
}

func filesystemSectionsSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"branches": integerSchema(),
		"versions": integerSchema(),
		"views":    integerSchema(),
	})
}

func filesystemListResponseSchema() openAPISchema {
	return objectSchema(map[string]openAPISchema{
		"active_branch":   stringSchema(""),
		"breadcrumbs":     arraySchema(schemaRef("FilesystemBreadcrumb")),
		"current_version": integerSchema(),
		"dataset_id":      stringSchema("uuid"),
		"entries":         arraySchema(schemaRef("FilesystemEntry")),
		"items":           arraySchema(schemaRef("FilesystemEntry")),
		"requested_path":  stringSchema(""),
		"root":            stringSchema(""),
		"sections":        schemaRef("FilesystemSections"),
	})
}

func generateSDK(input, output, lang string) error {
	spec, err := loadOpenAPISpec(input)
	if err != nil {
		return err
	}
	files, err := renderSDKFiles(spec, lang, input, output)
	if err != nil {
		return err
	}
	return writeGeneratedFiles(output, files)
}

func validateSDK(input, output, lang string) error {
	spec, err := loadOpenAPISpec(input)
	if err != nil {
		return err
	}
	files, err := renderSDKFiles(spec, lang, input, output)
	if err != nil {
		return err
	}
	command := regenerationCommand(lang, input, output)
	var diffs []string
	for _, rel := range sortedStringKeys(files) {
		expected := normalizeLineEndings(files[rel])
		actualBytes, err := os.ReadFile(filepath.Join(output, rel))
		if err != nil {
			diffs = append(diffs, rel)
			continue
		}
		if normalizeLineEndings(string(actualBytes)) != expected {
			diffs = append(diffs, rel)
		}
	}
	if len(diffs) > 0 {
		return fmt.Errorf("%s SDK drift detected: %s. Regenerate it with `%s`", lang, strings.Join(diffs, ", "), command)
	}
	return nil
}

func loadOpenAPISpec(path string) (openAPISpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return openAPISpec{}, err
	}
	var spec openAPISpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return openAPISpec{}, err
	}
	return spec, nil
}

func renderSDKFiles(spec openAPISpec, lang, input, output string) (map[string]string, error) {
	switch lang {
	case "typescript":
		return renderTypeScriptSDK(spec), nil
	case "python":
		return renderPythonSDK(spec), nil
	case "java":
		return renderJavaSDK(spec), nil
	default:
		return nil, fmt.Errorf("unknown SDK language %q", lang)
	}
}

func regenerationCommand(lang, input, output string) string {
	switch lang {
	case "typescript":
		return fmt.Sprintf(generateTypeScriptSDKCmd, input, output)
	case "python":
		return fmt.Sprintf(generatePythonSDKCmd, input, output)
	case "java":
		return fmt.Sprintf(generateJavaSDKCmd, input, output)
	default:
		return ""
	}
}

func writeGeneratedFiles(output string, files map[string]string) error {
	for _, rel := range sortedStringKeys(files) {
		if err := writeFile(filepath.Join(output, rel), []byte(files[rel])); err != nil {
			return err
		}
	}
	return nil
}

type ownedParameter struct {
	name     string
	required bool
	schema   openAPISchema
}

type operationRenderInfo struct {
	path                string
	method              string
	operationID         string
	summary             string
	description         string
	responseType        string
	requestType         string
	flatMethodName      string
	namespaceProperty   string
	namespaceMemberName string
	pathParameters      []ownedParameter
	queryParameters     []ownedParameter
	hasBody             bool
	mcpToolName         string
	apiVersion          string
	stability           string
}

func collectOperationRenderInfos(spec openAPISpec) []operationRenderInfo {
	var items []operationRenderInfo
	usedFlatMethodNames := map[string]int{}
	namespaceMethodCounters := map[string]map[string]int{}
	for _, path := range sortedStringKeys(spec.Paths) {
		methods := spec.Paths[path]
		for _, method := range sortedStringKeys(methods) {
			operation := methods[method]
			namespaceID := operation.XOpenFoundrySDKNamespace
			if namespaceID == "" {
				namespaceID = namespaceForOperation("", operation.OperationID, path, operation.Tags)
			}
			namespaceProperty := namespacePropertyName(namespaceID)
			namespaceMemberSeed := simpleOperationMemberName(operation)
			if namespaceMethodCounters[namespaceProperty] == nil {
				namespaceMethodCounters[namespaceProperty] = map[string]int{}
			}
			requestType := requestTypeForOperation(operation)
			mcpName := operation.XOpenFoundryMCPTool
			if mcpName == "" {
				mcpName = mcpToolName(namespaceID, lastToken(operation.OperationID))
			}
			items = append(items, operationRenderInfo{
				path:                path,
				method:              strings.ToUpper(method),
				operationID:         operation.OperationID,
				summary:             operation.Summary,
				description:         operation.Description,
				responseType:        responseTypeForOperation(operation),
				requestType:         requestType,
				flatMethodName:      uniqueMethodName(methodNameForOperation(operation), usedFlatMethodNames),
				namespaceProperty:   namespaceProperty,
				namespaceMemberName: uniqueMethodName(namespaceMemberSeed, namespaceMethodCounters[namespaceProperty]),
				pathParameters:      ownedParameters(operationPathParameters(operation)),
				queryParameters:     ownedParameters(operationQueryParameters(operation)),
				hasBody:             requestType != "",
				mcpToolName:         mcpName,
				apiVersion:          operation.XOpenFoundryAPIVersion,
				stability:           operation.XOpenFoundryStability,
			})
		}
	}
	return items
}

func ownedParameters(parameters []openAPIParameter) []ownedParameter {
	out := make([]ownedParameter, 0, len(parameters))
	for _, parameter := range parameters {
		out = append(out, ownedParameter{name: parameter.Name, required: parameter.Required, schema: parameter.Schema})
	}
	return out
}

func simpleOperationMemberName(operation openAPIOperation) string {
	return toCamelCase(lastToken(operation.OperationID))
}

func namespacePropertyName(namespace string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(namespace, "open_foundry."), "rest.")
	tokens := splitIdentifierTokens(trimmed)
	if len(tokens) == 0 {
		return "defaultApi"
	}
	var property strings.Builder
	for i, token := range tokens {
		isVersion := isAPIVersionSegment(token)
		if i == 0 {
			property.WriteString(strings.ToLower(token))
		} else if isVersion {
			property.WriteString(strings.ToUpper(token))
		} else {
			property.WriteString(toPascalCase(token))
		}
	}
	return property.String()
}

func orderedNamespaceProperties(operations []operationRenderInfo) []string {
	set := map[string]bool{}
	for _, operation := range operations {
		set[operation.namespaceProperty] = true
	}
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func requestTypeForOperation(operation openAPIOperation) string {
	if operation.RequestBody == nil {
		return ""
	}
	if media, ok := operation.RequestBody.Content["application/json"]; ok {
		return typescriptType(media.Schema)
	}
	for _, key := range sortedStringKeys(operation.RequestBody.Content) {
		return typescriptType(operation.RequestBody.Content[key].Schema)
	}
	return ""
}

func responseTypeForOperation(operation openAPIOperation) string {
	if response, ok := operation.Responses["200"]; ok {
		if media, ok := response.Content["application/json"]; ok {
			return typescriptType(media.Schema)
		}
		for _, key := range sortedStringKeys(response.Content) {
			return typescriptType(response.Content[key].Schema)
		}
	}
	return "unknown"
}

func operationPathParameters(operation openAPIOperation) []openAPIParameter {
	var parameters []openAPIParameter
	for _, parameter := range operation.Parameters {
		if parameter.In == "path" {
			parameters = append(parameters, parameter)
		}
	}
	return parameters
}

func operationQueryParameters(operation openAPIOperation) []openAPIParameter {
	var parameters []openAPIParameter
	for _, parameter := range operation.Parameters {
		if parameter.In == "query" {
			parameters = append(parameters, parameter)
		}
	}
	return parameters
}

func methodNameForOperation(operation openAPIOperation) string {
	parts := strings.Split(operation.OperationID, ".")
	packageLeaf := "openfoundry"
	service := "service"
	rpc := "call"
	if len(parts) >= 3 {
		packageLeaf = parts[len(parts)-3]
	}
	if len(parts) >= 2 {
		service = strings.TrimSuffix(parts[len(parts)-2], "Service")
	}
	if len(parts) >= 1 {
		rpc = parts[len(parts)-1]
	}
	return toCamelCase(packageLeaf) + toPascalCase(service) + toPascalCase(rpc)
}

func uniqueMethodName(name string, used map[string]int) string {
	count := used[name]
	if count == 0 {
		used[name] = 1
		return name
	}
	used[name] = count + 1
	return fmt.Sprintf("%s%d", name, count+1)
}

func buildMCPInputSchemaValue(operation operationRenderInfo) map[string]any {
	properties := map[string]any{}
	required := []string{}
	if len(operation.pathParameters) > 0 {
		pathProperties := map[string]any{}
		requiredPath := []string{}
		for _, parameter := range operation.pathParameters {
			pathProperties[parameter.name] = schemaToJSONValue(parameter.schema)
			if parameter.required {
				required = append(required, "path")
				requiredPath = append(requiredPath, parameter.name)
			}
		}
		pathSchema := map[string]any{"type": "object", "properties": pathProperties}
		if len(requiredPath) > 0 {
			pathSchema["required"] = requiredPath
		}
		properties["path"] = pathSchema
	}
	if len(operation.queryParameters) > 0 {
		queryProperties := map[string]any{}
		for _, parameter := range operation.queryParameters {
			queryProperties[parameter.name] = schemaToJSONValue(parameter.schema)
		}
		properties["query"] = map[string]any{"type": "object", "properties": queryProperties}
	}
	if operation.requestType != "" {
		properties["body"] = map[string]any{"$ref": "#/components/schemas/" + operation.requestType}
		if operation.hasBody && operation.method != "GET" && operation.method != "DELETE" {
			required = append(required, "body")
		}
	}
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		sort.Strings(required)
		schema["required"] = compactStrings(required)
	}
	return schema
}

func schemaToJSONValue(schema openAPISchema) any {
	data, _ := json.Marshal(schema)
	var value any
	_ = json.Unmarshal(data, &value)
	return value
}

func renderTypeScriptSDK(spec openAPISpec) map[string]string {
	return map[string]string{
		"package.json":        renderTypeScriptPackageJSON(spec.Info.Version),
		"tsconfig.json":       renderTypeScriptTSConfig(),
		"README.md":           renderTypeScriptREADME(spec.Info.Version),
		"src/index.ts":        renderTypeScriptIndex(spec),
		"src/mcp.ts":          renderTypeScriptMCP(spec),
		"src/react.ts":        renderTypeScriptReactHelper(),
		"src/react-shim.d.ts": renderTypeScriptReactShim(),
	}
}

func renderTypeScriptPackageJSON(version string) string {
	value := map[string]any{
		"name":        "@open-foundry/sdk",
		"version":     version,
		"private":     true,
		"type":        "module",
		"description": "Official TypeScript SDK generated from the OpenFoundry OpenAPI contract.",
		"license":     "AGPL-3.0-only",
		"main":        "./dist/index.js",
		"types":       "./dist/index.d.ts",
		"exports": map[string]any{
			".":       map[string]string{"import": "./dist/index.js", "types": "./dist/index.d.ts"},
			"./react": map[string]string{"import": "./dist/react.js", "types": "./dist/react.d.ts"},
			"./mcp":   map[string]string{"import": "./dist/mcp.js", "types": "./dist/mcp.d.ts"},
		},
		"files": []string{"dist", "src", "README.md"},
		"scripts": map[string]string{
			"build": "tsc -p .",
			"check": "tsc -p . --noEmit",
		},
	}
	data, _ := json.MarshalIndent(value, "", "  ")
	return string(data) + "\n"
}

func renderTypeScriptTSConfig() string {
	value := map[string]any{
		"compilerOptions": map[string]any{
			"target":           "ES2022",
			"module":           "ES2022",
			"moduleResolution": "Bundler",
			"strict":           true,
			"declaration":      true,
			"declarationMap":   false,
			"sourceMap":        false,
			"outDir":           "dist",
			"lib":              []string{"ES2022", "DOM"},
			"skipLibCheck":     true,
		},
		"include": []string{"src/**/*.ts", "src/**/*.d.ts"},
	}
	data, _ := json.MarshalIndent(value, "", "  ")
	return string(data) + "\n"
}

func renderTypeScriptREADME(version string) string {
	return fmt.Sprintf("# OpenFoundry TypeScript SDK\n\nGenerated from `%s`.\n\nVersion: `%s`\n\n## Usage\n\n```ts\nimport { OpenFoundryClient } from '@open-foundry/sdk';\n\nconst client = new OpenFoundryClient({\n  baseUrl: 'https://platform.example.com',\n  token: '<token>',\n  timeoutMs: 15_000,\n  retry: { maxAttempts: 2 },\n});\n\nconst me = await client.auth.getme();\nconst datasets = await client.dataset.listdatasets({ search: 'sales' });\n```\n\n## MCP bridging\n\n```ts\nimport { OPENFOUNDRY_MCP_TOOLS, callOpenFoundryMcpTool } from '@open-foundry/sdk/mcp';\n\nconst result = await callOpenFoundryMcpTool(client, OPENFOUNDRY_MCP_TOOLS[0].name, {\n  query: { page: 1, per_page: 20 },\n});\n```\n\n## React helpers\n\n```ts\nimport { OpenFoundryProvider, useOpenFoundry, useOpenFoundryQuery } from '@open-foundry/sdk/react';\n\nfunction DatasetCount() {\n  const client = useOpenFoundry();\n  const datasets = useOpenFoundryQuery(() => client.dataset.listdatasets(), [client]);\n  return <div>{datasets.data?.datasets?.length ?? 0}</div>;\n}\n\nfunction App() {\n  return (\n    <OpenFoundryProvider options={{ baseUrl: 'https://platform.example.com', token: '<token>' }}>\n      <DatasetCount />\n    </OpenFoundryProvider>\n  );\n}\n```\n", openAPIPath, version)
}

func renderTypeScriptReactHelper() string {
	return strings.Join([]string{
		"// This file is generated by `go run ./tools/of-cli docs generate-sdk-typescript`.",
		"// Do not edit manually.",
		"",
		"import { createContext, createElement, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';",
		"import { OpenFoundryClient, type OpenFoundryClientOptions } from './index';",
		"",
		"export interface OpenFoundryProviderProps {",
		"  client?: OpenFoundryClient;",
		"  options?: OpenFoundryClientOptions;",
		"  children?: ReactNode;",
		"}",
		"",
		"export interface OpenFoundryQueryState<T> {",
		"  data: T | null;",
		"  error: Error | null;",
		"  loading: boolean;",
		"  refetch: () => Promise<T | null>;",
		"}",
		"",
		"export interface OpenFoundryQueryOptions<T> {",
		"  enabled?: boolean;",
		"  initialData?: T | null;",
		"  preservePreviousData?: boolean;",
		"}",
		"",
		"export interface OpenFoundryMutationState<TResult, TArgs extends unknown[]> {",
		"  loading: boolean;",
		"  error: Error | null;",
		"  lastResult: TResult | null;",
		"  mutate: (...args: TArgs) => Promise<TResult>;",
		"  reset: () => void;",
		"}",
		"",
		"export interface OpenFoundryMutationOptions<TResult> {",
		"  onSuccess?: (result: TResult) => void;",
		"  onError?: (error: Error) => void;",
		"}",
		"",
		"const OpenFoundryContext = createContext<OpenFoundryClient | null>(null);",
		"",
		"export function OpenFoundryProvider(props: OpenFoundryProviderProps) {",
		"  const headersKey = stableSerialize(props.options?.headers ?? {});",
		"  const client = useMemo(() => {",
		"    if (props.client) {",
		"      return props.client;",
		"    }",
		"    if (!props.options) {",
		"      throw new Error('OpenFoundryProvider requires either a client or options');",
		"    }",
		"    return new OpenFoundryClient(props.options);",
		"  }, [props.client, props.options?.baseUrl, props.options?.fetch, headersKey]);",
		"  return createElement(OpenFoundryContext.Provider, { value: client }, props.children);",
		"}",
		"",
		"export function useOpenFoundry(): OpenFoundryClient {",
		"  const client = useContext(OpenFoundryContext);",
		"  if (!client) {",
		"    throw new Error('OpenFoundryProvider is missing from the React tree');",
		"  }",
		"  return client;",
		"}",
		"",
		"export function useOpenFoundryClient(options: OpenFoundryClientOptions): OpenFoundryClient {",
		"  const headersKey = stableSerialize(options.headers ?? {});",
		"  return useMemo(",
		"    () => new OpenFoundryClient(options),",
		"    [options.baseUrl, options.fetch, headersKey],",
		"  );",
		"}",
		"",
		"export function useOpenFoundryQuery<T>(",
		"  fetcher: () => Promise<T>,",
		"  deps: readonly unknown[] = [],",
		"  options: OpenFoundryQueryOptions<T> = {},",
		"): OpenFoundryQueryState<T> {",
		"  const fetcherRef = useRef(fetcher);",
		"  fetcherRef.current = fetcher;",
		"  const enabled = options.enabled ?? true;",
		"  const [data, setData] = useState<T | null>(options.initialData ?? null);",
		"  const [error, setError] = useState<Error | null>(null);",
		"  const [loading, setLoading] = useState(enabled);",
		"",
		"  const refetch = async (): Promise<T | null> => {",
		"    if (!enabled) {",
		"      setLoading(false);",
		"      return data;",
		"    }",
		"    setLoading(true);",
		"    setError(null);",
		"    try {",
		"      const result = await fetcherRef.current();",
		"      setData(result);",
		"      return result;",
		"    } catch (cause) {",
		"      const nextError = cause instanceof Error ? cause : new Error('OpenFoundry query failed');",
		"      setError(nextError);",
		"      return null;",
		"    } finally {",
		"      setLoading(false);",
		"    }",
		"  };",
		"",
		"  useEffect(() => {",
		"    if (!enabled) {",
		"      setLoading(false);",
		"      return;",
		"    }",
		"    if (!options.preservePreviousData && options.initialData === undefined) {",
		"      setData(null);",
		"    }",
		"    void refetch();",
		"  }, [enabled, ...deps]);",
		"",
		"  return { data, error, loading, refetch };",
		"}",
		"",
		"export function useOpenFoundryMutation<TResult, TArgs extends unknown[]>(",
		"  mutation: (...args: TArgs) => Promise<TResult>,",
		"  options: OpenFoundryMutationOptions<TResult> = {},",
		"): OpenFoundryMutationState<TResult, TArgs> {",
		"  const mutationRef = useRef(mutation);",
		"  mutationRef.current = mutation;",
		"  const [loading, setLoading] = useState(false);",
		"  const [error, setError] = useState<Error | null>(null);",
		"  const [lastResult, setLastResult] = useState<TResult | null>(null);",
		"",
		"  return {",
		"    loading,",
		"    error,",
		"    lastResult,",
		"    mutate: async (...args: TArgs): Promise<TResult> => {",
		"      setLoading(true);",
		"      setError(null);",
		"      try {",
		"        const result = await mutationRef.current(...args);",
		"        setLastResult(result);",
		"        options.onSuccess?.(result);",
		"        return result;",
		"      } catch (cause) {",
		"        const nextError = cause instanceof Error ? cause : new Error('OpenFoundry mutation failed');",
		"        setError(nextError);",
		"        options.onError?.(nextError);",
		"        throw nextError;",
		"      } finally {",
		"        setLoading(false);",
		"      }",
		"    },",
		"    reset: () => {",
		"      setError(null);",
		"      setLastResult(null);",
		"    },",
		"  };",
		"}",
		"",
		"export function createOpenFoundryQueryKey(...parts: unknown[]): string {",
		"  return stableSerialize(parts);",
		"}",
		"",
		"function stableSerialize(value: unknown): string {",
		"  if (value === null || value === undefined) {",
		"    return '';",
		"  }",
		"  try {",
		"    return JSON.stringify(value, Object.keys(value as Record<string, unknown>).sort());",
		"  } catch (_error) {",
		"    return '';",
		"  }",
		"}",
		"",
	}, "\n")
}

func renderTypeScriptReactShim() string {
	return strings.Join([]string{
		"declare module 'react' {",
		"  export type ReactNode = unknown;",
		"  export type SetStateAction<S> = S | ((previousState: S) => S);",
		"  export type Dispatch<A> = (value: A) => void;",
		"  export interface Context<T> {",
		"    Provider: unknown;",
		"  }",
		"  export function createContext<T>(defaultValue: T): Context<T>;",
		"  export function createElement(type: unknown, props: unknown, ...children: unknown[]): unknown;",
		"  export function useEffect(effect: () => void | (() => void), deps?: readonly unknown[]): void;",
		"  export function useContext<T>(context: Context<T>): T;",
		"  export function useMemo<T>(factory: () => T, deps: readonly unknown[]): T;",
		"  export function useRef<T>(initialValue: T): { current: T };",
		"  export function useState<S>(initialState: S | (() => S)): [S, Dispatch<SetStateAction<S>>];",
		"}",
		"",
	}, "\n")
}

func renderTypeScriptIndex(spec openAPISpec) string {
	operations := collectOperationRenderInfos(spec)
	namespaces := orderedNamespaceProperties(operations)
	lines := []string{
		"// This file is generated by `go run ./tools/of-cli docs generate-sdk-typescript`.",
		"// Do not edit manually.",
		"",
		fmt.Sprintf("export const OPENFOUNDRY_SDK_VERSION = %q;", spec.Info.Version),
		"",
		"export interface OpenFoundryRetryPolicy {",
		"  maxAttempts?: number;",
		"  backoffMs?: number;",
		"  retryOnStatus?: number[];",
		"  retryMethods?: string[];",
		"}",
		"",
		"export interface OpenFoundryClientOptions {",
		"  baseUrl: string;",
		"  fetch?: typeof fetch;",
		"  headers?: HeadersInit;",
		"  token?: string;",
		"  userAgent?: string;",
		"  timeoutMs?: number;",
		"  retry?: OpenFoundryRetryPolicy;",
		"}",
		"",
		"export interface OpenFoundryRequestInit extends Omit<RequestInit, 'body' | 'method' | 'signal'> {",
		"  headers?: HeadersInit;",
		"  timeoutMs?: number;",
		"  retry?: Partial<OpenFoundryRetryPolicy> | false;",
		"}",
		"export type OpenFoundryPathParams = Record<string, string | number | boolean>;",
		"export type OpenFoundryQueryPrimitive = string | number | boolean;",
		"export type OpenFoundryQueryValue = OpenFoundryQueryPrimitive | Array<OpenFoundryQueryPrimitive> | Record<string, unknown> | null | undefined;",
		"export type OpenFoundryQuery = Record<string, OpenFoundryQueryValue>;",
		"export interface OpenFoundryResponse<T> {",
		"  data: T;",
		"  status: number;",
		"  headers: Headers;",
		"  requestId: string | null;",
		"  raw: unknown;",
		"}",
		"export interface OpenFoundryOperationInput {",
		"  path?: OpenFoundryPathParams;",
		"  query?: OpenFoundryQuery;",
		"  body?: unknown;",
		"}",
		"export interface OpenFoundryOperationMeta {",
		"  operationId: string;",
		"  method: string;",
		"  path: string;",
		"  summary: string;",
		"  description?: string;",
		"  namespace: string;",
		"  namespaceMember: string;",
		"  apiVersion?: string;",
		"  stability?: string;",
		"  mcpTool: string;",
		"}",
		"",
		"export class OpenFoundryApiError extends Error {",
		"  readonly status: number;",
		"  readonly method: string;",
		"  readonly path: string;",
		"  readonly requestId: string | null;",
		"  readonly body: unknown;",
		"  readonly rawMessage: string;",
		"",
		"  constructor(input: { status: number; method: string; path: string; message: string; requestId?: string | null; body?: unknown; rawMessage?: string }) {",
		"    super(input.message);",
		"    this.name = 'OpenFoundryApiError';",
		"    this.status = input.status;",
		"    this.method = input.method;",
		"    this.path = input.path;",
		"    this.requestId = input.requestId ?? null;",
		"    this.body = input.body;",
		"    this.rawMessage = input.rawMessage ?? input.message;",
		"  }",
		"}",
		"",
	}

	for _, name := range sortedStringKeys(spec.Components.Schemas) {
		lines = append(lines, renderSchemaDeclaration(name, spec.Components.Schemas[name])...)
		lines = append(lines, "")
	}
	for _, name := range collectReferencedSchemaNames(spec) {
		if _, ok := spec.Components.Schemas[name]; !ok {
			lines = append(lines, fmt.Sprintf("export type %s = %s;", typescriptExportName(name), fallbackTypeScriptType(name)), "")
		}
	}

	lines = append(lines, "export const OPENFOUNDRY_OPERATION_REGISTRY: ReadonlyArray<OpenFoundryOperationMeta> = [")
	for _, operation := range operations {
		lines = append(lines,
			"  {",
			fmt.Sprintf("    operationId: %q,", operation.operationID),
			fmt.Sprintf("    method: %q,", operation.method),
			fmt.Sprintf("    path: %q,", operation.path),
			fmt.Sprintf("    summary: %q,", operation.summary),
		)
		if operation.description != "" {
			lines = append(lines, fmt.Sprintf("    description: %q,", operation.description))
		}
		lines = append(lines,
			fmt.Sprintf("    namespace: %q,", operation.namespaceProperty),
			fmt.Sprintf("    namespaceMember: %q,", operation.namespaceMemberName),
		)
		if operation.apiVersion != "" {
			lines = append(lines, fmt.Sprintf("    apiVersion: %q,", operation.apiVersion))
		}
		if operation.stability != "" {
			lines = append(lines, fmt.Sprintf("    stability: %q,", operation.stability))
		}
		lines = append(lines, fmt.Sprintf("    mcpTool: %q,", operation.mcpToolName), "  },")
	}
	lines = append(lines,
		"];",
		"",
		"export class OpenFoundryClient {",
		"  private readonly baseUrl: string;",
		"  private readonly fetchImpl: typeof fetch;",
		"  private readonly defaultHeaders: Headers;",
		"  private token?: string;",
		"  private readonly userAgent?: string;",
		"  private readonly timeoutMs: number;",
		"  private readonly retryPolicy: OpenFoundryRetryPolicy;",
		"",
		"  constructor(options: OpenFoundryClientOptions) {",
		"    this.baseUrl = options.baseUrl.replace(/\\/$/, '');",
		"    this.fetchImpl = options.fetch ?? fetch;",
		"    this.defaultHeaders = new Headers(options.headers ?? {});",
		"    this.token = options.token;",
		"    this.userAgent = options.userAgent;",
		"    this.timeoutMs = options.timeoutMs ?? 30_000;",
		"    this.retryPolicy = {",
		"      maxAttempts: options.retry?.maxAttempts ?? 1,",
		"      backoffMs: options.retry?.backoffMs ?? 250,",
		"      retryOnStatus: options.retry?.retryOnStatus ?? [408, 429, 500, 502, 503, 504],",
		"      retryMethods: options.retry?.retryMethods ?? ['GET', 'HEAD', 'OPTIONS'],",
		"    };",
		"  }",
		"",
		"  clone(overrides: Partial<OpenFoundryClientOptions> = {}): OpenFoundryClient {",
		"    return new OpenFoundryClient({",
		"      baseUrl: overrides.baseUrl ?? this.baseUrl,",
		"      fetch: overrides.fetch ?? this.fetchImpl,",
		"      headers: overrides.headers ?? (() => { const copied: Record<string, string> = {}; this.defaultHeaders.forEach((value, key) => { copied[key] = value; }); return copied; })(),",
		"      token: overrides.token ?? this.token,",
		"      userAgent: overrides.userAgent ?? this.userAgent,",
		"      timeoutMs: overrides.timeoutMs ?? this.timeoutMs,",
		"      retry: overrides.retry ?? this.retryPolicy,",
		"    });",
		"  }",
		"",
		"  withBearerToken(token: string): OpenFoundryClient {",
		"    return this.clone({ token });",
		"  }",
		"",
		"  setBearerToken(token: string | undefined): void {",
		"    this.token = token;",
		"  }",
		"",
		"  setDefaultHeader(name: string, value: string): void {",
		"    this.defaultHeaders.set(name, value);",
		"  }",
		"",
		"  removeDefaultHeader(name: string): void {",
		"    this.defaultHeaders.delete(name);",
		"  }",
		"",
	)

	for _, namespace := range namespaces {
		lines = append(lines, fmt.Sprintf("  readonly %s = {", namespace))
		for _, operation := range operations {
			if operation.namespaceProperty != namespace {
				continue
			}
			requestSignature := renderTypeScriptMethodSignatureOwned(operation.pathParameters, operation.queryParameters, bodyRequestType(operation))
			lines = append(lines, fmt.Sprintf("    %s: (%s) => this.%s(%s),", operation.namespaceMemberName, requestSignature, operation.flatMethodName, renderTypeScriptWrapperCallArguments(operation)))
		}
		lines = append(lines, "  } as const;", "")
	}
	for _, operation := range operations {
		requestSignature := renderTypeScriptMethodSignatureOwned(operation.pathParameters, operation.queryParameters, bodyRequestType(operation))
		pathArgument := "undefined"
		if len(operation.pathParameters) > 0 {
			pathArgument = renderTypeScriptPathArgumentOwned(operation.pathParameters)
		}
		queryArgument := "undefined"
		if len(operation.queryParameters) > 0 {
			queryArgument = "query as OpenFoundryQuery"
		}
		bodyArgument := "undefined"
		if operation.hasBody {
			bodyArgument = "body"
		}
		lines = append(lines,
			fmt.Sprintf("  async %s(%s): Promise<%s> {", operation.flatMethodName, requestSignature, operation.responseType),
			fmt.Sprintf("    return this.request<%s>(%q, %q, %s, %s, %s, init);", operation.responseType, operation.method, operation.path, pathArgument, queryArgument, bodyArgument),
			"  }",
			"",
		)
	}
	lines = append(lines,
		"  async callOperation(",
		"    operationId: string,",
		"    input: OpenFoundryOperationInput = {}, ",
		"    init: OpenFoundryRequestInit = {}, ",
		"  ): Promise<unknown> {",
		"    switch (operationId) {",
	)
	for _, operation := range operations {
		lines = append(lines, fmt.Sprintf("      case %q:", operation.operationID), fmt.Sprintf("        return this.%s(%s);", operation.flatMethodName, renderTypeScriptOperationCallArguments(operation)))
	}
	lines = append(lines,
		"      default:",
		"        throw new Error(`Unknown OpenFoundry operation: ${operationId}`);",
		"    }",
		"  }",
		"",
	)
	lines = append(lines, renderTypeScriptClientTail()...)
	return strings.Join(lines, "\n")
}

func renderTypeScriptClientTail() []string {
	return []string{
		"  async request<TResponse>(",
		"    method: string,",
		"    pathTemplate: string,",
		"    pathParams: OpenFoundryPathParams | undefined,",
		"    query: OpenFoundryQuery | undefined,",
		"    body: unknown,",
		"    init: OpenFoundryRequestInit = {}, ",
		"  ): Promise<TResponse> {",
		"    const response = await this.requestRaw<TResponse>(method, pathTemplate, pathParams, query, body, init);",
		"    return response.data;",
		"  }",
		"",
		"  async requestRaw<TResponse>(",
		"    method: string,",
		"    pathTemplate: string,",
		"    pathParams: OpenFoundryPathParams | undefined,",
		"    query: OpenFoundryQuery | undefined,",
		"    body: unknown,",
		"    init: OpenFoundryRequestInit = {}, ",
		"  ): Promise<OpenFoundryResponse<TResponse>> {",
		"    const path = this.interpolatePath(pathTemplate, pathParams);",
		"    const url = new URL(`${this.baseUrl}${path}`);",
		"    if (query) {",
		"      for (const [key, value] of Object.entries(query)) {",
		"        this.appendQueryParam(url, key, value);",
		"      }",
		"    }",
		"    const retryPolicy = this.mergeRetryPolicy(init.retry);",
		"    const maxAttempts = retryPolicy.maxAttempts ?? 1;",
		"    let attempt = 0;",
		"    while (attempt < maxAttempts) {",
		"      attempt += 1;",
		"      const controller = typeof AbortController !== 'undefined' ? new AbortController() : undefined;",
		"      const timeoutMs = init.timeoutMs ?? this.timeoutMs;",
		"      const timeoutHandle = controller && timeoutMs > 0 ? setTimeout(() => controller.abort(), timeoutMs) : undefined;",
		"      try {",
		"        const headers = this.buildHeaders(init.headers, body !== undefined);",
		"        const response = await this.fetchImpl(url.toString(), {",
		"          ...init,",
		"          method,",
		"          headers,",
		"          signal: controller?.signal,",
		"          body: body === undefined ? undefined : JSON.stringify(body),",
		"        });",
		"        const raw = await this.parseResponsePayload(response);",
		"        const requestId = response.headers.get('x-request-id');",
		"        if (!response.ok) {",
		"          throw new OpenFoundryApiError({",
		"            status: response.status,",
		"            method,",
		"            path,",
		"            requestId,",
		"            body: raw,",
		"            rawMessage: typeof raw === 'string' ? raw : JSON.stringify(raw ?? null),",
		"            message: this.errorMessageFromPayload(raw, response.statusText),",
		"          });",
		"        }",
		"        return {",
		"          data: raw as TResponse,",
		"          status: response.status,",
		"          headers: response.headers,",
		"          requestId,",
		"          raw,",
		"        };",
		"      } catch (cause) {",
		"        const error = cause instanceof OpenFoundryApiError",
		"          ? cause",
		"          : new OpenFoundryApiError({",
		"              status: 0,",
		"              method,",
		"              path,",
		"              message: cause instanceof Error ? cause.message : 'OpenFoundry network request failed',",
		"              rawMessage: cause instanceof Error ? cause.message : String(cause),",
		"            });",
		"        if (attempt >= maxAttempts || !this.shouldRetry(method, error.status, retryPolicy)) {",
		"          throw error;",
		"        }",
		"        await this.wait((retryPolicy.backoffMs ?? 250) * attempt);",
		"      } finally {",
		"        if (timeoutHandle !== undefined) {",
		"          clearTimeout(timeoutHandle);",
		"        }",
		"      }",
		"    }",
		"    throw new OpenFoundryApiError({ status: 0, method, path, message: 'OpenFoundry request exhausted retries' });",
		"  }",
		"",
		"  private buildHeaders(headers: HeadersInit | undefined, hasJsonBody: boolean): Headers {",
		"    const merged = new Headers(this.defaultHeaders);",
		"    if (headers) {",
		"      new Headers(headers).forEach((value, key) => merged.set(key, value));",
		"    }",
		"    if (this.token && !merged.has('authorization')) {",
		"      merged.set('authorization', `Bearer ${this.token}`);",
		"    }",
		"    if (this.userAgent && !merged.has('x-openfoundry-client')) {",
		"      merged.set('x-openfoundry-client', this.userAgent);",
		"    }",
		"    if (hasJsonBody && !merged.has('content-type')) {",
		"      merged.set('content-type', 'application/json');",
		"    }",
		"    return merged;",
		"  }",
		"",
		"  private appendQueryParam(url: URL, key: string, value: OpenFoundryQueryValue): void {",
		"    for (const entry of this.serializeQueryValue(value)) {",
		"      url.searchParams.append(key, entry);",
		"    }",
		"  }",
		"",
		"  private serializeQueryValue(value: OpenFoundryQueryValue): string[] {",
		"    if (value === undefined || value === null) {",
		"      return [];",
		"    }",
		"    if (Array.isArray(value)) {",
		"      return value.map((item) => String(item));",
		"    }",
		"    if (typeof value === 'object') {",
		"      return [JSON.stringify(value)];",
		"    }",
		"    return [String(value)];",
		"  }",
		"",
		"  private mergeRetryPolicy(override: Partial<OpenFoundryRetryPolicy> | false | undefined): OpenFoundryRetryPolicy {",
		"    if (override === false) {",
		"      return { maxAttempts: 1, backoffMs: 0, retryOnStatus: [], retryMethods: [] };",
		"    }",
		"    return {",
		"      maxAttempts: override?.maxAttempts ?? this.retryPolicy.maxAttempts ?? 1,",
		"      backoffMs: override?.backoffMs ?? this.retryPolicy.backoffMs ?? 250,",
		"      retryOnStatus: override?.retryOnStatus ?? this.retryPolicy.retryOnStatus ?? [408, 429, 500, 502, 503, 504],",
		"      retryMethods: override?.retryMethods ?? this.retryPolicy.retryMethods ?? ['GET', 'HEAD', 'OPTIONS'],",
		"    };",
		"  }",
		"",
		"  private shouldRetry(method: string, status: number, policy: OpenFoundryRetryPolicy): boolean {",
		"    const retryMethods = new Set((policy.retryMethods ?? []).map((entry) => entry.toUpperCase()));",
		"    const retryOnStatus = new Set(policy.retryOnStatus ?? []);",
		"    return retryMethods.has(method.toUpperCase()) && (status === 0 || retryOnStatus.has(status));",
		"  }",
		"",
		"  private async parseResponsePayload(response: Response): Promise<unknown> {",
		"    if (response.status === 204) {",
		"      return undefined;",
		"    }",
		"    const text = await response.text();",
		"    if (!text) {",
		"      return undefined;",
		"    }",
		"    try {",
		"      return JSON.parse(text) as unknown;",
		"    } catch (_error) {",
		"      return text;",
		"    }",
		"  }",
		"",
		"  private errorMessageFromPayload(payload: unknown, fallback: string): string {",
		"    if (typeof payload === 'string' && payload.trim()) {",
		"      return payload;",
		"    }",
		"    if (payload && typeof payload === 'object') {",
		"      const record = payload as Record<string, unknown>;",
		"      for (const key of ['message', 'error', 'detail', 'code']) {",
		"        const value = record[key];",
		"        if (typeof value === 'string' && value.trim()) {",
		"          return value;",
		"        }",
		"      }",
		"    }",
		"    return fallback || 'OpenFoundry request failed';",
		"  }",
		"",
		"  private interpolatePath(pathTemplate: string, pathParams: OpenFoundryPathParams | undefined): string {",
		"    if (!pathParams) {",
		"      return pathTemplate;",
		"    }",
		"    return Object.entries(pathParams).reduce(",
		"      (path, [key, value]) => path.replace(`{${key}}`, encodeURIComponent(String(value))),",
		"      pathTemplate,",
		"    );",
		"  }",
		"",
		"  private requiredPathParam(input: OpenFoundryOperationInput, name: string): string | number | boolean {",
		"    const value = input.path?.[name];",
		"    if (value === undefined || value === null) {",
		"      throw new Error(`Missing required path parameter: ${name}`);",
		"    }",
		"    return value;",
		"  }",
		"",
		"  private resolveBodyInput(input: OpenFoundryOperationInput): unknown {",
		"    if (input.body !== undefined) {",
		"      return input.body;",
		"    }",
		"    if (input.path || input.query) {",
		"      return undefined;",
		"    }",
		"    return input;",
		"  }",
		"",
		"  private wait(ms: number): Promise<void> {",
		"    return new Promise((resolve) => setTimeout(resolve, ms));",
		"  }",
		"}",
	}
}

func renderTypeScriptMCP(spec openAPISpec) string {
	operations := collectOperationRenderInfos(spec)
	lines := []string{
		"// This file is generated by `go run ./tools/of-cli docs generate-sdk-typescript`.",
		"// Do not edit manually.",
		"",
		"import {",
		"  OpenFoundryClient,",
		"  type OpenFoundryOperationInput,",
		"  type OpenFoundryRequestInit,",
		"} from './index';",
		"",
		"export interface OpenFoundryMcpTool {",
		"  name: string;",
		"  description: string;",
		"  operationId: string;",
		"  method: string;",
		"  path: string;",
		"  namespace: string;",
		"  namespaceMember: string;",
		"  inputSchema: Record<string, unknown>;",
		"  apiVersion?: string;",
		"  stability?: string;",
		"}",
		"",
		"export const OPENFOUNDRY_MCP_TOOLS: ReadonlyArray<OpenFoundryMcpTool> = [",
	}
	for _, operation := range operations {
		lines = append(lines,
			"  {",
			fmt.Sprintf("    name: %q,", operation.mcpToolName),
			fmt.Sprintf("    description: %q,", fallbackString(operation.description, operation.summary)),
			fmt.Sprintf("    operationId: %q,", operation.operationID),
			fmt.Sprintf("    method: %q,", operation.method),
			fmt.Sprintf("    path: %q,", operation.path),
			fmt.Sprintf("    namespace: %q,", operation.namespaceProperty),
			fmt.Sprintf("    namespaceMember: %q,", operation.namespaceMemberName),
			"    inputSchema:",
		)
		for _, line := range strings.Split(marshalPretty(buildMCPInputSchemaValue(operation)), "\n") {
			lines = append(lines, "      "+line)
		}
		lines = append(lines, "    ,")
		if operation.apiVersion != "" {
			lines = append(lines, fmt.Sprintf("    apiVersion: %q,", operation.apiVersion))
		}
		if operation.stability != "" {
			lines = append(lines, fmt.Sprintf("    stability: %q,", operation.stability))
		}
		lines = append(lines, "  },")
	}
	lines = append(lines,
		"];",
		"",
		"export function listOpenFoundryMcpTools(): ReadonlyArray<OpenFoundryMcpTool> {",
		"  return OPENFOUNDRY_MCP_TOOLS;",
		"}",
		"",
		"export async function callOpenFoundryMcpTool(",
		"  client: OpenFoundryClient,",
		"  toolName: string,",
		"  input: OpenFoundryOperationInput = {}, ",
		"  init: OpenFoundryRequestInit = {}, ",
		"): Promise<unknown> {",
		"  const tool = OPENFOUNDRY_MCP_TOOLS.find((entry) => entry.name === toolName);",
		"  if (!tool) {",
		"    throw new Error(`Unknown OpenFoundry MCP tool: ${toolName}`);",
		"  }",
		"  return client.callOperation(tool.operationId, input, init);",
		"}",
	)
	return strings.Join(lines, "\n")
}

func renderSchemaDeclaration(name string, schema openAPISchema) []string {
	exportName := typescriptExportName(name)
	if isObjectSchema(schema) {
		lines := []string{fmt.Sprintf("export interface %s {", exportName)}
		if schema.Properties != nil {
			for _, propertyName := range sortedSchemaKeys(*schema.Properties) {
				lines = append(lines, fmt.Sprintf("  %s?: %s;", renderPropertyName(propertyName), typescriptType((*schema.Properties)[propertyName])))
			}
		}
		if schema.AdditionalProperties != nil {
			lines = append(lines, fmt.Sprintf("  [key: string]: %s;", typescriptType(*schema.AdditionalProperties)))
		}
		return append(lines, "}")
	}
	return []string{fmt.Sprintf("export type %s = %s;", exportName, typescriptType(schema))}
}

func typescriptType(schema openAPISchema) string {
	if schema.Ref != "" {
		return typescriptExportName(lastRefSegment(schema.Ref))
	}
	switch schema.Type {
	case "string":
		return "string"
	case "integer", "number":
		return "number"
	case "boolean":
		return "boolean"
	case "array":
		if schema.Items == nil {
			return "Array<unknown>"
		}
		return fmt.Sprintf("Array<%s>", typescriptType(*schema.Items))
	case "object":
		if schema.AdditionalProperties != nil {
			return fmt.Sprintf("Record<string, %s>", typescriptType(*schema.AdditionalProperties))
		}
		if schema.Properties != nil {
			fields := []string{}
			for _, name := range sortedSchemaKeys(*schema.Properties) {
				fields = append(fields, fmt.Sprintf("%s?: %s", renderPropertyName(name), typescriptType((*schema.Properties)[name])))
			}
			return fmt.Sprintf("{ %s }", strings.Join(fields, "; "))
		}
		return "Record<string, unknown>"
	default:
		return "unknown"
	}
}

func collectReferencedSchemaNames(spec openAPISpec) []string {
	names := map[string]bool{}
	for _, name := range sortedStringKeys(spec.Components.Schemas) {
		collectReferencesFromSchema(spec.Components.Schemas[name], names)
	}
	for _, path := range sortedStringKeys(spec.Paths) {
		for _, method := range sortedStringKeys(spec.Paths[path]) {
			operation := spec.Paths[path][method]
			if operation.RequestBody != nil {
				for _, key := range sortedStringKeys(operation.RequestBody.Content) {
					collectReferencesFromSchema(operation.RequestBody.Content[key].Schema, names)
				}
			}
			for _, status := range sortedStringKeys(operation.Responses) {
				for _, key := range sortedStringKeys(operation.Responses[status].Content) {
					collectReferencesFromSchema(operation.Responses[status].Content[key].Schema, names)
				}
			}
		}
	}
	return sortedBoolKeys(names)
}

func collectReferencesFromSchema(schema openAPISchema, names map[string]bool) {
	if schema.Ref != "" {
		names[lastRefSegment(schema.Ref)] = true
	}
	if schema.Properties != nil {
		for _, name := range sortedSchemaKeys(*schema.Properties) {
			collectReferencesFromSchema((*schema.Properties)[name], names)
		}
	}
	if schema.Items != nil {
		collectReferencesFromSchema(*schema.Items, names)
	}
	if schema.AdditionalProperties != nil {
		collectReferencesFromSchema(*schema.AdditionalProperties, names)
	}
}

func renderTypeScriptMethodSignatureOwned(pathParameters, queryParameters []ownedParameter, requestType string) string {
	parts := []string{}
	for _, parameter := range pathParameters {
		parts = append(parts, fmt.Sprintf("%s: %s", renderTypeScriptVariableName(parameter.name), typescriptType(parameter.schema)))
	}
	if len(queryParameters) > 0 {
		parts = append(parts, fmt.Sprintf("query: { %s } = {}", renderTypeScriptQueryFields(queryParameters)))
	}
	if requestType != "" {
		parts = append(parts, fmt.Sprintf("body: %s", requestType))
	}
	parts = append(parts, "init: OpenFoundryRequestInit = {}")
	return strings.Join(parts, ", ")
}

func renderTypeScriptQueryFields(parameters []ownedParameter) string {
	fields := []string{}
	for _, parameter := range parameters {
		optional := "?"
		if parameter.required {
			optional = ""
		}
		fields = append(fields, fmt.Sprintf("%s%s: %s", renderPropertyName(parameter.name), optional, typescriptType(parameter.schema)))
	}
	return strings.Join(fields, "; ")
}

func renderTypeScriptPathArgumentOwned(pathParameters []ownedParameter) string {
	fields := []string{}
	for _, parameter := range pathParameters {
		fields = append(fields, fmt.Sprintf("%q: %s", parameter.name, renderTypeScriptVariableName(parameter.name)))
	}
	return fmt.Sprintf("{ %s }", strings.Join(fields, ", "))
}

func renderTypeScriptOperationCallArguments(operation operationRenderInfo) string {
	parts := []string{}
	for _, parameter := range operation.pathParameters {
		parts = append(parts, fmt.Sprintf("(input.path?.%s ?? this.requiredPathParam(input, %q)) as any", renderPropertyName(parameter.name), parameter.name))
	}
	if len(operation.queryParameters) > 0 {
		parts = append(parts, "((input.query ?? {}) as any)")
	}
	if operation.hasBody {
		if len(operation.pathParameters) == 0 && len(operation.queryParameters) == 0 {
			parts = append(parts, "this.resolveBodyInput(input) as any")
		} else {
			parts = append(parts, "(input.body as any)")
		}
	}
	parts = append(parts, "init")
	return strings.Join(parts, ", ")
}

func renderTypeScriptWrapperCallArguments(operation operationRenderInfo) string {
	parts := []string{}
	for _, parameter := range operation.pathParameters {
		parts = append(parts, renderTypeScriptVariableName(parameter.name))
	}
	if len(operation.queryParameters) > 0 {
		parts = append(parts, "query")
	}
	if operation.hasBody {
		parts = append(parts, "body")
	}
	parts = append(parts, "init")
	return strings.Join(parts, ", ")
}

func bodyRequestType(operation operationRenderInfo) string {
	if operation.hasBody {
		return operation.requestType
	}
	return ""
}

func renderPythonSDK(spec openAPISpec) map[string]string {
	return map[string]string{
		"pyproject.toml":              renderPythonPyproject(spec.Info.Version),
		"README.md":                   renderPythonREADME(spec.Info.Version),
		"openfoundry_sdk/__init__.py": renderPythonInit(),
		"openfoundry_sdk/models.py":   renderPythonModels(spec),
		"openfoundry_sdk/client.py":   renderPythonClient(spec),
		"openfoundry_sdk/mcp.py":      renderPythonMCP(spec),
	}
}

func renderPythonPyproject(version string) string {
	return fmt.Sprintf("[build-system]\nrequires = [\"setuptools>=68\"]\nbuild-backend = \"setuptools.build_meta\"\n\n[project]\nname = \"openfoundry-sdk\"\nversion = %q\ndescription = \"Official Python SDK generated from the OpenFoundry OpenAPI contract.\"\nreadme = \"README.md\"\nlicense = { text = \"Apache-2.0\" }\nrequires-python = \">=3.11\"\ndependencies = []\n\n[tool.setuptools.packages.find]\ninclude = [\"openfoundry_sdk*\"]\n", version)
}

func renderPythonREADME(version string) string {
	return fmt.Sprintf("# OpenFoundry Python SDK\n\nGenerated from `%s`.\n\nVersion: `%s`\n\n## Usage\n\n```python\nfrom openfoundry_sdk import OpenFoundryClient\n\nclient = OpenFoundryClient(\n    base_url=\"https://platform.example.com\",\n    token=\"<token>\",\n    timeout_seconds=15,\n    max_retries=2,\n)\n\nme = client.auth.auth_get_me()\ndatasets = client.dataset.listdatasets({\"search\": \"sales\"})\n```\n\n## MCP bridging\n\n```python\nfrom openfoundry_sdk.mcp import MCP_TOOL_REGISTRY, call_openfoundry_mcp_tool\n\nresult = call_openfoundry_mcp_tool(client, MCP_TOOL_REGISTRY[0][\"name\"], {\"query\": {\"page\": 1, \"per_page\": 20}})\n```\n", openAPIPath, version)
}

func renderPythonInit() string {
	return strings.Join([]string{
		"# This file is generated by `go run ./tools/of-cli docs generate-sdk-python`.",
		"# Do not edit manually.",
		"",
		"from .client import OpenFoundryClient",
		"from . import models",
		"from . import mcp",
		"",
		"__all__ = [\"OpenFoundryClient\", \"models\", \"mcp\"]",
		"",
	}, "\n")
}

func renderPythonModels(spec openAPISpec) string {
	lines := []string{
		"# This file is generated by `go run ./tools/of-cli docs generate-sdk-python`.",
		"# Do not edit manually.",
		"from __future__ import annotations",
		"",
		"import dataclasses",
		"from dataclasses import dataclass",
		"from typing import Any, get_args, get_origin",
		"",
		"JsonValue = Any",
		"",
		"def serialize_model(value: Any) -> Any:",
		"    if value is None:",
		"        return None",
		"    if dataclasses.is_dataclass(value):",
		"        return {field.name: serialize_model(getattr(value, field.name)) for field in dataclasses.fields(value) if getattr(value, field.name) is not None}",
		"    if isinstance(value, list):",
		"        return [serialize_model(item) for item in value]",
		"    if isinstance(value, dict):",
		"        return {key: serialize_model(item) for key, item in value.items() if item is not None}",
		"    return value",
		"",
		"def deserialize_model(model_type: Any, value: Any) -> Any:",
		"    if value is None:",
		"        return None",
		"    origin = get_origin(model_type)",
		"    if origin is list:",
		"        args = get_args(model_type)",
		"        item_type = args[0] if args else Any",
		"        return [deserialize_model(item_type, item) for item in value]",
		"    if origin is dict:",
		"        args = get_args(model_type)",
		"        value_type = args[1] if len(args) == 2 else Any",
		"        return {str(key): deserialize_model(value_type, item) for key, item in value.items()}",
		"    if origin is not None and str(origin).endswith('Union') or origin is getattr(__import__('types'), 'UnionType', object()):",
		"        args = [candidate for candidate in get_args(model_type) if candidate is not type(None)]",
		"        if len(args) == 1:",
		"            return deserialize_model(args[0], value)",
		"        return value",
		"    if model_type in (Any, str, int, float, bool):",
		"        return value",
		"    if isinstance(model_type, type) and dataclasses.is_dataclass(model_type) and isinstance(value, dict):",
		"        payload = {}",
		"        for field in dataclasses.fields(model_type):",
		"            payload[field.name] = deserialize_model(field.type, value.get(field.name))",
		"        return model_type(**payload)",
		"    return value",
		"",
	}
	for _, name := range sortedStringKeys(spec.Components.Schemas) {
		lines = append(lines, renderPythonSchemaDeclaration(name, spec.Components.Schemas[name])...)
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func renderPythonClient(spec openAPISpec) string {
	operations := collectOperationRenderInfos(spec)
	namespaces := orderedNamespaceProperties(operations)
	lines := []string{
		"# This file is generated by `go run ./tools/of-cli docs generate-sdk-python`.",
		"# Do not edit manually.",
		"from __future__ import annotations",
		"",
		"import json",
		"import time",
		"import urllib.error",
		"import urllib.parse",
		"import urllib.request",
		"from typing import Any, Mapping",
		"",
		"from . import models",
		"",
		fmt.Sprintf("OPENFOUNDRY_SDK_VERSION = %q", spec.Info.Version),
		"",
		"class OpenFoundryApiError(RuntimeError):",
		"    pass",
		"",
		"class _OperationNamespace:",
		"    def __init__(self, **methods: Any) -> None:",
		"        for name, method in methods.items():",
		"            setattr(self, name, method)",
		"",
		"class OpenFoundryClient:",
		"    def __init__(self, base_url: str, headers: Mapping[str, str] | None = None, token: str | None = None, timeout_seconds: float = 30.0, max_retries: int = 1, retry_backoff_seconds: float = 0.25, user_agent: str | None = None) -> None:",
		"        self.base_url = base_url.rstrip('/')",
		"        self.default_headers = dict(headers or {})",
		"        self.token = token",
		"        self.timeout_seconds = timeout_seconds",
		"        self.max_retries = max(1, max_retries)",
		"        self.retry_backoff_seconds = retry_backoff_seconds",
		"        self.user_agent = user_agent",
	}
	for _, namespace := range namespaces {
		assignments := []string{}
		for _, operation := range operations {
			if operation.namespaceProperty == namespace {
				assignments = append(assignments, fmt.Sprintf("%s=self.%s", operation.namespaceMemberName, pythonMethodName(operation.flatMethodName)))
			}
		}
		lines = append(lines, fmt.Sprintf("        self.%s = _OperationNamespace(%s)", namespace, strings.Join(assignments, ", ")))
	}
	lines = append(lines,
		"",
		"    def clone(self, **overrides: Any) -> \"OpenFoundryClient\":",
		"        return OpenFoundryClient(base_url=overrides.get(\"base_url\", self.base_url), headers=overrides.get(\"headers\", self.default_headers), token=overrides.get(\"token\", self.token), timeout_seconds=overrides.get(\"timeout_seconds\", self.timeout_seconds), max_retries=overrides.get(\"max_retries\", self.max_retries), retry_backoff_seconds=overrides.get(\"retry_backoff_seconds\", self.retry_backoff_seconds), user_agent=overrides.get(\"user_agent\", self.user_agent))",
		"",
		"    def with_bearer_token(self, token: str) -> \"OpenFoundryClient\":",
		"        return self.clone(token=token)",
		"",
	)
	for _, operation := range operations {
		signature := renderPythonMethodSignatureOwned(operation.pathParameters, operation.queryParameters, mapBodyType(operation))
		pathArgument := "None"
		if len(operation.pathParameters) > 0 {
			pathFields := []string{}
			for _, parameter := range operation.pathParameters {
				pathFields = append(pathFields, fmt.Sprintf("%q: %s", parameter.name, renderPythonVariableName(parameter.name)))
			}
			pathArgument = "{" + strings.Join(pathFields, ", ") + "}"
		}
		queryArgument := "None"
		if len(operation.queryParameters) > 0 {
			queryFields := []string{}
			for _, parameter := range operation.queryParameters {
				queryFields = append(queryFields, fmt.Sprintf("%q: %s", parameter.name, renderPythonVariableName(parameter.name)))
			}
			queryArgument = "{" + strings.Join(queryFields, ", ") + "}"
		}
		bodyArgument := "None"
		if operation.hasBody {
			bodyArgument = "body"
		}
		lines = append(lines,
			fmt.Sprintf("    def %s(%s) -> Any:", pythonMethodName(operation.flatMethodName), signature),
			fmt.Sprintf("        return self._request(%q, %q, %s, %s, %s, headers=headers)", operation.method, operation.path, pathArgument, queryArgument, bodyArgument),
			"",
		)
	}
	lines = append(lines,
		"    def call_operation(self, operation_id: str, input: Mapping[str, Any] | None = None, headers: Mapping[str, str] | None = None) -> Any:",
		"        payload = dict(input or {})",
		"        match operation_id:",
	)
	for _, operation := range operations {
		lines = append(lines, fmt.Sprintf("            case %q:", operation.operationID), fmt.Sprintf("                return self.%s(%s)", pythonMethodName(operation.flatMethodName), renderPythonOperationCallArguments(operation)))
	}
	lines = append(lines,
		"            case _:",
		"                raise ValueError(f\"Unknown OpenFoundry operation: {operation_id}\")",
		"",
		"    def _request(self, method: str, path_template: str, path_params: Mapping[str, Any] | None = None, query: Mapping[str, Any] | None = None, body: Any = None, headers: Mapping[str, str] | None = None) -> Any:",
		"        path = self._interpolate_path(path_template, path_params)",
		"        url = self._build_url(path, query)",
		"        request_headers = dict(self.default_headers)",
		"        if headers:",
		"            request_headers.update(dict(headers))",
		"        if self.token and \"authorization\" not in {key.lower() for key in request_headers}:",
		"            request_headers[\"authorization\"] = f\"Bearer {self.token}\"",
		"        if self.user_agent and \"x-openfoundry-client\" not in {key.lower() for key in request_headers}:",
		"            request_headers[\"x-openfoundry-client\"] = self.user_agent",
		"        payload = None",
		"        if body is not None:",
		"            request_headers.setdefault(\"content-type\", \"application/json\")",
		"            payload = json.dumps(models.serialize_model(body)).encode(\"utf-8\")",
		"        attempt = 0",
		"        while attempt < self.max_retries:",
		"            attempt += 1",
		"            request = urllib.request.Request(url=url, data=payload, method=method, headers=request_headers)",
		"            try:",
		"                with urllib.request.urlopen(request, timeout=self.timeout_seconds) as response:",
		"                    raw = response.read()",
		"                    return self._parse_payload(raw) if response.status != 204 else None",
		"            except urllib.error.HTTPError as error:",
		"                if attempt >= self.max_retries or error.code not in {408, 429, 500, 502, 503, 504}:",
		"                    raise OpenFoundryApiError(error.read().decode(\"utf-8\", errors=\"replace\")) from error",
		"                time.sleep(self.retry_backoff_seconds * attempt)",
		"            except urllib.error.URLError as error:",
		"                if attempt >= self.max_retries:",
		"                    raise OpenFoundryApiError(str(error)) from error",
		"                time.sleep(self.retry_backoff_seconds * attempt)",
		"        raise OpenFoundryApiError(\"OpenFoundry request exhausted retries\")",
		"",
		"    def _build_url(self, path: str, query: Mapping[str, Any] | None) -> str:",
		"        if not query:",
		"            return f\"{self.base_url}{path}\"",
		"        pairs: list[tuple[str, str]] = []",
		"        for key, value in query.items():",
		"            if value is None:",
		"                continue",
		"            if isinstance(value, (list, tuple)):",
		"                pairs.extend((key, str(item)) for item in value)",
		"            elif isinstance(value, dict):",
		"                pairs.append((key, json.dumps(models.serialize_model(value))))",
		"            else:",
		"                pairs.append((key, str(value)))",
		"        return f\"{self.base_url}{path}?{urllib.parse.urlencode(pairs)}\" if pairs else f\"{self.base_url}{path}\"",
		"",
		"    def _interpolate_path(self, path_template: str, path_params: Mapping[str, Any] | None) -> str:",
		"        path = path_template",
		"        for key, value in (path_params or {}).items():",
		"            path = path.replace('{' + key + '}', urllib.parse.quote(str(value), safe=''))",
		"        return path",
		"",
		"    def _parse_payload(self, raw: bytes) -> Any:",
		"        if not raw:",
		"            return None",
		"        text = raw.decode(\"utf-8\", errors=\"replace\")",
		"        try:",
		"            return json.loads(text)",
		"        except json.JSONDecodeError:",
		"            return text",
	)
	return strings.Join(lines, "\n")
}

func mapBodyType(operation operationRenderInfo) string {
	if operation.hasBody {
		return "models." + operation.requestType
	}
	return ""
}

func renderPythonMCP(spec openAPISpec) string {
	operations := collectOperationRenderInfos(spec)
	lines := []string{
		"# This file is generated by `go run ./tools/of-cli docs generate-sdk-python`.",
		"# Do not edit manually.",
		"from __future__ import annotations",
		"",
		"from typing import Any, Mapping",
		"",
		"from .client import OpenFoundryClient",
		"",
		"MCP_TOOL_REGISTRY: list[dict[str, Any]] = [",
	}
	for _, operation := range operations {
		lines = append(lines, "    {", fmt.Sprintf("        \"name\": %q,", operation.mcpToolName), fmt.Sprintf("        \"operation_id\": %q,", operation.operationID), "    },")
	}
	lines = append(lines,
		"]",
		"_MCP_TOOL_LOOKUP = {tool[\"name\"]: tool for tool in MCP_TOOL_REGISTRY}",
		"",
		"def list_openfoundry_mcp_tools() -> list[dict[str, Any]]:",
		"    return MCP_TOOL_REGISTRY",
		"",
		"def call_openfoundry_mcp_tool(client: OpenFoundryClient, tool_name: str, input: Mapping[str, Any] | None = None, headers: Mapping[str, str] | None = None) -> Any:",
		"    tool = _MCP_TOOL_LOOKUP.get(tool_name)",
		"    if tool is None:",
		"        raise ValueError(f\"Unknown OpenFoundry MCP tool: {tool_name}\")",
		"    return client.call_operation(tool[\"operation_id\"], input=input or {}, headers=headers)",
	)
	return strings.Join(lines, "\n")
}

func renderPythonSchemaDeclaration(name string, schema openAPISchema) []string {
	exportName := pythonExportName(name)
	if isObjectSchema(schema) && schema.AdditionalProperties == nil {
		lines := []string{"@dataclass(slots=True)", fmt.Sprintf("class %s:", exportName)}
		if schema.Properties == nil || len(*schema.Properties) == 0 {
			return append(lines, "    pass")
		}
		for _, propertyName := range sortedSchemaKeys(*schema.Properties) {
			lines = append(lines, fmt.Sprintf("    %s: %s | None = None", renderPythonVariableName(propertyName), pythonType((*schema.Properties)[propertyName])))
		}
		return lines
	}
	return []string{fmt.Sprintf("%s = %s", exportName, pythonType(schema))}
}

func renderPythonMethodSignatureOwned(pathParameters, queryParameters []ownedParameter, requestType string) string {
	parts := []string{"self"}
	for _, parameter := range pathParameters {
		parts = append(parts, fmt.Sprintf("%s: %s", renderPythonVariableName(parameter.name), pythonType(parameter.schema)))
	}
	for _, parameter := range queryParameters {
		parts = append(parts, fmt.Sprintf("%s: %s | None = None", renderPythonVariableName(parameter.name), pythonType(parameter.schema)))
	}
	if requestType != "" {
		parts = append(parts, fmt.Sprintf("body: %s", requestType))
	}
	parts = append(parts, "headers: Mapping[str, str] | None = None")
	return strings.Join(parts, ", ")
}

func renderPythonOperationCallArguments(operation operationRenderInfo) string {
	parts := []string{}
	for _, parameter := range operation.pathParameters {
		parts = append(parts, fmt.Sprintf("(payload.get('path') or {}).get(%q)", parameter.name))
	}
	for _, parameter := range operation.queryParameters {
		parts = append(parts, fmt.Sprintf("(payload.get('query') or {}).get(%q)", parameter.name))
	}
	if operation.hasBody {
		if len(operation.pathParameters) == 0 && len(operation.queryParameters) == 0 {
			parts = append(parts, "payload.get('body', payload)")
		} else {
			parts = append(parts, "payload.get('body')")
		}
	}
	parts = append(parts, "headers=headers")
	return strings.Join(parts, ", ")
}

func renderJavaSDK(spec openAPISpec) map[string]string {
	return map[string]string{
		"pom.xml":   renderJavaPOM(spec.Info.Version),
		"README.md": renderJavaREADME(spec.Info.Version),
		"src/main/java/com/openfoundry/sdk/OpenFoundryClient.java": renderJavaClient(spec),
	}
}

func renderJavaPOM(version string) string {
	return fmt.Sprintf("<project xmlns=\"http://maven.apache.org/POM/4.0.0\" xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\"\n  xsi:schemaLocation=\"http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd\">\n  <modelVersion>4.0.0</modelVersion>\n  <groupId>com.openfoundry</groupId>\n  <artifactId>openfoundry-sdk</artifactId>\n  <version>%s</version>\n  <name>OpenFoundry Java SDK</name>\n  <description>Official Java SDK generated from the OpenFoundry OpenAPI contract.</description>\n  <properties>\n    <maven.compiler.source>17</maven.compiler.source>\n    <maven.compiler.target>17</maven.compiler.target>\n    <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>\n  </properties>\n</project>\n", version)
}

func renderJavaREADME(version string) string {
	return fmt.Sprintf("# OpenFoundry Java SDK\n\nGenerated from `%s`.\n\nVersion: `%s`\n\n## Usage\n\n```java\nvar client = new OpenFoundryClient(\"https://platform.example.com\");\nvar meJson = client.authAuthGetme();\n```\n", openAPIPath, version)
}

func renderJavaClient(spec openAPISpec) string {
	lines := []string{
		"package com.openfoundry.sdk;",
		"",
		"// This file is generated by `go run ./tools/of-cli docs generate-sdk-java`.",
		"// Do not edit manually.",
		"",
		"import java.io.IOException;",
		"import java.net.URI;",
		"import java.net.URLEncoder;",
		"import java.net.http.HttpClient;",
		"import java.net.http.HttpRequest;",
		"import java.net.http.HttpResponse;",
		"import java.nio.charset.StandardCharsets;",
		"import java.util.LinkedHashMap;",
		"import java.util.Map;",
		"",
		"public final class OpenFoundryClient {",
		"    private final String baseUrl;",
		"    private final HttpClient httpClient;",
		"    private final Map<String, String> defaultHeaders;",
		"",
		"    public OpenFoundryClient(String baseUrl) {",
		"        this(baseUrl, HttpClient.newHttpClient(), Map.of());",
		"    }",
		"",
		"    public OpenFoundryClient(String baseUrl, HttpClient httpClient, Map<String, String> headers) {",
		"        this.baseUrl = baseUrl.endsWith(\"/\") ? baseUrl.substring(0, baseUrl.length() - 1) : baseUrl;",
		"        this.httpClient = httpClient;",
		"        this.defaultHeaders = new LinkedHashMap<>(headers);",
		"    }",
		"",
	}
	used := map[string]int{}
	for _, path := range sortedStringKeys(spec.Paths) {
		for _, method := range sortedStringKeys(spec.Paths[path]) {
			operation := spec.Paths[path][method]
			name := uniqueMethodName(methodNameForOperation(operation), used)
			hasBody := requestTypeForOperation(operation) != ""
			pathParameters := operationPathParameters(operation)
			queryParameters := operationQueryParameters(operation)
			lines = append(lines, fmt.Sprintf("    public String %s(%s) throws IOException, InterruptedException {", name, renderJavaMethodSignature(pathParameters, queryParameters, hasBody)))
			if len(pathParameters) > 0 {
				lines = append(lines, "        Map<String, Object> pathParams = new LinkedHashMap<>();")
				for _, parameter := range pathParameters {
					lines = append(lines, fmt.Sprintf("        pathParams.put(%q, %s);", parameter.Name, renderJavaVariableName(parameter.Name)))
				}
			} else {
				lines = append(lines, "        Map<String, Object> pathParams = Map.of();")
			}
			if len(queryParameters) > 0 {
				lines = append(lines, "        Map<String, Object> queryParams = new LinkedHashMap<>();")
				for _, parameter := range queryParameters {
					variableName := renderJavaVariableName(parameter.Name)
					lines = append(lines, fmt.Sprintf("        if (%s != null) { queryParams.put(%q, %s); }", variableName, parameter.Name, variableName))
				}
			} else {
				lines = append(lines, "        Map<String, Object> queryParams = Map.of();")
			}
			body := "null"
			if hasBody {
				body = "bodyJson"
			}
			lines = append(lines, fmt.Sprintf("        return request(%q, %q, pathParams, queryParams, %s);", strings.ToUpper(method), path, body), "    }", "")
		}
	}
	lines = append(lines,
		"    private String request(String method, String pathTemplate, Map<String, Object> pathParams, Map<String, Object> queryParams, String bodyJson) throws IOException, InterruptedException {",
		"        String path = interpolatePath(pathTemplate, pathParams);",
		"        String url = buildUrl(path, queryParams);",
		"        HttpRequest.BodyPublisher publisher = bodyJson == null ? HttpRequest.BodyPublishers.noBody() : HttpRequest.BodyPublishers.ofString(bodyJson);",
		"        HttpRequest.Builder builder = HttpRequest.newBuilder(URI.create(url)).method(method, publisher).header(\"content-type\", \"application/json\");",
		"        for (Map.Entry<String, String> entry : defaultHeaders.entrySet()) {",
		"            builder.header(entry.getKey(), entry.getValue());",
		"        }",
		"        HttpResponse<String> response = httpClient.send(builder.build(), HttpResponse.BodyHandlers.ofString());",
		"        if (response.statusCode() < 200 || response.statusCode() >= 300) {",
		"            throw new IOException(\"OpenFoundry request failed: \" + response.statusCode() + \" \" + response.body());",
		"        }",
		"        return response.body();",
		"    }",
		"",
		"    private String interpolatePath(String pathTemplate, Map<String, Object> pathParams) {",
		"        String path = pathTemplate;",
		"        for (Map.Entry<String, Object> entry : pathParams.entrySet()) {",
		"            path = path.replace(\"{\" + entry.getKey() + \"}\", urlEncode(String.valueOf(entry.getValue())));",
		"        }",
		"        return path;",
		"    }",
		"",
		"    private String buildUrl(String path, Map<String, Object> queryParams) {",
		"        if (queryParams.isEmpty()) {",
		"            return baseUrl + path;",
		"        }",
		"        StringBuilder query = new StringBuilder();",
		"        for (Map.Entry<String, Object> entry : queryParams.entrySet()) {",
		"            if (entry.getValue() == null) {",
		"                continue;",
		"            }",
		"            if (query.length() > 0) {",
		"                query.append('&');",
		"            }",
		"            query.append(urlEncode(entry.getKey())).append('=').append(urlEncode(String.valueOf(entry.getValue())));",
		"        }",
		"        return query.length() == 0 ? baseUrl + path : baseUrl + path + \"?\" + query;",
		"    }",
		"",
		"    private String urlEncode(String value) {",
		"        return URLEncoder.encode(value, StandardCharsets.UTF_8);",
		"    }",
		"}",
	)
	return strings.Join(lines, "\n")
}

func renderJavaMethodSignature(pathParameters, queryParameters []openAPIParameter, hasBody bool) string {
	parts := []string{}
	for _, parameter := range pathParameters {
		parts = append(parts, fmt.Sprintf("%s %s", javaType(parameter.Schema), renderJavaVariableName(parameter.Name)))
	}
	for _, parameter := range queryParameters {
		parts = append(parts, fmt.Sprintf("%s %s", boxedJavaType(parameter.Schema), renderJavaVariableName(parameter.Name)))
	}
	if hasBody {
		parts = append(parts, "String bodyJson")
	}
	return strings.Join(parts, ", ")
}

func pythonType(schema openAPISchema) string {
	if schema.Ref != "" {
		return pythonExportName(lastRefSegment(schema.Ref))
	}
	switch schema.Type {
	case "string":
		return "str"
	case "integer":
		return "int"
	case "number":
		return "float"
	case "boolean":
		return "bool"
	case "array":
		if schema.Items == nil {
			return "list[Any]"
		}
		return fmt.Sprintf("list[%s]", pythonType(*schema.Items))
	case "object":
		if schema.AdditionalProperties != nil {
			return fmt.Sprintf("dict[str, %s]", pythonType(*schema.AdditionalProperties))
		}
		return "dict[str, Any]"
	default:
		return "Any"
	}
}

func javaType(schema openAPISchema) string {
	if schema.Ref != "" {
		return "String"
	}
	switch schema.Type {
	case "string":
		return "String"
	case "integer":
		return "long"
	case "number":
		return "double"
	case "boolean":
		return "boolean"
	default:
		return "String"
	}
}

func boxedJavaType(schema openAPISchema) string {
	if schema.Ref != "" {
		return "String"
	}
	switch schema.Type {
	case "integer":
		return "Long"
	case "number":
		return "Double"
	case "boolean":
		return "Boolean"
	default:
		return "String"
	}
}

func renderPropertyName(name string) string {
	if isTypeScriptIdentifier(name) {
		return name
	}
	return fmt.Sprintf("%q", name)
}

func renderTypeScriptVariableName(name string) string {
	if isTypeScriptIdentifier(name) {
		return name
	}
	var out strings.Builder
	for _, ch := range name {
		if isASCIIAlphaNumeric(ch) || ch == '_' || ch == '$' {
			out.WriteRune(ch)
		} else {
			out.WriteByte('_')
		}
	}
	return out.String()
}

func renderPythonVariableName(name string) string {
	var out strings.Builder
	for _, ch := range name {
		if isASCIIAlphaNumeric(ch) || ch == '_' {
			out.WriteRune(ch)
		} else {
			out.WriteByte('_')
		}
	}
	return out.String()
}

func renderJavaVariableName(name string) string {
	parts := splitIdentifierTokens(name)
	if len(parts) == 0 {
		return "value"
	}
	first := strings.ToLower(parts[0])
	var rest strings.Builder
	for _, part := range parts[1:] {
		rest.WriteString(toPascalCase(part))
	}
	return first + rest.String()
}

func typescriptExportName(name string) string {
	var out strings.Builder
	capitalizeNext := true
	for _, ch := range name {
		if isASCIIAlphaNumeric(ch) {
			if capitalizeNext {
				out.WriteRune(toASCIIUpper(ch))
				capitalizeNext = false
			} else {
				out.WriteRune(ch)
			}
		} else {
			capitalizeNext = true
		}
	}
	if out.Len() == 0 {
		return "UnknownSchema"
	}
	return out.String()
}

func pythonExportName(name string) string {
	return typescriptExportName(name)
}

func pythonMethodName(name string) string {
	var out strings.Builder
	for i, ch := range name {
		if ch >= 'A' && ch <= 'Z' {
			if i > 0 {
				out.WriteByte('_')
			}
			out.WriteRune(ch + ('a' - 'A'))
		} else {
			out.WriteRune(ch)
		}
	}
	if out.Len() == 0 {
		return "operation"
	}
	return out.String()
}

func toCamelCase(name string) string {
	pascal := toPascalCase(name)
	if pascal == "" {
		return "operation"
	}
	return strings.ToLower(pascal[:1]) + pascal[1:]
}

func toPascalCase(name string) string {
	var out strings.Builder
	uppercaseNext := true
	for _, ch := range name {
		if isASCIIAlphaNumeric(ch) {
			if uppercaseNext {
				out.WriteRune(toASCIIUpper(ch))
				uppercaseNext = false
			} else {
				out.WriteRune(toASCIILower(ch))
			}
		} else {
			uppercaseNext = true
		}
	}
	if out.Len() == 0 {
		return "Operation"
	}
	return out.String()
}

func toKebabCase(value string) string {
	var out strings.Builder
	for i, ch := range value {
		if ch >= 'A' && ch <= 'Z' {
			if i > 0 {
				out.WriteByte('-')
			}
			out.WriteRune(ch + ('a' - 'A'))
		} else {
			out.WriteRune(ch)
		}
	}
	return out.String()
}

func isTypeScriptIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, ch := range name {
		if i == 0 {
			if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_' || ch == '$') {
				return false
			}
			continue
		}
		if !(isASCIIAlphaNumeric(ch) || ch == '_' || ch == '$') {
			return false
		}
	}
	return true
}

func isObjectSchema(schema openAPISchema) bool {
	return schema.Type == "object" || schema.Properties != nil || schema.AdditionalProperties != nil
}

func fallbackTypeScriptType(name string) string {
	if name == "Uuid" {
		return "string"
	}
	if name == "Value" {
		return "unknown"
	}
	if strings.HasSuffix(name, "Status") || strings.HasSuffix(name, "Type") || strings.HasSuffix(name, "Format") || strings.HasSuffix(name, "Mode") {
		return "string"
	}
	return "unknown"
}

func pythonFallbackType(name string) string {
	if name == "Uuid" {
		return "str"
	}
	if name == "Value" {
		return "Any"
	}
	if strings.HasSuffix(name, "Status") || strings.HasSuffix(name, "Type") || strings.HasSuffix(name, "Format") || strings.HasSuffix(name, "Mode") {
		return "str"
	}
	return "Any"
}

func sortedStringKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedBoolKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedSchemaKeys(m map[string]openAPISchema) []string {
	return sortedStringKeys(m)
}

func splitIdentifierTokens(value string) []string {
	fields := strings.FieldsFunc(value, func(ch rune) bool {
		return !isASCIIAlphaNumeric(ch)
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func compactStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := values[:0]
	var previous string
	for i, value := range values {
		if i == 0 || value != previous {
			out = append(out, value)
			previous = value
		}
	}
	return out
}

func marshalPretty(value any) string {
	data, _ := json.MarshalIndent(value, "", "  ")
	return string(data)
}

func normalizeLineEndings(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}

func fallbackString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func lastToken(value string) string {
	if idx := strings.LastIndex(value, "."); idx >= 0 {
		return value[idx+1:]
	}
	return value
}

func lastRefSegment(value string) string {
	if idx := strings.LastIndex(value, "/"); idx >= 0 {
		return value[idx+1:]
	}
	return value
}

func isASCIIAlphaNumeric(ch rune) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}

func toASCIIUpper(ch rune) rune {
	if ch >= 'a' && ch <= 'z' {
		return ch - ('a' - 'A')
	}
	return ch
}

func toASCIILower(ch rune) rune {
	if ch >= 'A' && ch <= 'Z' {
		return ch + ('a' - 'A')
	}
	return ch
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
