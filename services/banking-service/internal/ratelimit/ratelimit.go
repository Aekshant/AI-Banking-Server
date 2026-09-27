// Package ratelimit implements token-bucket rate limiting backed by Redis.
//
// Each client gets one bucket per rule. A bucket holds up to Capacity tokens
// and refills at RefillPerSecond. Every request takes one token; when the
// bucket is empty the request is rejected with 429 until a token refills.
// This allows short bursts (up to Capacity) while capping the sustained rate.
//
// Bucket state lives in Redis and is updated by a single Lua script, so the
// check-and-take is atomic across concurrent requests and across multiple
// instances of the service.
package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Rule is a named bucket configuration.
type Rule struct {
	Name            string
	Capacity        int
	RefillPerSecond float64
}

// ParseRule reads "capacity/refill_per_second", e.g. "10/1" for bursts of 10
// refilling at 1 token per second.
func ParseRule(name, spec string) (Rule, error) {
	capStr, rateStr, ok := strings.Cut(spec, "/")
	if !ok {
		return Rule{}, fmt.Errorf("rate limit %s: %q must be capacity/refill_per_second, e.g. 10/1", name, spec)
	}
	capacity, err := strconv.Atoi(strings.TrimSpace(capStr))
	if err != nil || capacity < 1 {
		return Rule{}, fmt.Errorf("rate limit %s: capacity %q must be a positive integer", name, capStr)
	}
	rate, err := strconv.ParseFloat(strings.TrimSpace(rateStr), 64)
	if err != nil || rate <= 0 {
		return Rule{}, fmt.Errorf("rate limit %s: refill rate %q must be a positive number", name, rateStr)
	}
	return Rule{Name: name, Capacity: capacity, RefillPerSecond: rate}, nil
}

type Result struct {
	Allowed    bool
	Remaining  int           // whole tokens left after this request
	RetryAfter time.Duration // when the next token is available; 0 if allowed
}

// Limiter takes one token from the bucket identified by key.
type Limiter interface {
	Allow(ctx context.Context, key string, rule Rule) (Result, error)
}
