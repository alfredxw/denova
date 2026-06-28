package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/agent"
	"nova/internal/book"
	"nova/internal/interactiveimage"
	"nova/internal/session"
)

type ImageAgentGenerateRequest struct {
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
}

func (a *App) GenerateImageWithAgent(ctx context.Context, req ImageAgentGenerateRequest) (ImageAgentGenerateResult, error) {
	return a.images().GenerateWithAgent(ctx, req)
}

func (s *ImageAppService) GenerateWithAgent(ctx context.Context, req ImageAgentGenerateRequest) (ImageAgentGenerateResult, error) {
	cfg, state, bookService, workspace, err := s.agentRuntimeSnapshot()
	if err != nil {
		return ImageAgentGenerateResult{}, err
	}
	cfg.ImagePresetToolPrompt = strings.TrimSpace(req.ToolPrompt)
	runner, err := buildImageAgentRunner(ctx, &cfg, state, req.SystemPrompt)
	if err != nil {
		return ImageAgentGenerateResult{}, err
	}
	conversation := &imageAgentConversation{
		message:       imageAgentMessage(req),
		sourceSummary: imageAgentSourceSummary(req),
	}
	var result ImageAgentGenerateResult
	var runErr error
	s.app.chatService.RunWithOptions(ctx, runner, conversation, bookService, agent.ChatRequest{
		Message: conversation.message,
	}, agent.RunOptions{
		AgentKind:          config.AgentKindImage,
		Workspace:          workspace,
		Mode:               "image",
		IdleTimeout:        agentIdleTimeout(cfg),
		ToolResultMaxBytes: agentToolResultMaxBytes(cfg),
		SystemPromptLog:    agent.BuildImageInstructionComposition(&cfg, state, req.SystemPrompt),
	}, func(ev agent.Event) {
		switch ev.Type {
		case "tool_result":
			if image := eventInteractiveImage(ev.Data); image != nil {
				result.InteractiveImage = image
			}
		case "error":
			if runErr == nil {
				runErr = errors.New(eventErrorMessage(ev.Data))
			}
		}
	})
	result.AssistantText = strings.TrimSpace(conversation.assistant)
	if runErr != nil {
		return result, runErr
	}
	if strings.TrimSpace(req.Purpose) == "interactive_image" && result.InteractiveImage == nil {
		return result, fmt.Errorf("图像 Agent 未生成互动图像")
	}
	output := result.AssistantText
	if result.InteractiveImage != nil {
		output = firstNonEmpty(output, result.InteractiveImage.ImagePath)
		log.Printf("[image-agent] generated interactive image workspace=%s story_id=%s branch_id=%s turn_id=%s path=%s", workspace, result.InteractiveImage.StoryID, result.InteractiveImage.BranchID, result.InteractiveImage.TurnID, result.InteractiveImage.ImagePath)
	} else {
		log.Printf("[image-agent] completed image request workspace=%s purpose=%s", workspace, strings.TrimSpace(req.Purpose))
	}
	s.app.persistAgentCall(config.AgentKindImage, conversation.message, output)
	return result, nil
}

func (s *ImageAppService) agentRuntimeSnapshot() (config.Config, *book.State, *book.Service, string, error) {
	app := s.app
	app.mu.RLock()
	if app.workspace == "" || app.bookService == nil || app.bookState == nil {
		app.mu.RUnlock()
		return config.Config{}, nil, nil, "", ErrNoWorkspace
	}
	if app.cfg == nil {
		app.mu.RUnlock()
		return config.Config{}, nil, nil, "", fmt.Errorf("运行配置未初始化")
	}
	cfg := *app.cfg
	state := app.bookState
	bookService := app.bookService
	workspace := app.workspace
	novaDir := cfg.NovaDir
	app.mu.RUnlock()

	cfg.Workspace = workspace
	if layered, err := config.LoadLayeredWithStartupConfig(novaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&cfg, layered)
	} else {
		log.Printf("[image-agent] 加载分层配置失败 workspace=%s err=%v", workspace, err)
	}
	return cfg, state, bookService, workspace, nil
}

type imageAgentConversation struct {
	message       string
	sourceSummary string
	assistant     string
}

func (c *imageAgentConversation) PrepareMessages(_, _ string) ([]*schema.Message, error) {
	if strings.TrimSpace(c.message) == "" {
		return nil, fmt.Errorf("图像 Agent 输入不能为空")
	}
	return []*schema.Message{schema.UserMessage(c.message)}, nil
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
		sb.WriteString("/<")
		sb.WriteString(skill)
		sb.WriteString(">\n\n")
	}
	sb.WriteString("# 图像生成请求\n\n")
	writeImageAgentField(&sb, "purpose", req.Purpose)
	writeImageAgentField(&sb, "story_id", req.StoryID)
	writeImageAgentField(&sb, "branch_id", req.BranchID)
	writeImageAgentField(&sb, "turn_id", req.TurnID)
	writeImageAgentField(&sb, "alt_text", req.AltText)
	if context := strings.TrimSpace(req.SourceContext); context != "" {
		sb.WriteString("\n## source_context\n\n")
		sb.WriteString(context)
		sb.WriteString("\n")
	}
	sb.WriteString("\n请读取所需 Skill 后调用 generate_image 完成图像生成。")
	return strings.TrimSpace(sb.String())
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
