// Package openaiclient builds the shared OpenAI SDK transport options used by
// both Chat Completions and Responses adapters.
package openaiclient

import (
	"sort"

	"github.com/openai/openai-go/v3/option"

	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/providers/protocols/internal/llmhttp"
)

// Options returns deterministic client options. Explicit headers are applied
// last so custom endpoints can replace the SDK's default Authorization header.
func Options(config providers.ModelConfig) []option.RequestOption {
	result := []option.RequestOption{option.WithAPIKey(config.APIKey)}
	if config.BaseURL != "" {
		result = append(result, option.WithBaseURL(config.BaseURL))
	}
	result = append(result, option.WithHTTPClient(llmhttp.Client(config.HTTPClient)))
	headerNames := make([]string, 0, len(config.Headers))
	for name := range config.Headers {
		headerNames = append(headerNames, name)
	}
	sort.Strings(headerNames)
	for _, name := range headerNames {
		result = append(result, option.WithHeader(name, config.Headers[name]))
	}
	return result
}
