package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"denova/internal/agents/session"
)

func TestResolveAgentChatAskReturnsCanonicalTerminalAfterRunReleased(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.GetOrCreate("agent-chat-terminal")
	if err != nil {
		t.Fatal(err)
	}
	service, binding := newAgentChatAskTestService(t, store, sess.ID)
	run := &agentChatRun{binding: binding, runtime: ideChatRuntime{sess: sess}}
	service.active[agentChatBindingKey(binding)] = run

	done := startAppAskWaiter(t, sess, "ask-agent-chat-terminal", "ide")
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
	service.releaseActiveRun(run)

	got, err := service.resolveAsk(
		context.Background(), binding, "ask-agent-chat-terminal",
		session.AskAnswered, []AgentAskAnswer{{QuestionID: "choice", SelectedOptionIDs: []string{"a"}}}, "",
	)
	if err != nil {
		t.Fatalf("terminal Ask replay after run release failed: %v", err)
	}
	if got.Status != want.Status || got.CancelReason != want.CancelReason {
		t.Fatalf("terminal Ask replay = %#v, want %#v", got, want)
	}
}

func TestResolveAgentChatAskCancelsColdPendingFromProjectStore(t *testing.T) {
	store, sess := reopenAppAskWithoutWaiterForSession(t, "agent-chat-cold", "ask-agent-chat-cold", "ide", session.AskCycleIdentity{
		CommandID: "command-agent-chat-cold", OperationID: "operation-agent-chat-cold", Cycle: 1,
	})
	service, binding := newAgentChatAskTestService(t, store, sess.ID)

	resolution, err := service.resolveAsk(
		context.Background(), binding, "ask-agent-chat-cold",
		session.AskAnswered, []AgentAskAnswer{{QuestionID: "choice", SelectedOptionIDs: []string{"a"}}}, "",
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

func TestResolveAgentChatAskReturnsNotFoundForUnknownID(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.GetOrCreate("agent-chat-unknown")
	if err != nil {
		t.Fatal(err)
	}
	service, binding := newAgentChatAskTestService(t, store, sess.ID)

	_, err = service.resolveAsk(
		context.Background(), binding, "missing-ask",
		session.AskCancelled, nil, "user_cancelled",
	)
	if !errors.Is(err, ErrAgentAskNotFound) {
		t.Fatalf("unknown AgentChat Ask error = %v, want %v", err, ErrAgentAskNotFound)
	}
}

func newAgentChatAskTestService(t *testing.T, store *session.Store, sessionID string) (*AgentChatAppService, AgentChatBinding) {
	t.Helper()
	denovaDir := t.TempDir()
	workspace := filepath.Join(denovaDir, bookProjectsDirName, "agent-chat-ask-book")
	if err := os.MkdirAll(filepath.Join(workspace, "setting"), 0o755); err != nil {
		t.Fatal(err)
	}
	registry := NewBookRegistry(denovaDir)
	if err := registry.Touch(workspace); err != nil {
		t.Fatal(err)
	}
	application := &App{
		bookRegistry:  registry,
		bookMetaStore: NewBookMetaStore(denovaDir),
	}
	application.ensureServices()
	canonical, err := canonicalWorkspacePath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	service := application.agentChatApp
	service.projects[canonical] = &agentChatProjectRuntime{workspace: canonical, store: store}
	return service, AgentChatBinding{Workspace: canonical, SessionID: sessionID}
}
