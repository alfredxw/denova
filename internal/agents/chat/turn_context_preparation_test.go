package chat

import (
	"context"
	agentcontext "denova/internal/agents/context"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents/session"
)

func TestPrepareTurnContextIsPureUntilExplicitCommit(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("prepared-turn")
	if err != nil {
		t.Fatal(err)
	}
	conversation := agentconversation.NewSessionConversationForAgent(sess, &config.Config{}, config.AgentKindIDE)
	conversation.BindAgentCycleIdentity(agentrun.CycleIdentity{
		CommandID: "prepare-turn-command", OperationID: "prepare-turn-operation", Cycle: 1,
	})

	prepared, err := prepareTurnContext(context.Background(), turnContextPreparationInput{
		Conversation: conversation,
		Request:      ChatRequest{Message: "告诉我当前时间"},
		Environment:  fixedTurnRuntimeEnvironment(nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := sess.MessageCountTotal(); got != 0 {
		t.Fatalf("pure turn preparation persisted %d messages before commit", got)
	}
	if prepared.OriginalMessage != "告诉我当前时间" || prepared.ResumeInterruption != nil {
		t.Fatalf("prepared turn identity = %#v", prepared)
	}
	final := finalModelUserMessage(prepared.ModelContext.Messages, "")
	if !strings.Contains(final, "Captured at: 2026-07-24T07:30:20Z") ||
		!strings.HasSuffix(strings.TrimSpace(final), "告诉我当前时间") {
		t.Fatalf("prepared model input is missing the runtime snapshot or raw request:\n%s", final)
	}

	if err := agentcontext.CommitModelInput(context.Background(), conversation, prepared.OriginalMessage, prepared.ModelContext); err != nil {
		t.Fatal(err)
	}
	visible := sess.History()
	if len(visible) != 1 || visible[0].Content != "告诉我当前时间" {
		t.Fatalf("commit must persist only the raw user message: %#v", visible)
	}
}

func TestPrepareTurnContextCarriesUserReferencesIntoCommitState(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("prepared-turn-references")
	if err != nil {
		t.Fatal(err)
	}
	conversation := agentconversation.NewSessionConversationForAgent(sess, &config.Config{}, config.AgentKindIDE)
	conversation.BindAgentCycleIdentity(agentrun.CycleIdentity{
		CommandID: "prepare-references-command", OperationID: "prepare-references-operation", Cycle: 1,
	})

	prepared, err := prepareTurnContext(context.Background(), turnContextPreparationInput{
		Conversation: conversation,
		Request: ChatRequest{
			Message:    "修改这一章",
			References: []string{"chapters/ch01.md"},
		},
		Environment: fixedTurnRuntimeEnvironment(nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := agentcontext.CommitModelInput(context.Background(), conversation, prepared.OriginalMessage, prepared.ModelContext); err != nil {
		t.Fatal(err)
	}
	visible := sess.History()
	if len(visible) != 1 || len(visible[0].UserReferences) != 1 ||
		visible[0].UserReferences[0].Kind != "file" ||
		visible[0].UserReferences[0].Label != "chapters/ch01.md" {
		t.Fatalf("prepared user references were not committed: %#v", visible)
	}
}
