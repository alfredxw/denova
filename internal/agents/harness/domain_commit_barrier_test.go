package harness

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"errors"
	"sync"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/agents/session"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestHarnessDomainCommitBarrierAbortBeforeOutputIntentKeepsUserOnly(t *testing.T) {
	conversation, sess := newHarnessBarrierSessionConversation(t)
	conversation.beforeOutputIntent = make(chan struct{})
	conversation.releaseOutputIntent = make(chan struct{})
	defer conversation.releaseIntent()
	service := newHarnessBarrierChatService(t)
	options := harnessBarrierRunOptions(sess.ID)
	done := runHarnessBarrierChat(t, service, conversation, options)

	waitHarnessEngineSignal(t, conversation.beforeOutputIntent, "pending output intent")
	harness := openHarnessBarrierBinding(t, service, options)
	operationID := activeHarnessBarrierOperation(t, harness)
	if _, err := harness.Submit(context.Background(), runstate.Abort{
		ID: "abort-before-output", OperationID: operationID, Reason: "user stopped",
	}); err != nil {
		t.Fatalf("submit abort before output intent: %v", err)
	}
	conversation.releaseIntent()
	outcome := waitRunControlOutcome(t, done)
	if outcome.Status != agentrun.OutcomeAborted {
		t.Fatalf("outcome = %+v, want aborted", outcome)
	}
	if got := sess.MessageCountTotal(); got != 1 {
		t.Fatalf("canonical messages = %d, want accepted user only", got)
	}
	history := sess.History()
	canonicalMessages := 0
	for _, entry := range history {
		if entry.Message == nil {
			continue
		}
		canonicalMessages++
		if entry.Role != string(agent.User) || entry.AgentCommandID == "" {
			t.Fatalf("unexpected canonical message after abort: %+v", entry)
		}
	}
	if canonicalMessages != 1 {
		t.Fatalf("canonical history count = %d, history=%+v", canonicalMessages, history)
	}
}

func TestHarnessDomainCommitBarrierOutputIntentBeforeAbortCommitsAndWins(t *testing.T) {
	conversation, sess := newHarnessBarrierSessionConversation(t)
	conversation.outputAuthorized = make(chan struct{})
	conversation.releaseOutputCommit = make(chan struct{})
	defer conversation.releaseCommit()
	service := newHarnessBarrierChatService(t)
	options := harnessBarrierRunOptions(sess.ID)
	done := runHarnessBarrierChat(t, service, conversation, options)

	waitHarnessEngineSignal(t, conversation.outputAuthorized, "authorized output intent")
	if pending, ok, err := conversation.SessionConversation.PendingAgentCycleCommit(agentrun.DomainCommitOutput); err != nil || !ok {
		t.Fatalf("assistant output was not staged before commit callback: pending=%+v ok=%t err=%v", pending, ok, err)
	}
	harness := openHarnessBarrierBinding(t, service, options)
	operationID := activeHarnessBarrierOperation(t, harness)
	observation, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	foundOutputIntent := false
	for _, commit := range observation.Snapshot.DomainCommits {
		if commit.Identity.Stage == runstate.DomainCommitOutput {
			foundOutputIntent = true
		}
	}
	if !foundOutputIntent {
		t.Fatalf("output commit callback ran before actor authorization: %+v", observation.Snapshot.DomainCommits)
	}
	_, abortErr := harness.Submit(context.Background(), runstate.Abort{
		ID: "abort-after-output", OperationID: operationID, Reason: "too late",
	})
	if !errors.Is(abortErr, runstate.ErrDomainCommitRejected) {
		t.Fatalf("late abort error = %v, want %v", abortErr, runstate.ErrDomainCommitRejected)
	}
	conversation.releaseCommit()
	outcome := waitRunControlOutcome(t, done)
	if outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("outcome = %+v, want completed", outcome)
	}
	if got := sess.MessageCountTotal(); got != 2 {
		t.Fatalf("canonical messages = %d, want user and assistant", got)
	}
	if receipt, ok := conversation.LastAgentCycleCommitReceipt(agentrun.DomainCommitOutput); !ok || receipt.Revision == "" {
		t.Fatalf("missing canonical output receipt: %+v ok=%t", receipt, ok)
	}
}

func TestHarnessCallerCancelWaitsWhenOutputCommitAlreadyFinalizing(t *testing.T) {
	conversation, sess := newHarnessBarrierSessionConversation(t)
	conversation.outputAuthorized = make(chan struct{})
	conversation.releaseOutputCommit = make(chan struct{})
	defer conversation.releaseCommit()
	service := newHarnessBarrierChatService(t)
	options := harnessBarrierRunOptions(sess.ID)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan agentrun.Outcome, 1)
	runner := newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("canonical answer", nil)}, true)
	runOutcomeTestGoroutine(done, "domain commit barrier run", func() agentrun.Outcome {
		return service.RunWithOptions(ctx, runner, conversation, nil, agentchat.ChatRequest{CommandID: "domain-commit-barrier", Message: "user input"}, options, nil)
	})

	waitHarnessEngineSignal(t, conversation.outputAuthorized, "authorized output intent")
	cancel()
	select {
	case outcome := <-done:
		t.Fatalf("caller returned before the authorized domain commit settled: %+v", outcome)
	case <-time.After(25 * time.Millisecond):
	}
	conversation.releaseCommit()
	outcome := waitRunControlOutcome(t, done)
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Error != nil || outcome.Content != "canonical answer" {
		t.Fatalf("authorized completion must win over rejected caller abort: %+v", outcome)
	}
	if got := sess.MessageCountTotal(); got != 2 {
		t.Fatalf("canonical messages = %d, want user and assistant", got)
	}
}

type harnessBarrierConversation struct {
	*agentconversation.SessionConversation
	beforeOutputIntent  chan struct{}
	releaseOutputIntent chan struct{}
	outputAuthorized    chan struct{}
	releaseOutputCommit chan struct{}
	intentOnce          sync.Once
	commitOnce          sync.Once
	releaseIntentOnce   sync.Once
	releaseCommitOnce   sync.Once
}

func (c *harnessBarrierConversation) PendingAgentCycleCommit(stage agentrun.DomainCommitStage) (agentrun.DomainCommitIntent, bool, error) {
	if stage == agentrun.DomainCommitOutput && c.beforeOutputIntent != nil {
		c.intentOnce.Do(func() { close(c.beforeOutputIntent) })
		<-c.releaseOutputIntent
	}
	return c.SessionConversation.PendingAgentCycleCommit(stage)
}

func (c *harnessBarrierConversation) CommitAgentCycleStage(ctx context.Context, stage agentrun.DomainCommitStage, outcome agentrun.Outcome) error {
	if stage == agentrun.DomainCommitOutput && agentrun.OutcomeMayCommitDomain(outcome) && c.outputAuthorized != nil {
		c.commitOnce.Do(func() { close(c.outputAuthorized) })
		<-c.releaseOutputCommit
	}
	return c.SessionConversation.CommitAgentCycleStage(ctx, stage, outcome)
}

func (c *harnessBarrierConversation) releaseIntent() {
	if c.releaseOutputIntent != nil {
		c.releaseIntentOnce.Do(func() { close(c.releaseOutputIntent) })
	}
}

func (c *harnessBarrierConversation) releaseCommit() {
	if c.releaseOutputCommit != nil {
		c.releaseCommitOnce.Do(func() { close(c.releaseOutputCommit) })
	}
}

func newHarnessBarrierSessionConversation(t *testing.T) (*harnessBarrierConversation, *session.Session) {
	t.Helper()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("barrier-session")
	if err != nil {
		t.Fatal(err)
	}
	return &harnessBarrierConversation{SessionConversation: agentconversation.NewSessionConversation(sess)}, sess
}

func newHarnessBarrierChatService(t *testing.T) *Service {
	t.Helper()
	service, err := newService(context.Background(), agentrun.DefaultLoopPolicy(), runstate.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	return service
}

func harnessBarrierRunOptions(sessionID string) agentrun.Options {
	return agentrun.Options{AgentKind: agentrun.AgentKindIDE, RootAgentName: "run-control-test", Workspace: "barrier-workspace", SessionID: sessionID}
}

func runHarnessBarrierChat(t *testing.T, service *Service, conversation agentchat.Conversation, options agentrun.Options) <-chan agentrun.Outcome {
	t.Helper()
	done := make(chan agentrun.Outcome, 1)
	runner := newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("canonical answer", nil)}, true)
	runOutcomeTestGoroutine(done, "domain commit race run", func() agentrun.Outcome {
		return service.RunWithOptions(
			context.Background(),
			runner,
			conversation, nil, agentchat.ChatRequest{CommandID: "domain-commit-race", Message: "user input"}, options, nil,
		)
	})
	return done
}

func openHarnessBarrierBinding(t *testing.T, service *Service, options agentrun.Options) *runstate.Harness {
	t.Helper()
	binding, err := agentrun.BindingForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	harness, err := service.coordinator.runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	return harness
}

func activeHarnessBarrierOperation(t *testing.T, harness *runstate.Harness) runstate.OperationID {
	t.Helper()
	observation, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observation.Snapshot.ActiveOperation == "" {
		t.Fatal("harness has no active operation")
	}
	return observation.Snapshot.ActiveOperation
}
