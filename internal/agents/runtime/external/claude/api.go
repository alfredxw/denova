package claude

import (
	"sort"
	"strings"

	"denova/config"
)

// apiEnvironment builds a private routing environment. It removes inherited
// provider switches and credentials so a gateway cannot receive another key.
func apiEnvironment(base []string, model config.ResolvedModelSettings) []string {
	env := make([]string, 0, len(base)+8)
	for _, entry := range base {
		key, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "ANTHROPIC_") || strings.HasPrefix(upper, "CLAUDE_CODE_USE_") || strings.HasPrefix(upper, "CLAUDE_CODE_SKIP_") || strings.HasPrefix(upper, "CLAUDE_CODE_OAUTH_") || upper == "CLAUDE_CODE_API_KEY_HELPER_TTL_MS" || upper == "CLAUDE_CODE_SUBAGENT_MODEL" {
			continue
		}
		env = append(env, entry)
	}
	// Anthropic SDK appends /v1/messages; Denova also accepts /v1 roots.
	baseURL := strings.TrimSuffix(model.BaseURL, "/v1")
	env = append(env, "ANTHROPIC_BASE_URL="+baseURL, "ANTHROPIC_MODEL="+model.Model)
	// Auxiliary calls must use the selected endpoint model as well.
	for _, family := range []string{"OPUS", "SONNET", "HAIKU", "FABLE"} {
		env = append(env, "ANTHROPIC_DEFAULT_"+family+"_MODEL="+model.Model)
	}
	if model.APIKey != "" {
		env = append(env, "ANTHROPIC_API_KEY="+model.APIKey)
	} else {
		// Local unauthenticated endpoints must never inherit a stored OAuth login.
		env = append(env, "ANTHROPIC_AUTH_TOKEN=denova-local")
	}
	var names, headers []string
	for name := range model.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		headers = append(headers, name+": "+model.Headers[name])
	}
	if len(headers) > 0 {
		env = append(env, "ANTHROPIC_CUSTOM_HEADERS="+strings.Join(headers, "\n"))
	}
	return env
}
