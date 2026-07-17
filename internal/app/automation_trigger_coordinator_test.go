package app

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"denova/config"
)

func TestAutomationTriggerCoordinatorDoesNotLoseEnqueueDuringIdleExit(t *testing.T) {
	workspace := t.TempDir()
	coordinator := newAutomationTriggerCoordinator()
	releaseIdle := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(releaseIdle) })
		coordinator.Close()
	})

	detached := make(chan struct{})
	secondRun := make(chan struct{})
	var idleCalls atomic.Int32
	var runCalls atomic.Int32
	coordinator.afterIdleDetach = func(string) {
		if idleCalls.Add(1) == 1 {
			close(detached)
			<-releaseIdle
		}
	}
	coordinator.afterRun = func(string) {
		if runCalls.Add(1) == 2 {
			close(secondRun)
		}
	}
	service := &AutomationAppService{
		app: &App{},
		snapshot: &automationWorkspaceSnapshot{
			workspace: workspace,
			novaDir:   filepath.Join(workspace, "user"),
			cfg:       config.Config{Workspace: workspace, NovaDir: filepath.Join(workspace, "user")},
		},
	}
	if !coordinator.Enqueue(service, "first", []string{"chapters/one.md"}) {
		t.Fatal("first enqueue was rejected")
	}
	select {
	case <-detached:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("first worker did not reach the idle-detached barrier")
	}
	// The first worker has removed its entry but has not yet returned. The
	// enqueue must create a distinct worker that the first defer cannot erase.
	if !coordinator.Enqueue(service, "second", []string{"chapters/two.md"}) {
		t.Fatal("second enqueue was rejected")
	}
	select {
	case <-secondRun:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("second enqueue was lost; process calls=%d", runCalls.Load())
	}
	releaseOnce.Do(func() { close(releaseIdle) })
	coordinator.Close()
	if got := runCalls.Load(); got != 2 {
		t.Fatalf("process calls = %d, want 2", got)
	}
}
