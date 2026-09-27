package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newLimiter runs the real Lua script against an in-memory Redis whose clock
// the test controls.
func newLimiter(t *testing.T) (*RedisLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	mr.SetTime(time.Unix(1_700_000_000, 0))
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return NewRedisLimiter(rdb), mr
}

func allow(t *testing.T, l *RedisLimiter, key string, rule Rule) Result {
	t.Helper()
	res, err := l.Allow(context.Background(), key, rule)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestBurstUpToCapacityThenReject(t *testing.T) {
	l, _ := newLimiter(t)
	rule := Rule{Name: "test", Capacity: 3, RefillPerSecond: 1}

	for i, wantRemaining := range []int{2, 1, 0} {
		res := allow(t, l, "client", rule)
		if !res.Allowed || res.Remaining != wantRemaining {
			t.Fatalf("request %d: got %+v, want allowed with %d remaining", i+1, res, wantRemaining)
		}
	}

	res := allow(t, l, "client", rule)
	if res.Allowed {
		t.Fatal("request over capacity was allowed")
	}
	if res.RetryAfter != time.Second {
		t.Errorf("retry after = %s, want 1s", res.RetryAfter)
	}
}

func TestRefillOverTime(t *testing.T) {
	l, mr := newLimiter(t)
	rule := Rule{Name: "test", Capacity: 2, RefillPerSecond: 2}

	allow(t, l, "client", rule)
	allow(t, l, "client", rule)
	if allow(t, l, "client", rule).Allowed {
		t.Fatal("empty bucket allowed a request")
	}

	mr.SetTime(time.Unix(1_700_000_000, 500_000_000)) // +0.5s refills 1 token at 2/s
	if !allow(t, l, "client", rule).Allowed {
		t.Fatal("request after refill was rejected")
	}
	if allow(t, l, "client", rule).Allowed {
		t.Fatal("only one token should have refilled")
	}
}

func TestRefillNeverExceedsCapacity(t *testing.T) {
	l, mr := newLimiter(t)
	rule := Rule{Name: "test", Capacity: 3, RefillPerSecond: 100}

	allow(t, l, "client", rule)
	mr.SetTime(time.Unix(1_700_000_000+60, 0)) // idle for a minute

	for i := range 3 {
		if !allow(t, l, "client", rule).Allowed {
			t.Fatalf("request %d rejected", i+1)
		}
	}
	if allow(t, l, "client", rule).Allowed {
		t.Fatal("bucket refilled beyond capacity")
	}
}

func TestSeparateKeysHaveSeparateBuckets(t *testing.T) {
	l, _ := newLimiter(t)
	rule := Rule{Name: "test", Capacity: 1, RefillPerSecond: 1}

	if !allow(t, l, "alice", rule).Allowed || !allow(t, l, "bob", rule).Allowed {
		t.Fatal("each client should get its own full bucket")
	}
	if allow(t, l, "alice", rule).Allowed {
		t.Fatal("alice's bucket should be empty")
	}
}

func TestIdleBucketExpires(t *testing.T) {
	l, mr := newLimiter(t)
	allow(t, l, "client", Rule{Name: "test", Capacity: 10, RefillPerSecond: 5})

	// Full refill takes 10/5 = 2s; the key expires 1s after that.
	if ttl := mr.TTL("ratelimit:client"); ttl != 3*time.Second {
		t.Errorf("ttl = %s, want 3s", ttl)
	}
}

func TestParseRule(t *testing.T) {
	r, err := ParseRule("transfer", "10/0.5")
	if err != nil || r != (Rule{Name: "transfer", Capacity: 10, RefillPerSecond: 0.5}) {
		t.Errorf("got %+v, %v", r, err)
	}

	for _, bad := range []string{"10", "0/1", "-1/1", "10/0", "10/-1", "a/1", "10/b", ""} {
		if _, err := ParseRule("x", bad); err == nil {
			t.Errorf("ParseRule(%q) should fail", bad)
		}
	}
}
