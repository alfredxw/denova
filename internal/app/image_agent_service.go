package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"strings"

	"denova/config"
	agents "denova/internal/agents"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/session"
	"denova/internal/interactiveimage"
)

type ImageAgentGenerateRequest struct {
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

type ImageAgentGenerateResult struct {
	AssistantText    string
	InteractiveImage *interactiveimage.Result
	Replayed         bool
}

type imageAgentAdmission struct {
	Replayed bool
}

type imageAgentRunHooks struct {
	OnAccepted         func(imageAgentAdmission) error
	OnInteractiveImage func(*interactiveimage.Result) error
}

func (a *App) GenerateImageWithAgent(ctx context.Context, req ImageAgentGenerateRequest) (ImageAgentGenerateResult, error) {
	return a.images().GenerateWithAgent(ctx, req)
}

func (s *ImageAppService) GenerateWithAgent(ctx context.Context, req ImageAgentGenerateRequest) (ImageAgentGenerateResult, error) {
	if err := validateImageAgentCommandID(req.CommandID); err != nil {
		return ImageAgentGenerateResult{}, err
	}
	runtime, err := s.acquireWorkspaceRuntime(ctx)
	if err != nil {
		return ImageAgentGenerateResult{}, err
	}
	defer runtime.Release()
	return s.generateWithAgent(runtime, req)
}

// generateWithAgent executes inside a caller-owned workspace operation. The
// interactive-image flow deliberately reuses this core so its story reads,
// display events, Agent run, asset writes, and journal append remain one
// admitted composite even after the workspace scope starts draining.
func (s *ImageAppService) generateWithAgent(runtime *imageWorkspaceRuntime, req ImageAgentGenerateRequest) (ImageAgentGenerateResult, error) {
	return s.generateWithAgentUsingHooks(runtime, req, imageAgentRunHooks{})
}

func (s *ImageAppService) generateWithAgentUsingHooks(runtime *imageWorkspaceRuntime, req ImageAgentGenerateRequest, hooks imageAgentRunHooks) (ImageAgentGenerateResult, error) {
	req.CommandID = strings.TrimSpace(req.CommandID)
	if err := validateImageAgentCommandID(req.CommandID); err != nil {
		return ImageAgentGenerateResult{}, err
	}
	if err := runtime.requireAgentAdapters(); err != nil {
		return ImageAgentGenerateResult{}, err
	}
	cfg := runtime.cfg
	novaDir := cfg.DataDir()
	if layered, loadErr := config.LoadLayeredWithStartupConfigAt(
		novaDir, runtime.workspace, config.ProjectConfigPath(cfg.ProjectStateDir),
	); loadErr == nil {
		applyLayeredSettingsToConfig(&cfg, layered)
	} else {
		log.Printf("[image-agent] 加载分层配置失败 workspace=%s err=%v", runtime.workspace, loadErr)
	}
	cfg.ImagePresetToolPrompt = strings.TrimSpace(req.ToolPrompt)
	runner, systemPrompt, err := buildImageAgentRunnerWithComposition(runtime.Context(), &cfg, runtime.bookState, req.SystemPrompt)
	if err != nil {
		return ImageAgentGenerateResult{}, err
	}
	conversation := &imageAgentConversation{
		message:       imageAgentMessage(req),
		sourceContext: strings.TrimSpace(req.SourceContext),
		sourceSummary: imageAgentSourceSummary(req),
		contextBudget: agents.ContextBudgetForAgent(&cfg, config.AgentKindImage),
		skillConfig:   cfg,
	}
	var result ImageAgentGenerateResult
	var runErr error
	var hookErr error
	runCtx, cancelRun := context.WithCancel(runtime.Context())
	defer cancelRun()
	accepted, err := runtime.chatService.StartWithOptions(runCtx, runner, conversation, runtime.bookService, agents.ChatRequest{
		CommandID: req.CommandID,
		Message:   conversation.message,
	}, agents.RunOptions{
		AgentKind:          config.AgentKindImage,
		StateRoot:          cfg.ProjectStateDir,
		Workspace:          runtime.workspace,
		StoryID:            req.StoryID,
		BranchID:           req.BranchID,
		TurnID:             req.TurnID,
		Mode:               "image",
		IdleTimeout:        agentIdleTimeout(cfg),
		ToolResultMaxBytes: agentToolResultMaxBytes(cfg),
		SystemPromptLog:    systemPrompt,
	}, func(ev agents.Event) {
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
		if errors.Is(err, agents.ErrInvalidCommand) {
			return result, fmt.Errorf("%w: command_id=%q", ErrAgentCommandConflict, req.CommandID)
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
	if outcome.Status != agents.RunOutcomeCompleted {
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
			return result, ErrImageAgentReplayResultUnavailable
		}
		return result, fmt.Errorf("图像 Agent 未生成互动图像")
	}
	output := result.AssistantText
	if result.InteractiveImage != nil {
		output = firstNonEmpty(output, result.InteractiveImage.ImagePath)
		log.Printf("[image-agent] generated interactive image workspace=%s story_id=%s branch_id=%s turn_id=%s path=%s", runtime.workspace, result.InteractiveImage.StoryID, result.InteractiveImage.BranchID, result.InteractiveImage.TurnID, result.InteractiveImage.ImagePath)
	} else {
		log.Printf("[image-agent] completed image request workspace=%s purpose=%s", runtime.workspace, strings.TrimSpace(req.Purpose))
	}
	if !result.Replayed {
		persistAgentCallWithStore(runtime.sessionStore, config.AgentKindImage, conversation.message, output)
	}
	return result, nil
}

func validateImageAgentCommandID(commandID string) error {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return ErrAgentCommandIDRequired
	}
	return agents.ValidateCommandID(commandID)
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

func (c *imageAgentConversation) ResolveExplicitSkills(ctx context.Context, message string) ([]agents.ExplicitSkillInvocation, error) {
	if c == nil {
		return nil, nil
	}
	return agents.ResolveExplicitSkillInvocations(ctx, &c.skillConfig, config.AgentKindImage, message)
}

func (c *imageAgentConversation) AssembleModelContext(ctx context.Context, _ string, input agents.ModelContextInput) (agents.ModelContextResult, error) {
	if strings.TrimSpace(c.message) == "" {
		return agents.ModelContextResult{}, fmt.Errorf("图像 Agent 输入不能为空")
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
		return agents.ModelContextResult{}, err
	}
	return agents.ModelContextResult{Messages: assembled.Messages, Context: assembled}, nil
}

func (c *imageAgentConversation) AppendAssistant(content string) error {
	c.assistant = strings.TrimSpace(content)
	return nil
}

func (c *imageAgentConversation) MarkInterrupted(_, _, _ string) error       { return nil }
func (c *imageAgentConversation) PendingInterruption() *session.Interruption { return nil }
func (c *imageAgentConversation) ResolveInterruption(string) error           { return nil }
func (c *imageAgentConversation) ContextSourceSummary() string               { return c.sourceSummary }

func imageAgentMessage(req ImageAgentGenerateRequest) string {
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

func imageAgentSourceSummary(req ImageAgentGenerateRequest) string {
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

func eventInteractiveImage(data interface{}) *interactiveimage.Result {
	payload, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	switch value := payload["interactive_image"].(type) {
	case *interactiveimage.Result:
		return value
	case interactiveimage.Result:
		return &value
	default:
		return nil
	}
}

func eventErrorMessage(data interface{}) string {
	payload, ok := data.(map[string]string)
	if ok {
		return firstNonEmpty(payload["message"], payload["error"], "图像 Agent 执行失败")
	}
	if generic, ok := data.(map[string]interface{}); ok {
		message, _ := generic["message"].(string)
		errorText, _ := generic["error"].(string)
		return firstNonEmpty(message, errorText, "图像 Agent 执行失败")
	}
	return "图像 Agent 执行失败"
}
