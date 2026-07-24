package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"denova/config"
	"denova/internal/agents/session"
	"denova/internal/book"
)

func TestCreateVersionCancellationDoesNotCommitFallbackOrAgentJournal(t *testing.T) {
	workspace := t.TempDir()
	bookService := book.NewService(workspace)
	if err := bookService.Create("draft.md", "file", "cancelled draft"); err != nil {
		t.Fatal(err)
	}
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	application := &App{
		cfg: &config.Config{Workspace: workspace, NovaDir: t.TempDir()}, workspace: workspace,
		bookService: bookService, versionService: book.NewVersionService(workspace), sessionStore: store,
	}
	started := make(chan struct{})
	restoreGenerator := application.setVersionSummaryGeneratorForTest(func(ctx context.Context, _ *config.Config, _ string) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})
	defer restoreGenerator()

	done := make(chan error, 1)
	runAppErrorTestGoroutine(done, "cancelled version creation", func() error {
		_, createErr := application.CreateVersion(context.Background(), "")
		return createErr
	})
	<-started
	_, scopes, _, err := application.beginWorkspaceTransitionTo(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := waitLifecycleScopes(context.Background(), scopes); err != nil {
		t.Fatal(err)
	}
	if createErr := <-done; !errors.Is(createErr, context.Canceled) {
		t.Fatalf("CreateVersion cancellation error = %v", createErr)
	}
	application.endWorkspaceTransition()
	defer application.Close()

	history, err := application.versionService.History(10)
	if err != nil || len(history) != 0 {
		t.Fatalf("cancelled version history = %#v err=%v", history, err)
	}
	journals, err := store.List("")
	if err != nil || len(journals) != 0 {
		t.Fatalf("cancelled summary journal = %#v err=%v", journals, err)
	}
}

func TestCreateVersionSummaryUsesCapturedWorkspaceAndSessionStore(t *testing.T) {
	workspaceA := t.TempDir()
	workspaceB := t.TempDir()
	bookA := book.NewService(workspaceA)
	if err := bookA.Create("draft.md", "file", "workspace A draft"); err != nil {
		t.Fatal(err)
	}
	storeA, err := session.NewStore(filepath.Join(t.TempDir(), "a"))
	if err != nil {
		t.Fatal(err)
	}
	storeB, err := session.NewStore(filepath.Join(t.TempDir(), "b"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Workspace: workspaceA, NovaDir: t.TempDir()}
	versionA := book.NewVersionService(workspaceA)
	application := &App{
		cfg: cfg, workspace: workspaceA, bookService: bookA,
		versionService: versionA, sessionStore: storeA,
	}
	defer application.Close()

	started := make(chan string, 1)
	release := make(chan struct{})
	restoreGenerator := application.setVersionSummaryGeneratorForTest(func(_ context.Context, runtimeCfg *config.Config, _ string) (string, error) {
		started <- runtimeCfg.Workspace
		<-release
		return "完善第一章冲突推进", nil
	})
	defer restoreGenerator()

	done := make(chan error, 1)
	runAppErrorTestGoroutine(done, "captured workspace version creation", func() error {
		_, createErr := application.CreateVersion(context.Background(), "")
		return createErr
	})
	if got := <-started; lifecycleWorkspaceKey(got) != lifecycleWorkspaceKey(workspaceA) {
		t.Fatalf("summary config workspace=%q want=%q", got, workspaceA)
	}
	application.mu.Lock()
	application.workspace = workspaceB
	application.bookService = book.NewService(workspaceB)
	application.versionService = book.NewVersionService(workspaceB)
	application.sessionStore = storeB
	application.mu.Unlock()
	close(release)
	if createErr := <-done; !errors.Is(createErr, ErrWorkspaceChanged) {
		t.Fatalf("CreateVersion drift error = %v", createErr)
	}

	journalA, err := storeA.List("")
	if err != nil || len(journalA) != 1 {
		t.Fatalf("captured workspace journal = %#v err=%v", journalA, err)
	}
	journalB, err := storeB.List("")
	if err != nil || len(journalB) != 0 {
		t.Fatalf("new workspace received stale summary journal = %#v err=%v", journalB, err)
	}
	historyA, err := versionA.History(10)
	if err != nil || len(historyA) != 0 {
		t.Fatalf("identity CAS still created a stale version: %#v err=%v", historyA, err)
	}
}
