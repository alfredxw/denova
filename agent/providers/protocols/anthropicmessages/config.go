package anthropicmessages

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/providers/protocols/internal/llmhttp"
)

type Adapter struct{}

func NewAdapter() *Adapter { return &Adapter{} }

func (*Adapter) ID() providers.ProtocolID { return providers.ProtocolAnthropicMessages }

type ChatModel struct {
	client        anthropic.Client
	config        providers.ModelConfig
	compatibility Compatibility
	options       *agent.Options
}

var (
	_ providers.ProtocolAdapter  = (*Adapter)(nil)
	_ agent.ToolCallingChatModel = (*ChatModel)(nil)
)

func (*Adapter) New(_ context.Context, config providers.ModelConfig) (agent.ToolCallingChatModel, error) {
	if config.Protocol != providers.ProtocolAnthropicMessages {
		return nil, fmt.Errorf("anthropic messages: protocol must be %q", providers.ProtocolAnthropicMessages)
	}
	if strings.TrimSpace(string(config.Provider)) == "" {
		return nil, fmt.Errorf("anthropic messages: provider is required")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("anthropic messages: model is required")
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		return nil, fmt.Errorf("anthropic messages: base URL is required")
	}
	cloned, err := config.Clone()
	if err != nil {
		return nil, fmt.Errorf("anthropic messages config: %w", err)
	}
	compatibility, err := resolveCompatibility(cloned)
	if err != nil {
		return nil, err
	}
	return &ChatModel{
		client:        anthropic.NewClient(anthropicClientOptions(cloned)...),
		config:        cloned,
		compatibility: compatibility,
		options:       &agent.Options{},
	}, nil
}

func anthropicClientOptions(config providers.ModelConfig) []option.RequestOption {
	result := []option.RequestOption{
		option.WithAPIKey(config.APIKey),
		option.WithBaseURL(config.BaseURL),
		option.WithHTTPClient(llmhttp.Client(config.HTTPClient)),
	}
	names := make([]string, 0, len(config.Headers))
	for name := range config.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result = append(result, option.WithHeader(name, config.Headers[name]))
	}
	return result
}
