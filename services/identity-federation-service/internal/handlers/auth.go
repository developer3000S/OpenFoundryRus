// Package handlers wires HTTP endpoints for identity-federation-service slice 1.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/openfoundry/openfoundry-go/libs/core-models/ids"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/models"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/repo"
	"github.com/openfoundry/openfoundry-go/services/identity-federation-service/internal/service"
)

// Auth wires register / login / token endpoints.
//
// WebAuthn detection is plugged in via a tiny interface so this file
// stays free of the heavy go-webauthn import surface (the concrete
// implementation lives in internal/webauthn). Pass nil to disable.
type Auth struct {
	Repo     *repo.Repo
	Issuer   *service.Issuer
	WebAuthn WebAuthnChecker // nil → WebAuthn detection skipped
}

// WebAuthnChecker is the bare-minimum surface Auth needs.
type WebAuthnChecker interface {
	HasCredentials(ctx context.Context, userID uuid.UUID) (bool, error)
}

// BootstrapStatus handles GET /api/v1/auth/bootstrap-status.
func (a *Auth) BootstrapStatus(w http.ResponseWriter, r *http.Request) {
	count, err := a.Repo.CountUsers(r.Context())
	if err != nil {
		slog.Error("count users failed", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to load bootstrap status")
		return
	}
	writeJSON(w, http.StatusOK, models.BootstrapStatusResponse{
		RequiresInitialAdmin: count == 0,
	})
}

// Register handles POST /api/v1/auth/register.
//
// Mirrors the Rust handler: argon2id password hash, advisory-lock-
// guarded transactional insert, first-user-becomes-admin election.
func (a *Auth) Register(w http.ResponseWriter, r *http.Request) {
	var body models.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Email == "" || body.Password == "" || body.Name == "" {
		writeJSONErr(w, http.StatusBadRequest, "email, password and name are required")
		return
	}

	hash, err := service.HashPassword(body.Password)
	if err != nil {
		slog.Error("hash password failed", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	user, role, err := a.Repo.CreateUserAndAssignRole(r.Context(), ids.New(), body.Email, body.Name, hash)
	if err != nil {
		if errors.Is(err, repo.ErrUserExists) {
			writeJSONErr(w, http.StatusConflict, "email already registered")
			return
		}
		slog.Error("register failed", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "registration failed")
		return
	}
	slog.Info("user registered",
		slog.String("user_id", user.ID.String()),
		slog.String("email", user.Email),
		slog.String("role", role),
	)
	writeJSON(w, http.StatusCreated, models.RegisterResponse{
		ID: user.ID, Email: user.Email, Name: user.Name,
	})
}

// Login handles POST /api/v1/auth/login.
//
// Slice 1 scope: password verification + JWT issuance. MFA returns
// `{"status":"mfa_required"}` with the MFA flag set; actual TOTP /
// WebAuthn challenge issuance arrives in slices 3 + 4.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var body models.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	user, err := a.Repo.FindUserByEmail(r.Context(), body.Email)
	if err != nil {
		slog.Error("lookup user failed", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "login failed")
		return
	}
	if user == nil {
		writeJSONErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !user.IsActive {
		writeJSONErr(w, http.StatusForbidden, "account disabled")
		return
	}

	if err := service.VerifyPassword(body.Password, user.PasswordHash); err != nil {
		writeJSONErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Slice 3: TOTP. WebAuthn arrives in slice 4 — `methods` will
	// grow accordingly. mfa_enforced=true users with no enrolment
	// yet still land in the MFA-required branch; the frontend
	// redirects them through the enrolment flow after the access
	// token has been issued (cannot enrol before authenticating).
	totpCfg, terr := a.Repo.FindTOTPConfig(r.Context(), user.ID)
	if terr != nil {
		slog.Error("login: find totp", slog.String("error", terr.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "login failed")
		return
	}
	totpEnabled := totpCfg != nil && totpCfg.Enabled
	hasWebAuthn := false
	if a.WebAuthn != nil {
		var werr error
		hasWebAuthn, werr = a.WebAuthn.HasCredentials(r.Context(), user.ID)
		if werr != nil {
			slog.Warn("login: webauthn lookup", slog.String("error", werr.Error()))
		}
	}
	if totpEnabled || hasWebAuthn || user.MFAEnforced {
		methods := []string{}
		if totpEnabled {
			methods = append(methods, "totp")
		}
		if hasWebAuthn {
			methods = append(methods, "webauthn")
		}
		challenge, cerr := service.IssueMFAChallenge(a.Issuer.JWT, user, "password")
		if cerr != nil {
			slog.Error("login: mfa challenge", slog.String("error", cerr.Error()))
			writeJSONErr(w, http.StatusInternalServerError, "login failed")
			return
		}
		writeJSON(w, http.StatusOK, models.LoginResponse{
			Status:         models.LoginStatusMFARequired,
			ChallengeToken: challenge,
			Methods:        methods,
			ExpiresIn:      int64(service.ChallengeTTL.Seconds()),
		})
		return
	}

	access, refresh, err := a.Issuer.IssueTokens(r.Context(), user, []string{"password"})
	if err != nil {
		slog.Error("issue tokens failed", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusInternalServerError, "login failed")
		return
	}
	// SG.4: stamp last_login_at + last_login_ip. Best-effort —
	// failure does not block the authentication response.
	if err := a.Repo.StampLogin(r.Context(), user.ID, time.Now().UTC(), clientIP(r)); err != nil {
		slog.Warn("stamp login", slog.String("user_id", user.ID.String()), slog.String("error", err.Error()))
	}
	writeJSON(w, http.StatusOK, models.LoginResponse{
		Status:       models.LoginStatusAuthenticated,
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(a.Issuer.AccessTTL.Seconds()),
	})
}

// clientIP picks an honest IP for the request: respects an explicit
// X-Forwarded-For (first hop) when present, otherwise falls back to
// r.RemoteAddr's host part. Empty string is acceptable — the caller
// stores it as NULL.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// First hop is the original client.
		if i := strings.Index(xff, ","); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// Refresh handles POST /api/v1/auth/token/refresh.
func (a *Auth) Refresh(w http.ResponseWriter, r *http.Request) {
	var body models.RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	access, refresh, err := a.Issuer.RefreshTokens(r.Context(), body.RefreshToken)
	if err != nil {
		// Both Invalid + Reused map to 401 — the client should drop
		// the family and reauthenticate. The slog log keeps them apart.
		slog.Warn("refresh failed", slog.String("error", err.Error()))
		writeJSONErr(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	writeJSON(w, http.StatusOK, models.TokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresIn:    int64(a.Issuer.AccessTTL.Seconds()),
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSONErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
