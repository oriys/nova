package ai

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
)

const defaultPromptDir = "configs/prompts/ai"

const (
	promptPlatformContext   = "platform_context"
	promptGenerateSystem    = "generate_system"
	promptGenerateUser      = "generate_user"
	promptReviewSystemBase  = "review_system_base"
	promptReviewSecurity    = "review_system_security"
	promptReviewCompliance  = "review_system_compliance"
	promptReviewSystemTail  = "review_system_tail"
	promptReviewUser        = "review_user"
	promptRewriteSystem     = "rewrite_system"
	promptRewriteUser       = "rewrite_user"
	promptDiagnosticsSystem = "diagnostics_system"
	promptDiagnosticsUser   = "diagnostics_user"
)

var promptTemplateFiles = map[string]string{
	promptPlatformContext:   "platform_context.tmpl",
	promptGenerateSystem:    "generate_system.tmpl",
	promptGenerateUser:      "generate_user.tmpl",
	promptReviewSystemBase:  "review_system_base.tmpl",
	promptReviewSecurity:    "review_system_security.tmpl",
	promptReviewCompliance:  "review_system_compliance.tmpl",
	promptReviewSystemTail:  "review_system_tail.tmpl",
	promptReviewUser:        "review_user.tmpl",
	promptRewriteSystem:     "rewrite_system.tmpl",
	promptRewriteUser:       "rewrite_user.tmpl",
	promptDiagnosticsSystem: "diagnostics_system.tmpl",
	promptDiagnosticsUser:   "diagnostics_user.tmpl",
}

var promptTemplateDescriptions = map[string]string{
	promptPlatformContext:   "Shared Nova platform rules and runtime conventions.",
	promptGenerateSystem:    "System prompt suffix for function generation.",
	promptGenerateUser:      "User prompt template for function generation requests.",
	promptReviewSystemBase:  "Base system prompt for code review.",
	promptReviewSecurity:    "Optional security review section.",
	promptReviewCompliance:  "Optional compliance review section.",
	promptReviewSystemTail:  "System prompt tail requiring tool-call output.",
	promptReviewUser:        "User prompt template for code review requests.",
	promptRewriteSystem:     "System prompt for code rewriting tasks.",
	promptRewriteUser:       "User prompt template for code rewrite requests.",
	promptDiagnosticsSystem: "System prompt for diagnostics analysis.",
	promptDiagnosticsUser:   "User prompt template for diagnostics analysis requests.",
}

type PromptTemplateMeta struct {
	Name        string `json:"name"`
	File        string `json:"file"`
	Description string `json:"description"`
	Customized  bool   `json:"customized"`
}

type PromptTemplate struct {
	Name        string `json:"name"`
	File        string `json:"file"`
	Description string `json:"description"`
	Customized  bool   `json:"customized"`
	Content     string `json:"content"`
}

//go:embed prompt_templates/*.tmpl
var embeddedPromptTemplates embed.FS

type promptManager struct {
	raw      map[string]string
	compiled map[string]*template.Template
}

func mustNewEmbeddedPromptManager() *promptManager {
	pm, err := newPromptManager("")
	if err != nil {
		panic(fmt.Sprintf("load embedded AI prompt templates: %v", err))
	}
	return pm
}

func newPromptManager(promptDir string) (*promptManager, error) {
	pm := &promptManager{
		raw:      make(map[string]string, len(promptTemplateFiles)),
		compiled: make(map[string]*template.Template, len(promptTemplateFiles)),
	}

	for id, filename := range promptTemplateFiles {
		content, err := loadPromptTemplate(promptDir, filename)
		if err != nil {
			return nil, err
		}
		pm.raw[id] = content

		tpl, err := templateFromString(id, content)
		if err != nil {
			return nil, fmt.Errorf("parse prompt template %q: %w", filename, err)
		}
		pm.compiled[id] = tpl
	}

	return pm, nil
}

func loadPromptTemplate(promptDir, filename string) (string, error) {
	embeddedPath := filepath.Join("prompt_templates", filename)
	content, err := embeddedPromptTemplates.ReadFile(embeddedPath)
	if err != nil {
		return "", fmt.Errorf("read embedded prompt template %q: %w", embeddedPath, err)
	}

	result := string(content)
	if strings.TrimSpace(promptDir) != "" {
		overridePath := filepath.Join(promptDir, filename)
		override, err := os.ReadFile(overridePath)
		switch {
		case err == nil:
			result = string(override)
		case errors.Is(err, os.ErrNotExist):
			// Fallback to embedded default template when override file does not exist.
		default:
			return "", fmt.Errorf("read prompt template override %q: %w", overridePath, err)
		}
	}

	return strings.TrimSpace(result), nil
}

func promptTemplateFile(name string) (string, bool) {
	file, ok := promptTemplateFiles[name]
	return file, ok
}

func listPromptTemplateNames() []string {
	names := make([]string, 0, len(promptTemplateFiles))
	for name := range promptTemplateFiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func templateFromString(name, content string) (*template.Template, error) {
	tpl, err := template.New(name).Option("missingkey=error").Parse(content)
	if err != nil {
		return nil, fmt.Errorf("parse template %q: %w", name, err)
	}
	return tpl, nil
}

func (p *promptManager) text(id string) (string, error) {
	value, ok := p.raw[id]
	if !ok {
		return "", fmt.Errorf("prompt template %q not found", id)
	}
	return value, nil
}

func (p *promptManager) render(id string, data any) (string, error) {
	tpl, ok := p.compiled[id]
	if !ok {
		return "", fmt.Errorf("prompt template %q not found", id)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render prompt template %q: %w", id, err)
	}
	return strings.TrimSpace(buf.String()), nil
}

func (p *promptManager) composeGeneratePrompts(req GenerateRequest) (string, string, error) {
	platformContext, err := p.text(promptPlatformContext)
	if err != nil {
		return "", "", err
	}
	generateSystem, err := p.text(promptGenerateSystem)
	if err != nil {
		return "", "", err
	}
	systemPrompt, err := joinPromptSectionsFromStrings(platformContext, generateSystem)
	if err != nil {
		return "", "", err
	}

	userPrompt, err := p.render(promptGenerateUser, struct {
		Runtime     string
		Description string
	}{
		Runtime:     req.Runtime,
		Description: req.Description,
	})
	if err != nil {
		return "", "", err
	}

	return systemPrompt, userPrompt, nil
}

func (p *promptManager) composeReviewPrompts(req ReviewRequest) (string, string, error) {
	platformContext, err := p.text(promptPlatformContext)
	if err != nil {
		return "", "", err
	}
	reviewBase, err := p.text(promptReviewSystemBase)
	if err != nil {
		return "", "", err
	}

	sections := []string{platformContext, reviewBase}
	if req.IncludeSecurity {
		reviewSecurity, err := p.text(promptReviewSecurity)
		if err != nil {
			return "", "", err
		}
		sections = append(sections, reviewSecurity)
	}
	if req.IncludeCompliance {
		reviewCompliance, err := p.text(promptReviewCompliance)
		if err != nil {
			return "", "", err
		}
		sections = append(sections, reviewCompliance)
	}
	reviewTail, err := p.text(promptReviewSystemTail)
	if err != nil {
		return "", "", err
	}
	sections = append(sections, reviewTail)

	systemPrompt, err := joinPromptSectionsFromStrings(sections...)
	if err != nil {
		return "", "", err
	}

	userPrompt, err := p.render(promptReviewUser, struct {
		Runtime string
		Code    string
	}{
		Runtime: req.Runtime,
		Code:    req.Code,
	})
	if err != nil {
		return "", "", err
	}

	return systemPrompt, userPrompt, nil
}

func (p *promptManager) composeRewritePrompts(req RewriteRequest) (string, string, error) {
	platformContext, err := p.text(promptPlatformContext)
	if err != nil {
		return "", "", err
	}
	rewriteSystem, err := p.text(promptRewriteSystem)
	if err != nil {
		return "", "", err
	}
	systemPrompt, err := joinPromptSectionsFromStrings(platformContext, rewriteSystem)
	if err != nil {
		return "", "", err
	}

	instructions := req.Instructions
	if strings.TrimSpace(instructions) == "" {
		instructions = "Improve code quality, error handling, and performance while following Nova platform conventions"
	}

	userPrompt, err := p.render(promptRewriteUser, struct {
		Runtime      string
		Instructions string
		Code         string
	}{
		Runtime:      req.Runtime,
		Instructions: instructions,
		Code:         req.Code,
	})
	if err != nil {
		return "", "", err
	}

	return systemPrompt, userPrompt, nil
}

type diagnosticsPromptData struct {
	FunctionName     string
	TotalInvocations int
	AvgDurationMs    string
	P50DurationMs    int64
	P95DurationMs    int64
	P99DurationMs    int64
	MaxDurationMs    int64
	ErrorRatePct     string
	ColdStartRatePct string
	SlowCount        int
	MemoryMB         int
	TimeoutS         int
	ErrorSamples     []DiagnosticsErrorSample
	SlowSamples      []DiagnosticsSlowSample
}

func (p *promptManager) composeDiagnosticsPrompts(req DiagnosticsAnalysisRequest) (string, string, error) {
	systemPrompt, err := p.text(promptDiagnosticsSystem)
	if err != nil {
		return "", "", err
	}

	errorSamples := req.ErrorSamples
	if len(errorSamples) > 5 {
		errorSamples = errorSamples[:5]
	}

	slowSamples := req.SlowSamples
	if len(slowSamples) > 5 {
		slowSamples = slowSamples[:5]
	}

	userPrompt, err := p.render(promptDiagnosticsUser, diagnosticsPromptData{
		FunctionName:     req.FunctionName,
		TotalInvocations: req.TotalInvocations,
		AvgDurationMs:    fmt.Sprintf("%.1f", req.AvgDurationMs),
		P50DurationMs:    req.P50DurationMs,
		P95DurationMs:    req.P95DurationMs,
		P99DurationMs:    req.P99DurationMs,
		MaxDurationMs:    req.MaxDurationMs,
		ErrorRatePct:     fmt.Sprintf("%.2f", req.ErrorRatePct),
		ColdStartRatePct: fmt.Sprintf("%.2f", req.ColdStartRatePct),
		SlowCount:        req.SlowCount,
		MemoryMB:         req.MemoryMB,
		TimeoutS:         req.TimeoutS,
		ErrorSamples:     errorSamples,
		SlowSamples:      slowSamples,
	})
	if err != nil {
		return "", "", err
	}

	return systemPrompt, userPrompt, nil
}

func joinPromptSectionsFromStrings(sections ...string) (string, error) {
	var builder strings.Builder
	for _, section := range sections {
		trimmed := strings.TrimSpace(section)
		if trimmed == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(trimmed)
	}

	result := strings.TrimSpace(builder.String())
	if result == "" {
		return "", fmt.Errorf("empty composed prompt")
	}
	return result, nil
}
