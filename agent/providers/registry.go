package providers

import (
	"context"
	"fmt"
	"sort"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// Provider describes vendor defaults and the protocols the vendor supports.
// It is immutable after registration.
type Provider struct {
	ID              ProviderID
	Name            string
	DefaultBaseURL  string
	DefaultProtocol ProtocolID
	Protocols       []ProtocolID
}

// ProtocolAdapter hides one wire protocol behind the agent model contract.
// Implementations must clone all mutable config they retain.
type ProtocolAdapter interface {
	ID() ProtocolID
	New(context.Context, ModelConfig) (agent.ToolCallingChatModel, error)
}

// Registry owns explicit provider and protocol registrations. It has no
// package-global state, making alternative catalogs straightforward to test or
// embed.
type Registry struct {
	providers map[ProviderID]registeredProvider
	protocols map[ProtocolID]ProtocolAdapter
}

type registeredProvider struct {
	definition Provider
	supported  map[ProtocolID]struct{}
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[ProviderID]registeredProvider),
		protocols: make(map[ProtocolID]ProtocolAdapter),
	}
}

// RegisterProvider adds one provider definition and rejects ambiguous catalog
// entries early.
func (registry *Registry) RegisterProvider(provider Provider) error {
	if registry == nil {
		return fmt.Errorf("register provider: nil registry")
	}
	provider.ID = ProviderID(strings.TrimSpace(string(provider.ID)))
	provider.Name = strings.TrimSpace(provider.Name)
	provider.DefaultBaseURL = strings.TrimSpace(provider.DefaultBaseURL)
	provider.DefaultProtocol = ProtocolID(strings.TrimSpace(string(provider.DefaultProtocol)))
	if provider.ID == "" {
		return fmt.Errorf("register provider: id is required")
	}
	if provider.Name == "" {
		return fmt.Errorf("register provider %q: name is required", provider.ID)
	}
	if _, exists := registry.providers[provider.ID]; exists {
		return fmt.Errorf("register provider %q: duplicate id", provider.ID)
	}

	supported := make(map[ProtocolID]struct{}, len(provider.Protocols))
	protocols := make([]ProtocolID, 0, len(provider.Protocols))
	for _, protocol := range provider.Protocols {
		protocol = ProtocolID(strings.TrimSpace(string(protocol)))
		if protocol == "" {
			return fmt.Errorf("register provider %q: empty protocol", provider.ID)
		}
		if _, duplicate := supported[protocol]; duplicate {
			return fmt.Errorf("register provider %q: duplicate protocol %q", provider.ID, protocol)
		}
		supported[protocol] = struct{}{}
		protocols = append(protocols, protocol)
	}
	if len(protocols) == 0 {
		return fmt.Errorf("register provider %q: at least one protocol is required", provider.ID)
	}
	if _, ok := supported[provider.DefaultProtocol]; !ok {
		return fmt.Errorf("register provider %q: default protocol %q is not supported", provider.ID, provider.DefaultProtocol)
	}
	sort.Slice(protocols, func(i, j int) bool { return protocols[i] < protocols[j] })
	provider.Protocols = protocols
	registry.providers[provider.ID] = registeredProvider{definition: provider, supported: supported}
	return nil
}

// RegisterProtocol adds one protocol implementation.
func (registry *Registry) RegisterProtocol(adapter ProtocolAdapter) error {
	if registry == nil {
		return fmt.Errorf("register protocol: nil registry")
	}
	if adapter == nil {
		return fmt.Errorf("register protocol: adapter is required")
	}
	id := ProtocolID(strings.TrimSpace(string(adapter.ID())))
	if id == "" {
		return fmt.Errorf("register protocol: adapter id is required")
	}
	if _, exists := registry.protocols[id]; exists {
		return fmt.Errorf("register protocol %q: duplicate id", id)
	}
	registry.protocols[id] = adapter
	return nil
}

// Resolve applies provider defaults and validates that the selected protocol
// has both catalog support and an installed adapter.
func (registry *Registry) Resolve(config ModelConfig) (ModelConfig, error) {
	if registry == nil {
		return ModelConfig{}, fmt.Errorf("resolve model: nil registry")
	}
	resolved, err := cloneModelConfig(config)
	if err != nil {
		return ModelConfig{}, fmt.Errorf("resolve model: %w", err)
	}
	resolved.Provider = ProviderID(strings.TrimSpace(string(resolved.Provider)))
	resolved.Protocol = ProtocolID(strings.TrimSpace(string(resolved.Protocol)))
	if resolved.Provider == "" {
		return ModelConfig{}, fmt.Errorf("resolve model: provider is required")
	}
	provider, ok := registry.providers[resolved.Provider]
	if !ok {
		return ModelConfig{}, fmt.Errorf("resolve model: provider %q is not registered", resolved.Provider)
	}
	if resolved.Protocol == "" {
		resolved.Protocol = provider.definition.DefaultProtocol
	}
	if _, ok := provider.supported[resolved.Protocol]; !ok {
		return ModelConfig{}, fmt.Errorf("resolve model: provider %q does not support protocol %q", resolved.Provider, resolved.Protocol)
	}
	if _, ok := registry.protocols[resolved.Protocol]; !ok {
		return ModelConfig{}, fmt.Errorf("resolve model: protocol %q has no registered adapter", resolved.Protocol)
	}
	if resolved.BaseURL == "" {
		resolved.BaseURL = provider.definition.DefaultBaseURL
	}
	if resolved.BaseURL == "" {
		return ModelConfig{}, fmt.Errorf("resolve model: provider %q requires a base URL", resolved.Provider)
	}
	if resolved.Model == "" {
		return ModelConfig{}, fmt.Errorf("resolve model: model is required")
	}
	if resolved.MaxOutputTokens != nil && *resolved.MaxOutputTokens <= 0 {
		return ModelConfig{}, fmt.Errorf("resolve model: max output tokens must be positive")
	}
	if err := validateOutputFormat(resolved.OutputFormat); err != nil {
		return ModelConfig{}, fmt.Errorf("resolve model: %w", err)
	}
	return resolved, nil
}

// NewChatModel resolves config and dispatches to exactly one protocol adapter.
func (registry *Registry) NewChatModel(ctx context.Context, config ModelConfig) (agent.ToolCallingChatModel, error) {
	resolved, err := registry.Resolve(config)
	if err != nil {
		return nil, err
	}
	model, err := registry.protocols[resolved.Protocol].New(ctx, resolved)
	if err != nil {
		return nil, fmt.Errorf("create %s model for provider %s: %w", resolved.Protocol, resolved.Provider, err)
	}
	return model, nil
}

func validateOutputFormat(format *OutputFormat) error {
	if format == nil {
		return nil
	}
	switch format.Type {
	case "", OutputFormatText, OutputFormatJSONObject:
		return nil
	case OutputFormatJSONSchema:
		if strings.TrimSpace(format.Name) == "" {
			return fmt.Errorf("JSON Schema output name is required")
		}
		if format.Schema == nil {
			return fmt.Errorf("JSON Schema output schema is required")
		}
		return nil
	default:
		return fmt.Errorf("unsupported output format %q", format.Type)
	}
}
