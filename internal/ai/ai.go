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

// Generate creates function code from a natural language description.
func (s *Service) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("AI service is not enabled")
	}

	systemPrompt := `You are an expert serverless function developer. Generate clean, production-ready function code for the Nova serverless platform.
The function should:
- Read input from a JSON file path passed as argv[1]
- Write JSON output to stdout
- Handle errors gracefully
- Be well-structured and follow best practices for the given runtime

Respond ONLY with a JSON object containing these fields:
- "code": the complete function source code as a string
- "explanation": a brief explanation of what the code does
- "function_name": a suggested snake_case function name`

	userPrompt := fmt.Sprintf("Generate a %s function that: %s", req.Runtime, req.Description)

	resp, err := s.chatCompletion(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("generate: %w", err)
	}

	var result GenerateResponse
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
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

	systemPrompt := `You are an expert code reviewer for serverless functions on the Nova platform.
Review the given function code and provide actionable feedback.

Respond ONLY with a JSON object containing these fields:
- "feedback": a detailed review as a string with markdown formatting
- "suggestions": an array of specific improvement suggestions
- "score": an integer from 1-10 rating the code quality`

	userPrompt := fmt.Sprintf("Review this %s function:\n\n```\n%s\n```", req.Runtime, req.Code)

	resp, err := s.chatCompletion(ctx, systemPrompt, userPrompt)
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

	systemPrompt := `You are an expert serverless function developer for the Nova platform.
Rewrite the given function code to improve it based on the provided instructions.
If no specific instructions are given, improve code quality, error handling, and performance.

Respond ONLY with a JSON object containing these fields:
- "code": the complete rewritten function source code
- "explanation": a brief explanation of what was changed and why`

	instructions := req.Instructions
	if instructions == "" {
		instructions = "Improve code quality, error handling, and performance"
	}
	userPrompt := fmt.Sprintf("Rewrite this %s function. Instructions: %s\n\n```\n%s\n```", req.Runtime, instructions, req.Code)

	resp, err := s.chatCompletion(ctx, systemPrompt, userPrompt)
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

// defaultTemperature is set low for deterministic, consistent code generation.
const defaultTemperature = 0.3

// chatCompletion sends a request to the OpenAI-compatible chat API.
func (s *Service) chatCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	reqBody := map[string]interface{}{
		"model": s.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"temperature": defaultTemperature,
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

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no response from AI model")
	}

	return chatResp.Choices[0].Message.Content, nil
}
