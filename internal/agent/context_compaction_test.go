package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"denova/config"
)

func TestCompactionSourceExcludesReasoningCurrentUserAndOldSummary(t *testing.T) {
	messages := []*schema.Message{
		NewContextCompactionSummaryMessage(1, "旧摘要"),
		schema.UserMessage("上一轮用户"),
		schema.AssistantMessage("上一轮回复", nil),
		schema.UserMessage("当前用户"),
	}
	messages[1].ReasoningContent = "user thinking"
	messages[2].ReasoningContent = "assistant thinking"

	source := compactionSourceMessages(messages, false)
	if len(source) != 2 {
		t.Fatalf("source len = %d, want 2: %#v", len(source), source)
	}
	if source[0].Content != "上一轮用户" || source[1].Content != "上一轮回复" {
		t.Fatalf("unexpected source transcript: %#v", source)
	}
	for _, msg := range source {
		if strings.TrimSpace(msg.ReasoningContent) != "" {
			t.Fatalf("reasoning content should be stripped: %#v", msg)
		}
	}
}

func TestBuildContextCompactionUsesExplicitSourceTranscript(t *testing.T) {
	previous := summarizeContextForCompaction
	defer func() { summarizeContextForCompaction = previous }()

	var capturedSource []*schema.Message
	var capturedReference string
	summarizeContextForCompaction = func(_ context.Context, _ *config.Config, _ string, _ string, source []*schema.Message, referenceContext string, _ int, _ contextCompactionPolicy, _ func(int, string)) (string, int, error) {
		capturedSource = source
		capturedReference = referenceContext
		return "压缩摘要：保留用户意图。", 100, nil
	}

	modelMessages := []*schema.Message{
		schema.UserMessage("当前模型指令"),
	}
	sourceMessages := []*schema.Message{
		schema.UserMessage("原始用户行动"),
		schema.AssistantMessage("原始剧情正文", nil),
	}
	sourceMessages[1].ReasoningContent = "剧情 thinking 不应进入压缩源"

	newMessages, result, err := BuildContextCompaction(context.Background(), &config.Config{}, config.AgentKindInteractiveStory, ContextCompactionInput{
		Messages:         modelMessages,
		SourceMessages:   sourceMessages,
		ReferenceContext: "Lore: plot_summary",
		Force:            true,
		KeepLatestUser:   true,
	}, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered || result.Epoch != 7 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(capturedSource) != 2 || capturedSource[0].Content != "原始用户行动" || capturedSource[1].Content != "原始剧情正文" {
		t.Fatalf("explicit source transcript was not used: %#v", capturedSource)
	}
	if capturedSource[1].ReasoningContent != "" {
		t.Fatalf("reasoning content should not reach compaction model: %#v", capturedSource[1])
	}
	if capturedReference != "Lore: plot_summary" {
		t.Fatalf("reference context = %q", capturedReference)
	}
	if len(newMessages) != 2 || !isContextCompactionMessage(newMessages[0]) || newMessages[1].Content != "当前模型指令" {
		t.Fatalf("unexpected compacted model messages: %#v", newMessages)
	}
}

func TestBuildContextCompactionUsesContextCompactionTargetRange(t *testing.T) {
	previous := summarizeContextForCompaction
	defer func() { summarizeContextForCompaction = previous }()

	var capturedPolicy contextCompactionPolicy
	summarizeContextForCompaction = func(_ context.Context, _ *config.Config, _ string, _ string, _ []*schema.Message, _ string, _ int, policy contextCompactionPolicy, _ func(int, string)) (string, int, error) {
		capturedPolicy = policy
		return "较完整的压缩摘要，保留用户目标、约束、事件和待办。", 100, nil
	}

	minRatio := 0.12
	maxRatio := 0.35
	cfg := &config.Config{AgentContexts: config.AgentContextSettings{
		ContextCompaction: config.AgentContextOverride{
			CompactionTargetMin: &minRatio,
			CompactionTargetMax: &maxRatio,
		},
	}}
	_, _, err := BuildContextCompaction(context.Background(), cfg, config.AgentKindIDE, ContextCompactionInput{
		Messages: []*schema.Message{
			schema.UserMessage("用户说了很多重要要求"),
			schema.AssistantMessage("助手完成了一些重要工作", nil),
		},
		Force:          true,
		KeepLatestUser: true,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if capturedPolicy.TargetMinRatio != minRatio || capturedPolicy.TargetMaxRatio != maxRatio {
		t.Fatalf("target range = %.2f-%.2f, want %.2f-%.2f", capturedPolicy.TargetMinRatio, capturedPolicy.TargetMaxRatio, minRatio, maxRatio)
	}
}

func TestBuildContextCompactionTriggersOnProjectedNinetyPercentUsage(t *testing.T) {
	previous := summarizeContextForCompaction
	defer func() { summarizeContextForCompaction = previous }()
	summarizeContextForCompaction = func(_ context.Context, _ *config.Config, _ string, _ string, _ []*schema.Message, _ string, _ int, _ contextCompactionPolicy, _ func(int, string)) (string, int, error) {
		return "压缩后的事实摘要。", 100, nil
	}

	cfg := &config.Config{OpenAIContextWindowTokens: 1000}
	messages := []*schema.Message{
		schema.UserMessage("上一轮用户行动"),
		schema.AssistantMessage("上一轮剧情结果", nil),
		schema.UserMessage("当前用户行动"),
	}
	_, result, err := BuildContextCompaction(context.Background(), cfg, config.AgentKindInteractiveStory, ContextCompactionInput{
		Messages:                 messages,
		ReservedCompletionTokens: 850,
		KeepLatestUser:           true,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.TokensBefore >= 900 {
		t.Fatalf("raw prompt should be below threshold for this test: %#v", result)
	}
	if !result.Triggered || result.ProjectedTokensBefore < 900 {
		t.Fatalf("projected prompt + completion reserve should trigger at 90%%: %#v", result)
	}
}

func TestBuildContextCompactionEmitsStreamingSummaryDelta(t *testing.T) {
	previous := summarizeContextForCompaction
	defer func() { summarizeContextForCompaction = previous }()

	summarizeContextForCompaction = func(_ context.Context, _ *config.Config, _ string, _ string, _ []*schema.Message, _ string, _ int, _ contextCompactionPolicy, emitDelta func(int, string)) (string, int, error) {
		emitDelta(1, "第一段")
		emitDelta(1, "第二段")
		return "第一段第二段", 100, nil
	}

	var events []Event
	_, result, err := BuildContextCompaction(context.Background(), &config.Config{}, config.AgentKindIDE, ContextCompactionInput{
		Messages: []*schema.Message{
			schema.UserMessage("用户提出了一个很长的需求"),
			schema.AssistantMessage("助手完成了很多上下文相关工作", nil),
		},
		Force:          true,
		KeepLatestUser: true,
		Emit:           func(event Event) { events = append(events, event) },
	}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered {
		t.Fatalf("expected compaction to trigger: %#v", result)
	}

	var deltas []string
	for _, event := range events {
		if event.Type != "context_compaction" {
			continue
		}
		data, ok := event.Data.(map[string]any)
		if !ok || data["status"] != "delta" {
			continue
		}
		if data["attempt"] != 1 {
			t.Fatalf("delta attempt = %#v, want 1", data["attempt"])
		}
		deltas = append(deltas, data["delta"].(string))
	}
	if strings.Join(deltas, "") != "第一段第二段" {
		t.Fatalf("delta stream = %q", strings.Join(deltas, ""))
	}
}

func TestContextCompactionPolicyUsesConfiguredRetainedTurns(t *testing.T) {
	cfg := &config.Config{}

	policy := resolveContextCompactionPolicy(cfg, config.AgentKindIDE)
	if policy.RetainedTurns != config.DefaultContextCompactionRetainedTurns {
		t.Fatalf("retained turns = %d, want default %d", policy.RetainedTurns, config.DefaultContextCompactionRetainedTurns)
	}

	retainedTurns := 3
	strategy := config.AgentContextCompactionStrategySummaryAgent
	cfg = &config.Config{AgentContexts: config.AgentContextSettings{
		IDE:               config.AgentContextOverride{CompactionStrategy: &strategy},
		ContextCompaction: config.AgentContextOverride{CompactionRecentTurns: &retainedTurns},
	}}
	policy = resolveContextCompactionPolicy(cfg, config.AgentKindIDE)
	if policy.RetainedTurns != 3 {
		t.Fatalf("retained turns = %d, want configured 3", policy.RetainedTurns)
	}
	if policy.Strategy != config.AgentContextCompactionStrategySummaryAgent {
		t.Fatalf("strategy = %q, want summary_agent", policy.Strategy)
	}
}

func TestBuildContextCompactionTranscriptKeepsAllIncrementalMessagesAndReferenceContext(t *testing.T) {
	messages := make([]*schema.Message, 0, 40)
	for i := 1; i <= 40; i++ {
		messages = append(messages, schema.UserMessage(strings.Repeat("旧消息", 2000)+":"+string(rune('A'+i%26))))
	}
	policy := contextCompactionPolicy{TargetMinRatio: 0.10, TargetMaxRatio: 0.25}
	existing := "既有压缩摘要：主角进入旧城。"
	reference := "有界参考上下文：关系=信任；任务=寻找钥匙。"
	inputChars := contextCompactionInputChars(existing, messages, reference)
	transcript := buildContextCompactionTranscript(messages, existing, reference, 1234, inputChars, "", policy)

	if strings.Contains(transcript, "omitted") || strings.Contains(transcript, "已截断") {
		t.Fatalf("compaction transcript should not report omitted content:\n%s", transcript[:200])
	}
	if !strings.Contains(transcript, existing) || !strings.Contains(transcript, reference) {
		t.Fatalf("transcript should include existing checkpoint and reference context:\n%s", transcript)
	}
	if !strings.Contains(transcript, "--- message 1 role=user ---") || !strings.Contains(transcript, "--- message 40 role=user ---") {
		t.Fatalf("transcript should include the full incremental message range")
	}
	minChars, maxChars := compactionTargetCharRange(inputChars, policy)
	wantRange := fmt.Sprintf("Target summary length: %d-%d characters", minChars, maxChars)
	if !strings.Contains(transcript, wantRange) {
		t.Fatalf("transcript missing character range %q:\n%s", wantRange, transcript[:300])
	}
}

func TestContextCompactionRetryInstructionExpandsTooShortSummary(t *testing.T) {
	got := contextCompactionRetryInstruction(80, 300, 900)
	if !strings.Contains(got, "too short: 80 characters") || !strings.Contains(got, "300-900 characters") || !strings.Contains(got, "Expand") {
		t.Fatalf("unexpected retry instruction: %s", got)
	}
}
