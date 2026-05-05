package proxy

import (
	"net/http"
	"strconv"
	"strings"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
)

// Header keys forwarded downstream — match the Rust gateway exactly so
// any service can verify caller identity / quotas without parsing the JWT.
const (
	HdrTenantScope             = "x-openfoundry-tenant-scope"
	HdrTenantTier              = "x-openfoundry-tenant-tier"
	HdrQuotaQueryLimit         = "x-openfoundry-quota-query-limit"
	HdrQuotaPipelineWorkers    = "x-openfoundry-quota-pipeline-workers"
	HdrQuotaRequestsPerMin     = "x-openfoundry-quota-requests-per-minute"
	HdrAuthSub                 = "x-openfoundry-auth-sub"
	HdrAuthEmail               = "x-openfoundry-auth-email"
	HdrAuthMethods             = "x-openfoundry-auth-methods"
	HdrZeroTrust               = "x-openfoundry-zero-trust"
	HdrOrgID                   = "x-openfoundry-org-id"
	HdrSessionKind             = "x-openfoundry-session-kind"
	HdrClassificationClearance = "x-openfoundry-classification-clearance"
	HdrScopeWorkspace          = "x-openfoundry-scope-workspace"
	HdrScopePathPrefixes       = "x-openfoundry-scope-path-prefixes"
	HdrAllowedOrgIDs           = "x-openfoundry-allowed-org-ids"
	HdrAllowedMarkings         = "x-openfoundry-allowed-markings"
	HdrRestrictedViewIDs       = "x-openfoundry-restricted-view-ids"
	HdrConsumerMode            = "x-openfoundry-consumer-mode"
	HdrGuestEmail              = "x-openfoundry-guest-email"
	HdrGuestAccess             = "x-openfoundry-guest-access"
)

// ApplyTenantHeaders sets the per-tenant headers on the upstream request.
func ApplyTenantHeaders(req *http.Request, t *authmw.TenantContext) {
	if t == nil {
		return
	}
	req.Header.Set(HdrTenantScope, t.ScopeID)
	req.Header.Set(HdrTenantTier, t.Tier)
	req.Header.Set(HdrQuotaQueryLimit, strconv.FormatUint(uint64(t.Quotas.MaxQueryLimit), 10))
	req.Header.Set(HdrQuotaPipelineWorkers, strconv.FormatUint(uint64(t.Quotas.MaxPipelineWorkers), 10))
	req.Header.Set(HdrQuotaRequestsPerMin, strconv.FormatUint(uint64(t.Quotas.RequestsPerMinute), 10))
}

// ApplyAuthContextHeaders mirrors the Rust `apply_auth_context_headers`
// function: copies subject / email / org / session-scope details onto
// the upstream request so downstream services can enforce ABAC without
// re-decoding the JWT.
func ApplyAuthContextHeaders(req *http.Request, c *authmw.Claims) {
	if c == nil {
		return
	}
	req.Header.Set(HdrAuthSub, c.Sub.String())
	req.Header.Set(HdrAuthEmail, c.Email)
	req.Header.Set(HdrAuthMethods, strings.Join(c.AuthMethods, ","))

	zeroTrust := "standard"
	if c.SessionScope != nil {
		zeroTrust = "scoped"
	}
	req.Header.Set(HdrZeroTrust, zeroTrust)

	if c.OrgID != nil {
		req.Header.Set(HdrOrgID, c.OrgID.String())
	}
	if c.SessionKind != nil && *c.SessionKind != "" {
		req.Header.Set(HdrSessionKind, *c.SessionKind)
	}
	if clr, ok := c.ClassificationClearance(); ok {
		req.Header.Set(HdrClassificationClearance, clr)
	}

	scope := c.SessionScope
	if scope == nil {
		return
	}
	if scope.Workspace != nil && *scope.Workspace != "" {
		req.Header.Set(HdrScopeWorkspace, *scope.Workspace)
	}
	if len(scope.AllowedPathPrefixes) > 0 {
		req.Header.Set(HdrScopePathPrefixes, strings.Join(scope.AllowedPathPrefixes, ","))
	}
	if len(scope.AllowedOrgIDs) > 0 {
		ids := make([]string, len(scope.AllowedOrgIDs))
		for i, id := range scope.AllowedOrgIDs {
			ids[i] = id.String()
		}
		req.Header.Set(HdrAllowedOrgIDs, strings.Join(ids, ","))
	}
	if len(scope.AllowedMarkings) > 0 {
		req.Header.Set(HdrAllowedMarkings, strings.Join(scope.AllowedMarkings, ","))
	}
	if len(scope.RestrictedViewIDs) > 0 {
		ids := make([]string, len(scope.RestrictedViewIDs))
		for i, id := range scope.RestrictedViewIDs {
			ids[i] = id.String()
		}
		req.Header.Set(HdrRestrictedViewIDs, strings.Join(ids, ","))
	}
	if scope.ConsumerMode {
		req.Header.Set(HdrConsumerMode, "true")
	}
	if scope.GuestEmail != nil && *scope.GuestEmail != "" {
		req.Header.Set(HdrGuestEmail, *scope.GuestEmail)
		req.Header.Set(HdrGuestAccess, "true")
	}
}
