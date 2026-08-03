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
// adapters and optional provider presets. Presets only supply conveniences;
// callers may still configure arbitrary provider IDs and routes.
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
