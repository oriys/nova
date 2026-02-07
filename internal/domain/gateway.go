package domain

import (
	"encoding/json"
	"time"
)

// GatewayRoute maps a domain+path to a function with optional auth and validation
type GatewayRoute struct {
	ID            string            `json:"id"`
	Domain        string            `json:"domain"`                    // "api.example.com"
	Path          string            `json:"path"`                      // "/v1/process" or "/v1/users/{id}"
	Methods       []string          `json:"methods,omitempty"`         // empty = all methods
	FunctionName  string            `json:"function_name"`
	AuthStrategy  string            `json:"auth_strategy"`             // "none", "inherit", "apikey", "jwt"
	AuthConfig    map[string]string `json:"auth_config,omitempty"`
	RequestSchema json.RawMessage   `json:"request_schema,omitempty"`
	RateLimit     *RouteRateLimit   `json:"rate_limit,omitempty"`
	CORS          *CORSConfig       `json:"cors,omitempty"`
	Enabled       bool              `json:"enabled"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// RouteRateLimit defines per-route rate limiting
type RouteRateLimit struct {
	RequestsPerSecond float64 `json:"requests_per_second"`
	BurstSize         int     `json:"burst_size"`
}

// CORSConfig defines CORS settings for a gateway route
type CORSConfig struct {
	AllowOrigins     []string `json:"allow_origins"`               // e.g. ["https://example.com"] or ["*"]
	AllowMethods     []string `json:"allow_methods,omitempty"`     // defaults to route Methods
	AllowHeaders     []string `json:"allow_headers,omitempty"`     // e.g. ["Content-Type", "Authorization"]
	ExposeHeaders    []string `json:"expose_headers,omitempty"`
	AllowCredentials bool     `json:"allow_credentials,omitempty"`
	MaxAge           int      `json:"max_age,omitempty"`           // preflight cache duration in seconds
}
