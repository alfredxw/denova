package modeltask

import (
	"context"
	"denova/internal/agents/run"
	"fmt"
	"log/slog"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/modelio"
	"denova/internal/agents/prompts"
)

// GenerateAutomationTriggerEvaluation uses the owning Project Agent model to
// judge one bounded trigger context without constructing another Agent.
func GenerateAutomationTriggerEvaluation(ctx context.Context, cfg *config.Config, agentKind, instruction string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("configuration is required")
	}
	if agentKind != config.AgentKindIDE && agentKind != config.AgentKindGeneral {
		return "", fmt.Errorf("automation trigger evaluation requires a Project Agent kind")
	}
	var runErr error
	traceCtx, finishTrace := agentrun.WithStandaloneTrace(ctx, cfg, agentKind, "automation_trigger", "generate", map[string]any{
		"instruction_chars": len([]rune(instruction)),
	})
	defer func() { finishTrace(runErr) }()
	modelCfg, err := modelio.ConfigForAgent(cfg, agentKind)
	if err != nil {
		runErr = err
		return "", fmt.Errorf("resolve automation model configuration: %w", err)
	}
	modelCfg = modelio.WithJSONObjectOutput(modelCfg)
	cm, err := modelio.NewChatModel(traceCtx, modelCfg)
	if err != nil {
		runErr = err
		return "", fmt.Errorf("create automation trigger evaluation model: %w", err)
	}
	system := "你是 Denova 的自动化触发评估器。你的唯一任务是根据用户提供的有界创作上下文判断语义触发条件是否已经满足。不要使用工具，不要假设未给出的剧情，不要输出 JSON 以外的内容。"
	slog.InfoContext(ctx, fmt.Sprintf("[automation-trigger-agent] evaluate begin instruction=%s", prompts.PartSummary(instruction)))
	composition, err := prompts.ComposeBuiltinSystemInstruction(cfg, agentKind, "automation_trigger", cfg.Workspace, "builtin_base", "自动化触发评估规则", "define the bounded semantic trigger evaluation task", system)
	if err != nil {
		runErr = err
		return "", err
	}
	messages := []*agent.Message{
		agent.SystemMessage(composition.Instruction()),
		agent.UserMessage(instruction),
	}
	if err := modelio.ValidateConfiguredInput(cfg, agentKind, messages, nil); err != nil {
		runErr = err
		return "", err
	}
	span, callID, llmTraceCtx := agentrun.BeginLLMCallTrace(traceCtx, agentKind, "automation_trigger", "generate", modelCfg, messages, nil, false)
	msg, err := cm.Generate(llmTraceCtx, messages)
	if err != nil {
		agentrun.FinishLLMCallTrace(span, callID, agentKind, "automation_trigger", "generate", modelCfg.Model, 0, nil, err, nil)
		runErr = err
		return "", fmt.Errorf("生成自动化触发评估失败: %w", err)
	}
	if msg == nil {
		runErr = fmt.Errorf("automation trigger evaluation returned no message")
		agentrun.FinishLLMCallTrace(span, callID, agentKind, "automation_trigger", "generate", modelCfg.Model, 0, nil, runErr, nil)
		return "", runErr
	}
	agentrun.FinishLLMCallTrace(span, callID, agentKind, "automation_trigger", "generate", modelCfg.Model, 0, msg, nil, nil)
	slog.InfoContext(ctx, fmt.Sprintf("[automation-trigger-agent] evaluate done output=%s", prompts.PartSummary(msg.Content)))
	return msg.Content, nil
}
