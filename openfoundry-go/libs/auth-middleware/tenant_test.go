package authmw_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
)

func TestQuotaTiersMatchRustNumbers(t *testing.T) {
	t.Parallel()
	std := authmw.QuotaStandard()
	team := authmw.QuotaTeam()
	ent := authmw.QuotaEnterprise()

	assert.Equal(t, uint32(2_000), std.MaxQueryLimit)
	assert.Equal(t, uint32(300), std.RequestsPerMinute)
	assert.Equal(t, uint32(5_000), team.MaxQueryLimit)
	assert.Equal(t, uint32(900), team.RequestsPerMinute)
	assert.Equal(t, uint32(10_000), ent.MaxQueryLimit)
	assert.Equal(t, uint32(5_000), ent.RequestsPerMinute)
}

func TestTenantContextResolvesTierFromAttributes(t *testing.T) {
	t.Parallel()
	orgID := uuid.New()
	c := &authmw.Claims{
		Sub:        uuid.New(),
		OrgID:      &orgID,
		Roles:      []string{"member"},
		Attributes: json.RawMessage(`{"tenant_tier":"team"}`),
	}
	tc := authmw.TenantContextFromClaims(c)
	assert.Equal(t, "team", tc.Tier)
	assert.Equal(t, orgID.String(), tc.ScopeID)
	assert.Equal(t, uint32(900), tc.Quotas.RequestsPerMinute)
	assert.Equal(t, &orgID, tc.TenantID)
}

func TestTenantContextAdminUpgradesToEnterprise(t *testing.T) {
	t.Parallel()
	c := &authmw.Claims{
		Sub:   uuid.New(),
		Roles: []string{"admin"},
	}
	tc := authmw.TenantContextFromClaims(c)
	assert.Equal(t, "enterprise", tc.Tier)
}

func TestTenantContextHonoursPerClaimOverrides(t *testing.T) {
	t.Parallel()
	c := &authmw.Claims{
		Sub:        uuid.New(),
		Attributes: json.RawMessage(`{"tenant_tier":"standard","tenant_quotas":{"requests_per_minute":50}}`),
	}
	tc := authmw.TenantContextFromClaims(c)
	assert.Equal(t, uint32(50), tc.Quotas.RequestsPerMinute)
	assert.Equal(t, uint32(2_000), tc.Quotas.MaxQueryLimit, "untouched override stays at tier default")
}

func TestTenantContextFallsBackToSubjectScope(t *testing.T) {
	t.Parallel()
	sub := uuid.New()
	c := &authmw.Claims{Sub: sub} // no org_id
	tc := authmw.TenantContextFromClaims(c)
	assert.Equal(t, sub.String(), tc.ScopeID)
	assert.Nil(t, tc.TenantID)
}

func TestClampRequestBody(t *testing.T) {
	t.Parallel()
	tc := authmw.TenantContext{Quotas: authmw.QuotaStandard()}
	assert.Equal(t, uint64(10*1024*1024), tc.ClampRequestBodyBytes(1<<30))
	assert.Equal(t, uint64(1024), tc.ClampRequestBodyBytes(1024))
}
