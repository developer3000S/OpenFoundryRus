package productdistribution_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/productdistribution"
)

func makePeer(status string) models.PeerOrganization {
	now := time.Now().UTC()
	return models.PeerOrganization{
		ID:                   uuid.New(),
		Slug:                 "peer",
		DisplayName:          "Peer",
		OrganizationType:     "partner",
		Region:               "eu-west-1",
		EndpointURL:          "https://peer.example.com",
		AuthMode:             "mtls",
		TrustLevel:           "partner",
		PublicKeyFingerprint: "fp-1",
		SharedScopes:         []string{"datasets"},
		Status:               status,
		LifecycleStage:       "active",
		AdminContacts:        []string{"ops@example.com"},
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

// Ports `rejects_active_contract_with_unauthenticated_peer` from
// services/federation-product-exchange-service/src/domain/governance.rs.
func TestValidateContractRejectsActiveContractWithUnauthenticatedPeer(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	peer := makePeer("pending")
	err := productdistribution.ValidateContract(
		&peer,
		"Partner contract",
		"select * from shared_dataset",
		[]string{"analytics"},
		100,
		"query_only",
		30,
		"active",
		now.Add(30*24*time.Hour),
		now,
	)
	if err == nil {
		t.Fatal("expected validation error for unauthenticated active contract peer")
	}
}

func TestValidateContractTable(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	authenticated := makePeer("authenticated")
	pending := makePeer("pending")

	type args struct {
		peer            *models.PeerOrganization
		name            string
		queryTemplate   string
		allowedPurposes []string
		maxRows         int64
		replicationMode string
		retentionDays   int32
		status          string
		expiresAt       time.Time
	}
	good := args{
		peer:            &authenticated,
		name:            "Contract",
		queryTemplate:   "select 1",
		allowedPurposes: []string{"analytics"},
		maxRows:         100,
		replicationMode: "incremental_replication",
		retentionDays:   30,
		status:          "active",
		expiresAt:       now.Add(24 * time.Hour),
	}

	cases := []struct {
		name    string
		mut     func(a *args)
		wantErr bool
	}{
		{name: "happy_path", mut: func(*args) {}, wantErr: false},
		{name: "blank_name", mut: func(a *args) { a.name = "  " }, wantErr: true},
		{name: "blank_query_template", mut: func(a *args) { a.queryTemplate = "" }, wantErr: true},
		{name: "non_positive_max_rows", mut: func(a *args) { a.maxRows = 0 }, wantErr: true},
		{name: "non_positive_retention", mut: func(a *args) { a.retentionDays = 0 }, wantErr: true},
		{name: "unsupported_status", mut: func(a *args) { a.status = "frozen" }, wantErr: true},
		{name: "unsupported_replication_mode", mut: func(a *args) { a.replicationMode = "tape" }, wantErr: true},
		{name: "expiry_in_past_active", mut: func(a *args) { a.expiresAt = now.Add(-time.Hour) }, wantErr: true},
		{name: "expiry_in_past_expired_status_allowed", mut: func(a *args) {
			a.status = "expired"
			a.expiresAt = now.Add(-time.Hour)
		}, wantErr: false},
		{name: "active_requires_purposes", mut: func(a *args) { a.allowedPurposes = nil }, wantErr: true},
		{name: "active_requires_authenticated_peer", mut: func(a *args) { a.peer = &pending }, wantErr: true},
		{name: "draft_with_pending_peer_ok", mut: func(a *args) {
			a.peer = &pending
			a.status = "draft"
			a.allowedPurposes = nil
		}, wantErr: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := good
			tc.mut(&a)
			err := productdistribution.ValidateContract(a.peer, a.name, a.queryTemplate, a.allowedPurposes, a.maxRows, a.replicationMode, a.retentionDays, a.status, a.expiresAt, now)
			gotErr := err != nil
			if gotErr != tc.wantErr {
				t.Fatalf("ValidateContract: gotErr=%v wantErr=%v err=%v", gotErr, tc.wantErr, err)
			}
		})
	}
}

// Ports `rejects_federated_runtime_with_mismatched_grant_peer` from
// services/federation-product-exchange-service/src/domain/governance.rs.
func TestValidateFederatedRuntimeRejectsMismatchedGrantPeer(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	provider := makePeer("authenticated")
	consumer := makePeer("authenticated")
	contract := &models.SharingContract{
		ID: uuid.New(), PeerID: provider.ID, Status: "active",
		ExpiresAt: now.Add(time.Hour),
	}
	share := &models.SharedDataset{
		ID: uuid.New(), ContractID: contract.ID,
		ProviderPeerID: provider.ID, ConsumerPeerID: consumer.ID,
		Status: "active",
	}
	grant := &models.AccessGrant{
		ShareID: share.ID, PeerID: provider.ID, // mismatch: should be consumer
		ExpiresAt: now.Add(time.Hour),
	}
	if err := productdistribution.ValidateFederatedRuntime(share, contract, grant, &provider, &consumer, now); err == nil {
		t.Fatal("expected validation error for mismatched grant peer")
	}
}

func TestValidateFederatedRuntimeAcceptsConsistentSetup(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	provider := makePeer("authenticated")
	consumer := makePeer("authenticated")
	contract := &models.SharingContract{
		ID: uuid.New(), PeerID: provider.ID, Status: "active",
		ExpiresAt: now.Add(time.Hour),
	}
	share := &models.SharedDataset{
		ID: uuid.New(), ContractID: contract.ID,
		ProviderPeerID: provider.ID, ConsumerPeerID: consumer.ID,
		Status: "active",
	}
	grant := &models.AccessGrant{
		ShareID: share.ID, PeerID: consumer.ID,
		ExpiresAt: now.Add(time.Hour),
	}
	if err := productdistribution.ValidateFederatedRuntime(share, contract, grant, &provider, &consumer, now); err != nil {
		t.Fatalf("expected validation to succeed, got %v", err)
	}
}

func TestValidateFederatedRuntimeRejectsExpiredContract(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	provider := makePeer("authenticated")
	consumer := makePeer("authenticated")
	contract := &models.SharingContract{
		ID: uuid.New(), PeerID: provider.ID, Status: "active",
		ExpiresAt: now.Add(-time.Hour),
	}
	share := &models.SharedDataset{
		ContractID: contract.ID,
		ProviderPeerID: provider.ID, ConsumerPeerID: consumer.ID,
		Status: "active",
	}
	grant := &models.AccessGrant{PeerID: consumer.ID}
	if err := productdistribution.ValidateFederatedRuntime(share, contract, grant, &provider, &consumer, now); err == nil || err.Error() != "sharing contract is not active" {
		t.Fatalf("want \"sharing contract is not active\", got %v", err)
	}
}

func TestValidateFederatedRuntimeRejectsInactiveShare(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	provider := makePeer("authenticated")
	consumer := makePeer("authenticated")
	contract := &models.SharingContract{PeerID: provider.ID, Status: "active", ExpiresAt: now.Add(time.Hour)}
	share := &models.SharedDataset{ProviderPeerID: provider.ID, ConsumerPeerID: consumer.ID, Status: "paused"}
	grant := &models.AccessGrant{PeerID: consumer.ID}
	if err := productdistribution.ValidateFederatedRuntime(share, contract, grant, &provider, &consumer, now); err == nil || err.Error() != "shared dataset is not active" {
		t.Fatalf("want \"shared dataset is not active\", got %v", err)
	}
}

func TestValidateFederatedRuntimeRejectsUnauthenticatedConsumer(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	provider := makePeer("authenticated")
	consumer := makePeer("pending")
	contract := &models.SharingContract{PeerID: provider.ID, Status: "active", ExpiresAt: now.Add(time.Hour)}
	share := &models.SharedDataset{ProviderPeerID: provider.ID, ConsumerPeerID: consumer.ID, Status: "active"}
	grant := &models.AccessGrant{PeerID: consumer.ID}
	if err := productdistribution.ValidateFederatedRuntime(share, contract, grant, &provider, &consumer, now); err == nil {
		t.Fatal("expected unauthenticated consumer to be rejected")
	}
}
