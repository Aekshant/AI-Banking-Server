package loans

import (
	"math/big"
	"testing"
)

func rat(s string) *big.Rat {
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		panic(s)
	}
	return r
}

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name     string
		score    int
		verified bool
		balance  string
		amount   string
		approved bool
		reason   string
		max      string
	}{
		{"score 700-749 gets 5x balance", 742, true, "108145.55", "500000", true, ReasonEligible, "540727.75"},
		{"exactly at limit is approved", 742, true, "100000", "500000", true, ReasonEligible, "500000.00"},
		{"one paisa over limit", 742, true, "100000", "500000.01", false, ReasonAmountExceedsLimit, "500000.00"},
		{"score 750+ gets 10x balance", 800, true, "100000", "1000000", true, ReasonEligible, "1000000.00"},
		{"score 650-699 gets 2x balance", 650, true, "100000", "200001", false, ReasonAmountExceedsLimit, "200000.00"},
		{"capped at 1 crore", 900, true, "5000000", "10000001", false, ReasonAmountExceedsLimit, "10000000.00"},
		{"score below 650", 649, true, "5000000", "1000", false, ReasonLowCreditScore, "0.00"},
		{"credit checked before 2FA", 600, false, "5000000", "1000", false, ReasonLowCreditScore, "0.00"},
		{"2FA not verified", 800, false, "5000000", "1000", false, ReasonIdentityNotVerified, "0.00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Evaluate(tt.score, tt.verified, rat(tt.balance), rat(tt.amount))
			if d.Approved != tt.approved || d.Reason != tt.reason || d.MaxEligibleAmount != tt.max {
				t.Errorf("got %+v, want approved=%v reason=%s max=%s", d, tt.approved, tt.reason, tt.max)
			}
		})
	}
}
