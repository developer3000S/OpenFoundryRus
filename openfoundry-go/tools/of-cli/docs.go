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

type openAPISpec struct {
	OpenAPI    string                    `json:"openapi"`
	Info       openAPIInfo               `json:"info"`
	Servers    []openAPIServer           `json:"servers,omitempty"`
	Tags       []openAPITag              `json:"tags,omitempty"`
	Security   []map[string][]string     `json:"security,omitempty"`
	Paths      map[string]map[string]any `json:"paths"`
	Components openAPIComponents         `json:"components"`
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
	Schemas         map[string]map[string]any `json:"schemas"`
	SecuritySchemes map[string]any            `json:"securitySchemes,omitempty"`
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
		return fmt.Errorf("OpenAPI drift detected in %s. Regenerate it with `go run ./tools/of-cli -- docs generate-openapi --output %s`", expected, expected)
	}
	return nil
}

func buildOpenAPI(protoDir string) (openAPISpec, error) {
	files, err := collectProtoFiles(protoDir)
	if err != nil {
		return openAPISpec{}, err
	}
	paths := map[string]map[string]any{}
	schemas := map[string]map[string]any{
		"ApiError": {"type": "object", "properties": map[string]any{"error": map[string]any{"type": "string"}}, "required": []string{"error"}},
	}
	tagSet := map[string]bool{}
	for _, file := range files {
		contentBytes, err := os.ReadFile(file)
		if err != nil {
			return openAPISpec{}, err
		}
		content := string(contentBytes)
		pkg := firstMatch(content, `(?m)^\s*package\s+([A-Za-z0-9_.]+)\s*;`)
		if pkg == "" {
			pkg = strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		}
		for _, msg := range parseMessages(content) {
			schemas[msg] = map[string]any{"type": "object", "description": "Schema parsed from proto message `" + msg + "`."}
		}
		for _, svc := range parseServices(content) {
			tagSet[svc.name] = true
			for _, rpc := range svc.rpcs {
				method := httpMethodForRPC(rpc.name)
				path := packageBasePath(pkg) + "/" + toKebabCase(svc.name) + "/" + toKebabCase(rpc.name)
				if paths[path] == nil {
					paths[path] = map[string]any{}
				}
				responses := map[string]any{"200": map[string]any{"description": "Successful response.", "content": jsonContent(refSchema(rpc.response))}, "default": map[string]any{"description": "Error response.", "content": jsonContent(refSchema("ApiError"))}}
				op := map[string]any{"summary": rpc.name, "operationId": svc.name + rpc.name, "tags": []string{svc.name}, "responses": responses, "x-openfoundry-sdk-namespace": namespaceFor(pkg, svc.name), "x-openfoundry-mcp-tool": toKebabCase(svc.name) + "_" + toKebabCase(rpc.name)}
				if method != "get" && method != "delete" {
					op["requestBody"] = map[string]any{"required": true, "content": jsonContent(refSchema(rpc.request))}
				}
				paths[path][method] = op
			}
		}
	}
	tags := make([]openAPITag, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, openAPITag{Name: tag})
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Name < tags[j].Name })
	return openAPISpec{OpenAPI: "3.1.0", Info: openAPIInfo{Title: "OpenFoundry API", Version: "0.1.0", Description: "Generated from OpenFoundry proto contracts."}, Servers: []openAPIServer{{URL: "http://localhost:8080", Description: "Local development"}}, Tags: tags, Security: []map[string][]string{{"bearerAuth": {}}}, Paths: paths, Components: openAPIComponents{Schemas: schemas, SecuritySchemes: map[string]any{"bearerAuth": map[string]any{"type": "http", "scheme": "bearer", "bearerFormat": "JWT"}}}}, nil
}

type serviceDef struct {
	name string
	rpcs []rpcDef
}
type rpcDef struct{ name, request, response string }

func parseServices(content string) []serviceDef {
	svcRe := regexp.MustCompile(`service\s+(\w+)\s*\{([\s\S]*?)\}`)
	rpcRe := regexp.MustCompile(`rpc\s+(\w+)\s*\(([^)]+)\)\s+returns\s+\(([^)]+)\)`)
	var services []serviceDef
	for _, m := range svcRe.FindAllStringSubmatch(content, -1) {
		svc := serviceDef{name: m[1]}
		for _, r := range rpcRe.FindAllStringSubmatch(m[2], -1) {
			svc.rpcs = append(svc.rpcs, rpcDef{name: r[1], request: cleanType(r[2]), response: cleanType(r[3])})
		}
		services = append(services, svc)
	}
	return services
}

func parseMessages(content string) []string {
	re := regexp.MustCompile(`(?m)^\s*message\s+(\w+)\s*\{`)
	seen := map[string]bool{}
	var out []string
	for _, m := range re.FindAllStringSubmatch(content, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
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

func generateSDK(input, output, lang string) error {
	if _, err := os.ReadFile(input); err != nil {
		return err
	}
	files := map[string]string{}
	switch lang {
	case "typescript":
		files["package.json"] = "{\n  \"name\": \"@openfoundry/sdk\",\n  \"version\": \"0.1.0\",\n  \"type\": \"module\"\n}\n"
		files["src/index.ts"] = "// This file is generated by `go run ./tools/of-cli -- docs generate-sdk-typescript`.\nexport type Json = unknown;\n"
	case "python":
		files["openfoundry_sdk/__init__.py"] = "# This file is generated by `go run ./tools/of-cli -- docs generate-sdk-python`.\n__all__ = []\n"
	case "java":
		files["src/main/java/com/openfoundry/sdk/OpenFoundryClient.java"] = "// This file is generated by `go run ./tools/of-cli -- docs generate-sdk-java`.\npackage com.openfoundry.sdk;\npublic final class OpenFoundryClient {}\n"
	default:
		return fmt.Errorf("unknown SDK language %q", lang)
	}
	for path, content := range files {
		if err := writeFile(filepath.Join(output, path), []byte(content)); err != nil {
			return err
		}
	}
	return nil
}

func validateSDK(input, output, lang string) error {
	tmp, err := os.MkdirTemp("", "of-cli-sdk-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := generateSDK(input, tmp, lang); err != nil {
		return err
	}
	var diffs []string
	if err := filepath.WalkDir(tmp, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(tmp, path)
		want, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		got, err := os.ReadFile(filepath.Join(output, rel))
		if err != nil {
			diffs = append(diffs, rel)
			return nil
		}
		if !bytes.Equal(want, got) {
			diffs = append(diffs, rel)
		}
		return nil
	}); err != nil {
		return err
	}
	if len(diffs) > 0 {
		return fmt.Errorf("%s SDK drift detected: %s", lang, strings.Join(diffs, ", "))
	}
	return nil
}

func firstMatch(s, pattern string) string {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(s)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}
func cleanType(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "stream")), ".")
}
func refSchema(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + cleanType(name)}
}
func jsonContent(schema map[string]any) map[string]any {
	return map[string]any{"application/json": map[string]any{"schema": schema}}
}
func packageBasePath(pkg string) string {
	return "/" + strings.ReplaceAll(strings.Trim(pkg, "."), ".", "/")
}
func namespaceFor(pkg, svc string) string {
	if pkg == "" {
		return toKebabCase(svc)
	}
	return strings.ReplaceAll(pkg, ".", ".") + "." + svc
}
func httpMethodForRPC(name string) string {
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "get") || strings.HasPrefix(lower, "list") || strings.HasPrefix(lower, "search") {
		return "get"
	}
	if strings.HasPrefix(lower, "delete") {
		return "delete"
	}
	if strings.HasPrefix(lower, "update") || strings.HasPrefix(lower, "patch") {
		return "patch"
	}
	return "post"
}
func toKebabCase(value string) string {
	re := regexp.MustCompile(`([a-z0-9])([A-Z])`)
	return strings.ToLower(strings.ReplaceAll(re.ReplaceAllString(value, `${1}-${2}`), "_", "-"))
}
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
