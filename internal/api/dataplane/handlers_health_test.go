package dataplane

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthLive(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()

	h.HealthLive(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", body["status"])
	}
}

func TestParseRangeParam(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantRange  int
		wantBucket int
	}{
		{"empty", "", 3600, 120},
		{"1m", "1m", 60, 2},
		{"5m", "5m", 300, 10},
		{"1h", "1h", 3600, 120},
		{"24h", "24h", 86400, 2880},
		{"invalid", "invalid", 3600, 120},
		{"zero", "0m", 3600, 120},
		{"negative", "-5m", 3600, 120},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRange, gotBucket := parseRangeParam(tt.input)
			if gotRange != tt.wantRange {
				t.Errorf("parseRangeParam(%q) range = %d, want %d", tt.input, gotRange, tt.wantRange)
			}
			if gotBucket != tt.wantBucket {
				t.Errorf("parseRangeParam(%q) bucket = %d, want %d", tt.input, gotBucket, tt.wantBucket)
			}
		})
	}
}
