package config

import "testing"

func clearProviderEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ATLASCLOUD_API_KEY", "")
	t.Setenv("ATLASCLOUD_API_BASE", "")
	t.Setenv("ATLASCLOUD_BASE_URL", "")
	t.Setenv("ATLASCLOUD_MODEL", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("OPENAI_MODEL", "")
}

func TestOverrideFromEnvSupportsAtlasCloudShortcut(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("ATLASCLOUD_API_KEY", "atlas-key")

	cfg := &Config{
		OpenAIAPIKey:  "deepseek-key",
		OpenAIBaseURL: "https://api.deepseek.com",
		OpenAIModel:   "deepseek-v4-pro",
	}

	overrideFromEnv(cfg)

	if cfg.OpenAIAPIKey != "atlas-key" {
		t.Fatalf("OpenAIAPIKey = %q, want atlas key", cfg.OpenAIAPIKey)
	}
	if cfg.OpenAIBaseURL != AtlasCloudAPIBaseURL {
		t.Fatalf("OpenAIBaseURL = %q, want Atlas Cloud base URL", cfg.OpenAIBaseURL)
	}
	if cfg.OpenAIModel != AtlasCloudDefaultModel {
		t.Fatalf("OpenAIModel = %q, want Atlas Cloud default model", cfg.OpenAIModel)
	}
}

func TestOpenAIEnvOverridesAtlasCloudShortcut(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("ATLASCLOUD_API_KEY", "atlas-key")
	t.Setenv("ATLASCLOUD_MODEL", "qwen/qwen3.5-flash")
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("OPENAI_BASE_URL", "https://proxy.example/v1")
	t.Setenv("OPENAI_MODEL", "custom-model")

	cfg := &Config{}

	overrideFromEnv(cfg)

	if cfg.OpenAIAPIKey != "openai-key" {
		t.Fatalf("OpenAIAPIKey = %q, want explicit OpenAI key", cfg.OpenAIAPIKey)
	}
	if cfg.OpenAIBaseURL != "https://proxy.example/v1" {
		t.Fatalf("OpenAIBaseURL = %q, want explicit OpenAI base URL", cfg.OpenAIBaseURL)
	}
	if cfg.OpenAIModel != "custom-model" {
		t.Fatalf("OpenAIModel = %q, want explicit OpenAI model", cfg.OpenAIModel)
	}
}

func TestAtlasCloudShortcutAcceptsBaseAndModelOverrides(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv("ATLASCLOUD_API_KEY", "atlas-key")
	t.Setenv("ATLASCLOUD_API_BASE", "https://atlas-proxy.example/v1")
	t.Setenv("ATLASCLOUD_MODEL", "deepseek-ai/deepseek-v4-pro")

	cfg := &Config{}

	overrideFromEnv(cfg)

	if cfg.OpenAIAPIKey != "atlas-key" {
		t.Fatalf("OpenAIAPIKey = %q, want atlas key", cfg.OpenAIAPIKey)
	}
	if cfg.OpenAIBaseURL != "https://atlas-proxy.example/v1" {
		t.Fatalf("OpenAIBaseURL = %q, want Atlas Cloud override base URL", cfg.OpenAIBaseURL)
	}
	if cfg.OpenAIModel != "deepseek-ai/deepseek-v4-pro" {
		t.Fatalf("OpenAIModel = %q, want Atlas Cloud override model", cfg.OpenAIModel)
	}
}
