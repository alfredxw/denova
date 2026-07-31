package session

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/localfs"
)

func TestStoreDeleteWaitsForCanonicalJournalLease(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetOrCreate("keep"); err != nil {
		t.Fatal(err)
	}
	target, err := store.GetOrCreate("delete-me")
	if err != nil {
		t.Fatal(err)
	}
	release, err := localfs.AcquireLease(context.Background(), target.filePath+".domain.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	result := make(chan error, 1)
	runSessionErrorTestGoroutine(result, "delete leased session", func() error {
		return store.Delete(target.ID)
	})
	select {
	case err := <-result:
		t.Fatalf("delete crossed the held journal lease: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestStoreDeleteByPrefixWaitsForEveryCanonicalJournalLease(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.GetOrCreate("story-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetOrCreate("story-b"); err != nil {
		t.Fatal(err)
	}
	release, err := localfs.AcquireLease(context.Background(), first.filePath+".domain.lock")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	result := make(chan error, 1)
	runSessionErrorTestGoroutine(result, "delete session prefix", func() error {
		return store.DeleteByPrefix("story-")
	})
	select {
	case err := <-result:
		t.Fatalf("prefix delete crossed the held journal lease: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestDeletedSessionHandleCannotAppendIntoRecreatedJournal(t *testing.T) {
	dir := t.TempDir()
	firstStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := firstStore.GetOrCreate("shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := stale.Append(agent.UserMessage("old journal")); err != nil {
		t.Fatal(err)
	}
	if _, err := firstStore.GetOrCreate("keep"); err != nil {
		t.Fatal(err)
	}

	secondStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := secondStore.Delete("shared"); err != nil {
		t.Fatal(err)
	}
	recreated, err := secondStore.GetOrCreate("shared")
	if err != nil {
		t.Fatal(err)
	}
	if err := recreated.Append(agent.UserMessage("new journal")); err != nil {
		t.Fatal(err)
	}

	appendErr := stale.Append(agent.UserMessage("must not cross incarnation"))
	if appendErr == nil || !strings.Contains(appendErr.Error(), "incarnation") {
		t.Fatalf("stale append error = %v", appendErr)
	}
	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("shared")
	if err != nil {
		t.Fatal(err)
	}
	messages := reloaded.GetMessages()
	if len(messages) != 1 || messages[0].Content != "new journal" {
		t.Fatalf("recreated journal was contaminated: %#v", messages)
	}
}

func runSessionErrorTestGoroutine(destination chan<- error, scope string, run func() error) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				destination <- fmt.Errorf("%s panic: %v", scope, recovered)
			}
		}()
		destination <- run()
	}()
}
