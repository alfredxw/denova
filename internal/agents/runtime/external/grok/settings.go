package grok

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

func environmentValue(env []string, name string) string {
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func originalHome(env []string) string {
	if home := environmentValue(env, "GROK_HOME"); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".grok")
}

// configPath resolves the configuration the native CLI actually uses.
// GROK_CONFIG_PATH can relocate it, so reading only ~/.grok/config.toml would
// hide models the user configured elsewhere.
func configPath(env []string) string {
	if path := strings.TrimSpace(environmentValue(env, "GROK_CONFIG_PATH")); path != "" {
		return path
	}
	return filepath.Join(originalHome(env), "config.toml")
}

// Import only inert model routing fields. Agent profiles, hooks, auth helper
// commands, MCP servers and plugins never enter the private execution home.
func modelConfiguration(env []string) ([]byte, bool, error) {
	body, err := os.ReadFile(configPath(env))
	if errors.Is(err, os.ErrNotExist) && strings.TrimSpace(environmentValue(env, "GROK_CONFIG_PATH")) == "" {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var source map[string]any
	if err := toml.Unmarshal(body, &source); err != nil {
		return nil, false, err
	}
	result := map[string]any{}
	fields := map[string]bool{"model": true, "name": true, "base_url": true, "api_base_url": true, "api_backend": true, "api_key": true, "env_key": true, "extra_headers": true, "temperature": true, "top_p": true, "context_window": true, "max_completion_tokens": true, "supports_reasoning_effort": true, "reasoning_effort": true, "reasoning_efforts": true, "model_provider": true}
	custom := false
	for _, section := range []string{"model", "model_providers"} {
		entries, _ := source[section].(map[string]any)
		filtered := map[string]any{}
		for name, raw := range entries {
			entry, _ := raw.(map[string]any)
			kept := map[string]any{}
			for key, value := range entry {
				if fields[key] {
					kept[key] = value
				}
			}
			if len(kept) > 0 {
				filtered[name] = kept
				for _, key := range []string{"base_url", "api_base_url", "api_key"} {
					if value, ok := kept[key].(string); ok && value != "" {
						custom = true
					}
				}
				if key, ok := kept["env_key"].(string); ok && environmentValue(env, key) != "" {
					custom = true
				}
			}
		}
		if len(filtered) > 0 {
			result[section] = filtered
		}
	}
	if models, ok := source["models"].(map[string]any); ok {
		selected := map[string]any{}
		// Preserve explicit model restrictions together with routing defaults.
		for _, name := range []string{"default", "default_reasoning_effort", "allowed_models"} {
			if value, ok := models[name]; ok {
				selected[name] = value
			}
		}
		if len(selected) > 0 {
			result["models"] = selected
		}
	}
	encoded, err := toml.Marshal(result)
	return encoded, custom, err
}
