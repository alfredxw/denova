// Package openaichatcompletions adapts OpenAI-compatible Chat Completions
// endpoints to the provider-neutral agent model contract.
package openaichatcompletions

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/openai/openai-go/v3"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/providers/protocols/internal/openaiclient"
	"github.com/alfredxw/denova/agent/providers/protocols/openaichatcompletions/internal/compat"
)

// Adapter constructs models that speak OpenAI Chat Completions.
type Adapter struct{}

// NewAdapter returns a stateless Chat Completions adapter.
func NewAdapter() *Adapter { return &Adapter{} }

// ID implements providers.ProtocolAdapter.
func (*Adapter) ID() providers.ProtocolID { return providers.ProtocolOpenAIChatCompletions }

// ChatModel implements agent.ToolCallingChatModel against Chat Completions.
type ChatModel struct {
	client        sdk.Client
	config        providers.ModelConfig
	compatibility Compatibility
	extraFields   map[string]any
	options       *agent.Options
}

var (
	_ providers.ProtocolAdapter  = (*Adapter)(nil)
	_ agent.ToolCallingChatModel = (*ChatModel)(nil)
)

// New constructs a Chat Completions model and applies compatibility repairs
// inside the protocol boundary. Construction performs no network request and
// adds no timeout or iteration limit.
func (*Adapter) New(_ context.Context, config providers.ModelConfig) (agent.ToolCallingChatModel, error) {
	if config.Provider == "" {
		return nil, fmt.Errorf("openai chat completions: provider is required")
	}
	if config.Protocol != providers.ProtocolOpenAIChatCompletions {
		return nil, fmt.Errorf("openai chat completions: protocol must be %q", providers.ProtocolOpenAIChatCompletions)
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("openai chat completions: model is required")
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		return nil, fmt.Errorf("openai chat completions: base URL is required")
	}
	cloned, err := config.Clone()
	if err != nil {
		return nil, fmt.Errorf("openai chat completions config: %w", err)
	}
	compatibility, err := resolveCompatibility(cloned)
	if err != nil {
		return nil, err
	}
	compatConfig := compat.Config{
		Model:                 cloned.Model,
		RepairTextToolCalls:   compatibility.RepairTextToolCalls,
		RepairInlineThinking:  compatibility.RepairInlineThinking,
		RequestReasoningSplit: compatibility.RequestReasoningSplit,
	}
	cloned.HTTPClient = compat.WrapHTTPClient(cloned.HTTPClient)

	extraFields := make(map[string]any, len(compatibility.ExtraBody)+1)
	for key, value := range compatibility.ExtraBody {
		extraFields[key] = value
	}
	for key, value := range compat.ExtraRequestFields(compatConfig) {
		extraFields[key] = value
	}
	model := &ChatModel{
		client:        sdk.NewClient(openaiclient.Options(cloned)...),
		config:        cloned,
		compatibility: compatibility,
		extraFields:   extraFields,
		options:       &agent.Options{},
	}
	return compat.Wrap(model, compatConfig), nil
}
