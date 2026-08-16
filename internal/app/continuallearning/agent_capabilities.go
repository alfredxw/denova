package continuallearning

import (
	"context"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/harnessstate"
	producttools "denova/internal/agents/tools"
)

// AgentHostCapabilities returns the complete root-only User State management
// surface for one Agent policy. Saved Script Tools are assembled separately
// from the immutable Harness snapshot and do not depend on this capability.
func (service *Service) AgentHostCapabilities(
	ctx context.Context,
	cfg *config.Config,
	agentKind string,
) (agents.AgentHostCapabilities, error) {
	host := agents.AgentHostCapabilities{Interactive: true}
	if cfg == nil || !cfg.Labs.DeveloperMode || !config.ResolveAgentTools(cfg, agentKind).Allows(config.AgentToolHarnessState) {
		return host, nil
	}
	if _, err := service.requireEnabled(); err != nil {
		return agents.AgentHostCapabilities{}, err
	}
	adapter, err := harnessstate.NewReadAdapter(service.manager)
	if err != nil {
		return agents.AgentHostCapabilities{}, err
	}
	binding, err := producttools.NewReadAdapterBinding(config.AgentToolHarnessState, adapter)
	if err != nil {
		return agents.AgentHostCapabilities{}, err
	}
	update, err := service.StateUpdateTool()
	if err != nil {
		return agents.AgentHostCapabilities{}, err
	}
	host.ReadAdapters = []producttools.ReadAdapterBinding{binding}
	host.RootTools = []agents.ToolDefinition{update}
	return host, nil
}
