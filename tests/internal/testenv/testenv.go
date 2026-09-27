// Package testenv gives integration and load tests access to a running
// banking-service (over HTTP) and its database (for setup and assertions).
//
// Every test creates its own customers with unique ids ("itest-<run>-NNN")
// and deletes them afterwards, so tests never touch seeded data.
package testenv

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	mrand "math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

type Env struct {
	BaseURL string
	DB      *pgxpool.Pool
	HTTP    *http.Client
	RunID   string

	seq atomic.Int64
}

// Envelope is the standard API response shape.
type Envelope struct {
	Status  int             `json:"status"`
	Message string          `json:"message"`
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
}

// New loads .env, connects to Postgres and checks the service is reachable.
// BASE_URL defaults to http://localhost:$PORT.
func New(ctx context.Context) (*Env, error) {
	if path, ok := findDotEnv(); ok {
		_ = godotenv.Load(path) // never overrides variables already set
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8001"
		}
		baseURL = "http://localhost:" + port
	}

	dbURL, err := databaseURL()
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres unreachable (is `docker compose up -d` running?): %w", err)
	}

	e := &Env{BaseURL: baseURL, DB: pool, HTTP: &http.Client{Timeout: 10 * time.Second}, RunID: randomHex(4)}
	if status, _, err := e.Do(ctx, http.MethodGet, "/health", nil); err != nil || status != http.StatusOK {
		pool.Close()
		return nil, fmt.Errorf("banking-service not healthy at %s (status %d, err %v); run tests via `make test-integration`", baseURL, status, err)
	}
	return e, nil
}

func (e *Env) Close() { e.DB.Close() }

// Key returns an idempotency key unique to this test run.
func (e *Env) Key(name string) string {
	return fmt.Sprintf("itest-%s-%s", e.RunID, name)
}

type Customer struct {
	ID          string
	FullName    string
	PAN         string
	Aadhaar     string
	Balance     string
	CreditScore int
	Verified    bool
}

// CreateCustomer inserts a test customer. Zero values get sensible defaults.
func (e *Env) CreateCustomer(ctx context.Context, c Customer) (Customer, error) {
	c.ID = fmt.Sprintf("itest-%s-%03d", e.RunID, e.seq.Add(1))
	if c.FullName == "" {
		c.FullName = "Test Customer"
	}
	if c.Balance == "" {
		c.Balance = "100000.00"
	}
	if c.CreditScore == 0 {
		c.CreditScore = 750
	}
	c.PAN = randomFrom("ABCDEFGHIJKLMNOPQRSTUVWXYZ", 5) + randomFrom("0123456789", 4) + randomFrom("ABCDEFGHIJKLMNOPQRSTUVWXYZ", 1)
	c.Aadhaar = "9" + randomFrom("0123456789", 11)

	_, err := e.DB.Exec(ctx, `
		INSERT INTO bank_customers (customer_id, full_name, pan_number, aadhaar_number,
		                            account_balance, credit_score, is_2fa_verified)
		VALUES ($1, $2, $3, $4, $5::numeric, $6, $7)`,
		c.ID, c.FullName, c.PAN, c.Aadhaar, c.Balance, c.CreditScore, c.Verified)
	return c, err
}

// DeleteCustomers removes test customers and every transaction touching them.
func (e *Env) DeleteCustomers(ctx context.Context, ids ...string) error {
	if _, err := e.DB.Exec(ctx, `DELETE FROM transactions WHERE from_account = ANY($1) OR to_account = ANY($1)`, ids); err != nil {
		return err
	}
	_, err := e.DB.Exec(ctx, `DELETE FROM bank_customers WHERE customer_id = ANY($1)`, ids)
	return err
}

// Balance returns an account balance as text, e.g. "100000.00".
func (e *Env) Balance(ctx context.Context, id string) (string, error) {
	var b string
	err := e.DB.QueryRow(ctx, `SELECT account_balance::text FROM bank_customers WHERE customer_id = $1`, id).Scan(&b)
	return b, err
}

// CountTransactions returns how many rows exist for an idempotency key.
func (e *Env) CountTransactions(ctx context.Context, key string) (int, error) {
	var n int
	err := e.DB.QueryRow(ctx, `SELECT count(*) FROM transactions WHERE idempotency_key = $1`, key).Scan(&n)
	return n, err
}

// Do sends a request with an optional JSON body (a string body is sent raw).
// Each call comes from a random client IP, so tests never share a
// rate-limit bucket. Use DoFrom to control the client IP.
func (e *Env) Do(ctx context.Context, method, path string, body any) (int, Envelope, error) {
	status, env, _, err := e.DoFrom(ctx, RandomIP(), method, path, body)
	return status, env, err
}

// DoFrom sends a request as clientIP (via X-Forwarded-For, which the test
// server trusts from localhost) and also returns the response headers.
func (e *Env) DoFrom(ctx context.Context, clientIP, method, path string, body any) (int, Envelope, http.Header, error) {
	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(b)
	default:
		buf, err := json.Marshal(b)
		if err != nil {
			return 0, Envelope{}, nil, err
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, e.BaseURL+path, reader)
	if err != nil {
		return 0, Envelope{}, nil, err
	}
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Forwarded-For", clientIP)

	resp, err := e.HTTP.Do(req)
	if err != nil {
		return 0, Envelope{}, nil, err
	}
	defer resp.Body.Close()

	var env Envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return resp.StatusCode, Envelope{}, resp.Header, fmt.Errorf("decode response: %w", err)
	}
	if env.Status != resp.StatusCode {
		return resp.StatusCode, env, resp.Header, fmt.Errorf("envelope status %d != HTTP status %d", env.Status, resp.StatusCode)
	}
	return resp.StatusCode, env, resp.Header, nil
}

// RandomIP returns a random address in 10.0.0.0/8 to act as a client IP.
func RandomIP() string {
	return fmt.Sprintf("10.%d.%d.%d", mrand.IntN(256), mrand.IntN(256), 1+mrand.IntN(254))
}

func databaseURL() (string, error) {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v, nil
	}
	user, pw, db := os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD"), os.Getenv("POSTGRES_DB")
	if user == "" || pw == "" || db == "" {
		return "", errors.New("set POSTGRES_USER, POSTGRES_PASSWORD and POSTGRES_DB in .env, or DATABASE_URL")
	}
	host, port := os.Getenv("POSTGRES_HOST"), os.Getenv("POSTGRES_PORT")
	if host == "" {
		host = "localhost"
	}
	if port == "" {
		port = "5432"
	}
	return fmt.Sprintf("postgres://%s@%s:%s/%s?sslmode=disable",
		url.UserPassword(user, pw).String(), host, port, db), nil
}

func findDotEnv() (string, bool) {
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

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func randomFrom(alphabet string, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[mrand.IntN(len(alphabet))]
	}
	return string(b)
}
