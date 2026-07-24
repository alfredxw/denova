// Package openai adapts OpenAI-compatible Chat Completions endpoints to the
// provider-neutral agent core model contract.
package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	agent "github.com/alfredxw/denova/agent"
)

// ReasoningEffortLevel is passed through to OpenAI-compatible providers.
// Keeping it provider-defined rather than validating it here allows newer
// compatible endpoints to add values without an agent core release.
type ReasoningEffortLevel string

const (
	ReasoningEffortLevelNone    ReasoningEffortLevel = "none"
	ReasoningEffortLevelMinimal ReasoningEffortLevel = "minimal"
	ReasoningEffortLevelLow     ReasoningEffortLevel = "low"
	ReasoningEffortLevelMedium  ReasoningEffortLevel = "medium"
	ReasoningEffortLevelHigh    ReasoningEffortLevel = "high"
	ReasoningEffortLevelXHigh   ReasoningEffortLevel = "xhigh"
)

// ChatCompletionResponseFormatType identifies a Chat Completions response
// format without exposing an SDK-owned parameter type.
type ChatCompletionResponseFormatType string

const (
	ChatCompletionResponseFormatTypeJSONObject ChatCompletionResponseFormatType = "json_object"
	ChatCompletionResponseFormatTypeJSONSchema ChatCompletionResponseFormatType = "json_schema"
	ChatCompletionResponseFormatTypeText       ChatCompletionResponseFormatType = "text"
)

// ChatCompletionResponseFormat describes text, JSON object, or JSON Schema
// output. It is intentionally serialized through option.WithJSONSet so the
// adapter also works with OpenAI-compatible endpoints whose SDK schema lags.
type ChatCompletionResponseFormat struct {
	Type       ChatCompletionResponseFormatType        `json:"type,omitempty"`
	JSONSchema *ChatCompletionResponseFormatJSONSchema `json:"json_schema,omitempty"`
}

// ChatCompletionResponseFormatJSONSchema is the provider-visible structured
// output declaration. Schema may be any JSON-marshalable schema value.
type ChatCompletionResponseFormatJSONSchema struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Schema      any    `json:"schema"`
	Strict      bool   `json:"strict"`
}

// Config configures an immutable OpenAI-compatible Chat Completions model.
// Per-call agent core options may override MaxTokens and the bound tool surface.
type Config struct {
	APIKey          string
	Model           string
	BaseURL         string
	HTTPClient      *http.Client
	Temperature     *float32
	MaxTokens       *int
	ReasoningEffort ReasoningEffortLevel
	ResponseFormat  *ChatCompletionResponseFormat
	ExtraFields     map[string]any
}

// ChatModel implements agent.ToolCallingChatModel against Chat Completions.
type ChatModel struct {
	client  sdk.Client
	config  Config
	options *agent.Options
}

var _ agent.ToolCallingChatModel = (*ChatModel)(nil)

// New constructs a Chat Completions model. Construction performs no network
// request and does not add a timeout or iteration limit.
func New(_ context.Context, config *Config) (*ChatModel, error) {
	if config == nil {
		return nil, fmt.Errorf("openai chat model: config is required")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("openai chat model: model is required")
	}

	cloned, err := cloneConfig(*config)
	if err != nil {
		return nil, fmt.Errorf("openai chat model config: %w", err)
	}
	clientOptions := []option.RequestOption{option.WithAPIKey(cloned.APIKey)}
	if cloned.BaseURL != "" {
		clientOptions = append(clientOptions, option.WithBaseURL(cloned.BaseURL))
	}
	if cloned.HTTPClient != nil {
		clientOptions = append(clientOptions, option.WithHTTPClient(cloned.HTTPClient))
	}

	return &ChatModel{
		client:  sdk.NewClient(clientOptions...),
		config:  cloned,
		options: &agent.Options{},
	}, nil
}

func cloneConfig(config Config) (Config, error) {
	clone := config
	if config.Temperature != nil {
		value := *config.Temperature
		clone.Temperature = &value
	}
	if config.MaxTokens != nil {
		value := *config.MaxTokens
		clone.MaxTokens = &value
	}
	if config.ExtraFields != nil {
		data, err := json.Marshal(config.ExtraFields)
		if err != nil {
			return Config{}, fmt.Errorf("marshal extra fields: %w", err)
		}
		if err := json.Unmarshal(data, &clone.ExtraFields); err != nil {
			return Config{}, fmt.Errorf("clone extra fields: %w", err)
		}
	}
	if config.ResponseFormat != nil {
		data, err := json.Marshal(config.ResponseFormat)
		if err != nil {
			return Config{}, fmt.Errorf("marshal response format: %w", err)
		}
		clone.ResponseFormat = &ChatCompletionResponseFormat{}
		if err := json.Unmarshal(data, clone.ResponseFormat); err != nil {
			return Config{}, fmt.Errorf("clone response format: %w", err)
		}
	}
	return clone, nil
}
