package ratelimit

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/oriys/nova/internal/auth"
)

// Middleware creates an HTTP middleware for rate limiting
func Middleware(limiter *Limiter, publicPaths []string) func(http.Handler) http.Handler {
	// Build public path set
	publicSet := make(map[string]bool, len(publicPaths))
	for _, p := range publicPaths {
		publicSet[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip rate limiting for public paths
			if isPublicPath(r.URL.Path, publicSet) {
				next.ServeHTTP(w, r)
				return
			}

			// Determine rate limit key and tier from identity
			var key, tier string

			if id := auth.GetIdentity(r.Context()); id != nil {
				// Authenticated request - use API key name or subject
				if id.KeyName != "" {
					key = KeyForAPIKey(id.KeyName)
				} else {
					key = KeyForAPIKey(id.Subject)
				}
				tier = id.Tier
			} else {
				// Anonymous request - use IP
				ip := getClientIP(r)
				key = KeyForGlobal(ip)
				tier = "default"
			}

			// Check rate limit
			result, err := limiter.Allow(r.Context(), key, tier)
			if err != nil {
				// On error, allow the request but log
				next.ServeHTTP(w, r)
				return
			}

			// Set rate limit headers
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", result.Remaining))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", result.ResetAt.Unix()))

			if !result.Allowed {
				retryAfter := int(result.ResetAt.Unix() - time.Now().Unix())
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"rate_limit_exceeded","message":"too many requests, please retry later"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isPublicPath checks if the given path should skip rate limiting
func isPublicPath(path string, publicSet map[string]bool) bool {
	if publicSet[path] {
		return true
	}

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

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	// Remove brackets for IPv6
	ip = strings.TrimPrefix(ip, "[")
	ip = strings.TrimSuffix(ip, "]")

	return ip
}
