package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/marketplace"
	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/models"
)

func TestHealthAliasMounted(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	srv := httptest.NewServer(BuildRouter(cfg, nil, nil, nil, observability.NewMetrics()))
	t.Cleanup(srv.Close)
	resp, err := srv.Client().Get(srv.URL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "federation-product-exchange-service", body["service"])
}

func TestMarketplaceV1BrowseAndPublishContracts(t *testing.T) {
	t.Parallel()
	srv, token := newMarketplaceTestServer(t)
	listing := createListingV1(t, srv.URL, token, map[string]any{"name": "Ops Widget", "slug": "ops-widget", "summary": "widget telemetry", "publisher": "Platform", "category_slug": "widgets", "package_kind": "widget", "repository_slug": "ops-widget", "tags": []string{"telemetry"}})

	var overview models.MarketplaceOverview
	decodeJSON(t, doJSON(t, "GET", srv.URL+"/v1/marketplace/overview", token, nil, 200), &overview)
	assert.Equal(t, 1, overview.ListingCount)
	assert.Equal(t, int64(0), overview.TotalInstalls)

	var categories models.ListResponse[models.CategoryDefinition]
	decodeJSON(t, doJSON(t, "GET", srv.URL+"/v1/marketplace/categories", token, nil, 200), &categories)
	require.NotEmpty(t, categories.Items)
	assert.Equal(t, "widgets", categories.Items[2].Slug)
	assert.Equal(t, 1, categories.Items[2].ListingCount)

	var list models.ListResponse[models.ListingDefinition]
	decodeJSON(t, doJSON(t, "GET", srv.URL+"/v1/marketplace/listings", token, nil, 200), &list)
	require.Len(t, list.Items, 1)
	assert.Equal(t, listing.ID, list.Items[0].ID)

	var search models.SearchResponse
	decodeJSON(t, doJSON(t, "GET", srv.URL+"/v1/marketplace/search?q=telemetry&category=widgets", token, nil, 200), &search)
	assert.Equal(t, "telemetry", search.Query)
	require.Len(t, search.Results, 1)
	assert.Greater(t, search.Results[0].Score, 0.45)

	var detail models.ListingDetail
	decodeJSON(t, doJSON(t, "GET", srv.URL+"/v1/marketplace/listings/"+listing.ID.String(), token, nil, 200), &detail)
	assert.Equal(t, listing.ID, detail.Listing.ID)

	var version models.PackageVersion
	decodeJSON(t, doJSON(t, "POST", srv.URL+"/v1/marketplace/listings/"+listing.ID.String()+"/versions", token, map[string]any{"version": "1.0.0", "changelog": "initial"}, 200), &version)
	assert.Equal(t, "1.0.0", version.Version)

	var versions models.ListResponse[models.PackageVersion]
	decodeJSON(t, doJSON(t, "GET", srv.URL+"/v1/marketplace/listings/"+listing.ID.String()+"/versions", token, nil, 200), &versions)
	require.Len(t, versions.Items, 1)
	assert.Equal(t, version.ID, versions.Items[0].ID)

	actionID := uuid.New()
	var updated models.PackageVersion
	decodeJSON(t, doJSON(t, "POST", srv.URL+"/v1/marketplace/listings/"+listing.ID.String()+"/actions", token, map[string]any{"action_type": map[string]any{"id": actionID.String(), "api_name": "approve"}, "dependencies": map[string]any{"object_type_ids": []string{uuid.New().String()}}}, 200), &updated)
	assert.Contains(t, string(updated.Manifest), "action_type")
}

func TestMarketplaceV1InstallAliasContract(t *testing.T) {
	t.Parallel()
	srv, token := newMarketplaceTestServer(t)
	listing := createListingV1(t, srv.URL, token, map[string]any{"name": "Runtime", "slug": "runtime-v1", "summary": "runtime", "publisher": "P", "category_slug": "connectors", "package_kind": "connector", "repository_slug": "runtime-v1"})
	doJSON(t, "POST", srv.URL+"/v1/marketplace/listings/"+listing.ID.String()+"/versions", token, map[string]any{"version": "1.0.0", "changelog": "initial"}, 200)
	var install models.InstallRecord
	decodeJSON(t, doJSON(t, "POST", srv.URL+"/v1/marketplace/installs", token, map[string]any{"listing_id": listing.ID.String(), "version": "1.0.0", "workspace_name": "staging"}, 200), &install)
	assert.Equal(t, "installed", install.Status)

	var installs models.ListResponse[models.InstallRecord]
	decodeJSON(t, doJSON(t, "GET", srv.URL+"/v1/marketplace/installs", token, nil, 200), &installs)
	require.Len(t, installs.Items, 1)
}

func TestDatasetProductsAndSchedulesContracts(t *testing.T) {
	t.Parallel()
	srv, token := newMarketplaceTestServer(t)

	productPayload := map[string]any{"name": "Claims Dataset", "include_schema": true, "schema": map[string]any{"claim_id": "string"}, "include_schedules": true, "schedules": []string{"daily"}}
	var product models.DatasetProduct
	decodeJSON(t, doJSON(t, "POST", srv.URL+"/v1/products/from-dataset/ri.foundry.main.dataset.source", token, productPayload, 200), &product)
	assert.Equal(t, "dataset", product.Manifest.Entity)
	assert.Equal(t, []string{"daily"}, product.Manifest.Schedules)

	var marketplaceProduct models.DatasetProduct
	decodeJSON(t, doJSON(t, "POST", srv.URL+"/v1/marketplace/products/from-dataset/ri.foundry.main.dataset.marketplace", token, productPayload, 200), &marketplaceProduct)
	assert.Equal(t, "ri.foundry.main.dataset.marketplace", marketplaceProduct.SourceDatasetRID)

	var productMirror models.DatasetProduct
	decodeJSON(t, doJSON(t, "GET", srv.URL+"/v1/marketplace/products/"+product.ID.String(), token, nil, 200), &productMirror)
	assert.Equal(t, product.ID, productMirror.ID)

	var productRootMirror models.DatasetProduct
	decodeJSON(t, doJSON(t, "GET", srv.URL+"/v1/products/"+product.ID.String(), token, nil, 200), &productRootMirror)
	assert.Equal(t, product.ID, productRootMirror.ID)

	projectID := uuid.New()
	installPayload := map[string]any{"target_project_id": projectID.String(), "target_dataset_rid": "ri.foundry.main.dataset.target"}
	var marketplaceInstall models.DatasetProductInstall
	decodeJSON(t, doJSON(t, "POST", srv.URL+"/v1/marketplace/products/"+product.ID.String()+"/install", token, installPayload, 200), &marketplaceInstall)
	assert.Equal(t, "pending", marketplaceInstall.Status)
	assert.Contains(t, string(marketplaceInstall.Details), "manifest_replay")

	var productInstall models.DatasetProductInstall
	decodeJSON(t, doJSON(t, "POST", srv.URL+"/v1/products/"+product.ID.String()+"/install", token, installPayload, 200), &productInstall)
	assert.Equal(t, "pending", productInstall.Status)
	assert.Equal(t, product.ID, productInstall.ProductID)

	versionID := uuid.New()
	var schedule models.AddScheduleManifestResponse
	decodeJSON(t, doJSON(t, "POST", srv.URL+"/v1/products/"+product.ID.String()+"/schedules", token, map[string]any{"product_version_id": versionID.String(), "manifest": map[string]any{"name": "daily", "trigger": map[string]any{"dataset_rid": "old-dataset"}, "target": map[string]any{"pipeline_rid": "old-pipeline"}, "scope_kind": "project"}}, 201), &schedule)
	assert.Equal(t, "daily", schedule.Name)

	var marketplaceSchedule models.AddScheduleManifestResponse
	decodeJSON(t, doJSON(t, "POST", srv.URL+"/v1/marketplace/products/"+product.ID.String()+"/schedules", token, map[string]any{"product_version_id": versionID.String(), "manifest": map[string]any{"name": "hourly", "trigger": map[string]any{"dataset_rid": "old-dataset"}, "target": map[string]any{"pipeline_rid": "old-pipeline"}, "scope_kind": "project"}}, 201), &marketplaceSchedule)
	assert.Equal(t, "hourly", marketplaceSchedule.Name)

	var materialised models.InstallSchedulesResponse
	decodeJSON(t, doJSON(t, "POST", srv.URL+"/v1/products/"+product.ID.String()+"/install:schedules", token, map[string]any{"product_version_id": versionID.String(), "rid_mapping": map[string]any{"pipeline": map[string]string{"old-pipeline": "new-pipeline"}, "dataset": map[string]string{"old-dataset": "new-dataset"}}, "activate_manifests": []string{"daily"}}, 200), &materialised)
	require.Len(t, materialised.Materialised, 1)
	assert.Contains(t, string(materialised.Materialised[0].Target), "new-pipeline")
	assert.Contains(t, string(materialised.Materialised[0].Trigger), "new-dataset")

	var marketplaceMaterialised models.InstallSchedulesResponse
	decodeJSON(t, doJSON(t, "POST", srv.URL+"/v1/marketplace/products/"+product.ID.String()+"/install:schedules", token, map[string]any{"product_version_id": versionID.String(), "rid_mapping": map[string]any{"pipeline": map[string]string{"old-pipeline": "marketplace-pipeline"}, "dataset": map[string]string{"old-dataset": "marketplace-dataset"}}, "activate_manifests": []string{"hourly"}}, 200), &marketplaceMaterialised)
	require.Len(t, marketplaceMaterialised.Materialised, 1)
	assert.Contains(t, string(marketplaceMaterialised.Materialised[0].Target), "marketplace-pipeline")
	assert.Contains(t, string(marketplaceMaterialised.Materialised[0].Trigger), "marketplace-dataset")
}

func createListingV1(t *testing.T, baseURL, token string, payload any) models.ListingDefinition {
	t.Helper()
	var listing models.ListingDefinition
	decodeJSON(t, doJSON(t, "POST", baseURL+"/v1/marketplace/listings", token, payload, 200), &listing)
	return listing
}

func decodeJSON(t *testing.T, body []byte, dst any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(body, dst))
}

func (r *memoryRepo) ListVersions(_ context.Context, listingID uuid.UUID) ([]models.PackageVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.listings[listingID]; !ok {
		return nil, marketplace.ErrNotFound
	}
	return append([]models.PackageVersion(nil), r.versions[listingID]...), nil
}

func (r *memoryRepo) IncludeActionInProduct(_ context.Context, listingID uuid.UUID, req models.IncludeActionRequest) (*models.PackageVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.listings[listingID]; !ok {
		return nil, marketplace.ErrNotFound
	}
	versions := r.versions[listingID]
	if len(versions) == 0 {
		return nil, marketplace.ErrValidation
	}
	idx := 0
	if req.VersionID != nil {
		idx = -1
		for i, candidate := range versions {
			if candidate.ID == *req.VersionID {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, marketplace.ErrVersionNotFound
		}
	}
	var manifest map[string]any
	if len(versions[idx].Manifest) == 0 {
		manifest = map[string]any{}
	} else {
		requireNoErr(json.Unmarshal(versions[idx].Manifest, &manifest))
	}
	var action any
	requireNoErr(json.Unmarshal(req.ActionType, &action))
	artifacts, _ := manifest["artifacts"].([]any)
	artifacts = append(artifacts, map[string]any{"kind": "action_type", "action_type": action, "dependencies": req.Dependencies})
	manifest["artifacts"] = artifacts
	versions[idx].Manifest = mustJSON(manifest)
	r.versions[listingID] = versions
	version := versions[idx]
	return &version, nil
}

func (r *memoryRepo) CreateDatasetProduct(_ context.Context, rid string, req models.CreateDatasetProductRequest) (*models.DatasetProduct, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if strings.TrimSpace(req.Name) == "" {
		return nil, marketplace.ErrValidation
	}
	if req.Version == "" {
		req.Version = "1.0.0"
	}
	if req.BootstrapMode == "" {
		req.BootstrapMode = "schema-only"
	}
	id, _ := uuid.NewV7()
	now := time.Now().UTC()
	retention := req.Retention
	if len(retention) == 0 || !req.IncludeRetention {
		retention = json.RawMessage(`[]`)
	}
	manifest := models.DatasetProductManifest{Entity: "dataset", Version: req.Version, Schema: req.Schema, Retention: retention, BranchingPolicy: req.BranchingPolicy, Schedules: req.Schedules, Bootstrap: models.DatasetProductBootstrap{Mode: req.BootstrapMode}}
	if !req.IncludeSchema {
		manifest.Schema = nil
	}
	if !req.IncludeBranches {
		manifest.BranchingPolicy = nil
	}
	if !req.IncludeSchedules {
		manifest.Schedules = []string{}
	}
	product := models.DatasetProduct{ID: id, Name: req.Name, SourceDatasetRID: rid, EntityType: "dataset", Version: req.Version, ProjectID: req.ProjectID, PublishedBy: req.PublishedBy, ExportIncludesData: req.ExportIncludesData, IncludeSchema: req.IncludeSchema, IncludeBranches: req.IncludeBranches, IncludeRetention: req.IncludeRetention, IncludeSchedules: req.IncludeSchedules, Manifest: manifest, BootstrapMode: req.BootstrapMode, PublishedAt: now, CreatedAt: now}
	if r.datasetProducts == nil {
		r.datasetProducts = map[uuid.UUID]models.DatasetProduct{}
	}
	r.datasetProducts[id] = product
	return &product, nil
}

func (r *memoryRepo) GetDatasetProduct(_ context.Context, productID uuid.UUID) (*models.DatasetProduct, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	product, ok := r.datasetProducts[productID]
	if !ok {
		return nil, marketplace.ErrNotFound
	}
	return &product, nil
}

func (r *memoryRepo) InstallDatasetProduct(_ context.Context, productID uuid.UUID, req models.InstallDatasetProductRequest) (*models.DatasetProductInstall, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	product, ok := r.datasetProducts[productID]
	if !ok {
		return nil, marketplace.ErrNotFound
	}
	if strings.TrimSpace(req.TargetDatasetRID) == "" {
		return nil, marketplace.ErrValidation
	}
	mode := product.BootstrapMode
	if req.BootstrapMode != nil {
		mode = *req.BootstrapMode
	}
	id, _ := uuid.NewV7()
	now := time.Now().UTC()
	install := models.DatasetProductInstall{ID: id, ProductID: productID, TargetProjectID: req.TargetProjectID, TargetDatasetRID: req.TargetDatasetRID, BootstrapMode: mode, Status: "pending", Details: mustJSON(map[string]any{"manifest_replay": product.Manifest, "source_dataset_rid": product.SourceDatasetRID, "version": product.Version}), InstalledBy: req.InstalledBy, CreatedAt: now}
	return &install, nil
}

func (r *memoryRepo) AddScheduleManifest(_ context.Context, req models.AddScheduleManifestRequest) (*models.AddScheduleManifestResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.schedules == nil {
		r.schedules = map[uuid.UUID][]models.ScheduleManifest{}
	}
	id, _ := uuid.NewV7()
	replaced := false
	items := r.schedules[req.ProductVersionID]
	for i := range items {
		if items[i].Name == req.Manifest.Name {
			items[i] = req.Manifest
			replaced = true
		}
	}
	if !replaced {
		items = append(items, req.Manifest)
	}
	r.schedules[req.ProductVersionID] = items
	return &models.AddScheduleManifestResponse{ID: id, ProductVersionID: req.ProductVersionID, Name: req.Manifest.Name}, nil
}

func (r *memoryRepo) MaterialiseInstallSchedules(_ context.Context, req models.InstallSchedulesRequest) (*models.InstallSchedulesResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	selected := map[string]struct{}{}
	for _, name := range req.ActivateManifests {
		selected[name] = struct{}{}
	}
	items := append([]models.ScheduleManifest(nil), r.schedules[req.ProductVersionID]...)
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	out := []models.MaterialisedSchedule{}
	for _, manifest := range items {
		if len(selected) > 0 {
			if _, ok := selected[manifest.Name]; !ok {
				continue
			}
		}
		manifest.Trigger = rewriteTestRIDs(manifest.Trigger, req.RidMapping)
		manifest.Target = rewriteTestRIDs(manifest.Target, req.RidMapping)
		out = append(out, models.MaterialisedSchedule{Name: manifest.Name, Trigger: manifest.Trigger, Target: manifest.Target, ScopeKind: manifest.ScopeKind, Defaults: manifest.Defaults})
	}
	return &models.InstallSchedulesResponse{ProductVersionID: req.ProductVersionID, Materialised: out}, nil
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	requireNoErr(err)
	return b
}

func requireNoErr(err error) {
	if err != nil {
		panic(err)
	}
}

func rewriteTestRIDs(raw json.RawMessage, mapping models.RidMapping) json.RawMessage {
	text := string(raw)
	for old, replacement := range mapping.Pipeline {
		text = strings.ReplaceAll(text, old, replacement)
	}
	for old, replacement := range mapping.Dataset {
		text = strings.ReplaceAll(text, old, replacement)
	}
	return json.RawMessage(text)
}
