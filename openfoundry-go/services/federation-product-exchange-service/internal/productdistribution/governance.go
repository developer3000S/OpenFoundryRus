package productdistribution

import (
	"fmt"
	"strings"
	"time"

	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/models"
)

// ValidateContract is the 1:1 port of governance::validate_contract from Rust.
// It enforces the lifecycle invariants for sharing contracts: required name,
// query template and replication mode; positive max-rows / retention; allowed
// status; expiry-in-future for non-expired contracts; and an authenticated
// peer + at least one allowed purpose for active contracts. The peer argument
// is consulted only when status == "active".
func ValidateContract(
	peer *models.PeerOrganization,
	name string,
	queryTemplate string,
	allowedPurposes []string,
	maxRowsPerQuery int64,
	replicationMode string,
	retentionDays int32,
	status string,
	expiresAt time.Time,
	now time.Time,
) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("contract name is required")
	}
	if strings.TrimSpace(queryTemplate) == "" {
		return fmt.Errorf("query template is required")
	}
	if maxRowsPerQuery <= 0 {
		return fmt.Errorf("max_rows_per_query must be greater than zero")
	}
	if retentionDays <= 0 {
		return fmt.Errorf("retention_days must be greater than zero")
	}
	if !matchesContractStatus(status) {
		return fmt.Errorf("unsupported contract status '%s'", status)
	}
	if err := ensureValidReplicationMode(replicationMode); err != nil {
		return err
	}
	if status != "expired" && !expiresAt.After(now) {
		return fmt.Errorf("contract expiry must be in the future")
	}
	if status == "active" {
		if len(allowedPurposes) == 0 {
			return fmt.Errorf("active contracts require at least one allowed purpose")
		}
		if err := ensurePeerAuthenticated(peer, "contract peer"); err != nil {
			return err
		}
	}
	return nil
}

// ValidateFederatedRuntime is the 1:1 port of
// `governance::validate_federated_runtime` from Rust. The federated-query
// consume handler invokes this before any rows are produced. The
// invariants mirror the Rust source verbatim:
//  1. share.status == "active"
//  2. contract.status == "active" and contract.expires_at > now
//  3. share.provider_peer_id == contract.peer_id
//  4. grant.peer_id == share.consumer_peer_id
//  5. provider and consumer peers are both in status "authenticated"
//
// Error messages mirror the Rust strings byte-exactly so the handler can
// surface them as 400 responses without translation.
func ValidateFederatedRuntime(
	share *models.SharedDataset,
	contract *models.SharingContract,
	grant *models.AccessGrant,
	providerPeer *models.PeerOrganization,
	consumerPeer *models.PeerOrganization,
	now time.Time,
) error {
	if share.Status != "active" {
		return fmt.Errorf("shared dataset is not active")
	}
	if contract.Status != "active" || !contract.ExpiresAt.After(now) {
		return fmt.Errorf("sharing contract is not active")
	}
	if contract.PeerID != share.ProviderPeerID {
		return fmt.Errorf("share provider does not match the contract owner peer")
	}
	if grant.PeerID != share.ConsumerPeerID {
		return fmt.Errorf("access grant is not bound to the consumer peer")
	}
	if err := ensurePeerAuthenticated(providerPeer, "provider peer"); err != nil {
		return err
	}
	if err := ensurePeerAuthenticated(consumerPeer, "consumer peer"); err != nil {
		return err
	}
	return nil
}

func ensurePeerAuthenticated(peer *models.PeerOrganization, label string) error {
	if peer == nil || peer.Status != authenticatedPeerStatus {
		return fmt.Errorf("%s must be authenticated", label)
	}
	return nil
}

func ensureValidReplicationMode(mode string) error {
	if _, ok := replicationRank(mode); !ok {
		return fmt.Errorf("unsupported replication mode '%s'", mode)
	}
	return nil
}

func matchesContractStatus(status string) bool {
	switch status {
	case "draft", "active", "suspended", "expired":
		return true
	}
	return false
}
