package agent

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"

	"nova/config"
	"nova/internal/prompts"
)

func GenerateInteractiveState(ctx context.Context, cfg *config.Config, instruction string) (string, error) {
	return generateInteractiveStateContent(ctx, cfg, instruction, nil)
}

func StreamInteractiveState(ctx context.Context, cfg *config.Config, instruction string, emit func(Event)) (string, error) {
	return generateInteractiveStateContent(ctx, cfg, instruction, emit)
}

func generateInteractiveStateContent(ctx context.Context, cfg *config.Config, instruction string, emit func(Event)) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	modelCfg := chatModelConfigForAgent(cfg, config.AgentKindInteractiveState)
	modelCfg.ResponseFormat = &openai.ChatCompletionResponseFormat{
		Type: openai.ChatCompletionResponseFormatTypeJSONObject,
	}
	cm, err := openai.NewChatModel(ctx, &modelCfg)
	if err != nil {
		return "", fmt.Errorf("创建互动状态模型失败: %w", err)
	}
	log.Printf("[interactive-state-agent] generate begin instruction=%s stream=%t", promptPartSummary(instruction), emit != nil)
	messages := []*schema.Message{
		schema.SystemMessage(protectedSystemInstruction(cfg, config.AgentKindInteractiveState, prompts.BuildInteractiveStateSystemInstruction())),
		schema.UserMessage(instruction),
	}
	if emit == nil {
		logFullModelInput(modelInputLogOptions{
			AgentKind: config.AgentKindInteractiveState,
			Source:    "interactive_state",
			Mode:      "generate",
			Config:    modelCfg,
			Messages:  messages,
		})
		msg, err := cm.Generate(ctx, messages)
		if err != nil {
			return "", fmt.Errorf("生成互动状态失败: %w", err)
		}
		if msg == nil {
			return "", fmt.Errorf("互动状态模型返回为空")
		}
		log.Printf("[interactive-state-agent] generate done output=%s", promptPartSummary(msg.Content))
		return msg.Content, nil
	}
	logFullModelInput(modelInputLogOptions{
		AgentKind: config.AgentKindInteractiveState,
		Source:    "interactive_state",
		Mode:      "stream",
		Config:    modelCfg,
		Messages:  messages,
	})
	stream, err := cm.Stream(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("生成互动状态失败: %w", err)
	}
	defer stream.Close()
	var content strings.Builder
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("接收互动状态失败: %w", err)
		}
		if msg == nil {
			continue
		}
		if msg.ReasoningContent != "" {
			emit(Event{Type: "thinking", Data: map[string]string{"content": msg.ReasoningContent}})
		}
		if msg.Content != "" {
			content.WriteString(msg.Content)
			emit(Event{Type: "chunk", Data: map[string]string{"content": msg.Content}})
		}
	}
	output := strings.TrimSpace(content.String())
	if output == "" {
		return "", fmt.Errorf("互动状态模型返回为空")
	}
	log.Printf("[interactive-state-agent] generate done output=%s", promptPartSummary(output))
	return output, nil
}
