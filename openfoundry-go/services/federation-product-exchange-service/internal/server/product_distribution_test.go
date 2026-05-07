package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/observability"
	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/productdistribution"
)

func TestProductDistributionCreatePeer(t *testing.T) {
	t.Parallel()
	srv, token, _ := newDistributionTestServer(t)
	body := doJSON(t, http.MethodPost, srv.URL+"/api/v1/product-distribution/peers", token, peerPayload("acme-health"), http.StatusOK)
	var peer models.PeerOrganization
	require.NoError(t, json.Unmarshal(body, &peer))
	assert.Equal(t, "acme-health", peer.Slug)
	assert.Equal(t, "pending", peer.Status)
	assert.Equal(t, "onboarding", peer.LifecycleStage)
}

func TestProductDistributionCreateShare(t *testing.T) {
	t.Parallel()
	srv, token, repo := newDistributionTestServer(t)
	provider, consumer, contractID := repo.seedShareDependencies()

	body := doJSON(t, http.MethodPost, srv.URL+"/api/v1/product-distribution/shares", token, sharePayload(contractID, provider.ID, consumer.ID), http.StatusOK)
	var manifest models.ShareManifest
	require.NoError(t, json.Unmarshal(body, &manifest))
	assert.Equal(t, "claims_eu", manifest.Share.DatasetName)
	require.NotNil(t, manifest.SyncStatus)
	assert.Equal(t, "ready", manifest.SyncStatus.Status)
	assert.Equal(t, int64(1), manifest.SyncStatus.BacklogRows)
}

func TestProductDistributionGetManifest(t *testing.T) {
	t.Parallel()
	srv, token, repo := newDistributionTestServer(t)
	manifest := repo.seedManifest()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/product-distribution/shares/"+manifest.Share.ID.String(), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var got models.ShareManifest
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, manifest.Share.ID, got.Share.ID)
	require.NotNil(t, got.SyncStatus)
	assert.Equal(t, manifest.SyncStatus.ID, got.SyncStatus.ID)
}

func TestProductDistributionUpdateSyncStatus(t *testing.T) {
	t.Parallel()
	srv, token, repo := newDistributionTestServer(t)
	manifest := repo.seedManifest()

	body := doJSON(t, http.MethodPatch, srv.URL+"/api/v1/product-distribution/shares/"+manifest.Share.ID.String()+"/sync-status", token, map[string]any{"status": "degraded", "rows_replicated": 42, "backlog_rows": 7, "audit_cursor": "cursor/test"}, http.StatusOK)
	var status models.SyncStatus
	require.NoError(t, json.Unmarshal(body, &status))
	assert.Equal(t, "degraded", status.Status)
	assert.Equal(t, int64(42), status.RowsReplicated)
	assert.Equal(t, int64(7), status.BacklogRows)
	assert.Equal(t, "cursor/test", status.AuditCursor)
}

func TestProductDistributionContractLifecycle(t *testing.T) {
	t.Parallel()
	srv, token, repo := newDistributionTestServer(t)
	provider, _, _ := repo.seedShareDependencies()

	expires := time.Now().UTC().Add(72 * time.Hour)
	createPayload := map[string]any{
		"peer_id":            provider.ID.String(),
		"name":               "Partner contract",
		"description":        "claims sharing",
		"dataset_locator":    "claims://eu",
		"allowed_purposes":   []string{"analytics"},
		"data_classes":       []string{"internal"},
		"residency_region":   "eu-west-1",
		"query_template":     "select * from claims",
		"max_rows_per_query": 250,
		"replication_mode":   "incremental_replication",
		"encryption_profile": "aes256",
		"retention_days":     30,
		"status":             "active",
		"expires_at":         expires.Format(time.RFC3339Nano),
	}

	body := doJSON(t, http.MethodPost, srv.URL+"/api/v1/product-distribution/contracts", token, createPayload, http.StatusOK)
	var created models.SharingContract
	require.NoError(t, json.Unmarshal(body, &created))
	assert.Equal(t, "Partner contract", created.Name)
	assert.Equal(t, "active", created.Status)
	require.NotNil(t, created.SignedAt)
	assert.WithinDuration(t, time.Now().UTC(), *created.SignedAt, 5*time.Second)

	body = doJSON(t, http.MethodGet, srv.URL+"/api/v1/product-distribution/contracts", token, nil, http.StatusOK)
	var listEnv struct {
		Items []models.SharingContract `json:"items"`
	}
	require.NoError(t, json.Unmarshal(body, &listEnv))
	require.Len(t, listEnv.Items, 2)

	patchBody := map[string]any{"max_rows_per_query": 999, "description": "patched"}
	body = doJSON(t, http.MethodPatch, srv.URL+"/api/v1/product-distribution/contracts/"+created.ID.String(), token, patchBody, http.StatusOK)
	var updated models.SharingContract
	require.NoError(t, json.Unmarshal(body, &updated))
	assert.Equal(t, int64(999), updated.MaxRowsPerQuery)
	assert.Equal(t, "patched", updated.Description)
}

func TestProductDistributionContractValidationErrors(t *testing.T) {
	t.Parallel()
	srv, token, repo := newDistributionTestServer(t)
	provider, _, _ := repo.seedShareDependencies()
	expires := time.Now().UTC().Add(72 * time.Hour)
	base := map[string]any{
		"peer_id":            provider.ID.String(),
		"name":               "Partner contract",
		"description":        "claims",
		"dataset_locator":    "claims://eu",
		"allowed_purposes":   []string{"analytics"},
		"data_classes":       []string{"internal"},
		"residency_region":   "eu-west-1",
		"query_template":     "select * from claims",
		"max_rows_per_query": 250,
		"replication_mode":   "incremental_replication",
		"encryption_profile": "aes256",
		"retention_days":     30,
		"status":             "active",
		"expires_at":         expires.Format(time.RFC3339Nano),
	}

	bad := cloneMap(base)
	bad["name"] = "  "
	body := doJSON(t, http.MethodPost, srv.URL+"/api/v1/product-distribution/contracts", token, bad, http.StatusBadRequest)
	var errBody map[string]string
	require.NoError(t, json.Unmarshal(body, &errBody))
	assert.Contains(t, errBody["error"], "contract name")

	bad = cloneMap(base)
	bad["replication_mode"] = "tape"
	body = doJSON(t, http.MethodPost, srv.URL+"/api/v1/product-distribution/contracts", token, bad, http.StatusBadRequest)
	require.NoError(t, json.Unmarshal(body, &errBody))
	assert.Contains(t, errBody["error"], "replication mode")

	bad = cloneMap(base)
	bad["expires_at"] = time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	body = doJSON(t, http.MethodPost, srv.URL+"/api/v1/product-distribution/contracts", token, bad, http.StatusBadRequest)
	require.NoError(t, json.Unmarshal(body, &errBody))
	assert.Contains(t, errBody["error"], "expiry")

	bad = cloneMap(base)
	bad["peer_id"] = uuid.New().String()
	body = doJSON(t, http.MethodPost, srv.URL+"/api/v1/product-distribution/contracts", token, bad, http.StatusBadRequest)
	require.NoError(t, json.Unmarshal(body, &errBody))
	assert.Contains(t, errBody["error"], "peer")
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func TestProductDistributionValidationAndAuth(t *testing.T) {
	t.Parallel()
	srv, token, repo := newDistributionTestServer(t)
	resp, err := http.Get(srv.URL + "/api/v1/product-distribution/peers")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	body := doJSON(t, http.MethodPost, srv.URL+"/api/v1/product-distribution/peers", token, map[string]any{"slug": "missing-name"}, http.StatusBadRequest)
	var errBody map[string]string
	require.NoError(t, json.Unmarshal(body, &errBody))
	assert.Contains(t, errBody["error"], "display name")

	provider, _, contractID := repo.seedShareDependencies()
	doJSON(t, http.MethodPost, srv.URL+"/api/v1/product-distribution/shares", token, sharePayload(contractID, provider.ID, provider.ID), http.StatusBadRequest)
}

func newDistributionTestServer(t *testing.T) (*httptest.Server, string, *distributionMemoryRepo) {
	t.Helper()
	cfg := testConfig()
	jwt := authmwConfigForTest()
	token := testToken(t, jwt)
	repo := newDistributionMemoryRepo()
	srv := httptest.NewServer(BuildRouter(cfg, jwt, nil, productdistribution.NewHandlers(repo), observability.NewMetrics()))
	t.Cleanup(srv.Close)
	return srv, token, repo
}

func authmwConfigForTest() *authmw.JWTConfig { return authmw.NewJWTConfig("test-secret") }

func peerPayload(slug string) map[string]any {
	return map[string]any{"slug": slug, "display_name": "Acme Health", "region": "eu-west-1", "endpoint_url": "https://nexus.example", "auth_mode": "mtls+jwt", "trust_level": "trusted", "public_key_fingerprint": "SHA256:TEST", "shared_scopes": []string{"claims"}, "admin_contacts": []string{"ops@example.test"}}
}

func sharePayload(contractID, providerID, consumerID uuid.UUID) map[string]any {
	return map[string]any{"contract_id": contractID.String(), "provider_peer_id": providerID.String(), "consumer_peer_id": consumerID.String(), "dataset_name": "claims_eu", "selector": map[string]any{"region": "EU"}, "provider_schema": map[string]any{"claim_id": "string"}, "consumer_schema": map[string]any{"claim_id": "string"}, "sample_rows": []map[string]any{{"claim_id": "CLM-1"}}, "replication_mode": "incremental_replication"}
}

type distributionMemoryRepo struct {
	mu        sync.Mutex
	peers     map[uuid.UUID]models.PeerOrganization
	contracts map[uuid.UUID]models.SharingContract
	shares    map[uuid.UUID]models.SharedDataset
	statuses  map[uuid.UUID]models.SyncStatus
	grants    map[uuid.UUID]models.AccessGrant
}

func newDistributionMemoryRepo() *distributionMemoryRepo {
	return &distributionMemoryRepo{peers: map[uuid.UUID]models.PeerOrganization{}, contracts: map[uuid.UUID]models.SharingContract{}, shares: map[uuid.UUID]models.SharedDataset{}, statuses: map[uuid.UUID]models.SyncStatus{}, grants: map[uuid.UUID]models.AccessGrant{}}
}

func (r *distributionMemoryRepo) ListPeers(context.Context) ([]models.PeerOrganization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]models.PeerOrganization, 0, len(r.peers))
	for _, peer := range r.peers {
		items = append(items, peer)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Slug < items[j].Slug })
	return items, nil
}

func (r *distributionMemoryRepo) CreatePeer(_ context.Context, req models.CreatePeerRequest) (*models.PeerOrganization, error) {
	if err := productdistribution.ValidateCreatePeer(req); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id, _ := uuid.NewV7()
	now := time.Now().UTC()
	if req.OrganizationType == "" {
		req.OrganizationType = "partner"
	}
	peer := models.PeerOrganization{ID: id, Slug: req.Slug, DisplayName: req.DisplayName, OrganizationType: req.OrganizationType, Region: req.Region, EndpointURL: req.EndpointURL, AuthMode: req.AuthMode, TrustLevel: req.TrustLevel, PublicKeyFingerprint: req.PublicKeyFingerprint, SharedScopes: req.SharedScopes, Status: "pending", LifecycleStage: "onboarding", AdminContacts: req.AdminContacts, CreatedAt: now, UpdatedAt: now}
	r.peers[id] = peer
	return &peer, nil
}

func (r *distributionMemoryRepo) GetPeer(_ context.Context, id uuid.UUID) (*models.PeerOrganization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	peer, ok := r.peers[id]
	if !ok {
		return nil, productdistribution.ErrNotFound
	}
	return &peer, nil
}

func (r *distributionMemoryRepo) UpdatePeer(_ context.Context, id uuid.UUID, req models.UpdatePeerRequest) (*models.PeerOrganization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	peer, ok := r.peers[id]
	if !ok {
		return nil, productdistribution.ErrNotFound
	}
	if req.DisplayName != nil {
		peer.DisplayName = *req.DisplayName
	}
	if req.Status != nil {
		peer.Status = *req.Status
	}
	peer.UpdatedAt = time.Now().UTC()
	r.peers[id] = peer
	return &peer, nil
}

func (r *distributionMemoryRepo) DeletePeer(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.peers[id]; !ok {
		return productdistribution.ErrNotFound
	}
	delete(r.peers, id)
	return nil
}

func (r *distributionMemoryRepo) ListContracts(context.Context) ([]models.SharingContract, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]models.SharingContract, 0, len(r.contracts))
	for _, c := range r.contracts {
		items = append(items, c)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items, nil
}

func (r *distributionMemoryRepo) CreateContract(_ context.Context, req models.CreateContractRequest) (*models.SharingContract, error) {
	r.mu.Lock()
	peer, ok := r.peers[req.PeerID]
	r.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%w: peer does not exist", productdistribution.ErrValidation)
	}
	now := time.Now().UTC()
	if err := productdistribution.ValidateContract(&peer, req.Name, req.QueryTemplate, req.AllowedPurposes, req.MaxRowsPerQuery, req.ReplicationMode, req.RetentionDays, req.Status, req.ExpiresAt, now); err != nil {
		return nil, fmt.Errorf("%w: %s", productdistribution.ErrValidation, err.Error())
	}
	id, _ := uuid.NewV7()
	var signedAt *time.Time
	if req.Status == "active" {
		t := now
		signedAt = &t
	}
	contract := models.SharingContract{
		ID:                id,
		PeerID:            req.PeerID,
		Name:              req.Name,
		Description:       req.Description,
		DatasetLocator:    req.DatasetLocator,
		AllowedPurposes:   append([]string(nil), req.AllowedPurposes...),
		DataClasses:       append([]string(nil), req.DataClasses...),
		ResidencyRegion:   req.ResidencyRegion,
		QueryTemplate:     req.QueryTemplate,
		MaxRowsPerQuery:   req.MaxRowsPerQuery,
		ReplicationMode:   req.ReplicationMode,
		EncryptionProfile: req.EncryptionProfile,
		RetentionDays:     req.RetentionDays,
		Status:            req.Status,
		SignedAt:          signedAt,
		ExpiresAt:         req.ExpiresAt,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	r.mu.Lock()
	r.contracts[id] = contract
	r.mu.Unlock()
	return &contract, nil
}

func (r *distributionMemoryRepo) UpdateContract(_ context.Context, id uuid.UUID, req models.UpdateContractRequest) (*models.SharingContract, error) {
	r.mu.Lock()
	current, ok := r.contracts[id]
	if !ok {
		r.mu.Unlock()
		return nil, productdistribution.ErrNotFound
	}
	peer, peerOK := r.peers[current.PeerID]
	r.mu.Unlock()
	if !peerOK {
		return nil, fmt.Errorf("%w: contract peer does not exist", productdistribution.ErrValidation)
	}
	now := time.Now().UTC()
	proposed := current
	if req.Name != nil {
		proposed.Name = *req.Name
	}
	if req.Description != nil {
		proposed.Description = *req.Description
	}
	if req.DatasetLocator != nil {
		proposed.DatasetLocator = *req.DatasetLocator
	}
	if req.AllowedPurposes != nil {
		proposed.AllowedPurposes = *req.AllowedPurposes
	}
	if req.DataClasses != nil {
		proposed.DataClasses = *req.DataClasses
	}
	if req.ResidencyRegion != nil {
		proposed.ResidencyRegion = *req.ResidencyRegion
	}
	if req.QueryTemplate != nil {
		proposed.QueryTemplate = *req.QueryTemplate
	}
	if req.MaxRowsPerQuery != nil {
		proposed.MaxRowsPerQuery = *req.MaxRowsPerQuery
	}
	if req.ReplicationMode != nil {
		proposed.ReplicationMode = *req.ReplicationMode
	}
	if req.EncryptionProfile != nil {
		proposed.EncryptionProfile = *req.EncryptionProfile
	}
	if req.RetentionDays != nil {
		proposed.RetentionDays = *req.RetentionDays
	}
	if req.Status != nil {
		proposed.Status = *req.Status
	}
	if req.ExpiresAt != nil {
		proposed.ExpiresAt = *req.ExpiresAt
	}
	if err := productdistribution.ValidateContract(&peer, proposed.Name, proposed.QueryTemplate, proposed.AllowedPurposes, proposed.MaxRowsPerQuery, proposed.ReplicationMode, proposed.RetentionDays, proposed.Status, proposed.ExpiresAt, now); err != nil {
		return nil, fmt.Errorf("%w: %s", productdistribution.ErrValidation, err.Error())
	}
	if proposed.Status == "active" && proposed.SignedAt == nil {
		t := now
		proposed.SignedAt = &t
	}
	proposed.UpdatedAt = now
	r.mu.Lock()
	r.contracts[id] = proposed
	r.mu.Unlock()
	return &proposed, nil
}

func (r *distributionMemoryRepo) ListShareManifests(context.Context) ([]models.ShareManifest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := []models.ShareManifest{}
	for _, share := range r.shares {
		status := r.statuses[share.ID]
		items = append(items, models.ShareManifest{Share: share, SyncStatus: &status})
	}
	return items, nil
}

func (r *distributionMemoryRepo) CreateShareManifest(_ context.Context, req models.CreateShareRequest) (*models.ShareManifest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	contract, ok := r.contracts[req.ContractID]
	if !ok {
		return nil, productdistribution.ErrNotFound
	}
	provider, ok := r.peers[req.ProviderPeerID]
	if !ok {
		return nil, productdistribution.ErrValidation
	}
	consumer, ok := r.peers[req.ConsumerPeerID]
	if !ok {
		return nil, productdistribution.ErrValidation
	}
	if req.DatasetName == "" || provider.ID == consumer.ID || contract.PeerID != provider.ID || provider.Status != "authenticated" || consumer.Status != "authenticated" {
		return nil, productdistribution.ErrValidation
	}
	id, _ := uuid.NewV7()
	now := time.Now().UTC()
	if len(req.Selector) == 0 {
		req.Selector = json.RawMessage(`{}`)
	}
	if len(req.ProviderSchema) == 0 {
		req.ProviderSchema = json.RawMessage(`{}`)
	}
	if len(req.ConsumerSchema) == 0 {
		req.ConsumerSchema = json.RawMessage(`{}`)
	}
	if len(req.SampleRows) == 0 {
		req.SampleRows = json.RawMessage(`[]`)
	}
	share := models.SharedDataset{ID: id, ContractID: req.ContractID, ProviderPeerID: req.ProviderPeerID, ConsumerPeerID: req.ConsumerPeerID, ProviderSpaceID: req.ProviderSpaceID, ConsumerSpaceID: req.ConsumerSpaceID, DatasetName: req.DatasetName, Selector: req.Selector, ProviderSchema: req.ProviderSchema, ConsumerSchema: req.ConsumerSchema, SampleRows: req.SampleRows, ReplicationMode: req.ReplicationMode, Status: "active", CreatedAt: now, UpdatedAt: now}
	var sampleRows []json.RawMessage
	_ = json.Unmarshal(req.SampleRows, &sampleRows)
	statusID, _ := uuid.NewV7()
	nextSyncAt := now.Add(4 * time.Hour)
	status := models.SyncStatus{ID: statusID, ShareID: id, Mode: req.ReplicationMode, Status: "ready", RowsReplicated: 0, BacklogRows: int64(len(sampleRows)), EncryptedInTransit: true, EncryptedAtRest: true, KeyVersion: contract.EncryptionProfile, NextSyncAt: &nextSyncAt, AuditCursor: "cursor/" + id.String(), UpdatedAt: now}
	r.shares[id] = share
	r.statuses[id] = status
	return &models.ShareManifest{Share: share, SyncStatus: &status}, nil
}

func (r *distributionMemoryRepo) GetShareManifest(_ context.Context, id uuid.UUID) (*models.ShareManifest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	share, ok := r.shares[id]
	if !ok {
		return nil, productdistribution.ErrNotFound
	}
	status := r.statuses[id]
	return &models.ShareManifest{Share: share, SyncStatus: &status}, nil
}

func (r *distributionMemoryRepo) ListSyncStatuses(context.Context) ([]models.SyncStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := []models.SyncStatus{}
	for _, status := range r.statuses {
		items = append(items, status)
	}
	return items, nil
}

func (r *distributionMemoryRepo) UpdateSyncStatus(_ context.Context, shareID uuid.UUID, req models.SyncStatusUpdateRequest) (*models.SyncStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	status, ok := r.statuses[shareID]
	if !ok {
		return nil, productdistribution.ErrNotFound
	}
	if req.Status != nil {
		status.Status = *req.Status
	}
	if req.RowsReplicated != nil {
		status.RowsReplicated = *req.RowsReplicated
	}
	if req.BacklogRows != nil {
		status.BacklogRows = *req.BacklogRows
	}
	if req.AuditCursor != nil {
		status.AuditCursor = *req.AuditCursor
	}
	status.UpdatedAt = time.Now().UTC()
	r.statuses[shareID] = status
	return &status, nil
}

func (r *distributionMemoryRepo) seedShareDependencies() (models.PeerOrganization, models.PeerOrganization, uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	providerID := uuid.New()
	consumerID := uuid.New()
	provider := models.PeerOrganization{ID: providerID, Slug: "provider", DisplayName: "Provider", OrganizationType: "host", Region: "eu", EndpointURL: "https://provider.example", AuthMode: "mtls", TrustLevel: "trusted", PublicKeyFingerprint: "SHA256:P", Status: "authenticated", LifecycleStage: "active", CreatedAt: now, UpdatedAt: now}
	consumer := models.PeerOrganization{ID: consumerID, Slug: "consumer", DisplayName: "Consumer", OrganizationType: "partner", Region: "eu", EndpointURL: "https://consumer.example", AuthMode: "mtls", TrustLevel: "trusted", PublicKeyFingerprint: "SHA256:C", Status: "authenticated", LifecycleStage: "active", CreatedAt: now, UpdatedAt: now}
	contractID := uuid.New()
	r.peers[providerID] = provider
	r.peers[consumerID] = consumer
	r.contracts[contractID] = models.SharingContract{ID: contractID, PeerID: providerID, Name: "Claims contract", Description: "claims share", DatasetLocator: "claims://eu", AllowedPurposes: []string{"claims"}, DataClasses: []string{"internal"}, ResidencyRegion: "eu-west-1", QueryTemplate: "SELECT * FROM claims", MaxRowsPerQuery: 100, ReplicationMode: "incremental_replication", EncryptionProfile: "key/test/v1", RetentionDays: 30, Status: "active", ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now, UpdatedAt: now}
	return provider, consumer, contractID
}

func (r *distributionMemoryRepo) seedManifest() models.ShareManifest {
	provider, consumer, contractID := r.seedShareDependencies()
	manifest, err := r.CreateShareManifest(context.Background(), decodeSharePayload(sharePayload(contractID, provider.ID, consumer.ID)))
	if err != nil {
		panic(err)
	}
	return *manifest
}

func decodeSharePayload(payload map[string]any) models.CreateShareRequest {
	b, _ := json.Marshal(payload)
	var req models.CreateShareRequest
	_ = json.Unmarshal(b, &req)
	return req
}

func (r *distributionMemoryRepo) GetShare(_ context.Context, id uuid.UUID) (*models.SharedDataset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	share, ok := r.shares[id]
	if !ok {
		return nil, productdistribution.ErrNotFound
	}
	return &share, nil
}

func (r *distributionMemoryRepo) GetContract(_ context.Context, id uuid.UUID) (*models.SharingContract, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	contract, ok := r.contracts[id]
	if !ok {
		return nil, productdistribution.ErrNotFound
	}
	return &contract, nil
}

func (r *distributionMemoryRepo) GetAccessGrantByShare(_ context.Context, shareID uuid.UUID) (*models.AccessGrant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, grant := range r.grants {
		if grant.ShareID == shareID {
			return &grant, nil
		}
	}
	return nil, productdistribution.ErrNotFound
}

func (r *distributionMemoryRepo) seedAccessGrant(shareID, peerID uuid.UUID, purposes []string) models.AccessGrant {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	id := uuid.New()
	grant := models.AccessGrant{
		ID: id, ShareID: shareID, PeerID: peerID,
		QueryTemplate: "SELECT * FROM shared_dataset", MaxRowsPerQuery: 100,
		AllowedPurposes: purposes,
		ExpiresAt: now.Add(24 * time.Hour), IssuedAt: now,
	}
	r.grants[id] = grant
	return grant
}

func (r *distributionMemoryRepo) expireContract(id uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.contracts[id]
	c.ExpiresAt = time.Now().UTC().Add(-time.Hour)
	r.contracts[id] = c
}

// TestProductDistributionConsumeQuery exercises the federated-consume
// endpoint end-to-end, covering the happy path plus the four 4xx cases
// the Rust handler / domain returns: missing share, write-oriented SQL,
// disallowed purpose, and expired contract.
func TestProductDistributionConsumeQuery(t *testing.T) {
	t.Parallel()
	srv, token, repo := newDistributionTestServer(t)
	provider, consumer, contractID := repo.seedShareDependencies()
	manifest, err := repo.CreateShareManifest(context.Background(), decodeSharePayload(sharePayload(contractID, provider.ID, consumer.ID)))
	require.NoError(t, err)
	shareID := manifest.Share.ID
	repo.seedAccessGrant(shareID, consumer.ID, []string{"claims"})

	// 200 — happy path.
	body := doJSON(t, http.MethodPost, srv.URL+"/api/v1/product-distribution/queries", token, map[string]any{
		"share_id": shareID.String(),
		"sql":      "select * from claims",
		"purpose":  "claims",
	}, http.StatusOK)
	var res models.FederatedQueryResult
	require.NoError(t, json.Unmarshal(body, &res))
	assert.Equal(t, "claims_eu", res.DatasetName)
	assert.Equal(t, "Provider", res.SourcePeer)
	assert.Equal(t, "incremental_replication", res.QueryMode)
	assert.Len(t, res.Rows, 1)
	assert.Equal(t, []string{"claim_id"}, res.Columns)

	// 404 — missing share.
	doJSON(t, http.MethodPost, srv.URL+"/api/v1/product-distribution/queries", token, map[string]any{
		"share_id": uuid.New().String(),
		"sql":      "select 1",
		"purpose":  "claims",
	}, http.StatusNotFound)

	// 400 — write-oriented SQL.
	writeBody := doJSON(t, http.MethodPost, srv.URL+"/api/v1/product-distribution/queries", token, map[string]any{
		"share_id": shareID.String(),
		"sql":      "DELETE FROM claims",
		"purpose":  "claims",
	}, http.StatusBadRequest)
	var writeErr map[string]string
	require.NoError(t, json.Unmarshal(writeBody, &writeErr))
	assert.Contains(t, writeErr["error"], "read-only")

	// 400 — purpose not allowed by the grant.
	purposeBody := doJSON(t, http.MethodPost, srv.URL+"/api/v1/product-distribution/queries", token, map[string]any{
		"share_id": shareID.String(),
		"sql":      "select 1",
		"purpose":  "marketing",
	}, http.StatusBadRequest)
	var purposeErr map[string]string
	require.NoError(t, json.Unmarshal(purposeBody, &purposeErr))
	assert.Contains(t, purposeErr["error"], "purpose")

	// 400 — expired contract.
	repo.expireContract(contractID)
	expiredBody := doJSON(t, http.MethodPost, srv.URL+"/api/v1/product-distribution/queries", token, map[string]any{
		"share_id": shareID.String(),
		"sql":      "select 1",
		"purpose":  "claims",
	}, http.StatusBadRequest)
	var expiredErr map[string]string
	require.NoError(t, json.Unmarshal(expiredBody, &expiredErr))
	assert.Contains(t, expiredErr["error"], "sharing contract")
}
