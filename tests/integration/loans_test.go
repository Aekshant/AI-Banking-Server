package integration

import (
	"net/http"
	"testing"

	"bank-agent-platform/tests/internal/testenv"
)

const loansPath = "/api/v1/loans/evaluate"

type loanResult struct {
	CustomerID        string  `json:"customer_id"`
	Approved          bool    `json:"approved"`
	CreditScore       int     `json:"credit_score"`
	Reason            string  `json:"reason"`
	MaxEligibleAmount float64 `json:"max_eligible_amount"`
}

func TestLoanEvaluation(t *testing.T) {
	tests := []struct {
		name     string
		customer testenv.Customer
		amount   any
		approved bool
		reason   string
		max      float64
	}{
		{"700-749 gets 5x balance", testenv.Customer{Balance: "100000.00", CreditScore: 742, Verified: true}, 500000, true, "ELIGIBLE", 500000},
		{"one paisa over the limit", testenv.Customer{Balance: "100000.00", CreditScore: 742, Verified: true}, 500000.01, false, "AMOUNT_EXCEEDS_LIMIT", 500000},
		{"750+ gets 10x balance", testenv.Customer{Balance: "100000.00", CreditScore: 800, Verified: true}, 1000000, true, "ELIGIBLE", 1000000},
		{"650-699 gets 2x balance", testenv.Customer{Balance: "100000.00", CreditScore: 650, Verified: true}, 200001, false, "AMOUNT_EXCEEDS_LIMIT", 200000},
		{"capped at 1 crore", testenv.Customer{Balance: "5000000.00", CreditScore: 900, Verified: true}, 10000001, false, "AMOUNT_EXCEEDS_LIMIT", 10000000},
		{"low credit score", testenv.Customer{Balance: "5000000.00", CreditScore: 649, Verified: true}, 1000, false, "LOW_CREDIT_SCORE", 0},
		{"2FA not verified", testenv.Customer{Balance: "5000000.00", CreditScore: 800, Verified: false}, 1000, false, "IDENTITY_NOT_VERIFIED", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := customer(t, tt.customer)

			status, resp := do(t, http.MethodPost, loansPath, map[string]any{"customer_id": c.ID, "loan_amount": tt.amount})
			expectStatus(t, status, http.StatusOK, resp)

			got := decode[loanResult](t, resp.Data)
			if got.CustomerID != c.ID || got.Approved != tt.approved || got.Reason != tt.reason ||
				got.CreditScore != tt.customer.CreditScore || got.MaxEligibleAmount != tt.max {
				t.Errorf("got %+v, want approved=%v reason=%s max=%v", got, tt.approved, tt.reason, tt.max)
			}
		})
	}
}

func TestLoanUnknownCustomer(t *testing.T) {
	status, resp := do(t, http.MethodPost, loansPath, map[string]any{"customer_id": "itest-missing", "loan_amount": 1000})
	expectStatus(t, status, http.StatusNotFound, resp)
}

func TestLoanValidation(t *testing.T) {
	c := customer(t, testenv.Customer{})

	tests := map[string]any{
		"zero amount":      map[string]any{"customer_id": c.ID, "loan_amount": 0},
		"negative amount":  map[string]any{"customer_id": c.ID, "loan_amount": -1},
		"missing amount":   map[string]any{"customer_id": c.ID},
		"missing customer": map[string]any{"loan_amount": 1000},
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			status, resp := do(t, http.MethodPost, loansPath, body)
			expectStatus(t, status, http.StatusBadRequest, resp)
		})
	}
}
