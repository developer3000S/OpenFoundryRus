// Package ratelimit implements the gateway's token-bucket rate limiter.
//
// Wire compatibility with the Rust implementation:
//
//   - Authenticated requests are keyed `tenant:<org_id>` with the
//     tenant tier's RequestsPerMinute as the limit.
//   - Anonymous requests are keyed `anonymous:<ip>` with the
//     configured AnonymousRequestsPerMinute as the limit.
//   - Burst capacity = min(configured BurstSize, limit).
//   - Refill rate = limit / 60 tokens per second.
//   - Response headers `x-ratelimit-limit`, `x-ratelimit-remaining`,
//     `x-ratelimit-reset` always set; `retry-after` set on 429.
//   - 429 body is `{"error":{"code":"rate_limit_exceeded","message":"rate limit exceeded"}}`.
//
// The Store interface lets us swap in-memory and Redis backends; this
// file ships the in-memory implementation. Redis is added under
// redis.go behind the same interface.
package ratelimit

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
	"github.com/openfoundry/openfoundry-go/services/edge-gateway-service/internal/errs"
)

// Outcome reports the result of a single Allow() check.
type Outcome struct {
	Allowed     bool
	Limit       uint32 // requests-per-minute applied
	Remaining   uint32 // tokens left in the bucket
	ResetAfter  time.Duration
}

// Store is the rate-limit backend abstraction.
//
// `key` is the dedup key (tenant or IP) and `limitPerMinute` /
// `burst` are the policy values for this caller. Implementations MUST
// be goroutine-safe.
type Store interface {
	Allow(key string, limitPerMinute, burst uint32) (Outcome, error)
}

// Config controls the limiter middleware.
type Config struct {
	AnonymousRequestsPerMinute uint32
	BurstSize                  uint32
	BucketTTL                  time.Duration
	JWT                        *authmw.JWTConfig // optional — anonymous-only when nil
}

// Middleware returns the rate-limit middleware.
//
// Classification:
//   - Bearer token + decode → tenant:<scope_id> with tenant-tier RPM.
//   - No bearer / decode failure → anonymous:<ip>.
func Middleware(cfg Config, store Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key, limit := classify(r, cfg)
			burst := minU32(cfg.BurstSize, limit)

			outcome, err := store.Allow(key, limit, burst)
			if err != nil {
				// Fail-open on backend errors: gateway must not become
				// the single point of failure for the whole platform.
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Limit", strconv.FormatUint(uint64(outcome.Limit), 10))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatUint(uint64(outcome.Remaining), 10))
			resetSecs := uint64(outcome.ResetAfter.Seconds())
			w.Header().Set("X-RateLimit-Reset", strconv.FormatUint(resetSecs, 10))

			if !outcome.Allowed {
				w.Header().Set("Retry-After", strconv.FormatUint(resetSecs, 10))
				errs.Write(w, http.StatusTooManyRequests,
					errs.CodeRateLimitExceeded, "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// classify returns (key, limit) for the request.
func classify(r *http.Request, cfg Config) (string, uint32) {
	if cfg.JWT != nil {
		if claims := tryDecode(r, cfg.JWT); claims != nil {
			tenant := authmw.TenantContextFromClaims(claims)
			return "tenant:" + tenant.ScopeID, tenant.Quotas.RequestsPerMinute
		}
	}
	return "anonymous:" + remoteIP(r), cfg.AnonymousRequestsPerMinute
}

func tryDecode(r *http.Request, cfg *authmw.JWTConfig) *authmw.Claims {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return nil
	}
	tok, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok {
		return nil
	}
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return nil
	}
	claims, err := authmw.DecodeToken(cfg, tok)
	if err != nil {
		return nil
	}
	return claims
}

// remoteIP extracts the caller IP, mirroring the Rust precedence:
// X-Forwarded-For → X-Real-IP → CF-Connecting-IP → RemoteAddr → "global".
func remoteIP(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		// Take the first hop.
		if idx := strings.Index(v, ","); idx > 0 {
			return strings.TrimSpace(v[:idx])
		}
		return strings.TrimSpace(v)
	}
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return v
	}
	if v := r.Header.Get("CF-Connecting-IP"); v != "" {
		return v
	}
	if r.RemoteAddr != "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil {
			return host
		}
		return r.RemoteAddr
	}
	return "global"
}

// ─── In-memory token bucket ─────────────────────────────────────────────

// MemoryStore is the default backend. Goroutine-safe; periodically
// evicts buckets idle longer than BucketTTL so the map doesn't grow
// without bound.
type MemoryStore struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	ttl     time.Duration
	now     func() time.Time // injectable clock for tests
}

type bucket struct {
	tokens     float64
	lastRefill time.Time
	lastTouch  time.Time
}

// NewMemoryStore returns an empty MemoryStore. Pass `0` for ttl to
// disable cleanup (tests / very small deployments).
func NewMemoryStore(ttl time.Duration) *MemoryStore {
	return &MemoryStore{
		buckets: make(map[string]*bucket),
		ttl:     ttl,
		now:     time.Now,
	}
}

// Allow implements Store.
func (s *MemoryStore) Allow(key string, limitPerMinute, burst uint32) (Outcome, error) {
	if limitPerMinute == 0 {
		// Treat as "deny all" rather than divide-by-zero.
		return Outcome{Allowed: false, Limit: 0, Remaining: 0, ResetAfter: time.Minute}, nil
	}
	if burst == 0 {
		burst = 1
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	b, ok := s.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(burst), lastRefill: now}
		s.buckets[key] = b
	}

	// Refill based on elapsed time.
	refillRate := float64(limitPerMinute) / 60.0
	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * refillRate
		if b.tokens > float64(burst) {
			b.tokens = float64(burst)
		}
		b.lastRefill = now
	}
	b.lastTouch = now

	allowed := b.tokens >= 1
	if allowed {
		b.tokens -= 1
	}
	remaining := uint32(0)
	if b.tokens > 0 {
		remaining = uint32(b.tokens)
	}

	resetAfter := time.Duration(0)
	if !allowed {
		// Time until at least 1 token is available.
		needed := 1 - b.tokens
		resetAfter = time.Duration((needed / refillRate) * float64(time.Second))
	}

	s.maybeEvict(now)

	return Outcome{
		Allowed:    allowed,
		Limit:      limitPerMinute,
		Remaining:  remaining,
		ResetAfter: resetAfter,
	}, nil
}

// maybeEvict drops buckets idle for longer than ttl. Called while the
// lock is held; runs at most once per call but stays O(N) so high-
// cardinality clusters should prefer the Redis backend.
func (s *MemoryStore) maybeEvict(now time.Time) {
	if s.ttl == 0 {
		return
	}
	cutoff := now.Add(-s.ttl)
	for k, b := range s.buckets {
		if b.lastTouch.Before(cutoff) {
			delete(s.buckets, k)
		}
	}
}

func minU32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}
