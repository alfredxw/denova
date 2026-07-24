package config

import "strings"

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
	SearXNGBaseURL               string `toml:"searxng_base_url,omitempty" json:"searxng_base_url,omitempty"`
	SearchMaxResults             *int   `toml:"search_max_results,omitempty" json:"search_max_results,omitempty"`
	SearchProviderTimeoutSeconds *int   `toml:"search_provider_timeout_seconds,omitempty" json:"search_provider_timeout_seconds,omitempty"`
	FetchMaxResponseKB           *int   `toml:"fetch_max_response_kb,omitempty" json:"fetch_max_response_kb,omitempty"`
	FetchMaxContentChars         *int   `toml:"fetch_max_content_chars,omitempty" json:"fetch_max_content_chars,omitempty"`
}

// WebAccessConfig is the resolved runtime configuration consumed by the
// web-access module. Data-size limits are positive and bounded; a zero provider
// timeout explicitly allows an unlimited external search request.
type WebAccessConfig struct {
	SearXNGBaseURL               string `toml:"searxng_base_url" json:"searxng_base_url"`
	SearchMaxResults             int    `toml:"search_max_results" json:"search_max_results"`
	SearchProviderTimeoutSeconds int    `toml:"search_provider_timeout_seconds" json:"search_provider_timeout_seconds"`
	FetchMaxResponseKB           int    `toml:"fetch_max_response_kb" json:"fetch_max_response_kb"`
	FetchMaxContentChars         int    `toml:"fetch_max_content_chars" json:"fetch_max_content_chars"`
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
	settings := WebAccessSettings{SearXNGBaseURL: strings.TrimSpace(runtime.SearXNGBaseURL)}
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
