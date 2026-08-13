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
		return "", fmt.Errorf("configuration is missing")
	}
	var runErr error
	traceCtx, finishTrace := agentrun.WithStandaloneTrace(ctx, cfg, config.AgentKindVersionSummary, "version_summary", "generate", map[string]any{
		"instruction_chars": len([]rune(instruction)),
	})
	defer func() { finishTrace(runErr) }()
	modelCfg, err := modelio.ConfigForAgent(cfg, config.AgentKindVersionSummary)
	if err != nil {
		runErr = err
		return "", fmt.Errorf("resolve version summary model configuration: %w", err)
	}
	cm, err := modelio.NewChatModel(traceCtx, modelCfg)
	if err != nil {
		runErr = err
		return "", fmt.Errorf("create version-summary model: %w", err)
	}
	slog.InfoContext(ctx, fmt.Sprintf("[version-summary-agent] generate begin instruction=%s", prompts.PartSummary(instruction)))
	composition, err := prompts.ComposeBuiltinSystemInstruction(cfg, config.AgentKindVersionSummary, "version_summary", cfg.Workspace, "builtin_base", "Version Summary Generation Rules", "define the version summary task and output constraint", "You are Denova's version-summary generator. Infer the core creative change in this save from the file changes. Output exactly one Chinese version summary of 10 to 30 Han characters. Do not include numbering, quotation marks, a colon, a final period, or any explanation.")
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
		return "", fmt.Errorf("generate version summary: %w", err)
	}
	if msg == nil {
		runErr = fmt.Errorf("version-summary model returned an empty response")
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindVersionSummary, "version_summary", "generate", modelCfg.Model, 0, nil, runErr, nil)
		return "", runErr
	}
	agentrun.FinishLLMCallTrace(span, callID, config.AgentKindVersionSummary, "version_summary", "generate", modelCfg.Model, 0, msg, nil, nil)
	summary := sanitizeVersionSummary(msg.Content)
	if summary == "" {
		runErr = fmt.Errorf("version summary is empty")
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
