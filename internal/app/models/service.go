// Package models owns model discovery and connection validation. It shares the
// same provider registry and profile resolution path as Agent execution.
package models

import (
	"context"
	"errors"
	"fmt"

	"denova/config"
	"denova/internal/agents/modelio"
)

// Host provides an immutable configuration snapshot for one validation call.
type Host interface {
	ModelConfigSnapshot() config.Config
}

type Service struct {
	host    Host
	runtime *modelio.Runtime
}

func NewService(host Host) *Service {
	return &Service{host: host, runtime: modelio.NewRuntime()}
}

type PingResult struct {
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latency_ms"`
	Provider  string `json:"provider"`
	Protocol  string `json:"protocol"`
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
}

type ModelInfo struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by,omitempty"`
}

type ListResult struct {
	Models   []ModelInfo `json:"models"`
	Provider string      `json:"provider"`
	Protocol string      `json:"protocol"`
	BaseURL  string      `json:"base_url"`
}

// ProviderRequestError distinguishes a valid model configuration whose real
// upstream request failed from a malformed settings draft.
type ProviderRequestError struct{ cause error }

func (err *ProviderRequestError) Error() string {
	if err == nil || err.cause == nil {
		return "model provider request failed"
	}
	return err.cause.Error()
}

func (err *ProviderRequestError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func IsProviderRequestError(err error) bool {
	var target *ProviderRequestError
	return errors.As(err, &target)
}

func (service *Service) Catalog() (modelio.Catalog, error) {
	if service == nil || service.runtime == nil {
		return modelio.Catalog{}, fmt.Errorf("model catalog: service is unavailable")
	}
	return service.runtime.Catalog()
}

// List returns optional OpenAI-compatible /models results as suggestions. A
// synthetic ID lets a new profile discover models before its custom model
// name has been entered; normal Agent resolution still requires a model.
func (service *Service) List(ctx context.Context, profile config.ModelProfileSettings) (ListResult, error) {
	if service == nil || service.host == nil {
		return ListResult{}, fmt.Errorf("model list: service host is unavailable")
	}
	if service.runtime == nil {
		return ListResult{}, fmt.Errorf("model list: provider runtime is unavailable")
	}
	if profile.ID == "" && profile.Model == "" {
		profile.ID = "__model_discovery_draft__"
	}
	snapshot := service.host.ModelConfigSnapshot()
	resolvedProfile, err := config.ResolveModelProfile(&snapshot, profile)
	if err != nil {
		return ListResult{}, fmt.Errorf("model list: %w", err)
	}
	discovered, err := service.runtime.ListModels(ctx, resolvedProfile)
	if err != nil {
		if modelio.IsModelListRequestError(err) {
			return ListResult{}, &ProviderRequestError{cause: fmt.Errorf("model list: %w", err)}
		}
		return ListResult{}, fmt.Errorf("model list: %w", err)
	}
	models := make([]ModelInfo, 0, len(discovered.Models))
	for _, model := range discovered.Models {
		models = append(models, ModelInfo{ID: model.ID, OwnedBy: model.OwnedBy})
	}
	return ListResult{
		Models:   models,
		Provider: discovered.Provider,
		Protocol: discovered.Protocol,
		BaseURL:  discovered.BaseURL,
	}, nil
}

// Ping performs a minimal real generation. This validates routing, transport,
// authentication, model availability, protocol serialization, and response
// parsing rather than merely checking whether an HTTP server is reachable.
func (service *Service) Ping(ctx context.Context, profile config.ModelProfileSettings) (PingResult, error) {
	if service == nil || service.host == nil {
		return PingResult{}, fmt.Errorf("model ping: service host is unavailable")
	}
	if service.runtime == nil {
		return PingResult{}, fmt.Errorf("model ping: provider runtime is unavailable")
	}
	snapshot := service.host.ModelConfigSnapshot()
	resolvedProfile, err := config.ResolveModelProfile(&snapshot, profile)
	if err != nil {
		return PingResult{}, fmt.Errorf("model ping: %w", err)
	}
	probe, err := service.runtime.Probe(ctx, resolvedProfile)
	if err != nil {
		if modelio.IsProbeRequestError(err) {
			return PingResult{}, &ProviderRequestError{cause: fmt.Errorf("model ping: %w", err)}
		}
		return PingResult{}, fmt.Errorf("model ping: %w", err)
	}
	return PingResult{
		OK:        true,
		LatencyMS: probe.Latency.Milliseconds(),
		Provider:  probe.Provider,
		Protocol:  probe.Protocol,
		BaseURL:   probe.BaseURL,
		Model:     probe.Model,
	}, nil
}
