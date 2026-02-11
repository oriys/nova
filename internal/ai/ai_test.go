package ai

import "testing"

func TestConfigFromStoreMap(t *testing.T) {
	base := DefaultConfig()

	t.Run("empty map returns base", func(t *testing.T) {
		got := ConfigFromStoreMap(base, map[string]string{})
		if got != base {
			t.Errorf("expected base config, got %+v", got)
		}
	})

	t.Run("overrides base_url", func(t *testing.T) {
		m := map[string]string{"ai_base_url": "https://custom.example.com/v1"}
		got := ConfigFromStoreMap(base, m)
		if got.BaseURL != "https://custom.example.com/v1" {
			t.Errorf("expected custom base_url, got %s", got.BaseURL)
		}
		// Other fields unchanged
		if got.Model != base.Model {
			t.Errorf("expected model %s, got %s", base.Model, got.Model)
		}
	})

	t.Run("overrides all fields", func(t *testing.T) {
		m := map[string]string{
			"ai_enabled":  "true",
			"ai_api_key":  "sk-test",
			"ai_model":    "gpt-4",
			"ai_base_url": "https://custom.example.com/v1",
		}
		got := ConfigFromStoreMap(base, m)
		if !got.Enabled {
			t.Error("expected enabled=true")
		}
		if got.APIKey != "sk-test" {
			t.Errorf("expected api_key sk-test, got %s", got.APIKey)
		}
		if got.Model != "gpt-4" {
			t.Errorf("expected model gpt-4, got %s", got.Model)
		}
		if got.BaseURL != "https://custom.example.com/v1" {
			t.Errorf("expected custom base_url, got %s", got.BaseURL)
		}
	})

	t.Run("empty values do not override", func(t *testing.T) {
		m := map[string]string{
			"ai_api_key":  "",
			"ai_model":    "",
			"ai_base_url": "",
		}
		got := ConfigFromStoreMap(base, m)
		if got.Model != base.Model {
			t.Errorf("empty model should not override, got %s", got.Model)
		}
		if got.BaseURL != base.BaseURL {
			t.Errorf("empty base_url should not override, got %s", got.BaseURL)
		}
	})

	t.Run("enabled with 1", func(t *testing.T) {
		m := map[string]string{"ai_enabled": "1"}
		got := ConfigFromStoreMap(base, m)
		if !got.Enabled {
			t.Error("expected enabled=true for value '1'")
		}
	})

	t.Run("does not mutate base", func(t *testing.T) {
		original := DefaultConfig()
		m := map[string]string{"ai_base_url": "https://other.example.com"}
		_ = ConfigFromStoreMap(original, m)
		if original.BaseURL != "https://api.openai.com/v1" {
			t.Errorf("base config was mutated: %s", original.BaseURL)
		}
	})
}
