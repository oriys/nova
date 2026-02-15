package domain

import "testing"

func TestGatewayRoute_AuthStrategies(t *testing.T) {
	strategies := []string{"none", "inherit", "apikey", "jwt"}
	for _, s := range strategies {
		route := GatewayRoute{AuthStrategy: s}
		if route.AuthStrategy != s {
			t.Errorf("AuthStrategy = %q, want %q", route.AuthStrategy, s)
		}
	}
}

func TestGatewayRoute_EnabledDefault(t *testing.T) {
	route := GatewayRoute{}
	if route.Enabled {
		t.Error("default GatewayRoute should not be enabled")
	}
}

func TestRouteRateLimit_Values(t *testing.T) {
	rl := RouteRateLimit{
		RequestsPerSecond: 100.0,
		BurstSize:         200,
	}
	if rl.RequestsPerSecond != 100.0 {
		t.Errorf("RequestsPerSecond = %f, want 100.0", rl.RequestsPerSecond)
	}
	if rl.BurstSize != 200 {
		t.Errorf("BurstSize = %d, want 200", rl.BurstSize)
	}
}

func TestCORSConfig_WildcardOrigin(t *testing.T) {
	cors := CORSConfig{
		AllowOrigins: []string{"*"},
	}
	if len(cors.AllowOrigins) != 1 || cors.AllowOrigins[0] != "*" {
		t.Error("expected wildcard origin")
	}
	if cors.AllowCredentials {
		t.Error("wildcard origin should not have AllowCredentials set")
	}
}

func TestCORSConfig_SpecificOrigins(t *testing.T) {
	cors := CORSConfig{
		AllowOrigins:     []string{"https://example.com", "https://app.example.com"},
		AllowMethods:     []string{"GET", "POST"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           3600,
	}
	if len(cors.AllowOrigins) != 2 {
		t.Errorf("expected 2 origins, got %d", len(cors.AllowOrigins))
	}
	if cors.MaxAge != 3600 {
		t.Errorf("MaxAge = %d, want 3600", cors.MaxAge)
	}
}
