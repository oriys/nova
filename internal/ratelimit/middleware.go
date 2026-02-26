package ratelimit

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/oriys/nova/internal/auth"
)

// Middleware creates an HTTP middleware for rate limiting
func Middleware(limiter *Limiter, publicPaths []string, trustedProxies ...string) func(http.Handler) http.Handler {
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
				ip := getClientIP(r, trustedProxies)
				key = KeyForGlobal(ip)
				tier = "default"
			}

			// Check rate limit
			result, err := limiter.Allow(r.Context(), key, tier)
			if err != nil {
				// Fail-closed: reject the request when the rate limit backend is unavailable
				// to prevent self-amplifying DoS when the backend is overloaded.
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(`{"error":"service_unavailable","message":"rate limit service temporarily unavailable"}`))
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

// getClientIP extracts the client IP from the request.
// X-Forwarded-For and X-Real-IP headers are only trusted when the remote
// address belongs to the configured list of trusted proxy CIDRs/IPs.
func getClientIP(r *http.Request, trustedProxies []string) string {
	remoteIP := extractRemoteIP(r.RemoteAddr)

	// Only trust proxy headers when the direct peer is a known proxy
	if isTrustedProxy(remoteIP, trustedProxies) {
		// Check X-Forwarded-For header — take the rightmost non-trusted IP
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			for i := len(parts) - 1; i >= 0; i-- {
				ip := strings.TrimSpace(parts[i])
				if ip != "" && !isTrustedProxy(ip, trustedProxies) {
					return ip
				}
			}
		}

		// Check X-Real-IP header
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}

	return remoteIP
}

// extractRemoteIP extracts the IP from a host:port RemoteAddr string.
func extractRemoteIP(addr string) string {
	ip := addr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	ip = strings.TrimPrefix(ip, "[")
	ip = strings.TrimSuffix(ip, "]")
	return ip
}

// isTrustedProxy checks if the given IP matches any trusted proxy entry.
// Entries can be plain IPs or CIDR notation.
func isTrustedProxy(ip string, trusted []string) bool {
	if len(trusted) == 0 {
		return false
	}
	for _, entry := range trusted {
		if strings.Contains(entry, "/") {
			_, cidr, err := net.ParseCIDR(entry)
			if err != nil {
				continue
			}
			if cidr.Contains(net.ParseIP(ip)) {
				return true
			}
		} else if ip == strings.TrimSpace(entry) {
			return true
		}
	}
	return false
}
