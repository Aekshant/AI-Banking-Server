package integration

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"bank-agent-platform/tests/internal/testenv"
)

// /metrics exposes business metrics that move when transfers happen.
func TestMetricsEndpoint(t *testing.T) {
	a := customer(t, testenv.Customer{Balance: "1000.00"})
	b := customer(t, testenv.Customer{})
	do(t, http.MethodPost, transferPath, transferReq{a.ID, b.ID, 10, env.Key(t.Name())})

	resp, err := env.HTTP.Get(env.BaseURL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	for _, want := range []string{
		`transfers_total{outcome="settled"}`,
		"transfer_amount_rupees_total",
		`http_requests_total{method="POST",route="/api/v1/accounts/transfer",status="201"}`,
		"db_pool_max_connections",
		"banking_service_build_info",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("/metrics missing %s", want)
		}
	}
	// Route templates, not raw paths, so customer ids never become metric labels.
	if strings.Contains(string(body), a.ID) {
		t.Error("/metrics contains a customer id")
	}
}

// Every response carries a request id; a valid incoming one is kept.
func TestRequestIDHeader(t *testing.T) {
	ctx := context.Background()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, env.BaseURL+"/health", nil)
	req.Header.Set("X-Request-ID", "agent-call-42")
	resp, err := env.HTTP.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := resp.Header.Get("X-Request-ID"); got != "agent-call-42" {
		t.Errorf("X-Request-ID = %q, want agent-call-42", got)
	}

	resp, err = env.HTTP.Get(env.BaseURL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get("X-Request-ID") == "" {
		t.Error("no X-Request-ID generated")
	}
}
