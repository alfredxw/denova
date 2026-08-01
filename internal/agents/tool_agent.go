package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"

	"denova/config"
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
		return "", fmt.Errorf("配置不存在")
	}
	sample = strings.TrimSpace(sample)
	if sample == "" {
		return "", fmt.Errorf("样本为空")
	}
	var runErr error
	traceCtx, finishTrace := withStandaloneRunTrace(ctx, cfg, config.AgentKindToolAgent, "tool_agent_chapter_split_regex", "generate", map[string]any{
		"sample_chars": len([]rune(sample)),
	})
	defer func() { finishTrace(runErr) }()
	jsonModelCfg := chatModelConfigForAgent(cfg, config.AgentKindToolAgent)
	jsonModelCfg = withJSONObjectOutput(jsonModelCfg)
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
	plainModelCfg := chatModelConfigForAgent(cfg, config.AgentKindToolAgent)
	regex, retryErr := generateChapterSplitRegex(traceCtx, cfg, plainModelCfg, instruction, "plain_text_retry")
	if retryErr != nil {
		runErr = retryErr
		return "", retryErr
	}
	return regex, nil
}

func generateChapterSplitRegex(ctx context.Context, cfg *config.Config, modelCfg providers.ModelConfig, instruction, attempt string) (string, error) {
	slog.InfoContext(ctx, fmt.Sprintf("[tool-agent] chapter regex model config attempt=%s provider=%q protocol=%q model=%q base_url=%q max_tokens=%d json_mode=%t", attempt, modelCfg.Provider, modelCfg.Protocol, modelCfg.Model, modelCfg.BaseURL, valueOrZero(modelCfg.MaxOutputTokens), modelCfg.OutputFormat != nil))
	cm, err := newChatModel(ctx, modelCfg)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[tool-agent] create chapter regex model failed attempt=%s err=%v", attempt, err))
		return "", fmt.Errorf("创建工具 Agent 模型失败: %w", err)
	}
	composition, err := composeBuiltinSystemInstruction(cfg, config.AgentKindToolAgent, "tool_agent", cfg.Workspace, "builtin_base", "章节分割正则任务", "define the structured chapter-regex inference task", chapterSplitRegexSystemInstruction())
	if err != nil {
		return "", err
	}
	messages := []*agent.Message{
		agent.SystemMessage(composition.Instruction()),
		agent.UserMessage(instruction),
	}
	if err := validateConfiguredProviderInput(cfg, config.AgentKindToolAgent, messages, nil); err != nil {
		return "", err
	}
	mode := "generate_" + attempt
	span, callID, traceCtx := beginLLMCallTrace(ctx, config.AgentKindToolAgent, "tool_agent_chapter_split_regex", mode, modelCfg, messages, nil, false)
	msg, err := cm.Generate(traceCtx, messages)
	if err != nil {
		finishLLMCallTrace(span, callID, config.AgentKindToolAgent, "tool_agent_chapter_split_regex", mode, modelCfg.Model, 0, nil, err, nil)
		slog.ErrorContext(ctx, fmt.Sprintf("[tool-agent] infer chapter split regex generate failed attempt=%s err=%v", attempt, err))
		return "", fmt.Errorf("工具 Agent 推断章节正则失败: %w", err)
	}
	if msg == nil {
		finishLLMCallTrace(span, callID, config.AgentKindToolAgent, "tool_agent_chapter_split_regex", mode, modelCfg.Model, 0, nil, fmt.Errorf("工具 Agent 返回为空"), nil)
		slog.InfoContext(ctx, fmt.Sprintf("[tool-agent] infer chapter split regex nil response attempt=%s", attempt))
		return "", fmt.Errorf("工具 Agent 返回为空")
	}
	finishLLMCallTrace(span, callID, config.AgentKindToolAgent, "tool_agent_chapter_split_regex", mode, modelCfg.Model, 0, msg, nil, nil)
	slog.InfoContext(ctx, fmt.Sprintf("[tool-agent] infer chapter split regex raw output attempt=%s content=%s reasoning=%s", attempt, promptPartSummary(msg.Content), promptPartSummary(msg.ReasoningContent)))
	regex, reason, err := parseChapterSplitRegexContent(msg.Content)
	if err != nil && strings.TrimSpace(msg.Content) == "" && strings.TrimSpace(msg.ReasoningContent) != "" {
		slog.InfoContext(ctx, fmt.Sprintf("[tool-agent] content empty, try parse reasoning content attempt=%s", attempt))
		regex, reason, err = parseChapterSplitRegexContent(msg.ReasoningContent)
	}
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[tool-agent] parse chapter regex failed attempt=%s err=%v content=%s content_raw=%q reasoning=%s reasoning_raw=%q extracted_raw=%q",
			attempt,
			err,
			promptPartSummary(msg.Content),
			safeLogPreview(msg.Content, chapterSplitRegexFailureLogBytes),
			promptPartSummary(msg.ReasoningContent),
			safeLogPreview(msg.ReasoningContent, chapterSplitRegexFailureLogBytes),
			safeLogPreview(extractJSONContent(msg.Content), chapterSplitRegexFailureLogBytes),
		))
		return "", fmt.Errorf("解析工具 Agent 输出失败: %w", err)
	}
	slog.InfoContext(ctx, fmt.Sprintf("[tool-agent] infer chapter split regex done attempt=%s regex=%q reason=%s", attempt, regex, promptPartSummary(reason)))
	return regex, nil
}

func parseChapterSplitRegexContent(content string) (string, string, error) {
	var payload chapterSplitRegexPayload
	if err := json.Unmarshal([]byte(extractJSONContent(content)), &payload); err != nil {
		return "", "", err
	}
	regex := strings.TrimSpace(payload.SplitRegex)
	if regex == "" {
		return "", strings.TrimSpace(payload.Reason), fmt.Errorf("工具 Agent 未返回 split_regex")
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

func chapterSplitRegexSystemInstruction() string {
	return strings.Join([]string{
		"你负责为 Denova 小说导入识别章节和分卷标题行。",
		"只输出 JSON object，schema 为 {\"split_regex\":\"...\",\"reason\":\"...\"}。",
		"split_regex 必须是 Go regexp，可用于逐行匹配章节标题行和分卷标题行；不要使用跨行匹配。",
		"如果标题里有编号前缀和正文标题，优先用第 1 个捕获组捕获完整章节标题；否则不使用捕获组也可以。",
		"正则应尽量保守，只匹配章节/分卷标题行，不匹配普通正文句子。",
		"不要返回 Markdown、解释文本或代码块。",
	}, "\n")
}

func buildChapterSplitRegexInstruction(sample string) string {
	var sb strings.Builder
	sb.WriteString("请从以下小说开头样本的短行候选中推断章节/分卷标题行正则。\n")
	sb.WriteString("要求：返回 Go regexp；逐行匹配；如果存在分卷标题，正则也要匹配分卷标题；命中行少于 2 个章节/分卷标题的正则不可用；优先匹配候选里重复出现的标题格式，不要匹配普通正文短句；只输出 JSON。\n\n")
	sb.WriteString("候选上下文：\n")
	sb.WriteString(sample)
	return sb.String()
}
