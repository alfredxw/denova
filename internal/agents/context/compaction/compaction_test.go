package compaction

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcontext "denova/internal/agents/context"
)

func TestCompactionSourceExcludesReasoningCurrentUserAndOldSummary(t *testing.T) {
	messages := []*agent.Message{
		agentcontext.NewCompactionSummaryMessage(1, "旧摘要"),
		agent.UserMessage("上一轮用户"),
		agent.AssistantMessage("上一轮回复", nil),
		agent.UserMessage("当前用户"),
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

func TestContextProjectionReserveUsesSingleToolResultBoundary(t *testing.T) {
	cfg := &config.Config{
		OpenAIContextWindowTokens: 1_000_000,
		AgentToolResultLimitKB:    192,
	}
	_, toolTokens := EstimateProjectionReserves(cfg, config.AgentKindIDE, 0)
	if want := 192 * 1024 / 3; toolTokens != want {
		t.Fatalf("tool result reserve = %d, want configured single-result boundary %d", toolTokens, want)
	}

	_, disabledTokens := EstimateProjectionReserves(cfg, config.AgentKindAutomation, 0)
	if disabledTokens != 0 {
		t.Fatalf("agent without cross-turn tool retention should reserve 0 tokens, got %d", disabledTokens)
	}
}

func TestBuildContextCompactionUsesExplicitSourceTranscript(t *testing.T) {
	var capturedRequest SummaryRequest
	summarize := func(_ context.Context, _ *config.Config, request SummaryRequest, _ func(int, string)) (string, error) {
		capturedRequest = request
		return "压缩摘要：保留用户意图。", nil
	}

	modelMessages := []*agent.Message{
		agent.UserMessage(strings.Repeat("旧模型历史", 80)),
		agent.AssistantMessage(strings.Repeat("旧模型结果", 80), nil),
		agent.UserMessage("当前模型指令"),
	}
	sourceMessages := []*agent.Message{
		agent.UserMessage("原始用户行动"),
		agent.AssistantMessage("原始剧情正文", nil),
	}
	sourceMessages[1].ReasoningContent = "剧情 thinking 不应进入压缩源"

	newMessages, result, err := Prepare(context.Background(), &config.Config{}, config.AgentKindInteractiveStory, coldTestInput(Input{
		Messages:         modelMessages,
		SourceMessages:   sourceMessages,
		ReferenceContext: "Lore: plot_summary",
		Force:            true,
		KeepLatestUser:   true,
	}, summarize), 7)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered || result.Epoch != 7 {
		t.Fatalf("unexpected result: %#v", result)
	}
	transcript := messageText(capturedRequest.Messages)
	if !strings.Contains(transcript, "原始用户行动") || !strings.Contains(transcript, "原始剧情正文") ||
		!strings.Contains(transcript, "Lore: plot_summary") || strings.Contains(transcript, "剧情 thinking") {
		t.Fatalf("explicit source was not assembled cleanly:\n%s", transcript)
	}
	if len(newMessages) != 2 || !agentcontext.IsCompactionSummaryMessage(newMessages[0]) || newMessages[1].Content != "当前模型指令" {
		t.Fatalf("unexpected compacted model messages: %#v", newMessages)
	}
}

func TestContextCompactionExplicitEmptySourceNeverAbsorbsPendingTail(t *testing.T) {
	called := false
	summarize := func(_ context.Context, _ *config.Config, _ SummaryRequest, _ func(int, string)) (string, error) {
		called = true
		return "must not run", nil
	}

	messages := []*agent.Message{agentcontext.NewCompactionSummaryMessage(3, "completed boundary already checkpointed")}
	for index := 0; index < 4; index++ {
		messages = append(messages, agent.UserMessage(fmt.Sprintf("pending interrupted input %d", index)))
		messages = append(messages, largePendingToolBatchForCompactionTest(index)...)
	}
	compacted, result, err := Prepare(context.Background(), &config.Config{}, config.AgentKindInteractiveStory, Input{
		Messages:           messages,
		SourceMessages:     nil,
		SourceMessagesSet:  true,
		ExistingCheckpoint: "completed boundary already checkpointed",
		Force:              true,
		KeepLatestUser:     true,
		Summarize:          summarize,
	}, 4)
	if err != nil {
		t.Fatal(err)
	}
	if called || result.Triggered || result.SkippedReason != "empty_source" {
		t.Fatalf("explicit empty source was not fail-closed: called=%t result=%#v", called, result)
	}
	if !reflect.DeepEqual(compacted, messages) {
		t.Fatalf("pending tail changed on empty-source compaction:\nwant=%#v\ngot=%#v", messages, compacted)
	}
}

func TestContextCompactionRejectsPendingDominatedProjectionAboveHardBand(t *testing.T) {
	summarize := func(_ context.Context, _ *config.Config, _ SummaryRequest, _ func(int, string)) (string, error) {
		return "small completed-history checkpoint", nil
	}

	source := []*agent.Message{
		agent.UserMessage(strings.Repeat("old removable turn ", 1800)),
		agent.AssistantMessage(strings.Repeat("old removable answer ", 1800), nil),
		agent.UserMessage("small retained turn"),
		agent.AssistantMessage("small retained answer", nil),
	}
	messages := append([]*agent.Message(nil), source...)
	for index := 0; index < 4; index++ {
		messages = append(messages, agent.UserMessage(fmt.Sprintf("pending interrupted input %d", index)))
		messages = append(messages, largePendingToolBatchForCompactionTest(index)...)
	}
	preview, _ := compactMessagesForModelThroughSource(messages, "small completed-history checkpoint", "", 1, 1, len(source))
	window := agentcontext.EstimateTokens(preview, nil)
	cfg := &config.Config{OpenAIContextWindowTokens: window}

	compacted, result, err := Prepare(context.Background(), cfg, config.AgentKindInteractiveStory, coldTestInput(Input{
		Messages:          messages,
		SourceMessages:    source,
		SourceMessagesSet: true,
		Force:             true,
		KeepLatestUser:    true,
	}, summarize), 1)
	if err == nil || !strings.Contains(err.Error(), "above hard publish band") {
		t.Fatalf("pending-dominated compaction error = %v, result=%#v", err, result)
	}
	if result.Triggered || result.SkippedReason != "no_progress" || result.TokensAfter < int(float64(window)*0.85) {
		t.Fatalf("pending-dominated candidate was not rejected: %#v", result)
	}
	if !reflect.DeepEqual(compacted, messages) {
		t.Fatalf("rejected compaction replaced the live pending tail")
	}
}

func largePendingToolBatchForCompactionTest(index int) []*agent.Message {
	callID := fmt.Sprintf("pending-call-%d", index)
	assistant := agent.AssistantMessage("inspect pending evidence", []agent.ToolCall{{
		ID: callID, Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: fmt.Sprintf(`{"path":"pending/%d.log"}`, index)},
	}})
	result := agent.ToolMessage(agent.TextToolResult(strings.Repeat(fmt.Sprintf("pending-result-%d ", index), 1600)), callID)
	return []*agent.Message{assistant, result}
}

func TestBuildContextCompactionUsesBackendOwnedTargetRange(t *testing.T) {
	policy := ResolvePolicy(&config.Config{}, config.AgentKindIDE)
	if policy.TargetMinRatio != config.DefaultContextCompactionTargetMinRatio ||
		policy.TargetMaxRatio != config.DefaultContextCompactionTargetMaxRatio {
		t.Fatalf(
			"target range = %.2f-%.2f, want %.2f-%.2f",
			policy.TargetMinRatio,
			policy.TargetMaxRatio,
			config.DefaultContextCompactionTargetMinRatio,
			config.DefaultContextCompactionTargetMaxRatio,
		)
	}
}

func TestBuildContextCompactionTriggersOnProjectedEightyPercentUsage(t *testing.T) {
	cfg := &config.Config{OpenAIContextWindowTokens: 10_000}
	messages := []*agent.Message{
		agent.UserMessage(strings.Repeat("上一轮用户行动", 30)),
		agent.AssistantMessage(strings.Repeat("上一轮剧情结果", 30), nil),
		agent.UserMessage("当前用户行动"),
	}
	model := &compactionForkCaptureModel{response: agent.AssistantMessage("unused", nil)}
	call := &agent.ModelCall{Model: model, Messages: messages, Options: []agent.ModelOption{agent.WithTools(nil)}}
	_, result, err := Prepare(context.Background(), cfg, config.AgentKindInteractiveStory, Input{
		Messages:                 messages,
		ReservedCompletionTokens: 8_500,
		KeepLatestUser:           true,
		PrimaryRequestSnapshot:   call.Snapshot(),
		Summarize: func(context.Context, *config.Config, SummaryRequest, func(int, string)) (string, error) {
			return "压缩后的事实摘要。", nil
		},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.TokensBefore >= 8_000 {
		t.Fatalf("raw prompt should be below threshold for this test: %#v", result)
	}
	if !result.Triggered || result.ProjectedTokensBefore < 8_000 {
		t.Fatalf("projected prompt + completion reserve should trigger at 80%%: %#v", result)
	}
}

func TestContextTokenProjectionCalibratesFromExactPreviousProviderUsage(t *testing.T) {
	prior := []*agent.Message{agent.UserMessage(strings.Repeat("calibration ", 80))}
	priorEstimate := agentcontext.EstimateTokens(prior, nil)
	response := agent.AssistantMessage("tool requested", nil)
	response.ResponseMeta = &agent.ResponseMeta{Usage: &agent.TokenUsage{PromptTokens: priorEstimate * 2}}
	messages := append(append([]*agent.Message(nil), prior...), response, agent.ToolMessage(agent.TextToolResult(strings.Repeat("delta ", 40)), "call-1"))

	observed, estimated := LatestPromptUsageCalibration(messages, nil)
	if observed != priorEstimate*2 || estimated != priorEstimate {
		t.Fatalf("usage calibration pair = observed:%d estimated:%d want %d/%d", observed, estimated, priorEstimate*2, priorEstimate)
	}
	local := agentcontext.EstimateTokens(messages, nil)
	calibrated := calibratedContextTokens(local, Input{
		ObservedPromptTokens: observed, ObservedEstimateTokens: estimated,
	})
	if calibrated != local*2 {
		t.Fatalf("calibrated current projection = %d, want %d from local=%d", calibrated, local*2, local)
	}
}

func TestContextTokenProjectionNeverCalibratesBelowLocalEstimate(t *testing.T) {
	const local = 800
	calibrated := calibratedContextTokens(local, Input{
		ObservedPromptTokens:   500,
		ObservedEstimateTokens: 1000,
	})
	if calibrated != local {
		t.Fatalf("downward provider calibration = %d, want local estimate %d", calibrated, local)
	}
}

func TestRecalculateProjectionKeepsCalibrationAndReserves(t *testing.T) {
	result := RecalculateProjection(Result{
		ObservedPromptTokens:     840,
		ObservedEstimateTokens:   450,
		ReservedCompletionTokens: 20,
		ReservedToolResultTokens: 30,
		ContextWindowTokens:      1000,
		Threshold:                0.85,
		RecoveryBand:             0.80,
	}, 450)
	if result.TokensAfter != 840 || result.ProjectedTokensAfter != 890 {
		t.Fatalf("recalculated projection = %#v", result)
	}
	if result.RecoveryBandMet || !result.Degraded || result.RecoveryTargetTokens != 680 {
		t.Fatalf("84%% calibrated projection was misclassified: %#v", result)
	}
}

func TestBuildContextCompactionEmitsStreamingSummaryDelta(t *testing.T) {
	summarize := func(_ context.Context, _ *config.Config, _ SummaryRequest, emitDelta func(int, string)) (string, error) {
		emitDelta(1, "第一段")
		emitDelta(1, "第二段")
		return "第一段第二段", nil
	}

	var events []Event
	_, result, err := Prepare(context.Background(), &config.Config{}, config.AgentKindIDE, coldTestInput(Input{
		Messages: []*agent.Message{
			agent.UserMessage(strings.Repeat("用户提出了一个很长的需求", 80)),
			agent.AssistantMessage(strings.Repeat("助手完成了很多上下文相关工作", 80), nil),
			agent.UserMessage("当前请求"),
		},
		Force:          true,
		KeepLatestUser: true,
		Emit:           func(event Event) { events = append(events, event) },
	}, summarize), 3)
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

func coldTestInput(input Input, summarize SummaryFunc) Input {
	input.ColdFallbackReason = "test_fixture"
	input.Summarize = summarize
	return input
}

func messageText(messages []*agent.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		if message != nil {
			parts = append(parts, message.Content)
		}
	}
	return strings.Join(parts, "\n")
}

func TestContextCompactionPolicyUsesBackendOwnedMechanics(t *testing.T) {
	policy := ResolvePolicy(&config.Config{}, config.AgentKindIDE)
	if policy.RetainedTurns != config.DefaultContextCompactionRetainedTurns {
		t.Fatalf("retained turns = %d, want default %d", policy.RetainedTurns, config.DefaultContextCompactionRetainedTurns)
	}
	if policy.Strategy != config.AgentContextCompactionStrategyCheckpointFork {
		t.Fatalf("strategy = %q, want checkpoint_fork", policy.Strategy)
	}
}

func TestContextCompactionRecoveryBandDistinguishesHealthyAndDegradedResults(t *testing.T) {
	healthy := Result{
		TokensBefore: 900, TokensAfter: 680, ContextWindowTokens: 1000,
		Threshold: 0.85, RecoveryBand: 0.80,
	}
	applyContextCompactionRecovery(&healthy)
	if healthy.RecoveryTargetTokens != 680 || !healthy.RecoveryBandMet || healthy.Degraded {
		t.Fatalf("healthy recovery = %#v", healthy)
	}

	degraded := healthy
	degraded.TokensAfter = 700
	applyContextCompactionRecovery(&degraded)
	if degraded.RecoveryBandMet || !degraded.Degraded {
		t.Fatalf("degraded recovery = %#v", degraded)
	}
	if err := Validate(degraded); err != nil {
		t.Fatalf("degraded but publishable result was rejected: %v", err)
	}

	unsafe := degraded
	unsafe.TokensAfter = 850
	applyContextCompactionRecovery(&unsafe)
	if err := Validate(unsafe); err == nil {
		t.Fatal("post-context at the 85% hard band must be rejected")
	}
}

func TestDegradedCompactionRetryLatchRequiresMaterialContextChange(t *testing.T) {
	if !NoProgressLatched(700, 1000, 0.85, 0.80, 20, 100, "same", 2, "same", 2) {
		t.Fatal("degraded checkpoint with an unchanged small tail should be latched")
	}
	if NoProgressLatched(700, 1000, 0.85, 0.80, 100, 100, "same", 2, "same", 2) {
		t.Fatal("enough new canonical context should release the latch")
	}
	if NoProgressLatched(700, 1000, 0.85, 0.80, 20, 100, "old", 2, "new", 2) {
		t.Fatal("a materially changed cleanup candidate set should release the latch")
	}
	if NoProgressLatched(700, 1000, 0.85, 0.80, 20, 100, "same", 2, "same", 3) {
		t.Fatal("a new cleanup candidate generation should release the latch")
	}
	if NoProgressLatched(650, 1000, 0.85, 0.80, 20, 100, "same", 2, "same", 2) {
		t.Fatal("a healthy checkpoint must not install a degraded latch")
	}
}

func TestBuildContextCompactionTranscriptKeepsAllIncrementalMessagesAndReferenceContext(t *testing.T) {
	messages := make([]*agent.Message, 0, 40)
	for i := 1; i <= 40; i++ {
		messages = append(messages, agent.UserMessage(strings.Repeat("旧消息", 2000)+":"+string(rune('A'+i%26))))
	}
	policy := Policy{AgentKind: config.AgentKindIDE, TargetMinRatio: 0.10, TargetMaxRatio: 0.25}
	existing := "既有压缩摘要：主角进入旧城。"
	reference := "有界参考上下文：关系=信任；任务=寻找钥匙。"
	inputChars := contextCompactionInputChars(existing, messages, reference)
	transcript := buildContextCompactionTranscript(messages, existing, reference, 1234, inputChars, policy)

	if strings.Contains(transcript, "omitted") || strings.Contains(transcript, "已截断") {
		t.Fatalf("compaction transcript should not report omitted content:\n%s", transcript[:200])
	}
	if !strings.Contains(transcript, existing) || !strings.Contains(transcript, reference) {
		t.Fatalf("transcript should include existing checkpoint and reference context:\n%s", transcript)
	}
	if !strings.Contains(transcript, "Source agent kind: ide") {
		t.Fatalf("transcript missing source mode: %s", transcript[:300])
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
