package modeltask

import (
	"context"
	"denova/internal/agents/run"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"

	"denova/config"
	"denova/internal/agents/modelio"
	"denova/internal/agents/prompts"
)

type chapterSplitRegexPayload struct {
	SplitRegex string `json:"split_regex"`
	Reason     string `json:"reason,omitempty"`
}

const (
	chapterSplitRegexFailureLogBytes = 32768
)

// InferChapterSplitRegex asks the model-only Tool Agent to infer a line-level Go regexp for chapter titles.
func InferChapterSplitRegex(ctx context.Context, cfg *config.Config, sample string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("configuration is missing")
	}
	sample = strings.TrimSpace(sample)
	if sample == "" {
		return "", fmt.Errorf("sample is empty")
	}
	var runErr error
	traceCtx, finishTrace := agentrun.WithStandaloneTrace(ctx, cfg, config.AgentKindToolAgent, "tool_agent_chapter_split_regex", "generate", map[string]any{
		"sample_chars": len([]rune(sample)),
	})
	defer func() { finishTrace(runErr) }()
	jsonModelCfg, err := modelio.ConfigForAgent(cfg, config.AgentKindToolAgent)
	if err != nil {
		runErr = err
		return "", fmt.Errorf("resolve tool Agent model configuration: %w", err)
	}
	jsonModelCfg = modelio.WithJSONObjectOutput(jsonModelCfg)
	instruction := buildChapterSplitRegexInstruction(sample)
	slog.InfoContext(ctx, fmt.Sprintf("[tool-agent] infer chapter split regex begin sample_chars=%d", len([]rune(sample))))
	regex, err := generateChapterSplitRegex(traceCtx, cfg, jsonModelCfg, instruction, "json_mode")
	if err == nil {
		return regex, nil
	}
	if traceCtx.Err() != nil {
		runErr = err
		return "", err
	}
	slog.ErrorContext(ctx, fmt.Sprintf("[tool-agent] json_mode failed, retry without response_format err=%v", err))
	plainModelCfg, configErr := modelio.ConfigForAgent(cfg, config.AgentKindToolAgent)
	if configErr != nil {
		runErr = configErr
		return "", fmt.Errorf("resolve tool Agent fallback model configuration: %w", configErr)
	}
	regex, retryErr := generateChapterSplitRegex(traceCtx, cfg, plainModelCfg, instruction, "plain_text_retry")
	if retryErr != nil {
		runErr = retryErr
		return "", retryErr
	}
	return regex, nil
}

func generateChapterSplitRegex(ctx context.Context, cfg *config.Config, modelCfg providers.ModelConfig, instruction, attempt string) (string, error) {
	slog.InfoContext(ctx, fmt.Sprintf("[tool-agent] chapter regex model config attempt=%s provider=%q protocol=%q model=%q base_url=%q max_tokens=%d json_mode=%t", attempt, modelCfg.Provider, modelCfg.Protocol, modelCfg.Model, modelCfg.BaseURL, valueOrZero(modelCfg.MaxOutputTokens), modelCfg.OutputFormat != nil))
	cm, err := modelio.NewChatModel(ctx, modelCfg)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[tool-agent] create chapter regex model failed attempt=%s err=%v", attempt, err))
		return "", fmt.Errorf("create Tool Agent model: %w", err)
	}
	composition, err := prompts.ComposeBuiltinSystemInstruction(cfg, config.AgentKindToolAgent, "tool_agent", cfg.Workspace, "builtin_base", "Chapter Split Regex Task", "define the structured chapter-regex inference task", prompts.ChapterSplitRegexSystemInstruction())
	if err != nil {
		return "", err
	}
	messages := []*agent.Message{
		agent.SystemMessage(composition.Instruction()),
		agent.UserMessage(instruction),
	}
	if err := modelio.ValidateConfiguredInput(cfg, config.AgentKindToolAgent, messages, nil); err != nil {
		return "", err
	}
	mode := "generate_" + attempt
	span, callID, traceCtx := agentrun.BeginLLMCallTrace(ctx, config.AgentKindToolAgent, "tool_agent_chapter_split_regex", mode, modelCfg, messages, nil, false)
	msg, err := cm.Generate(traceCtx, messages)
	if err != nil {
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindToolAgent, "tool_agent_chapter_split_regex", mode, modelCfg.Model, 0, nil, err, nil)
		slog.ErrorContext(ctx, fmt.Sprintf("[tool-agent] infer chapter split regex generate failed attempt=%s err=%v", attempt, err))
		return "", fmt.Errorf("Tool Agent chapter-regex inference: %w", err)
	}
	if msg == nil {
		agentrun.FinishLLMCallTrace(span, callID, config.AgentKindToolAgent, "tool_agent_chapter_split_regex", mode, modelCfg.Model, 0, nil, fmt.Errorf("Tool Agent returned an empty response"), nil)
		slog.InfoContext(ctx, fmt.Sprintf("[tool-agent] infer chapter split regex nil response attempt=%s", attempt))
		return "", fmt.Errorf("Tool Agent returned an empty response")
	}
	agentrun.FinishLLMCallTrace(span, callID, config.AgentKindToolAgent, "tool_agent_chapter_split_regex", mode, modelCfg.Model, 0, msg, nil, nil)
	slog.InfoContext(ctx, fmt.Sprintf("[tool-agent] infer chapter split regex raw output attempt=%s content=%s reasoning=%s", attempt, prompts.PartSummary(msg.Content), prompts.PartSummary(msg.ReasoningContent)))
	regex, reason, err := parseChapterSplitRegexContent(msg.Content)
	if err != nil && strings.TrimSpace(msg.Content) == "" && strings.TrimSpace(msg.ReasoningContent) != "" {
		slog.InfoContext(ctx, fmt.Sprintf("[tool-agent] content empty, try parse reasoning content attempt=%s", attempt))
		regex, reason, err = parseChapterSplitRegexContent(msg.ReasoningContent)
	}
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[tool-agent] parse chapter regex failed attempt=%s err=%v content=%s content_raw=%q reasoning=%s reasoning_raw=%q extracted_raw=%q",
			attempt,
			err,
			prompts.PartSummary(msg.Content),
			modelio.LogPreview(msg.Content, chapterSplitRegexFailureLogBytes),
			prompts.PartSummary(msg.ReasoningContent),
			modelio.LogPreview(msg.ReasoningContent, chapterSplitRegexFailureLogBytes),
			modelio.LogPreview(extractJSONContent(msg.Content), chapterSplitRegexFailureLogBytes),
		))
		return "", fmt.Errorf("parse Tool Agent output: %w", err)
	}
	slog.InfoContext(ctx, fmt.Sprintf("[tool-agent] infer chapter split regex done attempt=%s regex=%q reason=%s", attempt, regex, prompts.PartSummary(reason)))
	return regex, nil
}

func parseChapterSplitRegexContent(content string) (string, string, error) {
	var payload chapterSplitRegexPayload
	if err := json.Unmarshal([]byte(extractJSONContent(content)), &payload); err != nil {
		return "", "", err
	}
	regex := strings.TrimSpace(payload.SplitRegex)
	if regex == "" {
		return "", strings.TrimSpace(payload.Reason), fmt.Errorf("Tool Agent did not return split_regex")
	}
	return regex, strings.TrimSpace(payload.Reason), nil
}

func extractJSONContent(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSpace(content)
		content = strings.TrimSuffix(content, "```")
	}
	return strings.TrimSpace(content)
}

func valueOrZero(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func buildChapterSplitRegexInstruction(sample string) string {
	var sb strings.Builder
	sb.WriteString("Infer a regular expression for chapter or volume title lines from the short-line candidates in the opening novel sample below.\n")
	sb.WriteString("Requirements: return a Go regexp; match line by line; include volume titles when present; reject a regex that matches fewer than two chapter or volume titles; prefer repeated title formats in the candidates and do not match ordinary short prose sentences; output JSON only.\n\n")
	sb.WriteString("Candidate context:\n")
	sb.WriteString(sample)
	return sb.String()
}
