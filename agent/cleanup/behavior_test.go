package cleanup

import (
	"context"
	"fmt"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestStandardProtectsErrorsRecentAndPendingToolGroups(t *testing.T) {
	messages := behaviorHistory(8, 7_000, agent.ToolResultDeferred, agent.ToolResultContextNormal)
	messages[3].ToolResult.Status = agent.ToolResultError
	config := behaviorConfig(120_000)
	config.CacheState = CacheCold
	plan := behaviorPlan(t, messages, config)
	if plan.Action != agent.CleanupProject || plan.Metrics.ProtectedResults == 0 {
		t.Fatalf("error/recent protection plan=%#v", plan)
	}
	for _, replacement := range plan.Replacements {
		if replacement.MessageIndex == 3 || replacement.MessageIndex >= len(messages)-11 {
			t.Fatalf("protected or recent result selected: %#v", replacement)
		}
	}

	pending := []*agent.Message{
		agent.UserMessage("analyze this"),
		agent.AssistantMessage("", []agent.ToolCall{behaviorCall("pending", 0)}),
		behaviorResult("pending", strings.Repeat("pending evidence ", 30_000), agent.ToolResultDeferred, agent.ToolResultContextNormal),
	}
	pendingPlan := behaviorPlan(t, pending, behaviorConfig(80_000))
	if len(pendingPlan.Replacements) != 0 {
		t.Fatalf("unconsumed tool result was cleanup-eligible: %#v", pendingPlan)
	}
}

func TestStandardKeepsParallelToolBatchAtomic(t *testing.T) {
	messages := []*agent.Message{
		agent.UserMessage("inspect both"),
		agent.AssistantMessage("", []agent.ToolCall{behaviorCall("first", 0), behaviorCall("second", 1)}),
		behaviorResult("first", strings.Repeat("large result ", 20_000), agent.ToolResultDeferred, agent.ToolResultContextDiscardable),
		agent.AssistantMessage("partial result consumed", nil),
		agent.UserMessage("continue"),
	}
	config := behaviorConfig(40_000)
	config.CacheState = CacheCold
	plan := behaviorPlan(t, messages, config)
	if len(plan.Replacements) != 0 {
		t.Fatalf("incomplete parallel batch was cleanup-eligible: %#v", plan)
	}
}

func TestStandardUsesSharedWarmCacheGateAndColdRecoveryTarget(t *testing.T) {
	messages := behaviorHistory(10, 9_000, agent.ToolResultDeferred, agent.ToolResultContextDiscardable)
	messages = append(messages, agent.UserMessage(strings.Repeat("warm-suffix ", 12_000)))
	config := behaviorConfig(240_000)
	config.CacheState = CacheUnknown
	warm := behaviorPlan(t, messages, config)
	if warm.Action != agent.CleanupCompact || warm.Metrics.SkippedWarmSuffixCount == 0 || warm.Metrics.CacheViableCandidateTokens != 0 {
		t.Fatalf("warm cache gate plan=%#v", warm)
	}

	config.CacheState = CacheCold
	cold := behaviorPlan(t, messages, config)
	if cold.Action != agent.CleanupProject || cold.Metrics.ReclaimedTokens < cold.Metrics.MinimumCleanupTokens ||
		cold.Metrics.BodyPressureAfter > config.CleanupTarget {
		t.Fatalf("cold cache recovery plan=%#v", cold)
	}
	if len(cold.Replacements) == 0 || cold.Metrics.EarliestChanged != cold.Replacements[0].MessageIndex {
		t.Fatalf("cold cache mutation boundary=%#v", cold)
	}
}

func TestPlaceholderIsDeterministicAndNeverCopiesRawArguments(t *testing.T) {
	call := behaviorCall("call-1", 1)
	call.Function.Arguments = `{"authorization":"secret","path":"never-copy-this"}`
	message := behaviorResult("call-1", strings.Repeat("body", 2_000), agent.ToolResultDeferred, agent.ToolResultContextNormal)
	first, ok := renderPlaceholder(call, message, "call-2")
	if !ok {
		t.Fatal("render placeholder")
	}
	second, ok := renderPlaceholder(call, message, "call-2")
	if !ok || first != second {
		t.Fatalf("placeholder is not deterministic: first=%q second=%q", first, second)
	}
	if strings.Contains(first, "secret") || strings.Contains(first, "never-copy-this") ||
		!strings.Contains(first, "Superseded by tool call: call-2") {
		t.Fatalf("placeholder leaked arguments or lost metadata: %s", first)
	}
	if RendererVersion != "tool_result.placeholder.v1" {
		t.Fatalf("renderer version=%q", RendererVersion)
	}

	message.ToolResult.ContextHints.Recovery.Reference = map[string]any{"url": "[REDACTED]"}
	if _, ok := renderPlaceholder(call, message, ""); ok || !toolResultProtected(message) {
		t.Fatal("redacted-only recovery metadata authorized Cleanup")
	}
}

func TestAttachmentArtifactCannotAuthorizeCleanup(t *testing.T) {
	message := behaviorResult("attachment", strings.Repeat("body", 2_000), agent.ToolResultDeferred, agent.ToolResultContextNormal)
	path := ".denova/artifacts/attachment.png"
	message.ToolResult.ContextHints.Recovery = agent.ToolResultRecoveryHint{
		Kind: agent.ToolResultRecoveryArtifact, ArtifactPath: path,
	}
	message.ToolResult.Artifacts = []agent.ToolArtifactRef{{
		ID: "attachment", Purpose: agent.ToolArtifactPurposeAttachment,
		ReadablePath: path, ContentType: "image/png", Complete: true,
	}}
	if _, ok := renderPlaceholder(behaviorCall("attachment", 0), message, ""); ok || !toolResultProtected(message) {
		t.Fatal("attachment artifact authorized Cleanup")
	}
	message.ToolResult.Artifacts[0].Purpose = agent.ToolArtifactPurposeCompleteModelOutput
	if _, ok := renderPlaceholder(behaviorCall("attachment", 0), message, ""); !ok {
		t.Fatal("complete model-output artifact did not authorize Cleanup")
	}
}

func TestSelectionPrefersSupersededThenLargerResults(t *testing.T) {
	oldResult := behaviorResult("old", strings.Repeat("obsolete ", 5_000), agent.ToolResultDeferred, agent.ToolResultContextNormal)
	newResult := behaviorResult("new", strings.Repeat("current ", 5_000), agent.ToolResultDeferred, agent.ToolResultContextNormal)
	ordinaryResult := behaviorResult("ordinary", strings.Repeat("evidence ", 5_000), agent.ToolResultDeferred, agent.ToolResultContextNormal)
	oldResult.ToolResult.ContextHints.SupersessionKey = "read:same-target"
	newResult.ToolResult.ContextHints.SupersessionKey = "read:same-target"
	ordinaryResult.ToolResult.ContextHints.SupersessionKey = "read:other-target"
	messages := []*agent.Message{
		agent.UserMessage("old"), agent.AssistantMessage("", []agent.ToolCall{behaviorCall("old", 0)}), oldResult, agent.AssistantMessage("used old", nil),
		agent.UserMessage("new"), agent.AssistantMessage("", []agent.ToolCall{behaviorCall("new", 1)}), newResult, agent.AssistantMessage("used new", nil),
		agent.UserMessage("ordinary"), agent.AssistantMessage("", []agent.ToolCall{behaviorCall("ordinary", 2)}), ordinaryResult, agent.AssistantMessage("used ordinary", nil),
		agent.UserMessage("current turn"),
	}
	config := behaviorConfig(200_000)
	config.CacheState, config.KeepRecentGroups, config.KeepRecentTokens = CacheCold, 0, 0
	groups, _ := collectGroups(messages, config)
	prepareReplacements(messages, groups)
	selected := selectGroups(messages, groups, 12_000, 0, 12_000, config, false)
	if len(selected) == 0 || messages[selected[0].resultIndexes[0]].ToolCallID != "old" {
		t.Fatalf("superseded result was not selected first: %#v", selected)
	}

	large := behaviorResult("large", strings.Repeat("large ", 6_000), agent.ToolResultDeferred, agent.ToolResultContextNormal)
	small := behaviorResult("small", strings.Repeat("small ", 800), agent.ToolResultDeferred, agent.ToolResultContextNormal)
	large.ToolResult.ContextHints.SupersessionKey = "read:large"
	small.ToolResult.ContextHints.SupersessionKey = "read:small"
	messages = []*agent.Message{
		agent.UserMessage("large"), agent.AssistantMessage("", []agent.ToolCall{behaviorCall("large", 0)}), large, agent.AssistantMessage("used large", nil),
		agent.UserMessage("small"), agent.AssistantMessage("", []agent.ToolCall{behaviorCall("small", 1)}), small, agent.AssistantMessage("used small", nil),
		agent.UserMessage("current turn"),
	}
	groups, _ = collectGroups(messages, config)
	prepareReplacements(messages, groups)
	selected = selectGroups(messages, groups, 6_000, 0, 6_000, config, false)
	if len(selected) == 0 || messages[selected[0].resultIndexes[0]].ToolCallID != "large" {
		t.Fatalf("largest same-tier result was not selected first: %#v", selected)
	}
}

func behaviorPlan(t testing.TB, messages []*agent.Message, config StandardConfig) agent.CleanupPlan {
	t.Helper()
	manager := standardForTest(t, config)
	plan, err := manager.Plan(context.Background(), agent.CleanupPlanRequest{
		Messages: messages, ModelRequest: messages, CompactionAvailable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func behaviorConfig(window int) StandardConfig {
	return StandardConfig{
		Scope: PressureTotal, ContextWindowTokens: window,
		CleanupThreshold: .70, CleanupTarget: .60, CleanupMinTokens: 2_000,
		KeepRecentGroups: 3, KeepRecentTokens: 1_000, WarmSuffixTokens: 8_000,
		EagerMinTokens: 20_000, EagerMinContextRatio: .10,
		CompactionEnabled: true, CompactionThreshold: .85,
	}
}

func behaviorHistory(groups, resultWords int, retention agent.ToolResultRetentionMode, value agent.ToolResultContextValue) []*agent.Message {
	messages := []*agent.Message{agent.SystemMessage("stable system")}
	for index := 0; index < groups; index++ {
		callID := fmt.Sprintf("call-%d", index)
		messages = append(messages,
			agent.UserMessage(fmt.Sprintf("request %d", index)),
			agent.AssistantMessage("", []agent.ToolCall{behaviorCall(callID, index)}),
			behaviorResult(callID, fmt.Sprintf("rich-result-%d ", index)+strings.Repeat("payload ", resultWords), retention, value),
			agent.AssistantMessage(fmt.Sprintf("used result %d", index), nil),
		)
	}
	return append(messages, agent.UserMessage("current turn"))
}

func behaviorCall(callID string, index int) agent.ToolCall {
	return agent.ToolCall{
		ID: callID, Type: "function",
		Function: agent.FunctionCall{Name: "read", Arguments: fmt.Sprintf(`{"path":"chapter-%d.md"}`, index)},
	}
}

func behaviorResult(callID, content string, retention agent.ToolResultRetentionMode, value agent.ToolResultContextValue) *agent.Message {
	result := agent.TextToolResult(content)
	result.ResultRetention = retention
	result.ContextHints = &agent.ToolResultContextHints{
		Recovery: agent.ToolResultRecoveryHint{
			Kind: agent.ToolResultRecoveryRead,
			Reference: map[string]any{
				"path": ".denova/artifacts/session/" + callID + ".log",
			},
			EstimatedBytes: int64(len(content)), EstimatedTokens: EstimateStringTokens(content),
		},
		ContextValue: value, SupersessionKey: "read:chapter.md",
	}
	return agent.ToolMessage(result, callID, agent.WithToolName("read"))
}
