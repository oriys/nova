// Package audit provides HTTP middleware for platform-level audit logging.
// Every mutating request (POST, PUT, PATCH, DELETE) is captured in an
// append-only audit_logs table with full context: actor, tenant, resource,
// HTTP details, and outcome.
package audit

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oriys/nova/internal/auth"
	"github.com/oriys/nova/internal/logging"
	"github.com/oriys/nova/internal/store"
)

// maxBodyCapture limits how much of the request body is stored in the audit log.
const maxBodyCapture = 4096

// AuditStore is the subset of store.MetadataStore needed by the middleware.
type AuditStore interface {
	SaveAuditLog(ctx context.Context, log *store.AuditLog) error
}

// Middleware returns an HTTP middleware that records audit log entries for
// all mutating (non-GET/HEAD/OPTIONS) requests.
func Middleware(s AuditStore, skipPaths []string) func(http.Handler) http.Handler {
	skip := make(map[string]bool, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only audit mutating methods.
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			// Skip non-auditable paths (health, metrics).
			if skip[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			// Capture a limited portion of the request body.
			var bodySample string
			if r.Body != nil {
				buf := make([]byte, maxBodyCapture)
				n, _ := io.ReadAtLeast(r.Body, buf, 1)
				if n > 0 {
					bodySample = string(buf[:n])
				}
				// Reconstruct the body so handlers can still read it.
				remaining, _ := io.ReadAll(r.Body)
				r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(buf[:n]), bytes.NewReader(remaining)))
			}

			// Wrap response writer to capture status code.
			rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
			next.ServeHTTP(rec, r)

			// Extract actor and tenant from context (populated by auth middleware).
			actor, actorType := extractActor(r.Context())
			scope := store.TenantScopeFromContext(r.Context())
			resType, resName := classifyResource(r.URL.Path)
			action := classifyAction(r.Method)

			entry := &store.AuditLog{
				ID:              uuid.New().String(),
				TenantID:        scope.TenantID,
				Namespace:       scope.Namespace,
				Actor:           actor,
				ActorType:       actorType,
				Action:          action,
				ResourceType:    resType,
				ResourceName:    resName,
				HTTPMethod:      r.Method,
				HTTPPath:        r.URL.Path,
				StatusCode:      rec.code,
				RequestBody:     bodySample,
				ResponseSummary: "",
				IPAddress:       clientIP(r),
				UserAgent:       r.UserAgent(),
				CreatedAt:       time.Now().UTC(),
			}

			// Fire-and-forget: don't block the response for audit persistence.
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				// Carry tenant scope into the background context.
				ctx = store.WithTenantScope(ctx, entry.TenantID, entry.Namespace)
				if err := s.SaveAuditLog(ctx, entry); err != nil {
					logging.Op().Warn("failed to save audit log", "error", err, "path", entry.HTTPPath)
				}
			}()
		})
	}
}

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

// extractActor derives the actor identifier and type from the request context.
func extractActor(ctx context.Context) (string, string) {
	id := auth.GetIdentity(ctx)
	if id == nil {
		return "anonymous", "anonymous"
	}
	subject := id.Subject
	if strings.HasPrefix(subject, "apikey:") {
		return subject, "apikey"
	}
	return subject, "user"
}

// classifyResource infers resource type and name from the URL path.
// e.g. /functions/my-fn → ("function", "my-fn")
func classifyResource(path string) (string, string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return "unknown", ""
	}
	resourceType := singularize(parts[0])
	resourceName := ""
	if len(parts) >= 2 {
		resourceName = parts[1]
	}
	return resourceType, resourceName
}

// classifyAction maps HTTP methods to semantic actions.
func classifyAction(method string) string {
	switch method {
	case http.MethodPost:
		return "create"
	case http.MethodPut:
		return "update"
	case http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	default:
		return strings.ToLower(method)
	}
}

// singularize removes trailing 's' from resource names for consistency.
func singularize(s string) string {
	if strings.HasSuffix(s, "ses") {
		return s[:len(s)-2] // "processes" → "process"
	}
	if strings.HasSuffix(s, "ies") {
		return s[:len(s)-3] + "y" // "policies" → "policy"
	}
	if strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "ss") {
		return s[:len(s)-1]
	}
	return s
}

// clientIP returns the best estimate of the client IP address.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	// Fall back to RemoteAddr (may include port).
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i > 0 {
		return addr[:i]
	}
	return addr
}
