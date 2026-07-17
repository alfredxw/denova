package app

import (
	"context"
	"fmt"
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

func TestWorkspaceDirectorTaskGroupSerializesSameBranch(t *testing.T) {
	tasks := newWorkspaceDirectorTaskGroup()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})

	firstDone, ok := tasks.GoKeyed("story-1:main", func(context.Context) {
		close(firstStarted)
		<-releaseFirst
	})
	if !ok {
		t.Fatal("first keyed task rejected")
	}
	<-firstStarted
	secondDone, ok := tasks.GoKeyed("story-1:main", func(context.Context) {
		close(secondStarted)
	})
	if !ok {
		t.Fatal("second keyed task rejected")
	}
	select {
	case <-secondStarted:
		t.Fatal("second task started before the first task completed")
	default:
	}
	close(releaseFirst)
	<-firstDone
	<-secondStarted
	<-secondDone
	tasks.Close()
}

func TestWorkspaceDirectorTaskGroupWaitKeyWaitsForQueuedWork(t *testing.T) {
	tasks := newWorkspaceDirectorTaskGroup()
	release := make(chan struct{})
	_, ok := tasks.GoKeyed("story-1:main", func(context.Context) { <-release })
	if !ok {
		t.Fatal("keyed task rejected")
	}
	waitDone := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				waitDone <- fmt.Errorf("WaitKey test goroutine panic: %v", recovered)
			}
		}()
		waitDone <- tasks.WaitKey(context.Background(), "story-1:main")
	}()
	select {
	case err := <-waitDone:
		t.Fatalf("WaitKey returned before queued work completed: %v", err)
	default:
	}
	close(release)
	if err := <-waitDone; err != nil {
		t.Fatal(err)
	}
	tasks.Close()
}

func TestInteractiveForegroundWaitsForSchemaLaneButNotDerivedMaintenance(t *testing.T) {
	tasks := newWorkspaceDirectorTaskGroup()
	conversation := &interactiveConversation{storyID: "story-1"}
	schemaKey := interactiveStateSchemaMaintenanceKey(conversation, "main")
	derivedKey := interactiveDerivedMaintenanceKey(conversation, "main")
	if schemaKey == derivedKey {
		t.Fatalf("maintenance lanes must be distinct: %s", schemaKey)
	}

	releaseSchema := make(chan struct{})
	releaseDerived := make(chan struct{})
	derivedStarted := make(chan struct{})
	_, _ = tasks.GoKeyed(schemaKey, func(context.Context) { <-releaseSchema })
	derivedDone, _ := tasks.GoKeyed(derivedKey, func(context.Context) {
		close(derivedStarted)
		<-releaseDerived
	})
	<-derivedStarted
	waitDone := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				waitDone <- fmt.Errorf("schema lane wait panic: %v", recovered)
			}
		}()
		waitDone <- tasks.WaitKey(context.Background(), schemaKey)
	}()
	select {
	case err := <-waitDone:
		t.Fatalf("schema wait returned before schema completion: %v", err)
	default:
	}
	close(releaseSchema)
	if err := <-waitDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-derivedDone:
		t.Fatal("schema wait should not require derived Director/Memory maintenance")
	default:
	}
	close(releaseDerived)
	<-derivedDone
	tasks.Close()
}
