package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func record(t *testing.T, write func(c *gin.Context)) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	write(c)

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	return w.Code, body
}

func TestSuccessDefaults(t *testing.T) {
	code, body := record(t, func(c *gin.Context) { Success(c, "", gin.H{"id": 1}) })

	if code != http.StatusOK || body["status"] != 200.0 || body["success"] != true || body["message"] != "Success" {
		t.Fatalf("unexpected response: %d %v", code, body)
	}
	if body["data"].(map[string]any)["id"] != 1.0 {
		t.Fatalf("unexpected data: %v", body["data"])
	}
}

func TestNotFoundCustomMessageNullData(t *testing.T) {
	code, body := record(t, func(c *gin.Context) { NotFound(c, "Account not found", nil) })

	if code != http.StatusNotFound || body["success"] != false || body["message"] != "Account not found" {
		t.Fatalf("unexpected response: %d %v", code, body)
	}
	if v, ok := body["data"]; !ok || v != nil {
		t.Fatalf("data should be present and null, got %v", body)
	}
}

func TestInvalidStatusFallsBackTo500(t *testing.T) {
	code, body := record(t, func(c *gin.Context) { Create(c, http.StatusTeapot, "tea", true, nil) })

	if code != http.StatusInternalServerError || body["status"] != 500.0 || body["success"] != false {
		t.Fatalf("unexpected response: %d %v", code, body)
	}
}
