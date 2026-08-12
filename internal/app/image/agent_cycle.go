package imageapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentcontext "denova/internal/agents/context"
	agentconversation "denova/internal/agents/conversation"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	appagentruntime "denova/internal/app/agentruntime"
	appsettings "denova/internal/app/settings"
)

const (
	imageAgentRestoreDataType    = "denova.image.agent_request"
	imageAgentRestoreDataVersion = 1
)

func (service *Service) prepareAgentCycle(runtime *Runtime, request AgentGenerateRequest) (agentexecution.Cycle, *imageAgentConversation, error) {
	if err := runtime.requireAgentAdapters(); err != nil {
		return agentexecution.Cycle{}, nil, err
	}
	cfg := runtime.Config
	novaDir := cfg.DataDir()
	if layered, err := config.LoadLayeredWithStartupConfigAt(
		novaDir, runtime.Workspace, config.ProjectConfigPath(cfg.ProjectStateDir),
	); err == nil {
		appsettings.ApplyLayered(&cfg, layered)
	} else {
		slog.ErrorContext(runtime.Context(), fmt.Sprintf("[image-agent] load layered settings failed workspace=%s err=%v", runtime.Workspace, err))
	}
	cfg.ImagePresetToolPrompt = strings.TrimSpace(request.ToolPrompt)
	modelSelection := config.ResolveAgentModel(&cfg, config.AgentKindImage)
	slog.InfoContext(runtime.Context(), fmt.Sprintf(
		"[image-agent] preparing run workspace=%s profile_id=%s thinking_level=%s",
		runtime.Workspace, strings.TrimSpace(modelSelection.ProfileID), strings.TrimSpace(modelSelection.ThinkingLevel),
	))
	sess, err := prepareImageAgentSession(runtime.SessionStore)
	if err != nil {
		return agentexecution.Cycle{}, nil, err
	}
	builtAgent, err := appagentruntime.BuildImageAgent(appagentruntime.WithHarnessRun(runtime.Context(), request.CommandID), &cfg, runtime.BookState, request.SystemPrompt)
	if err != nil {
		return agentexecution.Cycle{}, nil, err
	}
	conversation := &imageAgentConversation{
		journal:       agentconversation.NewSessionConversationForAgent(sess, &cfg, config.AgentKindImage),
		message:       imageAgentMessage(request),
		sourceContext: strings.TrimSpace(request.SourceContext),
		sourceSummary: imageAgentSourceSummary(request),
		contextBudget: agentcontext.ContextBudgetForAgent(&cfg, config.AgentKindImage),
		skillConfig:   cfg,
	}
	restoreData, err := encodeImageAgentRestoreData(request)
	if err != nil {
		return agentexecution.Cycle{}, nil, err
	}
	return agentexecution.Cycle{
		Definition: builtAgent.Definition, Conversation: conversation,
		BookService: runtime.BookService,
		Request:     agentchat.ChatRequest{CommandID: request.CommandID, Message: conversation.message},
		Options: agentrun.Options{
			AgentKind: config.AgentKindImage, ProjectID: runtime.ProjectID,
			StateRoot: cfg.ProjectStateDir, SessionID: sess.ID, Workspace: runtime.Workspace,
			StoryID: request.StoryID, BranchID: request.BranchID, TurnID: request.TurnID,
			Mode: "image", RestoreData: restoreData,
			IdleTimeout: appagentruntime.IdleTimeout(cfg), ToolResultMaxBytes: appagentruntime.ToolResultMaxBytes(cfg),
			SystemPromptLog: builtAgent.Composition,
		},
	}, conversation, nil
}

// PrepareCycle rebuilds an Image Agent after cold recovery from the exact
// bounded request retained in Agent HostData. Image-specific context remains
// Denova-owned and never enters the reusable package's persistence schema.
func (service *Service) PrepareCycle(
	ctx context.Context,
	request agentexecution.CycleRestoreRequest,
	binding agentrun.RuntimeBinding,
) (agentexecution.Cycle, error) {
	restored, err := decodeImageAgentRestoreData(request.Options.RestoreData)
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	var runtime *Runtime
	if strings.TrimSpace(binding.ProjectID) != "" {
		runtime, err = service.AcquireProjectRuntime(ctx, binding.ProjectID)
	} else {
		runtime, err = service.AcquireRuntime(ctx, binding.Workspace)
	}
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	defer runtime.Release()
	if runtime.Workspace != binding.Workspace || restored.CommandID != string(request.CommandID) {
		return agentexecution.Cycle{}, fmt.Errorf("%w: Image Agent runtime changed", agentexecution.ErrCyclePreparationUnavailable)
	}
	cycle, _, err := service.prepareAgentCycle(runtime, restored)
	return cycle, err
}

func encodeImageAgentRestoreData(request AgentGenerateRequest) (*agentrun.RestoreData, error) {
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode Image Agent restore data: %w", err)
	}
	return &agentrun.RestoreData{Type: imageAgentRestoreDataType, Version: imageAgentRestoreDataVersion, Data: encoded}, nil
}

func decodeImageAgentRestoreData(data *agentrun.RestoreData) (AgentGenerateRequest, error) {
	if data == nil || data.Type != imageAgentRestoreDataType || data.Version != imageAgentRestoreDataVersion || len(data.Data) == 0 {
		return AgentGenerateRequest{}, fmt.Errorf("%w: Image Agent restore data is unavailable", agentexecution.ErrCyclePreparationUnavailable)
	}
	var request AgentGenerateRequest
	if err := json.Unmarshal(data.Data, &request); err != nil {
		return AgentGenerateRequest{}, fmt.Errorf("decode Image Agent restore data: %w", errors.Join(agentexecution.ErrCyclePreparationUnavailable, err))
	}
	if err := validateAgentCommandID(request.CommandID); err != nil {
		return AgentGenerateRequest{}, fmt.Errorf("%w: invalid Image Agent restore command: %v", agentexecution.ErrCyclePreparationUnavailable, err)
	}
	return request, nil
}
