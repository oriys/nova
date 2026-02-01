package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// TraceContext holds W3C trace context fields for propagation over vsock
type TraceContext struct {
	TraceParent string `json:"traceparent,omitempty"`
	TraceState  string `json:"tracestate,omitempty"`
}

// ExtractTraceContext extracts trace context from a context for propagation
func ExtractTraceContext(ctx context.Context) TraceContext {
	if !Enabled() {
		return TraceContext{}
	}

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	return TraceContext{
		TraceParent: carrier.Get("traceparent"),
		TraceState:  carrier.Get("tracestate"),
	}
}

// InjectTraceContext injects trace context from TraceContext into a context
func InjectTraceContext(ctx context.Context, tc TraceContext) context.Context {
	if tc.TraceParent == "" {
		return ctx
	}

	carrier := propagation.MapCarrier{
		"traceparent": tc.TraceParent,
		"tracestate":  tc.TraceState,
	}

	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// GetTraceID returns the trace ID from context as a string
func GetTraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().HasTraceID() {
		return ""
	}
	return span.SpanContext().TraceID().String()
}

// GetSpanID returns the span ID from context as a string
func GetSpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().HasSpanID() {
		return ""
	}
	return span.SpanContext().SpanID().String()
}
