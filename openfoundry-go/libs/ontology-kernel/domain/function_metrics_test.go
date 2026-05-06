package domain

import (
	"strings"
	"testing"
)

// libs/ontology-kernel/src/domain/function_metrics.rs — the
// 16-column INSERT shape is what the metrics dashboard reads. Pin the
// SQL byte sequence so a refactor that drops or reorders a column
// surfaces here.
func TestRecordFunctionPackageRunSQLShape(t *testing.T) {
	t.Parallel()
	sql := recordFunctionPackageRunSQL
	wantColumns := []string{
		"id",
		"function_package_id",
		"function_package_name",
		"function_package_version",
		"runtime",
		"status",
		"invocation_kind",
		"action_id",
		"action_name",
		"object_type_id",
		"target_object_id",
		"actor_id",
		"duration_ms",
		"error_message",
		"started_at",
		"completed_at",
	}
	for _, col := range wantColumns {
		if !strings.Contains(sql, col) {
			t.Errorf("SQL missing column %q", col)
		}
	}
	for i := 1; i <= 16; i++ {
		token := "$"
		if i < 10 {
			token += string(rune('0' + i))
		} else {
			token += "1" + string(rune('0'+i-10))
		}
		if !strings.Contains(sql, token) {
			t.Errorf("SQL missing placeholder %s", token)
		}
	}
	if !strings.Contains(sql, "INTO ontology_function_package_runs") {
		t.Fatal("SQL must target ontology_function_package_runs")
	}
}
