package conversation

import (
	"context"
	agentrun "denova/internal/agents/run"
	"errors"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"

	"denova/internal/agents/session"
)

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
	if err := conversation.AppendContextMessage(agent.ToolMessage(agent.TextToolResult("must not bypass actor"), "call-1")); !errors.Is(err, ErrMissingAgentCycleIdentity) {
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
	identity := agentrun.CycleIdentity{CommandID: agentrun.CommandID("command-1"), OperationID: agentrun.OperationID("operation-1"), Cycle: 1}
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
	intent, ok, err := conversation.PendingAgentCycleCommit(agentrun.DomainCommitOutput)
	if err != nil || !ok || intent.Identity != identity || intent.Stage != agentrun.DomainCommitOutput || intent.Hash == "" {
		t.Fatalf("pending intent = %+v ok=%t err=%v", intent, ok, err)
	}
	if err := conversation.CommitAgentCycle(context.Background(), agentrun.Outcome{Status: agentrun.OutcomeCompleted}); err != nil {
		t.Fatal(err)
	}
	if got := sess.MessageCountTotal(); got != 2 {
		t.Fatalf("message count after authorization = %d, want user and assistant", got)
	}
	if receipt, ok := conversation.LastAgentCycleCommitReceipt(agentrun.DomainCommitOutput); !ok || receipt.Identity != identity || receipt.Hash != intent.Hash || receipt.Revision == "" {
		t.Fatalf("commit receipt = %+v ok=%t", receipt, ok)
	}
}

func TestSessionConversationCommitsOnlyProviderContinuationWithFinalAssistant(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("provider-continuation")
	if err != nil {
		t.Fatal(err)
	}
	conversation := NewSessionConversation(sess)
	conversation.BindAgentCycleIdentity(agentrun.CycleIdentity{CommandID: "command-1", OperationID: "operation-1", Cycle: 1})

	config := providers.ModelConfig{
		Provider: providers.ProviderOpenAI, Protocol: providers.ProtocolOpenAIResponses,
		Model: "gpt-5", BaseURL: "https://api.openai.com/v1",
	}
	continuation, err := providers.NewContinuation(config, []any{
		map[string]any{"type": "reasoning", "encrypted_content": "encrypted-state"},
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata := session.MessageMetadata{ProviderContinuation: map[string]any{
		"request-id":                   "telemetry-must-not-persist",
		providers.ExtraKeyContinuation: continuation,
	}}
	if err := conversation.AppendAssistantWithMetadata("final answer", "readable reasoning", metadata); err != nil {
		t.Fatal(err)
	}
	if err := conversation.CommitAgentCycle(context.Background(), agentrun.Outcome{Status: agentrun.OutcomeCompleted}); err != nil {
		t.Fatal(err)
	}

	messages := sess.GetEffectiveMessages()
	if len(messages) != 1 || messages[0].Content != "final answer" || len(messages[0].Extra) != 1 {
		t.Fatalf("committed assistant = %#v", messages)
	}
	var output []any
	matched, err := providers.DecodeContinuation(messages[0].Extra, config, &output)
	if err != nil || !matched || len(output) != 1 {
		t.Fatalf("committed continuation = %#v matched=%t err=%v", messages[0].Extra, matched, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedStore, err := session.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopenedStore.Close() })
	reopened, err := reopenedStore.Get("provider-continuation")
	if err != nil {
		t.Fatal(err)
	}
	messages = reopened.GetEffectiveMessages()
	matched, err = providers.DecodeContinuation(messages[0].Extra, config, &output)
	if err != nil || !matched || len(messages) != 1 || len(output) != 1 {
		t.Fatalf("reopened continuation = %#v matched=%t err=%v", messages, matched, err)
	}
}

func TestSessionConversationDiscardsAssistantForAbortAndFailure(t *testing.T) {
	for _, status := range []agentrun.OutcomeStatus{agentrun.OutcomeAborted, agentrun.OutcomeFailed} {
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
			conversation.BindAgentCycleIdentity(agentrun.CycleIdentity{
				CommandID: "command-1", OperationID: "operation-1", Cycle: 1,
			})
			if _, err := assembleAndCommitModelContextForTest(conversation, "user input", "user input"); err != nil {
				t.Fatal(err)
			}
			if err := conversation.AppendAssistant("must not commit"); err != nil {
				t.Fatal(err)
			}
			if err := conversation.CommitAgentCycle(context.Background(), agentrun.Outcome{Status: status}); err != nil {
				t.Fatal(err)
			}
			if got := sess.MessageCountTotal(); got != 1 {
				t.Fatalf("message count = %d, aborted/failed assistant was committed", got)
			}
			if _, ok := conversation.LastAgentCycleCommitReceipt(agentrun.DomainCommitOutput); ok {
				t.Fatal("aborted/failed cycle exposed a commit receipt")
			}
		})
	}
}
