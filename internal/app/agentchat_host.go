package app

import (
	"context"
	"log/slog"

	"denova/config"
	agents "denova/internal/agents"
	agentexecution "denova/internal/agents/execution"
	agenttool "denova/internal/agents/tool"
	appsettings "denova/internal/app/settings"
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
	resources, err := host.app.ProjectFiles().ProjectVersions(projectID)
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
	if projectType == projectdomain.TypeAgents {
		return host.app.AgentsProjectAgentHostCapabilities(ctx, cfg, agentKind)
	}
	return host.app.AgentHostCapabilities(ctx, cfg, agentKind)
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
	if cfg.ProjectID == projectdomain.AgentsProjectID && len(mutations) > 0 {
		if _, err := host.app.SettingsService().Reload(appsettings.Global(), config.SettingsLayerUser); err != nil {
			slog.ErrorContext(ctx, "[internal/app/agentchat_host.go] reload Agent Profiles after Agents Project mutation failed",
				"project_id", projectdomain.AgentsProjectID,
				"error", err,
			)
		}
	}
}
