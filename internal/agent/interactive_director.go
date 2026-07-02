package agent

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/book"
	"denova/internal/prompts"
	"denova/internal/session"
)

const interactiveDirectorAgentLabel = "interactive-director-agent"

func GenerateInteractiveDirector(ctx context.Context, cfg *config.Config, instruction string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	modelCfg := chatModelConfigForAgent(cfg, config.AgentKindInteractiveDirector)
	log.Printf("[%s] generate begin instruction=%s", interactiveDirectorAgentLabel, promptPartSummary(instruction))
	messages := []*schema.Message{
		schema.SystemMessage(protectedSystemInstruction(cfg, config.AgentKindInteractiveDirector, prompts.BuildInteractiveDirectorSystemInstruction())),
		schema.UserMessage(instruction),
	}
	content, err := generateWithJSONFallback(ctx, modelCfg, messages, config.AgentKindInteractiveDirector, "interactive_director", interactiveDirectorAgentLabel)
	if err != nil {
		return "", fmt.Errorf("生成互动导演状态失败: %w", err)
	}
	log.Printf("[%s] generate done output=%s", interactiveDirectorAgentLabel, promptPartSummary(content))
	return content, nil
}

func GenerateInteractiveDirectorWithTools(ctx context.Context, cfg *config.Config, state *book.State, toolContext InteractiveStoryToolContext, instruction string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	if state == nil {
		return GenerateInteractiveDirector(ctx, cfg, instruction)
	}
	builtAgent, err := BuildInteractiveDirector(ctx, cfg, state, toolContext)
	if err != nil {
		return "", fmt.Errorf("构建互动导演 Agent 失败: %w", err)
	}
	runner := NewRunnerWithOptions(ctx, builtAgent, RunOptions{AgentKind: config.AgentKindInteractiveDirector, Workspace: cfg.Workspace})
	conversation := &singleInstructionConversation{instruction: instruction}
	bookService := book.NewService(state.Workspace())
	var runErr error
	NewChatService().RunWithOptions(ctx, runner, conversation, bookService, ChatRequest{Message: instruction}, RunOptions{
		AgentKind:          config.AgentKindInteractiveDirector,
		Workspace:          cfg.Workspace,
		ToolResultMaxBytes: 16 * 1024,
	}, func(event Event) {
		if event.Type != "error" {
			return
		}
		if data, ok := event.Data.(map[string]string); ok {
			runErr = fmt.Errorf("%s", data["message"])
		}
	})
	if runErr != nil {
		return "", runErr
	}
	output := strings.TrimSpace(conversation.output)
	if output == "" {
		return "", fmt.Errorf("互动导演 Agent 返回为空")
	}
	return output, nil
}

type singleInstructionConversation struct {
	instruction string
	output      string
}

func (c *singleInstructionConversation) PrepareMessages(_, agentMessage string) ([]*schema.Message, error) {
	message := strings.TrimSpace(agentMessage)
	if message == "" {
		message = c.instruction
	}
	return []*schema.Message{schema.UserMessage(message)}, nil
}

func (c *singleInstructionConversation) AppendAssistant(content string) error {
	c.output = content
	return nil
}

func (c *singleInstructionConversation) MarkInterrupted(_, assistantContent, _ string) error {
	c.output = assistantContent
	return nil
}

func (c *singleInstructionConversation) PendingInterruption() *session.Interruption {
	return nil
}

func (c *singleInstructionConversation) ResolveInterruption(string) error {
	return nil
}
