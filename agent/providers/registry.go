package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

// EndpointPreset supplies optional defaults for one provider/protocol route.
type EndpointPreset struct {
	BaseURL           string             `json:"base_url,omitempty"`
	Headers           map[string]string  `json:"-"`
	ProtocolOptions   json.RawMessage    `json:"-"`
	SessionKeyMapping *SessionKeyMapping `json:"-"`
}

// ProviderPreset is the registered provider identity exposed to callers and
// the Settings catalog, together with optional route defaults.
type ProviderPreset struct {
	ID              ProviderID                    `json:"id"`
	Name            string                        `json:"name"`
	DefaultProtocol ProtocolID                    `json:"default_protocol"`
	Endpoints       map[ProtocolID]EndpointPreset `json:"endpoints"`
}

// ProtocolAdapter hides one wire protocol behind the agent model contract.
// Implementations must clone all mutable config they retain and strictly
// validate their own ProtocolOptions schema.
type ProtocolAdapter interface {
	ID() ProtocolID
	New(context.Context, ModelConfig) (agent.ToolCallingChatModel, error)
}

// ModelInfo is one provider-advertised model that can be offered as a
// suggestion. Callers must continue accepting model IDs not present here.
type ModelInfo struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by,omitempty"`
}

// ModelListingAdapter is an optional protocol capability. An adapter should
// implement it only when its wire protocol defines an OpenAI-compatible model
// listing endpoint.
type ModelListingAdapter interface {
	ListModels(context.Context, ModelConfig) ([]ModelInfo, error)
}

// ErrModelListingUnsupported reports that a protocol has no optional model
// discovery capability. It does not make custom model configuration invalid.
var ErrModelListingUnsupported = errors.New("model listing is not supported by this protocol")

// Catalog is the immutable user-facing provider/protocol inventory. Providers
// are selectable identities and protocols are executable adapters.
type Catalog struct {
	Providers []ProviderPreset `json:"providers"`
	Protocols []ProtocolID     `json:"protocols"`
}

// Registry owns the closed provider catalog and executable protocol adapters.
// Registered providers may still select any installed protocol explicitly.
type Registry struct {
	presets   map[ProviderID]ProviderPreset
	protocols map[ProtocolID]ProtocolAdapter
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		presets:   make(map[ProviderID]ProviderPreset),
		protocols: make(map[ProtocolID]ProtocolAdapter),
	}
}

// RegisterProviderPreset adds one selectable provider and its known defaults.
func (registry *Registry) RegisterProviderPreset(preset ProviderPreset) error {
	if registry == nil {
		return fmt.Errorf("register provider preset: nil registry")
	}
	preset.ID = ProviderID(strings.TrimSpace(string(preset.ID)))
	preset.Name = strings.TrimSpace(preset.Name)
	preset.DefaultProtocol = ProtocolID(strings.TrimSpace(string(preset.DefaultProtocol)))
	if preset.ID == "" {
		return fmt.Errorf("register provider preset: id is required")
	}
	if preset.Name == "" {
		return fmt.Errorf("register provider preset %q: name is required", preset.ID)
	}
	if preset.DefaultProtocol == "" {
		return fmt.Errorf("register provider preset %q: default protocol is required", preset.ID)
	}
	if _, exists := registry.presets[preset.ID]; exists {
		return fmt.Errorf("register provider preset %q: duplicate id", preset.ID)
	}

	endpoints := make(map[ProtocolID]EndpointPreset, len(preset.Endpoints))
	for protocol, endpoint := range preset.Endpoints {
		protocol = ProtocolID(strings.TrimSpace(string(protocol)))
		if protocol == "" {
			return fmt.Errorf("register provider preset %q: empty endpoint protocol", preset.ID)
		}
		endpoint.BaseURL = strings.TrimSpace(endpoint.BaseURL)
		endpoint.Headers = cloneHeaders(endpoint.Headers)
		endpoint.ProtocolOptions = append(json.RawMessage(nil), endpoint.ProtocolOptions...)
		var err error
		endpoint.SessionKeyMapping, err = normalizeSessionKeyMapping(endpoint.SessionKeyMapping)
		if err != nil {
			return fmt.Errorf("register provider preset %q protocol %q: %w", preset.ID, protocol, err)
		}
		if _, err := mergeProtocolOptions(endpoint.ProtocolOptions, nil); err != nil {
			return fmt.Errorf("register provider preset %q protocol %q: %w", preset.ID, protocol, err)
		}
		endpoints[protocol] = endpoint
	}
	if _, ok := endpoints[preset.DefaultProtocol]; !ok {
		return fmt.Errorf("register provider preset %q: default protocol %q has no endpoint preset", preset.ID, preset.DefaultProtocol)
	}
	preset.Endpoints = endpoints
	registry.presets[preset.ID] = preset
	return nil
}

// RegisterProtocol adds one executable protocol adapter.
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

// Catalog returns detached, stable-sorted provider presets and installed
// protocol IDs for settings UIs and configuration tools.
func (registry *Registry) Catalog() Catalog {
	if registry == nil {
		return Catalog{}
	}
	result := Catalog{
		Providers: make([]ProviderPreset, 0, len(registry.presets)),
		Protocols: make([]ProtocolID, 0, len(registry.protocols)),
	}
	for _, preset := range registry.presets {
		result.Providers = append(result.Providers, cloneProviderPreset(preset))
	}
	for protocol := range registry.protocols {
		result.Protocols = append(result.Protocols, protocol)
	}
	sort.Slice(result.Providers, func(i, j int) bool { return result.Providers[i].Name < result.Providers[j].Name })
	sort.Slice(result.Protocols, func(i, j int) bool { return result.Protocols[i] < result.Protocols[j] })
	return result
}

// Resolve applies optional provider/endpoint defaults, then validates only the
// executable protocol seam and the complete route required by its adapter.
func (registry *Registry) Resolve(config ModelConfig) (ModelConfig, error) {
	return registry.resolve(config, true)
}

func (registry *Registry) resolve(config ModelConfig, requireModel bool) (ModelConfig, error) {
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
	preset, knownProvider := registry.presets[resolved.Provider]
	if !knownProvider {
		return ModelConfig{}, fmt.Errorf("resolve model: provider %q has no registered preset", resolved.Provider)
	}
	if resolved.Protocol == "" {
		resolved.Protocol = preset.DefaultProtocol
	}
	if _, ok := registry.protocols[resolved.Protocol]; !ok {
		return ModelConfig{}, fmt.Errorf("resolve model: protocol %q has no registered adapter", resolved.Protocol)
	}
	if endpoint, ok := preset.Endpoints[resolved.Protocol]; ok {
		if resolved.BaseURL == "" {
			resolved.BaseURL = endpoint.BaseURL
		}
		resolved.Headers = mergeHeaders(endpoint.Headers, resolved.Headers)
		if resolved.SessionKeyMapping == nil {
			resolved.SessionKeyMapping = cloneSessionKeyMapping(endpoint.SessionKeyMapping)
		}
		resolved.ProtocolOptions, err = mergeProtocolOptions(endpoint.ProtocolOptions, resolved.ProtocolOptions)
		if err != nil {
			return ModelConfig{}, fmt.Errorf("resolve model: %w", err)
		}
	}
	if resolved.BaseURL == "" {
		return ModelConfig{}, fmt.Errorf("resolve model: provider %q protocol %q requires a base URL", resolved.Provider, resolved.Protocol)
	}
	if requireModel && resolved.Model == "" {
		return ModelConfig{}, fmt.Errorf("resolve model: model is required")
	}
	if resolved.MaxOutputTokens == nil {
		if limits, ok := LookupModelLimits(resolved.Provider, resolved.Model); ok && limits.MaxOutputTokens > 0 {
			maxOutputTokens := limits.MaxOutputTokens
			resolved.MaxOutputTokens = &maxOutputTokens
		}
	}
	if resolved.MaxOutputTokens != nil && *resolved.MaxOutputTokens <= 0 {
		return ModelConfig{}, fmt.Errorf("resolve model: max output tokens must be positive")
	}
	if err := validateOutputFormat(resolved.OutputFormat); err != nil {
		return ModelConfig{}, fmt.Errorf("resolve model: %w", err)
	}
	return resolved, nil
}

// ListModelsWithResolvedConfig resolves provider defaults, then uses the
// protocol's optional discovery capability. The returned models are
// suggestions and never constrain ModelConfig.Model validation.
func (registry *Registry) ListModelsWithResolvedConfig(ctx context.Context, config ModelConfig) ([]ModelInfo, ModelConfig, error) {
	resolved, err := registry.resolve(config, false)
	if err != nil {
		return nil, ModelConfig{}, err
	}
	adapter, ok := registry.protocols[resolved.Protocol].(ModelListingAdapter)
	if !ok {
		return nil, resolved, fmt.Errorf("list models for protocol %q: %w", resolved.Protocol, ErrModelListingUnsupported)
	}
	models, err := adapter.ListModels(ctx, resolved)
	if err != nil {
		return nil, resolved, fmt.Errorf("list models for provider %s protocol %s: %w", resolved.Provider, resolved.Protocol, err)
	}
	return models, resolved, nil
}

// NewChatModel resolves config and dispatches to exactly one protocol adapter.
func (registry *Registry) NewChatModel(ctx context.Context, config ModelConfig) (agent.ToolCallingChatModel, error) {
	model, _, err := registry.NewChatModelWithResolvedConfig(ctx, config)
	return model, err
}

// NewChatModelWithResolvedConfig resolves the effective route and returns it
// alongside the model. Diagnostics such as connection validation use this to
// report the exact preset defaults that were exercised without resolving the
// same configuration twice.
func (registry *Registry) NewChatModelWithResolvedConfig(ctx context.Context, config ModelConfig) (agent.ToolCallingChatModel, ModelConfig, error) {
	resolved, err := registry.Resolve(config)
	if err != nil {
		return nil, ModelConfig{}, err
	}
	model, err := registry.protocols[resolved.Protocol].New(ctx, resolved)
	if err != nil {
		return nil, ModelConfig{}, fmt.Errorf("create %s model for provider %s: %w", resolved.Protocol, resolved.Provider, err)
	}
	return model, resolved, nil
}

func cloneProviderPreset(preset ProviderPreset) ProviderPreset {
	clone := preset
	clone.Endpoints = make(map[ProtocolID]EndpointPreset, len(preset.Endpoints))
	for protocol, endpoint := range preset.Endpoints {
		endpoint.Headers = cloneHeaders(endpoint.Headers)
		endpoint.ProtocolOptions = append(json.RawMessage(nil), endpoint.ProtocolOptions...)
		endpoint.SessionKeyMapping = cloneSessionKeyMapping(endpoint.SessionKeyMapping)
		clone.Endpoints[protocol] = endpoint
	}
	return clone
}

func mergeHeaders(defaults, overrides map[string]string) map[string]string {
	if len(defaults) == 0 && len(overrides) == 0 {
		return nil
	}
	result := make(map[string]string, len(defaults)+len(overrides))
	canonical := make(map[string]string, len(defaults)+len(overrides))
	for name, value := range defaults {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		key := http.CanonicalHeaderKey(trimmed)
		result[key] = value
		canonical[strings.ToLower(key)] = key
	}
	for name, value := range overrides {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		key := http.CanonicalHeaderKey(trimmed)
		if previous, ok := canonical[strings.ToLower(key)]; ok && previous != key {
			delete(result, previous)
		}
		result[key] = value
		canonical[strings.ToLower(key)] = key
	}
	return result
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
