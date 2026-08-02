package imageapp

import (
	"context"
	"crypto/sha256"
	agentchat "denova/internal/agents/chat"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	agents "denova/internal/agents"
	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	novaskills "denova/internal/agents/skills"
	appagentruntime "denova/internal/app/agentruntime"
	appsettings "denova/internal/app/settings"
	apptask "denova/internal/app/task"
	imageasset "denova/internal/image/asset"
)

type AgentGenerateRequest struct {
	CommandID     string
	Purpose       string
	SourceContext string
	SystemPrompt  string
	ToolPrompt    string
	SkillName     string
	StoryID       string
	BranchID      string
	TurnID        string
	AltText       string
}

type AgentGenerateResult struct {
	AssistantText    string
	InteractiveImage *imageasset.InteractiveResult
	Replayed         bool
}

type imageAgentAdmission struct {
	Replayed bool
}

type imageAgentRunHooks struct {
	OnAccepted         func(imageAgentAdmission) error
	OnInteractiveImage func(*imageasset.InteractiveResult) error
}

func (service *Service) GenerateWithAgent(ctx context.Context, req AgentGenerateRequest) (AgentGenerateResult, error) {
	if err := validateAgentCommandID(req.CommandID); err != nil {
		return AgentGenerateResult{}, err
	}
	runtime, err := service.AcquireRuntime(ctx, "")
	if err != nil {
		return AgentGenerateResult{}, err
	}
	defer runtime.Release()
	return service.generateWithAgent(runtime, req)
}

// generateWithAgent executes inside a caller-owned workspace operation. The
// interactive-image flow deliberately reuses this core so its story reads,
// display events, Agent run, asset writes, and journal append remain one
// admitted composite even after the workspace scope starts draining.
func (service *Service) generateWithAgent(runtime *Runtime, req AgentGenerateRequest) (AgentGenerateResult, error) {
	return service.generateWithAgentUsingHooks(runtime, req, imageAgentRunHooks{})
}

func (service *Service) generateWithAgentUsingHooks(runtime *Runtime, req AgentGenerateRequest, hooks imageAgentRunHooks) (AgentGenerateResult, error) {
	req.CommandID = strings.TrimSpace(req.CommandID)
	if err := validateAgentCommandID(req.CommandID); err != nil {
		return AgentGenerateResult{}, err
	}
	if err := runtime.requireAgentAdapters(); err != nil {
		return AgentGenerateResult{}, err
	}
	cfg := runtime.Config
	novaDir := cfg.DataDir()
	if layered, loadErr := config.LoadLayeredWithStartupConfigAt(
		novaDir, runtime.Workspace, config.ProjectConfigPath(cfg.ProjectStateDir),
	); loadErr == nil {
		appsettings.ApplyLayered(&cfg, layered)
	} else {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[image-agent] load layered settings failed workspace=%s err=%v", runtime.Workspace, loadErr))
	}
	cfg.ImagePresetToolPrompt = strings.TrimSpace(req.ToolPrompt)
	runner, systemPrompt, err := appagentruntime.BuildImage(runtime.Context(), &cfg, runtime.BookState, req.SystemPrompt)
	if err != nil {
		return AgentGenerateResult{}, err
	}
	conversation := &imageAgentConversation{
		message:       imageAgentMessage(req),
		sourceContext: strings.TrimSpace(req.SourceContext),
		sourceSummary: imageAgentSourceSummary(req),
		contextBudget: agentcontext.ContextBudgetForAgent(&cfg, config.AgentKindImage),
		skillConfig:   cfg,
	}
	var result AgentGenerateResult
	var runErr error
	var hookErr error
	runCtx, cancelRun := context.WithCancel(runtime.Context())
	defer cancelRun()
	accepted, err := runtime.ChatService.StartWithOptions(runCtx, runner, conversation, runtime.BookService, agentchat.ChatRequest{
		CommandID: req.CommandID,
		Message:   conversation.message,
	}, agentrun.Options{
		AgentKind:          config.AgentKindImage,
		StateRoot:          cfg.ProjectStateDir,
		Workspace:          runtime.Workspace,
		StoryID:            req.StoryID,
		BranchID:           req.BranchID,
		TurnID:             req.TurnID,
		Mode:               "image",
		IdleTimeout:        appagentruntime.IdleTimeout(cfg),
		ToolResultMaxBytes: appagentruntime.ToolResultMaxBytes(cfg),
		SystemPromptLog:    systemPrompt,
	}, func(ev agentrun.Event) {
		switch ev.Type {
		case "tool_result":
			if image := eventInteractiveImage(ev.Data); image != nil {
				result.InteractiveImage = image
				if hooks.OnInteractiveImage != nil && hookErr == nil {
					hookErr = hooks.OnInteractiveImage(image)
				}
			}
		case "error":
			if runErr == nil {
				runErr = errors.New(eventErrorMessage(ev.Data))
			}
		}
	})
	if err != nil {
		if errors.Is(err, agentrun.ErrInvalidCommand) {
			return result, fmt.Errorf("%w: command_id=%q", apptask.ErrCommandConflict, req.CommandID)
		}
		return result, err
	}
	result.Replayed = accepted.Receipt().Replayed
	if hooks.OnAccepted != nil {
		if err := hooks.OnAccepted(imageAgentAdmission{Replayed: result.Replayed}); err != nil {
			cancelRun()
			_ = accepted.Wait(runCtx)
			return result, err
		}
	}
	outcome := accepted.Wait(runCtx)
	result.AssistantText = strings.TrimSpace(conversation.assistant)
	if result.AssistantText == "" {
		result.AssistantText = strings.TrimSpace(outcome.Content)
	}
	if hookErr != nil {
		return result, hookErr
	}
	if runErr != nil {
		return result, runErr
	}
	if outcome.Status != agentrun.OutcomeCompleted {
		if outcome.Error != nil {
			return result, outcome.Error
		}
		return result, fmt.Errorf("image Agent did not complete: status=%s reason=%s", outcome.Status, strings.TrimSpace(outcome.Reason))
	}
	if err := runtime.Context().Err(); err != nil {
		return result, err
	}
	if strings.TrimSpace(req.Purpose) == "interactive_image" && result.InteractiveImage == nil {
		if result.Replayed {
			return result, ErrReplayResultUnavailable
		}
		return result, fmt.Errorf("image Agent did not generate an interactive image")
	}
	output := result.AssistantText
	if result.InteractiveImage != nil {
		output = firstNonEmpty(output, result.InteractiveImage.ImagePath)
		slog.InfoContext(context.Background(), fmt.Sprintf("[image-agent] generated interactive image workspace=%s story_id=%s branch_id=%s turn_id=%s path=%s", runtime.Workspace, result.InteractiveImage.StoryID, result.InteractiveImage.BranchID, result.InteractiveImage.TurnID, result.InteractiveImage.ImagePath))
	} else {
		slog.InfoContext(context.Background(), fmt.Sprintf("[image-agent] completed image request workspace=%s purpose=%s", runtime.Workspace, strings.TrimSpace(req.Purpose)))
	}
	if !result.Replayed {
		persistImageAgentCall(runtime.SessionStore, conversation.message, output)
	}
	return result, nil
}

func validateAgentCommandID(commandID string) error {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return apptask.ErrCommandIDRequired
	}
	return agentrun.ValidateCommandID(commandID)
}

type imageAgentConversation struct {
	message       string
	sourceContext string
	sourceSummary string
	assistant     string
	contextBudget agentcontext.Budget
	skillConfig   config.Config
}

func (c *imageAgentConversation) ModelContextBudget() agentcontext.Budget {
	if c == nil {
		return agentcontext.DefaultBudget()
	}
	return c.contextBudget
}

func (c *imageAgentConversation) ResolveExplicitSkills(ctx context.Context, message string) ([]novaskills.Invocation, error) {
	if c == nil {
		return nil, nil
	}
	return novaskills.ResolveConfiguredInvocations(ctx, &c.skillConfig, config.AgentKindImage, message)
}

func (c *imageAgentConversation) AssembleModelContext(ctx context.Context, _ string, input agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	if strings.TrimSpace(c.message) == "" {
		return agentcontext.ModelContextResult{}, fmt.Errorf("image Agent input is empty")
	}
	fragments := append([]agentcontext.Fragment(nil), input.Fragments...)
	if c.sourceContext != "" {
		fragments = append(fragments, agentcontext.Fragment{
			ID: "image_request_source_context", Source: "image.request.source_context", Title: "图像生成来源上下文",
			Purpose: "ground the generated image in the bounded source material selected for this request",
			Content: c.sourceContext, Placement: agentcontext.PlacementFinalUserPrefix, Included: true,
			Note: "source=image generation request; turn-scoped",
		})
	}
	assembled, err := agentcontext.NewAssembler(input.Budget).Assemble(ctx, agentcontext.AssembleRequest{
		Messages: []*agents.Message{agents.UserMessage(c.message)}, Fragments: fragments,
	})
	if err != nil {
		return agentcontext.ModelContextResult{}, err
	}
	return agentcontext.ModelContextResult{Messages: assembled.Messages, Context: assembled}, nil
}

func (c *imageAgentConversation) AppendAssistant(content string) error {
	c.assistant = strings.TrimSpace(content)
	return nil
}

func (c *imageAgentConversation) MarkInterrupted(_, _, _ string) error       { return nil }
func (c *imageAgentConversation) PendingInterruption() *session.Interruption { return nil }
func (c *imageAgentConversation) ResolveInterruption(string) error           { return nil }
func (c *imageAgentConversation) ContextSourceSummary() string               { return c.sourceSummary }

func imageAgentMessage(req AgentGenerateRequest) string {
	var sb strings.Builder
	if skill := strings.TrimSpace(req.SkillName); skill != "" {
		sb.WriteString("/")
		sb.WriteString(skill)
		sb.WriteString("\n\n")
	}
	sb.WriteString("# 图像生成请求\n\n")
	writeImageAgentField(&sb, "purpose", req.Purpose)
	writeImageAgentField(&sb, "story_id", req.StoryID)
	writeImageAgentField(&sb, "branch_id", req.BranchID)
	writeImageAgentField(&sb, "turn_id", req.TurnID)
	writeImageAgentField(&sb, "alt_text", req.AltText)
	writeImageAgentField(&sb, "source_context_sha256", imageAgentSemanticHash(req.SourceContext))
	writeImageAgentField(&sb, "system_prompt_sha256", imageAgentSemanticHash(req.SystemPrompt))
	writeImageAgentField(&sb, "tool_prompt_sha256", imageAgentSemanticHash(req.ToolPrompt))
	sb.WriteString("\n请读取所需 Skill 后调用 generate_image 完成图像生成。")
	return strings.TrimSpace(sb.String())
}

func imageAgentSemanticHash(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return fmt.Sprintf("%x", digest[:])
}

func writeImageAgentField(sb *strings.Builder, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	sb.WriteString("- ")
	sb.WriteString(key)
	sb.WriteString(": ")
	sb.WriteString(value)
	sb.WriteString("\n")
}

func imageAgentSourceSummary(req AgentGenerateRequest) string {
	var parts []string
	if strings.TrimSpace(req.Purpose) != "" {
		parts = append(parts, "purpose="+strings.TrimSpace(req.Purpose))
	}
	if strings.TrimSpace(req.SkillName) != "" {
		parts = append(parts, "skill="+strings.TrimSpace(req.SkillName))
	}
	if strings.TrimSpace(req.SourceContext) != "" {
		parts = append(parts, fmt.Sprintf("source_context_chars=%d", len([]rune(req.SourceContext))))
	}
	return strings.Join(parts, " ")
}

func eventInteractiveImage(data interface{}) *imageasset.InteractiveResult {
	payload, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	switch value := payload["interactive_image"].(type) {
	case *imageasset.InteractiveResult:
		return value
	case imageasset.InteractiveResult:
		return &value
	default:
		return nil
	}
}

func eventErrorMessage(data interface{}) string {
	payload, ok := data.(map[string]string)
	if ok {
		return firstNonEmpty(payload["message"], payload["error"], "image Agent execution failed")
	}
	if generic, ok := data.(map[string]interface{}); ok {
		message, _ := generic["message"].(string)
		errorText, _ := generic["error"].(string)
		return firstNonEmpty(message, errorText, "image Agent execution failed")
	}
	return "image Agent execution failed"
}

func persistImageAgentCall(store *session.Store, instruction, response string) {
	if store == nil {
		slog.WarnContext(context.Background(), "[image-agent] skip persistence reason=no_session_store")
		return
	}
	if err := session.PersistAgentCall(store, config.AgentKindImage, instruction, response); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[image-agent] persist session call failed err=%v", err))
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
