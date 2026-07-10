package app

import (
	"context"
	"testing"
)

func TestWorkspaceDirectorTaskGroupCancelsAndWaits(t *testing.T) {
	tasks := newWorkspaceDirectorTaskGroup()
	started := make(chan struct{})
	finished := make(chan struct{})
	done, ok := tasks.Go(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(finished)
	})
	if !ok {
		t.Fatal("new workspace task group rejected its first task")
	}
	<-started
	tasks.Close()
	<-done
	<-finished

	if _, ok := tasks.Go(func(context.Context) {}); ok {
		t.Fatal("closed workspace task group accepted a new task")
	}
}
