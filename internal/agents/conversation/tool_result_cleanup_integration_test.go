package conversation

import (
	"context"
	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/toolresult"
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
