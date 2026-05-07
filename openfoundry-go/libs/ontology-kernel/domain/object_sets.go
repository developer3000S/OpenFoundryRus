// Object set definition validation + filter / projection helpers.
//
// Mirrors `libs/ontology-kernel/src/domain/object_sets.rs` for the
// subset that does NOT depend on `domain::function_runtime`. The two
// store-bound helpers (`evaluate_object_set` and `resolve_traversals`,
// which call `load_accessible_object_set` / `load_linked_objects`)
// will land in iter 7c₅ alongside the function_runtime port.

package domain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
)

// EvaluateObjectSet mirrors `pub async fn evaluate_object_set`. It validates
// the definition and policy, loads tenant-scoped accessible objects through the
// function runtime helpers, applies filters/traversals/joins/projections, and
// returns the same response envelope as Rust.
func EvaluateObjectSet(ctx context.Context, state *ontologykernel.AppState, claims *authmw.Claims, definition models.ObjectSetDefinition, limit int, materialized bool) (models.ObjectSetEvaluationResponse, error) {
	if err := ValidateObjectSetDefinition(definition); err != nil {
		return models.ObjectSetEvaluationResponse{}, err
	}
	if err := EnforceObjectSetPolicy(claims, definition.Policy); err != nil {
		return models.ObjectSetEvaluationResponse{}, err
	}
	if limit < 0 {
		limit = 0
	}

	baseObjects, err := LoadAccessibleObjectSet(ctx, state, claims, definition.BaseObjectTypeID)
	if err != nil {
		return models.ObjectSetEvaluationResponse{}, err
	}
	filtered := make([]json.RawMessage, 0, len(baseObjects))
	for _, object := range baseObjects {
		if AllowsObjectSetMarking(claims, definition.Policy, object) && MatchesObjectSetFilters(object, definition.Filters) {
			filtered = append(filtered, object)
		}
	}

	var secondaryRows []json.RawMessage
	if definition.Join != nil {
		secondaryRows, err = LoadAccessibleObjectSet(ctx, state, claims, definition.Join.SecondaryObjectTypeID)
		if err != nil {
			return models.ObjectSetEvaluationResponse{}, err
		}
	}

	rows := []json.RawMessage{}
	traversalNeighborCount := 0
	for _, base := range filtered {
		neighbors, err := resolveObjectSetTraversals(ctx, state, claims, base, definition.Traversals)
		if err != nil {
			return models.ObjectSetEvaluationResponse{}, err
		}
		traversalNeighborCount += len(neighbors)
		seed, _ := json.Marshal(map[string]any{
			"base":          json.RawMessage(base),
			"neighbors":     neighbors,
			"what_if_label": definition.WhatIfLabel,
		})

		if definition.Join == nil {
			rows = append(rows, seed)
			continue
		}

		joined := []json.RawMessage{}
		for _, candidate := range secondaryRows {
			if JoinMatches(base, candidate, *definition.Join) {
				joined = append(joined, candidate)
			}
		}
		if len(joined) == 0 {
			if strings.EqualFold(definition.Join.JoinKind, "left") {
				rows = append(rows, AugmentObjectSetRowWithJoin(seed, json.RawMessage("null")))
			}
			continue
		}
		for _, item := range joined {
			rows = append(rows, AugmentObjectSetRowWithJoin(seed, item))
		}
	}

	totalRows := len(rows)
	if limit > len(rows) {
		limit = len(rows)
	}
	limited := make([]json.RawMessage, 0, limit)
	for _, row := range rows[:limit] {
		limited = append(limited, ProjectObjectSetRow(row, definition.Projections))
	}

	return models.ObjectSetEvaluationResponse{
		ObjectSet:              definition,
		TotalBaseMatches:       len(filtered),
		TotalRows:              totalRows,
		TraversalNeighborCount: traversalNeighborCount,
		Rows:                   limited,
		GeneratedAt:            time.Now().UTC(),
		Materialized:           materialized,
	}, nil
}

func resolveObjectSetTraversals(ctx context.Context, state *ontologykernel.AppState, claims *authmw.Claims, base json.RawMessage, traversals []models.ObjectSetTraversal) ([]json.RawMessage, error) {
	if len(traversals) == 0 {
		return []json.RawMessage{}, nil
	}
	idRaw := ResolveObjectSetPath(base, "id")
	idText, ok := jsonString(idRaw)
	if !ok {
		return []json.RawMessage{}, nil
	}
	id, err := uuid.Parse(idText)
	if err != nil {
		return []json.RawMessage{}, nil
	}
	frontier := []uuid.UUID{id}
	seen := map[uuid.UUID]bool{id: true}
	resolved := []json.RawMessage{}
	maxHops := int32(0)
	for _, traversal := range traversals {
		if traversal.MaxHops > maxHops {
			maxHops = traversal.MaxHops
		}
	}
	for hop := int32(0); hop < maxHops && len(frontier) > 0; hop++ {
		next := []uuid.UUID{}
		for _, current := range frontier {
			linked, err := LoadLinkedObjects(ctx, state, claims, current)
			if err != nil {
				return nil, err
			}
			for _, link := range linked {
				for _, traversal := range traversals {
					if traversal.MaxHops <= hop || !TraversalMatches(link, traversal) {
						continue
					}
					resolved = append(resolved, link)
					neighborText, ok := jsonString(ResolveObjectSetPath(link, "object.id"))
					if !ok {
						continue
					}
					neighborID, err := uuid.Parse(neighborText)
					if err == nil && !seen[neighborID] {
						seen[neighborID] = true
						next = append(next, neighborID)
					}
				}
			}
		}
		frontier = next
	}
	return resolved, nil
}

// ValidateObjectSetDefinition mirrors `pub fn validate_object_set_definition`.
// Returns nil on success or the verbatim Rust error string on the
// first failing rule.
func ValidateObjectSetDefinition(definition models.ObjectSetDefinition) error {
	if strings.TrimSpace(definition.Name) == "" {
		return fmt.Errorf("name is required")
	}
	for _, filter := range definition.Filters {
		if err := ValidateObjectSetFilter(filter); err != nil {
			return err
		}
	}
	for _, traversal := range definition.Traversals {
		if err := ValidateObjectSetTraversal(traversal); err != nil {
			return err
		}
	}
	if definition.Join != nil {
		if err := ValidateObjectSetJoin(*definition.Join); err != nil {
			return err
		}
	}
	for _, projection := range definition.Projections {
		if strings.TrimSpace(projection) == "" {
			return fmt.Errorf("projections cannot contain empty values")
		}
	}
	return ValidateObjectSetPolicy(definition.Policy)
}

// ValidateObjectSetFilter mirrors `fn validate_filter`.
func ValidateObjectSetFilter(filter models.ObjectSetFilter) error {
	if strings.TrimSpace(filter.Field) == "" {
		return fmt.Errorf("filters require a field")
	}
	switch filter.Operator {
	case "equals", "not_equals", "contains", "in", "exists", "gte", "lte":
		return nil
	default:
		return fmt.Errorf("unsupported filter operator '%s'", filter.Operator)
	}
}

// ValidateObjectSetTraversal mirrors `fn validate_traversal`.
func ValidateObjectSetTraversal(traversal models.ObjectSetTraversal) error {
	switch traversal.Direction {
	case "outbound", "inbound", "both":
	default:
		return fmt.Errorf("unsupported traversal direction '%s'", traversal.Direction)
	}
	if traversal.MaxHops <= 0 || traversal.MaxHops > 4 {
		return fmt.Errorf("traversal.max_hops must be between 1 and 4")
	}
	return nil
}

// ValidateObjectSetJoin mirrors `fn validate_join`.
func ValidateObjectSetJoin(join models.ObjectSetJoin) error {
	if strings.TrimSpace(join.LeftField) == "" || strings.TrimSpace(join.RightField) == "" {
		return fmt.Errorf("join fields cannot be empty")
	}
	switch join.JoinKind {
	case "inner", "left":
		return nil
	default:
		return fmt.Errorf("unsupported join kind '%s'", join.JoinKind)
	}
}

// ValidateObjectSetPolicy mirrors `fn validate_policy`. Calls
// [ValidateMarking] (access.go) for both the allowlist entries and
// the optional minimum_clearance.
func ValidateObjectSetPolicy(policy models.ObjectSetPolicy) error {
	for _, marking := range policy.AllowedMarkings {
		if err := ValidateMarking(marking); err != nil {
			return err
		}
	}
	if policy.MinimumClearance != nil {
		if err := ValidateMarking(*policy.MinimumClearance); err != nil {
			return err
		}
	}
	return nil
}

// EnforceObjectSetPolicy mirrors `fn enforce_object_set_policy`.
// Pure-claims enforcement; no IO.
func EnforceObjectSetPolicy(claims *authmw.Claims, policy models.ObjectSetPolicy) error {
	if policy.DenyGuestSessions && claims != nil && claims.IsGuestSession() {
		return fmt.Errorf("forbidden: object set blocks guest sessions")
	}
	if policy.MinimumClearance != nil {
		required, ok := MarkingRank(*policy.MinimumClearance)
		if !ok {
			return fmt.Errorf("invalid minimum clearance '%s'", *policy.MinimumClearance)
		}
		if ClearanceRank(claims) < required {
			return fmt.Errorf("forbidden: insufficient classification clearance for object set")
		}
	}
	if policy.RequiredRestrictedViewID != nil && claims != nil {
		if !claims.HasRole("admin") {
			allowed := claims.RestrictedViewIDs()
			matched := false
			for _, id := range allowed {
				if id == *policy.RequiredRestrictedViewID {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("forbidden: required restricted view is not present in the session")
			}
		}
	}
	return nil
}

// AllowsObjectSetMarking mirrors `fn allows_marking`. Receives the
// JSON-encoded object (Rust `&Value`) and inspects the `marking`
// field. Empty marking → reject; otherwise both the user's allowlist
// and the policy's allowlist must accept it.
func AllowsObjectSetMarking(claims *authmw.Claims, policy models.ObjectSetPolicy, object json.RawMessage) bool {
	marking, ok := jsonString(ResolveObjectSetPath(object, "marking"))
	if !ok || marking == "" {
		return false
	}
	if claims == nil || !claims.AllowsMarking(marking) {
		return false
	}
	if len(policy.AllowedMarkings) == 0 {
		return true
	}
	for _, candidate := range policy.AllowedMarkings {
		if strings.EqualFold(candidate, marking) {
			return true
		}
	}
	return false
}

// MatchesObjectSetFilters mirrors `fn matches_filters`. Walks every
// filter and returns true when ALL pass.
func MatchesObjectSetFilters(object json.RawMessage, filters []models.ObjectSetFilter) bool {
	for _, f := range filters {
		actual := ResolveObjectSetPath(object, f.Field)
		switch f.Operator {
		case "equals":
			if actual == nil || !objectSetRawEqual(actual, f.Value) {
				return false
			}
		case "not_equals":
			if actual != nil && objectSetRawEqual(actual, f.Value) {
				return false
			}
			if actual == nil {
				// Rust: actual != Some(filter.value) is true when
				// actual is None — so the predicate passes.
			}
		case "contains":
			if !rawContainsValue(actual, f.Value) {
				return false
			}
		case "in":
			items, ok := jsonArray(f.Value)
			if !ok {
				return false
			}
			if actual == nil {
				return false
			}
			match := false
			for _, item := range items {
				if objectSetRawEqual(item, actual) {
					match = true
					break
				}
			}
			if !match {
				return false
			}
		case "exists":
			expected := true
			var b bool
			if err := json.Unmarshal(f.Value, &b); err == nil {
				expected = b
			}
			if (actual != nil) != expected {
				return false
			}
		case "gte":
			c, ok := compareJSON(actual, f.Value)
			if !ok || c < 0 {
				return false
			}
		case "lte":
			c, ok := compareJSON(actual, f.Value)
			if !ok || c > 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// TraversalMatches mirrors `fn traversal_matches`.
func TraversalMatches(link json.RawMessage, traversal models.ObjectSetTraversal) bool {
	if traversal.Direction != "both" {
		dir, ok := jsonString(ResolveObjectSetPath(link, "direction"))
		if !ok || dir != traversal.Direction {
			return false
		}
	}
	if traversal.LinkTypeID != nil {
		s, ok := jsonString(ResolveObjectSetPath(link, "link_type_id"))
		if !ok {
			return false
		}
		if s != traversal.LinkTypeID.String() {
			return false
		}
	}
	if traversal.TargetObjectTypeID != nil {
		s, ok := jsonString(ResolveObjectSetPath(link, "object.object_type_id"))
		if !ok {
			return false
		}
		if s != traversal.TargetObjectTypeID.String() {
			return false
		}
	}
	return true
}

// JoinMatches mirrors `fn join_matches`. Both sides must resolve and
// compare equal.
func JoinMatches(base, candidate json.RawMessage, join models.ObjectSetJoin) bool {
	left := ResolveObjectSetPath(base, join.LeftField)
	right := ResolveObjectSetPath(candidate, join.RightField)
	return left != nil && objectSetRawEqual(left, right)
}

// AugmentObjectSetRowWithJoin mirrors `fn augment_row_with_join`.
// Re-marshals the row map with the `joined` field set.
func AugmentObjectSetRowWithJoin(row, joined json.RawMessage) json.RawMessage {
	obj, ok := jsonObject(row)
	if !ok {
		obj = map[string]json.RawMessage{}
	}
	obj["joined"] = joined
	out, _ := json.Marshal(obj)
	return out
}

// ProjectObjectSetRow mirrors `fn project_row`. Empty projections
// passes the row through unchanged.
func ProjectObjectSetRow(row json.RawMessage, projections []string) json.RawMessage {
	if len(projections) == 0 {
		return row
	}
	out := map[string]json.RawMessage{}
	for _, projection := range projections {
		v := resolveProjectionValue(row, projection)
		if v == nil {
			out[projection] = json.RawMessage("null")
			continue
		}
		out[projection] = v
	}
	res, _ := json.Marshal(out)
	return res
}

// resolveProjectionValue mirrors `fn resolve_projection_value`. Tries
// the wrapper-rooted path first; if absent and the projection is not
// already nested under base/joined/neighbors, retries under
// `base.<projection>`.
func resolveProjectionValue(row json.RawMessage, projection string) json.RawMessage {
	if strings.HasPrefix(projection, "base.") ||
		strings.HasPrefix(projection, "joined.") ||
		strings.HasPrefix(projection, "neighbors.") {
		return ResolveObjectSetPath(row, projection)
	}
	if v := ResolveObjectSetPath(row, projection); v != nil {
		return v
	}
	return ResolveObjectSetPath(row, "base."+projection)
}

// ResolveObjectSetPath mirrors `fn resolve_path`. Empty path returns
// the value itself; single-segment paths fall back to
// `properties.<seg>` and then `base.<seg>`; dotted paths walk each
// segment in order.
func ResolveObjectSetPath(value json.RawMessage, path string) json.RawMessage {
	if strings.TrimSpace(path) == "" {
		return value
	}
	if !strings.Contains(path, ".") {
		obj, ok := jsonObject(value)
		if !ok {
			return nil
		}
		if v, ok := obj[path]; ok {
			return v
		}
		if props, ok := obj["properties"]; ok {
			pp, ok := jsonObject(props)
			if ok {
				if v, ok := pp[path]; ok {
					return v
				}
			}
		}
		if base, ok := obj["base"]; ok {
			return ResolveObjectSetPath(base, path)
		}
		return nil
	}
	current := value
	for _, seg := range strings.Split(path, ".") {
		obj, ok := jsonObject(current)
		if !ok {
			return nil
		}
		next, ok := obj[seg]
		if !ok {
			return nil
		}
		current = next
	}
	return current
}

// objectSetRawEqual mirrors `Value == Value` after both sides have been
// canonicalised through Go's JSON decode/encode pair (so whitespace
// differences don't matter).
func objectSetRawEqual(a, b json.RawMessage) bool {
	return jsonEqual(a, b)
}

// rawContainsValue mirrors `fn contains_value`. String contains
// substring; array contains element.
func rawContainsValue(actual, expected json.RawMessage) bool {
	if actual == nil {
		return false
	}
	if as, ok := jsonString(actual); ok {
		es, ok := jsonString(expected)
		if !ok {
			return false
		}
		return strings.Contains(as, es)
	}
	if items, ok := jsonArray(actual); ok {
		for _, item := range items {
			if objectSetRawEqual(item, expected) {
				return true
			}
		}
		return false
	}
	return false
}

// (Avoid duplicate utility names with `domain.access.go` /
// `submission_eval.go`. The local helpers above are written in terms
// of the shared [jsonString], [jsonArray], [jsonObject], [jsonEqual],
// [compareJSON] implementations declared in submission_eval.go.)

// ensureBytesContainsAccessible — sanity import to avoid an unused
// compile error if a future trim drops the bytes import. The current
// implementations don't need it so the import was removed; this stub
// is a safety net for refactors.
var _ = bytes.Contains
