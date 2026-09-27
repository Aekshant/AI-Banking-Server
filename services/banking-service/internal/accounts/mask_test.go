package accounts

import "testing"

func TestMaskPAN(t *testing.T) {
	tests := map[string]string{
		"ABCPS1234F": "XXXXX1234F",
		"ABC":        "XXX",
		"":           "",
	}
	for in, want := range tests {
		if got := MaskPAN(in); got != want {
			t.Errorf("MaskPAN(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMaskAadhaar(t *testing.T) {
	tests := map[string]string{
		"234567899012": "XXXX XXXX 9012",
		"12345":        "XXXXX",
		"":             "",
	}
	for in, want := range tests {
		if got := MaskAadhaar(in); got != want {
			t.Errorf("MaskAadhaar(%q) = %q, want %q", in, got, want)
		}
	}
}
