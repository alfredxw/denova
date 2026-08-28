package continuallearning

import (
	"context"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/harnessstate"
	producttools "denova/internal/agents/tools"
	"denova/internal/agents/trajectory"
)

// AgentHostCapabilities returns the read-only User State inspection surface
// shared by ordinary Agents. State mutation belongs to the Harness Project's
// ordinary workspace tools.
func (service *Service) AgentHostCapabilities(
	ctx context.Context,
	cfg *config.Config,
	agentKind string,
) (agents.AgentHostCapabilities, error) {
	host := agents.AgentHostCapabilities{Interactive: true}
	if cfg == nil || !cfg.Labs.DeveloperMode || !cfg.Labs.HarnessStateEnabled ||
		!config.ResolveAgentTools(cfg, agentKind).Allows(config.AgentToolHarnessState) {
		return host, nil
	}
	if _, err := service.requireEnabled(); err != nil {
		return agents.AgentHostCapabilities{}, err
	}
	adapter, err := harnessstate.NewReadAdapter(service.published)
	if err != nil {
		return agents.AgentHostCapabilities{}, err
	}
	binding, err := producttools.NewReadAdapterBinding(config.AgentToolHarnessState, adapter)
	if err != nil {
		return agents.AgentHostCapabilities{}, err
	}
	host.ReadAdapters = []producttools.ReadAdapterBinding{binding}
	return host, nil
}

// HarnessProjectAgentHostCapabilities adds trajectory resources to the Draft
// workspace and exposes that same Draft through harness://state/current.
func (service *Service) HarnessProjectAgentHostCapabilities(
	ctx context.Context,
	cfg *config.Config,
	agentKind string,
) (agents.AgentHostCapabilities, error) {
	host := agents.AgentHostCapabilities{Interactive: true}
	runtime, err := service.requireEnabled()
	if err != nil {
		return agents.AgentHostCapabilities{}, err
	}
	stateAdapter, err := harnessstate.NewReadAdapter(service.manager)
	if err != nil {
		return agents.AgentHostCapabilities{}, err
	}
	stateBinding, err := producttools.NewReadAdapterBinding(config.AgentToolHarnessState, stateAdapter)
	if err != nil {
		return agents.AgentHostCapabilities{}, err
	}
	host.ReadAdapters = append(host.ReadAdapters, stateBinding)
	adapter, err := trajectory.NewReadAdapter(trajectory.Catalog{
		Sources:  service.host.TrajectorySources,
		Outcomes: service.outcomes,
		Limit:    runtime.Config.Labs.ContinualLearningTrajectoryCap,
	})
	if err != nil {
		return agents.AgentHostCapabilities{}, err
	}
	binding, err := producttools.NewReadAdapterBinding(config.AgentToolHarnessState, adapter)
	if err != nil {
		return agents.AgentHostCapabilities{}, err
	}
	host.ReadAdapters = append(host.ReadAdapters, binding)
	return host, nil
}
