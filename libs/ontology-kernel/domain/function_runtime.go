// Function-package runtime: parsing, validation, dispatch.
//
// Behavioural parity:
//
//   - TypeScript / JavaScript inline functions execute via Node
//     (`state.NodeRuntimeCommand`) using the same harness script the
//     Rust crate ships, embedded byte-for-byte via go:embed.
//   - Python inline functions execute through the injected
//     PythonInlineRuntime. The ontology-actions-service binary wires
//     that runtime from libs/python-sidecar when PYTHON_SIDECAR_BINARY
//     is configured; otherwise execution fails explicitly with
//     ErrPythonRuntimeNotWired while parse/validate remain available.
//   - LoadAccessibleObjectSet routes through ObjectStore.ListByType
//     because the SearchBackend interface is not yet ported. The
//     filter cascade (ensure_object_access + object_to_json) is
//     identical to the Rust source.
//   - LoadLinkedObjects is fully 1:1: walks LinkStore for both
//     directions, hydrates neighbours via the read-side helper.
package domain

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

//go:embed embed/typescript_runner.mjs
var typescriptRuntimeRunner string

// ErrPythonRuntimeNotWired is returned by ExecuteInlinePythonFunction
// when the host service has not injected a PythonInlineRuntime into
// AppState. The parser + capability validator still work for python
// configs so create/update/validate paths are fully functional.
var ErrPythonRuntimeNotWired = errors.New("execute_inline_python_function: python runtime not wired (set AppState.PythonRuntime to a libs/python-sidecar Manager)")

// ── Public types ────────────────────────────────────────────────────

// InlinePythonFunctionConfig mirrors `pub struct InlinePythonFunctionConfig`.
type InlinePythonFunctionConfig struct {
	Runtime string `json:"runtime"`
	Source  string `json:"source"`
}

// InlineTypeScriptFunctionConfig mirrors the TS variant of the same.
type InlineTypeScriptFunctionConfig struct {
	Runtime string `json:"runtime"`
	Source  string `json:"source"`
}

// InlineFunctionConfigKind enumerates the runtime variants. Rust uses
// `enum InlineFunctionConfig { Python, TypeScript }`; Go uses a
// tagged-union struct so callers can `switch` on `.Kind`.
type InlineFunctionConfigKind int

const (
	InlineFunctionPython InlineFunctionConfigKind = iota
	InlineFunctionTypeScript
)

// InlineFunctionConfig is the tagged union the Rust enum maps onto.
type InlineFunctionConfig struct {
	Kind       InlineFunctionConfigKind
	Python     *InlinePythonFunctionConfig
	TypeScript *InlineTypeScriptFunctionConfig
}

// RuntimeName mirrors `impl InlineFunctionConfig::runtime_name`.
func (c InlineFunctionConfig) RuntimeName() string {
	switch c.Kind {
	case InlineFunctionPython:
		if c.Python != nil {
			return c.Python.Runtime
		}
	case InlineFunctionTypeScript:
		if c.TypeScript != nil {
			return c.TypeScript.Runtime
		}
	}
	return ""
}

// SourceLen mirrors `impl InlineFunctionConfig::source_len`.
func (c InlineFunctionConfig) SourceLen() int {
	switch c.Kind {
	case InlineFunctionPython:
		if c.Python != nil {
			return len(c.Python.Source)
		}
	case InlineFunctionTypeScript:
		if c.TypeScript != nil {
			return len(c.TypeScript.Source)
		}
	}
	return 0
}

// ResolvedInlineFunction mirrors `pub struct ResolvedInlineFunction`.
type ResolvedInlineFunction struct {
	Config       InlineFunctionConfig
	Capabilities models.FunctionCapabilities
	Package      *models.FunctionPackageSummary
}

// RuntimeName mirrors the corresponding Rust accessor.
func (r ResolvedInlineFunction) RuntimeName() string { return r.Config.RuntimeName() }

// SourceLen mirrors the corresponding Rust accessor.
func (r ResolvedInlineFunction) SourceLen() int { return r.Config.SourceLen() }

// ── Private envelope types (matches Rust `*Config` private structs) ─

type functionPackageReferenceConfig struct {
	FunctionPackageID uuid.UUID `json:"function_package_id"`
}

type versionedFunctionPackageReferenceConfig struct {
	Name        string `json:"function_package_name"`
	Version     string `json:"function_package_version"`
	AutoUpgrade bool   `json:"function_package_auto_upgrade"`
}

type typeScriptRuntimeError struct {
	Message string `json:"message"`
}

type typeScriptRuntimeEnvelope struct {
	Result json.RawMessage         `json:"result"`
	Stdout []string                `json:"stdout"`
	Stderr []string                `json:"stderr"`
	Error  *typeScriptRuntimeError `json:"error"`
}

// ── Public entry points ─────────────────────────────────────────────

// ParseInlineFunctionConfig mirrors `pub fn parse_inline_function_config`.
//
// Returns (nil, nil) when `runtime` is absent — the caller treats that
// as "not an inline function". Surfaces typed errors for unsupported
// runtimes and empty source.
func ParseInlineFunctionConfig(config json.RawMessage) (*InlineFunctionConfig, error) {
	if len(config) == 0 {
		return nil, nil
	}
	var head struct {
		Runtime *string `json:"runtime"`
	}
	if err := json.Unmarshal(config, &head); err != nil {
		return nil, err
	}
	if head.Runtime == nil {
		return nil, nil
	}

	switch *head.Runtime {
	case "python":
		var parsed InlinePythonFunctionConfig
		if err := json.Unmarshal(config, &parsed); err != nil {
			return nil, err
		}
		if strings.TrimSpace(parsed.Source) == "" {
			return nil, errors.New("inline python function requires a non-empty source")
		}
		return &InlineFunctionConfig{Kind: InlineFunctionPython, Python: &parsed}, nil
	case "typescript", "javascript":
		var parsed InlineTypeScriptFunctionConfig
		if err := json.Unmarshal(config, &parsed); err != nil {
			return nil, err
		}
		if strings.TrimSpace(parsed.Source) == "" {
			return nil, fmt.Errorf("inline %s function requires a non-empty source", parsed.Runtime)
		}
		return &InlineFunctionConfig{Kind: InlineFunctionTypeScript, TypeScript: &parsed}, nil
	default:
		return nil, fmt.Errorf(
			"unsupported function runtime '%s', supported runtimes: 'python', 'typescript', 'javascript'",
			*head.Runtime,
		)
	}
}

// ValidateFunctionCapabilities mirrors `pub fn validate_function_capabilities`.
func ValidateFunctionCapabilities(
	config InlineFunctionConfig,
	capabilities models.FunctionCapabilities,
	pkg *models.FunctionPackageSummary,
) error {
	if uint64(config.SourceLen()) > capabilities.MaxSourceBytes {
		source := "inline function"
		if pkg != nil {
			source = fmt.Sprintf("function package '%s'", pkg.Name)
		}
		return fmt.Errorf("%s exceeds max_source_bytes (%d > %d)",
			source, config.SourceLen(), capabilities.MaxSourceBytes)
	}
	if capabilities.TimeoutSeconds == 0 || capabilities.TimeoutSeconds > 300 {
		return errors.New("timeout_seconds must be between 1 and 300 for ontology function execution")
	}
	if pkg != nil {
		if pkg.Entrypoint != "default" && pkg.Entrypoint != "handler" {
			return fmt.Errorf(
				"unsupported function package entrypoint '%s', supported values: default, handler",
				pkg.Entrypoint,
			)
		}
	}
	return nil
}

// ResolveInlineFunctionConfig mirrors `pub async fn resolve_inline_function_config`.
//
// Three-way dispatch on the config envelope:
//  1. `function_package_id` → load by id, parse runtime+source from
//     the row, attach the row's capabilities + summary.
//  2. `function_package_name` (+ version, + auto_upgrade) → load all
//     packages with that name, run version selection.
//  3. inline `runtime` + `source` → parse + default capabilities.
func ResolveInlineFunctionConfig(
	ctx context.Context,
	state *ontologykernel.AppState,
	config json.RawMessage,
) (*ResolvedInlineFunction, error) {
	if len(config) == 0 {
		return nil, nil
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(config, &asMap); err != nil {
		return nil, err
	}

	if raw, ok := asMap["function_package_id"]; ok {
		var ref functionPackageReferenceConfig
		if err := json.Unmarshal([]byte(`{"function_package_id":`+string(raw)+`}`), &ref); err != nil {
			return nil, fmt.Errorf("invalid function package reference: %s", err)
		}
		pkg, err := loadFunctionPackage(ctx, state, ref.FunctionPackageID)
		if err != nil {
			return nil, err
		}
		if pkg == nil {
			return nil, errors.New("referenced function package was not found")
		}
		summary := pkg.Summary()
		body, _ := json.Marshal(map[string]string{
			"runtime": pkg.Runtime,
			"source":  pkg.Source,
		})
		inline, err := ParseInlineFunctionConfig(body)
		if err != nil {
			return nil, err
		}
		if inline == nil {
			return nil, errors.New("function package does not define a supported runtime")
		}
		if err := ValidateFunctionCapabilities(*inline, pkg.Capabilities, &summary); err != nil {
			return nil, err
		}
		return &ResolvedInlineFunction{
			Config:       *inline,
			Capabilities: pkg.Capabilities,
			Package:      &summary,
		}, nil
	}

	if _, ok := asMap["function_package_name"]; ok {
		ref := versionedFunctionPackageReferenceConfig{}
		if raw, ok := asMap["function_package_name"]; ok {
			_ = json.Unmarshal(raw, &ref.Name)
		}
		if raw, ok := asMap["function_package_version"]; ok {
			_ = json.Unmarshal(raw, &ref.Version)
		}
		if raw, ok := asMap["function_package_auto_upgrade"]; ok {
			_ = json.Unmarshal(raw, &ref.AutoUpgrade)
		}
		packages, err := loadFunctionPackagesByName(ctx, state, ref.Name)
		if err != nil {
			return nil, err
		}
		selected, err := selectFunctionPackageVersion(packages, ref)
		if err != nil {
			return nil, err
		}
		if selected == nil {
			if ref.AutoUpgrade {
				return nil, fmt.Errorf(
					"no compatible function package version found for '%s' starting at %s",
					ref.Name, ref.Version,
				)
			}
			return nil, fmt.Errorf(
				"referenced function package '%s@%s' was not found", ref.Name, ref.Version,
			)
		}
		summary := selected.Summary()
		body, _ := json.Marshal(map[string]string{
			"runtime": selected.Runtime,
			"source":  selected.Source,
		})
		inline, err := ParseInlineFunctionConfig(body)
		if err != nil {
			return nil, err
		}
		if inline == nil {
			return nil, errors.New("function package does not define a supported runtime")
		}
		if err := ValidateFunctionCapabilities(*inline, selected.Capabilities, &summary); err != nil {
			return nil, err
		}
		return &ResolvedInlineFunction{
			Config:       *inline,
			Capabilities: selected.Capabilities,
			Package:      &summary,
		}, nil
	}

	inline, err := ParseInlineFunctionConfig(config)
	if err != nil {
		return nil, err
	}
	if inline == nil {
		return nil, nil
	}
	caps := models.DefaultFunctionCapabilities()
	if err := ValidateFunctionCapabilities(*inline, caps, nil); err != nil {
		return nil, err
	}
	return &ResolvedInlineFunction{
		Config:       *inline,
		Capabilities: caps,
		Package:      nil,
	}, nil
}

// ── Version selection (mirrors Rust private fns) ────────────────────

func supportsAutoUpgrade(major int, pre string) bool {
	return major >= 1 && pre == ""
}

func compatibleAutoUpgradeVersion(baselineMajor int, baselinePre string, baseline, candidate string) bool {
	if !supportsAutoUpgrade(baselineMajor, baselinePre) {
		return false
	}
	candMajor, _, _, candPre, ok := splitSemver(candidate)
	if !ok {
		return false
	}
	if candMajor != baselineMajor || candPre != "" {
		return false
	}
	return semverGreaterOrEqual(candidate, baseline)
}

func selectFunctionPackageVersion(
	packages []models.FunctionPackage,
	ref versionedFunctionPackageReferenceConfig,
) (*models.FunctionPackage, error) {
	requestedMajor, _, _, requestedPre, ok := splitSemver(ref.Version)
	if !ok {
		return nil, fmt.Errorf("function package version must be valid semver: invalid: %s", ref.Version)
	}

	if ref.AutoUpgrade {
		if !supportsAutoUpgrade(requestedMajor, requestedPre) {
			return nil, errors.New(
				"function package auto-upgrade requires a stable baseline version 1.0.0 or above",
			)
		}
		type candidate struct {
			pkg     models.FunctionPackage
			version string
		}
		compatible := []candidate{}
		for _, p := range packages {
			if compatibleAutoUpgradeVersion(requestedMajor, requestedPre, ref.Version, p.Version) {
				compatible = append(compatible, candidate{pkg: p, version: p.Version})
			}
		}
		sort.Slice(compatible, func(i, j int) bool {
			return semverGreaterOrEqual(compatible[i].version, compatible[j].version) &&
				compatible[i].version != compatible[j].version
		})
		if len(compatible) == 0 {
			return nil, nil
		}
		return &compatible[0].pkg, nil
	}

	for i := range packages {
		if packages[i].Version == ref.Version {
			return &packages[i], nil
		}
	}
	return nil, nil
}

// splitSemver returns major/minor/patch/pre + ok. ok=false on invalid.
func splitSemver(version string) (int, int, int, string, bool) {
	if version == "" {
		return 0, 0, 0, "", false
	}
	core := version
	pre := ""
	if idx := strings.Index(version, "-"); idx >= 0 {
		core = version[:idx]
		pre = version[idx+1:]
	}
	if idx := strings.Index(core, "+"); idx >= 0 {
		core = core[:idx]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return 0, 0, 0, "", false
	}
	maj, err1 := strconv.Atoi(parts[0])
	min, err2 := strconv.Atoi(parts[1])
	pat, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, "", false
	}
	return maj, min, pat, pre, true
}

func semverGreaterOrEqual(a, b string) bool {
	aMaj, aMin, aPat, _, aOk := splitSemver(a)
	bMaj, bMin, bPat, _, bOk := splitSemver(b)
	if !aOk || !bOk {
		return false
	}
	if aMaj != bMaj {
		return aMaj > bMaj
	}
	if aMin != bMin {
		return aMin > bMin
	}
	return aPat >= bPat
}

// ── Loaders (1:1 of the Rust private async fns) ─────────────────────

func loadFunctionPackage(
	ctx context.Context,
	state *ontologykernel.AppState,
	id uuid.UUID,
) (*models.FunctionPackage, error) {
	if state == nil {
		return nil, errors.New("function package state is not configured")
	}
	if state.DB == nil {
		return loadFunctionPackageFromDefinitions(ctx, state, id)
	}
	var row models.FunctionPackageRow
	err := state.DB.QueryRow(ctx, `
		SELECT id, name, version, display_name, description, runtime, source, entrypoint,
		       capabilities, owner_id, created_at, updated_at
		FROM ontology_function_packages WHERE id = $1`, id,
	).Scan(
		&row.ID, &row.Name, &row.Version, &row.DisplayName, &row.Description,
		&row.Runtime, &row.Source, &row.Entrypoint, &row.Capabilities,
		&row.OwnerID, &row.CreatedAt, &row.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, errSentinelNoRows) || strings.Contains(err.Error(), "no rows") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load function package: %w", err)
	}
	pkg := row.IntoPackage()
	return &pkg, nil
}

// errSentinelNoRows lets us avoid an explicit pgx import dependency
// in this branch — we already match on the message body anyway.
var errSentinelNoRows = errors.New("no rows in result set")

func loadFunctionPackagesByName(
	ctx context.Context,
	state *ontologykernel.AppState,
	name string,
) ([]models.FunctionPackage, error) {
	if state == nil {
		return nil, errors.New("function package state is not configured")
	}
	if state.DB == nil {
		return loadFunctionPackagesByNameFromDefinitions(ctx, state, name)
	}
	rows, err := state.DB.Query(ctx, `
		SELECT id, name, version, display_name, description, runtime, source, entrypoint,
		       capabilities, owner_id, created_at, updated_at
		FROM ontology_function_packages WHERE name = $1`, name,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load function packages: %w", err)
	}
	defer rows.Close()

	out := []models.FunctionPackage{}
	for rows.Next() {
		var row models.FunctionPackageRow
		if err := rows.Scan(
			&row.ID, &row.Name, &row.Version, &row.DisplayName, &row.Description,
			&row.Runtime, &row.Source, &row.Entrypoint, &row.Capabilities,
			&row.OwnerID, &row.CreatedAt, &row.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to decode function packages: %w", err)
		}
		out = append(out, row.IntoPackage())
	}
	return out, rows.Err()
}

func loadFunctionPackageFromDefinitions(
	ctx context.Context,
	state *ontologykernel.AppState,
	id uuid.UUID,
) (*models.FunctionPackage, error) {
	if state.Stores.Definitions == nil {
		return nil, errors.New("function package definition store is not configured")
	}
	rec, err := state.Stores.Definitions.Get(ctx, storage.DefinitionKind("function_package"), storage.DefinitionId(id.String()), storage.Strong())
	if err != nil {
		return nil, fmt.Errorf("failed to load function package: %w", err)
	}
	if rec == nil {
		return nil, nil
	}
	var pkg models.FunctionPackage
	if err := json.Unmarshal(rec.Payload, &pkg); err != nil {
		return nil, fmt.Errorf("invalid function package definition: %w", err)
	}
	return &pkg, nil
}

func loadFunctionPackagesByNameFromDefinitions(
	ctx context.Context,
	state *ontologykernel.AppState,
	name string,
) ([]models.FunctionPackage, error) {
	if state.Stores.Definitions == nil {
		return nil, errors.New("function package definition store is not configured")
	}
	page, err := state.Stores.Definitions.List(ctx, storage.DefinitionQuery{
		Kind: storage.DefinitionKind("function_package"),
		Page: storage.Page{Size: 10_000},
	}, storage.Strong())
	if err != nil {
		return nil, fmt.Errorf("failed to load function packages: %w", err)
	}
	out := []models.FunctionPackage{}
	for _, rec := range page.Items {
		var pkg models.FunctionPackage
		if err := json.Unmarshal(rec.Payload, &pkg); err != nil {
			return nil, fmt.Errorf("invalid function package definition: %w", err)
		}
		if pkg.Name == name {
			out = append(out, pkg)
		}
	}
	return out, nil
}

// ── Execution dispatcher ────────────────────────────────────────────

// ExecuteInlineFunction mirrors `pub async fn execute_inline_function`.
// Dispatches to TypeScript via Node subprocess (full 1:1) or Python
// via the not-yet-wired sentinel.
func ExecuteInlineFunction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action *models.ActionType,
	target *ObjectInstance,
	parameters map[string]json.RawMessage,
	resolved *ResolvedInlineFunction,
	justification *string,
) (json.RawMessage, error) {
	switch resolved.Config.Kind {
	case InlineFunctionPython:
		return ExecuteInlinePythonFunction(ctx, state, claims, action, target, parameters, resolved, justification)
	case InlineFunctionTypeScript:
		return executeInlineTypeScriptFunction(ctx, state, claims, action, target, parameters, resolved, justification)
	default:
		return nil, errors.New("unknown inline function runtime")
	}
}

// ExecuteInlinePythonFunction is the Python entry point.
//
// When AppState.PythonRuntime is wired (by injecting a Manager from
// libs/python-sidecar) the function builds the same JSON envelope the
// Rust PyO3 path used and forwards execution to the sidecar; the
// sidecar returns the enriched result (payload + stdout/stderr already
// merged) which is re-emitted to the caller verbatim. When the runtime
// is nil, ErrPythonRuntimeNotWired is returned so callers surface the
// explicit python_runtime_not_wired service-unavailable envelope while
// the parse + validate paths keep working.
func ExecuteInlinePythonFunction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action *models.ActionType,
	target *ObjectInstance,
	parameters map[string]json.RawMessage,
	resolved *ResolvedInlineFunction,
	justification *string,
) (json.RawMessage, error) {
	if state == nil || state.PythonRuntime == nil || resolved == nil || resolved.Config.Python == nil {
		if state == nil || state.PythonRuntime == nil {
			return nil, ErrPythonRuntimeNotWired
		}
		return nil, errors.New("execute_inline_python_function: missing python config")
	}

	objectSet, err := LoadAccessibleObjectSet(ctx, state, claims, action.ObjectTypeID)
	if err != nil {
		return nil, err
	}
	linked := []json.RawMessage{}
	if target != nil {
		linked, err = LoadLinkedObjects(ctx, state, claims, target.ID)
		if err != nil {
			return nil, err
		}
	}
	serviceToken, err := issueInlineFunctionToken(state, claims)
	if err != nil {
		return nil, err
	}

	var targetJSON any
	if target != nil {
		targetJSON = ObjectToJSON(*target)
	}
	actionJSON := map[string]any{
		"id":                   action.ID,
		"name":                 action.Name,
		"display_name":         action.DisplayName,
		"object_type_id":       action.ObjectTypeID,
		"operation_kind":       action.OperationKind,
		"permission_key":       action.PermissionKey,
		"authorization_policy": action.AuthorizationPolicy,
	}
	envelope := map[string]any{
		"context": map[string]any{
			"action":        actionJSON,
			"targetObject":  targetJSON,
			"parameters":    parameters,
			"objectSet":     objectSet,
			"linkedObjects": linked,
			"justification": justification,
			"contextNow":    time.Now().UTC().Format(time.RFC3339Nano),
		},
		"policy":             resolved.Capabilities,
		"functionPackage":    resolved.Package,
		"serviceToken":       serviceToken,
		"ontologyServiceUrl": state.OntologyServiceURL,
		"aiServiceUrl":       state.AIServiceURL,
		"dataConnection": map[string]any{
			"connectorManagementServiceUrl": state.ConnectorManagementServiceURL,
		},
		"connectorManagementServiceUrl": state.ConnectorManagementServiceURL,
	}
	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode python envelope: %w", err)
	}

	timeoutSecs := uint32(resolved.Capabilities.TimeoutSeconds)
	if timeoutSecs == 0 {
		timeoutSecs = 30
	}
	out, err := state.PythonRuntime.ExecuteInline(ctx, resolved.Config.Python.Source, envelopeJSON, timeoutSecs)
	if err != nil {
		return nil, err
	}
	if len(out.ResultJSON) == 0 {
		return json.RawMessage(`null`), nil
	}
	if !json.Valid(out.ResultJSON) {
		return nil, fmt.Errorf("failed to decode Python function response: malformed result JSON")
	}
	return json.RawMessage(out.ResultJSON), nil
}

// executeInlineTypeScriptFunction is the full 1:1 port of the Rust
// TypeScript path: build the JSON envelope, write user.ts + runner.mjs
// + input.json to a temp dir, exec `node --experimental-strip-types
// runner.mjs user.ts input.json` with a timeout, parse the JSON
// envelope on stdout, enrich with logs.
func executeInlineTypeScriptFunction(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	action *models.ActionType,
	target *ObjectInstance,
	parameters map[string]json.RawMessage,
	resolved *ResolvedInlineFunction,
	justification *string,
) (json.RawMessage, error) {
	objectSet, err := LoadAccessibleObjectSet(ctx, state, claims, action.ObjectTypeID)
	if err != nil {
		return nil, err
	}
	linked := []json.RawMessage{}
	if target != nil {
		linked, err = LoadLinkedObjects(ctx, state, claims, target.ID)
		if err != nil {
			return nil, err
		}
	}
	serviceToken, err := issueInlineFunctionToken(state, claims)
	if err != nil {
		return nil, err
	}

	var targetJSON any
	if target != nil {
		targetJSON = ObjectToJSON(*target)
	}
	actionJSON := map[string]any{
		"id":                   action.ID,
		"name":                 action.Name,
		"display_name":         action.DisplayName,
		"object_type_id":       action.ObjectTypeID,
		"operation_kind":       action.OperationKind,
		"permission_key":       action.PermissionKey,
		"authorization_policy": action.AuthorizationPolicy,
	}
	input := map[string]any{
		"context": map[string]any{
			"action":        actionJSON,
			"targetObject":  targetJSON,
			"parameters":    parameters,
			"objectSet":     objectSet,
			"linkedObjects": linked,
			"justification": justification,
			"contextNow":    time.Now().UTC().Format(time.RFC3339Nano),
		},
		"policy":             resolved.Capabilities,
		"functionPackage":    resolved.Package,
		"serviceToken":       serviceToken,
		"ontologyServiceUrl": state.OntologyServiceURL,
		"aiServiceUrl":       state.AIServiceURL,
		"dataConnection": map[string]any{
			"connectorManagementServiceUrl": state.ConnectorManagementServiceURL,
		},
		"connectorManagementServiceUrl": state.ConnectorManagementServiceURL,
	}

	tempDir, err := os.MkdirTemp("", "of-ontology-inline-ts-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create TypeScript function temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	userFile := filepath.Join(tempDir, "user.ts")
	runnerFile := filepath.Join(tempDir, "runner.mjs")
	inputFile := filepath.Join(tempDir, "input.json")

	if err := os.WriteFile(userFile, []byte(resolved.Config.TypeScript.Source), 0o600); err != nil {
		return nil, fmt.Errorf("failed to write TypeScript function source: %w", err)
	}
	if err := os.WriteFile(runnerFile, []byte(typescriptRuntimeRunner), 0o600); err != nil {
		return nil, fmt.Errorf("failed to write TypeScript runtime harness: %w", err)
	}
	inputBytes, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(inputFile, inputBytes, 0o600); err != nil {
		return nil, fmt.Errorf("failed to write TypeScript runtime input: %w", err)
	}

	timeoutSecs := resolved.Capabilities.TimeoutSeconds
	if timeoutSecs == 0 {
		timeoutSecs = 30
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, state.NodeRuntimeCommand,
		"--experimental-strip-types", runnerFile, userFile, inputFile)
	stdoutBytes, err := cmd.Output()
	if runCtx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("TypeScript function timed out after %d seconds", timeoutSecs)
	}
	stderr := ""
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = string(exitErr.Stderr)
		} else {
			return nil, fmt.Errorf("failed to start TypeScript function runtime: %w", err)
		}
	}

	stdout := strings.TrimSpace(string(stdoutBytes))
	stderr = strings.TrimSpace(stderr)
	var envelope typeScriptRuntimeEnvelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		return nil, fmt.Errorf(
			"failed to decode TypeScript function response: %s; raw stdout: %s; raw stderr: %s",
			err, stdout, stderr,
		)
	}

	if cmd.ProcessState != nil && !cmd.ProcessState.Success() {
		message := "TypeScript function failed"
		if envelope.Error != nil {
			message = envelope.Error.Message
		}
		return nil, fmt.Errorf("%s\n%s", message, stderr)
	}
	if envelope.Error != nil {
		return nil, errors.New(envelope.Error.Message)
	}
	return enrichTypeScriptResult(envelope.Result, envelope.Stdout, envelope.Stderr), nil
}

// enrichTypeScriptResult mirrors `fn enrich_typescript_result`.
func enrichTypeScriptResult(result json.RawMessage, stdout, stderr []string) json.RawMessage {
	if len(result) > 0 && string(result) != "null" {
		var asObj map[string]json.RawMessage
		if err := json.Unmarshal(result, &asObj); err == nil {
			if len(stdout) > 0 {
				b, _ := json.Marshal(stdout)
				asObj["stdout"] = b
			}
			if len(stderr) > 0 {
				b, _ := json.Marshal(stderr)
				asObj["stderr"] = b
			}
			hasOutput := false
			if raw, ok := asObj["output"]; ok && string(raw) != "null" {
				hasOutput = true
			}
			if !hasOutput && (len(stdout) > 0 || len(stderr) > 0) {
				out, _ := json.Marshal(map[string]any{
					"stdout": stdout,
					"stderr": stderr,
				})
				asObj["output"] = out
			}
			merged, _ := json.Marshal(asObj)
			return merged
		}
		// Non-object result — wrap.
		var rawAny any
		_ = json.Unmarshal(result, &rawAny)
		out, _ := json.Marshal(map[string]any{
			"output": rawAny,
			"stdout": stdout,
			"stderr": stderr,
		})
		return out
	}
	out, _ := json.Marshal(map[string]any{
		"output": map[string]any{"stdout": stdout, "stderr": stderr},
		"stdout": stdout,
		"stderr": stderr,
	})
	return out
}

// issueInlineFunctionToken mirrors `fn issue_inline_function_token`.
// Re-mints the caller's claims into a bearer token the inline runtime
// can use to call back into ontology / AI services.
func issueInlineFunctionToken(state *ontologykernel.AppState, claims *authmw.Claims) (string, error) {
	if state.JWTConfig == nil {
		return "", errors.New("function runtime token requires a JWT config on AppState")
	}
	now := time.Now().UTC()
	cloned := *claims
	cloned.IAT = now.Unix()
	cloned.EXP = now.Add(state.JWTConfig.AccessTTL).Unix()
	if cloned.EXP <= cloned.IAT {
		cloned.EXP = now.Add(time.Hour).Unix()
	}
	cloned.JTI = uuid.New()
	token, err := authmw.EncodeToken(state.JWTConfig, &cloned)
	if err != nil {
		return "", fmt.Errorf("failed to issue function runtime token: %w", err)
	}
	return "Bearer " + token, nil
}

// ── Object/link helpers ─────────────────────────────────────────────

// LoadAccessibleObjectSet mirrors `pub async fn load_accessible_object_set`.
//
// Routes through `state.Stores.Search` when configured (matching the
// Rust SearchBackend.search path). When Search is nil the helper
// falls back to `ObjectStore.ListByType` (capped at 5,000 to match the
// Rust limit) and applies the same EnsureObjectAccess + ObjectToJSON
// cascade — keeps unit tests + air-gapped binaries working without a
// search engine.
func LoadAccessibleObjectSet(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	objectTypeID uuid.UUID,
) ([]json.RawMessage, error) {
	tenant := TenantFromClaims(claims)
	const limit = 5_000
	if state.Stores.Search != nil {
		return loadAccessibleObjectSetViaSearch(ctx, state, claims, tenant, objectTypeID, limit)
	}
	return loadAccessibleObjectSetViaListByType(ctx, state, claims, tenant, objectTypeID, limit)
}

// loadAccessibleObjectSetViaSearch pages through SearchBackend.Search,
// loads each hit's full ObjectInstance from ObjectStore so the
// `EnsureObjectAccess` cascade gets the same shape as the Rust impl
// (the search hit's snippet alone doesn't carry the marking /
// organization_id / created_by needed by the access check).
func loadAccessibleObjectSetViaSearch(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	tenant storage.TenantId,
	objectTypeID uuid.UUID,
	limit int,
) ([]json.RawMessage, error) {
	out := []json.RawMessage{}
	typeID := storage.TypeId(objectTypeID.String())
	var token *string
	for {
		page, err := state.Stores.Search.Search(ctx, storage.SearchQuery{
			Tenant: tenant,
			TypeID: &typeID,
			Page:   storage.Page{Size: 256, Token: token},
		}, storage.Eventual())
		if err != nil {
			return nil, fmt.Errorf("search backend search failed: %w", err)
		}
		for _, hit := range page.Items {
			obj, err := state.Stores.Objects.Get(ctx, tenant, hit.ID, storage.Eventual())
			if err != nil {
				return nil, fmt.Errorf("search hit dereference failed: %w", err)
			}
			if obj == nil {
				continue
			}
			inst := ObjectStoreToObjectInstance(*obj, claims.OrgID)
			if inst == nil {
				continue
			}
			if EnsureObjectAccess(claims, inst) != nil {
				continue
			}
			out = append(out, ObjectToJSON(*inst))
			if len(out) >= limit {
				return out, nil
			}
		}
		if page.NextToken == nil {
			return out, nil
		}
		token = page.NextToken
	}
}

func loadAccessibleObjectSetViaListByType(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	tenant storage.TenantId,
	objectTypeID uuid.UUID,
	limit int,
) ([]json.RawMessage, error) {
	out := []json.RawMessage{}
	var token *string
	for {
		page, err := state.Stores.Objects.ListByType(
			ctx, tenant, storage.TypeId(objectTypeID.String()),
			storage.Page{Size: 256, Token: token}, storage.Eventual(),
		)
		if err != nil {
			return nil, fmt.Errorf("search backend type listing failed: %w", err)
		}
		for _, item := range page.Items {
			inst := ObjectStoreToObjectInstance(item, claims.OrgID)
			if inst == nil {
				continue
			}
			if EnsureObjectAccess(claims, inst) != nil {
				continue
			}
			out = append(out, ObjectToJSON(*inst))
			if len(out) >= limit {
				return out, nil
			}
		}
		if page.NextToken == nil {
			return out, nil
		}
		token = page.NextToken
	}
}

// LoadLinkedObjects mirrors `pub async fn load_linked_objects`.
// Walks both link directions for every link type.
func LoadLinkedObjects(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	objectID uuid.UUID,
) ([]json.RawMessage, error) {
	tenant := TenantFromClaims(claims)
	objectKey := storage.ObjectId(objectID.String())
	linkTypes, err := loadAllLinkTypesAsc(ctx, state)
	if err != nil {
		return nil, err
	}

	out := []json.RawMessage{}
	for _, lt := range linkTypes {
		linkTypeID := storage.LinkTypeId(lt.ID.String())
		// Outgoing.
		outgoing, err := state.Stores.Links.ListOutgoing(ctx, tenant, linkTypeID, objectKey,
			storage.Page{Size: 256}, storage.Eventual())
		if err != nil {
			return nil, fmt.Errorf("failed to load outgoing links: %w", err)
		}
		for _, link := range outgoing.Items {
			neighborID, err := uuid.Parse(string(link.To))
			if err != nil {
				continue
			}
			neighbor, err := loadNeighborInstance(ctx, state, claims, neighborID)
			if err != nil {
				return nil, err
			}
			if neighbor == nil {
				continue
			}
			if EnsureObjectAccess(claims, neighbor) != nil {
				continue
			}
			entry, _ := json.Marshal(map[string]any{
				"direction":    "outbound",
				"link_id":      fmt.Sprintf("%s:%s:%s", lt.ID, objectID, neighborID),
				"link_type_id": lt.ID,
				"link_name":    lt.Name,
				"object":       json.RawMessage(ObjectToJSON(*neighbor)),
			})
			out = append(out, entry)
		}
		// Incoming.
		incoming, err := state.Stores.Links.ListIncoming(ctx, tenant, linkTypeID, objectKey,
			storage.Page{Size: 256}, storage.Eventual())
		if err != nil {
			return nil, fmt.Errorf("failed to load incoming links: %w", err)
		}
		for _, link := range incoming.Items {
			neighborID, err := uuid.Parse(string(link.From))
			if err != nil {
				continue
			}
			neighbor, err := loadNeighborInstance(ctx, state, claims, neighborID)
			if err != nil {
				return nil, err
			}
			if neighbor == nil {
				continue
			}
			if EnsureObjectAccess(claims, neighbor) != nil {
				continue
			}
			entry, _ := json.Marshal(map[string]any{
				"direction":    "inbound",
				"link_id":      fmt.Sprintf("%s:%s:%s", lt.ID, neighborID, objectID),
				"link_type_id": lt.ID,
				"link_name":    lt.Name,
				"object":       json.RawMessage(ObjectToJSON(*neighbor)),
			})
			out = append(out, entry)
		}
	}
	return out, nil
}

func loadAllLinkTypesAsc(
	ctx context.Context,
	state *ontologykernel.AppState,
) ([]models.LinkType, error) {
	rows, err := state.DB.Query(ctx,
		`SELECT id, name, display_name, description, source_type_id, target_type_id,
		        cardinality, owner_id, created_at, updated_at
		 FROM link_types ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to load link type metadata: %w", err)
	}
	defer rows.Close()
	out := []models.LinkType{}
	for rows.Next() {
		var lt models.LinkType
		if err := rows.Scan(
			&lt.ID, &lt.Name, &lt.DisplayName, &lt.Description,
			&lt.SourceTypeID, &lt.TargetTypeID, &lt.Cardinality,
			&lt.OwnerID, &lt.CreatedAt, &lt.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to decode link types: %w", err)
		}
		out = append(out, lt)
	}
	return out, rows.Err()
}

// loadNeighborInstance is the read-side hop the Rust impl makes via
// `load_object_instance_from_read_model`. Without the SearchBackend
// port we go directly to ObjectStore.Get and project to ObjectInstance.
func loadNeighborInstance(
	ctx context.Context,
	state *ontologykernel.AppState,
	claims *authmw.Claims,
	neighborID uuid.UUID,
) (*ObjectInstance, error) {
	stored, err := state.Stores.Objects.Get(ctx,
		TenantFromClaims(claims), storage.ObjectId(neighborID.String()), storage.Eventual())
	if err != nil {
		return nil, fmt.Errorf("failed to load linked object: %w", err)
	}
	if stored == nil {
		return nil, nil
	}
	return ObjectStoreToObjectInstance(*stored, claims.OrgID), nil
}

// ObjectToJSON mirrors `pub fn object_to_json`.
func ObjectToJSON(object ObjectInstance) json.RawMessage {
	out, _ := json.Marshal(map[string]any{
		"id":              object.ID,
		"object_type_id":  object.ObjectTypeID,
		"organization_id": object.OrganizationID,
		"marking":         object.Marking,
		"properties":      object.Properties,
		"created_by":      object.CreatedBy,
		"created_at":      object.CreatedAt,
		"updated_at":      object.UpdatedAt,
	})
	return out
}
