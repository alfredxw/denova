package agents

import (
	"context"
	"errors"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/session"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestSessionConversationStructuralCursorRejectsStaleContextWrite(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("stale-structural-cursor")
	if err != nil {
		t.Fatal(err)
	}
	cursor := sess.ContextCursor()
	conversation := NewSessionConversation(sess).WithContextCursorBarrier(cursor)
	if err := sess.Append(agent.UserMessage("concurrent input")); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendContextMessage(agent.UserMessage("stale structural context")); !errors.Is(err, session.ErrContextRevisionConflict) {
		t.Fatalf("stale structural write error = %v, want %v", err, session.ErrContextRevisionConflict)
	}
}

func TestSessionConversationRejectsCanonicalWritesWithoutDurableCycleIdentity(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("missing-cycle")
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversation(sess)
	if err := conversation.AppendAssistant("must not bypass actor"); !errors.Is(err, ErrMissingAgentCycleIdentity) {
		t.Fatalf("assistant append error = %v, want %v", err, ErrMissingAgentCycleIdentity)
	}
	if err := conversation.AppendContextMessage(agent.ToolMessage("must not bypass actor", "call-1")); !errors.Is(err, ErrMissingAgentCycleIdentity) {
		t.Fatalf("context append error = %v, want %v", err, ErrMissingAgentCycleIdentity)
	}
	if got := sess.MessageCountTotal(); got != 0 {
		t.Fatalf("identity-less writes appended %d messages", got)
	}
}

func TestSessionConversationStagesAssistantUntilAuthorizedCycleCommit(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("staged-cycle")
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversation(sess)
	identity := HarnessCycleIdentity{CommandID: runstate.CommandID("command-1"), OperationID: runstate.OperationID("operation-1"), Cycle: 1}
	conversation.BindAgentCycleIdentity(identity)
	if _, err := assembleAndCommitModelContextForTest(conversation, "user input", "user input"); err != nil {
		t.Fatal(err)
	}
	if err := conversation.AppendAssistant("staged answer"); err != nil {
		t.Fatal(err)
	}
	if got := sess.MessageCountTotal(); got != 1 {
		t.Fatalf("message count before authorization = %d, want only accepted user input", got)
	}
	intent, ok, err := conversation.PendingAgentCycleCommit(HarnessDomainCommitOutput)
	if err != nil || !ok || intent.Identity != identity || intent.Stage != HarnessDomainCommitOutput || intent.Hash == "" {
		t.Fatalf("pending intent = %+v ok=%t err=%v", intent, ok, err)
	}
	if err := conversation.CommitAgentCycle(context.Background(), RunOutcome{Status: RunOutcomeCompleted}); err != nil {
		t.Fatal(err)
	}
	if got := sess.MessageCountTotal(); got != 2 {
		t.Fatalf("message count after authorization = %d, want user and assistant", got)
	}
	if receipt, ok := conversation.LastAgentCycleCommitReceipt(HarnessDomainCommitOutput); !ok || receipt.Identity != identity || receipt.Hash != intent.Hash || receipt.Revision == "" {
		t.Fatalf("commit receipt = %+v ok=%t", receipt, ok)
	}
}

func TestSessionConversationDiscardsAssistantForAbortAndFailure(t *testing.T) {
	for _, status := range []RunOutcomeStatus{RunOutcomeAborted, RunOutcomeFailed} {
		t.Run(string(status), func(t *testing.T) {
			store, err := session.NewStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			sess, err := store.GetOrCreate("discard-cycle")
			if err != nil {
				t.Fatal(err)
			}
			conversation := NewSessionConversation(sess)
			conversation.BindAgentCycleIdentity(HarnessCycleIdentity{
				CommandID: "command-1", OperationID: "operation-1", Cycle: 1,
			})
			if _, err := assembleAndCommitModelContextForTest(conversation, "user input", "user input"); err != nil {
				t.Fatal(err)
			}
			if err := conversation.AppendAssistant("must not commit"); err != nil {
				t.Fatal(err)
			}
			if err := conversation.CommitAgentCycle(context.Background(), RunOutcome{Status: status}); err != nil {
				t.Fatal(err)
			}
			if got := sess.MessageCountTotal(); got != 1 {
				t.Fatalf("message count = %d, aborted/failed assistant was committed", got)
			}
			if _, ok := conversation.LastAgentCycleCommitReceipt(HarnessDomainCommitOutput); ok {
				t.Fatal("aborted/failed cycle exposed a commit receipt")
			}
		})
	}
}
