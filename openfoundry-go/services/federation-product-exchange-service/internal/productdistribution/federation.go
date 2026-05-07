// Package productdistribution federation port. This file is the 1:1 Go
// port of `src/domain/federation.rs` from the Rust
// federation-product-exchange-service crate.
//
// The federated-query path is read-only by construction: the SQL must
// start with `select` or `with`, and any of a fixed list of write-oriented
// keywords (insert/update/delete/drop/alter/truncate/create/revoke/grant)
// causes the request to be rejected before any rows are produced.
package productdistribution

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/federation-product-exchange-service/internal/observability"
)

// ExecuteFederatedQuery is the 1:1 port of `domain::federation::execute_query`.
// Steps mirror the Rust source verbatim: ensure the SQL is read-only,
// validate the access grant against the requested purpose, clamp the
// requested limit, take the first `limit` rows from the share's
// sample_rows, derive the column set from the first row's JSON object
// keys (preserving declaration order), and resolve the source peer's
// display name from the peer index.
func ExecuteFederatedQuery(
	req *models.FederatedQueryRequest,
	share *models.SharedDataset,
	grant *models.AccessGrant,
	peers map[uuid.UUID]models.PeerOrganization,
) (*models.FederatedQueryResult, error) {
	if err := ensureReadOnlySQL(req.SQL); err != nil {
		return nil, err
	}
	if err := observability.ValidateAccess(grant, req.Purpose); err != nil {
		return nil, err
	}

	limit := observability.ResolveLimit(grant, req.Limit)
	sample, err := decodeSampleRows(share.SampleRows)
	if err != nil {
		return nil, err
	}
	rows := sample
	if len(rows) > limit {
		rows = rows[:limit]
	}
	if rows == nil {
		rows = []json.RawMessage{}
	}

	columns := []string{}
	if len(rows) > 0 {
		if cols, ok := extractColumns(rows[0]); ok {
			columns = cols
		}
	}

	sourcePeer := "unknown peer"
	if peer, ok := peers[share.ProviderPeerID]; ok {
		sourcePeer = peer.DisplayName
	}

	return &models.FederatedQueryResult{
		ShareID:     share.ID,
		DatasetName: share.DatasetName,
		SourcePeer:  sourcePeer,
		ExecutedSQL: req.SQL,
		QueryMode:   share.ReplicationMode,
		Limit:       limit,
		Columns:     columns,
		Rows:        rows,
	}, nil
}

// ensureReadOnlySQL is the 1:1 port of `federation::ensure_read_only_sql`.
// Empty SQL is rejected with "federated query SQL is required". Anything
// that doesn't start with `select` or `with` (after trim+lowercase) is
// rejected with "federated query must be read-only". A padded substring
// scan rejects any of the forbidden write keywords with "federated query
// contains a write-oriented SQL keyword".
func ensureReadOnlySQL(sql string) error {
	normalized := strings.ToLower(strings.TrimSpace(sql))
	if normalized == "" {
		return fmt.Errorf("federated query SQL is required")
	}
	if !strings.HasPrefix(normalized, "select") && !strings.HasPrefix(normalized, "with") {
		return fmt.Errorf("federated query must be read-only")
	}
	padded := " " + normalized + " "
	for _, keyword := range forbiddenSQLKeywords {
		if strings.Contains(padded, keyword) {
			return fmt.Errorf("federated query contains a write-oriented SQL keyword")
		}
	}
	return nil
}

// forbiddenSQLKeywords mirrors the Rust slice in `ensure_read_only_sql`.
// Order is preserved so the first-match scan returns the same string the
// Rust implementation would reject on.
var forbiddenSQLKeywords = []string{
	" insert ",
	" update ",
	" delete ",
	" drop ",
	" alter ",
	" truncate ",
	" create ",
	" revoke ",
	" grant ",
}

// decodeSampleRows materialises the JSON array stored in
// SharedDataset.SampleRows as a slice of raw JSON values, one per row.
// Empty / null payloads decode to an empty slice. A 4xx-friendly error
// surfaces when the stored sample is malformed.
func decodeSampleRows(raw json.RawMessage) ([]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return []json.RawMessage{}, nil
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(trimmed, &rows); err != nil {
		return nil, fmt.Errorf("federated query sample rows are not a JSON array: %w", err)
	}
	if rows == nil {
		rows = []json.RawMessage{}
	}
	return rows, nil
}

// extractColumns mirrors the Rust expression
// `rows.first().and_then(|v| v.as_object()).map(|o| o.keys().cloned().collect())`.
// Returns the JSON object keys in declaration order. Non-object first
// rows produce no columns.
func extractColumns(row json.RawMessage) ([]string, bool) {
	dec := json.NewDecoder(bytes.NewReader(row))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, false
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, false
	}
	keys := []string{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, false
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, false
		}
		keys = append(keys, key)
		if err := skipJSONValue(dec); err != nil {
			return nil, false
		}
	}
	return keys, true
}

func skipJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		for dec.More() {
			if _, err := dec.Token(); err != nil {
				return err
			}
			if err := skipJSONValue(dec); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	case '[':
		for dec.More() {
			if err := skipJSONValue(dec); err != nil {
				return err
			}
		}
		_, err = dec.Token()
		return err
	}
	return nil
}
