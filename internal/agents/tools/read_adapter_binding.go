package tools

import (
	"fmt"
	"strings"

	agenttools "github.com/alfredxw/denova/agent/tools"

	"denova/config"
)

// ReadAdapterBinding keeps each URI adapter attached to the capability that
// authorizes it. Workspace assembles one model-visible read tool after
// filtering these bindings against the resolved Agent policy.
type ReadAdapterBinding struct {
	capability string
	adapter    agenttools.ReadAdapter
}

// ReadAdapterFactory builds run-scoped adapter bindings after the Agent's
// effective capability policy is known.
type ReadAdapterFactory func(config.ResolvedAgentToolSettings) ([]ReadAdapterBinding, error)

func newReadAdapterBinding(capability string, adapter agenttools.ReadAdapter) (ReadAdapterBinding, error) {
	capability = strings.TrimSpace(capability)
	if capability == "" {
		return ReadAdapterBinding{}, fmt.Errorf("read adapter capability is required")
	}
	if adapter == nil {
		return ReadAdapterBinding{}, fmt.Errorf("read adapter for capability %q is nil", capability)
	}
	return ReadAdapterBinding{capability: capability, adapter: adapter}, nil
}

// NewReadAdapterBinding attaches an application-owned URI adapter to an
// existing tool capability. This keeps one model-visible read tool while
// allowing product layers to expose bounded resources such as trajectories.
func NewReadAdapterBinding(capability string, adapter agenttools.ReadAdapter) (ReadAdapterBinding, error) {
	return newReadAdapterBinding(capability, adapter)
}

func enabledReadAdapterBindings(settings config.ResolvedAgentToolSettings, bindings []ReadAdapterBinding) ([]ReadAdapterBinding, error) {
	enabled := make([]ReadAdapterBinding, 0, len(bindings))
	for index, binding := range bindings {
		if strings.TrimSpace(binding.capability) == "" {
			return nil, fmt.Errorf("read adapter binding at index %d has no capability", index)
		}
		if binding.adapter == nil {
			return nil, fmt.Errorf("read adapter binding at index %d for capability %q is nil", index, binding.capability)
		}
		if settings.Allows(binding.capability) {
			enabled = append(enabled, binding)
		}
	}
	return enabled, nil
}
