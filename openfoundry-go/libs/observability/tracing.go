package observability

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// InitTracing wires an OTel tracer provider that exports OTLP/gRPC to
// the endpoint named by OTEL_EXPORTER_OTLP_ENDPOINT (defaults to
// `localhost:4317`). The returned shutdown function MUST be called on
// process exit so spans flush before exit.
//
// If OTEL_TRACES_EXPORTER=none is set, tracing is silently disabled
// (returns a no-op shutdown). This matches the Rust workspace's
// `OTEL_TRACES_EXPORTER=none` escape hatch used in unit tests.
func InitTracing(ctx context.Context, service, version string) (shutdown func(context.Context) error, err error) {
	if os.Getenv("OTEL_TRACES_EXPORTER") == "none" {
		return func(context.Context) error { return nil }, nil
	}

	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:4317"
	}

	exporter, err := otlptrace.New(ctx, otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(), // TLS is terminated upstream by the collector.
	))
	if err != nil {
		return nil, fmt.Errorf("otlp exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(service),
			semconv.ServiceVersion(version),
		),
		resource.WithFromEnv(),
		resource.WithProcess(),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}
