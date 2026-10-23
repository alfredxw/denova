package grok

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestPrivateConfigurationPreservesRoutingAndExcludesExecutableSources(t *testing.T) {
	home := t.TempDir()
	config := `[model.custom]
model = "custom-id"
base_url = "http://127.0.0.1:1234/v1"
env_key = "TEST_MODEL_KEY"
agent_type = "unsafe-harness"
auth_provider = "shell-helper"
[model.custom.extra_headers]
X-Test = "header"
[model_providers.local]
base_url = "http://127.0.0.1:5678/v1"
api_key = "fixture-key"
[models]
default = "custom"
[mcp_servers.unscoped]
command = "unscoped-command"
[[hooks.PreToolUse]]
command = "hook-command"
[auth_provider.shell-helper]
command = "credential-command"
[plugins]
paths = ["plugin-path"]
`
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	body, custom, err := modelConfiguration([]string{"GROK_HOME=" + home, "TEST_MODEL_KEY=local-test"})
	if err != nil {
		t.Fatal(err)
	}
	if !custom {
		t.Fatal("custom API route was omitted")
	}
	for _, excluded := range []string{"unsafe-harness", "shell-helper", "unscoped-command", "hook-command", "credential-command", "plugin-path"} {
		if strings.Contains(string(body), excluded) {
			t.Fatalf("executable source imported: %s", excluded)
		}
	}
	var actual struct {
		Model map[string]struct {
			Model   string            `toml:"model"`
			BaseURL string            `toml:"base_url"`
			EnvKey  string            `toml:"env_key"`
			Headers map[string]string `toml:"extra_headers"`
		}
		Models struct{ Default string }
	}
	if err := toml.Unmarshal(body, &actual); err != nil {
		t.Fatal(err)
	}
	model := actual.Model["custom"]
	if model.Model != "custom-id" || model.BaseURL != "http://127.0.0.1:1234/v1" || model.EnvKey != "TEST_MODEL_KEY" || model.Headers["X-Test"] != "header" || actual.Models.Default != "custom" {
		t.Fatalf("routing changed: %+v", actual)
	}
	env := isolatedEnv([]string{"GROK_HOME=" + home, "GROK_AUTH_PATH=" + filepath.Join(home, "custom-auth.json"), "GROK_CONFIG=unscoped"}, "private-home", "")
	if environmentValue(env, "GROK_HOME") != "private-home" || environmentValue(env, "GROK_AUTH_PATH") != filepath.Join(home, "custom-auth.json") || environmentValue(env, "GROK_CONFIG") != "" {
		t.Fatalf("private environment lost credential location or isolation: %v", env)
	}
}

func TestNativeModelDisplayOverrideDoesNotImplyAuthentication(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("[model.grok-test]\nname=\"Test\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, custom, err := modelConfiguration([]string{"GROK_HOME=" + home})
	if err != nil {
		t.Fatal(err)
	}
	if custom {
		t.Fatal("display override was treated as configured API credentials")
	}
}

func TestExplicitConfigurationPathPreservesModelRestrictions(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(t.TempDir(), "models.toml")
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("[models]\ndefault = \"wrong\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	body := "[models]\ndefault = \"chosen\"\nallowed_models = [\"chosen\"]\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{"GROK_HOME=" + home, "GROK_CONFIG_PATH=" + path}
	encoded, _, err := modelConfiguration(env)
	if err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Models struct {
			Default string
			Allowed []string `toml:"allowed_models"`
		}
	}
	if err := toml.Unmarshal(encoded, &actual); err != nil {
		t.Fatal(err)
	}
	if actual.Models.Default != "chosen" || len(actual.Models.Allowed) != 1 || actual.Models.Allowed[0] != "chosen" {
		t.Fatalf("explicit configuration changed: %+v", actual)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := modelConfiguration(env); err == nil {
		t.Fatal("missing explicit configuration silently used defaults")
	}
	if _, _, err := modelConfiguration([]string{"GROK_HOME=" + t.TempDir()}); err != nil {
		t.Fatalf("missing optional default configuration: %v", err)
	}
}
