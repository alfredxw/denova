package codex

import (
	"reflect"
	"strings"
	"testing"

	"denova/config"
	"github.com/pelletier/go-toml/v2"
)

func TestAPIChildLaunchKeepsSecretsOutOfArguments(t *testing.T) {
	base := []string{"PATH=tools", "openai_api_key=old", "OPENAI_BASE_URL=https://wrong.invalid", "DENOVA_RUNTIME_HEADER_8=stale", "HTTPS_PROXY=http://proxy"}
	before := append([]string(nil), base...)
	args, env := apiLaunch([]string{"app-server"}, base, config.ResolvedModelSettings{BaseURL: "https://example.test/v1", APIKey: "new-secret", Headers: map[string]string{"X-Tenant": "header-secret"}})
	if !reflect.DeepEqual(base, before) {
		t.Fatal("mutated parent environment")
	}
	if strings.Contains(strings.Join(args, " "), "secret") {
		t.Fatal("credential leaked to arguments")
	}
	child := strings.Join(env, "\n")
	if strings.Contains(child, "stale") || strings.Contains(child, "=old") || strings.Contains(child, "wrong.invalid") || !strings.Contains(child, "DENOVA_RUNTIME_API_KEY=new-secret") || !strings.Contains(child, "HEADER_0=header-secret") {
		t.Fatal("child routing was not isolated")
	}
	var parsed struct {
		ModelProvider  string `toml:"model_provider"`
		ModelProviders map[string]struct {
			BaseURL string            `toml:"base_url"`
			EnvKey  string            `toml:"env_key"`
			Auth    bool              `toml:"requires_openai_auth"`
			Headers map[string]string `toml:"env_http_headers"`
		} `toml:"model_providers"`
	}
	if err := toml.Unmarshal([]byte(args[len(args)-3]+"\n"+args[len(args)-1]), &parsed); err != nil {
		t.Fatal(err)
	}
	provider := parsed.ModelProviders[parsed.ModelProvider]
	if provider.BaseURL != "https://example.test/v1" || provider.EnvKey != "DENOVA_RUNTIME_API_KEY" || provider.Auth || provider.Headers["X-Tenant"] != "DENOVA_RUNTIME_HEADER_0" {
		t.Fatal("invalid provider override")
	}
}
