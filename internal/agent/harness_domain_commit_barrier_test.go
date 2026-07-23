package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"denova/internal/agentruntime"
	"denova/internal/session"
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
	if _, err := harness.Submit(context.Background(), agentruntime.Abort{
		ID: "abort-before-output", OperationID: operationID, Reason: "user stopped",
	}); err != nil {
		t.Fatalf("submit abort before output intent: %v", err)
	}
	conversation.releaseIntent()
	outcome := waitRunControlOutcome(t, done)
	if outcome.Status != RunOutcomeAborted {
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
		if entry.Role != string(schema.User) || entry.AgentCommandID == "" {
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
	if pending, ok, err := conversation.SessionConversation.PendingAgentCycleCommit(HarnessDomainCommitOutput); err != nil || !ok {
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
		if commit.Identity.Stage == agentruntime.DomainCommitOutput {
			foundOutputIntent = true
		}
	}
	if !foundOutputIntent {
		t.Fatalf("output commit callback ran before actor authorization: %+v", observation.Snapshot.DomainCommits)
	}
	_, abortErr := harness.Submit(context.Background(), agentruntime.Abort{
		ID: "abort-after-output", OperationID: operationID, Reason: "too late",
	})
	if !errors.Is(abortErr, agentruntime.ErrDomainCommitRejected) {
		t.Fatalf("late abort error = %v, want %v", abortErr, agentruntime.ErrDomainCommitRejected)
	}
	conversation.releaseCommit()
	outcome := waitRunControlOutcome(t, done)
	if outcome.Status != RunOutcomeCompleted {
		t.Fatalf("outcome = %+v, want completed", outcome)
	}
	if got := sess.MessageCountTotal(); got != 2 {
		t.Fatalf("canonical messages = %d, want user and assistant", got)
	}
	if receipt, ok := conversation.LastAgentCycleCommitReceipt(HarnessDomainCommitOutput); !ok || receipt.Revision == "" {
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
	done := make(chan RunOutcome, 1)
	runner := newRunControlTestRunner(t, &runControlFixedModel{message: schema.AssistantMessage("canonical answer", nil)}, true)
	runOutcomeTestGoroutine(done, "domain commit barrier run", func() RunOutcome {
		return service.RunWithOptions(ctx, runner, conversation, nil, ChatRequest{CommandID: "domain-commit-barrier", Message: "user input"}, options, nil)
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
	if outcome.Status != RunOutcomeCompleted || outcome.Error != nil || outcome.Content != "canonical answer" {
		t.Fatalf("authorized completion must win over rejected caller abort: %+v", outcome)
	}
	if got := sess.MessageCountTotal(); got != 2 {
		t.Fatalf("canonical messages = %d, want user and assistant", got)
	}
}

type harnessBarrierConversation struct {
	*SessionConversation
	beforeOutputIntent  chan struct{}
	releaseOutputIntent chan struct{}
	outputAuthorized    chan struct{}
	releaseOutputCommit chan struct{}
	intentOnce          sync.Once
	commitOnce          sync.Once
	releaseIntentOnce   sync.Once
	releaseCommitOnce   sync.Once
}

func (c *harnessBarrierConversation) PendingAgentCycleCommit(stage HarnessDomainCommitStage) (HarnessDomainCommitIntent, bool, error) {
	if stage == HarnessDomainCommitOutput && c.beforeOutputIntent != nil {
		c.intentOnce.Do(func() { close(c.beforeOutputIntent) })
		<-c.releaseOutputIntent
	}
	return c.SessionConversation.PendingAgentCycleCommit(stage)
}

func (c *harnessBarrierConversation) CommitAgentCycleStage(ctx context.Context, stage HarnessDomainCommitStage, outcome RunOutcome) error {
	if stage == HarnessDomainCommitOutput && runOutcomeMayCommitDomain(outcome) && c.outputAuthorized != nil {
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
	return &harnessBarrierConversation{SessionConversation: NewSessionConversation(sess)}, sess
}

func newHarnessBarrierChatService(t *testing.T) *ChatService {
	t.Helper()
	service, err := newHarnessChatService(context.Background(), DefaultLoopPolicy(), agentruntime.NewMemoryJournalStore())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	return service
}

func harnessBarrierRunOptions(sessionID string) RunOptions {
	return RunOptions{AgentKind: AgentKindIDE, RootAgentName: "run-control-test", Workspace: "barrier-workspace", SessionID: sessionID}
}

func runHarnessBarrierChat(t *testing.T, service *ChatService, conversation Conversation, options RunOptions) <-chan RunOutcome {
	t.Helper()
	done := make(chan RunOutcome, 1)
	runner := newRunControlTestRunner(t, &runControlFixedModel{message: schema.AssistantMessage("canonical answer", nil)}, true)
	runOutcomeTestGoroutine(done, "domain commit race run", func() RunOutcome {
		return service.RunWithOptions(
			context.Background(),
			runner,
			conversation, nil, ChatRequest{CommandID: "domain-commit-race", Message: "user input"}, options, nil,
		)
	})
	return done
}

func openHarnessBarrierBinding(t *testing.T, service *ChatService, options RunOptions) *agentruntime.Harness {
	t.Helper()
	binding, err := harnessBindingForOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	harness, err := service.harness.runtime.Open(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	return harness
}

func activeHarnessBarrierOperation(t *testing.T, harness *agentruntime.Harness) agentruntime.OperationID {
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
