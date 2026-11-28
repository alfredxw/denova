package webaccess

import (
	"errors"
	"fmt"
	"time"

	"denova/config"
)

// ConfigFromSettings projects the user configuration into the shared Agent and
// settings-preview boundary. An empty selection preserves pre-service-list
// SearXNG routing, including environment-provided endpoints.
func ConfigFromSettings(value config.WebAccessConfig) Config {
	return Config{
		SearchProvider: value.SearchProvider, SearchFallback: value.SearchFallback,
		DeepSeek: value.DeepSeek, Perplexity: value.Perplexity, Tavily: value.Tavily, Keenable: value.Keenable,
		SearXNGBaseURL:        value.SearXNGBaseURL,
		SearchMaxResults:      value.SearchMaxResults,
		SearchProviderTimeout: time.Duration(value.SearchProviderTimeoutSeconds) * time.Second,
		FetchMaxResponseBytes: int64(value.FetchMaxResponseKB) * 1024,
		FetchMaxContentChars:  value.FetchMaxContentChars,
	}
}

// searchFailure carries a stable code for localized product errors. Its English
// message is safe for Agent feedback and logs: never include upstream bodies,
// credentials, or the user's query.
type searchFailure struct {
	code    string
	message string
}

func (err *searchFailure) Error() string { return err.message }

func searchError(code, message string) error { return &searchFailure{code: code, message: message} }

// SearchErrorCode maps failures to the finite product error vocabulary.
func SearchErrorCode(err error) string {
	var failure *searchFailure
	if errors.As(err, &failure) {
		return failure.code
	}
	return "unavailable"
}

func canFallbackAfter(err error) bool {
	switch SearchErrorCode(err) {
	case "unavailable", "rate_limit":
		return true
	default:
		return false
	}
}

func (client *Client) configureSearch() {
	settings := config.WebAccessSettings{
		SearchProvider: client.config.SearchProvider, SearchFallback: client.config.SearchFallback,
		DeepSeek: client.config.DeepSeek, Perplexity: client.config.Perplexity, Tavily: client.config.Tavily, Keenable: client.config.Keenable,
	}
	if err := settings.Validate(); err != nil {
		client.searchConfigError = searchError("configuration", "Invalid search service configuration. Check the selected service and mode.")
		return
	}
	// Legacy SearXNG also falls back on empty results. New explicit selections
	// fall back only on transient failures and only when the user opts in.
	client.legacySearchFallback = client.config.SearchProvider == "" && client.config.SearchFallback == ""
	if client.config.SearchProvider == "" {
		client.config.SearchProvider = ProviderBuiltin
		if client.config.SearXNGBaseURL != "" {
			client.config.SearchProvider = ProviderSearXNG
		}
	}
	if client.config.SearchFallback == "" {
		client.config.SearchFallback = "none"
		if client.legacySearchFallback {
			client.config.SearchFallback = ProviderBuiltin
		}
	}
}

func (client *Client) selectedService() *config.SearchServiceSettings {
	switch client.config.SearchProvider {
	case ProviderDeepSeek:
		return client.config.DeepSeek
	case ProviderPerplexity:
		return client.config.Perplexity
	case ProviderTavily:
		return client.config.Tavily
	case ProviderKeenable:
		return client.config.Keenable
	default:
		return nil
	}
}

func missingSearchService(provider string) error {
	return searchError("configuration", fmt.Sprintf("Configure the %s search service in Settings before searching.", provider))
}
