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
	level, err := ParseThinkingLevel(string(clone.ThinkingLevel))
	if err != nil {
		return ModelConfig{}, err
	}
	clone.ThinkingLevel = level
	return clone, nil
}
