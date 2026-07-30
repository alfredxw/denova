package agents

import (
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/session"
)

type compactionForkCaptureModel struct {
	response *agent.Message
	inputs   [][]*agent.Message
	options  []*agent.Options
	streams  int
	requests int
}

func (model *compactionForkCaptureModel) Generate(_ context.Context, input []*agent.Message, opts ...agent.ModelOption) (*agent.Message, error) {
	model.capture(input, opts)
	if model.response == nil {
		return nil, io.EOF
	}
	return model.response.Clone(), nil
}

func (model *compactionForkCaptureModel) Stream(_ context.Context, input []*agent.Message, opts ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	model.capture(input, opts)
	model.streams++
	if model.response == nil {
		return agent.StreamReaderFromArray([]*agent.Message{}), nil
	}
	return agent.StreamReaderFromArray([]*agent.Message{model.response.Clone()}), nil
}

func (model *compactionForkCaptureModel) capture(input []*agent.Message, opts []agent.ModelOption) {
	model.requests++
	messages := make([]*agent.Message, len(input))
	for index, message := range input {
		if message != nil {
			messages[index] = message.Clone()
		}
	}
	model.inputs = append(model.inputs, messages)
	model.options = append(model.options, agent.GetCommonOptions(nil, opts...))
}

func TestContextCompactionForkAppendsOnlyStableTailToExactPrimaryRequest(t *testing.T) {
	response := agent.AssistantMessage("## Goal\nPreserve the requested outcome.", nil)
	response.ResponseMeta = &agent.ResponseMeta{Usage: &agent.TokenUsage{
		PromptTokens: 1800, PromptTokenDetails: agent.PromptTokenDetails{CachedTokens: 1500},
	}}
	model := &compactionForkCaptureModel{response: response}
	primary := []*agent.Message{
		agent.SystemMessage("stable system"),
		agent.UserMessage("old request"),
		agent.AssistantMessage("old result", nil),
		agent.UserMessage("current request"),
	}
	tools := []*agent.ToolInfo{{Name: "read", Desc: "read"}, {Name: "search", Desc: "search"}}
	call := &agent.ModelCall{
		Model: model, Messages: primary,
		Options: []agent.ModelOption{
			agent.WithTools(tools),
			agent.WithMaxTokens(2048),
			agent.WithToolChoice(agent.ToolChoiceAllowed, "read", "search"),
		},
	}
	ctx := contextWithCompactionRequestSnapshot(context.Background(), call.Snapshot())
	policy := contextCompactionPolicy{
		AgentKind: config.AgentKindIDE, ContextWindowTokens: 100_000,
		RetainedTurns: 1, TargetMinRatio: 0.05, TargetMaxRatio: 0.20,
	}
	source := primary[1:3]
	summary, _, execution, attempted, err := summarizeContextWithPrimaryFork(
		ctx, &config.Config{}, config.AgentKindIDE, "", source, "", EstimateContextTokens(source, nil), policy, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !attempted || summary != response.Content || model.requests != 1 {
		t.Fatalf("fork result = attempted:%t summary:%q calls:%d", attempted, summary, model.requests)
	}
	if got := model.inputs[0][:len(primary)]; !reflect.DeepEqual(got, primary) {
		t.Fatalf("provider prefix changed:\ngot  %#v\nwant %#v", got, primary)
	}
	if len(model.inputs[0]) != len(primary)+1 {
		t.Fatalf("provider messages = %d, want exact prefix plus one tail", len(model.inputs[0]))
	}
	tail := model.inputs[0][len(primary)].Content
	for _, heading := range []string{"## Goal", "## Constraints", "## Current state", "## Next actions", "## Critical context that must not be lost"} {
		if !strings.Contains(tail, heading) {
			t.Fatalf("stable checkpoint tail is missing %q:\n%s", heading, tail)
		}
	}
	resolved := model.options[0]
	if len(resolved.Tools) != 2 || resolved.Tools[0].Name != "read" || resolved.Tools[1].Name != "search" {
		t.Fatalf("tool order/schema changed: %#v", resolved.Tools)
	}
	if resolved.MaxTokens == nil || *resolved.MaxTokens != 2048 || resolved.ToolChoice == nil || *resolved.ToolChoice != agent.ToolChoiceAllowed {
		t.Fatalf("cache-sensitive options changed: %#v", resolved)
	}
	if !reflect.DeepEqual(resolved.AllowedToolNames, []string{"read", "search"}) {
		t.Fatalf("allowed tools changed: %#v", resolved.AllowedToolNames)
	}
	if execution.Mode != contextCompactionExecutionCacheSafeFork || execution.CacheReadTokens != 1500 || execution.LayerCount != 1 ||
		execution.CacheIdentityStatus != contextCompactionCacheIdentityExact || execution.CacheUsageStatus != contextCompactionCacheUsageRead || execution.CacheMissReason != "" {
		t.Fatalf("execution metrics = %#v", execution)
	}
	if execution.ExpectedCachedPrefixTokens != EstimateContextTokens(primary, tools) || execution.CheckpointOutputReserve != 4000 {
		t.Fatalf("capacity/cache projection = %#v", execution)
	}
}

func TestContextCompactionForkReportsProviderCacheUsageUncertainty(t *testing.T) {
	tests := []struct {
		name       string
		response   *agent.Message
		usage      string
		missReason string
	}{
		{
			name: "zero_or_unreported",
			response: func() *agent.Message {
				message := agent.AssistantMessage("checkpoint", nil)
				message.ResponseMeta = &agent.ResponseMeta{Usage: &agent.TokenUsage{PromptTokens: 100}}
				return message
			}(),
			usage:      contextCompactionCacheUsageZero,
			missReason: contextCompactionCacheMissZero,
		},
		{
			name:       "usage_unavailable",
			response:   agent.AssistantMessage("checkpoint", nil),
			usage:      contextCompactionCacheUsageMissing,
			missReason: contextCompactionCacheMissMissing,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			primary := []*agent.Message{
				agent.SystemMessage("stable system"),
				agent.UserMessage("old request"),
				agent.AssistantMessage("old result", nil),
				agent.UserMessage("current request"),
			}
			model := &compactionForkCaptureModel{response: test.response}
			call := &agent.ModelCall{Model: model, Messages: primary, Options: []agent.ModelOption{agent.WithTools(nil)}}
			ctx := contextWithCompactionRequestSnapshot(context.Background(), call.Snapshot())
			_, _, execution, attempted, err := summarizeContextWithPrimaryFork(
				ctx,
				&config.Config{},
				config.AgentKindIDE,
				"",
				primary[1:3],
				"",
				EstimateContextTokens(primary[1:3], nil),
				contextCompactionPolicy{AgentKind: config.AgentKindIDE, ContextWindowTokens: 100_000, RetainedTurns: 1},
				nil,
			)
			if err != nil || !attempted {
				t.Fatalf("fork = attempted:%t err:%v", attempted, err)
			}
			if execution.CacheIdentityStatus != contextCompactionCacheIdentityExact || execution.CacheUsageStatus != test.usage || execution.CacheMissReason != test.missReason {
				t.Fatalf("cache attribution = %#v", execution)
			}
			if execution.CacheReadTokens != 0 || execution.CacheWriteTokensKnown {
				t.Fatalf("unsupported usage must remain unknown: %#v", execution)
			}
		})
	}
}

func TestManualCompactionRequiresPrimarySnapshotWithinSingleWindow(t *testing.T) {
	previous := summarizeContextForCompaction
	defer func() { summarizeContextForCompaction = previous }()
	called := false
	summarizeContextForCompaction = func(_ context.Context, _ *config.Config, _ string, _ string, _ []*agent.Message, _ string, _ int, _ contextCompactionPolicy, _ func(int, string)) (string, int, error) {
		called = true
		return "unexpected cold checkpoint", 1, nil
	}
	messages := []*agent.Message{
		agent.UserMessage("old request"), agent.AssistantMessage("old answer", nil), agent.UserMessage("current request"),
	}
	unchanged, result, err := PrepareContextCompaction(context.Background(), &config.Config{}, config.AgentKindIDE, ContextCompactionInput{
		Messages: messages, Force: true, KeepLatestUser: true,
	}, 1)
	if err == nil || !strings.Contains(err.Error(), "requires the final primary request snapshot") {
		t.Fatalf("manual snapshot error = %v result=%#v", err, result)
	}
	if called || result.Triggered || !reflect.DeepEqual(unchanged, messages) {
		t.Fatalf("snapshot-less manual compaction started a cold model: called=%t result=%#v", called, result)
	}
}

func TestOversizedManualCompactionUsesLayeredFallback(t *testing.T) {
	previous := summarizeContextForCompaction
	defer func() { summarizeContextForCompaction = previous }()
	calls := 0
	summarizeContextForCompaction = func(_ context.Context, _ *config.Config, _ string, _ string, _ []*agent.Message, _ string, _ int, _ contextCompactionPolicy, _ func(int, string)) (string, int, error) {
		calls++
		return "## Goal\nPreserve the oversized manual source.\n\n## Current state\nThe bounded checkpoint is ready.", 50_000, nil
	}
	messages := []*agent.Message{
		agent.UserMessage(strings.Repeat("oversized source request ", 2200)),
		agent.AssistantMessage(strings.Repeat("oversized source result ", 2200), nil),
		agent.UserMessage("current request"),
	}
	compacted, result, err := PrepareContextCompaction(context.Background(), &config.Config{}, config.AgentKindIDE, ContextCompactionInput{
		Messages: messages, Force: true, KeepLatestUser: true, ContextWindowTokens: 10_000,
		ReservedCompletionTokens: 512, ReservedToolResultTokens: 512,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if calls == 0 || !result.Triggered || result.ExecutionMode != contextCompactionExecutionLayeredCold ||
		result.FallbackReason != contextCompactionFallbackManualSourceWindow {
		t.Fatalf("oversized fallback = calls:%d result:%#v", calls, result)
	}
	if len(compacted) != 2 || !IsContextCompactionSummaryMessage(compacted[0]) || compacted[1].Content != "current request" {
		t.Fatalf("oversized fallback projection = %#v", messageContents(compacted))
	}
}

func TestCompactionForkReserveHonorsLargePrimaryCompletionBudget(t *testing.T) {
	output, _ := compactionForkReserves(0, 160_000, contextCompactionPolicy{CheckpointOutputReserve: 24_000}, &agent.Options{})
	if output != 24_000 {
		t.Fatalf("fork output reserve = %d, want primary completion reserve 24000", output)
	}
	policy := ContextPressurePolicy{
		AgentKind: config.AgentKindInteractiveStory, ContextWindowTokens: 30_000,
		CheckpointOutputReserve: 24_000, CompactionPromptTokens: 2_000, SafetyMarginTokens: 1_000,
	}
	if !compactionForkCapacityPressure([]*agent.Message{agent.UserMessage(strings.Repeat("story context ", 1200))}, nil, policy, &agent.Options{}) {
		t.Fatal("large Game completion reserve did not advance compaction pressure")
	}
}

func TestCompactionForkAndColdFallbackShareCheckpointSchema(t *testing.T) {
	cold := contextCompactionSystemInstruction()
	fork := buildCacheSafeCompactionPrompt(
		contextCompactionPolicy{AgentKind: config.AgentKindIDE, RetainedTurns: 1},
		"", "", 100, 1000, []int{0, 1}, nil,
	)
	schema := contextCompactionCheckpointSchema()
	if !strings.Contains(cold, schema) || !strings.Contains(fork, schema) {
		t.Fatalf("cold and cache-safe paths do not share the stable schema:\ncold=%s\nfork=%s", cold, fork)
	}
	for _, heading := range contextCompactionCheckpointHeadings {
		if strings.Count(cold, heading) != 1 || strings.Count(fork, heading) != 1 {
			t.Fatalf("heading %q differs across compaction paths", heading)
		}
	}
	for _, obsolete := range []string{"【历史事件时间线】", "【任务目标与用户约束】"} {
		if strings.Contains(cold, obsolete) {
			t.Fatalf("cold fallback still requires obsolete schema %q", obsolete)
		}
	}
}

func TestSessionCompactionMapsDynamicFinalUserAndPostToolBatchToPrimaryFork(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("dynamic-compaction-fork")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendContextMessages(
		agent.UserMessage(strings.Repeat("large prior request ", 900)),
		agent.AssistantMessage(strings.Repeat("large prior answer ", 900), nil),
	); err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversationForAgentWithRuntimeContext(
		sess, &config.Config{}, config.AgentKindIDE,
		"dynamic workspace state", "chapter cursor is after the city gate",
	)
	assembled, err := assembleAndCommitModelContextForTest(conversation, "continue", "continue")
	if err != nil {
		t.Fatal(err)
	}
	callMessage := agent.AssistantMessage("", []agent.ToolCall{{
		ID: "call-post-assembly", Type: "function",
		Function: agent.FunctionCall{Name: "read", Arguments: `{"path":"chapter.md"}`},
	}})
	toolMessage := agent.ToolMessage(agent.TextToolResult("chapter evidence"), "call-post-assembly", agent.WithToolName("read"))
	if err := conversation.AppendContextMessages(callMessage, toolMessage); err != nil {
		t.Fatal(err)
	}
	primary := append(cloneContextMessages(assembled), callMessage.Clone(), toolMessage.Clone())
	source, _, sourceStart, _, err := conversation.compactionIncrementalSource(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !containsMessageContent(source, "continue") || containsMessageContent(source, "chapter cursor is after the city gate") {
		t.Fatalf("canonical source absorbed the dynamic wrapper: %#v", messageContents(source))
	}
	mapped, ok := conversation.providerVisibleCompactionSource(source, sourceStart)
	if !ok {
		t.Fatal("exact assembler canonical/effective pair was not mapped")
	}
	if positions, _, visible := locateCompactionSourceInPrimary(primary, mapped); !visible || len(positions) != len(mapped) {
		t.Fatalf("mapped post-tool source was not contiguous in final primary snapshot: positions=%v visible=%t", positions, visible)
	}

	model := &compactionForkCaptureModel{response: agent.AssistantMessage("## Goal\nContinue the chapter.\n\n## Current state\nThe city gate was reached.", nil)}
	request := &agent.ModelCall{Model: model, Messages: primary}
	ctx := contextWithCompactionRequestSnapshot(context.Background(), request.Snapshot())
	compacted, result, err := conversation.CompactContextIfNeeded(ctx, ContextCompactionInput{
		Messages: primary, Force: true, KeepLatestUser: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered || result.ExecutionMode != contextCompactionExecutionCacheSafeFork || model.requests != 1 {
		t.Fatalf("compaction did not use one cache-safe fork: result=%#v requests=%d", result, model.requests)
	}
	if got := model.inputs[0][:len(primary)]; !reflect.DeepEqual(got, primary) {
		t.Fatalf("cache-safe fork changed primary prefix:\ngot=%#v\nwant=%#v", got, primary)
	}
	if len(compacted) == 0 || !IsContextCompactionSummaryMessage(compacted[0]) {
		t.Fatalf("missing transient checkpoint: %#v", compacted)
	}
}

func TestSessionManualCompactionMapsNormalizedLegacyProtocolToPrimaryFork(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("normalized-legacy-compaction-fork")
	if err != nil {
		t.Fatal(err)
	}
	legacyUser := agent.UserMessage(strings.Repeat("old request ", 600))
	legacyUser.ToolCallID = "legacy-malformed-user-result"
	legacyUser.ToolName = "read"
	if err := sess.AppendContextMessages(
		legacyUser,
		agent.AssistantMessage("old answer", nil),
		agent.UserMessage("current request"),
	); err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversationForAgent(sess, &config.Config{}, config.AgentKindIDE)
	snapshot, err := sess.SnapshotContext(config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	canonical := conversation.modelHistory(snapshot)
	normalized, err := NormalizeModelContextMessages(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized) != len(canonical) || canonical[0].ToolCallID == "" || normalized[0].ToolCallID != "" || normalized[0].ToolName != "" {
		t.Fatalf("legacy fixture was not repaired as expected: %#v", normalized)
	}
	primary := append([]*agent.Message{agent.SystemMessage("stable system")}, normalized...)
	model := &compactionForkCaptureModel{response: agent.AssistantMessage("## Goal\nContinue the current request.", nil)}
	request := &agent.ModelCall{Model: model, Messages: primary, Options: []agent.ModelOption{agent.WithTools(nil)}}
	compacted, result, err := conversation.CompactContextIfNeeded(context.Background(), ContextCompactionInput{
		Messages: canonical, Force: true, KeepLatestUser: true, PrimaryRequestSnapshot: request.Snapshot(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered || result.ExecutionMode != contextCompactionExecutionCacheSafeFork || model.requests != 1 {
		t.Fatalf("normalized manual compaction = result:%#v requests:%d", result, model.requests)
	}
	if got := model.inputs[0][:len(primary)]; !reflect.DeepEqual(got, primary) {
		t.Fatalf("normalized provider prefix changed:\ngot=%#v\nwant=%#v", got, primary)
	}
	if _, err := NormalizeModelContextMessages(compacted); err != nil {
		t.Fatalf("compacted legacy context is invalid: %v\n%#v", err, compacted)
	}
	if result.TokensAfter != EstimateContextTokens(compacted, nil) {
		t.Fatalf("post-normalizer token accounting = %d, want %d", result.TokensAfter, EstimateContextTokens(compacted, nil))
	}
}

func TestStagedRewindDefersAutomaticCompactionAfterExactSourceMapping(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("staged-rewind-compaction")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendContextMessages(
		agent.UserMessage(strings.Repeat("large safe old request ", 900)),
		agent.AssistantMessage(strings.Repeat("large safe old answer ", 900), nil),
	); err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversationForAgentWithRuntimeContext(
		sess, &config.Config{}, config.AgentKindIDE,
		"dynamic workspace state", "cursor remains on the safe branch",
	)
	assembled, err := assembleAndCommitModelContextForTest(conversation, "inspect safely", "inspect safely")
	if err != nil {
		t.Fatal(err)
	}
	conversation.cycleMu.Lock()
	base := conversation.contextWindowBase
	if base != nil {
		base = &contextWindowModelBase{
			cursor: base.cursor, canonical: cloneContextMessages(base.canonical), effective: cloneContextMessages(base.effective),
		}
	}
	conversation.cycleMu.Unlock()
	if base == nil {
		t.Fatal("missing frozen assembler model base")
	}
	boundary, err := session.NewContextBoundarySnapshot(base.cursor, assembled, base.canonical, 4*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	discardedCall := agent.AssistantMessage("", []agent.ToolCall{{
		ID: "discarded-call", Type: "function", Function: agent.FunctionCall{Name: "read"},
	}})
	discardedTool := agent.ToolMessage(agent.TextToolResult("DISCARDED BRANCH TOOL RESULT"), "discarded-call", agent.WithToolName("read"))
	if err := conversation.AppendContextMessages(discardedCall, discardedTool); err != nil {
		t.Fatal(err)
	}
	rewind := session.ContextOperation{
		Kind: session.ContextOperationRewind, AgentKind: config.AgentKindIDE,
		CheckpointID: "cp-staged", MessageCount: boundary.Cursor.MessageCount,
		ResolvedBoundary: boundary, Report: "safe staged rewind finding",
	}
	if err := conversation.StageContextOperation(rewind); err != nil {
		t.Fatal(err)
	}
	postCall := agent.AssistantMessage("", []agent.ToolCall{{
		ID: "post-rewind-call", Type: "function", Function: agent.FunctionCall{Name: "read"},
	}})
	postTool := agent.ToolMessage(agent.TextToolResult("safe post-rewind evidence"), "post-rewind-call", agent.WithToolName("read"))
	if err := conversation.AppendContextMessages(postCall, postTool); err != nil {
		t.Fatal(err)
	}
	primary := append(cloneContextMessages(boundary.EffectivePrefix), newContextRewindSummaryMessage(rewind))
	primary = append(primary, postCall.Clone(), postTool.Clone())
	canonical, provider, _, _, _, found, err := conversation.stagedRewindCompactionSource(primary, true)
	if err != nil {
		t.Fatal(err)
	}
	if !found || containsMessageContent(canonical, "DISCARDED BRANCH") || !containsMessageContent(canonical, "safe post-rewind evidence") {
		t.Fatalf("staged rewind canonical source is wrong: found=%t source=%#v", found, messageContents(canonical))
	}
	if positions, _, visible := locateCompactionSourceInPrimary(primary, provider); !visible || len(positions) != len(provider) {
		t.Fatalf("staged rewind provider source was not exact: positions=%v visible=%t", positions, visible)
	}

	model := &compactionForkCaptureModel{response: agent.AssistantMessage("## Current state\nThe safe rewind branch and post-tool evidence are preserved.", nil)}
	request := &agent.ModelCall{Model: model, Messages: primary}
	ctx := contextWithCompactionRequestSnapshot(context.Background(), request.Snapshot())
	unchanged, result, err := conversation.CompactContextIfNeeded(ctx, ContextCompactionInput{
		Messages: primary, Planned: true, Automatic: true, KeepLatestUser: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Triggered || result.SkippedReason != "staged_rewind_pending" || model.requests != 0 {
		t.Fatalf("staged rewind started a second structural change: result=%#v requests=%d", result, model.requests)
	}
	if !reflect.DeepEqual(unchanged, primary) || containsMessageContent(unchanged, "DISCARDED BRANCH") ||
		!containsMessageContent(unchanged, "safe post-rewind evidence") {
		t.Fatalf("staged rewind request changed while maintenance was deferred: %#v", messageContents(unchanged))
	}
}

func containsMessageContent(messages []*agent.Message, content string) bool {
	for _, message := range messages {
		if message != nil && strings.Contains(message.Content, content) {
			return true
		}
	}
	return false
}

func TestLocateCompactionSourceUsesNewestContiguousRepeatedToolTurn(t *testing.T) {
	user := agent.UserMessage("repeat this exact request")
	assistant := agent.AssistantMessage("", []agent.ToolCall{{
		ID: "provider-local", Type: "function",
		Function: agent.FunctionCall{Name: "write", Arguments: `{"path":"chapter.md"}`},
	}})
	result := agent.ToolMessage(agent.ToolResult{
		ModelContent: "saved", Status: agent.ToolResultSuccess,
		ResultRetention: agent.ToolResultProtected,
		ProtectedReceipt: &agent.ToolResultProtectedReceipt{
			SanitizedArguments: `{"path":"chapter.md"}`,
			Outcome:            `{"schema":"tool_result.receipt.v2","status":"success"}`,
		},
	}, "provider-local", agent.WithToolName("write"))
	primary := []*agent.Message{
		agent.SystemMessage("stable system"),
		NewContextCompactionSummaryMessage(1, "prior checkpoint"),
		user.Clone(), assistant.Clone(), result.Clone(), agent.AssistantMessage("old completion", nil),
		user.Clone(), assistant.Clone(), result.Clone(),
	}

	positions, _, ok := locateCompactionSourceInPrimary(primary, []*agent.Message{user, assistant, result})
	if !ok || !reflect.DeepEqual(positions, []int{6, 7, 8}) {
		t.Fatalf("repeated source mapped to %#v (ok=%t), want newest interval [6 7 8]", positions, ok)
	}
}

func TestSameProviderVisibleMessageIncludesMultimodalAndToolReceipt(t *testing.T) {
	left := agent.UserMessage("same text")
	left.MultiContent = []json.RawMessage{json.RawMessage(`{"type":"text","text":"left"}`)}
	right := left.Clone()
	right.MultiContent[0] = json.RawMessage(`{"type":"text","text":"right"}`)
	if sameProviderVisibleMessage(left, right) {
		t.Fatal("multimodal payload difference was ignored")
	}

	left = agent.ToolMessage(agent.ToolResult{
		ModelContent: "same", Status: agent.ToolResultSuccess,
		ProtectedReceipt: &agent.ToolResultProtectedReceipt{Outcome: `{"status":"success"}`},
	}, "call", agent.WithToolName("write"))
	right = left.Clone()
	right.ToolResult.ProtectedReceipt.Outcome = `{"status":"error"}`
	if sameProviderVisibleMessage(left, right) {
		t.Fatal("tool-result continuity receipt difference was ignored")
	}
}

func TestContextCompactionForkDeniesToolCallsWithoutExecutingAnotherAgentLoop(t *testing.T) {
	model := &compactionForkCaptureModel{response: agent.AssistantMessage("", []agent.ToolCall{{
		ID: "call-1", Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: `{}`},
	}})}
	primary := []*agent.Message{agent.SystemMessage("system"), agent.UserMessage("old source"), agent.UserMessage("current")}
	call := &agent.ModelCall{Model: model, Messages: primary, Options: []agent.ModelOption{agent.WithTools([]*agent.ToolInfo{{Name: "read"}})}}
	ctx := contextWithCompactionRequestSnapshot(context.Background(), call.Snapshot())
	_, _, _, attempted, err := summarizeContextWithPrimaryFork(
		ctx, &config.Config{}, config.AgentKindIDE, "", primary[1:2], "", 10,
		contextCompactionPolicy{AgentKind: config.AgentKindIDE, ContextWindowTokens: 100_000, RetainedTurns: 1}, nil,
	)
	if !attempted || err == nil || !strings.Contains(err.Error(), "denied 1 requested tool call") {
		t.Fatalf("tool-call result = attempted:%t err:%v", attempted, err)
	}
	if model.requests != 1 {
		t.Fatalf("fork must execute one model turn, got %d calls", model.requests)
	}
}

func TestContextCompactionForkFallsBackColdOnlyWhenCapacityDoesNotFit(t *testing.T) {
	model := &compactionForkCaptureModel{response: agent.AssistantMessage("unused", nil)}
	primary := []*agent.Message{
		agent.SystemMessage(strings.Repeat("s", 2600)),
		agent.UserMessage(strings.Repeat("history ", 500)),
		agent.UserMessage("current"),
	}
	call := &agent.ModelCall{Model: model, Messages: primary, Options: []agent.ModelOption{agent.WithTools(nil)}}
	ctx := contextWithCompactionRequestSnapshot(context.Background(), call.Snapshot())
	policy := contextCompactionPolicy{AgentKind: config.AgentKindIDE, ContextWindowTokens: 1000, RetainedTurns: 1}
	source := primary[1:2]

	previous := summarizeContextForCompaction
	defer func() { summarizeContextForCompaction = previous }()
	coldCalls := 0
	summarizeContextForCompaction = func(_ context.Context, _ *config.Config, _ string, _ string, _ []*agent.Message, _ string, _ int, _ contextCompactionPolicy, _ func(int, string)) (string, int, error) {
		coldCalls++
		return "cold checkpoint", 20, nil
	}
	summary, _, execution, err := summarizeContextInLayers(
		ctx, &config.Config{}, config.AgentKindIDE, "", source, "", EstimateContextTokens(source, nil), policy, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if summary != "cold checkpoint" || coldCalls != 1 || model.requests != 0 {
		t.Fatalf("fallback calls = summary:%q cold:%d primary:%d", summary, coldCalls, model.requests)
	}
	if execution.Mode != contextCompactionExecutionLayeredCold || execution.FallbackReason != contextCompactionFallbackCapacity ||
		execution.CacheIdentityStatus != contextCompactionCacheIdentityCold || execution.CacheUsageStatus != contextCompactionCacheUsageCold || execution.CacheMissReason != contextCompactionFallbackCapacity {
		t.Fatalf("fallback execution = %#v", execution)
	}
}

func TestContextCompactionForkRejectsSourceMismatchInsteadOfStartingColdModel(t *testing.T) {
	model := &compactionForkCaptureModel{response: agent.AssistantMessage("unused", nil)}
	primary := []*agent.Message{agent.SystemMessage("system"), agent.UserMessage("current")}
	call := &agent.ModelCall{Model: model, Messages: primary, Options: []agent.ModelOption{agent.WithTools(nil)}}
	ctx := contextWithCompactionRequestSnapshot(context.Background(), call.Snapshot())

	previous := summarizeContextForCompaction
	defer func() { summarizeContextForCompaction = previous }()
	coldCalls := 0
	summarizeContextForCompaction = func(_ context.Context, _ *config.Config, _ string, _ string, _ []*agent.Message, _ string, _ int, _ contextCompactionPolicy, _ func(int, string)) (string, int, error) {
		coldCalls++
		return "unexpected", 1, nil
	}
	_, _, execution, err := summarizeContextInLayers(
		ctx, &config.Config{}, config.AgentKindIDE, "", []*agent.Message{agent.UserMessage("missing canonical source")}, "", 10,
		contextCompactionPolicy{AgentKind: config.AgentKindIDE, ContextWindowTokens: 100_000, RetainedTurns: 1}, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "does not match the final primary request") {
		t.Fatalf("source mismatch error = %v", err)
	}
	if coldCalls != 0 || model.requests != 0 {
		t.Fatalf("source mismatch started another model: cold=%d primary=%d", coldCalls, model.requests)
	}
	if execution.FallbackReason != contextCompactionFallbackSourceNotVisible {
		t.Fatalf("execution = %#v", execution)
	}
}
