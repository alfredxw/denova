// Package providers defines Denova's provider-neutral model catalog and the
// registry seam used to select a wire protocol adapter.
package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ProviderID identifies an API vendor or OpenAI-compatible endpoint family.
type ProviderID string

// ProtocolID identifies the wire protocol used for model requests.
type ProtocolID string

const (
	ProviderOpenAI           ProviderID = "openai"
	ProviderDeepSeek         ProviderID = "deepseek"
	ProviderOpenAICompatible ProviderID = "openai-compatible"

	ProtocolOpenAIChatCompletions ProtocolID = "openai-chat-completions"
	ProtocolOpenAIResponses       ProtocolID = "openai-responses"
	ProtocolAnthropicMessages     ProtocolID = "anthropic-messages"
	ProtocolGoogleGenerativeAI    ProtocolID = "google-generative-ai"
)

// OutputFormatType is the provider-neutral structured-output mode.
type OutputFormatType string

const (
	OutputFormatText       OutputFormatType = "text"
	OutputFormatJSONObject OutputFormatType = "json_object"
	OutputFormatJSONSchema OutputFormatType = "json_schema"
)

// OutputFormat describes plain text, JSON object, or JSON Schema output.
type OutputFormat struct {
	Type        OutputFormatType `json:"type"`
	Name        string           `json:"name,omitempty"`
	Description string           `json:"description,omitempty"`
	Schema      any              `json:"schema,omitempty"`
	Strict      bool             `json:"strict,omitempty"`
}

// ModelConfig is the only configuration accepted at the provider registry
// seam. It deliberately contains no protocol SDK values or arbitrary request
// fields. Protocol-specific compatibility stays inside adapters.
type ModelConfig struct {
	Provider ProviderID
	Protocol ProtocolID
	Model    string
	BaseURL  string
	APIKey   string
	// Headers contains explicit endpoint headers. Provider presets supply only
	// non-secret defaults; profile values win case-insensitively.
	Headers map[string]string
	// ProtocolOptions is an adapter-owned JSON object. The registry merges a
	// provider endpoint preset with the profile override, while only the selected
	// protocol adapter knows and validates its schema.
	ProtocolOptions json.RawMessage
	// HTTPClient is an optional caller-owned transport dependency. Adapters may
	// retain it, so callers must not mutate the client after registration.
	HTTPClient      *http.Client
	Temperature     *float32
	MaxOutputTokens *int
	ThinkingLevel   ThinkingLevel
	OutputFormat    *OutputFormat
}

// Clone returns a detached configuration safe for an adapter to retain.
func (config ModelConfig) Clone() (ModelConfig, error) {
	return cloneModelConfig(config)
}

func cloneModelConfig(config ModelConfig) (ModelConfig, error) {
	clone := config
	clone.Headers = cloneHeaders(config.Headers)
	clone.ProtocolOptions = append(json.RawMessage(nil), config.ProtocolOptions...)
	if config.Temperature != nil {
		value := *config.Temperature
		clone.Temperature = &value
	}
	if config.MaxOutputTokens != nil {
		value := *config.MaxOutputTokens
		clone.MaxOutputTokens = &value
	}
	if config.OutputFormat != nil {
		clone.OutputFormat = &OutputFormat{
			Type:        config.OutputFormat.Type,
			Name:        config.OutputFormat.Name,
			Description: config.OutputFormat.Description,
			Strict:      config.OutputFormat.Strict,
		}
		if config.OutputFormat.Schema != nil {
			data, err := json.Marshal(config.OutputFormat.Schema)
			if err != nil {
				return ModelConfig{}, fmt.Errorf("marshal output JSON Schema: %w", err)
			}
			if err := json.Unmarshal(data, &clone.OutputFormat.Schema); err != nil {
				return ModelConfig{}, fmt.Errorf("clone output JSON Schema: %w", err)
			}
		}
	}
	clone.Model = strings.TrimSpace(clone.Model)
	clone.BaseURL = strings.TrimSpace(clone.BaseURL)
	if len(clone.ProtocolOptions) != 0 {
		if !json.Valid(clone.ProtocolOptions) {
			return ModelConfig{}, fmt.Errorf("protocol options must be valid JSON")
		}
		var value any
		if err := json.Unmarshal(clone.ProtocolOptions, &value); err != nil {
			return ModelConfig{}, fmt.Errorf("decode protocol options: %w", err)
		}
		if _, ok := value.(map[string]any); !ok {
			return ModelConfig{}, fmt.Errorf("protocol options must be a JSON object")
		}
	}
	level, err := ParseThinkingLevel(string(clone.ThinkingLevel))
	if err != nil {
		return ModelConfig{}, err
	}
	clone.ThinkingLevel = level
	return clone, nil
}

func cloneHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	clone := make(map[string]string, len(headers))
	for name, value := range headers {
		clone[name] = value
	}
	return clone
}
