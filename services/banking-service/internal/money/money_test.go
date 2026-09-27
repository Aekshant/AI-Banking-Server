package money

import "testing"

func TestValid(t *testing.T) {
	valid := []string{"5000", "0.01", "1.5", "125000.50", "9999999999999.99"}
	invalid := []string{"0", "0.00", "-5", "1.234", "1e3", "abc", "", "10000000000000", ".5"}

	for _, a := range valid {
		if !Valid(a) {
			t.Errorf("Valid(%q) = false, want true", a)
		}
	}
	for _, a := range invalid {
		if Valid(a) {
			t.Errorf("Valid(%q) = true, want false", a)
		}
	}
}
