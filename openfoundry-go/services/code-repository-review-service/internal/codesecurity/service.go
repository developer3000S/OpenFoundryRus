package codesecurity

import (
	"context"
	"encoding/json"

	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/code-repository-review-service/internal/repo"
)

// Service connects a Scanner to the Postgres persistence layer.
type Service struct {
	Scanner Scanner
	Repo    *repo.CodeSecurityRepo
}

type PersistedScanResult struct {
	Scan     models.CodeSecurityScan      `json:"scan"`
	Findings []models.CodeSecurityFinding `json:"findings"`
}

func (s *Service) ScanAndPersist(ctx context.Context, req ScanRequest) (PersistedScanResult, error) {
	result, err := s.Scanner.Scan(ctx, req)
	if err != nil {
		return PersistedScanResult{}, err
	}
	scanPayload, err := ScanPayload(req)
	if err != nil {
		return PersistedScanResult{}, err
	}
	findingPayloads := make([]json.RawMessage, 0, len(result.Findings))
	for _, finding := range result.Findings {
		payload, err := FindingPayload(finding)
		if err != nil {
			return PersistedScanResult{}, err
		}
		findingPayloads = append(findingPayloads, payload)
	}
	scan, findings, err := s.Repo.CreateScanWithFindings(ctx, scanPayload, findingPayloads)
	if err != nil {
		return PersistedScanResult{}, err
	}
	return PersistedScanResult{Scan: scan, Findings: findings}, nil
}
