package authmw

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTAlgorithm enumerates the algorithms this package can sign / verify.
type JWTAlgorithm string

const (
	AlgHS256 JWTAlgorithm = "HS256"
	AlgRS256 JWTAlgorithm = "RS256"
)

// GeneratedSecretBytes is the size of an auto-generated HS256 secret
// (32 bytes = 256 bits, matching the Rust constant).
const GeneratedSecretBytes = 32

// JWTConfig is the signing / validation configuration for a service.
//
// Mirrors `auth_middleware::JwtConfig`. Default TTLs match the Rust
// defaults (3600s access, 7d refresh) so issued tokens behave identically.
type JWTConfig struct {
	secret       []byte
	rsaPrivKey   *rsa.PrivateKey
	rsaPubKey    *rsa.PublicKey
	Issuer       string
	Audience     string
	KeyID        string
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
}

// NewJWTConfig builds an HS256 config from a raw secret string.
func NewJWTConfig(secret string) *JWTConfig {
	return &JWTConfig{
		secret:     []byte(secret),
		AccessTTL:  time.Hour,
		RefreshTTL: 7 * 24 * time.Hour,
	}
}

// FromSecretBytes builds an HS256 config from raw bytes (for KMS-sourced secrets).
func FromSecretBytes(secret []byte) *JWTConfig {
	return &JWTConfig{
		secret:     secret,
		AccessTTL:  time.Hour,
		RefreshTTL: 7 * 24 * time.Hour,
	}
}

// Generate mints an HS256 config backed by a random 256-bit secret.
//
// The secret only lives in memory for the lifetime of the returned
// value. Use a persistence mechanism for long-lived deployments.
func Generate() (*JWTConfig, error) {
	buf := make([]byte, GeneratedSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate secret: %w", err)
	}
	return FromSecretBytes(buf), nil
}

// WithIssuer / WithAudience / WithKeyID / WithRSAKeys are builder helpers
// matching the Rust fluent API.
func (c *JWTConfig) WithIssuer(iss string) *JWTConfig     { c.Issuer = iss; return c }
func (c *JWTConfig) WithAudience(aud string) *JWTConfig   { c.Audience = aud; return c }
func (c *JWTConfig) WithKeyID(kid string) *JWTConfig      { c.KeyID = kid; return c }
func (c *JWTConfig) WithAccessTTL(d time.Duration) *JWTConfig {
	c.AccessTTL = d
	return c
}
func (c *JWTConfig) WithRefreshTTL(d time.Duration) *JWTConfig {
	c.RefreshTTL = d
	return c
}

// WithRSAKeys upgrades the config to RS256.
func (c *JWTConfig) WithRSAKeys(privPEM, pubPEM string) (*JWTConfig, error) {
	priv, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privPEM))
	if err != nil {
		return nil, fmt.Errorf("parse rsa private key: %w", err)
	}
	pub, err := jwt.ParseRSAPublicKeyFromPEM([]byte(pubPEM))
	if err != nil {
		return nil, fmt.Errorf("parse rsa public key: %w", err)
	}
	c.rsaPrivKey = priv
	c.rsaPubKey = pub
	return c, nil
}

// Algorithm reports which signing algorithm this config will use.
func (c *JWTConfig) Algorithm() JWTAlgorithm {
	if c.rsaPrivKey != nil && c.rsaPubKey != nil {
		return AlgRS256
	}
	return AlgHS256
}

// EncodeToken serialises the claims into a signed JWT string.
func EncodeToken(c *JWTConfig, claims *Claims) (string, error) {
	var method jwt.SigningMethod
	var key any
	switch c.Algorithm() {
	case AlgHS256:
		method = jwt.SigningMethodHS256
		key = c.secret
	case AlgRS256:
		method = jwt.SigningMethodRS256
		key = c.rsaPrivKey
	default:
		return "", errors.New("unsupported signing algorithm")
	}
	tok := jwt.NewWithClaims(method, jwtClaimsWrapper{Claims: claims})
	if c.KeyID != "" {
		tok.Header["kid"] = c.KeyID
	}
	signed, err := tok.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return signed, nil
}

// DecodeToken validates the signed token and returns the embedded Claims.
func DecodeToken(c *JWTConfig, token string) (*Claims, error) {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{string(c.Algorithm())}))
	wrapper := &jwtClaimsWrapper{Claims: &Claims{}}
	parsed, err := parser.ParseWithClaims(token, wrapper, func(t *jwt.Token) (any, error) {
		switch c.Algorithm() {
		case AlgHS256:
			return c.secret, nil
		case AlgRS256:
			return c.rsaPubKey, nil
		}
		return nil, fmt.Errorf("unsupported alg %v", t.Method.Alg())
	})
	if err != nil {
		// jwt-go validates exp before we get here; surface that as a
		// typed-expired error so callers can reauthenticate.
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, &JWTError{Kind: errKindExpired, Cause: err}
		}
		return nil, &JWTError{Kind: errKindInvalid, Cause: err}
	}
	if !parsed.Valid {
		return nil, &JWTError{Kind: errKindInvalid, Cause: errors.New("token not valid")}
	}
	if c.Issuer != "" && (wrapper.ISS == nil || *wrapper.ISS != c.Issuer) {
		return nil, &JWTError{Kind: errKindInvalid, Cause: fmt.Errorf("issuer mismatch")}
	}
	if c.Audience != "" && (wrapper.AUD == nil || *wrapper.AUD != c.Audience) {
		return nil, &JWTError{Kind: errKindInvalid, Cause: fmt.Errorf("audience mismatch")}
	}
	if wrapper.Claims.IsExpired() {
		return nil, &JWTError{Kind: errKindExpired}
	}
	return wrapper.Claims, nil
}

// jwtClaimsWrapper adapts our Claims type to the jwt-go interface.
type jwtClaimsWrapper struct {
	*Claims
}

func (w jwtClaimsWrapper) GetExpirationTime() (*jwt.NumericDate, error) {
	return jwt.NewNumericDate(time.Unix(w.EXP, 0)), nil
}

func (w jwtClaimsWrapper) GetIssuedAt() (*jwt.NumericDate, error) {
	return jwt.NewNumericDate(time.Unix(w.IAT, 0)), nil
}

func (w jwtClaimsWrapper) GetNotBefore() (*jwt.NumericDate, error) { return nil, nil }
func (w jwtClaimsWrapper) GetIssuer() (string, error) {
	if w.ISS == nil {
		return "", nil
	}
	return *w.ISS, nil
}

func (w jwtClaimsWrapper) GetSubject() (string, error) { return w.Sub.String(), nil }
func (w jwtClaimsWrapper) GetAudience() (jwt.ClaimStrings, error) {
	if w.AUD == nil {
		return nil, nil
	}
	return jwt.ClaimStrings{*w.AUD}, nil
}

// JWTError categorises decode failures.
type JWTError struct {
	Kind  errKind
	Cause error
}

type errKind int

const (
	errKindInvalid errKind = iota
	errKindExpired
)

func (e *JWTError) Error() string {
	switch e.Kind {
	case errKindExpired:
		return "token expired"
	default:
		if e.Cause != nil {
			return fmt.Sprintf("invalid token: %s", e.Cause)
		}
		return "invalid token"
	}
}

func (e *JWTError) Unwrap() error { return e.Cause }

// IsExpired reports whether err is a token-expired JWTError.
func IsExpired(err error) bool {
	var je *JWTError
	if !errors.As(err, &je) {
		return false
	}
	return je.Kind == errKindExpired
}
