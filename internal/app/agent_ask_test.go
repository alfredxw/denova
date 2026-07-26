package app

import (
	"context"
	"errors"
	"testing"
	"time"

	agents "denova/internal/agents"
	"denova/internal/agents/session"
)

func TestResolveSessionAskResumesExactIDEWaiter(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.Create("ask-session")
	if err != nil {
		t.Fatal(err)
	}
	chat := agents.NewEphemeralChatService()
	t.Cleanup(func() { _ = chat.Close(context.Background()) })
	application := &App{workspace: "/book", sessionStore: store, session: sess, chatService: chat}
	done := startAppAskWaiter(t, sess, "ask-ide", "ide")
	if view := application.WritingAgentActiveView(context.Background()); view.PendingAsk == nil || view.PendingAsk.ID != "ask-ide" {
		t.Fatalf("Writing active projection omitted pending Ask: %#v", view)
	}

	resolution, err := application.AnswerSessionAsk(context.Background(), sess.ID, "ask-ide", []AgentAskAnswer{{
		QuestionID: "choice", SelectedOptionIDs: []string{"a"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Status != session.AskAnswered {
		t.Fatalf("resolution = %#v", resolution)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestResolveConfigManagerAskCannotCrossScope(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	chat := agents.NewEphemeralChatService()
	t.Cleanup(func() { _ = chat.Close(context.Background()) })
	application := &App{workspace: "/book", sessionStore: store, chatService: chat}
	scope := ConfigManagerRequest{Origin: "settings", ResourceID: "agent-a"}
	sessionID, err := configManagerSessionID(scope)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	done := startAppAskWaiter(t, sess, "ask-config", "config_manager")
	if view := application.ConfigManagerAgentActiveView(context.Background(), scope); view.PendingAsk == nil || view.PendingAsk.ID != "ask-config" {
		t.Fatalf("Config Manager active projection omitted pending Ask: %#v", view)
	}

	otherScope := ConfigManagerRequest{Origin: "settings", ResourceID: "agent-b"}
	if _, err := application.CancelConfigManagerAsk(context.Background(), otherScope, "ask-config", "user_cancelled"); err == nil {
		t.Fatal("another Config Manager scope resolved the pending Ask")
	}
	if _, err := application.CancelConfigManagerAsk(context.Background(), scope, "ask-config", "user_cancelled"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestAnswerSessionAskReturnsCanonicalCancellationForColdOrphan(t *testing.T) {
	store, sess := reopenAppAskWithoutWaiter(t, "ask-cold-app", session.AskCycleIdentity{
		CommandID: "command-cold-app", OperationID: "operation-cold-app", Cycle: 1,
	})
	application := &App{workspace: "/book", sessionStore: store, session: sess}
	resolution, err := application.AnswerSessionAsk(context.Background(), sess.ID, "ask-cold-app", []AgentAskAnswer{{
		QuestionID: "choice", SelectedOptionIDs: []string{"a"},
	}})
	if err != nil || resolution.Status != session.AskCancelled || resolution.CancelReason != "runtime_continuation_unavailable" {
		t.Fatalf("cold AnswerSessionAsk resolution = %#v error=%v", resolution, err)
	}
	if pending := sess.PendingAsk(""); pending != nil {
		t.Fatalf("cold host answer left pending Ask: %#v", pending)
	}
	history := sess.History()
	if len(history) != 1 || history[0].Ask == nil || history[0].Ask.Status != session.AskCancelled ||
		history[0].Ask.CancelReason != "runtime_continuation_unavailable" {
		t.Fatalf("cold host answer history = %#v", history)
	}
}

func TestResolveAgentAskReturnsCanonicalTerminalAndNotFound(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.Create("ask-terminal")
	if err != nil {
		t.Fatal(err)
	}
	application := &App{workspace: "/book", sessionStore: store, session: sess}
	done := startAppAskWaiter(t, sess, "ask-terminal", "ide")
	want, err := application.CancelSessionAsk(context.Background(), sess.ID, "ask-terminal", "user_cancelled")
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got, err := application.AnswerSessionAsk(context.Background(), sess.ID, "ask-terminal", []AgentAskAnswer{{
		QuestionID: "choice", SelectedOptionIDs: []string{"a"},
	}})
	if err != nil || got.Status != want.Status || got.CancelReason != want.CancelReason {
		t.Fatalf("terminal replay = %#v error=%v, want %#v", got, err, want)
	}
	if _, err := application.CancelSessionAsk(context.Background(), sess.ID, "missing", "user_cancelled"); !errors.Is(err, ErrAgentAskNotFound) {
		t.Fatalf("unknown Ask error = %v, want %v", err, ErrAgentAskNotFound)
	}
}

func TestActiveViewsHideColdPendingAskWhenRuntimeProjectionUnavailable(t *testing.T) {
	t.Run("writing", func(t *testing.T) {
		store, sess := reopenAppAskWithoutWaiter(t, "ask-cold-writing-view", session.AskCycleIdentity{
			CommandID: "command-writing", OperationID: "operation-writing", Cycle: 1,
		})
		application := &App{
			workspace: "/book", sessionStore: store, session: sess,
			chatService: &agents.ChatService{},
		}
		view := application.WritingAgentActiveView(context.Background())
		if view.RuntimeProjectionOK || view.PendingAsk != nil {
			t.Fatalf("cold writing active view = %#v", view)
		}
		if pending := sess.PendingAsk(""); pending == nil || pending.Status != session.AskPending {
			t.Fatalf("unavailable projection unexpectedly changed durable Ask: %#v", pending)
		}
	})

	t.Run("config_manager", func(t *testing.T) {
		scope := ConfigManagerRequest{Origin: "settings", ResourceID: "agent-cold"}
		sessionID, err := configManagerSessionID(scope)
		if err != nil {
			t.Fatal(err)
		}
		store, sess := reopenAppAskWithoutWaiterForSession(t, sessionID, "ask-cold-config-view", "config_manager", session.AskCycleIdentity{
			CommandID: "command-config", OperationID: "operation-config", Cycle: 1,
		})
		application := &App{
			workspace: "/book", sessionStore: store,
			chatService: &agents.ChatService{},
		}
		view := application.ConfigManagerAgentActiveView(context.Background(), scope)
		if view.RuntimeProjectionOK || view.PendingAsk != nil {
			t.Fatalf("cold Config Manager active view = %#v", view)
		}
		if pending := sess.PendingAsk(""); pending == nil || pending.Status != session.AskPending {
			t.Fatalf("unavailable projection unexpectedly changed durable Ask: %#v", pending)
		}
	})
}

func TestColdRuntimeAskReconciliationRequiresExactCycle(t *testing.T) {
	_, sess := reopenAppAskWithoutWaiter(t, "ask-cold-cycle", session.AskCycleIdentity{
		CommandID: "command-cycle", OperationID: "operation-cycle", Cycle: 2,
	})
	notCold := agents.RuntimeStatus{
		Phase: agents.RunPhaseRunning, ActiveCommandID: "command-cycle", ActiveOperation: "operation-cycle", ActiveCycle: 2,
	}
	if reconciled, err := reconcileColdPendingAsk(context.Background(), sess, notCold); err != nil || reconciled {
		t.Fatalf("healthy runtime reconciliation = reconciled:%t err:%v", reconciled, err)
	}
	wrongCycle := notCold
	wrongCycle.RecoveryPaused = true
	wrongCycle.ActiveCycle = 3
	if reconciled, err := reconcileColdPendingAsk(context.Background(), sess, wrongCycle); err != nil || reconciled {
		t.Fatalf("mismatched runtime reconciliation = reconciled:%t err:%v", reconciled, err)
	}
	matching := wrongCycle
	matching.ActiveCycle = 2
	if reconciled, err := reconcileColdPendingAsk(context.Background(), sess, matching); err != nil || !reconciled {
		t.Fatalf("matching runtime reconciliation = reconciled:%t err:%v", reconciled, err)
	}
	if pending := sess.PendingAsk(""); pending != nil {
		t.Fatalf("matching runtime left pending Ask: %#v", pending)
	}
}

func startAppAskWaiter(t *testing.T, sess *session.Session, id, agentKind string) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := sess.AwaitAsk(context.Background(), session.AskInteraction{
			ID: id, ToolCallID: id, AgentKind: agentKind,
			Questions: []session.AskQuestion{{
				ID: "choice", Question: "Choose", Options: []session.AskOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
			}},
		})
		done <- err
	}()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if sess.PendingAsk(id) != nil {
			return done
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("Ask %q did not become pending", id)
	return done
}

func reopenAppAskWithoutWaiter(t *testing.T, id string, identity session.AskCycleIdentity) (*session.Store, *session.Session) {
	return reopenAppAskWithoutWaiterForSession(t, "ask-cold-session", id, "ide", identity)
}

func reopenAppAskWithoutWaiterForSession(t *testing.T, sessionID, id, agentKind string, identity session.AskCycleIdentity) (*session.Store, *session.Session) {
	t.Helper()
	directory := t.TempDir()
	store, err := session.NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, awaitErr := sess.AwaitAsk(ctx, session.AskInteraction{
			ID: id, ToolCallID: id, AgentKind: agentKind,
			AgentCommandID: identity.CommandID, AgentOperationID: identity.OperationID, AgentCycle: identity.Cycle,
			Questions: []session.AskQuestion{{
				ID: "choice", Question: "Choose", Options: []session.AskOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
			}},
		})
		done <- awaitErr
	}()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if sess.PendingAsk(id) != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if sess.PendingAsk(id) == nil {
		t.Fatalf("Ask %q did not become pending", id)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case awaitErr := <-done:
		if !errors.Is(awaitErr, context.Canceled) {
			t.Fatalf("closed waiter error = %v", awaitErr)
		}
	case <-time.After(time.Second):
		t.Fatal("closed Ask waiter did not stop")
	}
	reopenedStore, err := session.NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopenedStore.Close() })
	reopened, err := reopenedStore.Get(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	return reopenedStore, reopened
}
