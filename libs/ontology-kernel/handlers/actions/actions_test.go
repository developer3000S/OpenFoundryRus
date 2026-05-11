package actions

import (
	"encoding/json"
	"math"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
)

// Mirrors the implicit Rust unit `parse_window_to_seconds`.
func TestParseWindowToSeconds(t *testing.T) {
	t.Parallel()
	cases := map[string]uint64{
		"30":     30 * 86_400, // bare numeric → days
		"30d":    30 * 86_400,
		"12h":    12 * 3_600,
		"45m":    45 * 60,
		"120s":   120,
		"2w":     2 * 7 * 86_400,
		"  30d ": 30 * 86_400, // trim leading / trailing whitespace
	}
	for in, want := range cases {
		got, err := parseWindowToSeconds(in)
		if err != nil {
			t.Errorf("parseWindowToSeconds(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseWindowToSeconds(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestParseWindowToSeconds_Errors(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "   ", "abc", "30y", "-5d"} {
		if _, err := parseWindowToSeconds(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}

// Mirrors the Rust `percentile_95_duration_ms` semantics:
// linear interpolation between adjacent ranks.
func TestPercentile95DurationMs(t *testing.T) {
	t.Parallel()
	if got := percentile95DurationMs(nil); got != nil {
		t.Fatalf("expected nil for empty input")
	}
	if got := percentile95DurationMs([]float64{42}); got == nil || *got != 42 {
		t.Fatalf("single sample: got %v", got)
	}
	// 100 evenly spaced points → p95 ≈ 94.05 (interp between idx 94 and 95).
	values := make([]float64, 100)
	for i := range values {
		values[i] = float64(i)
	}
	got := percentile95DurationMs(values)
	if got == nil || math.Abs(*got-94.05) > 1e-9 {
		t.Fatalf("p95 drift: got %v, want 94.05", got)
	}
}

// Mirrors the Rust `action_selection_kind` classifier.
func TestActionSelectionKind(t *testing.T) {
	t.Parallel()
	a := models.ActionType{InputSchema: []models.ActionInputField{
		{Name: "target", PropertyType: "object_reference"},
	}}
	if got := actionSelectionKind(a); got != "single" {
		t.Fatalf("single: got %s", got)
	}
	a.InputSchema = append(a.InputSchema, models.ActionInputField{
		Name: "targets", PropertyType: "object_reference_list",
	})
	if got := actionSelectionKind(a); got != "bulk" {
		t.Fatalf("bulk: got %s", got)
	}
	a = models.ActionType{InputSchema: []models.ActionInputField{
		{Name: "set", PropertyType: "object_set"},
	}}
	if got := actionSelectionKind(a); got != "bulk" {
		t.Fatalf("object_set bulk: got %s", got)
	}
}

func TestParseOperationKind(t *testing.T) {
	t.Parallel()
	// Mirrors the Rust `enum ActionOperationKind` exactly.
	for _, kind := range []string{
		"create_object", "update_object", "modify_object", "create_or_modify_object",
		"create_link", "delete_link", "delete_object",
		"invoke_function", "invoke_webhook",
		"create_interface", "modify_interface", "delete_interface",
		"create_interface_link", "delete_interface_link",
	} {
		if _, err := parseOperationKind(kind); err != nil {
			t.Errorf("expected %q to be valid: %v", kind, err)
		}
	}
	if _, err := parseOperationKind("noop"); err == nil {
		t.Fatal("expected error for unknown operation_kind")
	}
}

// scanJSONStringField is a perf shortcut over the standard library —
// confirm it agrees with json.Unmarshal on representative inputs.
func TestScanJSONStringField(t *testing.T) {
	t.Parallel()
	body := []byte(`{"action_type_id":"abc-123","status":"failure","duration_ms":42}`)
	if v, ok := scanJSONStringField(body, "action_type_id"); !ok || v != "abc-123" {
		t.Errorf("action_type_id: %v, %v", v, ok)
	}
	if v, ok := scanJSONStringField(body, "status"); !ok || v != "failure" {
		t.Errorf("status: %v, %v", v, ok)
	}
	if _, ok := scanJSONStringField(body, "missing"); ok {
		t.Fatal("missing field must return ok=false")
	}
	if v, ok := scanJSONNumberField(body, "duration_ms"); !ok || v != 42 {
		t.Errorf("duration_ms: %v, %v", v, ok)
	}
}

func TestValidateActionDefinitionAcceptsFunctionPackageRule(t *testing.T) {
	t.Parallel()
	state := newTestState(t)
	objectTypeID := uuid.New()
	packageID := uuid.New()
	seedObjectTypeDefinition(t, state, objectTypeID)
	seedFunctionPackageDefinition(t, state, models.FunctionPackage{
		ID:      packageID,
		Name:    "trail_effort",
		Version: "1.0.0",
		Runtime: "python",
		Source:  "result = {'ok': True}",
	})

	config, _ := json.Marshal(map[string]any{"function_package_id": packageID})
	err := validateActionDefinition(
		httptest.NewRequest("POST", "/ontology/actions", nil),
		state,
		objectTypeID,
		"invoke_function",
		nil,
		nil,
		config,
	)
	if err != nil {
		t.Fatalf("validateActionDefinition: %v", err)
	}
}

func TestValidateActionDefinitionRejectsMixedFunctionAndOntologyRules(t *testing.T) {
	t.Parallel()
	state := newTestState(t)
	objectTypeID := uuid.New()
	packageID := uuid.New()
	seedObjectTypeDefinition(t, state, objectTypeID)
	seedFunctionPackageDefinition(t, state, models.FunctionPackage{
		ID:      packageID,
		Name:    "trail_effort",
		Version: "1.0.0",
		Runtime: "python",
		Source:  "result = {'ok': True}",
	})

	config, _ := json.Marshal(map[string]any{
		"operation": map[string]any{
			"function_package_id": packageID,
			"property_mappings":   []map[string]any{{"property_name": "status", "input_name": "status"}},
		},
	})
	err := validateActionDefinition(
		httptest.NewRequest("POST", "/ontology/actions", nil),
		state,
		objectTypeID,
		"invoke_function",
		nil,
		nil,
		config,
	)
	if err == nil || !strings.Contains(err.Error(), "function rule cannot be combined with other Ontology rules") {
		t.Fatalf("expected mixed function/rule rejection, got %v", err)
	}
}
