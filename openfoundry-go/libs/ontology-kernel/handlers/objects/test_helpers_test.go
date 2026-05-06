package objects

import (
	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
)

// claimsWithDefaultTenant satisfies the *Claims pointer that the
// public helpers expect. Empty OrgID → tenant resolves to "default",
// matching the Rust `tenant_from_claims` fallback.
type claimsWithDefaultTenant = authmw.Claims
