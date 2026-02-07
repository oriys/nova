package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/oriys/nova/internal/domain"
)

// Identity represents an authenticated identity
type Identity struct {
	Subject  string                 // "user:xxx" or "apikey:name"
	KeyName  string                 // API Key name (empty for JWT auth)
	Tier     string                 // Rate limit tier: "default", "premium", etc.
	Claims   map[string]any         // JWT claims or API key metadata
	Policies []domain.PolicyBinding // Authorization policies (from API key or JWT)
}

// contextKey is used for storing Identity in context
type contextKey struct{}

// identityKey is the context key for Identity
var identityKey = contextKey{}

// WithIdentity adds an Identity to the context
func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, identityKey, id)
}

// GetIdentity retrieves the Identity from context
func GetIdentity(ctx context.Context) *Identity {
	if id, ok := ctx.Value(identityKey).(*Identity); ok {
		return id
	}
	return nil
}

// Authenticator is the interface for authentication providers
type Authenticator interface {
	// Authenticate attempts to authenticate the request
	// Returns an Identity if successful, nil otherwise
	Authenticate(r *http.Request) *Identity
}

// Middleware creates an HTTP middleware that requires authentication
func Middleware(authenticators []Authenticator, publicPaths []string) func(http.Handler) http.Handler {
	// Build a set of public paths for fast lookup
	publicSet := make(map[string]bool, len(publicPaths))
	for _, p := range publicPaths {
		publicSet[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if path is public
			if isPublicPath(r.URL.Path, publicSet) {
				next.ServeHTTP(w, r)
				return
			}

			// Try each authenticator in order
			for _, auth := range authenticators {
				if id := auth.Authenticate(r); id != nil {
					// Authentication successful
					ctx := WithIdentity(r.Context(), id)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// No authenticator succeeded
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("WWW-Authenticate", `Bearer realm="nova"`)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized","message":"valid authentication required"}`))
		})
	}
}

// isPublicPath checks if the given path should skip authentication
func isPublicPath(path string, publicSet map[string]bool) bool {
	// Exact match
	if publicSet[path] {
		return true
	}

	// Check for prefix matches (paths ending with /*)
	for p := range publicSet {
		if strings.HasSuffix(p, "/*") {
			prefix := strings.TrimSuffix(p, "*")
			if strings.HasPrefix(path, prefix) {
				return true
			}
		}
	}

	return false
}
