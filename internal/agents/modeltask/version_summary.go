package modeltask

import (
	"context"
	"denova/internal/agents/run"
	"fmt"
	"log/slog"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/modelio"
	"denova/internal/agents/prompts"
)

// GenerateVersionSummary 根据版本变更上下文生成一行中文版本说明。
func GenerateVersionSummary(ctx context.Context, cfg *config.Config, instruction string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	var runErr error
	traceCtx, finishTrace := agentrun.WithStandaloneTrace(ctx, cfg, config.AgentKindVersionSummary, "version_summary", "generate", map[string]any{
		"instruction_chars": len([]rune(instruction)),
	})
	defer func() { finishTrace(runErr) }()
	modelCfg := modelio.ConfigForAgent(cfg, config.AgentKindVersionSummary)
	cm, err := modelio.NewChatModel(traceCtx, modelCfg)
	if err != nil {
		runErr = err
		return "", fmt.Errorf("创建版本说明模型失败: %w", err)
	}
	slog.InfoContext(ctx, fmt.Sprintf("[version-summary-agent] generate begin instruction=%s", prompts.PartSummary(instruction)))
	composition, err := prompts.ComposeBuiltinSystemInstruction(cfg, config.AgentKindVersionSummary, "version_summary", cfg.Workspace, "builtin_base", "版本说明生成规则", "define the version summary task and output constraint", "你是 Denova 小说工作台的版本说明生成器。根据文件变更推理这次保存的核心创作变化。只输出一句中文版本说明，10 到 30 个汉字，不要编号、引号、冒号、句号或解释。")
	if err != nil {
		runErr = err
		return "", err
	}
	messages := []*agent.Message{
		agent.SystemMessage(composition.Instruction()),
		agent.UserMessage(instruction),
	}
	if err := modelio.ValidateConfiguredInput(cfg, config.AgentKindVersionSummary, messages, nil); err != nil {
		runErr = err
		return "", err
	}
	span, callID, llmTraceCtx := agentrun.BeginLLMCallTrace(traceCtx, config.AgentKindVersionSummary, "version_summary", "generate", modelCfg, messages, nil, false)
	msg, err := cm.Generate(llmTraceCtx, messages)
	if err != nil {
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindVersionSummary, "version_summary", "generate", modelCfg.Model, 0, nil, err, nil)
		runErr = err
		return "", fmt.Errorf("生成版本说明失败: %w", err)
	}
	if msg == nil {
		runErr = fmt.Errorf("版本说明模型返回为空")
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindVersionSummary, "version_summary", "generate", modelCfg.Model, 0, nil, runErr, nil)
		return "", runErr
	}
	agentrun.FinishLLMCallTrace(span, callID, config.AgentKindVersionSummary, "version_summary", "generate", modelCfg.Model, 0, msg, nil, nil)
	summary := sanitizeVersionSummary(msg.Content)
	if summary == "" {
		runErr = fmt.Errorf("版本说明为空")
		return "", runErr
	}
	slog.InfoContext(ctx, fmt.Sprintf("[version-summary-agent] generate done summary=%q", summary))
	return summary, nil
}

func sanitizeVersionSummary(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	content = strings.Split(content, "\n")[0]
	content = strings.TrimSpace(content)
	content = strings.Trim(content, "`\"'“”‘’。；; ")
	runes := []rune(content)
	if len(runes) > 60 {
		content = string(runes[:60])
	}
	return strings.TrimSpace(content)
}
