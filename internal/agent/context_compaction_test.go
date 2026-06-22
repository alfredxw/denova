package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"nova/config"
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
	summarizeContextForCompaction = func(_ context.Context, _ *config.Config, _ string, source []*schema.Message, referenceContext string, _ int, _ contextCompactionPolicy, _ func(int, string)) (string, error) {
		capturedSource = source
		capturedReference = referenceContext
		return "压缩摘要：保留用户意图。", nil
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
		ReferenceContext: "Story Memory: plot_summary",
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
	if capturedReference != "Story Memory: plot_summary" {
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
	summarizeContextForCompaction = func(_ context.Context, _ *config.Config, _ string, _ []*schema.Message, _ string, _ int, policy contextCompactionPolicy, _ func(int, string)) (string, error) {
		capturedPolicy = policy
		return "较完整的压缩摘要，保留用户目标、约束、事件和待办。", nil
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

func TestBuildContextCompactionEmitsStreamingSummaryDelta(t *testing.T) {
	previous := summarizeContextForCompaction
	defer func() { summarizeContextForCompaction = previous }()

	summarizeContextForCompaction = func(_ context.Context, _ *config.Config, _ string, _ []*schema.Message, _ string, _ int, _ contextCompactionPolicy, emitDelta func(int, string)) (string, error) {
		emitDelta(1, "第一段")
		emitDelta(1, "第二段")
		return "第一段第二段", nil
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
	cfg = &config.Config{AgentContexts: config.AgentContextSettings{
		ContextCompaction: config.AgentContextOverride{CompactionRecentTurns: &retainedTurns},
	}}
	policy = resolveContextCompactionPolicy(cfg, config.AgentKindIDE)
	if policy.RetainedTurns != 3 {
		t.Fatalf("retained turns = %d, want configured 3", policy.RetainedTurns)
	}
}
