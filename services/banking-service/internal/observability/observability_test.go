package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel/trace"
)

func TestLoggerAddsRequestAndTraceIDs(t *testing.T) {
	var buf bytes.Buffer
	logger, err := NewLogger(&buf, "json", "info")
	if err != nil {
		t.Fatal(err)
	}

	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled,
	}))
	ctx = context.WithValue(ctx, requestIDKey{}, "req-123")

	logger.With("component", "test").InfoContext(ctx, "hello")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("not JSON: %s", buf.String())
	}
	for k, want := range map[string]string{
		"msg": "hello", "request_id": "req-123", "component": "test",
		"trace_id": "4bf92f3577b34da6a3ce929d0e0e4736", "span_id": "00f067aa0ba902b7",
	} {
		if rec[k] != want {
			t.Errorf("%s = %v, want %s", k, rec[k], want)
		}
	}
}

func TestLoggerWithoutContextHasNoIDs(t *testing.T) {
	var buf bytes.Buffer
	logger, _ := NewLogger(&buf, "json", "info")
	logger.Info("plain")

	if strings.Contains(buf.String(), "trace_id") || strings.Contains(buf.String(), "request_id") {
		t.Errorf("unexpected ids: %s", buf.String())
	}
}

func TestLoggerRejectsBadConfig(t *testing.T) {
	if _, err := NewLogger(&bytes.Buffer{}, "xml", "info"); err == nil {
		t.Error("expected error for LOG_FORMAT=xml")
	}
	if _, err := NewLogger(&bytes.Buffer{}, "json", "loud"); err == nil {
		t.Error("expected error for LOG_LEVEL=loud")
	}
}

func newRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestIDMiddleware(), HTTPMetrics())
	r.GET("/accounts/:id", func(c *gin.Context) {
		c.String(http.StatusOK, RequestID(c.Request.Context()))
	})
	return r
}

func TestRequestIDGeneratedAndEchoed(t *testing.T) {
	w := httptest.NewRecorder()
	newRouter().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/accounts/a", nil))

	id := w.Header().Get(RequestIDHeader)
	if len(id) != 16 || w.Body.String() != id {
		t.Errorf("header %q, body %q: want the same generated 16-char id", id, w.Body.String())
	}
}

func TestRequestIDReusesValidIncomingAndRejectsUnsafe(t *testing.T) {
	for incoming, keep := range map[string]bool{
		"agent-call-42":          true,
		"bad id with spaces":     false,
		strings.Repeat("a", 129): false,
		"<script>":               false,
	} {
		req := httptest.NewRequest(http.MethodGet, "/accounts/a", nil)
		req.Header.Set(RequestIDHeader, incoming)
		w := httptest.NewRecorder()
		newRouter().ServeHTTP(w, req)

		if got := w.Header().Get(RequestIDHeader); (got == incoming) != keep {
			t.Errorf("incoming %q -> %q, keep=%v", incoming, got, keep)
		}
	}
}

func TestHTTPMetricsUseRouteTemplate(t *testing.T) {
	r := newRouter()
	before := testutil.ToFloat64(httpRequests.WithLabelValues("GET", "/accounts/:id", "200"))
	for _, id := range []string{"customer-1", "customer-2", "customer-3"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/accounts/"+id, nil))
	}
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/nope", nil))

	if got := testutil.ToFloat64(httpRequests.WithLabelValues("GET", "/accounts/:id", "200")) - before; got != 3 {
		t.Errorf("requests for /accounts/:id = %v, want 3 (one series for all ids)", got)
	}
	if testutil.ToFloat64(httpRequests.WithLabelValues("GET", "unmatched", "404")) < 1 {
		t.Error("unmatched route not counted under route=\"unmatched\"")
	}
}

func TestMetricsEndpointServesOpenMetricsWithExemplars(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { // stand-in for otelgin: attach a sampled span
		traceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
		spanID, _ := trace.SpanIDFromHex("b7ad6b7169203331")
		sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled})
		c.Request = c.Request.WithContext(trace.ContextWithSpanContext(c.Request.Context(), sc))
	}, HTTPMetrics())
	r.GET("/traced", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/metrics", gin.WrapH(MetricsHandler()))

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/traced", nil))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Accept", "application/openmetrics-text")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `trace_id="0af7651916cd43dd8448eb211c80319c"`) {
		t.Error("latency histogram has no exemplar with the trace id")
	}
	for _, name := range []string{"banking_service_build_info", "go_goroutines", "http_requests_in_flight"} {
		if !strings.Contains(body, name) {
			t.Errorf("/metrics missing %s", name)
		}
	}
}

func init() { slog.SetDefault(slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))) }

func TestWithoutTracingProducesUnsampledContext(t *testing.T) {
	sc := trace.SpanContextFromContext(WithoutTracing(context.Background()))
	if !sc.IsValid() || sc.IsSampled() {
		t.Errorf("want a valid, unsampled span context, got valid=%v sampled=%v", sc.IsValid(), sc.IsSampled())
	}
}
