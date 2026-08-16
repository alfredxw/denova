package modelio

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/providers/builtin"

	"denova/config"
)

// Catalog is the stable application projection of installed protocol
// adapters and the closed provider preset catalog. Custom routes use the
// compatible provider preset with an explicit installed protocol.
type Catalog struct {
	Providers []ProviderPreset `json:"providers"`
	Protocols []string         `json:"protocols"`
}

type ProviderPreset struct {
	ID              string                    `json:"id"`
	Name            string                    `json:"name"`
	DefaultProtocol string                    `json:"default_protocol"`
	Endpoints       map[string]EndpointPreset `json:"endpoints"`
}

type EndpointPreset struct {
	BaseURL string `json:"base_url,omitempty"`
}

// ProbeResult identifies the effective route exercised by a successful
// provider connection probe.
type ProbeResult struct {
	Latency  time.Duration
	Provider string
	Protocol string
	BaseURL  string
	Model    string
}

// ModelListResult is a provider-advertised suggestion set for one effective
// protocol route. It never constrains custom model IDs.
type ModelListResult struct {
	Models   []providers.ModelInfo
	Provider string
	Protocol string
	BaseURL  string
}

// ProbeRequestError means configuration and adapter construction succeeded,
// but the actual provider request or its response failed.
type ProbeRequestError struct{ cause error }

func (err *ProbeRequestError) Error() string {
	if err == nil || err.cause == nil {
		return "model probe request failed"
	}
	return err.cause.Error()
}

func (err *ProbeRequestError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func IsProbeRequestError(err error) bool {
	var target *ProbeRequestError
	return errors.As(err, &target)
}

// ModelListRequestError means route resolution succeeded but the upstream
// endpoint rejected, omitted, or failed its optional /models request.
type ModelListRequestError struct{ cause error }

func (err *ModelListRequestError) Error() string {
	if err == nil || err.cause == nil {
		return "model listing request failed"
	}
	return err.cause.Error()
}

func (err *ModelListRequestError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func IsModelListRequestError(err error) bool {
	var target *ModelListRequestError
	return errors.As(err, &target)
}

// Runtime owns one immutable built-in registry. It is safe for concurrent use
// after lazy initialization and keeps all public Agent-module dependencies on
// the internal Agent side of the application boundary.
type Runtime struct {
	once     sync.Once
	registry *providers.Registry
	err      error
}

var defaultRuntime = NewRuntime()

func NewRuntime() *Runtime { return &Runtime{} }

func (runtime *Runtime) Catalog() (Catalog, error) {
	registry, err := runtime.providerRegistry()
	if err != nil {
		return Catalog{}, err
	}
	providerCatalog := registry.Catalog()
	catalog := Catalog{
		Providers: make([]ProviderPreset, 0, len(providerCatalog.Providers)),
		Protocols: make([]string, 0, len(providerCatalog.Protocols)),
	}
	for _, protocol := range providerCatalog.Protocols {
		catalog.Protocols = append(catalog.Protocols, string(protocol))
	}
	for _, preset := range providerCatalog.Providers {
		projected := ProviderPreset{
			ID:              string(preset.ID),
			Name:            preset.Name,
			DefaultProtocol: string(preset.DefaultProtocol),
			Endpoints:       make(map[string]EndpointPreset, len(preset.Endpoints)),
		}
		for protocol, endpoint := range preset.Endpoints {
			projected.Endpoints[string(protocol)] = EndpointPreset{BaseURL: endpoint.BaseURL}
		}
		catalog.Providers = append(catalog.Providers, projected)
	}
	return catalog, nil
}

// Probe performs a minimal real generation through the same resolver and
// adapter path as normal Agents. The caller context is the only lifetime
// bound; LLM requests do not receive an implicit timeout.
func (runtime *Runtime) Probe(ctx context.Context, resolved config.ResolvedModelSettings) (ProbeResult, error) {
	registry, err := runtime.providerRegistry()
	if err != nil {
		return ProbeResult{}, err
	}
	modelConfig, err := ConfigFromResolved(resolved)
	if err != nil {
		return ProbeResult{}, err
	}
	model, effective, err := registry.NewChatModelWithResolvedConfig(ctx, modelConfig)
	if err != nil {
		return ProbeResult{}, err
	}
	logBaseURL := loggableModelBaseURL(effective.BaseURL)

	slog.InfoContext(ctx, fmt.Sprintf(
		"[agents/modelio] model probe begin provider=%s protocol=%s model=%q base_url=%q",
		effective.Provider, effective.Protocol, effective.Model, logBaseURL,
	))
	startedAt := time.Now()
	response, err := model.Generate(
		ctx,
		[]*agent.Message{agent.UserMessage("Reply with exactly OK.")},
		agent.WithMaxTokens(16),
	)
	latency := time.Since(startedAt)
	if err != nil {
		slog.WarnContext(ctx, fmt.Sprintf(
			"[agents/modelio] model probe failed provider=%s protocol=%s model=%q base_url=%q latency_ms=%d err=%v",
			effective.Provider, effective.Protocol, effective.Model, logBaseURL, latency.Milliseconds(), err,
		))
		return ProbeResult{}, &ProbeRequestError{cause: fmt.Errorf("provider request: %w", err)}
	}
	if response == nil {
		err = fmt.Errorf("provider returned no response")
		slog.WarnContext(ctx, fmt.Sprintf(
			"[agents/modelio] model probe failed provider=%s protocol=%s model=%q base_url=%q latency_ms=%d err=%v",
			effective.Provider, effective.Protocol, effective.Model, logBaseURL, latency.Milliseconds(), err,
		))
		return ProbeResult{}, &ProbeRequestError{cause: err}
	}
	slog.InfoContext(ctx, fmt.Sprintf(
		"[agents/modelio] model probe succeeded provider=%s protocol=%s model=%q base_url=%q latency_ms=%d",
		effective.Provider, effective.Protocol, effective.Model, logBaseURL, latency.Milliseconds(),
	))
	return ProbeResult{
		Latency:  latency,
		Provider: string(effective.Provider),
		Protocol: string(effective.Protocol),
		BaseURL:  effective.BaseURL,
		Model:    effective.Model,
	}, nil
}

// ListModels calls the optional protocol discovery endpoint without imposing
// a request timeout. Unsupported protocols fail before any network request.
func (runtime *Runtime) ListModels(ctx context.Context, resolved config.ResolvedModelSettings) (ModelListResult, error) {
	registry, err := runtime.providerRegistry()
	if err != nil {
		return ModelListResult{}, err
	}
	modelConfig, err := ConfigFromResolved(resolved)
	if err != nil {
		return ModelListResult{}, err
	}
	models, effective, err := registry.ListModelsWithResolvedConfig(ctx, modelConfig)
	if err != nil {
		if errors.Is(err, providers.ErrModelListingUnsupported) {
			return ModelListResult{}, err
		}
		slog.WarnContext(ctx, fmt.Sprintf(
			"[agents/modelio] model listing failed provider=%s protocol=%s base_url=%q err=%v",
			effective.Provider, effective.Protocol, loggableModelBaseURL(effective.BaseURL), err,
		))
		return ModelListResult{}, &ModelListRequestError{cause: err}
	}
	slog.InfoContext(ctx, fmt.Sprintf(
		"[agents/modelio] model listing succeeded provider=%s protocol=%s base_url=%q count=%d",
		effective.Provider, effective.Protocol, loggableModelBaseURL(effective.BaseURL), len(models),
	))
	return ModelListResult{
		Models:   models,
		Provider: string(effective.Provider),
		Protocol: string(effective.Protocol),
		BaseURL:  effective.BaseURL,
	}, nil
}

func loggableModelBaseURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "<invalid>"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return parsed.String()
}

func (runtime *Runtime) NewChatModel(ctx context.Context, modelConfig providers.ModelConfig) (agent.ToolCallingChatModel, error) {
	registry, err := runtime.providerRegistry()
	if err != nil {
		return nil, err
	}
	return registry.NewChatModel(ctx, modelConfig)
}

func (runtime *Runtime) providerRegistry() (*providers.Registry, error) {
	if runtime == nil {
		return nil, fmt.Errorf("model provider runtime is nil")
	}
	runtime.once.Do(func() {
		runtime.registry, runtime.err = builtin.NewRegistry()
	})
	if runtime.err != nil {
		return nil, fmt.Errorf("create model provider registry: %w", runtime.err)
	}
	return runtime.registry, nil
}
