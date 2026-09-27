package integration

import (
	"net/http"
	"testing"
)

func TestHealth(t *testing.T) {
	status, resp := do(t, http.MethodGet, "/health", nil)
	expectStatus(t, status, http.StatusOK, resp)

	data := decode[struct{ Database, Redis string }](t, resp.Data)
	if data.Database != "up" || data.Redis != "up" {
		t.Errorf("database = %q, redis = %q, want both up", data.Database, data.Redis)
	}
}

func TestUnknownRouteUsesEnvelope(t *testing.T) {
	status, resp := do(t, http.MethodGet, "/does-not-exist", nil)
	expectStatus(t, status, http.StatusNotFound, resp)
}

func TestWrongMethodUsesEnvelope(t *testing.T) {
	status, resp := do(t, http.MethodPost, "/health", nil)
	expectStatus(t, status, http.StatusMethodNotAllowed, resp)
}
