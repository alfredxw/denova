// Package providers defines Denova's provider-neutral model catalog and the
// registry seam used to select a wire protocol adapter.
package providers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/net/http/httpguts"
)

// ProviderID identifies a registered API vendor or compatibility preset.
type ProviderID string

// ProtocolID identifies the wire protocol used for model requests.
type ProtocolID string

// SessionKeyLocation identifies where a protocol adapter writes the
// provider-neutral per-call SessionKey.
type SessionKeyLocation string

const (
	ProviderOpenAI           ProviderID = "openai"
	ProviderDeepSeek         ProviderID = "deepseek"
	ProviderAnthropic        ProviderID = "anthropic"
	ProviderGoogle           ProviderID = "google"
	ProviderVolcengine       ProviderID = "volcengine"
	ProviderMiniMax          ProviderID = "minimax"
	ProviderMiniMaxCN        ProviderID = "minimax-cn"
	ProviderOpenAICompatible ProviderID = "openai-compatible"

	ProtocolOpenAIChatCompletions ProtocolID = "openai-chat-completions"
	ProtocolOpenAIResponses       ProtocolID = "openai-responses"
	ProtocolAnthropicMessages     ProtocolID = "anthropic-messages"

	SessionKeyLocationNone   SessionKeyLocation = "none"
	SessionKeyLocationHeader SessionKeyLocation = "header"
	SessionKeyLocationBody   SessionKeyLocation = "body"
)

// SessionKeyMapping configures the provider wire field for SessionKey. A nil
// mapping inherits the provider preset; Location=none explicitly disables it.
// Header and body mappings write one top-level field named by Name.
type SessionKeyMapping struct {
	Location SessionKeyLocation `toml:"location" json:"location"`
	Name     string             `toml:"name,omitempty" json:"name,omitempty"`
}

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
	// SessionKeyMapping maps the provider-neutral per-call SessionKey to the
	// selected endpoint. Nil inherits the provider endpoint preset.
	SessionKeyMapping *SessionKeyMapping
	// HTTPClient is an optional caller-owned transport dependency. Adapters may
	// retain it, so callers must not mutate the client after registration.
	HTTPClient  *http.Client
	Temperature *float32
	// MaxOutputTokens is an internal request capability resolved by the provider
	// registry or a bounded operation. It is not a user Profile preference.
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
	var err error
	clone.SessionKeyMapping, err = normalizeSessionKeyMapping(config.SessionKeyMapping)
	if err != nil {
		return ModelConfig{}, err
	}
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

func normalizeSessionKeyMapping(mapping *SessionKeyMapping) (*SessionKeyMapping, error) {
	if mapping == nil {
		return nil, nil
	}
	clone := *mapping
	clone.Location = SessionKeyLocation(strings.ToLower(strings.TrimSpace(string(clone.Location))))
	clone.Name = strings.TrimSpace(clone.Name)
	switch clone.Location {
	case SessionKeyLocationNone:
		clone.Name = ""
	case SessionKeyLocationHeader:
		if clone.Name == "" {
			return nil, fmt.Errorf("session key header name is required")
		}
		if len(clone.Name) > 128 || !httpguts.ValidHeaderFieldName(clone.Name) {
			return nil, fmt.Errorf("invalid session key header name %q", clone.Name)
		}
		clone.Name = http.CanonicalHeaderKey(clone.Name)
	case SessionKeyLocationBody:
		if !validSessionKeyBodyFieldName(clone.Name) {
			return nil, fmt.Errorf("invalid session key body field name %q", clone.Name)
		}
	default:
		return nil, fmt.Errorf("unsupported session key mapping location %q", clone.Location)
	}
	return &clone, nil
}

func cloneSessionKeyMapping(mapping *SessionKeyMapping) *SessionKeyMapping {
	if mapping == nil {
		return nil
	}
	clone := *mapping
	return &clone
}

func validSessionKeyBodyFieldName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for _, char := range name {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
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
