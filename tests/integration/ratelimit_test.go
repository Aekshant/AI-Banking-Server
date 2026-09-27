package integration

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"bank-agent-platform/tests/internal/testenv"
)

// A single client that bursts past the bucket capacity gets 429s with a
// Retry-After header, while other clients are unaffected.
func TestRateLimitBurst(t *testing.T) {
	c := customer(t, testenv.Customer{})
	path := "/api/v1/accounts/" + c.ID + "/profile"
	ip := testenv.RandomIP()
	ctx := context.Background()

	status, _, header, err := env.DoFrom(ctx, ip, http.MethodGet, path, nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("first request: %d %v", status, err)
	}
	limit, err := strconv.Atoi(header.Get("X-RateLimit-Limit"))
	if err != nil || limit < 1 {
		t.Fatalf("X-RateLimit-Limit = %q (is rate limiting enabled?)", header.Get("X-RateLimit-Limit"))
	}
	if got := header.Get("X-RateLimit-Remaining"); got != strconv.Itoa(limit-1) {
		t.Errorf("X-RateLimit-Remaining = %s, want %d", got, limit-1)
	}

	// Use up the rest of the burst; the bucket refills a little meanwhile,
	// so keep going until the first rejection.
	var rejected testenv.Envelope
	var rejectedHeader http.Header
	for i := 0; i < limit*3; i++ {
		status, resp, h, err := env.DoFrom(ctx, ip, http.MethodGet, path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if status == http.StatusTooManyRequests {
			rejected, rejectedHeader = resp, h
			break
		}
	}
	if rejectedHeader == nil {
		t.Fatalf("no 429 after %d requests from one client", limit*3)
	}

	expectStatus(t, rejected.Status, http.StatusTooManyRequests, rejected)
	if ra, err := strconv.Atoi(rejectedHeader.Get("Retry-After")); err != nil || ra < 1 {
		t.Errorf("Retry-After = %q, want a positive number of seconds", rejectedHeader.Get("Retry-After"))
	}
	if rejectedHeader.Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("X-RateLimit-Remaining = %q, want 0", rejectedHeader.Get("X-RateLimit-Remaining"))
	}

	// A different client still has a full bucket.
	status, _, _, err = env.DoFrom(ctx, testenv.RandomIP(), http.MethodGet, path, nil)
	if err != nil || status != http.StatusOK {
		t.Errorf("other client: status %d, %v; want 200", status, err)
	}
}

// Each endpoint group has its own bucket: exhausting reads must not block transfers.
func TestRateLimitBucketsArePerEndpointGroup(t *testing.T) {
	a := customer(t, testenv.Customer{})
	b := customer(t, testenv.Customer{})
	ip := testenv.RandomIP()
	ctx := context.Background()

	for i := 0; i < 200; i++ {
		status, _, _, err := env.DoFrom(ctx, ip, http.MethodGet, "/api/v1/accounts/"+a.ID+"/profile", nil)
		if err != nil {
			t.Fatal(err)
		}
		if status == http.StatusTooManyRequests {
			break
		}
	}

	status, resp, _, err := env.DoFrom(ctx, ip, http.MethodPost, transferPath, transferReq{a.ID, b.ID, 1, env.Key(t.Name())})
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, status, http.StatusCreated, resp)
}
