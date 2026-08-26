package app

import (
	"context"
	"log/slog"

	"denova/config"
	agents "denova/internal/agents"
	agentexecution "denova/internal/agents/execution"
	agenttool "denova/internal/agents/tool"
	"denova/internal/book"
	projectdomain "denova/internal/project"
)

// agentChatHost is the narrow composition adapter between the project-scoped
// AgentChat owner and process-wide mutation/lifecycle policy.
type agentChatHost struct {
	app *App
}

func (host agentChatHost) BaseRuntime() (config.Config, *agentexecution.Runtime) {
	if host.app == nil {
		return config.Config{}, nil
	}
	host.app.mu.RLock()
	defer host.app.mu.RUnlock()
	var cfg config.Config
	if host.app.cfg != nil {
		cfg = *host.app.cfg
	}
	return cfg, host.app.executionRuntime
}

func (host agentChatHost) ProjectVersionService(projectID string) (*book.VersionService, error) {
	if host.app == nil {
		return nil, ErrNoWorkspace
	}
	resources, err := host.app.ProjectFiles().BookVersions(projectID)
	if err != nil {
		return nil, err
	}
	return resources.VersionService, nil
}

func (host agentChatHost) CurrentWorkspace() string {
	if host.app == nil {
		return ""
	}
	host.app.mu.RLock()
	defer host.app.mu.RUnlock()
	return host.app.workspace
}

func (host agentChatHost) ProjectAgentHostCapabilities(
	ctx context.Context,
	projectType projectdomain.Type,
	cfg *config.Config,
	agentKind string,
) (agents.AgentHostCapabilities, error) {
	if host.app == nil {
		return agents.AgentHostCapabilities{}, nil
	}
	if projectType == projectdomain.TypeHarness {
		return host.app.ContinualLearning().HarnessProjectAgentHostCapabilities(ctx, cfg, agentKind)
	}
	return host.app.HarnessAgentHostCapabilities(ctx, cfg, agentKind)
}

func (host agentChatHost) OnVerifiedMutations(
	ctx context.Context,
	source string,
	versionService *book.VersionService,
	cfg config.Config,
	mutations []agenttool.Mutation,
	verification agenttool.Verification,
) {
	if host.app == nil {
		return
	}
	host.app.verifiedWorkspaceMutationCallback(
		source, versionService, versionAutoSettingsForConfig(&cfg),
	)(ctx, mutations, verification)
	if cfg.ProjectID == projectdomain.HarnessProjectID && len(mutations) > 0 {
		if err := host.app.ContinualLearning().RecordCurrentState(ctx, "Harness Agent update"); err != nil {
			slog.WarnContext(ctx, "[harness-state] live Agent changes were not recorded as a valid version", "error", err)
		}
	}
}
