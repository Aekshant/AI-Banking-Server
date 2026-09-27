// Package config loads settings from the environment. Values in a .env file
// (found by searching the working directory and its parents) are loaded
// first, but never override variables that are already set.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"

	"banking-service/internal/ratelimit"
)

type Config struct {
	Port        string
	DatabaseURL string

	LogLevel  string // debug, info, warn, error
	LogFormat string // json, text

	RedisAddr     string
	RedisPassword string

	// TrustedProxies lists proxy IPs/CIDRs allowed to set X-Forwarded-For.
	// Empty means the client IP is always the TCP peer address, so clients
	// cannot spoof their IP to dodge rate limits.
	TrustedProxies []string

	RateLimitEnabled bool
	RateLimits       RateLimits
}

// RateLimits holds one token-bucket rule per group of endpoints.
type RateLimits struct {
	Read     ratelimit.Rule // GET account profile
	Transfer ratelimit.Rule // POST transfer
	Loan     ratelimit.Rule // POST loan evaluation
}

// Load reads configuration. DATABASE_URL takes precedence; otherwise the URL
// is built from POSTGRES_USER, POSTGRES_PASSWORD and POSTGRES_DB (required)
// plus POSTGRES_HOST, POSTGRES_PORT and POSTGRES_SSLMODE (optional).
func Load() (Config, error) {
	if path, ok := FindDotEnv(); ok {
		if err := godotenv.Load(path); err != nil {
			return Config{}, fmt.Errorf("load %s: %w", path, err)
		}
	}

	cfg := Config{
		Port:          getEnv("PORT", "8001"),
		LogLevel:      getEnv("LOG_LEVEL", "info"),
		LogFormat:     getEnv("LOG_FORMAT", "json"),
		RedisAddr:     net.JoinHostPort(getEnv("REDIS_HOST", "localhost"), getEnv("REDIS_PORT", "6379")),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
	}

	dbURL, err := DatabaseURL()
	if err != nil {
		return Config{}, err
	}
	cfg.DatabaseURL = dbURL

	for _, p := range strings.Split(os.Getenv("TRUSTED_PROXIES"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			cfg.TrustedProxies = append(cfg.TrustedProxies, p)
		}
	}

	cfg.RateLimitEnabled, err = strconv.ParseBool(getEnv("RATE_LIMIT_ENABLED", "true"))
	if err != nil {
		return Config{}, fmt.Errorf("RATE_LIMIT_ENABLED must be true or false: %w", err)
	}
	cfg.RateLimits, err = loadRateLimits()
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// loadRateLimits reads RATE_LIMIT_<GROUP>="capacity/refill_per_second".
func loadRateLimits() (RateLimits, error) {
	var rl RateLimits
	for _, r := range []struct {
		rule          *ratelimit.Rule
		name, env, df string
	}{
		{&rl.Read, "read", "RATE_LIMIT_READ", "30/10"},
		{&rl.Transfer, "transfer", "RATE_LIMIT_TRANSFER", "10/1"},
		{&rl.Loan, "loan", "RATE_LIMIT_LOAN", "10/2"},
	} {
		rule, err := ratelimit.ParseRule(r.name, getEnv(r.env, r.df))
		if err != nil {
			return RateLimits{}, fmt.Errorf("%s: %w", r.env, err)
		}
		*r.rule = rule
	}
	return rl, nil
}

// DatabaseURL returns DATABASE_URL, or builds one from the POSTGRES_* variables.
func DatabaseURL() (string, error) {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v, nil
	}

	var missing []string
	required := func(key string) string {
		v := os.Getenv(key)
		if v == "" {
			missing = append(missing, key)
		}
		return v
	}
	user := required("POSTGRES_USER")
	password := required("POSTGRES_PASSWORD")
	dbName := required("POSTGRES_DB")
	if len(missing) > 0 {
		return "", errors.New("missing required environment variables: " + strings.Join(missing, ", ") +
			" (copy .env.example to .env at the repo root, or set DATABASE_URL)")
	}

	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, password), // escapes special characters
		Host:     net.JoinHostPort(getEnv("POSTGRES_HOST", "localhost"), getEnv("POSTGRES_PORT", "5432")),
		Path:     "/" + dbName,
		RawQuery: "sslmode=" + url.QueryEscape(getEnv("POSTGRES_SSLMODE", "disable")),
	}
	return u.String(), nil
}

// FindDotEnv returns the nearest .env file in the working directory or any parent.
func FindDotEnv() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		path := filepath.Join(dir, ".env")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
