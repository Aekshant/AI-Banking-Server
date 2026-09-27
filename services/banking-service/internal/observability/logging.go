package observability

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// NewLogger builds a structured logger. format is "json" (default) or
// "text"; level is debug, info (default), warn or error.
//
// Every record logged with a request context automatically carries
// request_id, trace_id and span_id, which is what links a log line in Loki
// to its trace in Tempo.
func NewLogger(w io.Writer, format, level string) (*slog.Logger, error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(strings.ToUpper(orDefault(level, "info")))); err != nil {
		return nil, fmt.Errorf("LOG_LEVEL %q: must be debug, info, warn or error", level)
	}
	opts := &slog.HandlerOptions{Level: lvl}

	var h slog.Handler
	switch strings.ToLower(orDefault(format, "json")) {
	case "json":
		h = slog.NewJSONHandler(w, opts)
	case "text":
		h = slog.NewTextHandler(w, opts)
	default:
		return nil, fmt.Errorf("LOG_FORMAT %q: must be json or text", format)
	}
	return slog.New(contextHandler{h}), nil
}

// contextHandler adds request and trace ids from the context to each record.
type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	// Only sampled traces reach Tempo, so only they are worth linking to.
	if sc := trace.SpanContextFromContext(ctx); sc.IsSampled() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
