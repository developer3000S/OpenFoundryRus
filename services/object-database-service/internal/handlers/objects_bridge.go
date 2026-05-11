// Bridge handlers that adapt the gateway-fronted ontology paths
// (`/api/v1/ontology/types/{type_id}/objects[/...]`) onto the canonical
// ObjectStore shapes the service already implements. The gateway routes
// these prefixes here without rewriting the URL, so they need a dedicated
// adapter — apps/web's lib/api/ontology.ts is the canonical wire shape.
//
// Wire mapping
//   - frontend tenant ← `default` (single-tenant PoC; org_id from header
//     `x-of-tenant` overrides when present, so the gateway can inject it).
//   - object_type_id  ← storage.TypeId
//   - properties      ← payload (json-opaque)
//   - created_at      ← created_at_ms (RFC3339)
//   - updated_at      ← updated_at_ms
//   - created_by      ← `system` (PoC; real impl pulls from owner)
package handlers

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gocql/gocql"
	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/object-database-service/internal/storage"
)

const defaultTenant = "default"

// ontologyObject is the wire shape the SPA's ObjectInstance type expects.
type ontologyObject struct {
	ID             string         `json:"id"`
	ObjectTypeID   string         `json:"object_type_id"`
	Properties     map[string]any `json:"properties"`
	CreatedBy      string         `json:"created_by"`
	OrganizationID *string        `json:"organization_id,omitempty"`
	Marking        *string        `json:"marking,omitempty"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

func tenantFromRequest(r *http.Request) storage.TenantId {
	if t := strings.TrimSpace(r.Header.Get("x-of-tenant")); t != "" {
		return storage.TenantId(t)
	}
	return storage.TenantId(defaultTenant)
}

func toOntologyObject(obj *storage.Object) ontologyObject {
	props := map[string]any{}
	if len(obj.Payload) > 0 {
		_ = json.Unmarshal(obj.Payload, &props)
	}
	createdAt := time.UnixMilli(obj.UpdatedAtMs).UTC().Format(time.RFC3339Nano)
	if obj.CreatedAtMs != nil {
		createdAt = time.UnixMilli(*obj.CreatedAtMs).UTC().Format(time.RFC3339Nano)
	}
	out := ontologyObject{
		ID:             string(obj.ID),
		ObjectTypeID:   string(obj.TypeID),
		Properties:     props,
		CreatedBy:      "system",
		OrganizationID: obj.OrganizationID,
		CreatedAt:      createdAt,
		UpdatedAt:      time.UnixMilli(obj.UpdatedAtMs).UTC().Format(time.RFC3339Nano),
	}
	if len(obj.Markings) > 0 {
		m := string(obj.Markings[0])
		out.Marking = &m
	}
	return out
}

// ListObjectsByOntologyType serves GET /api/v1/ontology/types/{type_id}/objects.
// Pagination on the SPA side is page+per_page; we map per_page → storage.Page.Size.
//
// `total` is the underlying cardinality, computed via a separate unbounded list
// against the same tenant+type. This is O(N) — fine for PoC scale (10⁴) and
// the in-memory store. For Cassandra at 10⁶+ rows, swap to a denormalised
// counter (see NEXT-STEPS.md §4.1).
func (h *Handlers) ListObjectsByOntologyType(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromRequest(r)
	typeID := storage.TypeId(chi.URLParam(r, "type_id"))

	q := r.URL.Query()
	perPage := uint32(25)
	if v := q.Get("per_page"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil && n > 0 {
			if n > 5000 {
				n = 5000
			}
			perPage = uint32(n)
		}
	}
	page := 1
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	consistency := parseConsistency(q.Get("consistency"))

	res, err := h.Objects.ListByType(r.Context(), tenant, typeID, storage.Page{Size: perPage}, consistency)
	if err != nil {
		writeError(w, err)
		return
	}
	items := make([]ontologyObject, 0, len(res.Items))
	for i := range res.Items {
		items = append(items, toOntologyObject(&res.Items[i]))
	}

	total := len(items)
	if perPage < 5000 || res.NextToken != nil {
		// Page is potentially a slice of a larger set; ask for the full list
		// to materialise the real total. Skip when caller already pulled the
		// whole set in one shot (per_page>=5000 and no continuation).
		full, err := h.Objects.ListByType(r.Context(), tenant, typeID, storage.Page{Size: 1_000_000}, consistency)
		if err == nil {
			total = len(full.Items)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     items,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

// GetObjectByOntologyType serves GET /api/v1/ontology/types/{type_id}/objects/{object_id}.
func (h *Handlers) GetObjectByOntologyType(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromRequest(r)
	objID := storage.ObjectId(chi.URLParam(r, "object_id"))
	obj, err := h.Objects.Get(r.Context(), tenant, objID, parseConsistency(r.URL.Query().Get("consistency")))
	if err != nil {
		writeError(w, err)
		return
	}
	if obj == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, toOntologyObject(obj))
}

// UpdateObjectByOntologyType serves PATCH /api/v1/ontology/types/{type_id}/objects/{object_id}.
// Body shape: `{ properties: {...}, replace?: bool }`. The default behavior
// merges the provided properties into the existing payload, matching the SPA's
// inline-action update flow.
func (h *Handlers) UpdateObjectByOntologyType(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromRequest(r)
	typeID := storage.TypeId(chi.URLParam(r, "type_id"))
	objID := storage.ObjectId(chi.URLParam(r, "object_id"))

	var body struct {
		Properties map[string]any `json:"properties"`
		Replace    bool           `json:"replace"`
		Marking    *string        `json:"marking,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	existing, err := h.Objects.Get(r.Context(), tenant, objID, parseConsistency(r.URL.Query().Get("consistency")))
	if err != nil {
		writeError(w, err)
		return
	}
	if existing == nil || existing.TypeID != typeID {
		http.NotFound(w, r)
		return
	}

	props := map[string]any{}
	if !body.Replace && len(existing.Payload) > 0 {
		_ = json.Unmarshal(existing.Payload, &props)
	}
	for k, v := range body.Properties {
		props[k] = v
	}
	payload, err := json.Marshal(props)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	next := *existing
	next.Payload = payload
	next.UpdatedAtMs = time.Now().UnixMilli()
	if body.Marking != nil && strings.TrimSpace(*body.Marking) != "" {
		next.Markings = []storage.MarkingId{storage.MarkingId(strings.TrimSpace(*body.Marking))}
	}
	expected := existing.Version
	outcome, err := h.Objects.Put(r.Context(), next, &expected)
	if err != nil {
		writeError(w, err)
		return
	}
	if outcome.Kind == storage.PutVersionConflict {
		writeOutcomeResponse(w, outcome)
		return
	}
	if outcome.NewVersion > 0 {
		next.Version = outcome.NewVersion
	}
	writeJSON(w, http.StatusOK, toOntologyObject(&next))
}

// DeleteObjectByOntologyType serves DELETE /api/v1/ontology/types/{type_id}/objects/{object_id}.
// Matches the SPA's `deleteObject(typeId, objectId)` contract.
func (h *Handlers) DeleteObjectByOntologyType(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromRequest(r)
	objID := storage.ObjectId(chi.URLParam(r, "object_id"))
	deleted, err := h.Objects.Delete(r.Context(), tenant, objID)
	if err != nil {
		writeError(w, err)
		return
	}
	if !deleted {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// queryFilter mirrors the WorkshopVariable.static_filter shape from
// apps/web/src/routes/apps/WorkshopEditorPage.tsx — same operators, same
// JSON keys, so the SPA can forward filters verbatim.
type queryFilter struct {
	PropertyName string `json:"property_name"`
	Operator     string `json:"operator,omitempty"` // equals | contains | gte | lte | gt | lt | in
	Value        any    `json:"value"`
}

type querySort struct {
	PropertyName string `json:"property_name"`
	Direction    string `json:"direction,omitempty"`
}

type queryAggregation struct {
	ID           string `json:"id,omitempty"`
	Alias        string `json:"alias,omitempty"`
	Function     string `json:"function"`
	PropertyName string `json:"property_name,omitempty"`
}

type queryAggregationResult struct {
	ID           string `json:"id"`
	Alias        string `json:"alias,omitempty"`
	Function     string `json:"function"`
	PropertyName string `json:"property_name,omitempty"`
	Value        any    `json:"value"`
	Count        int    `json:"count"`
}

type querySearchAround struct {
	SourceObjectIDs    []string `json:"source_object_ids"`
	LinkTypeID         string   `json:"link_type_id,omitempty"`
	LinkTypeIDs        []string `json:"link_type_ids"`
	Direction          string   `json:"direction,omitempty"`
	Depth              int      `json:"depth,omitempty"`
	TargetObjectTypeID string   `json:"target_object_type_id,omitempty"`
}

type linkedEdgeResponse struct {
	LinkID         string         `json:"link_id"`
	LinkTypeID     string         `json:"link_type_id"`
	SourceObjectID string         `json:"source_object_id"`
	TargetObjectID string         `json:"target_object_id"`
	Direction      string         `json:"direction"`
	Depth          int            `json:"depth"`
	Properties     map[string]any `json:"properties,omitempty"`
}

type queryKNN struct {
	PropertyName      string    `json:"property_name,omitempty"`
	Property          string    `json:"property,omitempty"`
	VectorProperty    string    `json:"vector_property,omitempty"`
	EmbeddingProperty string    `json:"embedding_property,omitempty"`
	Vector            []float64 `json:"vector,omitempty"`
	QueryVector       []float64 `json:"query_vector,omitempty"`
	K                 int       `json:"k,omitempty"`
	KValue            int       `json:"k_value,omitempty"`
	KValueCamel       int       `json:"kValue,omitempty"`
	Metric            string    `json:"metric,omitempty"`
}

type queryKNNResult struct {
	ObjectID     string  `json:"object_id"`
	Rank         int     `json:"rank"`
	Score        float64 `json:"score"`
	Distance     float64 `json:"distance"`
	Metric       string  `json:"metric"`
	PropertyName string  `json:"property_name"`
}

type queryRequest struct {
	// Filters is the WorkshopVariable.static_filter[s] shape — the richer
	// form with `operator` per filter.
	Filters []queryFilter `json:"filters"`
	// Equals is the existing SPA shape used by lib/api/ontology.ts:queryObjects
	// — a flat map { property: expected }. Treated as "equals" filters.
	Equals  map[string]any `json:"equals"`
	Page    int            `json:"page"`
	PerPage uint32         `json:"per_page"`
	// Limit mirrors the SPA's `queryObjects` body: cap on items when no
	// per_page is set. We unify with per_page below.
	Limit             int                `json:"limit"`
	Sort              []querySort        `json:"sort"`
	IncludeCount      bool               `json:"include_count"`
	Aggregations      []queryAggregation `json:"aggregations"`
	SelectedObjectIDs []string           `json:"selected_object_ids"`
	SearchAround      *querySearchAround `json:"search_around,omitempty"`
	KNN               *queryKNN          `json:"knn,omitempty"`
}

func matchesFilter(props map[string]any, f queryFilter) bool {
	actual, ok := props[f.PropertyName]
	if !ok {
		// "not present" matches "" target so the SPA's `equals ""` checks work.
		actualStr := ""
		expectedStr := strings.ToLower(strings.TrimSpace(toStringValue(f.Value)))
		switch strings.ToLower(strings.TrimSpace(f.Operator)) {
		case "is_empty":
			return true
		case "is_not_empty":
			return false
		case "not_equals", "neq", "!=":
			return !strings.EqualFold(actualStr, expectedStr)
		default:
			return strings.EqualFold(actualStr, expectedStr)
		}
	}
	actualStr := strings.ToLower(strings.TrimSpace(toStringValue(actual)))
	expectedStr := strings.ToLower(strings.TrimSpace(toStringValue(f.Value)))
	switch strings.ToLower(strings.TrimSpace(f.Operator)) {
	case "contains":
		return strings.Contains(actualStr, expectedStr)
	case "not_equals", "neq", "!=":
		return actualStr != expectedStr
	case "gte", ">=":
		return compareFilterValues(actual, f.Value) >= 0
	case "lte", "<=":
		return compareFilterValues(actual, f.Value) <= 0
	case "gt", ">":
		return compareFilterValues(actual, f.Value) > 0
	case "lt", "<":
		return compareFilterValues(actual, f.Value) < 0
	case "in":
		return filterValueContains(f.Value, actual)
	case "is_empty":
		return strings.TrimSpace(toStringValue(actual)) == ""
	case "is_not_empty":
		return strings.TrimSpace(toStringValue(actual)) != ""
	default: // "equals" + unknown
		return actualStr == expectedStr
	}
}

func compareFilterValues(actual any, expected any) int {
	actualNumber, actualOK := numericFilterValue(actual)
	expectedNumber, expectedOK := numericFilterValue(expected)
	if actualOK && expectedOK {
		switch {
		case actualNumber < expectedNumber:
			return -1
		case actualNumber > expectedNumber:
			return 1
		default:
			return 0
		}
	}
	return strings.Compare(
		strings.ToLower(strings.TrimSpace(toStringValue(actual))),
		strings.ToLower(strings.TrimSpace(toStringValue(expected))),
	)
}

func numericFilterValue(v any) (float64, bool) {
	switch t := v.(type) {
	case json.Number:
		n, err := t.Float64()
		return n, err == nil
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case int32:
		return float64(t), true
	case uint:
		return float64(t), true
	case uint64:
		return float64(t), true
	case uint32:
		return float64(t), true
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func filterValueContains(container any, actual any) bool {
	actualStr := strings.ToLower(strings.TrimSpace(toStringValue(actual)))
	switch values := container.(type) {
	case []any:
		for _, value := range values {
			if strings.ToLower(strings.TrimSpace(toStringValue(value))) == actualStr {
				return true
			}
		}
	case []string:
		for _, value := range values {
			if strings.ToLower(strings.TrimSpace(value)) == actualStr {
				return true
			}
		}
	default:
		return strings.ToLower(strings.TrimSpace(toStringValue(container))) == actualStr
	}
	return false
}

func toStringValue(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case json.Number:
		return t.String()
	case float64:
		// avoid scientific notation for integral floats
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		// fall back to JSON encoding — covers nested objects and arrays
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func sortOntologyObjects(items []ontologyObject, sorts []querySort) {
	sorts = compactQuerySorts(sorts)
	if len(sorts) == 0 {
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		for _, item := range sorts {
			cmp := compareFilterValues(items[i].Properties[item.PropertyName], items[j].Properties[item.PropertyName])
			if cmp == 0 {
				continue
			}
			if isQuerySortDescending(item.Direction) {
				return cmp > 0
			}
			return cmp < 0
		}
		return strings.Compare(items[i].ID, items[j].ID) < 0
	})
}

func compactQuerySorts(sorts []querySort) []querySort {
	out := make([]querySort, 0, len(sorts))
	for _, item := range sorts {
		propertyName := strings.TrimSpace(item.PropertyName)
		if propertyName == "" {
			continue
		}
		direction := "asc"
		if isQuerySortDescending(item.Direction) {
			direction = "desc"
		}
		out = append(out, querySort{PropertyName: propertyName, Direction: direction})
	}
	return out
}

func isQuerySortDescending(direction string) bool {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "desc", "descending", "-1":
		return true
	default:
		return false
	}
}

func computeObjectQueryAggregations(items []ontologyObject, specs []queryAggregation) []queryAggregationResult {
	out := make([]queryAggregationResult, 0, len(specs))
	for _, spec := range specs {
		fn := normalizeQueryAggregationFunction(spec.Function)
		if fn == "" {
			continue
		}
		propertyName := strings.TrimSpace(spec.PropertyName)
		id := strings.TrimSpace(spec.ID)
		if id == "" {
			id = strings.TrimSpace(spec.Alias)
		}
		if id == "" && propertyName != "" {
			id = fn + ":" + propertyName
		}
		if id == "" {
			id = fn
		}

		result := queryAggregationResult{
			ID:           id,
			Alias:        strings.TrimSpace(spec.Alias),
			Function:     fn,
			PropertyName: propertyName,
		}
		switch fn {
		case "count":
			count := len(items)
			if propertyName != "" {
				count = 0
				for _, item := range items {
					if !isQueryEmptyValue(item.Properties[propertyName]) {
						count++
					}
				}
			}
			result.Value = count
			result.Count = count
		case "distinct_count", "approx_distinct":
			seen := map[string]struct{}{}
			for _, item := range items {
				value := item.ID
				if propertyName != "" {
					value = toStringValue(item.Properties[propertyName])
				}
				value = strings.ToLower(strings.TrimSpace(value))
				if value == "" {
					continue
				}
				seen[value] = struct{}{}
			}
			result.Value = len(seen)
			result.Count = len(seen)
		default:
			numbers := make([]float64, 0, len(items))
			for _, item := range items {
				if n, ok := numericFilterValue(item.Properties[propertyName]); ok {
					numbers = append(numbers, n)
				}
			}
			result.Count = len(numbers)
			if len(numbers) == 0 {
				result.Value = nil
				break
			}
			total := 0.0
			min := numbers[0]
			max := numbers[0]
			for _, n := range numbers {
				total += n
				if n < min {
					min = n
				}
				if n > max {
					max = n
				}
			}
			switch fn {
			case "sum":
				result.Value = total
			case "avg":
				result.Value = total / float64(len(numbers))
			case "min":
				result.Value = min
			case "max":
				result.Value = max
			default:
				continue
			}
		}
		out = append(out, result)
	}
	return out
}

func normalizeQueryAggregationFunction(fn string) string {
	switch strings.ToLower(strings.TrimSpace(fn)) {
	case "count":
		return "count"
	case "sum":
		return "sum"
	case "avg", "average", "mean":
		return "avg"
	case "min":
		return "min"
	case "max":
		return "max"
	case "distinct", "unique", "count_distinct", "distinct_count":
		return "distinct_count"
	case "approx_distinct":
		return "approx_distinct"
	default:
		return strings.ToLower(strings.TrimSpace(fn))
	}
}

func isQueryEmptyValue(value any) bool {
	return strings.TrimSpace(toStringValue(value)) == ""
}

func applyObjectKNN(items []ontologyObject, config queryKNN, fallbackK int) ([]ontologyObject, []queryKNNResult, map[string]any, error) {
	propertyName := normalizedKNNPropertyName(config)
	if propertyName == "" {
		return nil, nil, nil, &storage.RepoError{Kind: storage.ErrInvalidArgument, Msg: "knn requires a vector property_name"}
	}
	queryVector := normalizedKNNVector(config)
	if len(queryVector) == 0 {
		return nil, nil, nil, &storage.RepoError{Kind: storage.ErrInvalidArgument, Msg: "knn requires a non-empty vector"}
	}
	if len(queryVector) > 2048 {
		return nil, nil, nil, &storage.RepoError{Kind: storage.ErrInvalidArgument, Msg: "knn vector dimension cannot exceed 2048"}
	}
	k, err := normalizedKNNK(config, fallbackK)
	if err != nil {
		return nil, nil, nil, err
	}
	metric := normalizeKNNMetric(config.Metric)
	type candidate struct {
		item     ontologyObject
		score    float64
		distance float64
	}
	candidates := []candidate{}
	for _, item := range items {
		vector, ok := numericVectorValue(item.Properties[propertyName])
		if !ok || len(vector) != len(queryVector) {
			continue
		}
		score, distance, ok := scoreKNNVectors(queryVector, vector, metric)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate{item: item, score: score, distance: distance})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return strings.Compare(candidates[i].item.ID, candidates[j].item.ID) < 0
	})
	if len(candidates) > k {
		candidates = candidates[:k]
	}
	out := make([]ontologyObject, 0, len(candidates))
	results := make([]queryKNNResult, 0, len(candidates))
	for idx, candidate := range candidates {
		item := candidate.item
		if item.Properties == nil {
			item.Properties = map[string]any{}
		}
		rank := idx + 1
		item.Properties["__of_knn_rank"] = rank
		item.Properties["__of_knn_score"] = candidate.score
		item.Properties["__of_knn_distance"] = candidate.distance
		item.Properties["__of_knn_metric"] = metric
		out = append(out, item)
		results = append(results, queryKNNResult{
			ObjectID:     item.ID,
			Rank:         rank,
			Score:        candidate.score,
			Distance:     candidate.distance,
			Metric:       metric,
			PropertyName: propertyName,
		})
	}
	return out, results, map[string]any{
		"property_name":           propertyName,
		"k":                       k,
		"metric":                  metric,
		"query_vector_dimension":  len(queryVector),
		"matched_vector_count":    len(candidates),
		"max_supported_dimension": 2048,
	}, nil
}

func normalizedKNNPropertyName(config queryKNN) string {
	for _, value := range []string{config.PropertyName, config.Property, config.VectorProperty, config.EmbeddingProperty} {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizedKNNVector(config queryKNN) []float64 {
	if len(config.Vector) > 0 {
		return config.Vector
	}
	return config.QueryVector
}

func normalizedKNNK(config queryKNN, fallback int) (int, error) {
	k := config.K
	if k == 0 {
		k = config.KValue
	}
	if k == 0 {
		k = config.KValueCamel
	}
	explicit := k != 0
	if k < 0 {
		return 0, &storage.RepoError{Kind: storage.ErrInvalidArgument, Msg: "knn k must be positive"}
	}
	if k == 0 {
		k = fallback
	}
	if k <= 0 {
		k = 10
	}
	if k > 100 {
		if !explicit {
			return 100, nil
		}
		return 0, &storage.RepoError{Kind: storage.ErrInvalidArgument, Msg: "knn k cannot exceed 100"}
	}
	return k, nil
}

func normalizeKNNMetric(metric string) string {
	switch strings.ToLower(strings.TrimSpace(metric)) {
	case "euclidean", "l2", "distance":
		return "euclidean"
	case "dot", "dot_product", "inner_product":
		return "dot"
	default:
		return "cosine"
	}
}

func numericVectorValue(value any) ([]float64, bool) {
	switch typed := value.(type) {
	case []float64:
		return typed, len(typed) > 0
	case []int:
		out := make([]float64, 0, len(typed))
		for _, entry := range typed {
			out = append(out, float64(entry))
		}
		return out, len(out) > 0
	case []any:
		out := make([]float64, 0, len(typed))
		for _, entry := range typed {
			number, ok := numericFilterValue(entry)
			if !ok {
				return nil, false
			}
			out = append(out, number)
		}
		return out, len(out) > 0
	case string:
		var out []float64
		if err := json.Unmarshal([]byte(strings.TrimSpace(typed)), &out); err != nil {
			return nil, false
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

func scoreKNNVectors(query []float64, candidate []float64, metric string) (float64, float64, bool) {
	if len(query) == 0 || len(query) != len(candidate) {
		return 0, 0, false
	}
	switch metric {
	case "euclidean":
		total := 0.0
		for i := range query {
			delta := query[i] - candidate[i]
			total += delta * delta
		}
		distance := math.Sqrt(total)
		return 1 / (1 + distance), distance, true
	case "dot":
		score := 0.0
		for i := range query {
			score += query[i] * candidate[i]
		}
		return score, -score, true
	default:
		dot := 0.0
		queryNorm := 0.0
		candidateNorm := 0.0
		for i := range query {
			dot += query[i] * candidate[i]
			queryNorm += query[i] * query[i]
			candidateNorm += candidate[i] * candidate[i]
		}
		if queryNorm == 0 || candidateNorm == 0 {
			return 0, 0, false
		}
		score := dot / (math.Sqrt(queryNorm) * math.Sqrt(candidateNorm))
		return score, 1 - score, true
	}
}

// QueryObjectsByOntologyType serves POST /api/v1/ontology/types/{type_id}/objects/query.
// Server-side filter pushdown for the SPA's WorkshopVariable.static_filter / static_filters.
// Today the InMemory store can't filter natively (no native CQL); we materialise
// the full per-tenant+type list and filter in Go. For Cassandra this should be
// replaced with a secondary-index lookup (`SELECT … WHERE type_id=? AND property_name=value`)
// once the schema supports it.
func (h *Handlers) QueryObjectsByOntologyType(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromRequest(r)
	typeID := storage.TypeId(chi.URLParam(r, "type_id"))

	var body queryRequest
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Merge the two filter shapes: `equals` map gets normalised into the
	// richer Filters list as plain equals.
	for k, v := range body.Equals {
		body.Filters = append(body.Filters, queryFilter{PropertyName: k, Operator: "equals", Value: v})
	}
	perPage := body.PerPage
	if perPage == 0 && body.Limit > 0 {
		perPage = uint32(body.Limit)
	}
	if perPage == 0 {
		perPage = 25
	}
	if perPage > 5000 {
		perPage = 5000
	}
	page := body.Page
	if page < 1 {
		page = 1
	}

	full, err := h.Objects.ListByType(r.Context(), tenant, typeID, storage.Page{Size: 1_000_000}, parseConsistency(r.URL.Query().Get("consistency")))
	if err != nil {
		writeError(w, err)
		return
	}

	// `objects_by_type` is append-only: every Put fans out a new row
	// keyed on (type_id, updated_at, object_id). After an in-place
	// property update the table holds both the old and the new
	// summary for the same object_id. Dedupe by keeping the most
	// recently updated row per object_id before applying the filter
	// — otherwise stale `properties_summary` values pollute counts.
	latestByID := map[storage.ObjectId]int{}
	for i := range full.Items {
		obj := &full.Items[i]
		if prev, ok := latestByID[obj.ID]; ok {
			if full.Items[prev].UpdatedAtMs >= obj.UpdatedAtMs {
				continue
			}
		}
		latestByID[obj.ID] = i
	}
	deduped := make([]int, 0, len(latestByID))
	for _, idx := range latestByID {
		deduped = append(deduped, idx)
	}
	selectedIDs := map[string]bool{}
	for _, id := range body.SelectedObjectIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			selectedIDs[trimmed] = true
		}
	}
	linkedEdges := []linkedEdgeResponse{}
	searchAroundContract := map[string]any(nil)
	if body.SearchAround != nil {
		searchIDs, edges, err := h.resolveLinkedSearchAround(r, tenant, typeID, *body.SearchAround)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(selectedIDs) > 0 {
			for id := range selectedIDs {
				if !searchIDs[id] {
					delete(selectedIDs, id)
				}
			}
		} else {
			selectedIDs = searchIDs
		}
		linkedEdges = edges
		searchAroundContract = map[string]any{
			"source_object_ids":     compactStrings(body.SearchAround.SourceObjectIDs),
			"link_type_ids":         normalizedSearchAroundLinkTypeIDs(*body.SearchAround),
			"direction":             normalizeSearchAroundDirection(body.SearchAround.Direction),
			"depth":                 clampSearchAroundDepth(body.SearchAround.Depth),
			"target_object_type_id": string(typeID),
		}
	}

	matched := make([]ontologyObject, 0)
	for _, idx := range deduped {
		obj := &full.Items[idx]
		if len(selectedIDs) > 0 && !selectedIDs[string(obj.ID)] {
			continue
		}
		props := map[string]any{}
		if len(obj.Payload) > 0 {
			_ = json.Unmarshal(obj.Payload, &props)
		}
		ok := true
		for _, f := range body.Filters {
			if !matchesFilter(props, f) {
				ok = false
				break
			}
		}
		if ok {
			matched = append(matched, toOntologyObject(obj))
		}
	}

	knnResults := []queryKNNResult(nil)
	knnContract := map[string]any(nil)
	if body.KNN != nil {
		var err error
		matched, knnResults, knnContract, err = applyObjectKNN(matched, *body.KNN, int(perPage))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		sortOntologyObjects(matched, body.Sort)
	}
	aggregations := computeObjectQueryAggregations(matched, body.Aggregations)
	total := len(matched)
	start := (page - 1) * int(perPage)
	end := start + int(perPage)
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	pageItems := matched[start:end]
	pageIDs := map[string]bool{}
	for _, item := range pageItems {
		pageIDs[item.ID] = true
	}
	pageLinkedEdges := linkedEdges[:0]
	for _, edge := range linkedEdges {
		if pageIDs[edge.SourceObjectID] || pageIDs[edge.TargetObjectID] {
			pageLinkedEdges = append(pageLinkedEdges, edge)
		}
	}
	pageKNNResults := knnResults[:0]
	for _, result := range knnResults {
		if pageIDs[result.ObjectID] {
			pageKNNResults = append(pageKNNResults, result)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":         pageItems,
		"total":        total,
		"count":        total,
		"page":         page,
		"per_page":     perPage,
		"aggregations": aggregations,
		"linked_edges": pageLinkedEdges,
		"knn_results":  pageKNNResults,
		"object_set": map[string]any{
			"object_type_id":      string(typeID),
			"filters":             body.Filters,
			"sort":                compactQuerySorts(body.Sort),
			"limit":               perPage,
			"include_count":       body.IncludeCount,
			"aggregations":        body.Aggregations,
			"selected_object_ids": body.SelectedObjectIDs,
			"search_around":       searchAroundContract,
			"knn":                 knnContract,
		},
	})
}

func (h *Handlers) resolveLinkedSearchAround(
	r *http.Request,
	tenant storage.TenantId,
	targetTypeID storage.TypeId,
	config querySearchAround,
) (map[string]bool, []linkedEdgeResponse, error) {
	sourceIDs := compactStrings(config.SourceObjectIDs)
	if len(sourceIDs) == 0 {
		return map[string]bool{}, []linkedEdgeResponse{}, nil
	}
	linkTypeIDs := normalizedSearchAroundLinkTypeIDs(config)
	if len(linkTypeIDs) == 0 {
		return nil, nil, &storage.RepoError{Kind: storage.ErrInvalidArgument, Msg: "search_around requires at least one link_type_id"}
	}
	direction := normalizeSearchAroundDirection(config.Direction)
	depth := clampSearchAroundDepth(config.Depth)
	limit := 5000
	targetIDs := map[string]bool{}
	edges := []linkedEdgeResponse{}
	seenEdges := map[string]bool{}
	seenNodes := map[string]bool{}
	frontier := sourceIDs
	for _, id := range sourceIDs {
		seenNodes[id] = true
	}

	for currentDepth := 1; currentDepth <= depth && len(frontier) > 0 && len(targetIDs) < limit; currentDepth++ {
		nextFrontier := []string{}
		for _, nodeID := range frontier {
			for _, linkTypeID := range linkTypeIDs {
				if direction == "outgoing" || direction == "both" {
					items, err := collectOutgoingSearchAroundLinks(r, h.Links, tenant, storage.LinkTypeId(linkTypeID), storage.ObjectId(nodeID), limit-len(edges))
					if err != nil {
						return nil, nil, err
					}
					for _, link := range items {
						edge, neighbor := linkedSearchAroundEdge(link, "outgoing", currentDepth)
						if !seenEdges[edge.LinkID] {
							seenEdges[edge.LinkID] = true
							edges = append(edges, edge)
						}
						if !seenNodes[neighbor] {
							seenNodes[neighbor] = true
							nextFrontier = append(nextFrontier, neighbor)
						}
						targetIDs[neighbor] = true
					}
				}
				if direction == "incoming" || direction == "both" {
					items, err := collectIncomingSearchAroundLinks(r, h.Links, tenant, storage.LinkTypeId(linkTypeID), storage.ObjectId(nodeID), limit-len(edges))
					if err != nil {
						return nil, nil, err
					}
					for _, link := range items {
						edge, neighbor := linkedSearchAroundEdge(link, "incoming", currentDepth)
						if !seenEdges[edge.LinkID] {
							seenEdges[edge.LinkID] = true
							edges = append(edges, edge)
						}
						if !seenNodes[neighbor] {
							seenNodes[neighbor] = true
							nextFrontier = append(nextFrontier, neighbor)
						}
						targetIDs[neighbor] = true
					}
				}
			}
		}
		frontier = nextFrontier
	}

	if config.TargetObjectTypeID != "" && storage.TypeId(strings.TrimSpace(config.TargetObjectTypeID)) != targetTypeID {
		targetIDs = map[string]bool{}
	}
	return targetIDs, edges, nil
}

func collectOutgoingSearchAroundLinks(r *http.Request, links storage.LinkStore, tenant storage.TenantId, linkType storage.LinkTypeId, from storage.ObjectId, budget int) ([]storage.Link, error) {
	if budget <= 0 {
		return nil, nil
	}
	res, err := links.ListOutgoing(r.Context(), tenant, linkType, from, storage.Page{Size: uint32(clampInt(budget, 1, 5000))}, parseConsistency(r.URL.Query().Get("consistency")))
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}

func collectIncomingSearchAroundLinks(r *http.Request, links storage.LinkStore, tenant storage.TenantId, linkType storage.LinkTypeId, to storage.ObjectId, budget int) ([]storage.Link, error) {
	if budget <= 0 {
		return nil, nil
	}
	res, err := links.ListIncoming(r.Context(), tenant, linkType, to, storage.Page{Size: uint32(clampInt(budget, 1, 5000))}, parseConsistency(r.URL.Query().Get("consistency")))
	if err != nil {
		return nil, err
	}
	return res.Items, nil
}

func linkedSearchAroundEdge(link storage.Link, direction string, depth int) (linkedEdgeResponse, string) {
	props := map[string]any{}
	if link.Payload != nil && len(*link.Payload) > 0 {
		_ = json.Unmarshal(*link.Payload, &props)
	}
	edge := linkedEdgeResponse{
		LinkID:         stableSearchAroundLinkID(link),
		LinkTypeID:     string(link.LinkType),
		SourceObjectID: string(link.From),
		TargetObjectID: string(link.To),
		Direction:      direction,
		Depth:          depth,
		Properties:     props,
	}
	if direction == "incoming" {
		return edge, string(link.From)
	}
	return edge, string(link.To)
}

func stableSearchAroundLinkID(link storage.Link) string {
	return strings.Join([]string{string(link.LinkType), string(link.From), string(link.To)}, ":")
}

func normalizedSearchAroundLinkTypeIDs(config querySearchAround) []string {
	ids := compactStrings(config.LinkTypeIDs)
	if strings.TrimSpace(config.LinkTypeID) != "" {
		ids = append(ids, strings.TrimSpace(config.LinkTypeID))
	}
	return compactStrings(ids)
}

func normalizeSearchAroundDirection(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "incoming", "inbound":
		return "incoming"
	case "both", "any", "all":
		return "both"
	default:
		return "outgoing"
	}
}

func clampSearchAroundDepth(depth int) int {
	return clampInt(depth, 1, 5)
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func compactStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

// CreateObjectByOntologyType serves POST /api/v1/ontology/types/{type_id}/objects.
// Body shape: `{ properties: {...} }`. Used by the SPA to seed manual rows; the
// real bulk path is the indexer (see docs/poc-online-retail/RUNTIME-INDEXER.md).
func (h *Handlers) CreateObjectByOntologyType(w http.ResponseWriter, r *http.Request) {
	tenant := tenantFromRequest(r)
	typeID := storage.TypeId(chi.URLParam(r, "type_id"))

	var body struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload, err := json.Marshal(body.Properties)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	now := time.Now().UnixMilli()
	// Mint a TimeUUID (UUIDv1) so the value is accepted by the Cassandra
	// `object_id timeuuid` column. UUIDv4 from google/uuid would be rejected by
	// gocql with "Invalid version for TimeUUID type".
	id := storage.ObjectId(gocql.TimeUUID().String())
	owner := storage.OwnerId(strings.TrimSpace(string(tenant)))
	if claims, ok := authmw.FromContext(r.Context()); ok && claims != nil {
		owner = storage.OwnerId(claims.Sub.String())
	} else if hdr := r.Header.Get("Authorization"); strings.HasPrefix(hdr, "Bearer ") {
		// The gateway already validated the JWT; here we only decode the
		// subject (UUID) to use as the object owner. Best-effort — if it
		// fails we fall through to the deterministic PoC fallback below.
		if sub := jwtSubjectUnverified(strings.TrimPrefix(hdr, "Bearer ")); sub != "" {
			owner = storage.OwnerId(sub)
		}
	}
	if _, err := uuid.Parse(string(owner)); err != nil {
		// Deterministic PoC owner UUID — only used when the request is
		// unauthenticated (e.g. seed scripts or /healthz introspection).
		owner = storage.OwnerId("00000000-0000-1000-8000-000000000001")
	}
	obj := storage.Object{
		Tenant:      tenant,
		ID:          id,
		TypeID:      typeID,
		Version:     1,
		Payload:     payload,
		Owner:       &owner,
		CreatedAtMs: &now,
		UpdatedAtMs: now,
	}
	if _, err := h.Objects.Put(r.Context(), obj, nil); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toOntologyObject(&obj))
}

// jwtSubjectUnverified extracts the `sub` claim from a JWT without
// validating its signature. The gateway already authenticated the
// request; here we only need the user id to populate Object.Owner.
func jwtSubjectUnverified(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var payload struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Sub)
}
