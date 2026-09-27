package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const RequestIDHeader = "X-Request-ID"

type requestIDKey struct{}

// validRequestID accepts client-supplied ids that are safe to log and echo.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// RequestID returns the request id stored in ctx, or "".
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// RequestIDMiddleware reuses a valid incoming X-Request-ID or generates one,
// echoes it in the response, and stores it in the request context.
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if !validRequestID.MatchString(id) {
			id = newRequestID()
		}
		c.Header(RequestIDHeader, id)
		c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), requestIDKey{}, id))
		trace.SpanFromContext(c.Request.Context()).SetAttributes(attribute.String("request.id", id))
		c.Next()
	}
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// quietPaths are not access-logged or counted: they are polled constantly
// by health checks and Prometheus.
var quietPaths = map[string]bool{"/health": true, "/metrics": true}

// AccessLog writes one structured log line per request.
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		if quietPaths[c.Request.URL.Path] {
			return
		}

		status := c.Writer.Status()
		level := slog.LevelInfo
		switch {
		case status >= 500:
			level = slog.LevelError
		case status >= 400:
			level = slog.LevelWarn
		}

		slog.LogAttrs(c.Request.Context(), level, "request",
			slog.String("method", c.Request.Method),
			slog.String("route", route(c)),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", status),
			slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
			slog.String("client_ip", c.ClientIP()),
			slog.Int("bytes", c.Writer.Size()),
		)
	}
}

// HTTPMetrics records request counts and latency per route template. Latency
// samples carry the trace id as an exemplar, so a slow bucket in Grafana
// links straight to an example trace.
func HTTPMetrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}
		start := time.Now()
		httpInFlight.Inc()
		defer httpInFlight.Dec()

		c.Next()

		r := route(c)
		httpRequests.WithLabelValues(c.Request.Method, r, strconv.Itoa(c.Writer.Status())).Inc()

		elapsed := time.Since(start).Seconds()
		obs := httpDuration.WithLabelValues(c.Request.Method, r)
		if sc := trace.SpanContextFromContext(c.Request.Context()); sc.IsSampled() {
			obs.(prometheus.ExemplarObserver).ObserveWithExemplar(elapsed, prometheus.Labels{"trace_id": sc.TraceID().String()})
			return
		}
		obs.Observe(elapsed)
	}
}

// route returns the matched route template (e.g. /api/v1/accounts/:id/profile)
// so metrics don't get one series per customer id.
func route(c *gin.Context) string {
	if r := c.FullPath(); r != "" {
		return r
	}
	return "unmatched"
}
