package observability

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// HTTPMiddleware wraps an http.Handler with OpenTelemetry tracing.
// It extracts trace context from incoming requests and creates server spans.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !Enabled() {
			next.ServeHTTP(w, r)
			return
		}

		// Extract trace context from incoming request headers
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		// Start a new span for this request
		ctx, span := Tracer().Start(ctx, r.Method+" "+r.URL.Path,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPMethod(r.Method),
				semconv.HTTPTarget(r.URL.Path),
				semconv.HTTPScheme(r.URL.Scheme),
				attribute.String("http.host", r.Host),
				attribute.String("http.user_agent", r.UserAgent()),
			),
		)
		defer span.End()

		// Wrap response writer to capture status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Serve request with traced context
		next.ServeHTTP(rw, r.WithContext(ctx))

		// Record response attributes
		span.SetAttributes(
			semconv.HTTPStatusCode(rw.statusCode),
			attribute.Int64("http.response_size", rw.bytesWritten),
		)

		// Mark span as error if status >= 400
		if rw.statusCode >= 400 {
			span.SetStatus(1, http.StatusText(rw.statusCode)) // codes.Error = 1
		}
	})
}

// responseWriter wraps http.ResponseWriter to capture status code and bytes written
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// TracingHandler wraps a specific handler function with tracing
func TracingHandler(name string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !Enabled() {
			handler(w, r)
			return
		}

		ctx, span := StartServerSpan(r.Context(), name,
			attribute.String("http.method", r.Method),
			attribute.String("http.path", r.URL.Path),
		)
		defer span.End()

		handler(w, r.WithContext(ctx))
	}
}
