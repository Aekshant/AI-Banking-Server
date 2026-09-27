package config

import (
	"strings"
	"testing"
)

func clearDBEnv(t *testing.T) {
	for _, k := range []string{"DATABASE_URL", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB",
		"POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_SSLMODE"} {
		t.Setenv(k, "")
	}
}

func TestDatabaseURLFromParts(t *testing.T) {
	clearDBEnv(t)
	t.Setenv("POSTGRES_USER", "bank")
	t.Setenv("POSTGRES_PASSWORD", "p@ss:w/rd")
	t.Setenv("POSTGRES_DB", "bankdb")
	t.Setenv("POSTGRES_HOST", "db.internal")

	got, err := DatabaseURL()
	if err != nil {
		t.Fatal(err)
	}
	want := "postgres://bank:p%40ss%3Aw%2Frd@db.internal:5432/bankdb?sslmode=disable"
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestDatabaseURLOverride(t *testing.T) {
	clearDBEnv(t)
	t.Setenv("DATABASE_URL", "postgres://x:y@h:1/d")

	got, err := DatabaseURL()
	if err != nil || got != "postgres://x:y@h:1/d" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestDatabaseURLMissing(t *testing.T) {
	clearDBEnv(t)
	t.Setenv("POSTGRES_USER", "bank")

	_, err := DatabaseURL()
	if err == nil || !strings.Contains(err.Error(), "POSTGRES_PASSWORD, POSTGRES_DB") {
		t.Errorf("expected missing-variable error, got %v", err)
	}
}

func TestRateLimitDefaults(t *testing.T) {
	for _, k := range []string{"RATE_LIMIT_READ", "RATE_LIMIT_TRANSFER", "RATE_LIMIT_LOAN"} {
		t.Setenv(k, "")
	}
	rl, err := loadRateLimits()
	if err != nil {
		t.Fatal(err)
	}
	if rl.Transfer.Capacity != 10 || rl.Transfer.RefillPerSecond != 1 || rl.Read.Capacity != 30 || rl.Loan.RefillPerSecond != 2 {
		t.Errorf("unexpected defaults: %+v", rl)
	}
}

func TestRateLimitInvalid(t *testing.T) {
	t.Setenv("RATE_LIMIT_TRANSFER", "ten")
	if _, err := loadRateLimits(); err == nil || !strings.Contains(err.Error(), "RATE_LIMIT_TRANSFER") {
		t.Errorf("expected error naming RATE_LIMIT_TRANSFER, got %v", err)
	}
}
