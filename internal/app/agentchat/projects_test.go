package agentchat

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	appagentruntime "denova/internal/app/agentruntime"
	apptask "denova/internal/app/task"
	projectdomain "denova/internal/project"
)

func TestVisibleProjectSessionsKeepsRunningOutsideRecentWindow(t *testing.T) {
	metas := make([]session.SessionMeta, ProjectSessionsLimit+2)
	for index := range metas {
		metas[index] = session.SessionMeta{ID: fmt.Sprintf("session-%d", index), UpdatedAt: time.Unix(int64(index), 0)}
	}
	projectID := "project-alpha"
	detached := metas[len(metas)-1].ID
	running := map[string]struct{}{
		bindingKey(Binding{ProjectID: projectID, SessionID: detached}): {},
	}

	visible := visibleProjectSessions(metas, projectID, running)
	if len(visible) != ProjectSessionsLimit+1 {
		t.Fatalf("visible session count = %d, want %d", len(visible), ProjectSessionsLimit+1)
	}
	if visible[len(visible)-1].ID != detached {
		t.Fatalf("last visible session = %q, want detached running session %q", visible[len(visible)-1].ID, detached)
	}
}

func TestCloseProjectRejectsRunningConversation(t *testing.T) {
	service, binding, _ := newInjectedService(t, nil, "close-project-session")
	task := apptask.New(func(ctx context.Context, _ *apptask.Task, _ func(agentrun.Event)) {
		<-ctx.Done()
	})
	t.Cleanup(func() {
		task.Abort()
		<-task.Done()
	})
	service.active[bindingKey(binding)] = &run{binding: binding, task: task}

	if err := service.CloseProject(context.Background(), binding.ProjectID); !errors.Is(err, appagentruntime.ErrOperationActive) {
		t.Fatalf("CloseProject error = %v, want ErrOperationActive", err)
	}
}

func TestTurnBusyPolicyKeepsInteractiveAdmissionImmediateAndBackgroundWaitCancellable(t *testing.T) {
	service, binding, _ := newInjectedService(t, nil, "busy-policy-session")
	blocker, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	service.active[bindingKey(binding)] = &run{binding: binding, task: blocker}
	t.Cleanup(func() { blocker.RejectStart(context.Canceled) })

	request := TurnRequest{
		Binding:     binding,
		ChatRequest: ChatRequest{CommandID: "busy-policy-turn", Message: "continue"},
	}
	if _, err := service.AcceptTurn(context.Background(), request); !errors.Is(err, appagentruntime.ErrOperationActive) {
		t.Fatalf("interactive busy admission error = %v, want ErrOperationActive", err)
	}

	waitCtx, cancel := context.WithCancel(context.Background())
	cancel()
	request.Policy.BusyPolicy = TurnBusyWait
	if _, err := service.AcceptTurn(waitCtx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("background busy admission error = %v, want context cancellation", err)
	}
}

func newInjectedService(t *testing.T, store *session.Store, sessionID string) (*Service, Binding, *session.Store) {
	t.Helper()
	denovaDir := t.TempDir()
	workspace := filepath.Join(denovaDir, projectdomain.ContentDirectoryName, "agent-chat-test-book")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := projectdomain.NewRegistry(denovaDir)
	record, err := registry.EnsureBook(workspace)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.EnsureState(record)
	if err != nil {
		t.Fatal(err)
	}
	if store == nil {
		store, err = session.NewStore(layout.SessionsDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
	}
	service := NewService(nil, registry)
	service.projects[record.ID] = &projectRuntime{
		projectID: record.ID, projectType: record.Type, agentKind: agentrun.AgentKindIDE,
		stateRoot: layout.StateRoot, workspace: layout.ContentRoot, store: store,
	}
	return service, Binding{
		ProjectID: record.ID, Workspace: layout.ContentRoot, SessionID: sessionID,
		agentKind: agentrun.AgentKindIDE, stateRoot: layout.StateRoot,
	}, store
}
