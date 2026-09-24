package claude

import (
	"reflect"
	"strings"
	"testing"

	"denova/config"
)

func TestAPIEnvironmentIsolatesCredentialsAndAuxiliaryModels(t *testing.T) {
	base := []string{"PATH=tools", "anthropic_auth_token=old-token", "ANTHROPIC_API_KEY=old-key", "ANTHROPIC_BASE_URL=https://wrong.invalid", "CLAUDE_CODE_USE_BEDROCK=1", "CLAUDE_CODE_OAUTH_TOKEN=old-oauth", "CLAUDE_CODE_SUBAGENT_MODEL=old-model"}
	before := append([]string(nil), base...)
	env := apiEnvironment(base, config.ResolvedModelSettings{BaseURL: "https://example.test/api/v1", APIKey: "new-key", Model: "gateway-model", Headers: map[string]string{"X-Tenant": "tenant", "Authorization": "Bearer custom-token"}})
	if !reflect.DeepEqual(base, before) {
		t.Fatal("mutated parent environment")
	}
	got := map[string]string{}
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		got[key] = value
	}
	if got["ANTHROPIC_BASE_URL"] != "https://example.test/api" || got["ANTHROPIC_API_KEY"] != "new-key" || got["ANTHROPIC_MODEL"] != "gateway-model" || got["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "gateway-model" {
		t.Fatal("incorrect endpoint routing")
	}
	if strings.Contains(strings.Join(env, "\n"), "old-") || got["CLAUDE_CODE_USE_BEDROCK"] != "" {
		t.Fatal("inherited another provider")
	}
	if got["ANTHROPIC_CUSTOM_HEADERS"] != "Authorization: Bearer custom-token\nX-Tenant: tenant" {
		t.Fatal("custom headers lost")
	}
	noKey := strings.Join(apiEnvironment(base, config.ResolvedModelSettings{BaseURL: "http://localhost", Model: "local"}), "\n")
	if !strings.Contains(noKey, "ANTHROPIC_AUTH_TOKEN=denova-local") {
		t.Fatal("unauthenticated endpoint could inherit subscription login")
	}
}
