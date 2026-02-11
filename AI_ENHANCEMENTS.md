# Nova Platform AI Enhancements

## Overview

This document describes the AI-powered enhancements added to the Nova serverless platform to improve developer experience, performance optimization, and security.

## Features Implemented

### 1. AI-Powered Diagnostics Analysis 🔍

**Endpoint**: `POST /functions/{name}/diagnostics/analyze`

**Description**: Analyzes function performance metrics and provides intelligent insights using OpenAI GPT models.

**Features**:
- Natural language summary of function health
- Root cause analysis for errors and performance issues
- Anomaly detection (latency spikes, error patterns, cold start excess)
- Structured recommendations with priority and expected impact
- Performance scoring (1-10)

**Request Parameters**:
- `window` (query): Time window for analysis (e.g., "24h", "7d")

**Response Example**:
```json
{
  "summary": "Function shows high cold start rate (35%) affecting P95 latency. Memory usage appears optimal.",
  "root_causes": [
    "High cold start rate due to min_replicas=0",
    "Timeout threshold too low for occasional large payloads"
  ],
  "recommendations": [
    {
      "category": "scaling",
      "priority": "high",
      "action": "Set min_replicas=1 to maintain at least one warm instance",
      "expected_impact": "Reduce P95 latency by 40% and eliminate cold start delays"
    }
  ],
  "anomalies": [
    {
      "type": "cold_start_excess",
      "severity": "high",
      "description": "Cold start rate is 35%, significantly above typical 10-15%"
    }
  ],
  "performance_score": 6
}
```

---

### 2. Performance Optimization Advisor 📊

**Endpoint**: `GET /functions/{name}/recommendations`

**Description**: Analyzes historical invocation data to provide data-driven optimization recommendations.

**Features**:
- Memory optimization suggestions based on actual usage patterns
- Timeout configuration recommendations
- Scaling policy optimization (min/max replicas)
- Cold start reduction strategies
- Confidence scores for each recommendation

**Request Parameters**:
- `days` (query): Lookback period in days (default: 7, max: 90)

**Response Example**:
```json
{
  "recommendations": [
    {
      "category": "scaling",
      "priority": "high",
      "current_value": 0,
      "recommended_value": 1,
      "reasoning": "Cold start rate is 28.5%. Setting min_replicas=1 will keep at least one instance warm",
      "expected_impact": "Reduce cold start rate to near 0% for consistent traffic",
      "metrics": {
        "current_cold_start_rate": "28.5%"
      }
    },
    {
      "category": "memory",
      "priority": "medium",
      "current_value": 128,
      "recommended_value": 256,
      "reasoning": "Average latency is 1245.3ms. Increasing memory may improve performance",
      "expected_impact": "Potentially reduce latency by 20-40%",
      "metrics": {
        "current_avg_latency": "1245.3ms"
      }
    }
  ],
  "confidence": 0.7,
  "estimated_savings": "Enable AI service for advanced cost estimation",
  "analysis_summary": "Analyzed function 'my-function' with rule-based advisor. Found 2 recommendations."
}
```

**Algorithm**:
1. Fetches last N days of invocation logs
2. Calculates cold start rate, error rate, latency metrics
3. Applies rule-based heuristics:
   - Cold start rate > 20% + min_replicas=0 → Recommend min_replicas=1
   - Avg latency > 1000ms + memory ≤ 256MB → Recommend doubling memory
   - Timeouts detected → Recommend increasing timeout

---

### 3. Enhanced AI Code Review with Security & Compliance 🛡️

**Endpoint**: `POST /ai/review`

**Description**: Extended code review to include comprehensive security vulnerability scanning and compliance checking.

**New Request Fields**:
- `include_security` (boolean): Enable security vulnerability scanning
- `include_compliance` (boolean): Enable compliance checking

**Security Scanning Covers**:
- SQL injection, command injection, code injection
- Cross-site scripting (XSS), CSRF vulnerabilities
- Insecure cryptographic implementations
- Hardcoded secrets, API keys, passwords
- Unsafe deserialization
- Path traversal vulnerabilities
- Authentication/authorization bypass
- Denial of service vectors

**Compliance Standards**:
- GDPR: Personal data handling, consent, data retention
- PCI-DSS: Credit card data security
- HIPAA: Protected health information (PHI) handling
- SOC2: Security controls and logging
- OWASP Top 10: Common web vulnerabilities

**Response Example**:
```json
{
  "feedback": "Overall code quality: 7/10. Good error handling but security concerns detected.",
  "suggestions": [
    "Add input validation for user-supplied parameters",
    "Use parameterized queries instead of string concatenation"
  ],
  "score": 7,
  "security_issues": [
    {
      "severity": "critical",
      "type": "sql_injection",
      "description": "Potential SQL injection vulnerability in database query construction",
      "line_number": 42,
      "remediation": "Use parameterized queries or ORM methods to prevent SQL injection"
    },
    {
      "severity": "high",
      "type": "hardcoded_secret",
      "description": "API key appears to be hardcoded in source code",
      "line_number": 15,
      "remediation": "Move API key to environment variables or secrets management system"
    }
  ],
  "compliance_issues": [
    {
      "standard": "GDPR",
      "violation": "personal_data_retention",
      "description": "User email addresses stored indefinitely without retention policy",
      "severity": "medium"
    }
  ]
}
```

---

## Technical Implementation

### Backend

**New Packages**:
- `internal/ai/` - AI service with OpenAI integration
- `internal/advisor/` - Performance advisor with rule-based recommendations

**Key Files Modified**:
- `internal/ai/ai.go` - AI service core with OpenAI function calling
- `internal/api/dataplane/handlers.go` - Added diagnostics analysis and recommendations endpoints
- `internal/api/controlplane/ai_handlers.go` - AI service API handlers

**OpenAI Function Calling**:
Uses OpenAI's structured outputs (function calling) for deterministic responses:
- `generate_function_code` - Code generation
- `review_function_code` - Code review with security/compliance
- `rewrite_function_code` - Code improvement
- `analyze_function_diagnostics` - Performance analysis

### Frontend

**Updated Files**:
- `lumen/lib/api.ts` - Added TypeScript types and API client methods

**New API Methods**:
```typescript
// Diagnostics analysis
aiApi.analyzeDiagnostics(functionName: string, params?: { window?: string })

// Performance recommendations
functionsApi.recommendations(name: string, days?: number)

// Enhanced code review
aiApi.review({
  code: string,
  runtime: string,
  include_security?: boolean,
  include_compliance?: boolean
})
```

---

## Configuration

### Enabling AI Features

Set the following environment variables or configure via API:

```bash
# Enable AI service
NOVA_AI_ENABLED=true

# OpenAI API key
NOVA_AI_API_KEY=sk-...

# Model to use (default: gpt-4o-mini)
NOVA_AI_MODEL=gpt-4o-mini

# Base URL (for custom deployments)
NOVA_AI_BASE_URL=https://api.openai.com/v1
```

### API Configuration

```bash
# Get current AI configuration
curl http://localhost:9000/ai/config

# Update AI configuration
curl -X PUT http://localhost:9000/ai/config \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "api_key": "sk-...",
    "model": "gpt-4o-mini"
  }'
```

---

## Use Cases

### 1. Debugging Performance Issues

```bash
# Get diagnostics analysis
curl http://localhost:9000/functions/my-function/diagnostics/analyze

# Review recommendations
curl http://localhost:9000/functions/my-function/recommendations?days=30
```

**Benefit**: Quickly identify performance bottlenecks and get actionable recommendations without manual analysis.

### 2. Pre-Deployment Security Review

```bash
# Review function code with security scanning
curl -X POST http://localhost:9000/ai/review \
  -H "Content-Type: application/json" \
  -d '{
    "code": "...",
    "runtime": "python",
    "include_security": true,
    "include_compliance": true
  }'
```

**Benefit**: Catch security vulnerabilities and compliance issues before deployment.

### 3. Continuous Optimization

Enable automated recommendations for all functions and monitor:
- Cold start rates
- Memory utilization
- Error patterns
- Cost optimization opportunities

---

## Future Enhancements

### Phase 3: Workflow Intelligence
- Workflow execution failure prediction
- DAG dependency optimization
- Enhanced error context for multi-step workflows

### Phase 5: Self-Healing
- Intelligent retry strategies based on error patterns
- Automatic resource scaling based on predictions
- Deployment rollback recommendations

---

## API Reference

### Diagnostics Analysis

```
POST /functions/{name}/diagnostics/analyze?window=24h
```

### Performance Recommendations

```
GET /functions/{name}/recommendations?days=7
```

### AI Code Review

```
POST /ai/review
Content-Type: application/json

{
  "code": "function code here",
  "runtime": "python",
  "include_security": true,
  "include_compliance": true
}
```

### AI Configuration

```
GET  /ai/config
PUT  /ai/config
GET  /ai/status
GET  /ai/models
POST /ai/generate
POST /ai/rewrite
```

---

## Cost Considerations

AI features use OpenAI API calls:
- Code review: ~1-3K tokens per request
- Diagnostics analysis: ~500-1K tokens per request  
- Performance recommendations: Runs locally (no AI cost when using rule-based mode)

**Recommendation**: Enable AI selectively for critical functions or use rule-based alternatives where possible.

---

## Security & Privacy

- API keys are masked in API responses
- Code sent to OpenAI for review is transient (not stored)
- Diagnostic data contains only aggregated metrics, not function code
- All AI features can be disabled via configuration

---

## Contributing

To extend AI capabilities:

1. Add new function tools in `internal/ai/ai.go`
2. Implement service methods for new analysis types
3. Add API endpoints in `internal/api/`
4. Update frontend types in `lumen/lib/api.ts`

Example:
```go
// Define new AI function tool
var myNewTool = map[string]interface{}{
    "type": "function",
    "function": map[string]interface{}{
        "name": "my_analysis",
        // ... schema
    },
}

// Add service method
func (s *Service) MyAnalysis(ctx context.Context, req MyRequest) (*MyResponse, error) {
    resp, err := s.chatCompletionWithTools(ctx, systemPrompt, userPrompt, 
        []interface{}{myNewTool}, "my_analysis", maxTokens)
    // ... parse and return
}
```

---

## License

Same as Nova platform license.
