package domain

import (
	"errors"
	"strings"
)

// P4 — Parameterized union views.
//
// When a dataset is registered as a parameterized union view, its preview
// is UNION ALL over the per-deployment transactions of every output
// dataset, augmented with a synthetic _deployment_key column carrying the
// run's deployment value.
//
// This file is pure-SQL composition: tests can assert the exact query
// shape without booting Postgres.

// UnionViewSpec describes a parameterized union view. Mirrors the Rust
// `domain::parameterized_view::UnionViewSpec`. A wire-side alias of the
// same shape lives in models.UnionViewSpec.
type UnionViewSpec struct {
	UnionViewDatasetRID string
	OutputDatasetRIDs   []string
	DeploymentKeyParam  string
}

// ErrUnionViewSpecEmpty is returned when no output dataset RIDs are supplied.
var ErrUnionViewSpecEmpty = errors.New("output_dataset_rids must not be empty")

// ErrUnionViewSpecForbiddenChar is returned when an RID contains a quote,
// double quote, or semicolon. The composer interpolates RIDs verbatim into
// the SQL, so any of those characters could compose an injection-prone
// query if accepted from operator-supplied input.
var ErrUnionViewSpecForbiddenChar = errors.New("output_dataset_rid contains forbidden character")

// ComposeUnionViewSQL composes the SQL preview query for a parameterized
// union view.
//
// Each output dataset contributes a single SELECT * over its underlying
// physical view, augmented with the literal _deployment_key column the
// build pass writes onto every transaction. UNION ALL preserves duplicates
// across deployments — the doc warns that consumers see the raw rows, not
// a deduplicated projection.
//
// dataset_rid is interpolated verbatim into the SQL. The function rejects
// any RID containing a single quote, double quote, or semicolon so callers
// (the preview handler) cannot accidentally compose an injection-prone
// query from operator-supplied input.
func ComposeUnionViewSQL(spec *UnionViewSpec) (string, error) {
	if len(spec.OutputDatasetRIDs) == 0 {
		return "", ErrUnionViewSpecEmpty
	}
	for _, rid := range spec.OutputDatasetRIDs {
		if strings.ContainsAny(rid, "'\";") {
			return "", ErrUnionViewSpecForbiddenChar
		}
	}
	parts := make([]string, 0, len(spec.OutputDatasetRIDs))
	for _, rid := range spec.OutputDatasetRIDs {
		parts = append(parts, "SELECT *, deployment_key AS _deployment_key "+
			"FROM dataset_transactions "+
			"WHERE dataset_rid = '"+rid+"' "+
			"AND deployment_key IS NOT NULL")
	}
	return strings.Join(parts, " UNION ALL "), nil
}
