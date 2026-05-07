package domain

import (
	"errors"
	"strings"
	"testing"
)

func sampleSpec() UnionViewSpec {
	return UnionViewSpec{
		UnionViewDatasetRID: "ri.foundry.main.dataset.alpha-view",
		OutputDatasetRIDs: []string{
			"ri.foundry.main.dataset.alpha-out",
			"ri.foundry.main.dataset.beta-out",
		},
		DeploymentKeyParam: "region",
	}
}

func TestUnionViewSQLIncludesDeploymentKeyColumn(t *testing.T) {
	spec := sampleSpec()
	sql, err := ComposeUnionViewSQL(&spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(sql, "_deployment_key") {
		t.Fatalf("missing _deployment_key column: %q", sql)
	}
	if !strings.Contains(sql, "UNION ALL") {
		t.Fatalf("missing UNION ALL: %q", sql)
	}
	if !strings.Contains(sql, "ri.foundry.main.dataset.alpha-out") {
		t.Fatalf("missing alpha-out rid: %q", sql)
	}
	if !strings.Contains(sql, "ri.foundry.main.dataset.beta-out") {
		t.Fatalf("missing beta-out rid: %q", sql)
	}
}

func TestUnionViewSQLFiltersOutNonParameterizedTransactions(t *testing.T) {
	spec := sampleSpec()
	sql, err := ComposeUnionViewSQL(&spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(sql, "deployment_key IS NOT NULL") {
		t.Fatalf("missing deployment_key IS NOT NULL guard: %q", sql)
	}
}

func TestUnionViewSQLRejectsQuoteInRID(t *testing.T) {
	bad := sampleSpec()
	bad.OutputDatasetRIDs[0] = bad.OutputDatasetRIDs[0] + "'"
	_, err := ComposeUnionViewSQL(&bad)
	if !errors.Is(err, ErrUnionViewSpecForbiddenChar) {
		t.Fatalf("err = %v, want ErrUnionViewSpecForbiddenChar", err)
	}
}

func TestUnionViewSQLRejectsEmptyOutputs(t *testing.T) {
	bad := sampleSpec()
	bad.OutputDatasetRIDs = nil
	if _, err := ComposeUnionViewSQL(&bad); err == nil {
		t.Fatal("expected error for empty outputs")
	}
}

func TestUnionViewSQLEmitsOneSelectPerOutput(t *testing.T) {
	spec := sampleSpec()
	sql, err := ComposeUnionViewSQL(&spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Count(sql, "SELECT *"); got != 2 {
		t.Fatalf("SELECT * count = %d, want 2 (sql=%q)", got, sql)
	}
}

func TestUnionViewSQLRejectsSemicolonAndDoubleQuoteInRID(t *testing.T) {
	cases := []string{";", "\""}
	for _, ch := range cases {
		bad := sampleSpec()
		bad.OutputDatasetRIDs[0] = bad.OutputDatasetRIDs[0] + ch
		if _, err := ComposeUnionViewSQL(&bad); !errors.Is(err, ErrUnionViewSpecForbiddenChar) {
			t.Fatalf("char %q: err = %v, want ErrUnionViewSpecForbiddenChar", ch, err)
		}
	}
}
