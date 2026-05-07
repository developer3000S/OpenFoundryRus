package productdistribution_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/productdistribution"
)

// Ports `rejects_non_read_only_sql` from
// services/federation-product-exchange-service/src/domain/federation.rs.
func TestExecuteFederatedQueryRejectsNonReadOnlySQL(t *testing.T) {
	t.Parallel()
	req := &models.FederatedQueryRequest{
		ShareID: uuid.New(),
		SQL:     "DELETE FROM shared_dataset",
		Purpose: "analytics",
	}
	share, grant, peers := federationFixtures(t)
	if _, err := productdistribution.ExecuteFederatedQuery(req, share, grant, peers); err == nil {
		t.Fatal("expected error for write-oriented SQL")
	}
}

// Ports `accepts_select_sql` from the Rust source.
func TestExecuteFederatedQueryAcceptsSelectSQL(t *testing.T) {
	t.Parallel()
	req := &models.FederatedQueryRequest{
		ShareID: uuid.New(),
		SQL:     "SELECT * FROM shared_dataset LIMIT 10",
		Purpose: "analytics",
	}
	share, grant, peers := federationFixtures(t)
	res, err := productdistribution.ExecuteFederatedQuery(req, share, grant, peers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.QueryMode != share.ReplicationMode {
		t.Fatalf("query_mode mismatch: got %q want %q", res.QueryMode, share.ReplicationMode)
	}
	if res.SourcePeer != "Provider" {
		t.Fatalf("source_peer mismatch: got %q want %q", res.SourcePeer, "Provider")
	}
}

func TestExecuteFederatedQueryRejectsEmptySQL(t *testing.T) {
	t.Parallel()
	req := &models.FederatedQueryRequest{SQL: "  \t\n"}
	share, grant, peers := federationFixtures(t)
	_, err := productdistribution.ExecuteFederatedQuery(req, share, grant, peers)
	if err == nil || err.Error() != "federated query SQL is required" {
		t.Fatalf("want SQL-required error, got %v", err)
	}
}

func TestExecuteFederatedQueryAcceptsMixedCaseWithCTE(t *testing.T) {
	t.Parallel()
	req := &models.FederatedQueryRequest{
		SQL:     "WiTh recent AS (SELECT 1) SELECT * FROM recent",
		Purpose: "analytics",
	}
	share, grant, peers := federationFixtures(t)
	if _, err := productdistribution.ExecuteFederatedQuery(req, share, grant, peers); err != nil {
		t.Fatalf("expected WITH ... to be accepted, got %v", err)
	}
}

func TestExecuteFederatedQueryRejectsHiddenWriteKeyword(t *testing.T) {
	t.Parallel()
	req := &models.FederatedQueryRequest{
		SQL:     "SELECT * FROM t ; UPDATE other SET x = 1",
		Purpose: "analytics",
	}
	share, grant, peers := federationFixtures(t)
	if _, err := productdistribution.ExecuteFederatedQuery(req, share, grant, peers); err == nil {
		t.Fatal("expected hidden UPDATE keyword to be rejected")
	}
}

func TestExecuteFederatedQueryFallsBackToUnknownPeer(t *testing.T) {
	t.Parallel()
	share, grant, _ := federationFixtures(t)
	req := &models.FederatedQueryRequest{SQL: "SELECT 1", Purpose: "analytics"}
	res, err := productdistribution.ExecuteFederatedQuery(req, share, grant, map[uuid.UUID]models.PeerOrganization{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SourcePeer != "unknown peer" {
		t.Fatalf("source_peer fallback mismatch: got %q", res.SourcePeer)
	}
}

func TestExecuteFederatedQueryClampsLimit(t *testing.T) {
	t.Parallel()
	share, grant, peers := federationFixtures(t)
	share.SampleRows = json.RawMessage(`[{"a":1},{"a":2},{"a":3}]`)
	grant.MaxRowsPerQuery = 2
	req := &models.FederatedQueryRequest{SQL: "select 1", Purpose: "analytics"}
	res, err := productdistribution.ExecuteFederatedQuery(req, share, grant, peers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Limit != 2 {
		t.Fatalf("limit clamp mismatch: got %d want 2", res.Limit)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("row count mismatch: got %d want 2", len(res.Rows))
	}
	if len(res.Columns) != 1 || res.Columns[0] != "a" {
		t.Fatalf("columns mismatch: got %v", res.Columns)
	}
}

func federationFixtures(t *testing.T) (*models.SharedDataset, *models.AccessGrant, map[uuid.UUID]models.PeerOrganization) {
	t.Helper()
	now := time.Now().UTC()
	providerID := uuid.New()
	consumerID := uuid.New()
	share := &models.SharedDataset{
		ID:              uuid.New(),
		ContractID:      uuid.New(),
		ProviderPeerID:  providerID,
		ConsumerPeerID:  consumerID,
		DatasetName:     "claims_eu",
		ReplicationMode: "incremental_replication",
		Status:          "active",
		SampleRows:      json.RawMessage(`[{"claim_id":"CLM-1"}]`),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	grant := &models.AccessGrant{
		ID:              uuid.New(),
		ShareID:         share.ID,
		PeerID:          consumerID,
		QueryTemplate:   "SELECT * FROM shared_dataset",
		MaxRowsPerQuery: 100,
		AllowedPurposes: []string{"analytics"},
		ExpiresAt:       now.Add(24 * time.Hour),
		IssuedAt:        now,
	}
	peers := map[uuid.UUID]models.PeerOrganization{
		providerID: {ID: providerID, DisplayName: "Provider"},
		consumerID: {ID: consumerID, DisplayName: "Consumer"},
	}
	return share, grant, peers
}
