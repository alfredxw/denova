package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/providers/builtin"
)

var ErrRuntimeModelProfile = errors.New("runtime API model profile is unavailable or incompatible")

// ResolveRuntimeModel resolves an explicit API profile for one execution. It
// never falls back to a default profile or CLI account. The caller must keep
// the returned credentials in memory and out of journals, logs and responses.
// CLI runtimes own sampling and protocol behavior; only routing is reused.
func ResolveRuntimeModel(cfg *Config, selection RuntimeSelection) (ResolvedModelSettings, error) {
	id := selection.ModelProfileID()
	if cfg == nil || id == "" || !ModelProfileExists(cfg, id) {
		return ResolvedModelSettings{}, ErrRuntimeModelProfile
	}
	resolved, err := ResolveModelProfile(cfg, ModelProfileSettings{ID: id})
	if err != nil {
		return ResolvedModelSettings{}, ErrRuntimeModelProfile
	}
	// Adapter-specific routing cannot silently disappear in a different runtime.
	if len(resolved.ProtocolOptions) != 0 || (resolved.SessionKeyMapping != nil && resolved.SessionKeyMapping.Location != providers.SessionKeyLocationNone) {
		return ResolvedModelSettings{}, ErrRuntimeModelProfile
	}
	registry, err := builtin.NewRegistry()
	if err != nil {
		return ResolvedModelSettings{}, err
	}
	provider := providers.ProviderID(resolved.Provider)
	if provider == "" {
		provider = providers.ProviderOpenAICompatible
	}
	route, err := registry.Resolve(providers.ModelConfig{Provider: provider, Protocol: providers.ProtocolID(resolved.Protocol), BaseURL: resolved.BaseURL, Model: resolved.Model, APIKey: resolved.APIKey, Headers: resolved.Headers})
	if err != nil {
		return ResolvedModelSettings{}, ErrRuntimeModelProfile
	}
	want := providers.ProtocolOpenAIResponses
	if selection.Kind == RuntimeClaude {
		want = providers.ProtocolAnthropicMessages
	}
	if route.Protocol != want {
		return ResolvedModelSettings{}, ErrRuntimeModelProfile
	}
	u, err := url.Parse(route.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ResolvedModelSettings{}, ErrRuntimeModelProfile
	}
	for key, value := range route.Headers {
		if strings.ContainsAny(key+value, "\r\n\x00") {
			return ResolvedModelSettings{}, ErrRuntimeModelProfile
		}
	}
	if strings.ContainsAny(route.APIKey, "\r\n\x00") {
		return ResolvedModelSettings{}, ErrRuntimeModelProfile
	}
	resolved.Protocol, resolved.BaseURL, resolved.Headers = string(route.Protocol), strings.TrimRight(route.BaseURL, "/"), route.Headers
	if strings.TrimSpace(resolved.Model) == "" {
		return ResolvedModelSettings{}, fmt.Errorf("%w: model is empty", ErrRuntimeModelProfile)
	}
	return resolved, nil
}
