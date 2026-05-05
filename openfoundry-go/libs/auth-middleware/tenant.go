package authmw

import (
	"encoding/json"

	"github.com/google/uuid"
)

// TenantQuotaPolicy is the resource budget a tenant tier carries through
// every gateway hop. Values match the Rust crate verbatim so dashboards,
// runbooks and the upstream services see the same numbers regardless
// of which language emitted the headers.
type TenantQuotaPolicy struct {
	MaxQueryLimit              uint32 `json:"max_query_limit"`
	MaxDistributedQueryWorkers uint32 `json:"max_distributed_query_workers"`
	MaxPipelineWorkers         uint32 `json:"max_pipeline_workers"`
	MaxRequestBodyBytes        uint64 `json:"max_request_body_bytes"`
	RequestsPerMinute          uint32 `json:"requests_per_minute"`
}

// QuotaStandard / Team / Enterprise — three tiers with the exact
// numbers the Rust workspace ships with.
func QuotaStandard() TenantQuotaPolicy {
	return TenantQuotaPolicy{
		MaxQueryLimit:              2_000,
		MaxDistributedQueryWorkers: 2,
		MaxPipelineWorkers:         2,
		MaxRequestBodyBytes:        10 * 1024 * 1024,
		RequestsPerMinute:          300,
	}
}

func QuotaTeam() TenantQuotaPolicy {
	return TenantQuotaPolicy{
		MaxQueryLimit:              5_000,
		MaxDistributedQueryWorkers: 4,
		MaxPipelineWorkers:         4,
		MaxRequestBodyBytes:        20 * 1024 * 1024,
		RequestsPerMinute:          900,
	}
}

func QuotaEnterprise() TenantQuotaPolicy {
	return TenantQuotaPolicy{
		MaxQueryLimit:              10_000,
		MaxDistributedQueryWorkers: 8,
		MaxPipelineWorkers:         8,
		MaxRequestBodyBytes:        50 * 1024 * 1024,
		RequestsPerMinute:          5_000,
	}
}

// TenantContext is the per-request tenant projection the gateway
// derives from JWT claims and forwards downstream as
// `x-openfoundry-tenant-*` / `x-openfoundry-quota-*` headers.
type TenantContext struct {
	TenantID  *uuid.UUID
	ScopeID   string
	Tier      string
	Workspace string
	Quotas    TenantQuotaPolicy
}

// TenantContextFromClaims resolves a TenantContext from validated JWT claims.
//
// Tier resolution precedence (matches Rust):
//  1. claims.attributes["tenant_tier"] (string)
//  2. "enterprise" if the subject has the admin role
//  3. "standard"
//
// Quotas: pick by tier, then apply per-claim overrides under
// `attributes["tenant_quotas"]`. ScopeID falls back to subject when
// org_id is unset so anonymous-but-authenticated users still produce
// stable rate-limit keys.
func TenantContextFromClaims(c *Claims) TenantContext {
	attrs := decodeAttributes(c.Attributes)

	tier, _ := attrs["tenant_tier"].(string)
	if tier == "" {
		if c.HasRole("admin") {
			tier = "enterprise"
		} else {
			tier = "standard"
		}
	}

	quotas := quotaFor(tier)
	applyQuotaOverrides(&quotas, attrs["tenant_quotas"])

	workspace, _ := attrs["workspace"].(string)

	scopeID := c.Sub.String()
	if c.OrgID != nil {
		scopeID = c.OrgID.String()
	}

	return TenantContext{
		TenantID:  c.OrgID,
		ScopeID:   scopeID,
		Tier:      tier,
		Workspace: workspace,
		Quotas:    quotas,
	}
}

// AnonymousTenant returns the standard-tier context the gateway uses
// when no JWT is present (or decode fails).
func AnonymousTenant() TenantContext {
	return TenantContext{
		ScopeID: "global",
		Tier:    "standard",
		Quotas:  QuotaStandard(),
	}
}

// ClampQueryLimit / ClampQueryWorkers / ClampPipelineWorkers /
// ClampRequestBodyBytes apply a per-tenant ceiling to a request value.
// Mirror the Rust accessors verbatim — `max(1)` floor included.
func (t TenantContext) ClampQueryLimit(requested uint32) uint32 {
	return clampU32(requested, t.Quotas.MaxQueryLimit)
}
func (t TenantContext) ClampQueryWorkers(requested uint32) uint32 {
	return clampU32(requested, t.Quotas.MaxDistributedQueryWorkers)
}
func (t TenantContext) ClampPipelineWorkers(requested uint32) uint32 {
	return clampU32(requested, t.Quotas.MaxPipelineWorkers)
}
func (t TenantContext) ClampRequestBodyBytes(requested uint64) uint64 {
	if t.Quotas.MaxRequestBodyBytes == 0 {
		return min64(requested, 1)
	}
	return min64(requested, t.Quotas.MaxRequestBodyBytes)
}

func clampU32(req, max uint32) uint32 {
	if max == 0 {
		max = 1
	}
	if req < max {
		return req
	}
	return max
}

func min64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func quotaFor(tier string) TenantQuotaPolicy {
	switch tier {
	case "enterprise":
		return QuotaEnterprise()
	case "team":
		return QuotaTeam()
	default:
		return QuotaStandard()
	}
}

func decodeAttributes(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var attrs map[string]any
	if err := json.Unmarshal(raw, &attrs); err != nil {
		return nil
	}
	return attrs
}

func applyQuotaOverrides(q *TenantQuotaPolicy, raw any) {
	overrides, ok := raw.(map[string]any)
	if !ok {
		return
	}
	if v, ok := overrides["max_query_limit"].(float64); ok {
		q.MaxQueryLimit = uint32(v)
	}
	if v, ok := overrides["max_distributed_query_workers"].(float64); ok {
		q.MaxDistributedQueryWorkers = uint32(v)
	}
	if v, ok := overrides["max_pipeline_workers"].(float64); ok {
		q.MaxPipelineWorkers = uint32(v)
	}
	if v, ok := overrides["max_request_body_bytes"].(float64); ok {
		q.MaxRequestBodyBytes = uint64(v)
	}
	if v, ok := overrides["requests_per_minute"].(float64); ok {
		q.RequestsPerMinute = uint32(v)
	}
}
