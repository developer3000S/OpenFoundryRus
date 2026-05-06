package repo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// OAuthState is one row in oauth_state. The plaintext state token is
// the row key; verifier + nonce travel with it. SamlRequestID is
// non-nil when the row backs a SAML flow (slice 5b) so the callback
// can validate InResponseTo on the IdP's response — OIDC rows leave
// it nil.
type OAuthState struct {
	State          string
	CodeVerifier   string
	Provider       string
	RedirectAfter  string
	Nonce          string
	SamlRequestID  *string
	IssuedAt       time.Time
	ExpiresAt      time.Time
}

// InsertOAuthState persists a state row with the configured TTL.
func (r *Repo) InsertOAuthState(ctx context.Context, s *OAuthState) error {
	_, err := r.Pool.Exec(ctx,
		`INSERT INTO oauth_state (state, code_verifier, provider, redirect_after, nonce, saml_request_id, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		s.State, s.CodeVerifier, s.Provider, s.RedirectAfter, s.Nonce, s.SamlRequestID, s.ExpiresAt,
	)
	return err
}

// ConsumeOAuthState looks up + deletes the state row in one shot.
//
// Returns nil + nil err when the state is unknown / expired (caller
// should treat as 401). Successful return: the row was present, not
// expired, and is now deleted.
func (r *Repo) ConsumeOAuthState(ctx context.Context, state string) (*OAuthState, error) {
	row := r.Pool.QueryRow(ctx,
		`DELETE FROM oauth_state
		 WHERE state = $1 AND expires_at > NOW()
		 RETURNING state, code_verifier, provider, redirect_after, nonce, saml_request_id, issued_at, expires_at`,
		state,
	)
	s := &OAuthState{}
	err := row.Scan(&s.State, &s.CodeVerifier, &s.Provider, &s.RedirectAfter, &s.Nonce, &s.SamlRequestID, &s.IssuedAt, &s.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// ─── External identities ────────────────────────────────────────────────

// ExternalIdentity binds an OpenFoundry user to one (provider, external_id) tuple.
type ExternalIdentity struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Provider    string
	ExternalID  string
	Email       string
	LastLoginAt *time.Time
	CreatedAt   time.Time
}

// FindExternalIdentity returns the binding or nil when absent.
func (r *Repo) FindExternalIdentity(ctx context.Context, provider, externalID string) (*ExternalIdentity, error) {
	row := r.Pool.QueryRow(ctx,
		`SELECT id, user_id, provider, external_id, COALESCE(email, ''), last_login_at, created_at
		 FROM user_external_identities WHERE provider = $1 AND external_id = $2`,
		provider, externalID,
	)
	e := &ExternalIdentity{}
	if err := row.Scan(&e.ID, &e.UserID, &e.Provider, &e.ExternalID, &e.Email, &e.LastLoginAt, &e.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return e, nil
}

// LinkExternalIdentity binds an existing user to (provider, external_id).
//
// Caller is the SSO callback handler — slice 5a's policy is:
//   1. If provider+external_id already binds to a user → log them in.
//   2. Else if email matches an existing user → link + log in.
//   3. Else create a new user with auth_source=<provider>.
//
// This function only persists the binding; the caller orchestrates the policy.
func (r *Repo) LinkExternalIdentity(ctx context.Context, e *ExternalIdentity) error {
	_, err := r.Pool.Exec(ctx,
		`INSERT INTO user_external_identities (id, user_id, provider, external_id, email, last_login_at)
		 VALUES ($1, $2, $3, $4, $5, NOW())
		 ON CONFLICT (provider, external_id) DO UPDATE SET
		   email = EXCLUDED.email,
		   last_login_at = NOW()`,
		e.ID, e.UserID, e.Provider, e.ExternalID, e.Email,
	)
	return err
}

// CreateUserForSSO inserts a fresh users row with auth_source=<provider>.
// Used by the SSO callback when no existing user matches.
//
// Slice 5a does NOT auto-assign a role — first user still elects via
// /auth/register. SSO-created users get no role until an admin
// promotes them through the slice 6 user-management endpoints.
func (r *Repo) CreateUserForSSO(ctx context.Context, id uuid.UUID, email, name, provider string) error {
	_, err := r.Pool.Exec(ctx,
		`INSERT INTO users (id, email, name, password_hash, is_active, auth_source)
		 VALUES ($1, $2, $3, '', true, $4)`,
		id, email, name, provider,
	)
	return err
}
