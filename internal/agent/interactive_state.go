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
)

const interactiveStateAgentLabel = "interactive-state-agent"

func GenerateInteractiveState(ctx context.Context, cfg *config.Config, instruction string) (string, error) {
	return generateInteractiveStateContent(ctx, cfg, instruction, nil)
}

func StreamInteractiveState(ctx context.Context, cfg *config.Config, instruction string, emit func(Event)) (string, error) {
	return generateInteractiveStateContent(ctx, cfg, instruction, emit)
}

func GenerateInteractiveStateWithTools(ctx context.Context, cfg *config.Config, state *book.State, toolContext InteractiveStoryToolContext, instruction string, emit func(Event)) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	if state == nil {
		return generateInteractiveStateContent(ctx, cfg, instruction, emit)
	}
	builtAgent, err := BuildInteractiveState(ctx, cfg, state, toolContext)
	if err != nil {
		return "", fmt.Errorf("构建互动状态 Agent 失败: %w", err)
	}
	runner := NewRunnerWithOptions(ctx, builtAgent, RunOptions{AgentKind: config.AgentKindInteractiveState, Workspace: cfg.Workspace})
	conversation := &singleInstructionConversation{instruction: instruction}
	bookService := book.NewService(state.Workspace())
	var runErr error
	NewChatService().RunWithOptions(ctx, runner, conversation, bookService, ChatRequest{Message: instruction}, RunOptions{
		AgentKind:          config.AgentKindInteractiveState,
		Workspace:          cfg.Workspace,
		ToolResultMaxBytes: 16 * 1024,
	}, func(event Event) {
		if emit != nil {
			emit(event)
		}
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
		return "", fmt.Errorf("互动状态 Agent 返回为空")
	}
	return output, nil
}

func generateInteractiveStateContent(ctx context.Context, cfg *config.Config, instruction string, emit func(Event)) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	modelCfg := chatModelConfigForAgent(cfg, config.AgentKindInteractiveState)
	log.Printf("[%s] generate begin instruction=%s stream=%t", interactiveStateAgentLabel, promptPartSummary(instruction), emit != nil)
	messages := []*schema.Message{
		schema.SystemMessage(protectedSystemInstruction(cfg, config.AgentKindInteractiveState, prompts.BuildInteractiveStateSystemInstruction())),
		schema.UserMessage(instruction),
	}
	if emit == nil {
		content, err := generateWithJSONFallback(ctx, modelCfg, messages, config.AgentKindInteractiveState, "interactive_state", interactiveStateAgentLabel)
		if err != nil {
			return "", fmt.Errorf("生成互动状态失败: %w", err)
		}
		log.Printf("[%s] generate done output=%s", interactiveStateAgentLabel, promptPartSummary(content))
		return content, nil
	}
	content, err := streamWithJSONFallback(ctx, modelCfg, messages, emit, config.AgentKindInteractiveState, "interactive_state", interactiveStateAgentLabel)
	if err != nil {
		return "", fmt.Errorf("生成互动状态失败: %w", err)
	}
	log.Printf("[%s] generate done output=%s", interactiveStateAgentLabel, promptPartSummary(content))
	return content, nil
}
