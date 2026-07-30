package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/interactive"
)

func TestInteractiveToolResultCleanupStagesAfterSettlementAndPreservesRichTurn(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "cleanup adapter", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	summary := &agent.ToolResultSummary{
		Status: agent.ToolResultSuccess, ResultRetention: agent.ToolResultDeferred,
		ContextHints: &agent.ToolResultContextHints{
			Recovery:     agent.ToolResultRecoveryHint{Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"path": "lore/cast.md"}},
			ContextValue: agent.ToolResultContextDiscardable,
		},
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		BranchID: "main", User: "inspect", Narrative: "evidence incorporated",
		ModelContextMessages: []interactive.ModelContextMessage{
			{Role: "assistant", ToolCalls: []interactive.ModelContextToolCall{{
				ID: "call-read", Type: "function", Function: interactive.ModelContextFunctionCall{Name: "read", Arguments: `{"path":"lore/cast.md"}`},
			}}},
			{Role: "tool", Content: strings.Repeat("complete cast evidence ", 1000), ToolCallID: "call-read", ToolName: "read", ToolResult: summary},
		},
	}); err != nil {
		t.Fatal(err)
	}
	conversation := newInteractiveConversation(store, "", workspace, story.ID, "main", "", 0, &config.Config{})
	storyContext, err := conversation.storyContextForCycle()
	if err != nil {
		t.Fatal(err)
	}
	history, compaction, err := conversation.modelHistoryForCycle(storyContext)
	if err != nil {
		t.Fatal(err)
	}
	visible := interactiveEffectiveModelMessages(buildInteractiveModelVisibleHistory(history, compaction), compaction)
	if len(visible) != 4 || visible[2].ToolCallID != "call-read" {
		t.Fatalf("unexpected provider history: %#v", visible)
	}
	placeholder := "[Older tool result removed. Recovery: read lore/cast.md.]"
	plan := agents.ToolResultCleanupPlan{
		Replacements: []agents.ToolResultCleanupReplacement{{
			MessageIndex: 2, ToolCallID: "call-read", Placeholder: placeholder,
			OriginalTokens: agents.EstimateContextTokens([]*agents.Message{visible[2]}, nil), PlaceholderTokens: 20,
		}},
		ReclaimedTokens: 1000, EarliestChanged: 1, RendererVersion: "tool-result-placeholder/v1",
	}
	if err := conversation.StageToolResultCleanup(context.Background(), visible, plan); err != nil {
		t.Fatal(err)
	}
	if err := conversation.CommitPostSettlementToolResultCleanup(context.Background(), "settled-game-operation"); err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ToolResultCleanup == nil || snapshot.Turns[0].ModelContextMessages[1].Content == placeholder {
		t.Fatalf("cleanup record/raw turn boundary is wrong: %#v", snapshot)
	}
	projected := applyInteractiveToolResultCleanup(visible, *snapshot.ToolResultCleanup)
	if projected[2].Content != placeholder || visible[2].Content == placeholder {
		t.Fatalf("interactive cleanup did not remain a model-only projection: projected=%q raw=%q", projected[2].Content, visible[2].Content)
	}
}

func TestInteractiveToolResultCleanupIndexesResolvedAcceptanceProjection(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "resolved cleanup", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		BranchID: "main", User: "opening", Narrative: "opening answer",
	}); err != nil {
		t.Fatal(err)
	}

	interrupted := interactive.DomainCommitIdentity{CommandID: "cleanup-interrupted", OperationID: "cleanup-interrupted-op", Cycle: 1}
	input, err := interactive.NewPlayerInputIntent(interrupted, "main", "inspect interrupted lore")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, input); err != nil {
		t.Fatal(err)
	}
	summary := &agent.ToolResultSummary{
		Status: agent.ToolResultSuccess, ResultRetention: agent.ToolResultDeferred,
		ContextHints: &agent.ToolResultContextHints{
			Recovery:     agent.ToolResultRecoveryHint{Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"path": "lore/interrupted.md"}},
			ContextValue: agent.ToolResultContextDiscardable,
		},
	}
	const callID = "resolved-cleanup-call"
	const richResult = "complete interrupted evidence that remains durable"
	intents, err := interactive.NewModelContextBatchIntents(interrupted, "main", 0, []interactive.ModelContextMessage{
		{Role: "assistant", ToolCalls: []interactive.ModelContextToolCall{{
			ID: callID, Type: "function", Function: interactive.ModelContextFunctionCall{Name: "read", Arguments: `{"path":"lore/interrupted.md"}`},
		}}},
		{Role: "tool", ToolCallID: callID, ToolName: "read", Content: richResult, ToolResult: summary},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendModelContextBatch(story.ID, intents[0]); err != nil {
		t.Fatal(err)
	}

	settling := interactive.DomainCommitIdentity{CommandID: "cleanup-settling", OperationID: "cleanup-settling-op", Cycle: 1}
	settlingInput, err := interactive.NewPlayerInputIntent(settling, "main", "settle later")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, settlingInput); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AppendTurnWithState(story.ID, interactive.AppendTurnWithStateRequest{
		BranchID: "main", User: "settle later", Narrative: "settled answer",
		AgentKind: "interactive_story", AgentCommandID: settling.CommandID,
		AgentOperationID: settling.OperationID, AgentCycle: settling.Cycle,
	}); err != nil {
		t.Fatal(err)
	}

	visible := interactiveProjectionFromStoreForTest(t, store, story.ID, agents.HarnessCycleIdentity{}).Messages
	targetIndex := -1
	for index, message := range visible {
		if message != nil && message.Role == agents.RoleTool && message.ToolCallID == callID {
			targetIndex = index
			break
		}
	}
	if targetIndex < 0 || visible[targetIndex].Content != richResult {
		t.Fatalf("resolved tool result missing from canonical acceptance projection: %#v", visible)
	}

	conversation := newInteractiveConversation(store, "", workspace, story.ID, "main", "", 0, &config.Config{})
	const placeholder = "[Older tool result removed. Recovery: read lore/interrupted.md.]"
	plan := agents.ToolResultCleanupPlan{
		Replacements: []agents.ToolResultCleanupReplacement{{
			MessageIndex: targetIndex, ToolCallID: callID, Placeholder: placeholder,
			OriginalTokens: agents.EstimateContextTokens([]*agents.Message{visible[targetIndex]}, nil), PlaceholderTokens: 20,
		}},
		ReclaimedTokens: 100, EarliestChanged: targetIndex, RendererVersion: "tool-result-placeholder/v1",
	}
	if err := conversation.StageToolResultCleanup(context.Background(), visible, plan); err != nil {
		t.Fatal(err)
	}
	if err := conversation.CommitPostSettlementToolResultCleanup(context.Background(), "resolved-cleanup-operation"); err != nil {
		t.Fatal(err)
	}

	projected := interactiveProjectionFromStoreForTest(t, interactive.NewStore(workspace), story.ID, agents.HarnessCycleIdentity{})
	if len(projected.Messages) <= targetIndex || projected.Messages[targetIndex].Content != placeholder {
		t.Fatalf("resolved cleanup index did not survive cold projection: %#v", projected.Messages)
	}
	snapshot, err := store.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	owner := snapshot.Turns[len(snapshot.Turns)-1]
	if len(owner.ResolvedPlayerInputContexts) != 1 ||
		owner.ResolvedPlayerInputContexts[0].ModelContextBatches[0].Messages[1].Content != richResult {
		t.Fatalf("model-only cleanup changed rich resolved context: %#v", owner.ResolvedPlayerInputContexts)
	}
}

func TestLatestInteractiveModelPromptUsageUsesFinalMatchingCall(t *testing.T) {
	events := []interactive.TokenUsageEvent{
		{AgentKind: config.AgentKindIDE, PromptTokens: 90_000, ModelCalls: 1},
		{
			AgentKind: config.AgentKindInteractiveStory, PromptTokens: 42_000, CachedPromptTokens: 20_000, ModelCalls: 2,
			UsageCalls: []interactive.TokenUsageCall{
				{Index: 1, PromptTokens: 18_000, CachedPromptTokens: 8_000},
				{Index: 2, PromptTokens: 24_000, CachedPromptTokens: 12_000},
			},
		},
	}
	snapshot := interactive.Snapshot{TokenUsageEvents: events}
	prompt, cached, ok := latestInteractiveModelPromptUsage(snapshot, config.AgentKindInteractiveStory)
	if !ok || prompt != 24_000 || cached != 12_000 {
		t.Fatalf("latest prompt usage = (%d, %d, %t)", prompt, cached, ok)
	}
	snapshot.ToolResultCleanup = &interactive.ToolResultCleanupEvent{Ts: "2099-01-01T00:00:00Z"}
	if _, _, ok := latestInteractiveModelPromptUsage(snapshot, config.AgentKindInteractiveStory); ok {
		t.Fatal("usage from before a structural context change remained active")
	}
}

func TestInteractiveContextPressurePolicyFailsClosedWhenCanonicalSnapshotIsUnavailable(t *testing.T) {
	workspace := t.TempDir()
	conversation := newInteractiveConversation(
		interactive.NewStore(workspace), "", workspace, "missing-story", "main", "", 0, &config.Config{},
	)
	policy := conversation.ContextPressurePolicy([]*agents.Message{agents.UserMessage("current")})
	if policy.CleanupEnabled || policy.ObservedPromptTokens != 0 {
		t.Fatalf("cleanup remained enabled without canonical Game context: %#v", policy)
	}
}

func TestInteractiveCleanupDefersPendingSideBatchWithReusedProviderCallID(t *testing.T) {
	workspace := t.TempDir()
	store := interactive.NewStore(workspace)
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "pending cleanup boundary", StoryTellerID: "classic"})
	if err != nil {
		t.Fatal(err)
	}
	summary := &agent.ToolResultSummary{
		Status: agent.ToolResultSuccess, ResultRetention: agent.ToolResultDeferred,
		ContextHints: &agent.ToolResultContextHints{
			Recovery:     agent.ToolResultRecoveryHint{Kind: agent.ToolResultRecoveryRead, Reference: map[string]any{"path": "lore/reused.md"}},
			ContextValue: agent.ToolResultContextDiscardable,
		},
	}
	completedMessages := []interactive.ModelContextMessage{
		{Role: "assistant", ToolCalls: []interactive.ModelContextToolCall{{
			ID: "reused-call", Type: "function", Function: interactive.ModelContextFunctionCall{Name: "read", Arguments: `{"path":"lore/old.md"}`},
		}}},
		{Role: "tool", ToolCallID: "reused-call", ToolName: "read", Content: "completed evidence", ToolResult: summary},
	}
	if _, err := store.AppendTurn(story.ID, interactive.AppendTurnRequest{
		BranchID: "main", User: "completed input", Narrative: "completed answer", ModelContextMessages: completedMessages,
	}); err != nil {
		t.Fatal(err)
	}
	identity := interactive.DomainCommitIdentity{CommandID: "pending-command", OperationID: "pending-operation", Cycle: 1}
	input, err := interactive.NewPlayerInputIntent(identity, "main", "pending input")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitPlayerInput(story.ID, input); err != nil {
		t.Fatal(err)
	}
	pendingMessages := []interactive.ModelContextMessage{
		{Role: "assistant", ToolCalls: []interactive.ModelContextToolCall{{
			ID: "reused-call", Type: "function", Function: interactive.ModelContextFunctionCall{Name: "read", Arguments: `{"path":"lore/pending.md"}`},
		}}},
		{Role: "tool", ToolCallID: "reused-call", ToolName: "read", Content: "pending evidence", ToolResult: summary},
	}
	intents, err := interactive.NewModelContextBatchIntents(identity, "main", 0, pendingMessages)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendModelContextBatch(story.ID, intents[0]); err != nil {
		t.Fatal(err)
	}

	conversation := newInteractiveConversation(store, "", workspace, story.ID, "main", "", 0, &config.Config{})
	storyCtx, err := conversation.storyContextForCycle()
	if err != nil {
		t.Fatal(err)
	}
	history, compaction, err := conversation.modelHistoryForCycle(storyCtx)
	if err != nil {
		t.Fatal(err)
	}
	visible := interactiveEffectiveModelMessages(buildInteractiveModelVisibleHistory(history, compaction), compaction)
	visible = append(visible, agents.UserMessage("pending input"))
	visible = append(visible, schemaMessagesFromInteractiveContext(pendingMessages)...)
	pendingIndex := len(visible) - 1
	plan := agents.ToolResultCleanupPlan{Replacements: []agents.ToolResultCleanupReplacement{{
		MessageIndex: pendingIndex, ToolCallID: "reused-call", Placeholder: "must not persist",
		OriginalTokens: agents.EstimateContextTokens([]*agents.Message{visible[pendingIndex]}, nil), PlaceholderTokens: 4,
	}}}
	if err := conversation.StageToolResultCleanup(context.Background(), visible, plan); !errors.Is(err, errInteractiveCleanupBlockedByPendingModelContext) {
		t.Fatalf("pending side-batch cleanup error = %v", err)
	}
	policy := conversation.ContextPressurePolicy(visible)
	if policy.CleanupEnabled {
		t.Fatal("pending side batch must disable cleanup before planning")
	}
	policy.ContextWindowTokens = 10
	policy.ReservedTokens = 10
	if decision := agents.PlanContextPressure(visible, nil, policy); decision.Action != agents.ContextMaintenanceCompaction {
		t.Fatalf("hard pressure with pending side batch must compact: %#v", decision)
	}

	reloaded := interactive.NewStore(workspace)
	snapshot, err := reloaded.Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ToolResultCleanup != nil || len(snapshot.PendingModelContextBatches) != 1 ||
		snapshot.Turns[0].ModelContextMessages[1].Content != "completed evidence" ||
		snapshot.PendingModelContextBatches[0].Messages[1].Content != "pending evidence" {
		t.Fatalf("failed cleanup changed durable game context: %#v", snapshot)
	}
}

// interactiveEffectiveModelMessages is intentionally test-only. Production
// assembly and cleanup indexing must use buildInteractiveModelContextProjection
// so resolved and pending input boundaries cannot diverge.
func interactiveEffectiveModelMessages(history interactiveTurnHistory, compaction *interactive.ContextCompactionEvent) []*agents.Message {
	messages := make([]*agents.Message, 0, len(history.Turns)*3+1)
	if compaction != nil && strings.TrimSpace(compaction.Summary) != "" {
		messages = append(messages, agents.NewContextCompactionSummaryMessage(compaction.Epoch, compaction.Summary))
	}
	for _, turn := range history.Turns {
		messages = append(messages, agents.UserMessage(turn.User))
		messages = append(messages, schemaMessagesFromInteractiveContext(turn.ModelContextMessages)...)
		messages = append(messages, agents.AssistantMessage(turn.Narrative, nil))
	}
	return messages
}
