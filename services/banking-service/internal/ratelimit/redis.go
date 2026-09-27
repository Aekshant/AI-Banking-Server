package ratelimit

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// tokenBucket refills the bucket for the time elapsed since it was last
// used, then tries to take one token. Redis's own clock is used so every
// service instance agrees on "now".
//
// KEYS[1] bucket key; ARGV[1] capacity; ARGV[2] refill rate (tokens/second)
// Returns {allowed (0|1), tokens left, seconds until next token}.
var tokenBucket = redis.NewScript(`
local capacity = tonumber(ARGV[1])
local rate     = tonumber(ARGV[2])

local t   = redis.call('TIME')
local now = tonumber(t[1]) + tonumber(t[2]) / 1000000

local state  = redis.call('HMGET', KEYS[1], 'tokens', 'ts')
local tokens = tonumber(state[1]) or capacity
local ts     = tonumber(state[2]) or now

tokens = math.min(capacity, tokens + math.max(0, now - ts) * rate)

local allowed, retry_after = 0, 0
if tokens >= 1 then
  tokens  = tokens - 1
  allowed = 1
else
  retry_after = (1 - tokens) / rate
end

redis.call('HSET', KEYS[1], 'tokens', tokens, 'ts', now)
-- An idle bucket refills completely after capacity/rate seconds; drop it then.
redis.call('PEXPIRE', KEYS[1], math.ceil(capacity / rate * 1000) + 1000)

-- Lua numbers become integers in Redis replies, so return fractions as strings.
return {allowed, tostring(tokens), tostring(retry_after)}
`)

type RedisLimiter struct {
	rdb    *redis.Client
	prefix string
}

func NewRedisLimiter(rdb *redis.Client) *RedisLimiter {
	return &RedisLimiter{rdb: rdb, prefix: "ratelimit:"}
}

func (l *RedisLimiter) Allow(ctx context.Context, key string, rule Rule) (Result, error) {
	reply, err := tokenBucket.Run(ctx, l.rdb, []string{l.prefix + key}, rule.Capacity, rule.RefillPerSecond).Slice()
	if err != nil {
		return Result{}, err
	}
	if len(reply) != 3 {
		return Result{}, fmt.Errorf("unexpected token bucket reply: %v", reply)
	}

	allowed, _ := reply[0].(int64)
	tokens, err := parseFloat(reply[1])
	if err != nil {
		return Result{}, err
	}
	retryAfter, err := parseFloat(reply[2])
	if err != nil {
		return Result{}, err
	}

	return Result{
		Allowed:    allowed == 1,
		Remaining:  int(math.Floor(tokens)),
		RetryAfter: time.Duration(retryAfter * float64(time.Second)),
	}, nil
}

func parseFloat(v any) (float64, error) {
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("unexpected token bucket value %v", v)
	}
	return strconv.ParseFloat(s, 64)
}
