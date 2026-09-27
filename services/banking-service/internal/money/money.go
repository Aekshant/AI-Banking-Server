// Package money validates and compares rupee amounts exactly, without floats.
package money

import (
	"math/big"
	"regexp"
)

// Up to 13 integer digits and 2 decimals, to fit NUMERIC(15, 2).
var amountPattern = regexp.MustCompile(`^\d{1,13}(\.\d{1,2})?$`)
var zeroPattern = regexp.MustCompile(`^0+(\.0*)?$`)

// Valid reports whether amount is a positive value with at most 2 decimals.
func Valid(amount string) bool {
	return amountPattern.MatchString(amount) && !zeroPattern.MatchString(amount)
}

// Parse converts a decimal string (e.g. a NUMERIC column as text) to an exact
// rational number.
func Parse(amount string) (*big.Rat, bool) {
	return new(big.Rat).SetString(amount)
}

// Format renders an amount with exactly 2 decimals.
func Format(amount *big.Rat) string {
	return amount.FloatString(2)
}
