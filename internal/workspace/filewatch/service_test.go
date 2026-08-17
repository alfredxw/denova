package filewatch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServiceObservesRecursiveAndAtomicWorkspaceChanges(t *testing.T) {
	root := t.TempDir()
	chapters := filepath.Join(root, "chapters")
	if err := os.MkdirAll(chapters, 0o755); err != nil {
		t.Fatal(err)
	}
	chapter := filepath.Join(chapters, "ch01.md")
	if err := os.WriteFile(chapter, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}

	service := NewService()
	t.Cleanup(service.Close)
	events, unsubscribe, err := service.Subscribe("project-one", root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unsubscribe)
	initial := <-events
	if !initial.Resync || initial.ProjectID != "project-one" || initial.Workspace != root {
		t.Fatalf("initial event = %#v", initial)
	}

	deadline := time.After(800 * time.Millisecond)
	waitFor := func(path string, changeType ChangeType) Event {
		t.Helper()
		for {
			select {
			case event := <-events:
				for _, change := range event.Changes {
					if change.Path == path && change.Type == changeType {
						return event
					}
				}
			case <-deadline:
				t.Fatalf("timed out waiting for %s %s", changeType, path)
			}
		}
	}

	if err := os.WriteFile(chapter, []byte("after"), 0o644); err != nil {
		t.Fatal(err)
	}
	updated := waitFor("chapters/ch01.md", ChangeUpdated)
	if updated.Source != eventSource || len(updated.Paths) == 0 {
		t.Fatalf("updated event missing normalized metadata: %#v", updated)
	}

	volume := filepath.Join(chapters, "volume-1")
	if err := os.MkdirAll(volume, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(volume, "ch02.md"), []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor("chapters/volume-1/ch02.md", ChangeAdded)

	temporary := filepath.Join(chapters, ".ch01.denova-test.tmp")
	if err := os.WriteFile(temporary, []byte("atomic"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporary, chapter); err != nil {
		t.Fatal(err)
	}
	waitFor("chapters/ch01.md", ChangeUpdated)
}

func TestServiceReplacesLaggingSubscriberSuffixWithResync(t *testing.T) {
	service := NewService()
	t.Cleanup(service.Close)
	events, unsubscribe, err := service.Subscribe("project-demo", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unsubscribe)
	<-events

	service.mu.Lock()
	entry := service.projects["project-demo"]
	for index := 0; index < 9; index++ {
		broadcastProjectEvent(entry, Event{
			ProjectID: "project-demo",
			Workspace: "/books/demo",
			Source:    eventSource,
			Changes:   []Change{{Path: "chapters/ch01.md", Type: ChangeUpdated}},
		})
	}
	service.mu.Unlock()

	event := <-events
	if !event.Resync || len(event.Changes) != 0 {
		t.Fatalf("lagging subscriber event = %#v, want one authoritative resync", event)
	}
	if buffered := len(events); buffered != 0 {
		t.Fatalf("lagging subscriber retained %d stale events", buffered)
	}
}

func TestServiceRejectsPreviousProjectGenerationAfterRelink(t *testing.T) {
	service := NewService()
	t.Cleanup(service.Close)
	first := t.TempDir()
	second := t.TempDir()
	firstEvents, unsubscribeFirst, err := service.Subscribe("project-one", first)
	if err != nil {
		t.Fatal(err)
	}
	<-firstEvents
	service.mu.Lock()
	previousGeneration := service.projects["project-one"].generation
	service.mu.Unlock()

	service.CloseProject("project-one")
	unsubscribeFirst()
	secondEvents, unsubscribeSecond, err := service.Subscribe("project-one", second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unsubscribeSecond)
	relinkEvent := <-secondEvents
	if !relinkEvent.Resync || relinkEvent.ProjectID != "project-one" || relinkEvent.Workspace != second {
		t.Fatalf("Project relink event = %#v", relinkEvent)
	}
	service.mu.Lock()
	currentGeneration := service.projects["project-one"].generation
	service.mu.Unlock()

	service.publish("project-one", previousGeneration, Event{
		ProjectID: "project-one",
		Workspace: first,
		Source:    eventSource,
		Changes:   []Change{{Path: "old.md", Type: ChangeUpdated}},
	})
	service.publish("project-one", currentGeneration, Event{
		ProjectID: "project-one",
		Workspace: second,
		Source:    eventSource,
		Changes:   []Change{{Path: "new.md", Type: ChangeUpdated}},
	})

	event := <-secondEvents
	if event.Workspace != second || len(event.Changes) != 1 || event.Changes[0].Path != "new.md" {
		t.Fatalf("received stale workspace generation: %#v", event)
	}
}
