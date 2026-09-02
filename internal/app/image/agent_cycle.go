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
	imagepreset "denova/internal/image/preset"
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
		novaDir, runtime.Workspace, config.ProjectConfigPath(cfg.ProjectStoreDir),
	); err == nil {
		appsettings.ApplyLayered(&cfg, layered)
	} else {
		slog.ErrorContext(runtime.Context(), fmt.Sprintf("[image-agent] load layered settings failed workspace=%s err=%v", runtime.Workspace, err))
	}
	usingDefaultAgent := strings.TrimSpace(request.CustomAgentID) == ""
	if usingDefaultAgent {
		request.CustomAgentID = cfg.DefaultImageAgentID
	}
	if err := config.ApplyCustomAgent(&cfg, config.AgentKindImage, request.CustomAgentID); err != nil {
		if !usingDefaultAgent || !errors.Is(err, config.ErrCustomAgentNotFound) {
			return agentexecution.Cycle{}, nil, err
		}
		slog.WarnContext(runtime.Context(), fmt.Sprintf(
			"[image-agent] configured default custom Agent is unavailable; using built-in Image Agent custom_agent_id=%q error=%v",
			request.CustomAgentID, err,
		))
		request.CustomAgentID = ""
		if fallbackErr := config.ApplyCustomAgent(&cfg, config.AgentKindImage, ""); fallbackErr != nil {
			return agentexecution.Cycle{}, nil, fallbackErr
		}
	}
	if presetID := strings.TrimSpace(request.ImagePresetID); presetID != "" {
		preset := loadImagePreset(cfg.DataDir(), presetID)
		if strings.TrimSpace(request.SystemPrompt) == "" {
			request.SystemPrompt = preset.PromptForTargets(imagepreset.TargetAgentSystem)
		}
		if strings.TrimSpace(request.ToolPrompt) == "" {
			request.ToolPrompt = preset.PromptForTargets(imagepreset.TargetToolRequest)
		}
	}
	cfg.ImagePresetToolPrompt = strings.TrimSpace(request.ToolPrompt)
	modelSelection := config.ResolveAgentModel(&cfg, config.AgentKindImage)
	slog.InfoContext(runtime.Context(), fmt.Sprintf(
		"[image-agent] preparing run workspace=%s custom_agent_id=%q profile_id=%s thinking_level=%s",
		runtime.Workspace, cfg.ActiveCustomAgentID, strings.TrimSpace(modelSelection.ProfileID), strings.TrimSpace(modelSelection.ThinkingLevel),
	))
	sess, err := prepareImageAgentSession(runtime.SessionStore, cfg.ActiveCustomAgentID)
	if err != nil {
		return agentexecution.Cycle{}, nil, err
	}
	builtAgent, err := appagentruntime.BuildImageAgent(runtime.Context(), &cfg, runtime.BookState, request.SystemPrompt)
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
			StateRoot: cfg.ProjectStoreDir, SessionID: sess.ID, Workspace: runtime.Workspace,
			StoryID: request.StoryID, BranchID: request.BranchID, TurnID: request.TurnID,
			Mode: "image", RestoreData: restoreData,
			IdleTimeout: appagentruntime.IdleTimeout(cfg), ToolResultMaxBytes: appagentruntime.ToolResultMaxBytes(cfg),
			SystemPromptLog: builtAgent.Composition,
		},
	}, conversation, nil
}

// PrepareCycle rebuilds Image-specific context from the bounded request kept in
// Agent HostData. That product context remains Denova-owned and never enters
// the reusable package's persistence schema.
func (service *Service) PrepareCycle(
	ctx context.Context,
	request agentexecution.CycleRestoreRequest,
	binding agentrun.RuntimeBinding,
) (agentexecution.Cycle, error) {
	restored, err := decodeImageAgentRestoreData(request.Options.RestoreData)
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	runtime, err := service.AcquireProjectRuntime(ctx, binding.ProjectID)
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	defer runtime.Release()
	if runtime.ProjectID != binding.ProjectID || restored.CommandID != string(request.CommandID) {
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
