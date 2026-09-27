package integration

import (
	"net/http"
	"strings"
	"testing"

	"bank-agent-platform/tests/internal/testenv"
)

type profile struct {
	CustomerID    string `json:"customer_id"`
	FullName      string `json:"full_name"`
	PAN           string `json:"pan"`
	Aadhaar       string `json:"aadhaar"`
	CreditScore   int    `json:"credit_score"`
	Is2FAVerified bool   `json:"is_2fa_verified"`
}

func TestProfileMasksPII(t *testing.T) {
	c := customer(t, testenv.Customer{FullName: "Rahul Sharma", Balance: "125000.50", CreditScore: 742, Verified: true})

	status, resp := do(t, http.MethodGet, "/api/v1/accounts/"+c.ID+"/profile", nil)
	expectStatus(t, status, http.StatusOK, resp)

	p := decode[profile](t, resp.Data)
	if p.CustomerID != c.ID || p.FullName != "Rahul Sharma" || p.CreditScore != 742 || !p.Is2FAVerified {
		t.Errorf("unexpected profile: %+v", p)
	}
	if want := "XXXXX" + c.PAN[5:]; p.PAN != want {
		t.Errorf("pan = %q, want %q", p.PAN, want)
	}
	if want := "XXXX XXXX " + c.Aadhaar[8:]; p.Aadhaar != want {
		t.Errorf("aadhaar = %q, want %q", p.Aadhaar, want)
	}

	// Raw PII must not appear anywhere in the response.
	body := string(resp.Data)
	if strings.Contains(body, c.PAN) || strings.Contains(body, c.Aadhaar) {
		t.Errorf("response leaks raw PII: %s", body)
	}
}

func TestProfileBalanceIsExact(t *testing.T) {
	c := customer(t, testenv.Customer{Balance: "125000.50"})

	_, resp := do(t, http.MethodGet, "/api/v1/accounts/"+c.ID+"/profile", nil)
	if !strings.Contains(string(resp.Data), `"balance":125000.50`) {
		t.Errorf("balance not returned exactly: %s", resp.Data)
	}
}

func TestProfileNotFound(t *testing.T) {
	status, resp := do(t, http.MethodGet, "/api/v1/accounts/itest-missing/profile", nil)
	expectStatus(t, status, http.StatusNotFound, resp)
}
