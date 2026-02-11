package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Config holds AI service configuration.
type Config struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
	BaseURL string `json:"base_url"`
}

// DefaultConfig returns sensible defaults for the AI service.
func DefaultConfig() Config {
	return Config{
		Enabled: false,
		Model:   "gpt-4o-mini",
		BaseURL: "https://api.openai.com/v1",
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
	Code    string `json:"code"`
	Runtime string `json:"runtime"`
}

// ReviewResponse is the response for code review.
type ReviewResponse struct {
	Feedback    string   `json:"feedback"`
	Suggestions []string `json:"suggestions,omitempty"`
	Score       int      `json:"score,omitempty"`
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

// Service provides AI-powered code operations.
type Service struct {
	cfg    Config
	client *http.Client
}

// NewService creates a new AI service.
func NewService(cfg Config) *Service {
	return &Service{
		cfg: cfg,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
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
	s.cfg = cfg
}

// defaultTemperature is set low for deterministic, consistent code generation.
const defaultTemperature = 0.3

// maxGenerateTokens limits the response size for code generation.
const maxGenerateTokens = 4096

// maxReviewTokens limits the response size for code review.
const maxReviewTokens = 2048

// maxRewriteTokens limits the response size for code rewriting.
const maxRewriteTokens = 4096

// --- Nova platform system prompt ---

const novaPlatformContext = `You are an expert developer for the Nova serverless platform.

## Nova Platform Specification

Nova is a serverless function platform. Functions run inside isolated Firecracker microVMs.

### Handler Convention
- Functions receive a JSON event object and a context object
- Functions return a JSON-serializable result
- Under the hood, the Nova agent reads JSON from a file path (argv[1]) and captures JSON from stdout
- Function authors write idiomatic handler functions — the platform wraps execution

### Supported Runtimes
| Runtime   | ID       | Handler Signature                                              | Compiled |
|-----------|----------|----------------------------------------------------------------|----------|
| Python    | python   | def handler(event, context) -> dict                            | No       |
| Node.js   | node     | function handler(event, context) -> object; module.exports     | No       |
| Go        | go       | func Handler(event json.RawMessage, ctx Context) (any, error)  | Yes      |
| Rust      | rust     | pub fn handler(event: Value, ctx: Context) -> Result<Value>    | Yes      |
| Java      | java     | public static Object handler(String event, Map context)        | Yes      |
| Ruby      | ruby     | def handler(event, context) -> Hash                            | No       |
| PHP       | php      | function handler($event, $context) -> array                    | No       |
| .NET      | dotnet   | public static object Handle(string eventJson, Dict context)    | Yes      |
| Deno      | deno     | export function handler(event, context) -> object              | No       |
| Bun       | bun      | function handler(event, context) -> object; module.exports     | No       |

Note: Go requires an exported Handler (capitalized) function. .NET uses Handle as the entry method name. All other runtimes use lowercase handler.

### Function Constraints
- Function name: 1-64 chars, [A-Za-z0-9_-]
- Memory: 128–10240 MB (default 128)
- Timeout: 1–900 seconds (default 30)
- Handler format: runtime-specific (e.g., "main.handler" for Python/Node, "handler" for Go/Rust)
- Code must be self-contained in a single file (dependencies via layers)

### Code Examples

**Python:**
` + "```python" + `
def handler(event, context):
    name = event.get("name", "World")
    return {"message": f"Hello, {name}!"}
` + "```" + `

**Node.js:**
` + "```javascript" + `
function handler(event, context) {
  const name = event.name || 'World';
  return { message: ` + "`Hello, ${name}!`" + ` };
}
module.exports = { handler };
` + "```" + `

**Go:**
` + "```go" + `
package main

import "encoding/json"

type Event struct {
    Name string ` + "`" + `json:"name"` + "`" + `
}

func Handler(event json.RawMessage, ctx Context) (interface{}, error) {
    var e Event
    json.Unmarshal(event, &e)
    if e.Name == "" { e.Name = "World" }
    return map[string]string{"message": "Hello, " + e.Name + "!"}, nil
}
` + "```" + `

**Rust:**
` + "```rust" + `
use serde_json::Value;

pub fn handler(event: Value, ctx: crate::context::Context) -> Result<Value, String> {
    let name = event.get("name").and_then(|v| v.as_str()).unwrap_or("World");
    Ok(serde_json::json!({ "message": format!("Hello, {}!", name) }))
}
` + "```" + `

**Ruby:**
` + "```ruby" + `
def handler(event, context)
  name = event['name'] || 'World'
  { message: "Hello, #{name}!" }
end
` + "```" + `

**PHP:**
` + "```php" + `
<?php
function handler($event, $context) {
    $name = $event['name'] ?? 'World';
    return ['message' => "Hello, $name!"];
}
` + "```" + `
`

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

// --- Service methods ---

// Generate creates function code from a natural language description.
func (s *Service) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("AI service is not enabled")
	}

	systemPrompt := novaPlatformContext + `

## Your Task
Generate a complete, production-ready function for the Nova platform.
The code MUST follow the handler convention for the specified runtime.
Always call the generate_function_code function with your result.`

	userPrompt := fmt.Sprintf("Generate a **%s** function that: %s", req.Runtime, req.Description)

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

	systemPrompt := novaPlatformContext + `

## Your Task
Review the given function code for the Nova serverless platform.
Evaluate: correctness, error handling, security, performance, and adherence to Nova handler conventions.
Always call the review_function_code function with your result.`

	userPrompt := fmt.Sprintf("Review this **%s** function for the Nova platform:\n\n```\n%s\n```", req.Runtime, req.Code)

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

	systemPrompt := novaPlatformContext + `

## Your Task
Rewrite the given function code to improve it. The result MUST follow Nova handler conventions for the specified runtime.
Always call the rewrite_function_code function with your result.`

	instructions := req.Instructions
	if instructions == "" {
		instructions = "Improve code quality, error handling, and performance while following Nova platform conventions"
	}
	userPrompt := fmt.Sprintf("Rewrite this **%s** function for the Nova platform.\n\nInstructions: %s\n\n```\n%s\n```", req.Runtime, instructions, req.Code)

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

// --- OpenAI API types (following the official specification) ---

// chatCompletionRequest matches the OpenAI Chat Completions API request format.
// Reference: https://platform.openai.com/docs/api-reference/chat/create
type chatCompletionRequest struct {
	Model       string               `json:"model"`
	Messages    []chatMessage        `json:"messages"`
	Tools       []interface{}        `json:"tools,omitempty"`
	ToolChoice  interface{}          `json:"tool_choice,omitempty"`
	Temperature float64              `json:"temperature"`
	MaxTokens   int                  `json:"max_tokens,omitempty"`
}

// chatMessage represents a message in the OpenAI chat format.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionResponse matches the OpenAI Chat Completions API response format.
type chatCompletionResponse struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []chatChoice       `json:"choices"`
	Usage   *chatCompletionUsage `json:"usage,omitempty"`
}

// chatChoice represents a single choice in the API response.
type chatChoice struct {
	Index        int                `json:"index"`
	Message      chatChoiceMessage  `json:"message"`
	FinishReason string             `json:"finish_reason"`
}

// chatChoiceMessage represents the message content in a choice.
type chatChoiceMessage struct {
	Role      string          `json:"role"`
	Content   *string         `json:"content"`
	ToolCalls []toolCall      `json:"tool_calls,omitempty"`
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
