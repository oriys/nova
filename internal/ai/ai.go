package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config holds AI service configuration.
type Config struct {
	Enabled   bool   `json:"enabled"`
	APIKey    string `json:"api_key"`
	Model     string `json:"model"`
	BaseURL   string `json:"base_url"`
	PromptDir string `json:"prompt_dir"`
}

// DefaultConfig returns sensible defaults for the AI service.
func DefaultConfig() Config {
	return Config{
		Enabled:   false,
		Model:     "gpt-4o-mini",
		BaseURL:   "https://api.openai.com/v1",
		PromptDir: defaultPromptDir,
	}
}

// GenerateRequest is the request payload for code generation.
type GenerateRequest struct {
	Description string `json:"description"`
	Runtime     string `json:"runtime"`
}

// GenerateResponse is the response for code generation.
type GenerateResponse struct {
	Code         string `json:"code"`
	Explanation  string `json:"explanation,omitempty"`
	FunctionName string `json:"function_name,omitempty"`
}

// ReviewRequest is the request payload for code review.
type ReviewRequest struct {
	Code              string `json:"code"`
	Runtime           string `json:"runtime"`
	IncludeSecurity   bool   `json:"include_security,omitempty"`   // Include security vulnerability scanning
	IncludeCompliance bool   `json:"include_compliance,omitempty"` // Include compliance checks
}

// ReviewResponse is the response for code review.
type ReviewResponse struct {
	Feedback         string            `json:"feedback"`
	Suggestions      []string          `json:"suggestions,omitempty"`
	Score            int               `json:"score,omitempty"`
	SecurityIssues   []SecurityIssue   `json:"security_issues,omitempty"`
	ComplianceIssues []ComplianceIssue `json:"compliance_issues,omitempty"`
}

// SecurityIssue represents a detected security vulnerability.
type SecurityIssue struct {
	Severity    string `json:"severity"` // critical, high, medium, low
	Type        string `json:"type"`     // e.g., sql_injection, command_injection, xss
	Description string `json:"description"`
	LineNumber  int    `json:"line_number,omitempty"`
	Remediation string `json:"remediation"`
}

// ComplianceIssue represents a compliance violation.
type ComplianceIssue struct {
	Standard    string `json:"standard"` // e.g., GDPR, PCI-DSS, HIPAA
	Violation   string `json:"violation"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

// RewriteRequest is the request payload for code rewriting.
type RewriteRequest struct {
	Code         string `json:"code"`
	Runtime      string `json:"runtime"`
	Instructions string `json:"instructions,omitempty"`
}

// RewriteResponse is the response for code rewriting.
type RewriteResponse struct {
	Code        string `json:"code"`
	Explanation string `json:"explanation,omitempty"`
}

// DiagnosticsAnalysisRequest is the request payload for analyzing function diagnostics.
type DiagnosticsAnalysisRequest struct {
	FunctionName     string                   `json:"function_name"`
	TotalInvocations int                      `json:"total_invocations"`
	AvgDurationMs    float64                  `json:"avg_duration_ms"`
	P50DurationMs    int64                    `json:"p50_duration_ms"`
	P95DurationMs    int64                    `json:"p95_duration_ms"`
	P99DurationMs    int64                    `json:"p99_duration_ms"`
	MaxDurationMs    int64                    `json:"max_duration_ms"`
	ErrorRatePct     float64                  `json:"error_rate_pct"`
	ColdStartRatePct float64                  `json:"cold_start_rate_pct"`
	SlowCount        int                      `json:"slow_count"`
	ErrorSamples     []DiagnosticsErrorSample `json:"error_samples,omitempty"`
	SlowSamples      []DiagnosticsSlowSample  `json:"slow_samples,omitempty"`
	MemoryMB         int                      `json:"memory_mb,omitempty"`
	TimeoutS         int                      `json:"timeout_s,omitempty"`
}

// DiagnosticsErrorSample represents a sample error for analysis.
type DiagnosticsErrorSample struct {
	Timestamp    string `json:"timestamp"`
	ErrorMessage string `json:"error_message"`
	DurationMs   int64  `json:"duration_ms"`
	ColdStart    bool   `json:"cold_start"`
}

// DiagnosticsSlowSample represents a slow invocation sample.
type DiagnosticsSlowSample struct {
	Timestamp  string `json:"timestamp"`
	DurationMs int64  `json:"duration_ms"`
	ColdStart  bool   `json:"cold_start"`
}

// DiagnosticsAnalysisResponse is the response for diagnostics analysis.
type DiagnosticsAnalysisResponse struct {
	Summary          string                      `json:"summary"`
	RootCauses       []string                    `json:"root_causes"`
	Recommendations  []DiagnosticsRecommendation `json:"recommendations"`
	Anomalies        []DiagnosticsAnomaly        `json:"anomalies"`
	PerformanceScore int                         `json:"performance_score"`
}

// DiagnosticsRecommendation represents an actionable recommendation.
type DiagnosticsRecommendation struct {
	Category       string `json:"category"`
	Priority       string `json:"priority"`
	Action         string `json:"action"`
	ExpectedImpact string `json:"expected_impact"`
}

// DiagnosticsAnomaly represents a detected anomaly.
type DiagnosticsAnomaly struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

// Service provides AI-powered code operations.
type Service struct {
	cfg     Config
	client  *http.Client
	prompts *promptManager
}

var ErrPromptTemplateNotFound = errors.New("prompt template not found")
var ErrInvalidPromptTemplate = errors.New("invalid prompt template")

// NewService creates a new AI service.
func NewService(cfg Config) *Service {
	cfg.PromptDir = normalizePromptDir(cfg.PromptDir)
	prompts, err := newPromptManager(cfg.PromptDir)
	if err != nil {
		prompts = mustNewEmbeddedPromptManager()
	}
	return &Service{
		cfg: cfg,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		prompts: prompts,
	}
}

// Enabled returns whether the AI service is configured and enabled.
func (s *Service) Enabled() bool {
	return s.cfg.Enabled && s.cfg.APIKey != ""
}

// GetConfig returns the current AI configuration (with API key masked).
func (s *Service) GetConfig() Config {
	c := s.cfg
	if len(c.APIKey) > 8 {
		c.APIKey = c.APIKey[:4] + "****" + c.APIKey[len(c.APIKey)-4:]
	} else if c.APIKey != "" {
		c.APIKey = "****"
	}
	return c
}

// UpdateConfig applies new configuration to the service.
func (s *Service) UpdateConfig(cfg Config) {
	cfg.PromptDir = normalizePromptDir(cfg.PromptDir)
	s.cfg = cfg
	prompts, err := newPromptManager(cfg.PromptDir)
	if err == nil {
		s.prompts = prompts
	}
}

// ListPromptTemplates returns all supported prompt templates and override status.
func (s *Service) ListPromptTemplates() ([]PromptTemplateMeta, error) {
	dir := s.cfg.PromptDir
	items := make([]PromptTemplateMeta, 0, len(promptTemplateFiles))
	for _, name := range listPromptTemplateNames() {
		file, _ := promptTemplateFile(name)
		_, statErr := os.Stat(filepath.Join(dir, file))
		customized := statErr == nil
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return nil, fmt.Errorf("stat prompt template %q: %w", name, statErr)
		}
		items = append(items, PromptTemplateMeta{
			Name:        name,
			File:        file,
			Description: promptTemplateDescriptions[name],
			Customized:  customized,
		})
	}
	return items, nil
}

// GetPromptTemplate returns a single prompt template content.
func (s *Service) GetPromptTemplate(name string) (*PromptTemplate, error) {
	file, ok := promptTemplateFile(name)
	if !ok {
		return nil, ErrPromptTemplateNotFound
	}
	content, err := s.prompts.text(name)
	if err != nil {
		return nil, fmt.Errorf("read prompt template %q: %w", name, err)
	}

	_, statErr := os.Stat(filepath.Join(s.cfg.PromptDir, file))
	customized := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, fmt.Errorf("stat prompt template %q: %w", name, statErr)
	}

	return &PromptTemplate{
		Name:        name,
		File:        file,
		Description: promptTemplateDescriptions[name],
		Customized:  customized,
		Content:     content,
	}, nil
}

// UpdatePromptTemplate persists a prompt template override and reloads templates in memory.
func (s *Service) UpdatePromptTemplate(name, content string) (*PromptTemplate, error) {
	file, ok := promptTemplateFile(name)
	if !ok {
		return nil, ErrPromptTemplateNotFound
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("%w: prompt content cannot be empty", ErrInvalidPromptTemplate)
	}
	if _, err := templateFromString(name, content); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPromptTemplate, err)
	}

	dir := s.cfg.PromptDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create prompt dir %q: %w", dir, err)
	}
	target := filepath.Join(dir, file)
	tmp, err := os.CreateTemp(dir, file+".*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create temp prompt template: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.WriteString(content + "\n"); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("write temp prompt template: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("close temp prompt template: %w", err)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("replace prompt template: %w", err)
	}

	prompts, err := newPromptManager(s.cfg.PromptDir)
	if err != nil {
		return nil, fmt.Errorf("reload prompt templates: %w", err)
	}
	s.prompts = prompts

	return s.GetPromptTemplate(name)
}

func normalizePromptDir(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return defaultPromptDir
	}
	return dir
}

// defaultTemperature is set low for deterministic, consistent code generation.
const defaultTemperature = 0.3

// maxGenerateTokens limits the response size for code generation.
const maxGenerateTokens = 4096

// maxReviewTokens limits the response size for code review.
const maxReviewTokens = 2048

// maxRewriteTokens limits the response size for code rewriting.
const maxRewriteTokens = 4096

// --- OpenAI function tool definitions ---

// generateFunctionTool is the OpenAI function tool schema for code generation.
var generateFunctionTool = map[string]interface{}{
	"type": "function",
	"function": map[string]interface{}{
		"name":        "generate_function_code",
		"description": "Generate production-ready serverless function code for the Nova platform based on a natural language description.",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"code": map[string]interface{}{
					"type":        "string",
					"description": "The complete function source code, ready to deploy. Must follow the Nova handler convention for the specified runtime.",
				},
				"explanation": map[string]interface{}{
					"type":        "string",
					"description": "A brief explanation of what the generated function does and key design decisions.",
				},
				"function_name": map[string]interface{}{
					"type":        "string",
					"description": "A suggested function name in snake_case format (1-64 chars, [A-Za-z0-9_-]).",
					"pattern":     "^[A-Za-z0-9_-]{1,64}$",
				},
			},
			"required":             []string{"code", "explanation", "function_name"},
			"additionalProperties": false,
		},
		"strict": true,
	},
}

// reviewFunctionTool is the OpenAI function tool schema for code review.
var reviewFunctionTool = map[string]interface{}{
	"type": "function",
	"function": map[string]interface{}{
		"name":        "review_function_code",
		"description": "Review serverless function code for quality, correctness, security, and Nova platform best practices.",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"feedback": map[string]interface{}{
					"type":        "string",
					"description": "Detailed code review feedback with markdown formatting. Cover: correctness, error handling, security, performance, and Nova platform conventions.",
				},
				"suggestions": map[string]interface{}{
					"type":        "array",
					"description": "List of specific, actionable improvement suggestions.",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
				"score": map[string]interface{}{
					"type":        "integer",
					"description": "Code quality score from 1 (poor) to 10 (excellent).",
					"minimum":     1,
					"maximum":     10,
				},
				"security_issues": map[string]interface{}{
					"type":        "array",
					"description": "List of detected security vulnerabilities.",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"severity": map[string]interface{}{
								"type":        "string",
								"description": "Severity: critical, high, medium, or low",
								"enum":        []string{"critical", "high", "medium", "low"},
							},
							"type": map[string]interface{}{
								"type":        "string",
								"description": "Type of vulnerability (e.g., sql_injection, command_injection, xss, insecure_crypto)",
							},
							"description": map[string]interface{}{
								"type":        "string",
								"description": "Description of the security issue",
							},
							"line_number": map[string]interface{}{
								"type":        "integer",
								"description": "Line number where the issue occurs (if identifiable)",
							},
							"remediation": map[string]interface{}{
								"type":        "string",
								"description": "How to fix the vulnerability",
							},
						},
						"required": []string{"severity", "type", "description", "remediation"},
					},
				},
				"compliance_issues": map[string]interface{}{
					"type":        "array",
					"description": "List of compliance violations.",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"standard": map[string]interface{}{
								"type":        "string",
								"description": "Compliance standard (e.g., GDPR, PCI-DSS, HIPAA, SOC2)",
							},
							"violation": map[string]interface{}{
								"type":        "string",
								"description": "Type of violation",
							},
							"description": map[string]interface{}{
								"type":        "string",
								"description": "Description of the compliance issue",
							},
							"severity": map[string]interface{}{
								"type":        "string",
								"description": "Severity: critical, high, medium, or low",
								"enum":        []string{"critical", "high", "medium", "low"},
							},
						},
						"required": []string{"standard", "violation", "description", "severity"},
					},
				},
			},
			"required":             []string{"feedback", "suggestions", "score"},
			"additionalProperties": false,
		},
		"strict": true,
	},
}

// rewriteFunctionTool is the OpenAI function tool schema for code rewriting.
var rewriteFunctionTool = map[string]interface{}{
	"type": "function",
	"function": map[string]interface{}{
		"name":        "rewrite_function_code",
		"description": "Rewrite and improve serverless function code following Nova platform conventions and best practices.",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"code": map[string]interface{}{
					"type":        "string",
					"description": "The complete rewritten function source code. Must follow the Nova handler convention for the specified runtime.",
				},
				"explanation": map[string]interface{}{
					"type":        "string",
					"description": "A brief explanation of what was changed and why.",
				},
			},
			"required":             []string{"code", "explanation"},
			"additionalProperties": false,
		},
		"strict": true,
	},
}

// analyzeDiagnosticsTool is the OpenAI function tool schema for diagnostics analysis.
var analyzeDiagnosticsTool = map[string]interface{}{
	"type": "function",
	"function": map[string]interface{}{
		"name":        "analyze_function_diagnostics",
		"description": "Analyze serverless function performance diagnostics and provide insights, root cause analysis, and recommendations.",
		"parameters": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"summary": map[string]interface{}{
					"type":        "string",
					"description": "A concise natural language summary of the function's overall health and performance.",
				},
				"root_causes": map[string]interface{}{
					"type":        "array",
					"description": "List of identified root causes for performance issues or errors.",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
				"recommendations": map[string]interface{}{
					"type":        "array",
					"description": "List of actionable recommendations to improve function performance.",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"category": map[string]interface{}{
								"type":        "string",
								"description": "Category of recommendation (e.g., 'resource', 'architecture', 'configuration')",
							},
							"priority": map[string]interface{}{
								"type":        "string",
								"description": "Priority level: 'critical', 'high', 'medium', or 'low'",
								"enum":        []string{"critical", "high", "medium", "low"},
							},
							"action": map[string]interface{}{
								"type":        "string",
								"description": "Specific action to take (e.g., 'Increase memory from 128MB to 512MB')",
							},
							"expected_impact": map[string]interface{}{
								"type":        "string",
								"description": "Expected improvement (e.g., 'Reduce P95 latency by 40%')",
							},
						},
						"required": []string{"category", "priority", "action", "expected_impact"},
					},
				},
				"anomalies": map[string]interface{}{
					"type":        "array",
					"description": "List of detected anomalies or unusual patterns.",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"type": map[string]interface{}{
								"type":        "string",
								"description": "Anomaly type (e.g., 'latency_spike', 'error_pattern', 'cold_start_excess')",
							},
							"severity": map[string]interface{}{
								"type":        "string",
								"description": "Severity: 'critical', 'high', 'medium', or 'low'",
								"enum":        []string{"critical", "high", "medium", "low"},
							},
							"description": map[string]interface{}{
								"type":        "string",
								"description": "Description of the anomaly",
							},
						},
						"required": []string{"type", "severity", "description"},
					},
				},
				"performance_score": map[string]interface{}{
					"type":        "integer",
					"description": "Overall performance score from 1 (poor) to 10 (excellent)",
					"minimum":     1,
					"maximum":     10,
				},
			},
			"required":             []string{"summary", "root_causes", "recommendations", "anomalies", "performance_score"},
			"additionalProperties": false,
		},
		"strict": true,
	},
}

// --- Service methods ---

// Generate creates function code from a natural language description.
func (s *Service) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("AI service is not enabled")
	}

	systemPrompt, userPrompt, err := s.prompts.composeGeneratePrompts(req)
	if err != nil {
		return nil, fmt.Errorf("build prompts: %w", err)
	}

	resp, err := s.chatCompletionWithTools(ctx, systemPrompt, userPrompt, []interface{}{generateFunctionTool}, "generate_function_code", maxGenerateTokens)
	if err != nil {
		return nil, fmt.Errorf("generate: %w", err)
	}

	var result GenerateResponse
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		// Fallback: treat raw response as code
		result.Code = resp
		result.Explanation = "Generated code"
	}
	return &result, nil
}

// Review analyzes function code and provides feedback.
func (s *Service) Review(ctx context.Context, req ReviewRequest) (*ReviewResponse, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("AI service is not enabled")
	}

	systemPrompt, userPrompt, err := s.prompts.composeReviewPrompts(req)
	if err != nil {
		return nil, fmt.Errorf("build prompts: %w", err)
	}

	resp, err := s.chatCompletionWithTools(ctx, systemPrompt, userPrompt, []interface{}{reviewFunctionTool}, "review_function_code", maxReviewTokens)
	if err != nil {
		return nil, fmt.Errorf("review: %w", err)
	}

	var result ReviewResponse
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		result.Feedback = resp
	}
	return &result, nil
}

// Rewrite improves or rewrites function code based on instructions.
func (s *Service) Rewrite(ctx context.Context, req RewriteRequest) (*RewriteResponse, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("AI service is not enabled")
	}

	systemPrompt, userPrompt, err := s.prompts.composeRewritePrompts(req)
	if err != nil {
		return nil, fmt.Errorf("build prompts: %w", err)
	}

	resp, err := s.chatCompletionWithTools(ctx, systemPrompt, userPrompt, []interface{}{rewriteFunctionTool}, "rewrite_function_code", maxRewriteTokens)
	if err != nil {
		return nil, fmt.Errorf("rewrite: %w", err)
	}

	var result RewriteResponse
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		result.Code = resp
		result.Explanation = "Rewritten code"
	}
	return &result, nil
}

// AnalyzeDiagnostics analyzes function performance diagnostics and provides insights.
func (s *Service) AnalyzeDiagnostics(ctx context.Context, req DiagnosticsAnalysisRequest) (*DiagnosticsAnalysisResponse, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("AI service is not enabled")
	}

	systemPrompt, userPrompt, err := s.prompts.composeDiagnosticsPrompts(req)
	if err != nil {
		return nil, fmt.Errorf("build prompts: %w", err)
	}

	resp, err := s.chatCompletionWithTools(ctx, systemPrompt, userPrompt, []interface{}{analyzeDiagnosticsTool}, "analyze_function_diagnostics", maxReviewTokens)
	if err != nil {
		return nil, fmt.Errorf("analyze diagnostics: %w", err)
	}

	var result DiagnosticsAnalysisResponse
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		// Fallback: treat response as summary
		result.Summary = resp
		result.RootCauses = []string{}
		result.Recommendations = []DiagnosticsRecommendation{}
		result.Anomalies = []DiagnosticsAnomaly{}
		result.PerformanceScore = 5
	}
	return &result, nil
}

// ModelEntry represents a single model from the OpenAI-compatible /models endpoint.
type ModelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ListModelsResponse is the response from the OpenAI-compatible /models endpoint.
type ListModelsResponse struct {
	Object string       `json:"object"`
	Data   []ModelEntry `json:"data"`
}

// ListModels fetches available models from the configured base URL.
func (s *Service) ListModels(ctx context.Context) (*ListModelsResponse, error) {
	url := s.cfg.BaseURL + "/models"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if s.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result ListModelsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// --- OpenAI API types (following the official specification) ---

// chatCompletionRequest matches the OpenAI Chat Completions API request format.
// Reference: https://platform.openai.com/docs/api-reference/chat/create
type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Tools       []interface{} `json:"tools,omitempty"`
	ToolChoice  interface{}   `json:"tool_choice,omitempty"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// chatMessage represents a message in the OpenAI chat format.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionResponse matches the OpenAI Chat Completions API response format.
type chatCompletionResponse struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []chatChoice         `json:"choices"`
	Usage   *chatCompletionUsage `json:"usage,omitempty"`
}

// chatChoice represents a single choice in the API response.
type chatChoice struct {
	Index        int               `json:"index"`
	Message      chatChoiceMessage `json:"message"`
	FinishReason string            `json:"finish_reason"`
}

// chatChoiceMessage represents the message content in a choice.
type chatChoiceMessage struct {
	Role      string     `json:"role"`
	Content   *string    `json:"content"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
}

// toolCall represents a function call requested by the model.
type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

// functionCall contains the function name and arguments.
type functionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// chatCompletionUsage tracks token usage.
type chatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// chatCompletionWithTools sends a request to the OpenAI Chat Completions API with function tools.
// It extracts the structured arguments from the tool call matching expectedFn.
func (s *Service) chatCompletionWithTools(ctx context.Context, systemPrompt, userPrompt string, tools []interface{}, expectedFn string, maxTokens int) (string, error) {
	reqBody := chatCompletionRequest{
		Model: s.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Tools:       tools,
		ToolChoice:  map[string]interface{}{"type": "function", "function": map[string]string{"name": expectedFn}},
		Temperature: defaultTemperature,
		MaxTokens:   maxTokens,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := s.cfg.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatCompletionResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no response from AI model")
	}

	choice := chatResp.Choices[0]

	// Extract structured output from tool calls (OpenAI function calling)
	for _, tc := range choice.Message.ToolCalls {
		if tc.Type == "function" && tc.Function.Name == expectedFn {
			return tc.Function.Arguments, nil
		}
	}

	// Fallback: if the model returned content instead of a tool call
	if choice.Message.Content != nil && *choice.Message.Content != "" {
		return *choice.Message.Content, nil
	}

	return "", fmt.Errorf("no tool call or content in AI response")
}
