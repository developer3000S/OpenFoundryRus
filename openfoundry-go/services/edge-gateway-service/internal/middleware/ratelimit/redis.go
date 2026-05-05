package ratelimit

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore is the distributed rate-limit backend.
//
// Implements the same atomic token-bucket math as the Rust gateway via
// a Lua script (single round-trip, no race window). The Lua script is
// the same shape as the Rust crate's: bucket_ttl seeds the EXPIRE,
// the bucket key is `<prefix>:<id>`, and the script returns
// (allowed, tokens_left, reset_after_ms).
//
// On any Redis-side error the middleware wrapper falls back to letting
// the request through — the gateway must not become the cluster's
// single point of failure.
type RedisStore struct {
	Client    redis.UniversalClient
	KeyPrefix string
	TTL       time.Duration
}

// NewRedisStore wires a RedisStore.
func NewRedisStore(client redis.UniversalClient, keyPrefix string, ttl time.Duration) *RedisStore {
	if keyPrefix == "" {
		keyPrefix = "openfoundry:ratelimit"
	}
	if ttl == 0 {
		ttl = 5 * time.Minute
	}
	return &RedisStore{Client: client, KeyPrefix: keyPrefix, TTL: ttl}
}

// luaTokenBucket is the atomic refill+consume script.
//
// KEYS[1] = bucket hash key
// ARGV[1] = limit per minute
// ARGV[2] = burst capacity
// ARGV[3] = now in seconds (float)
// ARGV[4] = TTL seconds (integer)
//
// Returns: { allowed, remaining_tokens_floor, reset_after_ms }.
const luaTokenBucket = `
local key = KEYS[1]
local limit_per_minute = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])
local refill_rate = limit_per_minute / 60

local data = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(data[1])
local last_refill = tonumber(data[2])

if tokens == nil then
  tokens = burst
  last_refill = now
else
  local elapsed = now - last_refill
  if elapsed > 0 then
    tokens = math.min(burst, tokens + elapsed * refill_rate)
    last_refill = now
  end
end

local allowed = 0
local reset_after_ms = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
else
  local needed = 1 - tokens
  reset_after_ms = math.ceil((needed / refill_rate) * 1000)
end

redis.call('HMSET', key, 'tokens', tokens, 'last_refill', last_refill)
redis.call('EXPIRE', key, ttl)

return { allowed, math.floor(math.max(tokens, 0)), reset_after_ms }
`

// Allow implements Store.
func (s *RedisStore) Allow(key string, limitPerMinute, burst uint32) (Outcome, error) {
	if limitPerMinute == 0 {
		return Outcome{Allowed: false, Limit: 0, ResetAfter: time.Minute}, nil
	}
	if burst == 0 {
		burst = 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	bucketKey := s.KeyPrefix + ":" + key
	now := strconv.FormatFloat(float64(time.Now().UnixMilli())/1000.0, 'f', 3, 64)
	res, err := s.Client.Eval(ctx, luaTokenBucket, []string{bucketKey},
		strconv.FormatUint(uint64(limitPerMinute), 10),
		strconv.FormatUint(uint64(burst), 10),
		now,
		strconv.FormatFloat(s.TTL.Seconds(), 'f', 0, 64),
	).Result()
	if err != nil {
		return Outcome{}, err
	}

	parts, _ := res.([]any)
	if len(parts) != 3 {
		return Outcome{}, errInvalidLuaResponse
	}
	allowed, _ := parts[0].(int64)
	remaining, _ := parts[1].(int64)
	resetMs, _ := parts[2].(int64)

	return Outcome{
		Allowed:    allowed == 1,
		Limit:      limitPerMinute,
		Remaining:  uint32(remaining),
		ResetAfter: time.Duration(resetMs) * time.Millisecond,
	}, nil
}

// errInvalidLuaResponse signals the Lua script returned an unexpected shape.
var errInvalidLuaResponse = errInvalid("ratelimit: unexpected Lua response shape")

type errInvalid string

func (e errInvalid) Error() string { return string(e) }
