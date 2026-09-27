package loans

import (
	"math/big"

	"banking-service/internal/money"
)

// Reason codes returned by Evaluate.
const (
	ReasonEligible            = "ELIGIBLE"
	ReasonLowCreditScore      = "LOW_CREDIT_SCORE"
	ReasonIdentityNotVerified = "IDENTITY_NOT_VERIFIED"
	ReasonAmountExceedsLimit  = "AMOUNT_EXCEEDS_LIMIT"
)

// Lending policy. Deterministic by design: the same inputs always produce
// the same decision, with no model involved.
const (
	MinCreditScore = 650
	// MaxLoanAmount is an absolute cap of ₹1 crore regardless of profile.
	MaxLoanAmount = 1_00_00_000
)

// balanceMultiplier returns how many times the account balance a customer
// may borrow, based on credit score.
func balanceMultiplier(creditScore int) int64 {
	switch {
	case creditScore >= 750:
		return 10
	case creditScore >= 700:
		return 5
	default:
		return 2
	}
}

type Decision struct {
	Approved          bool
	Reason            string
	MaxEligibleAmount string // 2 decimals; "0.00" when not eligible at all
}

// Evaluate applies the lending rules in order; the first failing rule
// decides the reason:
//
//  1. credit score must be at least MinCreditScore
//  2. identity must be verified (2FA)
//  3. amount must not exceed min(balance x multiplier, MaxLoanAmount)
func Evaluate(creditScore int, is2FAVerified bool, balance, loanAmount *big.Rat) Decision {
	if creditScore < MinCreditScore {
		return Decision{Reason: ReasonLowCreditScore, MaxEligibleAmount: "0.00"}
	}
	if !is2FAVerified {
		return Decision{Reason: ReasonIdentityNotVerified, MaxEligibleAmount: "0.00"}
	}

	limit := new(big.Rat).Mul(balance, new(big.Rat).SetInt64(balanceMultiplier(creditScore)))
	if ceiling := new(big.Rat).SetInt64(MaxLoanAmount); limit.Cmp(ceiling) > 0 {
		limit = ceiling
	}

	if loanAmount.Cmp(limit) > 0 {
		return Decision{Reason: ReasonAmountExceedsLimit, MaxEligibleAmount: money.Format(limit)}
	}
	return Decision{Approved: true, Reason: ReasonEligible, MaxEligibleAmount: money.Format(limit)}
}
