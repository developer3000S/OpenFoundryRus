package repo

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/models"
)

type txBeginner interface {
	BeginTx(ctx context.Context, txOptions pgx.TxOptions) (pgx.Tx, error)
}

// CodeSecurityRepo persists scanner runs into the absorbed
// code-security-scanning-service tables.
type CodeSecurityRepo struct {
	DB txBeginner
}

func (r *CodeSecurityRepo) CreateScanWithFindings(ctx context.Context, scanPayload json.RawMessage, findingPayloads []json.RawMessage) (models.CodeSecurityScan, []models.CodeSecurityFinding, error) {
	tx, err := r.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return models.CodeSecurityScan{}, nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	scanID, err := uuid.NewV7()
	if err != nil {
		scanID = uuid.New()
	}
	var scan models.CodeSecurityScan
	if err := tx.QueryRow(ctx, `
INSERT INTO code_security_scans (id, payload, created_at, updated_at)
VALUES ($1, $2::jsonb, NOW(), NOW())
RETURNING id, payload, created_at, updated_at`, scanID, scanPayload).
		Scan(&scan.ID, &scan.Payload, &scan.CreatedAt, &scan.UpdatedAt); err != nil {
		return models.CodeSecurityScan{}, nil, err
	}

	findings := make([]models.CodeSecurityFinding, 0, len(findingPayloads))
	for _, payload := range findingPayloads {
		findingID, err := uuid.NewV7()
		if err != nil {
			findingID = uuid.New()
		}
		var finding models.CodeSecurityFinding
		if err := tx.QueryRow(ctx, `
INSERT INTO code_security_findings (id, parent_id, payload, created_at)
VALUES ($1, $2, $3::jsonb, NOW())
RETURNING id, parent_id, payload, created_at`, findingID, scan.ID, payload).
			Scan(&finding.ID, &finding.ParentID, &finding.Payload, &finding.CreatedAt); err != nil {
			return models.CodeSecurityScan{}, nil, err
		}
		findings = append(findings, finding)
	}

	if err := tx.Commit(ctx); err != nil {
		return models.CodeSecurityScan{}, nil, err
	}
	committed = true
	return scan, findings, nil
}
