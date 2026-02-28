package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oriys/nova/internal/domain"
)

func TestTransformCase(t *testing.T) {
	tests := []struct {
		input     string
		transform domain.ParamTransform
		want      string
	}{
		{"user_name", domain.ParamTransformCamelCase, "userName"},
		{"content_type_header", domain.ParamTransformCamelCase, "contentTypeHeader"},
		{"userName", domain.ParamTransformSnakeCase, "user_name"},
		{"HTTPResponse", domain.ParamTransformSnakeCase, "http_response"},
		{"userName", domain.ParamTransformKebabCase, "user-name"},
		{"hello", domain.ParamTransformUpperCase, "HELLO"},
		{"HELLO", domain.ParamTransformLowerCase, "hello"},
		{"hello", domain.ParamTransformUpperFirst, "Hello"},
		{"hello", domain.ParamTransformNone, "hello"},
		{"", domain.ParamTransformUpperFirst, ""},
	}
	for _, tt := range tests {
		got := domain.TransformCase(tt.input, tt.transform)
		if got != tt.want {
			t.Errorf("transformCase(%q, %q) = %q, want %q", tt.input, tt.transform, got, tt.want)
		}
	}
}

func TestCoerceString(t *testing.T) {
	tests := []struct {
		input string
		typ   domain.ParamType
		want  any
		err   bool
	}{
		{"hello", domain.ParamTypeString, "hello", false},
		{"42", domain.ParamTypeInteger, int64(42), false},
		{"3.14", domain.ParamTypeFloat, 3.14, false},
		{"true", domain.ParamTypeBoolean, true, false},
		{"yes", domain.ParamTypeBoolean, true, false},
		{"0", domain.ParamTypeBoolean, false, false},
		{"off", domain.ParamTypeBoolean, false, false},
		{"invalid", domain.ParamTypeBoolean, false, true},
		{`{"a":1}`, domain.ParamTypeJSON, map[string]any{"a": float64(1)}, false},
		{"not-json", domain.ParamTypeJSON, nil, true},
	}
	for _, tt := range tests {
		got, err := domain.CoerceString(tt.input, tt.typ)
		if (err != nil) != tt.err {
			t.Errorf("coerceString(%q, %q) error = %v, wantErr %v", tt.input, tt.typ, err, tt.err)
			continue
		}
		if err == nil {
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tt.want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("coerceString(%q, %q) = %v, want %v", tt.input, tt.typ, string(gotJSON), string(wantJSON))
			}
		}
	}
}

func TestApplyParamMappings_Query(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test?user_id=42&name=john&active=true", nil)
	payload := json.RawMessage(`{}`)

	mappings := []domain.ParamMapping{
		{Source: domain.ParamSourceQuery, Name: "user_id", Target: "userId", Transform: domain.ParamTransformCamelCase, Type: domain.ParamTypeInteger},
		{Source: domain.ParamSourceQuery, Name: "name", Target: "Name", Transform: domain.ParamTransformUpperFirst},
		{Source: domain.ParamSourceQuery, Name: "active", Target: "isActive", Type: domain.ParamTypeBoolean},
	}

	result, err := applyParamMappings(payload, r, nil, mappings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]any
	json.Unmarshal(result, &obj)

	if obj["userId"] != float64(42) {
		t.Errorf("userId = %v, want 42", obj["userId"])
	}
	if obj["Name"] != "John" {
		t.Errorf("Name = %v, want John", obj["Name"])
	}
	if obj["isActive"] != true {
		t.Errorf("isActive = %v, want true", obj["isActive"])
	}
}

func TestApplyParamMappings_Path(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	payload := json.RawMessage(`{}`)
	pathParams := map[string]string{"id": "123"}

	mappings := []domain.ParamMapping{
		{Source: domain.ParamSourcePath, Name: "id", Target: "recordId", Type: domain.ParamTypeInteger},
	}

	result, err := applyParamMappings(payload, r, pathParams, mappings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]any
	json.Unmarshal(result, &obj)

	if obj["recordId"] != float64(123) {
		t.Errorf("recordId = %v, want 123", obj["recordId"])
	}
}

func TestApplyParamMappings_Body(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(`{"user_name":"alice","count":"5"}`))
	r.Header.Set("Content-Type", "application/json")
	payload := json.RawMessage(`{"user_name":"alice","count":"5"}`)

	mappings := []domain.ParamMapping{
		{Source: domain.ParamSourceBody, Name: "user_name", Target: "userName", Transform: domain.ParamTransformCamelCase},
		{Source: domain.ParamSourceBody, Name: "count", Target: "total", Type: domain.ParamTypeInteger},
	}

	result, err := applyParamMappings(payload, r, nil, mappings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]any
	json.Unmarshal(result, &obj)

	if obj["userName"] != "alice" {
		t.Errorf("userName = %v, want alice", obj["userName"])
	}
	// count "5" from body is a string, should be coerced to integer
	if obj["total"] != float64(5) {
		t.Errorf("total = %v, want 5", obj["total"])
	}
}

func TestApplyParamMappings_Header(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.Header.Set("X-Request-ID", "req-abc-123")
	payload := json.RawMessage(`{}`)

	mappings := []domain.ParamMapping{
		{Source: domain.ParamSourceHeader, Name: "X-Request-ID", Target: "requestId"},
	}

	result, err := applyParamMappings(payload, r, nil, mappings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]any
	json.Unmarshal(result, &obj)

	if obj["requestId"] != "req-abc-123" {
		t.Errorf("requestId = %v, want req-abc-123", obj["requestId"])
	}
}

func TestApplyParamMappings_Required(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test", nil) // no query params
	payload := json.RawMessage(`{}`)

	mappings := []domain.ParamMapping{
		{Source: domain.ParamSourceQuery, Name: "token", Required: true},
	}

	_, err := applyParamMappings(payload, r, nil, mappings)
	if err == nil {
		t.Fatal("expected error for missing required param")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error = %v, want 'required' in message", err)
	}
}

func TestApplyParamMappings_Default(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	payload := json.RawMessage(`{}`)

	mappings := []domain.ParamMapping{
		{Source: domain.ParamSourceQuery, Name: "page", Target: "page", Default: 1},
	}

	result, err := applyParamMappings(payload, r, nil, mappings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]any
	json.Unmarshal(result, &obj)

	if obj["page"] != float64(1) {
		t.Errorf("page = %v, want 1", obj["page"])
	}
}

func TestApplyParamMappings_TargetDefaultsToName(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/test?color=blue", nil)
	payload := json.RawMessage(`{}`)

	mappings := []domain.ParamMapping{
		{Source: domain.ParamSourceQuery, Name: "color"},
	}

	result, err := applyParamMappings(payload, r, nil, mappings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var obj map[string]any
	json.Unmarshal(result, &obj)

	if obj["color"] != "blue" {
		t.Errorf("color = %v, want blue", obj["color"])
	}
}
