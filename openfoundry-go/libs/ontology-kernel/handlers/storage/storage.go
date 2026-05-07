// Package storage ports `libs/ontology-kernel/src/handlers/storage.rs`
// 1:1: the `GetStorageInsights` endpoint that powers the
// `/api/v1/ontology/storage/insights` route.
//
// Wire-format parity is byte-identical: same field names + JSON
// tags as the Rust struct (`OntologyStorageInsightsResponse`), same
// fan-out into runtime + declarative metrics, same Cassandra index
// catalogue, same `database_backend` / `access_driver` / etc. labels.
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/google/uuid"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	ontologykernel "github.com/openfoundry/openfoundry-go/libs/ontology-kernel"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/domain"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/handlers/links"
	"github.com/openfoundry/openfoundry-go/libs/ontology-kernel/models"
	storage "github.com/openfoundry/openfoundry-go/libs/storage-abstraction"
)

// StorageTableMetric mirrors `pub struct StorageTableMetric`.
type StorageTableMetric struct {
	Key         string `json:"key"`
	TableName   string `json:"table_name"`
	Label       string `json:"label"`
	Role        string `json:"role"`
	RecordCount int64  `json:"record_count"`
}

// StorageDistributionMetric mirrors `pub struct StorageDistributionMetric`.
type StorageDistributionMetric struct {
	ID    uuid.UUID `json:"id"`
	Label string    `json:"label"`
	Count int64     `json:"count"`
}

// StorageSearchKindMetric mirrors `pub struct StorageSearchKindMetric`.
type StorageSearchKindMetric struct {
	Kind  string `json:"kind"`
	Count int64  `json:"count"`
}

// OntologyStorageInsightsResponse mirrors the Rust struct of the same
// name. Field names + JSON tags + ordering match verbatim.
type OntologyStorageInsightsResponse struct {
	DatabaseBackend        string                          `json:"database_backend"`
	AccessDriver           string                          `json:"access_driver"`
	GraphProjection        string                          `json:"graph_projection"`
	SearchProjection       string                          `json:"search_projection"`
	FunnelRuntime          string                          `json:"funnel_runtime"`
	TableMetrics           []StorageTableMetric            `json:"table_metrics"`
	IndexDefinitions       []domain.StorageIndexDefinition `json:"index_definitions"`
	ObjectTypeDistribution []StorageDistributionMetric     `json:"object_type_distribution"`
	LinkTypeDistribution   []StorageDistributionMetric     `json:"link_type_distribution"`
	SearchDocumentsTotal   int64                           `json:"search_documents_total"`
	SearchDocumentsByKind  []StorageSearchKindMetric       `json:"search_documents_by_kind"`
	// Rust `Option<DateTime<Utc>>` carries no skip_serializing_if, so
	// None serialises to `"latest_*_at":null`. Dropping `omitempty`
	// keeps the Go output byte-identical when no metrics exist yet.
	LatestObjectWriteAt *time.Time `json:"latest_object_write_at"`
	LatestLinkWriteAt   *time.Time `json:"latest_link_write_at"`
	LatestFunnelRunAt   *time.Time `json:"latest_funnel_run_at"`
}

// objectRuntimeMetrics / linkRuntimeMetrics / funnelRuntimeMetrics
// mirror the private Rust structs of the same names.
type objectRuntimeMetrics struct {
	total         int64
	distribution  []StorageDistributionMetric
	latestWriteAt *time.Time
}

type linkRuntimeMetrics struct {
	total         int64
	distribution  []StorageDistributionMetric
	latestWriteAt *time.Time
}

type funnelRuntimeMetrics struct {
	total       int64
	latestRunAt *time.Time
}

// GetStorageInsights mirrors `pub async fn get_storage_insights`.
//
// Pulls the same six metric streams the Rust impl does, serialises
// the same response envelope, and emits the same 500/Internal-server-
// error shape on any failure path. The handler is auth-required —
// the caller is responsible for the JWT extraction (the
// ontology-actions-service router does this in the chi middleware
// chain before reaching here).
func GetStorageInsights(state *ontologykernel.AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing claims"})
			return
		}

		ctx := r.Context()

		objectRuntime, err := loadObjectRuntimeMetrics(ctx, state, claims)
		if err != nil {
			dbError(w, fmt.Sprintf("failed to load object runtime metrics: %s", err))
			return
		}

		linkRuntime, err := loadLinkRuntimeMetrics(ctx, state, claims)
		if err != nil {
			dbError(w, fmt.Sprintf("failed to load link runtime metrics: %s", err))
			return
		}

		funnelRuntime, err := loadFunnelRuntimeMetrics(ctx, state, claims)
		if err != nil {
			dbError(w, fmt.Sprintf("failed to load funnel runtime metrics: %s", err))
			return
		}

		tableMetrics, err := loadTableMetrics(ctx, state, objectRuntime.total, linkRuntime.total, funnelRuntime.total)
		if err != nil {
			dbError(w, fmt.Sprintf("failed to load storage table metrics: %s", err))
			return
		}

		indexDefinitions, err := loadIndexDefinitions(ctx, state)
		if err != nil {
			dbError(w, fmt.Sprintf("failed to load storage index definitions: %s", err))
			return
		}

		searchTotal, searchByKind, err := loadSearchDocumentMetrics(ctx, state, claims)
		if err != nil {
			dbError(w, fmt.Sprintf("failed to load search document metrics: %s", err))
			return
		}

		latestObjectWriteAt := objectRuntime.latestWriteAt

		writeJSON(w, http.StatusOK, OntologyStorageInsightsResponse{
			DatabaseBackend:        "PostgreSQL + Cassandra",
			AccessDriver:           "repository + storage-abstraction",
			GraphProjection:        "link_types + LinkStore/Cassandra",
			SearchProjection:       "domain::indexer search documents",
			FunnelRuntime:          "ontology_funnel_sources + actions_log.actions_log",
			TableMetrics:           tableMetrics,
			IndexDefinitions:       indexDefinitions,
			ObjectTypeDistribution: objectRuntime.distribution,
			LinkTypeDistribution:   linkRuntime.distribution,
			SearchDocumentsTotal:   searchTotal,
			SearchDocumentsByKind:  searchByKind,
			LatestObjectWriteAt:    latestObjectWriteAt,
			LatestLinkWriteAt:      linkRuntime.latestWriteAt,
			LatestFunnelRunAt:      funnelRuntime.latestRunAt,
		})
	}
}

// ── Per-metric loaders (1:1 with the private Rust functions) ────────

func loadObjectRuntimeMetrics(
	ctx context.Context, state *ontologykernel.AppState, claims *authmw.Claims,
) (objectRuntimeMetrics, error) {
	tenant := domain.TenantFromClaims(claims)
	objectTypes, err := loadObjectTypesForStorage(ctx, state)
	if err != nil {
		return objectRuntimeMetrics{}, fmt.Errorf("failed to load object type metadata: %w", err)
	}
	var (
		total         int64
		latestWriteAt *time.Time
		distribution  = make([]StorageDistributionMetric, 0, len(objectTypes))
	)

	for _, ot := range objectTypes {
		var count int64
		var token *string
		for {
			page, err := state.Stores.Objects.ListByType(
				ctx, tenant, storage.TypeId(ot.ID.String()),
				storage.Page{Size: 200, Token: token},
				storage.Strong(),
			)
			if err != nil {
				return objectRuntimeMetrics{}, fmt.Errorf("failed to list object runtime metrics for type %s: %w", ot.ID, err)
			}
			for _, obj := range page.Items {
				count++
				ts := time.UnixMilli(obj.UpdatedAtMs).UTC()
				if latestWriteAt == nil || ts.After(*latestWriteAt) {
					tt := ts
					latestWriteAt = &tt
				}
			}
			if page.NextToken == nil {
				break
			}
			token = page.NextToken
		}
		total += count
		distribution = append(distribution, StorageDistributionMetric{
			ID:    ot.ID,
			Label: ot.DisplayName,
			Count: count,
		})
	}

	sort.Slice(distribution, func(i, j int) bool {
		if distribution[i].Count != distribution[j].Count {
			return distribution[i].Count > distribution[j].Count
		}
		return distribution[i].Label < distribution[j].Label
	})
	if len(distribution) > 8 {
		distribution = distribution[:8]
	}

	return objectRuntimeMetrics{
		total:         total,
		distribution:  distribution,
		latestWriteAt: latestWriteAt,
	}, nil
}

func loadLinkRuntimeMetrics(
	ctx context.Context, state *ontologykernel.AppState, claims *authmw.Claims,
) (linkRuntimeMetrics, error) {
	tenant := domain.TenantFromClaims(claims)
	linkTypes, err := loadLinkTypesForStorage(ctx, state)
	if err != nil {
		return linkRuntimeMetrics{}, fmt.Errorf("failed to load link type metadata: %w", err)
	}
	var (
		total         int64
		latestWriteAt *time.Time
		distribution  = make([]StorageDistributionMetric, 0, len(linkTypes))
	)

	for i := range linkTypes {
		lt := linkTypes[i]
		instances, err := links.CollectLinkInstancesForType(ctx, state, tenant, &lt)
		if err != nil {
			return linkRuntimeMetrics{}, err
		}
		count := int64(len(instances))
		total += count
		for _, inst := range instances {
			ts := inst.CreatedAt
			if latestWriteAt == nil || ts.After(*latestWriteAt) {
				tt := ts
				latestWriteAt = &tt
			}
		}
		distribution = append(distribution, StorageDistributionMetric{
			ID:    lt.ID,
			Label: lt.DisplayName,
			Count: count,
		})
	}

	sort.Slice(distribution, func(i, j int) bool {
		if distribution[i].Count != distribution[j].Count {
			return distribution[i].Count > distribution[j].Count
		}
		return distribution[i].Label < distribution[j].Label
	})
	if len(distribution) > 8 {
		distribution = distribution[:8]
	}

	return linkRuntimeMetrics{
		total:         total,
		distribution:  distribution,
		latestWriteAt: latestWriteAt,
	}, nil
}

func loadFunnelRuntimeMetrics(
	ctx context.Context, state *ontologykernel.AppState, claims *authmw.Claims,
) (funnelRuntimeMetrics, error) {
	tenant := domain.TenantFromClaims(claims)
	runs, err := domain.ListRunsForTenant(ctx, state.Stores.Actions, tenant)
	if err != nil {
		return funnelRuntimeMetrics{}, fmt.Errorf("failed to list funnel runtime metrics: %w", err)
	}
	var latestRunAt *time.Time
	for i := range runs {
		ts := runs[i].StartedAt
		if latestRunAt == nil || ts.After(*latestRunAt) {
			tt := ts
			latestRunAt = &tt
		}
	}
	return funnelRuntimeMetrics{
		total:       int64(len(runs)),
		latestRunAt: latestRunAt,
	}, nil
}

func loadTableMetrics(
	ctx context.Context, state *ontologykernel.AppState,
	objectRuntimeTotal, linkRuntimeTotal, funnelRuntimeTotal int64,
) ([]StorageTableMetric, error) {
	counts, err := loadDefinitionCountsForStorage(ctx, state)
	if err != nil {
		return nil, err
	}
	return []StorageTableMetric{
		{Key: "object_types", TableName: "object_types", Label: "Object types", Role: "Schema", RecordCount: counts.ObjectTypes},
		{Key: "properties", TableName: "properties", Label: "Properties", Role: "Schema", RecordCount: counts.Properties},
		{Key: "link_types", TableName: "link_types", Label: "Link types", Role: "Schema", RecordCount: counts.LinkTypes},
		{Key: "interfaces", TableName: "ontology_interfaces", Label: "Interfaces", Role: "Schema", RecordCount: counts.Interfaces},
		{Key: "interface_properties", TableName: "interface_properties", Label: "Interface properties", Role: "Schema", RecordCount: counts.InterfaceProperties},
		{Key: "shared_property_types", TableName: "shared_property_types", Label: "Shared property types", Role: "Schema", RecordCount: counts.SharedPropertyTypes},
		{Key: "action_types", TableName: "action_types", Label: "Action types", Role: "Runtime", RecordCount: counts.ActionTypes},
		{Key: "function_packages", TableName: "ontology_function_packages", Label: "Function packages", Role: "Runtime", RecordCount: counts.FunctionPackages},
		{Key: "object_instances", TableName: "ontology_objects.objects_by_id", Label: "Object rows", Role: "Runtime", RecordCount: objectRuntimeTotal},
		{Key: "link_instances", TableName: "links_outgoing + links_incoming", Label: "Link rows", Role: "Runtime", RecordCount: linkRuntimeTotal},
		{Key: "object_sets", TableName: "ontology_object_sets", Label: "Object sets", Role: "Runtime", RecordCount: counts.ObjectSets},
		{Key: "funnel_sources", TableName: "ontology_funnel_sources", Label: "Funnel sources", Role: "Ingestion", RecordCount: counts.FunnelSources},
		{Key: "funnel_runs", TableName: "actions_log.actions_log(kind=funnel_run)", Label: "Funnel runs", Role: "Ingestion", RecordCount: funnelRuntimeTotal},
		{Key: "projects", TableName: "ontology_projects", Label: "Ontology projects", Role: "Governance", RecordCount: counts.Projects},
	}, nil
}

func loadIndexDefinitions(
	ctx context.Context, state *ontologykernel.AppState,
) ([]domain.StorageIndexDefinition, error) {
	pgDefs := []domain.StorageIndexDefinition{}
	if state.DB != nil {
		var err error
		pgDefs, err = domain.LoadPGIndexDefinitions(ctx, state.DB)
		if err != nil {
			return nil, err
		}
	}
	combined := append(pgDefs, domain.CassandraIndexDefinitions()...)
	sort.Slice(combined, func(i, j int) bool {
		if combined[i].TableName != combined[j].TableName {
			return combined[i].TableName < combined[j].TableName
		}
		return combined[i].IndexName < combined[j].IndexName
	})
	return combined, nil
}

func loadSearchDocumentMetrics(
	ctx context.Context, state *ontologykernel.AppState, claims *authmw.Claims,
) (int64, []StorageSearchKindMetric, error) {
	if state.DB == nil {
		objectRuntime, err := loadObjectRuntimeMetrics(ctx, state, claims)
		if err != nil {
			return 0, nil, err
		}
		if objectRuntime.total == 0 {
			return 0, []StorageSearchKindMetric{}, nil
		}
		return objectRuntime.total, []StorageSearchKindMetric{{Kind: "object", Count: objectRuntime.total}}, nil
	}
	docs, err := domain.BuildSearchDocuments(ctx, state, claims, nil, nil)
	if err != nil {
		return 0, nil, err
	}
	byKind := map[string]int64{}
	for _, d := range docs {
		byKind[d.Kind]++
	}
	out := make([]StorageSearchKindMetric, 0, len(byKind))
	for kind, count := range byKind {
		out = append(out, StorageSearchKindMetric{Kind: kind, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Kind < out[j].Kind
	})
	return int64(len(docs)), out, nil
}

// ── HTTP plumbing ────────────────────────────────────────────────────

func dbError(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

// Compile-time guards — avoid the linter flagging unused imports
// during the iterative rollout (models is referenced through
// domain in the link-instance helper, but Go re-exports through
// types so the import is real).
var _ = models.LinkType{}

func loadObjectTypesForStorage(ctx context.Context, state *ontologykernel.AppState) ([]models.ObjectType, error) {
	if state.DB != nil {
		return domain.LoadObjectTypesAll(ctx, state.DB)
	}
	page, err := state.Stores.Definitions.List(ctx, storage.DefinitionQuery{Kind: storage.DefinitionKind(domain.ActionRepoObjectKind), Page: storage.Page{Size: 10_000}}, storage.Strong())
	if err != nil {
		return nil, err
	}
	out := make([]models.ObjectType, 0, len(page.Items))
	for _, rec := range page.Items {
		var item models.ObjectType
		if err := json.Unmarshal(rec.Payload, &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func loadLinkTypesForStorage(ctx context.Context, state *ontologykernel.AppState) ([]models.LinkType, error) {
	if state.DB != nil {
		return domain.LoadLinkTypesAll(ctx, state.DB)
	}
	page, err := state.Stores.Definitions.List(ctx, storage.DefinitionQuery{Kind: storage.DefinitionKind(domain.ActionRepoLinkTypeKind), Page: storage.Page{Size: 10_000}}, storage.Strong())
	if err != nil {
		return nil, err
	}
	out := make([]models.LinkType, 0, len(page.Items))
	for _, rec := range page.Items {
		var item models.LinkType
		if err := json.Unmarshal(rec.Payload, &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func loadDefinitionCountsForStorage(ctx context.Context, state *ontologykernel.AppState) (domain.DefinitionCounts, error) {
	if state.DB != nil {
		return domain.LoadDefinitionCounts(ctx, state.DB)
	}
	count := func(kind storage.DefinitionKind) (int64, error) {
		n, err := state.Stores.Definitions.Count(ctx, storage.DefinitionQuery{Kind: kind, Page: storage.Page{Size: 10_000}}, storage.Strong())
		return int64(n), err
	}
	var out domain.DefinitionCounts
	pairs := []struct {
		dst  *int64
		kind storage.DefinitionKind
	}{
		{&out.ObjectTypes, storage.DefinitionKind(domain.ActionRepoObjectKind)},
		{&out.Properties, storage.DefinitionKind(domain.ActionRepoPropertyKind)},
		{&out.LinkTypes, storage.DefinitionKind(domain.ActionRepoLinkTypeKind)},
		{&out.ActionTypes, storage.DefinitionKind(domain.ActionTypeKind)},
		{&out.FunnelSources, storage.DefinitionKind("funnel_source")},
	}
	for _, pair := range pairs {
		n, err := count(pair.kind)
		if err != nil {
			return domain.DefinitionCounts{}, err
		}
		*pair.dst = n
	}
	return out, nil
}
