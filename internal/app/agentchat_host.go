package app

import (
	"context"

	"denova/config"
	agentconversation "denova/internal/agents/conversation"
	agentharness "denova/internal/agents/harness"
	"denova/internal/agents/session"
	agenttool "denova/internal/agents/tool"
	"denova/internal/book"
)

// agentChatHost is the narrow composition adapter between the project-scoped
// AgentChat owner and process-wide mutation/lifecycle policy.
type agentChatHost struct {
	app *App
}

func (host agentChatHost) BaseRuntime() (config.Config, *agentharness.Service) {
	if host.app == nil {
		return config.Config{}, nil
	}
	host.app.mu.RLock()
	defer host.app.mu.RUnlock()
	var cfg config.Config
	if host.app.cfg != nil {
		cfg = *host.app.cfg
	}
	return cfg, host.app.chatService
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

func (host agentChatHost) ResolveAsk(
	ctx context.Context,
	target *session.Session,
	projectID, workspace, askID, status string,
	answers []agentconversation.HostAskAnswer,
	cancelReason string,
) (agentconversation.HostAskResolution, error) {
	if host.app == nil {
		return agentconversation.HostAskResolution{}, ErrNoWorkspace
	}
	return host.app.resolveAgentAsk(ctx, target, projectID, workspace, askID, status, answers, cancelReason)
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
}
