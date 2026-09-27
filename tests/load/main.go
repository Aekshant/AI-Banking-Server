// Command load runs a transfer load test against a running banking-service.
//
// It creates its own test accounts, fires random transfers between them from
// many concurrent workers (re-sending some requests to exercise idempotency),
// then checks that no money was created or lost, and deletes the accounts.
//
//	go run ./load -workers 20 -requests 2000
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"bank-agent-platform/tests/internal/testenv"
)

func main() {
	workers := flag.Int("workers", 20, "concurrent clients")
	requests := flag.Int("requests", 2000, "total transfer requests")
	accounts := flag.Int("accounts", 10, "test accounts to spread transfers across")
	retryRatio := flag.Float64("retry-ratio", 0.1, "fraction of requests that re-send an earlier request")
	flag.Parse()

	if err := run(*workers, *requests, *accounts, *retryRatio); err != nil {
		log.Fatal(err)
	}
}

type sent struct {
	from, to, key string
	amount        int
}

func run(workers, requests, numAccounts int, retryRatio float64) error {
	ctx := context.Background()
	env, err := testenv.New(ctx)
	if err != nil {
		return err
	}
	defer env.Close()

	ids := make([]string, numAccounts)
	for i := range ids {
		c, err := env.CreateCustomer(ctx, testenv.Customer{Balance: "1000000.00"})
		if err != nil {
			return err
		}
		ids[i] = c.ID
	}
	defer func() {
		if err := env.DeleteCustomers(ctx, ids...); err != nil {
			log.Printf("cleanup: %v", err)
		}
	}()
	before, err := total(ctx, env, ids)
	if err != nil {
		return err
	}

	var (
		next      atomic.Int64
		mu        sync.Mutex
		latencies = make([]time.Duration, 0, requests)
		statuses  = map[int]int{}
		history   []sent
		errs      atomic.Int64
		wg        sync.WaitGroup
	)

	start := time.Now()
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				n := next.Add(1)
				if n > int64(requests) {
					return
				}

				mu.Lock()
				var req sent
				if len(history) > 0 && rand.Float64() < retryRatio {
					req = history[rand.IntN(len(history))] // retry an earlier request
				} else {
					from, to := rand.IntN(numAccounts), rand.IntN(numAccounts-1)
					if to >= from {
						to++
					}
					req = sent{ids[from], ids[to], env.Key(fmt.Sprintf("load-%d", n)), 1 + rand.IntN(500)}
					history = append(history, req)
				}
				mu.Unlock()

				t0 := time.Now()
				status, _, err := env.Do(ctx, http.MethodPost, "/api/v1/accounts/transfer", map[string]any{
					"from_account": req.from, "to_account": req.to,
					"amount": req.amount, "idempotency_key": req.key,
				})
				elapsed := time.Since(t0)
				if err != nil {
					errs.Add(1)
				}

				mu.Lock()
				latencies = append(latencies, elapsed)
				statuses[status]++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	duration := time.Since(start)

	after, err := total(ctx, env, ids)
	if err != nil {
		return err
	}

	slices.Sort(latencies)
	pct := func(p float64) time.Duration { return latencies[int(float64(len(latencies)-1)*p)] }

	fmt.Printf("Transfer load test\n")
	fmt.Printf("  target       %s\n", env.BaseURL)
	fmt.Printf("  workers      %d\n", workers)
	fmt.Printf("  requests     %d (%.0f%% retries of earlier requests)\n", requests, retryRatio*100)
	fmt.Printf("  accounts     %d\n", numAccounts)
	fmt.Printf("  duration     %s\n", duration.Round(time.Millisecond))
	fmt.Printf("  throughput   %.0f req/s\n", float64(requests)/duration.Seconds())
	fmt.Printf("  latency      p50 %s  p95 %s  p99 %s  max %s\n",
		pct(0.50).Round(time.Microsecond*100), pct(0.95).Round(time.Microsecond*100),
		pct(0.99).Round(time.Microsecond*100), latencies[len(latencies)-1].Round(time.Microsecond*100))
	fmt.Printf("  statuses     %v\n", statuses)
	fmt.Printf("  errors       %d\n", errs.Load())
	fmt.Printf("  money        %s before, %s after\n", before, after)

	unexpected := requests - statuses[http.StatusCreated] - statuses[http.StatusOK]
	switch {
	case before != after:
		fmt.Println("  result       FAIL: total money changed")
		os.Exit(1)
	case unexpected > 0 || errs.Load() > 0:
		fmt.Printf("  result       FAIL: %d unexpected responses\n", unexpected)
		os.Exit(1)
	default:
		fmt.Println("  result       PASS: every request settled or replayed, total money unchanged")
	}
	return nil
}

func total(ctx context.Context, env *testenv.Env, ids []string) (string, error) {
	var sum string
	err := env.DB.QueryRow(ctx, `SELECT sum(account_balance)::text FROM bank_customers WHERE customer_id = ANY($1)`, ids).Scan(&sum)
	return sum, err
}
