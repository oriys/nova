package observability

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// StartSpan creates a new span with the given name and attributes
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return Tracer().Start(ctx, name,
		trace.WithAttributes(attrs...),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
}

// StartServerSpan creates a new server span (for incoming requests)
func StartServerSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return Tracer().Start(ctx, name,
		trace.WithAttributes(attrs...),
		trace.WithSpanKind(trace.SpanKindServer),
	)
}

// SpanFromContext returns the current span from context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// SetSpanError marks the span as errored
func SetSpanError(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// SetSpanOK marks the span as successful
func SetSpanOK(span trace.Span) {
	span.SetStatus(codes.Ok, "")
}

// Common attribute keys for Nova spans
var (
	AttrFunctionName = attribute.Key("nova.function.name")
	AttrFunctionID   = attribute.Key("nova.function.id")
	AttrRuntime      = attribute.Key("nova.runtime")
	AttrColdStart    = attribute.Key("nova.cold_start")
	AttrRequestID    = attribute.Key("nova.request_id")
	AttrDurationMs   = attribute.Key("nova.duration_ms")
	AttrVMID         = attribute.Key("nova.vm.id")
)
