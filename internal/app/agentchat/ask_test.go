package agentchat

import (
	"context"
	"errors"
	"testing"
	"time"

	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/session"
	conversationapp "denova/internal/app/conversation"
)

func TestResolveAskReturnsCanonicalTerminalAfterRunReleased(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.GetOrCreate("agent-chat-terminal")
	if err != nil {
		t.Fatal(err)
	}
	service, binding, _ := newInjectedService(t, store, sess.ID)
	active := &run{binding: binding, runtime: conversationapp.Runtime{Session: sess}}
	service.active[bindingKey(binding)] = active

	done := startAskWaiter(t, sess, "ask-agent-chat-terminal")
	want, err := service.resolveAsk(
		context.Background(), binding, "ask-agent-chat-terminal",
		session.AskCancelled, nil, "user_cancelled",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	service.releaseActiveRun(active)

	got, err := service.resolveAsk(
		context.Background(), binding, "ask-agent-chat-terminal",
		session.AskAnswered, []agentconversation.HostAskAnswer{{QuestionID: "choice", SelectedOptionIDs: []string{"a"}}}, "",
	)
	if err != nil {
		t.Fatalf("terminal Ask replay after run release failed: %v", err)
	}
	if got.Status != want.Status || got.CancelReason != want.CancelReason {
		t.Fatalf("terminal Ask replay = %#v, want %#v", got, want)
	}
}

func TestResolveAskCancelsColdPendingFromProjectStore(t *testing.T) {
	store, sess := reopenAskWithoutWaiter(t, "agent-chat-cold", "ask-agent-chat-cold")
	service, binding, _ := newInjectedService(t, store, sess.ID)

	resolution, err := service.resolveAsk(
		context.Background(), binding, "ask-agent-chat-cold",
		session.AskAnswered, []agentconversation.HostAskAnswer{{QuestionID: "choice", SelectedOptionIDs: []string{"a"}}}, "",
	)
	if err != nil {
		t.Fatalf("cold AgentChat Ask resolution failed: %v", err)
	}
	if resolution.Status != session.AskCancelled || resolution.CancelReason != "runtime_continuation_unavailable" {
		t.Fatalf("cold AgentChat Ask resolution = %#v", resolution)
	}
	if pending := sess.PendingAsk(""); pending != nil {
		t.Fatalf("cold AgentChat Ask remained pending: %#v", pending)
	}
}

func TestResolveAskReturnsNotFoundForUnknownID(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.GetOrCreate("agent-chat-unknown")
	if err != nil {
		t.Fatal(err)
	}
	service, binding, _ := newInjectedService(t, store, sess.ID)

	_, err = service.resolveAsk(context.Background(), binding, "missing-ask", session.AskCancelled, nil, "user_cancelled")
	if !errors.Is(err, agentconversation.ErrAskNotFound) {
		t.Fatalf("unknown AgentChat Ask error = %v, want %v", err, agentconversation.ErrAskNotFound)
	}
}

func startAskWaiter(t *testing.T, sess *session.Session, id string) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, err := sess.AwaitAsk(context.Background(), askInteraction(id))
		done <- err
	}()
	waitForPendingAsk(t, sess, id)
	return done
}

func reopenAskWithoutWaiter(t *testing.T, sessionID, id string) (*session.Store, *session.Session) {
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
	interaction := askInteraction(id)
	interaction.AgentCommandID = "command-agent-chat-cold"
	interaction.AgentOperationID = "operation-agent-chat-cold"
	interaction.AgentCycle = 1
	go func() {
		_, awaitErr := sess.AwaitAsk(ctx, interaction)
		done <- awaitErr
	}()
	waitForPendingAsk(t, sess, id)
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

func askInteraction(id string) session.AskInteraction {
	return session.AskInteraction{
		ID: id, ToolCallID: id, AgentKind: "ide",
		Questions: []session.AskQuestion{{
			ID: "choice", Question: "Choose",
			Options: []session.AskOption{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
		}},
	}
}

func waitForPendingAsk(t *testing.T, sess *session.Session, id string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if sess.PendingAsk(id) != nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("Ask %q did not become pending", id)
}
