package observability

import (
	"context"
	"crypto/rand"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const ServiceName = "banking-service"

// SetupTracing exports traces over OTLP/gRPC when OTEL_EXPORTER_OTLP_ENDPOINT
// (or OTEL_EXPORTER_OTLP_TRACES_ENDPOINT) is set, e.g. http://localhost:4317.
// Otherwise tracing stays a no-op. Standard OTEL_* variables apply, such as
// OTEL_TRACES_SAMPLER and OTEL_RESOURCE_ATTRIBUTES.
//
// The returned function flushes pending spans; call it on shutdown.
func SetupTracing(ctx context.Context) (shutdown func(context.Context) error, err error) {
	// W3C trace context lets traces continue across services (e.g. from the agent).
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		slog.Info("tracing disabled (set OTEL_EXPORTER_OTLP_ENDPOINT to enable)")
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, err
	}

	// OTEL_SERVICE_NAME / OTEL_RESOURCE_ATTRIBUTES (WithFromEnv) override the defaults.
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(ServiceName), semconv.ServiceVersion(Version)),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(res))
	otel.SetTracerProvider(tp)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		slog.Warn("opentelemetry", "error", err)
	}))

	slog.Info("tracing enabled")
	return tp.Shutdown, nil
}

// WithoutTracing returns a context whose spans are not recorded. The /health
// endpoint uses it so Docker's frequent health checks don't flood Tempo with
// Postgres and Redis ping traces.
func WithoutTracing(ctx context.Context) context.Context {
	var traceID trace.TraceID
	var spanID trace.SpanID
	_, _ = rand.Read(traceID[:])
	_, _ = rand.Read(spanID[:])
	// A valid but unsampled parent: the parent-based sampler drops every child.
	return trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID, SpanID: spanID, TraceFlags: 0,
	}))
}
