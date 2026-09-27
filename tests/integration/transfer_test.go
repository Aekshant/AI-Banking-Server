package integration

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"bank-agent-platform/tests/internal/testenv"
)

const transferPath = "/api/v1/accounts/transfer"

type transferReq struct {
	FromAccount    string `json:"from_account"`
	ToAccount      string `json:"to_account"`
	Amount         any    `json:"amount"`
	IdempotencyKey string `json:"idempotency_key"`
}

type transaction struct {
	TransactionID string  `json:"transaction_id"`
	Status        string  `json:"status"`
	Amount        float64 `json:"amount"`
}

func TestTransferSettles(t *testing.T) {
	a := customer(t, testenv.Customer{Balance: "10000.00"})
	b := customer(t, testenv.Customer{Balance: "500.00"})
	key := env.Key(t.Name())

	status, resp := do(t, http.MethodPost, transferPath, transferReq{a.ID, b.ID, 2500.75, key})
	expectStatus(t, status, http.StatusCreated, resp)

	tx := decode[transaction](t, resp.Data)
	if tx.TransactionID == "" || tx.Status != "SETTLED" || tx.Amount != 2500.75 {
		t.Errorf("unexpected transaction: %+v", tx)
	}
	if got := balance(t, a.ID); got != "7499.25" {
		t.Errorf("source balance = %s, want 7499.25", got)
	}
	if got := balance(t, b.ID); got != "3000.75" {
		t.Errorf("destination balance = %s, want 3000.75", got)
	}
	if n := txCount(t, key); n != 1 {
		t.Errorf("transaction rows = %d, want 1", n)
	}
}

func TestTransferRetryReturnsOriginal(t *testing.T) {
	a := customer(t, testenv.Customer{Balance: "10000.00"})
	b := customer(t, testenv.Customer{Balance: "0.00"})
	req := transferReq{a.ID, b.ID, 5000, env.Key(t.Name())}

	_, first := do(t, http.MethodPost, transferPath, req)
	status, retry := do(t, http.MethodPost, transferPath, req)
	expectStatus(t, status, http.StatusOK, retry)

	if decode[transaction](t, first.Data).TransactionID != decode[transaction](t, retry.Data).TransactionID {
		t.Error("retry returned a different transaction")
	}
	if got := balance(t, a.ID); got != "5000.00" {
		t.Errorf("source balance = %s, want 5000.00 (debited once)", got)
	}
}

func TestTransferKeyReusedForDifferentTransfer(t *testing.T) {
	a := customer(t, testenv.Customer{Balance: "10000.00"})
	b := customer(t, testenv.Customer{})
	key := env.Key(t.Name())

	do(t, http.MethodPost, transferPath, transferReq{a.ID, b.ID, 100, key})
	status, resp := do(t, http.MethodPost, transferPath, transferReq{a.ID, b.ID, 200, key})
	expectStatus(t, status, http.StatusConflict, resp)

	if got := balance(t, a.ID); got != "9900.00" {
		t.Errorf("source balance = %s, want 9900.00", got)
	}
}

func TestTransferInsufficientFundsChangesNothing(t *testing.T) {
	a := customer(t, testenv.Customer{Balance: "100.00"})
	b := customer(t, testenv.Customer{Balance: "0.00"})
	key := env.Key(t.Name())

	status, resp := do(t, http.MethodPost, transferPath, transferReq{a.ID, b.ID, 100.01, key})
	expectStatus(t, status, http.StatusBadRequest, resp)

	if balance(t, a.ID) != "100.00" || balance(t, b.ID) != "0.00" {
		t.Error("balances changed after a failed transfer")
	}
	if n := txCount(t, key); n != 0 {
		t.Errorf("transaction rows = %d, want 0", n)
	}
}

func TestTransferEntireBalance(t *testing.T) {
	a := customer(t, testenv.Customer{Balance: "100.00"})
	b := customer(t, testenv.Customer{Balance: "0.00"})

	status, resp := do(t, http.MethodPost, transferPath, transferReq{a.ID, b.ID, 100, env.Key(t.Name())})
	expectStatus(t, status, http.StatusCreated, resp)
	if got := balance(t, a.ID); got != "0.00" {
		t.Errorf("source balance = %s, want 0.00", got)
	}
}

func TestTransferUnknownAccount(t *testing.T) {
	a := customer(t, testenv.Customer{})

	status, resp := do(t, http.MethodPost, transferPath, transferReq{a.ID, "itest-missing", 10, env.Key(t.Name())})
	expectStatus(t, status, http.StatusNotFound, resp)
}

func TestTransferValidation(t *testing.T) {
	a := customer(t, testenv.Customer{})
	b := customer(t, testenv.Customer{})

	tests := map[string]any{
		"same account":    transferReq{a.ID, a.ID, 10, env.Key("v1")},
		"zero amount":     transferReq{a.ID, b.ID, 0, env.Key("v2")},
		"negative amount": transferReq{a.ID, b.ID, -10, env.Key("v3")},
		"three decimals":  transferReq{a.ID, b.ID, 1.234, env.Key("v4")},
		"too large":       transferReq{a.ID, b.ID, 1e14, env.Key("v5")},
		"missing key":     transferReq{a.ID, b.ID, 10, ""},
		"amount as text":  transferReq{a.ID, b.ID, "ten", env.Key("v6")},
		"malformed json":  `{"from_account": `,
		"empty body":      `{}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			status, resp := do(t, http.MethodPost, transferPath, body)
			expectStatus(t, status, http.StatusBadRequest, resp)
		})
	}
	if balance(t, a.ID) != "100000.00" {
		t.Error("a rejected request changed a balance")
	}
}

// Many identical requests at once must move money exactly once.
func TestTransferConcurrentRetries(t *testing.T) {
	a := customer(t, testenv.Customer{Balance: "1000.00"})
	b := customer(t, testenv.Customer{Balance: "0.00"})
	req := transferReq{a.ID, b.ID, 100, env.Key(t.Name())}

	statuses := parallel(t, 20, func(int) any { return req })

	if statuses[http.StatusCreated] != 1 || statuses[http.StatusOK] != 19 {
		t.Errorf("statuses = %v, want one 201 and nineteen 200", statuses)
	}
	if n := txCount(t, req.IdempotencyKey); n != 1 {
		t.Errorf("transaction rows = %d, want 1", n)
	}
	if got := balance(t, a.ID); got != "900.00" {
		t.Errorf("source balance = %s, want 900.00", got)
	}
}

// Opposite-direction transfers at once must not deadlock or lose money.
func TestTransferConcurrentBothDirections(t *testing.T) {
	a := customer(t, testenv.Customer{Balance: "100000.00"})
	b := customer(t, testenv.Customer{Balance: "100000.00"})

	statuses := parallel(t, 100, func(i int) any {
		if i%2 == 0 {
			return transferReq{a.ID, b.ID, 10, env.Key(fmt.Sprintf("%s-ab-%d", t.Name(), i))}
		}
		return transferReq{b.ID, a.ID, 7, env.Key(fmt.Sprintf("%s-ba-%d", t.Name(), i))}
	})

	if statuses[http.StatusCreated] != 100 {
		t.Errorf("statuses = %v, want 100 x 201", statuses)
	}
	// 50 x -10 + 50 x +7 = -150 for a, +150 for b.
	if got := balance(t, a.ID); got != "99850.00" {
		t.Errorf("a balance = %s, want 99850.00", got)
	}
	if got := balance(t, b.ID); got != "100150.00" {
		t.Errorf("b balance = %s, want 100150.00", got)
	}
}

// Two debits racing for the same money: only as many as the balance allows succeed.
func TestTransferConcurrentNoOverdraft(t *testing.T) {
	a := customer(t, testenv.Customer{Balance: "500.00"})
	b := customer(t, testenv.Customer{Balance: "0.00"})

	statuses := parallel(t, 20, func(i int) any {
		return transferReq{a.ID, b.ID, 100, env.Key(fmt.Sprintf("%s-%d", t.Name(), i))}
	})

	if statuses[http.StatusCreated] != 5 || statuses[http.StatusBadRequest] != 15 {
		t.Errorf("statuses = %v, want five 201 and fifteen 400", statuses)
	}
	if got := balance(t, a.ID); got != "0.00" {
		t.Errorf("source balance = %s, want 0.00", got)
	}
	if got := balance(t, b.ID); got != "500.00" {
		t.Errorf("destination balance = %s, want 500.00", got)
	}
}

// parallel sends n transfer requests at once and counts response statuses.
func parallel(t *testing.T, n int, body func(i int) any) map[int]int {
	t.Helper()
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		statuses = map[int]int{}
		start    = make(chan struct{})
	)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			status, _, err := env.Do(context.Background(), http.MethodPost, transferPath, body(i))
			if err != nil {
				t.Errorf("request %d: %v", i, err)
			}
			mu.Lock()
			statuses[status]++
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()
	return statuses
}
