package domain

import (
	"encoding/json"
	"time"
)

// GatewayRoute maps a domain+path to a function with optional auth and validation
type GatewayRoute struct {
	ID            string            `json:"id"`
	TenantID      string            `json:"tenant_id"`
	Domain        string            `json:"domain"`            // "api.example.com"
	Path          string            `json:"path"`              // "/v1/process" or "/v1/users/{id}"
	Methods       []string          `json:"methods,omitempty"` // empty = all methods
	FunctionName  string            `json:"function_name"`
	WorkflowName  string            `json:"workflow_name,omitempty"`
	AuthStrategy  string            `json:"auth_strategy"` // "none", "inherit", "apikey", "jwt"
	AuthConfig    map[string]string `json:"auth_config,omitempty"`
	RequestSchema json.RawMessage   `json:"request_schema,omitempty"`
	ParamMapping  []ParamMapping    `json:"param_mapping,omitempty"` // parameter extraction & transformation rules
	RateLimit     *RouteRateLimit   `json:"rate_limit,omitempty"`
	CORS          *CORSConfig       `json:"cors,omitempty"`
	TimeoutMs     int               `json:"timeout_ms,omitempty"`   // per-route invoke timeout (0 = no extra timeout)
	RetryPolicy   *RouteRetryPolicy `json:"retry_policy,omitempty"` // optional per-route invoke retry policy
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
	AllowOrigins     []string `json:"allow_origins"`           // e.g. ["https://example.com"] or ["*"]
	AllowMethods     []string `json:"allow_methods,omitempty"` // defaults to route Methods
	AllowHeaders     []string `json:"allow_headers,omitempty"` // e.g. ["Content-Type", "Authorization"]
	ExposeHeaders    []string `json:"expose_headers,omitempty"`
	AllowCredentials bool     `json:"allow_credentials,omitempty"`
	MaxAge           int      `json:"max_age,omitempty"` // preflight cache duration in seconds
}

// RouteRetryPolicy defines retry behavior for function invocation errors.
type RouteRetryPolicy struct {
	MaxAttempts int `json:"max_attempts"`         // total attempts including initial try
	BackoffMs   int `json:"backoff_ms,omitempty"` // fixed delay between retry attempts
}

// ParamMapping defines how to extract a value from the HTTP request (query string,
// path parameter, request body, or header) and place it into the function payload
// with optional name remapping, case transformation, and type coercion.
//
// Example mappings:
//
//	{"source":"query",  "name":"user_id",   "target":"userId",   "transform":"camel_case"}
//	{"source":"path",   "name":"id",        "target":"recordId", "type":"integer"}
//	{"source":"body",   "name":"email",     "target":"Email",    "transform":"upper_first"}
//	{"source":"header", "name":"X-Request-ID", "target":"requestId"}
//	{"source":"query",  "name":"active",    "target":"isActive", "type":"boolean"}
type ParamMapping struct {
	Source    ParamSource    `json:"source"`              // where to read the value
	Name      string         `json:"name"`                // source field/key name
	Target    string         `json:"target,omitempty"`    // destination key in payload (default = Name)
	Transform ParamTransform `json:"transform,omitempty"` // string case transformation
	Type      ParamType      `json:"type,omitempty"`      // type coercion (default = string)
	Default   any            `json:"default,omitempty"`   // fallback when source value is absent
	Required  bool           `json:"required,omitempty"`  // reject request if missing
}

// ParamSource identifies where to extract a parameter value.
type ParamSource string

const (
	ParamSourceQuery  ParamSource = "query"  // URL query string (?key=val)
	ParamSourcePath   ParamSource = "path"   // path parameter ({id})
	ParamSourceBody   ParamSource = "body"   // JSON request body field
	ParamSourceHeader ParamSource = "header" // HTTP header
)

// ParamTransform describes a string case transformation.
type ParamTransform string

const (
	ParamTransformNone       ParamTransform = ""            // no transformation
	ParamTransformCamelCase  ParamTransform = "camel_case"  // user_name → userName
	ParamTransformSnakeCase  ParamTransform = "snake_case"  // userName → user_name
	ParamTransformUpperCase  ParamTransform = "upper_case"  // hello → HELLO
	ParamTransformLowerCase  ParamTransform = "lower_case"  // HELLO → hello
	ParamTransformUpperFirst ParamTransform = "upper_first" // hello → Hello
	ParamTransformKebabCase  ParamTransform = "kebab_case"  // userName → user-name
)

// ParamType describes the target type for coercion.
type ParamType string

const (
	ParamTypeString  ParamType = ""        // default — keep as string
	ParamTypeInteger ParamType = "integer" // "42" → 42
	ParamTypeFloat   ParamType = "float"   // "3.14" → 3.14
	ParamTypeBoolean ParamType = "boolean" // "true" → true, "1" → true
	ParamTypeJSON    ParamType = "json"    // parse as raw JSON value
)
