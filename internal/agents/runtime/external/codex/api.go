package codex

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"denova/config"
)

// apiLaunch overrides only this process. No credentials appear in argv or in
// the user's shared config; a dedicated provider cannot reuse stored login.
func apiLaunch(args, environment []string, model config.ResolvedModelSettings) ([]string, []string) {
	env := make([]string, 0, len(environment)+len(model.Headers)+1)
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "DENOVA_RUNTIME_") || upper == "OPENAI_BASE_URL" || upper == "OPENAI_API_KEY" || upper == "CODEX_API_KEY" {
			continue
		}
		env = append(env, entry)
	}
	provider := map[string]string{"name": "Denova", "base_url": model.BaseURL, "wire_api": "responses"}
	if model.APIKey != "" {
		provider["env_key"] = "DENOVA_RUNTIME_API_KEY"
		env = append(env, "DENOVA_RUNTIME_API_KEY="+model.APIKey)
	}
	headers := map[string]string{}
	names := make([]string, 0, len(model.Headers))
	for key := range model.Headers {
		names = append(names, key)
	}
	sort.Strings(names)
	for i, name := range names {
		variable := fmt.Sprintf("DENOVA_RUNTIME_HEADER_%d", i)
		headers[name] = variable
		env = append(env, variable+"="+model.Headers[name])
	}
	// Codex recursively merges provider tables with user configuration. A fresh
	// identity prevents inherited headers, auth helpers and query parameters.
	providerID := "denova-" + rand.Text()
	var fields []string
	quote := func(value string) string { encoded, _ := json.Marshal(value); return string(encoded) }
	for _, key := range []string{"name", "base_url", "wire_api", "env_key"} {
		if value, ok := provider[key]; ok {
			fields = append(fields, key+"="+quote(value))
		}
	}
	fields = append(fields, "requires_openai_auth=false")
	var headerFields []string
	for _, name := range names {
		headerFields = append(headerFields, quote(name)+"="+quote(headers[name]))
	}
	fields = append(fields, "env_http_headers={"+strings.Join(headerFields, ",")+"}")
	args = append(args, "-c", "model_provider="+quote(providerID), "-c", "model_providers."+providerID+"={"+strings.Join(fields, ",")+"}")
	return args, env
}
