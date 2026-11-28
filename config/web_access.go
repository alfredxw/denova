package config

import (
	"fmt"
	"strings"
)

const (
	DefaultWebSearchMaxResults             = 10
	MaxWebSearchMaxResults                 = 20
	DefaultWebSearchProviderTimeoutSeconds = 10
	DefaultWebFetchMaxResponseKB           = 8 * 1024
	MaxWebFetchMaxResponseKB               = 64 * 1024
	DefaultWebFetchMaxContentChars         = 256 * 1024
	MaxWebFetchMaxContentChars             = 256 * 1024
)

// WebAccessSettings is the persisted user-level web access configuration.
// Pointer limits preserve the layered-settings distinction between inheritance
// and an explicit value. Workspace settings intentionally cannot override it.
type WebAccessSettings struct {
	SearchProvider               string                 `toml:"search_provider,omitempty" json:"search_provider,omitempty"`
	SearchFallback               string                 `toml:"search_fallback,omitempty" json:"search_fallback,omitempty"`
	DeepSeek                     *SearchServiceSettings `toml:"deepseek,omitempty" json:"deepseek,omitempty"`
	Perplexity                   *SearchServiceSettings `toml:"perplexity,omitempty" json:"perplexity,omitempty"`
	Tavily                       *SearchServiceSettings `toml:"tavily,omitempty" json:"tavily,omitempty"`
	Keenable                     *SearchServiceSettings `toml:"keenable,omitempty" json:"keenable,omitempty"`
	SearXNGBaseURL               string                 `toml:"searxng_base_url,omitempty" json:"searxng_base_url,omitempty"`
	SearchMaxResults             *int                   `toml:"search_max_results,omitempty" json:"search_max_results,omitempty"`
	SearchProviderTimeoutSeconds *int                   `toml:"search_provider_timeout_seconds,omitempty" json:"search_provider_timeout_seconds,omitempty"`
	FetchMaxResponseKB           *int                   `toml:"fetch_max_response_kb,omitempty" json:"fetch_max_response_kb,omitempty"`
	FetchMaxContentChars         *int                   `toml:"fetch_max_content_chars,omitempty" json:"fetch_max_content_chars,omitempty"`
}

// WebAccessConfig is the resolved runtime configuration consumed by the
// web-access module. Data-size limits are positive and bounded; a zero provider
// timeout explicitly allows an unlimited external search request.
type WebAccessConfig struct {
	SearchProvider               string                 `toml:"search_provider,omitempty" json:"search_provider,omitempty"`
	SearchFallback               string                 `toml:"search_fallback,omitempty" json:"search_fallback,omitempty"`
	DeepSeek                     *SearchServiceSettings `toml:"deepseek,omitempty" json:"deepseek,omitempty"`
	Perplexity                   *SearchServiceSettings `toml:"perplexity,omitempty" json:"perplexity,omitempty"`
	Tavily                       *SearchServiceSettings `toml:"tavily,omitempty" json:"tavily,omitempty"`
	Keenable                     *SearchServiceSettings `toml:"keenable,omitempty" json:"keenable,omitempty"`
	SearXNGBaseURL               string                 `toml:"searxng_base_url" json:"searxng_base_url"`
	SearchMaxResults             int                    `toml:"search_max_results" json:"search_max_results"`
	SearchProviderTimeoutSeconds int                    `toml:"search_provider_timeout_seconds" json:"search_provider_timeout_seconds"`
	FetchMaxResponseKB           int                    `toml:"fetch_max_response_kb" json:"fetch_max_response_kb"`
	FetchMaxContentChars         int                    `toml:"fetch_max_content_chars" json:"fetch_max_content_chars"`
}

// SearchServiceSettings stores one account per hosted search service. Mode is
// service-specific: Perplexity web/fast, Tavily basic/advanced; DeepSeek and
// Keenable have none. Keenable's API key is optional (keyless endpoint).
// Presence represents a saved service, including an unfinished configuration.
type SearchServiceSettings struct {
	APIKey string `toml:"api_key,omitempty" json:"api_key,omitempty"`
	Mode   string `toml:"mode,omitempty" json:"mode,omitempty"`
}

// Validate permits unfinished credentials while rejecting unknown routing and
// billing modes. An unfinished service must never prevent Agent/Fetch startup.
func (settings WebAccessSettings) Validate() error {
	switch settings.SearchProvider {
	case "", "builtin", "searxng", "deepseek", "perplexity", "tavily", "keenable":
	default:
		return fmt.Errorf("unsupported search provider %q", settings.SearchProvider)
	}
	switch settings.SearchFallback {
	case "", "none", "builtin":
	default:
		return fmt.Errorf("unsupported search fallback %q", settings.SearchFallback)
	}
	for _, service := range []struct {
		name     string
		settings *SearchServiceSettings
		modes    []string
	}{
		{"deepseek", settings.DeepSeek, []string{""}},
		{"perplexity", settings.Perplexity, []string{"", "web", "fast"}},
		{"tavily", settings.Tavily, []string{"", "basic", "advanced"}},
		{"keenable", settings.Keenable, []string{""}},
	} {
		if service.settings == nil {
			continue
		}
		valid := false
		for _, mode := range service.modes {
			valid = valid || service.settings.Mode == mode
		}
		if !valid {
			return fmt.Errorf("unsupported %s search mode %q", service.name, service.settings.Mode)
		}
	}
	return nil
}

func DefaultWebAccessConfig() WebAccessConfig {
	return WebAccessConfig{
		SearchMaxResults:             DefaultWebSearchMaxResults,
		SearchProviderTimeoutSeconds: DefaultWebSearchProviderTimeoutSeconds,
		FetchMaxResponseKB:           DefaultWebFetchMaxResponseKB,
		FetchMaxContentChars:         DefaultWebFetchMaxContentChars,
	}
}

func DefaultWebAccessSettings() WebAccessSettings {
	defaults := DefaultWebAccessConfig()
	return WebAccessSettings{
		SearchMaxResults:             intPtr(defaults.SearchMaxResults),
		SearchProviderTimeoutSeconds: intPtr(defaults.SearchProviderTimeoutSeconds),
		FetchMaxResponseKB:           intPtr(defaults.FetchMaxResponseKB),
		FetchMaxContentChars:         intPtr(defaults.FetchMaxContentChars),
	}
}

func MergeWebAccessSettings(parent, child WebAccessSettings) WebAccessSettings {
	out := parent
	if child.SearchProvider != "" {
		out.SearchProvider = child.SearchProvider
	}
	if child.SearchFallback != "" {
		out.SearchFallback = child.SearchFallback
	}
	if child.DeepSeek != nil {
		out.DeepSeek = child.DeepSeek
	}
	if child.Perplexity != nil {
		out.Perplexity = child.Perplexity
	}
	if child.Tavily != nil {
		out.Tavily = child.Tavily
	}
	if child.Keenable != nil {
		out.Keenable = child.Keenable
	}
	if strings.TrimSpace(child.SearXNGBaseURL) != "" {
		out.SearXNGBaseURL = child.SearXNGBaseURL
	}
	if child.SearchMaxResults != nil {
		out.SearchMaxResults = child.SearchMaxResults
	}
	if child.SearchProviderTimeoutSeconds != nil {
		out.SearchProviderTimeoutSeconds = child.SearchProviderTimeoutSeconds
	}
	if child.FetchMaxResponseKB != nil {
		out.FetchMaxResponseKB = child.FetchMaxResponseKB
	}
	if child.FetchMaxContentChars != nil {
		out.FetchMaxContentChars = child.FetchMaxContentChars
	}
	return out
}

func ResolveWebAccessSettings(settings WebAccessSettings) WebAccessConfig {
	settings = sanitizeWebAccessSettings(settings)
	defaults := DefaultWebAccessConfig()
	return WebAccessConfig{
		SearchProvider: settings.SearchProvider, SearchFallback: settings.SearchFallback,
		DeepSeek: settings.DeepSeek, Perplexity: settings.Perplexity, Tavily: settings.Tavily, Keenable: settings.Keenable,
		SearXNGBaseURL:               settings.SearXNGBaseURL,
		SearchMaxResults:             boundedSettingsInt(settings.SearchMaxResults, defaults.SearchMaxResults, 1, MaxWebSearchMaxResults),
		SearchProviderTimeoutSeconds: settingsNonNegativeInt(settings.SearchProviderTimeoutSeconds, defaults.SearchProviderTimeoutSeconds),
		FetchMaxResponseKB:           boundedSettingsInt(settings.FetchMaxResponseKB, defaults.FetchMaxResponseKB, 1, MaxWebFetchMaxResponseKB),
		FetchMaxContentChars:         boundedSettingsInt(settings.FetchMaxContentChars, defaults.FetchMaxContentChars, 1, MaxWebFetchMaxContentChars),
	}
}

// ResolveWebAccessConfig normalizes a runtime value assembled directly in
// tests or by a host, filling omitted fields with the same persisted defaults.
func ResolveWebAccessConfig(runtime WebAccessConfig) WebAccessConfig {
	return ResolveWebAccessSettings(settingsFromWebAccessConfig(runtime))
}

func settingsFromWebAccessConfig(runtime WebAccessConfig) WebAccessSettings {
	settings := WebAccessSettings{
		SearXNGBaseURL: strings.TrimSpace(runtime.SearXNGBaseURL),
		SearchProvider: runtime.SearchProvider, SearchFallback: runtime.SearchFallback,
		DeepSeek: runtime.DeepSeek, Perplexity: runtime.Perplexity, Tavily: runtime.Tavily, Keenable: runtime.Keenable,
	}
	if runtime.SearchMaxResults > 0 {
		settings.SearchMaxResults = intPtr(runtime.SearchMaxResults)
	}
	if runtime.SearchProviderTimeoutSeconds >= 0 {
		settings.SearchProviderTimeoutSeconds = intPtr(runtime.SearchProviderTimeoutSeconds)
	}
	if runtime.FetchMaxResponseKB > 0 {
		settings.FetchMaxResponseKB = intPtr(runtime.FetchMaxResponseKB)
	}
	if runtime.FetchMaxContentChars > 0 {
		settings.FetchMaxContentChars = intPtr(runtime.FetchMaxContentChars)
	}
	return sanitizeWebAccessSettings(settings)
}

func sanitizeWebAccessSettings(settings WebAccessSettings) WebAccessSettings {
	settings.SearchProvider = strings.TrimSpace(settings.SearchProvider)
	settings.SearchFallback = strings.TrimSpace(settings.SearchFallback)
	for _, service := range []**SearchServiceSettings{&settings.DeepSeek, &settings.Perplexity, &settings.Tavily, &settings.Keenable} {
		if *service != nil {
			copy := **service
			copy.APIKey = strings.TrimSpace(copy.APIKey)
			copy.Mode = strings.TrimSpace(copy.Mode)
			*service = &copy
		}
	}
	settings.SearXNGBaseURL = normalizeSearXNGBaseURL(settings.SearXNGBaseURL)
	settings.SearchMaxResults = normalizeBoundedPositiveInt(settings.SearchMaxResults, MaxWebSearchMaxResults)
	settings.SearchProviderTimeoutSeconds = normalizeNonNegativeInt(settings.SearchProviderTimeoutSeconds)
	settings.FetchMaxResponseKB = normalizeBoundedPositiveInt(settings.FetchMaxResponseKB, MaxWebFetchMaxResponseKB)
	settings.FetchMaxContentChars = normalizeBoundedPositiveInt(settings.FetchMaxContentChars, MaxWebFetchMaxContentChars)
	return settings
}

func normalizeSearXNGBaseURL(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" || strings.Contains(value, "://") {
		return value
	}
	if strings.HasPrefix(value, "//") {
		return "https:" + value
	}
	return "https://" + value
}

func normalizeNonNegativeInt(value *int) *int {
	if value == nil || *value < 0 {
		return nil
	}
	return value
}

func settingsNonNegativeInt(value *int, fallback int) int {
	if value == nil || *value < 0 {
		return fallback
	}
	return *value
}

func normalizeBoundedPositiveInt(value *int, maximum int) *int {
	if value == nil || *value <= 0 {
		return nil
	}
	if *value > maximum {
		bounded := maximum
		return &bounded
	}
	return value
}

func boundedSettingsInt(value *int, fallback, minimum, maximum int) int {
	if value == nil {
		return fallback
	}
	if *value < minimum {
		return minimum
	}
	if *value > maximum {
		return maximum
	}
	return *value
}
