package lifecycle

import (
	"context"
	"testing"

	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"

	agent "github.com/alfredxw/denova/agent"
)

func TestSessionConversationBoundaryCommitsAndReconcilesPublicHashes(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("public-boundary")
	if err != nil {
		t.Fatal(err)
	}
	options := agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, SessionID: sess.ID, Workspace: "/book",
		RootAgentName: "DenovaAgent",
	}
	conversation := agentconversation.NewSessionConversationForAgent(sess, nil, options.AgentKind)
	inputCallbacks := 0
	committer, err := NewSessionConversationCommitter(SessionCommitterConfig{
		Conversation: conversation, Session: sess, Options: options,
		Request: agentchat.ChatRequest{CommandID: "command-1", Message: "first request"},
		InputCommitted: func(context.Context) error {
			inputCallbacks++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := NewConversationBoundary(ConversationBoundaryConfig{
		Conversation:      conversation,
		Request:           agentchat.ChatRequest{CommandID: "command-1", Message: "first request"},
		Options:           options,
		ContextIdentity:   agent.CapabilityIdentity{Kind: "context.session-boundary-test", Version: 1},
		CanonicalIdentity: agent.CapabilityIdentity{Kind: "canonical.session-boundary-test", Version: 1},
		Committer:         committer,
	})
	if err != nil {
		t.Fatal(err)
	}
	inputIdentity := agent.CommitIdentity{
		CommandID: "command-1", RunID: "run-1", Cycle: 1, Stage: agent.CommitInput,
	}
	inputReceipt, err := boundary.CanonicalAdapter().MaterializeInput(context.Background(), agent.InputCommitRequest{
		Identity: inputIdentity, Hash: "public-input-hash", Input: agent.Text("first request"),
	})
	if err != nil || inputReceipt.Revision == "" {
		t.Fatalf("input receipt=%#v error=%v", inputReceipt, err)
	}
	replayedInput, err := boundary.CanonicalAdapter().MaterializeInput(context.Background(), agent.InputCommitRequest{
		Identity: inputIdentity, Hash: "public-input-hash", Input: agent.Text("first request"),
	})
	if err != nil || replayedInput.Revision != inputReceipt.Revision || inputCallbacks != 1 {
		t.Fatalf("replayed input=%#v callbacks=%d error=%v", replayedInput, inputCallbacks, err)
	}
	outputIdentity := inputIdentity
	outputIdentity.Stage = agent.CommitOutput
	outputReceipt, err := boundary.CanonicalAdapter().CommitOutput(context.Background(), agent.OutputCommitRequest{
		Identity: outputIdentity, Hash: "public-output-hash", Message: *agent.AssistantMessage("answer", nil),
	})
	if err != nil || outputReceipt.Revision == "" {
		t.Fatalf("output receipt=%#v error=%v", outputReceipt, err)
	}
	for _, check := range []struct {
		identity agent.CommitIdentity
		hash     string
	}{
		{inputIdentity, "public-input-hash"},
		{outputIdentity, "public-output-hash"},
	} {
		result, reconcileErr := boundary.CanonicalAdapter().Reconcile(context.Background(), agent.ReconcileRequest{
			Identity: check.identity, Hash: check.hash,
		})
		if reconcileErr != nil || !result.Found || result.Revision == "" {
			t.Fatalf("reconcile=%#v error=%v", result, reconcileErr)
		}
	}
	history := sess.History()
	if len(history) != 2 || history[0].Content != "first request" || history[1].Content != "answer" ||
		history[0].AgentCanonicalHash != "public-input-hash" || history[1].AgentCanonicalHash != "public-output-hash" {
		t.Fatalf("session history=%#v", history)
	}
}
