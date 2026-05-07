package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

import saga "github.com/openfoundry/openfoundry-go/libs/saga"

const retentionSweepPath = "/api/v1/audit/retention/sweep"

// RetentionSweepInput mirrors the Rust struct.
type RetentionSweepInput struct {
	TenantID      string `json:"tenant_id"`
	OlderThanDays uint32 `json:"older_than_days,omitempty"`
	DryRun        bool   `json:"dry_run,omitempty"`
}

// UnmarshalJSON applies `default = 90` for older_than_days when the
// caller omits it (mirrors the Rust `#[serde(default = "default_days")]`).
func (in *RetentionSweepInput) UnmarshalJSON(data []byte) error {
	type alias RetentionSweepInput
	out := alias{OlderThanDays: 90}
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	if out.OlderThanDays == 0 {
		out.OlderThanDays = 90
	}
	*in = RetentionSweepInput(out)
	return nil
}

// RetentionSweepOutput mirrors the Rust struct.
type RetentionSweepOutput struct {
	Evicted       uint64 `json:"evicted"`
	OlderThanDays uint32 `json:"older_than_days"`
	DryRun        bool   `json:"dry_run"`
}

// RetentionSweepClient is the injectable boundary to audit-compliance-service.
type RetentionSweepClient interface {
	Sweep(ctx context.Context, in RetentionSweepInput) (RetentionSweepOutput, error)
}

// HTTPRetentionSweepClient calls audit-compliance-service's retention sweep endpoint.
type HTTPRetentionSweepClient struct {
	BaseURL     string
	BearerToken string
	HTTPClient  *http.Client
}

func NewHTTPRetentionSweepClient(client *http.Client, baseURL, bearerToken string) *HTTPRetentionSweepClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPRetentionSweepClient{HTTPClient: client, BaseURL: baseURL, BearerToken: bearerToken}
}

func (c *HTTPRetentionSweepClient) Sweep(ctx context.Context, in RetentionSweepInput) (RetentionSweepOutput, error) {
	if c == nil || c.HTTPClient == nil {
		return RetentionSweepOutput{}, fmt.Errorf("retention sweep client is not configured")
	}
	endpoint, err := joinEndpoint(c.BaseURL, retentionSweepPath)
	if err != nil {
		return RetentionSweepOutput{}, err
	}
	body, err := json.Marshal(in)
	if err != nil {
		return RetentionSweepOutput{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return RetentionSweepOutput{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.BearerToken)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return RetentionSweepOutput{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return RetentionSweepOutput{}, &AuditComplianceError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(payload))}
	}
	var out RetentionSweepOutput
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return RetentionSweepOutput{}, err
	}
	return out, nil
}

// AuditComplianceError captures non-2xx responses from audit-compliance-service.
type AuditComplianceError struct {
	StatusCode int
	Body       string
}

func (e *AuditComplianceError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("audit-compliance returned HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("audit-compliance returned HTTP %d: %s", e.StatusCode, e.Body)
}

func joinEndpoint(baseURL, path string) (string, error) {
	if strings.TrimSpace(baseURL) == "" {
		return "", fmt.Errorf("audit-compliance base URL is required")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	return u.String(), nil
}

// EvictRetentionEligible is the single step of the `retention.sweep`
// saga.
type EvictRetentionEligible struct {
	Client RetentionSweepClient
}

// StepName satisfies SagaStep.
func (EvictRetentionEligible) StepName() string { return "evict_retention_eligible" }

// Execute satisfies SagaStep by invoking audit-compliance-service.
func (s EvictRetentionEligible) Execute(ctx context.Context, in RetentionSweepInput) (RetentionSweepOutput, error) {
	if strings.TrimSpace(in.TenantID) == "" {
		return RetentionSweepOutput{}, saga.StepFailure(s.StepName(), "invalid input: tenant_id is required")
	}
	if in.OlderThanDays == 0 {
		return RetentionSweepOutput{}, saga.StepFailure(s.StepName(), "invalid input: older_than_days must be greater than 0")
	}
	if s.Client == nil {
		return RetentionSweepOutput{}, saga.StepFailure(s.StepName(), "retention sweep client is not configured")
	}
	out, err := s.Client.Sweep(ctx, in)
	if err != nil {
		if ctx.Err() != nil {
			return RetentionSweepOutput{}, saga.StepFailure(s.StepName(), "retention sweep request canceled: "+ctx.Err().Error())
		}
		if auditErr, ok := err.(*AuditComplianceError); ok {
			if auditErr.StatusCode >= 500 {
				return RetentionSweepOutput{}, saga.StepFailure(s.StepName(), "audit-compliance retention sweep unavailable: "+auditErr.Error())
			}
			return RetentionSweepOutput{}, saga.StepFailure(s.StepName(), "audit-compliance retention sweep rejected request: "+auditErr.Error())
		}
		return RetentionSweepOutput{}, saga.StepFailure(s.StepName(), "audit-compliance retention sweep failed: "+err.Error())
	}
	return out, nil
}

// Compensate satisfies SagaStep — nothing to compensate.
func (EvictRetentionEligible) Compensate(_ context.Context, _ RetentionSweepInput) error { return nil }
