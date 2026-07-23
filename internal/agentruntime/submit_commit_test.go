package agentruntime_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"denova/internal/agentruntime"
)

func TestSubmitCancellationAfterAdmissionReturnsCommittedReceipt(t *testing.T) {
	t.Parallel()

	store := newCommitBarrierStore()
	runtime, err := agentruntime.NewRuntime(
		agentruntime.NewScriptedEngine(agentruntime.EngineScript{Continue: make(chan struct{})}),
		store,
		agentruntime.RuntimeConfig{},
	)
	if err != nil {
		t.Fatal(err)
	}
	harness, err := runtime.Open(context.Background(), agentruntime.WritingBinding{
		Workspace: "/book", SessionID: "submit-admission",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	type submitResult struct {
		receipt agentruntime.Receipt
		err     error
	}
	result := make(chan submitResult, 1)
	runExternalTestGoroutine(result, func(recovered any) submitResult {
		return submitResult{err: fmt.Errorf("submit after admission panic: %v", recovered)}
	}, func() submitResult {
		receipt, submitErr := harness.Submit(ctx, agentruntime.StartTurn{
			ID: "accepted-before-cancel", Input: agentruntime.UserInput{Text: "write"},
		})
		return submitResult{receipt: receipt, err: submitErr}
	})

	<-store.committed
	cancel()
	select {
	case got := <-result:
		t.Fatalf("Submit returned before admitted commit resolved: receipt=%+v err=%v", got.receipt, got.err)
	case <-time.After(20 * time.Millisecond):
	}
	close(store.release)
	got := <-result
	if got.err != nil {
		t.Fatalf("Submit returned an ambiguous cancellation after commit: %v", got.err)
	}
	if got.receipt.CommandID != "accepted-before-cancel" || got.receipt.OperationID == "" || got.receipt.Cursor == 0 {
		t.Fatalf("committed receipt = %+v", got.receipt)
	}
}

type commitBarrierStore struct {
	base      *agentruntime.MemoryJournalStore
	committed chan struct{}
	release   chan struct{}
	once      sync.Once
}

func newCommitBarrierStore() *commitBarrierStore {
	return &commitBarrierStore{
		base: agentruntime.NewMemoryJournalStore(), committed: make(chan struct{}), release: make(chan struct{}),
	}
}

func (s *commitBarrierStore) OpenJournal(ctx context.Context, key string) (agentruntime.Journal, error) {
	journal, err := s.base.OpenJournal(ctx, key)
	if err != nil {
		return nil, err
	}
	return &commitBarrierJournal{Journal: journal, store: s}, nil
}

type commitBarrierJournal struct {
	agentruntime.Journal
	store *commitBarrierStore
}

func (j *commitBarrierJournal) Append(ctx context.Context, expected agentruntime.Cursor, payloads []agentruntime.EventPayload) ([]agentruntime.Event, error) {
	committed, err := j.Journal.Append(ctx, expected, payloads)
	if err != nil {
		return nil, err
	}
	j.store.once.Do(func() { close(j.store.committed) })
	<-j.store.release
	return committed, nil
}
