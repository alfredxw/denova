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
	summarizeContextForCompaction = func(_ context.Context, _ *config.Config, _ string, source []*schema.Message, referenceContext string, _ int, _ contextCompactionPolicy) (string, error) {
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
	summarizeContextForCompaction = func(_ context.Context, _ *config.Config, _ string, _ []*schema.Message, _ string, _ int, policy contextCompactionPolicy) (string, error) {
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

func TestContextCompactionPolicyUsesCompactionAgentRetainedTurns(t *testing.T) {
	ideTurns := 3
	compactionTurns := 12
	cfg := &config.Config{AgentContexts: config.AgentContextSettings{
		IDE:               config.AgentContextOverride{CompactionRecentTurns: &ideTurns},
		ContextCompaction: config.AgentContextOverride{CompactionRecentTurns: &compactionTurns},
	}}

	policy := resolveContextCompactionPolicy(cfg, config.AgentKindIDE)
	if policy.RetainedRecentTurns != compactionTurns {
		t.Fatalf("retained turns = %d, want context_compaction setting %d", policy.RetainedRecentTurns, compactionTurns)
	}
}
