package testingx

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/libs/core-models/ids"
)

// JWTSecret is the shared HS256 secret used across the integration
// test suite. Long enough for HS256 length checks; never used outside tests.
const JWTSecret = "openfoundry-shared-test-secret-do-not-use-in-prod-aaaa"

// JWTConfig builds an authmw.JWTConfig from JWTSecret.
func JWTConfig() *authmw.JWTConfig {
	return authmw.NewJWTConfig(JWTSecret)
}

// DevToken issues a Bearer-ready JWT with the requested permissions.
//
// The subject is randomised so each call produces a distinct user. The
// returned token passes the production Middleware without special-case
// handling — same contract as the Rust `dev_token` helper.
func DevToken(cfg *authmw.JWTConfig, permissions []string) (string, error) {
	now := time.Now()
	c := &authmw.Claims{
		Sub:         ids.New(),
		IAT:         now.Unix(),
		EXP:         now.Add(cfg.AccessTTL).Unix(),
		JTI:         ids.New(),
		Email:       "tester@openfoundry.test",
		Name:        "Integration Tester",
		Roles:       []string{"admin"},
		Permissions: permissions,
		Attributes:  json.RawMessage(`null`),
		AuthMethods: []string{"password"},
	}
	return authmw.EncodeToken(cfg, c)
}

// SeedDataset inserts a minimal `datasets` row and returns its id.
//
// Works against any schema that uses the canonical column set:
// (id, rid, name, format, storage_path, owner_id) — both the catalog
// and versioning schemas follow this convention.
func SeedDataset(ctx context.Context, pool *pgxpool.Pool, rid, name, format string) (uuid.UUID, error) {
	id := ids.New()
	_, err := pool.Exec(ctx,
		`INSERT INTO datasets (id, rid, name, format, storage_path, owner_id)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		id, rid, name, format, "local://"+rid+"/v1", ids.New(),
	)
	return id, err
}
