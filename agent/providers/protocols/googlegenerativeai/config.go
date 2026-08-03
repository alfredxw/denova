package googlegenerativeai

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/genai"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/providers/protocols/internal/llmhttp"
)

type Adapter struct{}

func NewAdapter() *Adapter { return &Adapter{} }

func (*Adapter) ID() providers.ProtocolID { return providers.ProtocolGoogleGenerativeAI }

type ChatModel struct {
	client        *genai.Client
	config        providers.ModelConfig
	compatibility Compatibility
	options       *agent.Options
}

var (
	_ providers.ProtocolAdapter  = (*Adapter)(nil)
	_ agent.ToolCallingChatModel = (*ChatModel)(nil)
)

func (*Adapter) New(ctx context.Context, config providers.ModelConfig) (agent.ToolCallingChatModel, error) {
	if config.Protocol != providers.ProtocolGoogleGenerativeAI {
		return nil, fmt.Errorf("google generative AI: protocol must be %q", providers.ProtocolGoogleGenerativeAI)
	}
	if strings.TrimSpace(string(config.Provider)) == "" {
		return nil, fmt.Errorf("google generative AI: provider is required")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("google generative AI: model is required")
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		return nil, fmt.Errorf("google generative AI: base URL is required")
	}
	cloned, err := config.Clone()
	if err != nil {
		return nil, fmt.Errorf("google generative AI config: %w", err)
	}
	compatibility, err := resolveCompatibility(cloned)
	if err != nil {
		return nil, err
	}
	headers := make(http.Header, len(cloned.Headers))
	for name, value := range cloned.Headers {
		headers.Set(name, value)
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:     cloned.APIKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: llmhttp.Client(cloned.HTTPClient),
		HTTPOptions: genai.HTTPOptions{
			BaseURL:    cloned.BaseURL,
			APIVersion: compatibility.APIVersion,
			Headers:    headers,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("google generative AI client: %w", err)
	}
	return &ChatModel{
		client:        client,
		config:        cloned,
		compatibility: compatibility,
		options:       &agent.Options{},
	}, nil
}
