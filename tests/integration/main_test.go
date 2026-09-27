// Package integration tests banking-service end to end: real HTTP requests
// against a running service, backed by a real Postgres.
//
// Run with `make test-integration`, which starts the service for you.
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"bank-agent-platform/tests/internal/testenv"
)

var env *testenv.Env

func TestMain(m *testing.M) {
	var err error
	env, err = testenv.New(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "integration setup failed:", err)
		os.Exit(1)
	}
	code := m.Run()
	env.Close()
	os.Exit(code)
}

// customer creates a test customer that is deleted when the test ends.
func customer(t *testing.T, c testenv.Customer) testenv.Customer {
	t.Helper()
	created, err := env.CreateCustomer(context.Background(), c)
	if err != nil {
		t.Fatalf("create customer: %v", err)
	}
	t.Cleanup(func() {
		if err := env.DeleteCustomers(context.Background(), created.ID); err != nil {
			t.Errorf("cleanup %s: %v", created.ID, err)
		}
	})
	return created
}

func do(t *testing.T, method, path string, body any) (int, testenv.Envelope) {
	t.Helper()
	status, resp, err := env.Do(context.Background(), method, path, body)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return status, resp
}

func balance(t *testing.T, id string) string {
	t.Helper()
	b, err := env.Balance(context.Background(), id)
	if err != nil {
		t.Fatalf("balance %s: %v", id, err)
	}
	return b
}

func txCount(t *testing.T, key string) int {
	t.Helper()
	n, err := env.CountTransactions(context.Background(), key)
	if err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	return n
}

func decode[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode data %s: %v", raw, err)
	}
	return v
}

func expectStatus(t *testing.T, got, want int, resp testenv.Envelope) {
	t.Helper()
	if got != want {
		t.Fatalf("status = %d, want %d (message %q)", got, want, resp.Message)
	}
	if resp.Success != (want < 400) {
		t.Fatalf("success = %v for status %d", resp.Success, want)
	}
}
