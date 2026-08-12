package cleanup

import (
	"context"
	"fmt"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

type requestCaptureModel struct{}

func (requestCaptureModel) Generate(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.Message, error) {
	return agent.AssistantMessage("unused", nil), nil
}

func (requestCaptureModel) Stream(context.Context, []*agent.Message, ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	return agent.StreamReaderFromArray([]*agent.Message{agent.AssistantMessage("unused", nil)}), nil
}

func recoverableGroup(id, payload string, retention agent.ToolResultRetentionMode, recovery agent.ToolResultRecoveryHint) []*agent.Message {
	return []*agent.Message{
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: id, Type: "function", Function: agent.FunctionCall{Name: "read", Arguments: fmt.Sprintf(`{"path":%q}`, id+".md")},
		}}),
		{
			Role: agent.ToolRole, Content: payload, ToolCallID: id, ToolName: "read",
			ToolResult: &agent.ToolResultSummary{
				Status: agent.ToolResultSuccess, ResultRetention: retention,
				ContextHints: &agent.ToolResultContextHints{Recovery: recovery},
			},
		},
		agent.AssistantMessage("noted", nil),
		agent.UserMessage("continue"),
	}
}

func standardForTest(t testing.TB, config StandardConfig) agent.CleanupManager {
	t.Helper()
	manager, err := Standard(config)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

type cleanupLifecycleResult struct {
	reason    string
	metrics   agent.CleanupMetrics
	committed bool
}

type cleanupPlanCapture struct {
	agent.CleanupManager
	plan agent.CleanupPlan
}

func (capture *cleanupPlanCapture) Plan(ctx context.Context, request agent.CleanupPlanRequest) (agent.CleanupPlan, error) {
	plan, err := capture.CleanupManager.Plan(ctx, request)
	capture.plan = plan
	return plan, err
}

func runCleanupLifecycle(
	t *testing.T,
	instruction string,
	messages []*agent.Message,
	config StandardConfig,
) cleanupLifecycleResult {
	t.Helper()
	manager := &cleanupPlanCapture{CleanupManager: standardForTest(t, config)}
	owner, err := agent.New(context.Background(), agent.Definition{
		Name: "cleanup-lifecycle-test", Instructions: instruction,
		Model: requestCaptureModel{}, Cleanup: manager,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), agent.NamedSession("cleanup-lifecycle-test"))
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) == 0 || messages[len(messages)-1] == nil || messages[len(messages)-1].Role != agent.User {
		t.Fatal("cleanup lifecycle fixture must end with the prospective user message")
	}
	if _, err := session.SyncTranscript(context.Background(), agent.TranscriptSyncRequest{
		Source:         agent.CapabilityIdentity{Kind: "cleanup.lifecycle-test", Version: 1},
		SourceRevision: 1,
		Messages:       messages[:len(messages)-1],
	}); err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), agent.Input{
		Text: messages[len(messages)-1].Content, IdempotencyKey: "cleanup-lifecycle-test-run",
	})
	if err != nil {
		t.Fatal(err)
	}
	var result cleanupLifecycleResult
	for event := range run.Events() {
		switch payload := event.Payload.(type) {
		case agent.CleanupSkipped:
			result.reason, result.metrics = payload.Reason, payload.Metrics
		case agent.CleanupCompleted:
			result.reason, result.metrics = payload.Reason, payload.Metrics
		case agent.CleanupCommitted:
			result.committed = true
		}
	}
	if _, err := run.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Below-threshold observations intentionally do not emit lifecycle noise or
	// consume the run's one durable maintenance mutation. Retain the exact plan
	// here so policy tests can still assert its authenticated pressure metrics.
	if result.reason == "" {
		result.reason, result.metrics = manager.plan.Reason, manager.plan.Metrics
	}
	return result
}

func TestStandardEagerCleanupBatchesEveryCacheViableSettledResult(t *testing.T) {
	messages := append(recoverableGroup(
		"first", strings.Repeat("a", 1200), agent.ToolResultEagerCandidate,
		agent.ToolResultRecoveryHint{Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"path": "first.md"}},
	), recoverableGroup(
		"second", strings.Repeat("b", 1200), agent.ToolResultEagerCandidate,
		agent.ToolResultRecoveryHint{Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"path": "second.md"}},
	)...)
	manager := standardForTest(t, StandardConfig{
		ContextWindowTokens: 10_000, ReservedTokens: 0,
		CleanupThreshold: .70, CleanupTarget: .55, CompactionThreshold: .90,
		CacheState: CacheCold, EagerMinTokens: 1, EagerMinContextRatio: .01,
	})
	plan, err := manager.Plan(context.Background(), agent.CleanupPlanRequest{Messages: messages, ModelRequest: messages})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Action != agent.CleanupProject || plan.Reason != "eager_recoverable_result" || len(plan.Replacements) != 2 {
		t.Fatalf("eager plan=%#v", plan)
	}
	if plan.Metrics.EagerCandidateCount != 2 || plan.Metrics.EagerSelectedCount != 2 ||
		plan.Metrics.ReplacementCount != 2 || !plan.Metrics.EagerOnly ||
		plan.Metrics.ExecutionMode != "agent_projection" || plan.Metrics.ProviderCacheState != string(CacheCold) ||
		plan.Metrics.LocalProjectedTokens <= 0 || plan.Metrics.EffectiveTokens < plan.Metrics.LocalProjectedTokens {
		t.Fatalf("eager telemetry=%#v", plan.Metrics)
	}
}

func TestStandardLargeRecoverableResultIsNotProtectedByRecentWindow(t *testing.T) {
	messages := recoverableGroup(
		"large", strings.Repeat("large ", 1000), agent.ToolResultDeferred,
		agent.ToolResultRecoveryHint{Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"path": "large.md"}},
	)
	manager := standardForTest(t, StandardConfig{
		ContextWindowTokens: 3_000,
		CleanupThreshold:    .35, CleanupTarget: .20, CompactionThreshold: .95,
		KeepRecentGroups: 10, KeepRecentTokens: 100_000, CacheState: CacheCold,
		EagerMinTokens: 100, EagerMinContextRatio: .10,
	})
	plan, err := manager.Plan(context.Background(), agent.CleanupPlanRequest{Messages: messages, ModelRequest: messages})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Action != agent.CleanupProject || len(plan.Replacements) != 1 || plan.Metrics.CandidateTokens == 0 {
		t.Fatalf("large recoverable plan=%#v", plan)
	}
}

func TestStandardBodyAfterPrefixDoesNotMutateStablePrefixPressure(t *testing.T) {
	prefix := agent.SystemMessage(strings.Repeat("stable ", 1500))
	bodyMessages := recoverableGroup(
		"body", strings.Repeat("body ", 500), agent.ToolResultDeferred,
		agent.ToolResultRecoveryHint{Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"path": "body.md"}},
	)
	base := StandardConfig{
		ContextWindowTokens: 4_000, CleanupThreshold: .75, CleanupTarget: .70,
		CompactionThreshold: .95, CacheState: CacheCold, EagerMinTokens: 100_000,
	}
	body := runCleanupLifecycle(t, prefix.Content, bodyMessages, base)
	base.Scope = PressureTotal
	total := runCleanupLifecycle(t, prefix.Content, bodyMessages, base)
	if body.committed || body.reason != "below_cleanup_threshold" {
		t.Fatalf("body-after-prefix plan=%#v", body)
	}
	if total.metrics.BodyPressureBefore <= body.metrics.BodyPressureBefore || !total.committed {
		t.Fatalf("total plan=%#v body=%#v", total, body)
	}
}

func TestStandardUsesObservedProviderPromptTokens(t *testing.T) {
	messages := recoverableGroup(
		"observed", strings.Repeat("payload ", 500), agent.ToolResultDeferred,
		agent.ToolResultRecoveryHint{Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"path": "observed.md"}},
	)
	messages[len(messages)-2].ResponseMeta = &agent.ResponseMeta{Usage: &agent.TokenUsage{PromptTokens: 1_800}}
	manager := standardForTest(t, StandardConfig{
		Scope: PressureTotal, ContextWindowTokens: 2_000,
		CleanupThreshold: .70, CleanupTarget: .60, CompactionThreshold: .95,
		CacheState: CacheCold, EagerMinTokens: 100_000,
	})
	plan, err := manager.Plan(context.Background(), agent.CleanupPlanRequest{Messages: messages, ModelRequest: messages})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Metrics.EstimatedTokensBefore < 1_800 || plan.Metrics.PressureBefore < .9 || plan.Action != agent.CleanupProject {
		t.Fatalf("provider-observed plan=%#v", plan)
	}
}

func TestStandardHardOverflowTemporarilyDropsCacheAndRecentPreferences(t *testing.T) {
	messages := append([]*agent.Message{agent.SystemMessage(strings.Repeat("prefix ", 900))}, recoverableGroup(
		"overflow", strings.Repeat("payload ", 350), agent.ToolResultDeferred,
		agent.ToolResultRecoveryHint{Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"path": "overflow.md"}},
	)...)
	manager := standardForTest(t, StandardConfig{
		Scope: PressureTotal, ContextWindowTokens: 1_500,
		CleanupThreshold: .80, CleanupTarget: .70, CompactionThreshold: .95,
		KeepRecentGroups: 10, KeepRecentTokens: 100_000, WarmSuffixTokens: 0,
		CacheState: CacheWarm, EagerMinTokens: 100_000,
	})
	plan, err := manager.Plan(context.Background(), agent.CleanupPlanRequest{Messages: messages, ModelRequest: messages, CompactionAvailable: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Metrics.PressureBefore < 1 || len(plan.Replacements) != 1 ||
		(plan.Action != agent.CleanupProject && plan.Action != agent.CleanupCompact) {
		t.Fatalf("hard-overflow plan=%#v", plan)
	}
}

func TestStandardCompactionFallbackUsesOnlyRuntimeAvailability(t *testing.T) {
	messages := []*agent.Message{agent.UserMessage(strings.Repeat("irreducible ", 1000))}
	manager := standardForTest(t, StandardConfig{
		Scope: PressureTotal, ContextWindowTokens: 1_000,
		CleanupThreshold: .60, CleanupTarget: .50, CompactionThreshold: .70,
	})
	unavailable, err := manager.Plan(context.Background(), agent.CleanupPlanRequest{
		Messages: messages, ModelRequest: messages,
	})
	if err != nil {
		t.Fatal(err)
	}
	available, err := manager.Plan(context.Background(), agent.CleanupPlanRequest{
		Messages: messages, ModelRequest: messages, CompactionAvailable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if unavailable.FallbackToCompaction || unavailable.Action == agent.CleanupCompact ||
		!available.FallbackToCompaction || available.Action != agent.CleanupCompact {
		t.Fatalf("runtime Compaction availability: unavailable=%#v available=%#v", unavailable, available)
	}
}

func TestUsableRecoveryAcceptsGenericCompleteReferenceAndRejectsLossyNestedValues(t *testing.T) {
	if !usableRecovery(agent.ToolResultRecoveryHint{
		Kind: agent.ToolResultRecoveryRefetch, Reference: map[string]any{"custom_locator": "denova", "mode": "fresh"},
	}, nil) {
		t.Fatal("complete custom recovery arguments were not recoverable")
	}
	if !usableRecovery(agent.ToolResultRecoveryHint{
		Kind:      agent.ToolResultRecoveryRerun,
		Reference: map[string]any{"batch": []any{map[string]any{"custom_cursor": "cursor-7"}}},
	}, nil) {
		t.Fatal("nested complete rerun arguments were not accepted")
	}
	for name, reference := range map[string]map[string]any{
		"redacted":  {"nested": map[string]any{"credential": "[REDACTED]"}},
		"truncated": {"batch": []any{map[string]any{"cursor": "ok"}, "[TRUNCATED]"}},
		"long":      {"query": "prefix...[truncated]"},
		"non-json":  {"value": func() {}},
	} {
		if usableRecovery(agent.ToolResultRecoveryHint{Kind: agent.ToolResultRecoveryRerun, Reference: reference}, nil) {
			t.Fatalf("%s lossy reference was treated as recoverable: %#v", name, reference)
		}
	}
}

func TestProjectionMetricsPreserveMessageIndexZeroAsEarliestChange(t *testing.T) {
	messages := []*agent.Message{
		{Role: agent.ToolRole, Content: strings.Repeat("old ", 100)},
		agent.UserMessage("middle"),
		{Role: agent.ToolRole, Content: strings.Repeat("new ", 100)},
		agent.AssistantMessage("tail", nil),
	}
	metrics := agent.CleanupMetrics{}
	applyProjectionMetrics(messages, []agent.CleanupReplacement{
		{MessageIndex: 0, OriginalTokens: 100, PlaceholderTokens: 10},
		{MessageIndex: 2, OriginalTokens: 100, PlaceholderTokens: 10},
	}, &metrics)
	if metrics.EarliestChanged != 0 || metrics.ReclaimedTokens != 180 ||
		metrics.WarmSuffixTokens != EstimateMessages(messages[1:]) {
		t.Fatalf("projection metrics = %#v", metrics)
	}
}

func TestStandardRejectsUnknownCacheState(t *testing.T) {
	if _, err := Standard(StandardConfig{
		ContextWindowTokens: 1_000, CacheState: CacheState("partly-warm"),
	}); err == nil || !strings.Contains(err.Error(), "CacheState") {
		t.Fatalf("invalid CacheState error = %v", err)
	}
}
