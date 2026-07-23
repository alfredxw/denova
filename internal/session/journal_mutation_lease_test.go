package session

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"denova/internal/filelease"
)

func TestDisplayMutationRefreshesDomainCommitFromAnotherSession(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	displayWriter, err := store.GetOrCreate("shared-mutation")
	if err != nil {
		t.Fatal(err)
	}
	domainWriter, err := loadSession(displayWriter.filePath)
	if err != nil {
		t.Fatal(err)
	}
	lookupObserver, err := loadSession(displayWriter.filePath)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := NewDomainCommitIntent(
		DomainCommitIdentity{CommandID: "command-1", OperationID: "operation-1", Cycle: 1},
		schema.AssistantMessage("canonical answer", nil),
		MessageMetadata{RunID: "run-1", AgentKind: "ide"},
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := domainWriter.CommitDomainMessage(intent)
	if err != nil {
		t.Fatal(err)
	}
	observed, found, err := lookupObserver.FindDomainCommit(intent.Identity, intent.Message.Role, intent.Hash)
	if err != nil {
		t.Fatalf("refresh stale domain commit lookup: %v", err)
	}
	if !found || observed != receipt {
		t.Fatalf("stale lookup receipt = %+v found=%t, want %+v", observed, found, receipt)
	}
	if err := displayWriter.AppendDisplayEvent(DisplayEvent{ID: "thinking-1", Role: "thinking", Content: "display only"}); err != nil {
		t.Fatalf("append display after independent domain commit: %v", err)
	}

	reloaded, err := loadSession(displayWriter.filePath)
	if err != nil {
		t.Fatal(err)
	}
	history := reloaded.History()
	if len(history) != 2 || history[0].Content != "canonical answer" || history[1].Content != "display only" {
		t.Fatalf("reloaded canonical history = %#v, want domain message followed by display event", history)
	}
}

func TestDisplayMutationWaitsForCanonicalJournalLease(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	displayWriter, err := store.GetOrCreate("lease-order")
	if err != nil {
		t.Fatal(err)
	}
	domainWriter, err := loadSession(displayWriter.filePath)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := NewDomainCommitIntent(
		DomainCommitIdentity{CommandID: "command-lease", OperationID: "operation-lease", Cycle: 1},
		schema.AssistantMessage("leased answer", nil),
		MessageMetadata{},
	)
	if err != nil {
		t.Fatal(err)
	}

	release, err := filelease.Acquire(context.Background(), displayWriter.filePath+".domain.lock")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(displayWriter.filePath)
	if err != nil {
		t.Fatal(err)
	}
	displayResult := make(chan error, 1)
	domainResult := make(chan error, 1)
	displayStarted := make(chan struct{})
	domainStarted := make(chan struct{})
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				displayResult <- fmt.Errorf("display mutation panic: %v", recovered)
			}
		}()
		close(displayStarted)
		displayResult <- displayWriter.AppendDisplayEvent(DisplayEvent{ID: "thinking-lease", Role: "thinking", Content: "waiting"})
	}()
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				domainResult <- fmt.Errorf("domain mutation panic: %v", recovered)
			}
		}()
		close(domainStarted)
		_, commitErr := domainWriter.CommitDomainMessage(intent)
		domainResult <- commitErr
	}()
	<-displayStarted
	<-domainStarted

	blocked := time.NewTimer(40 * time.Millisecond)
	select {
	case err := <-displayResult:
		blocked.Stop()
		_ = release()
		t.Fatalf("display mutation completed while canonical lease was held: %v", err)
	case err := <-domainResult:
		blocked.Stop()
		_ = release()
		t.Fatalf("domain mutation completed while canonical lease was held: %v", err)
	case <-blocked.C:
	}
	afterBlocked, err := os.Stat(displayWriter.filePath)
	if err != nil {
		_ = release()
		t.Fatal(err)
	}
	if afterBlocked.Size() != before.Size() {
		_ = release()
		t.Fatalf("journal size changed behind held lease: before=%d after=%d", before.Size(), afterBlocked.Size())
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}

	for name, result := range map[string]<-chan error{"display": displayResult, "domain": domainResult} {
		select {
		case err := <-result:
			if err != nil {
				t.Fatalf("%s mutation after lease release: %v", name, err)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatalf("%s mutation remained blocked after lease release", name)
		}
	}
	reloaded, err := loadSession(displayWriter.filePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(reloaded.History()); got != 2 {
		t.Fatalf("canonical history length = %d, want both committed mutations", got)
	}
}

func TestConcurrentIndependentSessionMutationsKeepJournalCanonical(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	displayWriter, err := store.GetOrCreate("mutation-stress")
	if err != nil {
		t.Fatal(err)
	}
	domainWriter, err := loadSession(displayWriter.filePath)
	if err != nil {
		t.Fatal(err)
	}

	const writesPerSide = 12
	type mutationResult struct {
		name string
		err  error
	}
	results := make(chan mutationResult, writesPerSide*2)
	launch := func(name string, mutation func() error) {
		go func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					results <- mutationResult{name: name, err: fmt.Errorf("panic: %v", recovered)}
				}
			}()
			results <- mutationResult{name: name, err: mutation()}
		}()
	}
	for index := 0; index < writesPerSide; index++ {
		index := index
		launch(fmt.Sprintf("display-%d", index), func() error {
			return displayWriter.AppendDisplayEvent(DisplayEvent{
				ID: fmt.Sprintf("thinking-%d", index), Role: "thinking", Content: fmt.Sprintf("display-%d", index),
			})
		})
		launch(fmt.Sprintf("domain-%d", index), func() error {
			intent, intentErr := NewDomainCommitIntent(
				DomainCommitIdentity{CommandID: fmt.Sprintf("command-%d", index), OperationID: fmt.Sprintf("operation-%d", index), Cycle: 1},
				schema.AssistantMessage(fmt.Sprintf("answer-%d", index), nil),
				MessageMetadata{},
			)
			if intentErr != nil {
				return intentErr
			}
			_, commitErr := domainWriter.CommitDomainMessage(intent)
			return commitErr
		})
	}
	for index := 0; index < writesPerSide*2; index++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("%s: %v", result.name, result.err)
		}
	}
	if err := displayWriter.RefreshCanonical(context.Background()); err != nil {
		t.Fatalf("refresh display writer: %v", err)
	}
	if err := domainWriter.RefreshCanonical(context.Background()); err != nil {
		t.Fatalf("refresh domain writer: %v", err)
	}
	if got := len(displayWriter.History()); got != writesPerSide*2 {
		t.Fatalf("display writer history length = %d, want %d", got, writesPerSide*2)
	}
	if got := len(domainWriter.History()); got != writesPerSide*2 {
		t.Fatalf("domain writer history length = %d, want %d", got, writesPerSide*2)
	}
	if _, err := loadSession(displayWriter.filePath); err != nil {
		t.Fatalf("reload canonical journal after concurrent success: %v", err)
	}
}

func TestCanonicalRefreshKeepsLocalDisplayTailPending(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	localWriter, err := store.GetOrCreate("display-tail-merge")
	if err != nil {
		t.Fatal(err)
	}
	if err := localWriter.AppendDisplayEvent(DisplayEvent{ID: "call-1", Role: "tool_call", Name: "read_file", Status: "running"}); err != nil {
		t.Fatal(err)
	}
	remoteWriter, err := loadSession(localWriter.filePath)
	if err != nil {
		t.Fatal(err)
	}

	if err := localWriter.AppendDisplayToolArgs("call-1", "read_file", `{"local":true}`); err != nil {
		t.Fatal(err)
	}
	if err := remoteWriter.AppendDisplayToolArgs("call-1", "read_file", `{"remote":true}`); err != nil {
		t.Fatal(err)
	}
	if err := remoteWriter.UpdateDisplayToolStatus("call-1", "read_file", "running"); err != nil {
		t.Fatal(err)
	}
	if err := localWriter.UpdateDisplayToolResult("call-1", "read_file", "success", "ok"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := loadSession(localWriter.filePath)
	if err != nil {
		t.Fatal(err)
	}
	history := reloaded.History()
	wantArgs := `{"remote":true}{"local":true}`
	if len(history) != 1 || history[0].Args != wantArgs || history[0].Status != "success" {
		t.Fatalf("merged display state = %#v, want canonical remote tail followed by flushed local tail", history)
	}
}
