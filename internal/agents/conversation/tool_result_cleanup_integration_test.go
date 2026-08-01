package conversation

import (
	"context"
	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/toolresult"
	"errors"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/session"
)

func TestSessionToolResultCleanupPersistsProjectionWithoutRewritingHistory(t *testing.T) {
	directory := t.TempDir()
	store, err := session.NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("cleanup-end-to-end")
	if err != nil {
		t.Fatal(err)
	}
	rich := pressureToolResult("call-read", strings.Repeat("complete evidence ", 3000), agent.ToolResultDeferred, agent.ToolResultContextDiscardable)
	if err := sess.AppendContextMessages(
		agent.UserMessage("inspect"),
		agent.AssistantMessage("", []agent.ToolCall{pressureCall("call-read", 1)}),
		rich,
		agent.AssistantMessage("evidence incorporated", nil),
		agent.UserMessage("continue"),
	); err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversationForAgent(sess, &config.Config{}, agentrun.AgentKindIDE)
	snapshot, err := sess.SnapshotContext(agentrun.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	visible := conversation.modelHistory(snapshot)
	placeholder := "[Older tool result removed. Recovery: read chapter-1.md.]"
	plan := toolresult.CleanupPlan{
		Replacements: []toolresult.CleanupReplacement{{
			MessageIndex: 2, ToolCallID: "call-read", Placeholder: placeholder,
			OriginalTokens: agentcontext.EstimateMessageTokens(visible[2]), PlaceholderTokens: agentcontext.EstimateStringTokens(placeholder),
		}},
		ReclaimedTokens: 1000, EarliestChanged: 1, RendererVersion: agentcontext.ToolResultPlaceholderRendererVersion,
	}
	if err := conversation.StageToolResultCleanup(context.Background(), visible, plan); err != nil {
		t.Fatal(err)
	}
	if err := conversation.CommitPostSettlementToolResultCleanup(context.Background(), "settled-operation"); err != nil {
		t.Fatal(err)
	}

	raw := sess.GetEffectiveMessages()
	if len(raw) != 5 || raw[2].Content != rich.Content {
		t.Fatalf("cleanup rewrote canonical/display history: %#v", raw)
	}
	reloadedStore, err := session.NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.GetOrCreate("cleanup-end-to-end")
	if err != nil {
		t.Fatal(err)
	}
	reloadedSnapshot, err := reloaded.SnapshotContext(agentrun.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	projected := NewSessionConversationForAgent(reloaded, &config.Config{}, agentrun.AgentKindIDE).modelHistory(reloadedSnapshot)
	if len(projected) != 5 || projected[2].Content != placeholder {
		t.Fatalf("cleanup projection did not survive reload: %#v", projected)
	}
}

func TestSessionCleanupDefersAcrossRewindWithReusedProviderCallID(t *testing.T) {
	directory := t.TempDir()
	store, err := session.NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("cleanup-rewind-id-reuse")
	if err != nil {
		t.Fatal(err)
	}
	call := pressureCall("provider-local-call", 0)
	kept := pressureToolResult("provider-local-call", strings.Repeat("kept evidence ", 2000), agent.ToolResultDeferred, agent.ToolResultContextDiscardable)
	prefix := []*agent.Message{agent.AssistantMessage("", []agent.ToolCall{call}), kept}
	if err := sess.AppendContextMessages(prefix...); err != nil {
		t.Fatal(err)
	}
	checkpointCursor := sess.ContextCursor()
	boundary, err := session.NewContextBoundarySnapshot(checkpointCursor, prefix, prefix, 4*1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	locator, err := sess.StoreContextBoundary("cleanup-rewind-checkpoint", boundary)
	if err != nil {
		t.Fatal(err)
	}
	discarded := pressureToolResult("provider-local-call", "discarded branch evidence", agent.ToolResultDeferred, agent.ToolResultContextDiscardable)
	if err := sess.AppendContextMessages(agent.AssistantMessage("discarded exploration", []agent.ToolCall{call}), discarded); err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendWithMetadata(agent.AssistantMessage("final after rewind", nil), session.MessageMetadata{
		ContextOperations: []session.ContextOperation{{
			Kind: session.ContextOperationRewind, AgentKind: agentrun.AgentKindIDE,
			CheckpointID: "cleanup-rewind-checkpoint", MessageCount: checkpointCursor.MessageCount,
			BoundaryID: "cleanup-rewind-checkpoint", BoundaryLocator: locator, Report: "keep the original evidence",
		}},
	}); err != nil {
		t.Fatal(err)
	}

	conversation := NewSessionConversationForAgent(sess, &config.Config{}, agentrun.AgentKindIDE)
	snapshot, err := sess.SnapshotContext(agentrun.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	visible := conversation.modelHistory(snapshot)
	resultIndex := -1
	for index, message := range visible {
		if message != nil && message.Role == agent.ToolRole && message.Content == kept.Content {
			resultIndex = index
			break
		}
	}
	if resultIndex < 0 || strings.Contains(joinedContextMessageContent(visible), "discarded branch evidence") {
		t.Fatalf("rewind projection = %#v", visible)
	}
	plan := toolresult.CleanupPlan{Replacements: []toolresult.CleanupReplacement{{
		MessageIndex: resultIndex, ToolCallID: "provider-local-call", Placeholder: "rewind-unsafe placeholder",
		OriginalTokens: agentcontext.EstimateMessageTokens(visible[resultIndex]), PlaceholderTokens: 4,
	}}}
	if err := conversation.StageToolResultCleanup(context.Background(), visible, plan); !errors.Is(err, errToolResultCleanupBlockedByRewind) {
		t.Fatalf("cleanup across active rewind error = %v", err)
	}
	policy := conversation.ContextPressurePolicy(visible)
	if policy.CleanupEnabled {
		t.Fatal("active rewind must disable cleanup before planning")
	}
	policy.ContextWindowTokens = 10
	policy.ReservedTokens = 10
	decision := agentcontext.PlanContextPressure(visible, nil, policy)
	if decision.Action != agentcontext.ContextMaintenanceCompaction {
		t.Fatalf("hard pressure with active rewind must choose compaction: %#v", decision)
	}

	reloadedStore, err := session.NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.GetOrCreate("cleanup-rewind-id-reuse")
	if err != nil {
		t.Fatal(err)
	}
	reloadedSnapshot, err := reloaded.SnapshotContext(agentrun.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	reloadedVisible := NewSessionConversationForAgent(reloaded, &config.Config{}, agentrun.AgentKindIDE).modelHistory(reloadedSnapshot)
	if !agentcontext.MessagesEqual(visible, reloadedVisible) {
		t.Fatalf("rewind projection changed after reload:\ntransient=%#v\nreloaded=%#v", visible, reloadedVisible)
	}
}

func TestSessionCleanupDefersSameTurnStagedRewind(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("cleanup-staged-rewind")
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversationForAgent(sess, &config.Config{}, agentrun.AgentKindIDE)
	conversation.BindAgentCycleIdentity(agentrun.CycleIdentity{CommandID: "command", OperationID: "operation", Cycle: 1})
	if err := conversation.StageContextOperation(session.ContextOperation{
		Kind: session.ContextOperationRewind, AgentKind: agentrun.AgentKindIDE, CheckpointID: "pending-rewind",
	}); err != nil {
		t.Fatal(err)
	}
	visible := []*agent.Message{pressureToolResult(
		"call", strings.Repeat("rich ", 1000), agent.ToolResultDeferred, agent.ToolResultContextDiscardable,
	)}
	if policy := conversation.ContextPressurePolicy(visible); policy.CleanupEnabled {
		t.Fatal("same-turn staged rewind must disable cleanup before durable publication")
	}
	err = conversation.StageToolResultCleanup(context.Background(), visible, toolresult.CleanupPlan{
		Replacements: []toolresult.CleanupReplacement{{MessageIndex: 0, ToolCallID: "call", Placeholder: "receipt"}},
	})
	if !errors.Is(err, errToolResultCleanupBlockedByRewind) {
		t.Fatalf("staged rewind cleanup error = %v", err)
	}
}
