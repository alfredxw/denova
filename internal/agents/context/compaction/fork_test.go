package compaction

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/prompts"
)

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
	policy := Policy{
		AgentKind: config.AgentKindIDE, ContextWindowTokens: 100_000,
		RetainedTurns: 1, TargetMinRatio: 0.05, TargetMaxRatio: 0.20,
	}
	source := primary[1:3]
	summary, _, execution, attempted, err := summarizeContextWithPrimaryFork(
		context.Background(), &config.Config{}, config.AgentKindIDE, "", source, source, "",
		agentcontext.EstimateTokens(source, nil), policy, call.Snapshot(), "", nil,
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
	if execution.Mode != ExecutionCacheSafeFork || execution.CacheReadTokens != 1500 || execution.LayerCount != 1 ||
		execution.CacheIdentityStatus != CacheIdentityExact || execution.CacheUsageStatus != CacheUsageRead || execution.CacheMissReason != "" {
		t.Fatalf("execution metrics = %#v", execution)
	}
	if execution.ExpectedCachedPrefixTokens != agentcontext.EstimateTokens(primary, tools) || execution.CheckpointOutputReserve != 4000 {
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
			usage:      CacheUsageZero,
			missReason: CacheMissZero,
		},
		{
			name:       "usage_unavailable",
			response:   agent.AssistantMessage("checkpoint", nil),
			usage:      CacheUsageMissing,
			missReason: CacheMissMissing,
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
			_, _, execution, attempted, err := summarizeContextWithPrimaryFork(
				context.Background(),
				&config.Config{},
				config.AgentKindIDE,
				"",
				primary[1:3],
				primary[1:3],
				"",
				agentcontext.EstimateTokens(primary[1:3], nil),
				Policy{AgentKind: config.AgentKindIDE, ContextWindowTokens: 100_000, RetainedTurns: 1},
				call.Snapshot(),
				"",
				nil,
			)
			if err != nil || !attempted {
				t.Fatalf("fork = attempted:%t err:%v", attempted, err)
			}
			if execution.CacheIdentityStatus != CacheIdentityExact || execution.CacheUsageStatus != test.usage || execution.CacheMissReason != test.missReason {
				t.Fatalf("cache attribution = %#v", execution)
			}
			if execution.CacheReadTokens != 0 || execution.CacheWriteTokensKnown {
				t.Fatalf("unsupported usage must remain unknown: %#v", execution)
			}
		})
	}
}

func TestManualCompactionRequiresPrimarySnapshotWithinSingleWindow(t *testing.T) {
	called := false
	summarize := func(_ context.Context, _ *config.Config, _ SummaryRequest, _ func(int, string)) (string, error) {
		called = true
		return "unexpected cold checkpoint", nil
	}
	messages := []*agent.Message{
		agent.UserMessage("old request"), agent.AssistantMessage("old answer", nil), agent.UserMessage("current request"),
	}
	unchanged, result, err := Prepare(context.Background(), &config.Config{}, config.AgentKindIDE, Input{
		Messages: messages, Force: true, KeepLatestUser: true, Summarize: summarize,
	}, 1)
	if err == nil || !strings.Contains(err.Error(), "requires the final primary request snapshot") {
		t.Fatalf("manual snapshot error = %v result=%#v", err, result)
	}
	if called || result.Triggered || !reflect.DeepEqual(unchanged, messages) {
		t.Fatalf("snapshot-less manual compaction started a cold model: called=%t result=%#v", called, result)
	}
}

func TestOversizedManualCompactionUsesLayeredFallback(t *testing.T) {
	calls := 0
	summarize := func(_ context.Context, _ *config.Config, _ SummaryRequest, _ func(int, string)) (string, error) {
		calls++
		return "## Goal\nPreserve the oversized manual source.\n\n## Current state\nThe bounded checkpoint is ready.", nil
	}
	messages := []*agent.Message{
		agent.UserMessage(strings.Repeat("oversized source request ", 2200)),
		agent.AssistantMessage(strings.Repeat("oversized source result ", 2200), nil),
		agent.UserMessage("current request"),
	}
	compacted, result, err := Prepare(context.Background(), &config.Config{}, config.AgentKindIDE, Input{
		Messages: messages, Force: true, KeepLatestUser: true, ContextWindowTokens: 10_000,
		ReservedCompletionTokens: 512, ReservedToolResultTokens: 512, Summarize: summarize,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if calls == 0 || !result.Triggered || result.ExecutionMode != ExecutionLayeredCold ||
		result.FallbackReason != fallbackManualSourceWindow {
		t.Fatalf("oversized fallback = calls:%d result:%#v", calls, result)
	}
	if len(compacted) != 2 || !agentcontext.IsCompactionSummaryMessage(compacted[0]) || compacted[1].Content != "current request" {
		t.Fatalf("oversized fallback projection = %#v", messageContents(compacted))
	}
}

func TestCompactionForkReserveHonorsLargePrimaryCompletionBudget(t *testing.T) {
	output, _ := compactionForkReserves(0, 160_000, Policy{CheckpointOutputReserve: 24_000}, &agent.Options{})
	if output != 24_000 {
		t.Fatalf("fork output reserve = %d, want primary completion reserve 24000", output)
	}
	policy := ForkCapacityPolicy{
		AgentKind: config.AgentKindInteractiveStory, ContextWindowTokens: 30_000,
		CheckpointOutputReserve: 24_000, CompactionPromptTokens: 2_000, SafetyMarginTokens: 1_000,
	}
	if !ForkCapacityPressure([]*agent.Message{agent.UserMessage(strings.Repeat("story context ", 1200))}, nil, policy, &agent.Options{}) {
		t.Fatal("large Game completion reserve did not advance compaction pressure")
	}
}

func TestCompactionForkAndColdFallbackShareCheckpointSchema(t *testing.T) {
	cold := prompts.ContextCompactionSystemInstruction()
	fork := buildCacheSafeCompactionPrompt(
		Policy{AgentKind: config.AgentKindIDE, RetainedTurns: 1},
		"", "", 100, 1000, []int{0, 1}, nil,
	)
	schema := agentcontext.CompactionCheckpointSchema()
	if !strings.Contains(cold, schema) || !strings.Contains(fork, schema) {
		t.Fatalf("cold and cache-safe paths do not share the stable schema:\ncold=%s\nfork=%s", cold, fork)
	}
	for _, heading := range strings.Split(strings.TrimSpace(agentcontext.CompactionCheckpointSchema()), "\n") {
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

func TestCheckpointGuidanceIsAppendedToPrimaryAndColdCompactionRequests(t *testing.T) {
	guidance := "Preserve rejected approaches and exact verification evidence."
	policy := Policy{
		AgentKind: config.AgentKindIDE, RetainedTurns: 1,
		CheckpointGuidance: guidance, TargetMinRatio: 0.05, TargetMaxRatio: 0.20,
	}
	primary := buildCacheSafeCompactionPrompt(policy, "", "", 100, 1000, []int{0, 1}, nil)
	cold := buildContextCompactionTranscript([]*agent.Message{agent.UserMessage("source")}, "", "", 100, 1000, policy)
	for name, prompt := range map[string]string{"primary": primary, "cold": cold} {
		if strings.Count(prompt, guidance) != 1 || strings.Count(prompt, "<checkpoint_guidance>") != 1 {
			t.Fatalf("%s compaction prompt did not include guidance exactly once:\n%s", name, prompt)
		}
		if !strings.Contains(prompt, compactionGuidancePrecedence) {
			t.Fatalf("%s compaction prompt omitted guidance precedence", name)
		}
	}

	sources := BuiltinPromptSources()
	if len(sources.IDE.Sources) != 3 || len(sources.InteractiveStory.Sources) != 3 {
		t.Fatalf("read-only compaction prompt sources are incomplete: IDE=%#v Game=%#v", sources.IDE, sources.InteractiveStory)
	}
	if !strings.Contains(sources.IDE.Sources[2].Content, "Workspace/writing requirements") ||
		!strings.Contains(sources.InteractiveStory.Sources[2].Content, "Game-mode requirements") {
		t.Fatalf("Agent-specific compaction sources are incorrect: IDE=%q Game=%q", sources.IDE.Sources[2].Content, sources.InteractiveStory.Sources[2].Content)
	}
}

func TestAutomaticCompactionForkPromptFitsDeclaredReserve(t *testing.T) {
	locators := make([]string, 200)
	positions := make([]int, len(locators))
	for index := range locators {
		positions[index] = index
		locators[index] = "[source path=" + strings.Repeat("very-long-segment/", 200) + "]"
	}
	for _, agentKind := range []string{config.AgentKindIDE, config.AgentKindInteractiveStory} {
		prompt := buildCacheSafeCompactionPrompt(
			Policy{
				AgentKind: agentKind, RetainedTurns: 1,
				CheckpointGuidance: strings.Repeat("界", config.MaxCheckpointGuidanceRunes),
			},
			"", "", 100_000, 400_000, positions, locators,
		)
		if tokens := agentcontext.EstimateStringTokens(prompt); tokens > ForkPromptReserve {
			t.Fatalf("%s automatic Compaction fork prompt = %d tokens, exceeds declared reserve %d", agentKind, tokens, ForkPromptReserve)
		}
		if !strings.Contains(prompt, agentcontext.CompactionCheckpointSchema()) {
			t.Fatalf("%s bounded fork prompt lost the checkpoint schema", agentKind)
		}
	}
}

func TestContextCompactionForkRejectsPromptBeyondDeclaredReserve(t *testing.T) {
	model := &compactionForkCaptureModel{response: agent.AssistantMessage("unused", nil)}
	primary := []*agent.Message{agent.SystemMessage("system"), agent.UserMessage("source"), agent.UserMessage("current")}
	call := &agent.ModelCall{Model: model, Messages: primary}
	_, _, execution, attempted, err := summarizeContextWithPrimaryFork(
		context.Background(), &config.Config{}, config.AgentKindIDE, "", primary[1:2], primary[1:2], "", 10,
		Policy{
			AgentKind: config.AgentKindIDE, ContextWindowTokens: 100_000, RetainedTurns: 1,
			CheckpointGuidance: strings.Repeat("界", ForkPromptReserve),
		},
		call.Snapshot(), "", nil,
	)
	if !attempted || err == nil || !strings.Contains(err.Error(), "fork prompt requires") {
		t.Fatalf("oversized fork prompt = attempted:%t execution:%#v err:%v", attempted, execution, err)
	}
	if model.requests != 0 {
		t.Fatalf("oversized fork prompt reached the model %d time(s)", model.requests)
	}
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
		agentcontext.NewCompactionSummaryMessage(1, "prior checkpoint"),
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
	_, _, _, attempted, err := summarizeContextWithPrimaryFork(
		context.Background(), &config.Config{}, config.AgentKindIDE, "", primary[1:2], primary[1:2], "", 10,
		Policy{AgentKind: config.AgentKindIDE, ContextWindowTokens: 100_000, RetainedTurns: 1}, call.Snapshot(), "", nil,
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
	call := &agent.ModelCall{Model: model, Messages: primary, Options: []agent.ModelOption{
		agent.WithTools(nil), agent.WithMaxTokens(8_000),
	}}
	policy := Policy{AgentKind: config.AgentKindIDE, ContextWindowTokens: 10_000, RetainedTurns: 1}
	source := primary[1:2]

	coldCalls := 0
	summarize := func(_ context.Context, _ *config.Config, _ SummaryRequest, _ func(int, string)) (string, error) {
		coldCalls++
		return "cold checkpoint", nil
	}
	summary, _, execution, err := summarizeContextInLayers(
		context.Background(), &config.Config{}, config.AgentKindIDE, "", source, source, "",
		agentcontext.EstimateTokens(source, nil), policy, call.Snapshot(), "", summarize, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if summary != "cold checkpoint" || coldCalls != 1 || model.requests != 0 {
		t.Fatalf("fallback calls = summary:%q cold:%d primary:%d", summary, coldCalls, model.requests)
	}
	if execution.Mode != ExecutionLayeredCold || execution.FallbackReason != FallbackCapacity ||
		execution.CacheIdentityStatus != CacheIdentityCold || execution.CacheUsageStatus != CacheUsageCold || execution.CacheMissReason != FallbackCapacity {
		t.Fatalf("fallback execution = %#v", execution)
	}
}

func TestContextCompactionForkRejectsSourceMismatchInsteadOfStartingColdModel(t *testing.T) {
	model := &compactionForkCaptureModel{response: agent.AssistantMessage("unused", nil)}
	primary := []*agent.Message{agent.SystemMessage("system"), agent.UserMessage("current")}
	call := &agent.ModelCall{Model: model, Messages: primary, Options: []agent.ModelOption{agent.WithTools(nil)}}

	coldCalls := 0
	summarize := func(_ context.Context, _ *config.Config, _ SummaryRequest, _ func(int, string)) (string, error) {
		coldCalls++
		return "unexpected", nil
	}
	_, _, execution, err := summarizeContextInLayers(
		context.Background(), &config.Config{}, config.AgentKindIDE, "",
		[]*agent.Message{agent.UserMessage("missing canonical source")}, nil, "", 10,
		Policy{AgentKind: config.AgentKindIDE, ContextWindowTokens: 100_000, RetainedTurns: 1},
		call.Snapshot(), "", summarize, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "does not match the final primary request") {
		t.Fatalf("source mismatch error = %v", err)
	}
	if coldCalls != 0 || model.requests != 0 {
		t.Fatalf("source mismatch started another model: cold=%d primary=%d", coldCalls, model.requests)
	}
	if execution.FallbackReason != FallbackSourceNotVisible {
		t.Fatalf("execution = %#v", execution)
	}
}
