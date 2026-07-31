// Package openairesponses adapts OpenAI Responses endpoints to Denova's
// provider-neutral agent model contract.
package openairesponses

import (
	"context"
	"fmt"
	"strings"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

// Adapter constructs models that speak the OpenAI Responses protocol.
type Adapter struct{}

func NewAdapter() *Adapter { return &Adapter{} }

func (*Adapter) ID() providers.ProtocolID { return providers.ProtocolOpenAIResponses }

type ChatModel struct {
	client  sdk.Client
	config  providers.ModelConfig
	options *agent.Options
}

var (
	_ providers.ProtocolAdapter  = (*Adapter)(nil)
	_ agent.ToolCallingChatModel = (*ChatModel)(nil)
)

// New constructs a Responses model. It performs no network request and adds
// no timeout or iteration limit.
func (*Adapter) New(_ context.Context, config providers.ModelConfig) (agent.ToolCallingChatModel, error) {
	if config.Provider == "" {
		return nil, fmt.Errorf("openai responses: provider is required")
	}
	if config.Protocol != providers.ProtocolOpenAIResponses {
		return nil, fmt.Errorf("openai responses: protocol must be %q", providers.ProtocolOpenAIResponses)
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("openai responses: model is required")
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		return nil, fmt.Errorf("openai responses: base URL is required")
	}
	cloned, err := config.Clone()
	if err != nil {
		return nil, fmt.Errorf("openai responses config: %w", err)
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
