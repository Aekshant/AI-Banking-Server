package accounts

import "strings"

// MaskPAN hides the 5 leading letters of a PAN: ABCDE1234F -> XXXXX1234F.
func MaskPAN(pan string) string {
	if len(pan) != 10 {
		return strings.Repeat("X", len(pan))
	}
	return "XXXXX" + pan[5:]
}

// MaskAadhaar keeps only the last 4 digits: 123456789012 -> XXXX XXXX 9012.
func MaskAadhaar(aadhaar string) string {
	if len(aadhaar) != 12 {
		return strings.Repeat("X", len(aadhaar))
	}
	return "XXXX XXXX " + aadhaar[8:]
}
