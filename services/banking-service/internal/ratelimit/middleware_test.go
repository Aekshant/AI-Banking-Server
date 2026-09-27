package ratelimit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type fakeLimiter struct {
	result  Result
	err     error
	lastKey string
}

func (f *fakeLimiter) Allow(_ context.Context, key string, _ Rule) (Result, error) {
	f.lastKey = key
	return f.result, f.err
}

func serve(t *testing.T, l Limiter) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	reached := false
	r.GET("/", Middleware(l, Rule{Name: "read", Capacity: 30, RefillPerSecond: 10}), func(c *gin.Context) {
		reached = true
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.7:5555"
	r.ServeHTTP(w, req)
	return w, reached
}

func TestMiddlewareAllows(t *testing.T) {
	l := &fakeLimiter{result: Result{Allowed: true, Remaining: 29}}
	w, reached := serve(t, l)

	if !reached || w.Code != http.StatusOK {
		t.Fatalf("request not passed through: %d", w.Code)
	}
	if w.Header().Get("X-RateLimit-Limit") != "30" || w.Header().Get("X-RateLimit-Remaining") != "29" {
		t.Errorf("rate limit headers = %v", w.Header())
	}
	if l.lastKey != "read:203.0.113.7" {
		t.Errorf("bucket key = %q, want read:203.0.113.7", l.lastKey)
	}
}

func TestMiddlewareRejectsWith429(t *testing.T) {
	l := &fakeLimiter{result: Result{Allowed: false, RetryAfter: 1500 * time.Millisecond}}
	w, reached := serve(t, l)

	if reached {
		t.Fatal("rejected request reached the handler")
	}
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "2" {
		t.Errorf("Retry-After = %q, want 2 (rounded up)", got)
	}

	var body struct {
		Status  int  `json:"status"`
		Success bool `json:"success"`
		Data    struct {
			RetryAfterSeconds int `json:"retry_after_seconds"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != 429 || body.Success || body.Data.RetryAfterSeconds != 2 {
		t.Errorf("unexpected body: %s", w.Body)
	}
}

func TestMiddlewareFailsOpen(t *testing.T) {
	l := &fakeLimiter{err: errors.New("redis: connection refused")}
	w, reached := serve(t, l)

	if !reached || w.Code != http.StatusOK {
		t.Fatalf("limiter error should allow the request, got %d", w.Code)
	}
}
