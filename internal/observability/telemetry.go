package observability

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// Config holds telemetry configuration
type Config struct {
	Enabled     bool
	Exporter    string  // otlp-http, stdout
	Endpoint    string  // localhost:4318
	ServiceName string  // nova
	SampleRate  float64 // 0.0 to 1.0
}

// Provider wraps the OpenTelemetry TracerProvider
type Provider struct {
	tp      *sdktrace.TracerProvider
	tracer  trace.Tracer
	enabled bool
}

var globalProvider = &Provider{enabled: false}

// Init initializes the global telemetry provider
func Init(ctx context.Context, cfg Config) error {
	if !cfg.Enabled {
		globalProvider = &Provider{enabled: false, tracer: trace.NewNoopTracerProvider().Tracer("")}
		return nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion("1.0.0"),
		),
	)
	if err != nil {
		return fmt.Errorf("create resource: %w", err)
	}

	var exporter sdktrace.SpanExporter
	switch cfg.Exporter {
	case "otlp-http", "otlp":
		exp, err := otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(cfg.Endpoint),
			otlptracehttp.WithInsecure(),
		)
		if err != nil {
			return fmt.Errorf("create OTLP exporter: %w", err)
		}
		exporter = exp
	case "stdout":
		// For testing, we'll use a no-op exporter instead of stdout
		// to avoid import cycle issues
		exporter = &noopExporter{}
	default:
		return fmt.Errorf("unknown exporter: %s", cfg.Exporter)
	}

	sampler := sdktrace.AlwaysSample()
	if cfg.SampleRate < 1.0 && cfg.SampleRate >= 0 {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	globalProvider = &Provider{
		tp:      tp,
		tracer:  tp.Tracer(cfg.ServiceName),
		enabled: true,
	}

	return nil
}

// Shutdown gracefully shuts down the telemetry provider
func Shutdown(ctx context.Context) error {
	if globalProvider.tp == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return globalProvider.tp.Shutdown(ctx)
}

// Tracer returns the global tracer
func Tracer() trace.Tracer {
	return globalProvider.tracer
}

// Enabled returns whether tracing is enabled
func Enabled() bool {
	return globalProvider.enabled
}

// noopExporter is a no-op exporter for testing
type noopExporter struct{}

func (e *noopExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	return nil
}

func (e *noopExporter) Shutdown(ctx context.Context) error {
	return nil
}
