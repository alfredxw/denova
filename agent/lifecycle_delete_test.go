package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	runstate "github.com/alfredxw/denova/agent/internal/runtime"
	agentsession "github.com/alfredxw/denova/agent/session"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
)

type blockingDeleteStore struct {
	agentsession.Store
	mu                sync.Mutex
	opens             int
	deleteStarted     chan struct{}
	releaseDelete     chan struct{}
	secondOpenStarted chan struct{}
	deleteOnce        sync.Once
	secondOpenOnce    sync.Once
}

type failingDeleteStore struct {
	agentsession.Store
	err error
}

type deletionBlockingModel struct {
	started chan struct{}
	once    sync.Once
}

func (model *deletionBlockingModel) Generate(ctx context.Context, _ []*Message, _ ...ModelOption) (*Message, error) {
	model.once.Do(func() { close(model.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}

func (model *deletionBlockingModel) Stream(ctx context.Context, _ []*Message, _ ...ModelOption) (*StreamReader[*Message], error) {
	model.once.Do(func() { close(model.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}

func (*failingDeleteStore) Volatile() bool { return true }

func (store *failingDeleteStore) Delete(context.Context, agentsession.Key) error {
	return store.err
}

func (store *blockingDeleteStore) Open(ctx context.Context, key agentsession.Key) (agentsession.Log, error) {
	store.mu.Lock()
	store.opens++
	second := store.opens == 2
	store.mu.Unlock()
	if second {
		store.secondOpenOnce.Do(func() { close(store.secondOpenStarted) })
	}
	return store.Store.Open(ctx, key)
}

func (store *blockingDeleteStore) Delete(ctx context.Context, key agentsession.Key) error {
	store.deleteOnce.Do(func() { close(store.deleteStarted) })
	select {
	case <-store.releaseDelete:
	case <-ctx.Done():
		return ctx.Err()
	}
	return store.Store.Delete(ctx, key)
}

func TestAgentRunDeletesTemporarySessionAfterSettlement(t *testing.T) {
	store := agentsession.Memory()
	owner, err := New(
		context.Background(),
		Definition{Model: &lifecycleModel{responses: []*Message{AssistantMessage("done", nil)}}},
		WithSessionStore(store),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), Text("work"))
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := run.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("result=%#v error=%v", result, waitErr)
	}
	keys, err := store.List(context.Background(), SessionSelector{Namespace: "temporary"})
	if err != nil || len(keys) != 0 {
		t.Fatalf("temporary Session catalog=%#v error=%v", keys, err)
	}
}

func TestAgentRunDeletesTemporarySessionWhenAdmissionIsRejected(t *testing.T) {
	store := agentsession.Memory()
	owner, err := New(context.Background(), Definition{Model: &lifecycleModel{}}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	if _, err := owner.Run(context.Background(), Input{}); err == nil {
		t.Fatal("empty anonymous Run input was accepted")
	}
	keys, err := store.List(context.Background(), SessionSelector{Namespace: "temporary"})
	if err != nil || len(keys) != 0 {
		t.Fatalf("rejected temporary Session catalog=%#v error=%v", keys, err)
	}
}

func TestAgentRunSurfacesTemporarySessionDeletionFailure(t *testing.T) {
	deleteErr := errors.New("temporary deletion failed")
	store := &failingDeleteStore{Store: agentsession.Memory(), err: deleteErr}
	owner, err := New(
		context.Background(),
		Definition{Model: &lifecycleModel{responses: []*Message{AssistantMessage("done", nil)}}},
		WithSessionStore(store),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	run, err := owner.Run(context.Background(), Text("work"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := run.Wait(context.Background())
	if result.Status != ResultCompleted || !errors.Is(err, deleteErr) {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

func TestDeleteSessionsFencesConcurrentReopenUntilDurableDelete(t *testing.T) {
	store := &blockingDeleteStore{
		Store: agentsession.Memory(), deleteStarted: make(chan struct{}),
		releaseDelete: make(chan struct{}), secondOpenStarted: make(chan struct{}),
	}
	owner, err := New(context.Background(), Definition{Model: &lifecycleModel{}}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	key := NamedSession("delete-reopen-fence")
	if _, err := owner.Session(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	deleteResult := make(chan error, 1)
	safeGo(func() {
		deleteResult <- owner.DeleteSessions(context.Background(), SessionSelector{
			Namespace: key.Namespace, ID: key.ID,
		})
	}, func(err error) { deleteResult <- err })
	<-store.deleteStarted

	reopened := make(chan *Session, 1)
	reopenErr := make(chan error, 1)
	safeGo(func() {
		session, openErr := owner.Session(context.Background(), key)
		if openErr != nil {
			reopenErr <- openErr
			return
		}
		reopened <- session
	}, func(err error) { reopenErr <- err })
	select {
	case <-store.secondOpenStarted:
		t.Fatal("matching Session reached Store.Open before durable deletion completed")
	case session := <-reopened:
		t.Fatalf("matching Session reopened before durable deletion completed: %#v", session)
	case err := <-reopenErr:
		t.Fatalf("matching Session open failed while waiting for delete fence: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(store.releaseDelete)
	if err := <-deleteResult; err != nil {
		t.Fatal(err)
	}
	select {
	case session := <-reopened:
		if err := session.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	case err := <-reopenErr:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("matching Session did not reopen after durable deletion")
	}
}

func TestDeleteSessionsIDPrefixClosesOnlyMatchingActiveRuntimeLanes(t *testing.T) {
	store := agentsession.Memory()
	model := &lifecycleModel{responses: []*Message{AssistantMessage("kept", nil)}}
	owner, err := New(context.Background(), Definition{Model: model}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	removed, err := owner.Session(context.Background(), NamedSession("remove-active"))
	if err != nil {
		t.Fatal(err)
	}
	keep, err := owner.Session(context.Background(), NamedSession("keep-active"))
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.DeleteSessions(context.Background(), SessionSelector{
		Namespace: agentsession.DefaultNamespace, IDPrefix: "remove-",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := removed.Run(context.Background(), Text("must not run")); !errors.Is(err, ErrAgentClosed) {
		t.Fatalf("deleted active Session run error=%v", err)
	}
	run, err := keep.Run(context.Background(), Text("still works"))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := run.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("unmatched Session result=%#v error=%v", result, err)
	}
}

func TestDeleteSessionsCascadesThroughDurableChildSessionTree(t *testing.T) {
	store := agentsession.Memory()
	owner, err := New(context.Background(), Definition{Model: &lifecycleModel{}}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	parent := SessionKey{Namespace: "project", ID: "parent", Attributes: map[string]string{"project": "book"}}
	childAttributes, err := ChildSessionAttributes(parent)
	if err != nil {
		t.Fatal(err)
	}
	childAttributes["agent"] = "researcher"
	child := SessionKey{Namespace: "task.researcher", ID: "child", Attributes: childAttributes}
	grandchildAttributes, err := ChildSessionAttributes(child)
	if err != nil {
		t.Fatal(err)
	}
	grandchildAttributes["agent"] = "reviewer"
	grandchild := SessionKey{Namespace: "task.reviewer", ID: "grandchild", Attributes: grandchildAttributes}
	unrelated := SessionKey{Namespace: "project", ID: "unrelated", Attributes: map[string]string{"project": "other"}}
	for _, key := range []SessionKey{parent, child, grandchild, unrelated} {
		session, openErr := owner.Session(context.Background(), key)
		if openErr != nil {
			t.Fatalf("open %#v: %v", key, openErr)
		}
		if closeErr := session.Close(context.Background()); closeErr != nil {
			t.Fatalf("close %#v: %v", key, closeErr)
		}
	}
	if err := owner.DeleteSessions(context.Background(), SessionSelector{
		Namespace: parent.Namespace, ID: parent.ID, Attributes: parent.Attributes,
	}); err != nil {
		t.Fatal(err)
	}
	keys, err := owner.ListSessions(context.Background(), SessionSelector{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].ID != unrelated.ID {
		t.Fatalf("remaining Session tree = %#v", keys)
	}
	recreated, err := owner.Session(context.Background(), child)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := recreated.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Cursor != 0 || snapshot.ActiveRunID != "" || len(snapshot.RecentRuns) != 0 {
		t.Fatalf("deleted child recovered stale state: %#v", snapshot)
	}
}

func TestDeleteSessionsAbortsAndRemovesActiveDurableChild(t *testing.T) {
	store := agentsession.Memory()
	model := &deletionBlockingModel{started: make(chan struct{})}
	owner, err := New(context.Background(), Definition{Model: model}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	parent := SessionKey{Namespace: "project", ID: "active-parent", Attributes: map[string]string{"project": "book"}}
	parentSession, err := owner.Session(context.Background(), parent)
	if err != nil {
		t.Fatal(err)
	}
	if err := parentSession.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	attributes, err := ChildSessionAttributes(parent)
	if err != nil {
		t.Fatal(err)
	}
	attributes["agent"] = "researcher"
	child := SessionKey{Namespace: "task.researcher", ID: "active-child", Attributes: attributes}
	childSession, err := owner.Session(context.Background(), child)
	if err != nil {
		t.Fatal(err)
	}
	run, err := childSession.Run(context.Background(), Input{Text: "block", IdempotencyKey: "active-child-run"})
	if err != nil {
		t.Fatal(err)
	}
	<-model.started

	if err := owner.DeleteSessions(context.Background(), SessionSelector{
		Namespace: parent.Namespace, ID: parent.ID, Attributes: parent.Attributes,
	}); err != nil {
		t.Fatal(err)
	}
	result, waitErr := run.Wait(context.Background())
	if waitErr == nil && result.Status == ResultCompleted {
		t.Fatalf("active child Run completed successfully after parent deletion: %#v", result)
	}
	keys, err := owner.ListSessions(context.Background(), SessionSelector{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Fatalf("active child tree survived durable deletion: %#v", keys)
	}
}

func TestBuiltInFileSessionUsesCheckpointAndPersistentHistoricalCommandIndex(t *testing.T) {
	root := t.TempDir()
	store, err := sessionfile.NewWithOptions(root, sessionfile.Options{CheckpointTailRecords: 1})
	if err != nil {
		t.Fatal(err)
	}
	identity := CapabilityIdentity{Kind: "model.file-checkpoint-test", Version: 1, ConfigHash: "stable"}
	firstOwner, err := New(context.Background(), Definition{
		Key: "file-checkpoint-test", Model: &lifecycleModel{responses: []*Message{
			AssistantMessage("persisted", nil), AssistantMessage("newer one", nil), AssistantMessage("newer two", nil),
		}},
		ModelIdentity: identity,
	}, WithSessionStore(store), WithLimits(Limits{RetainedCommandLimit: 1}))
	if err != nil {
		t.Fatal(err)
	}
	session, err := firstOwner.Session(context.Background(), NamedSession("checkpointed"))
	if err != nil {
		t.Fatal(err)
	}
	input := Input{Text: "remember", IdempotencyKey: "historical-command"}
	firstRun, err := session.Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := firstRun.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	firstRunID := firstRun.ID()
	for index, text := range []string{"newer command one", "newer command two"} {
		run, runErr := session.Run(context.Background(), Input{
			Text: text, IdempotencyKey: fmt.Sprintf("newer-command-%d", index),
		})
		if runErr != nil {
			t.Fatal(runErr)
		}
		if _, waitErr := run.Wait(context.Background()); waitErr != nil {
			t.Fatal(waitErr)
		}
	}
	if err := firstOwner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	checkpointLog, err := store.Open(context.Background(), NamedSession("checkpointed"))
	if err != nil {
		t.Fatal(err)
	}
	checkpointed, ok := checkpointLog.(interface {
		ReplayRuntimeCheckpoint(context.Context, runstate.JournalCheckpointState) (runstate.JournalReplayStats, error)
	})
	if !ok {
		t.Fatalf("built-in file Log type %T has no checkpoint acceleration", checkpointLog)
	}
	restored := runstate.NewJournalCheckpointState(bindingForSession(NamedSession("checkpointed")))
	stats, err := checkpointed.ReplayRuntimeCheckpoint(context.Background(), restored)
	if err != nil {
		t.Fatal(err)
	}
	if stats.SnapshotGeneration == 0 || stats.TailBytesRead != 0 || stats.EventsRead != 0 {
		t.Fatalf("cold file replay did not use checkpoint plus bounded tail: %#v", stats)
	}
	if err := checkpointLog.Close(); err != nil {
		t.Fatal(err)
	}

	secondModel := &lifecycleModel{}
	secondOwner, err := New(context.Background(), Definition{
		Key: "file-checkpoint-test", Model: secondModel, ModelIdentity: identity,
	}, WithSessionStore(store), WithLimits(Limits{RetainedCommandLimit: 1}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondOwner.Close(context.Background()) })
	reopened, err := secondOwner.Session(context.Background(), NamedSession("checkpointed"))
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := reopened.Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed() || replayed.ID() != firstRunID {
		t.Fatalf("historical receipt replayed=%t id=%q want=%q", replayed.Replayed(), replayed.ID(), firstRunID)
	}
	if result, err := replayed.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("replayed result=%#v error=%v", result, err)
	}
	if calls := secondModel.calls(); len(calls) != 0 {
		t.Fatalf("historical command re-executed model: %#v", calls)
	}
}

func TestDeletedFileSessionDoesNotReappearAfterReopen(t *testing.T) {
	store, err := sessionfile.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity := CapabilityIdentity{Kind: "model.delete-test", Version: 1, ConfigHash: "stable"}
	firstOwner, err := New(context.Background(), Definition{
		Key: "delete-test", Model: &lifecycleModel{responses: []*Message{AssistantMessage("old answer", nil)}},
		ModelIdentity: identity,
	}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	session, err := firstOwner.Session(context.Background(), NamedSession("deleted"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Input{Text: "old question", IdempotencyKey: "old-command"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := session.Delete(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := firstOwner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	model := &lifecycleModel{responses: []*Message{AssistantMessage("new answer", nil)}}
	secondOwner, err := New(context.Background(), Definition{
		Key: "delete-test", Model: model, ModelIdentity: identity,
	}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondOwner.Close(context.Background()) })
	reopened, err := secondOwner.Session(context.Background(), NamedSession("deleted"))
	if err != nil {
		t.Fatal(err)
	}
	newRun, err := reopened.Run(context.Background(), Input{Text: "new question", IdempotencyKey: "new-command"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newRun.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := model.calls()
	if len(calls) != 1 || len(calls[0]) != 1 || calls[0][0].Content != "new question" {
		t.Fatalf("reopened deleted transcript=%#v", calls)
	}
}

func TestDeleteSessionsUsesPersistentCatalogAfterRestart(t *testing.T) {
	store, err := sessionfile.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity := CapabilityIdentity{Kind: "model.catalog-delete-test", Version: 1, ConfigHash: "stable"}
	definition := func(model BaseChatModel) Definition {
		return Definition{Key: "catalog-delete-test", Model: model, ModelIdentity: identity}
	}
	firstOwner, err := New(context.Background(), definition(&lifecycleModel{responses: []*Message{
		AssistantMessage("one", nil), AssistantMessage("two", nil), AssistantMessage("keep", nil),
	}}), WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	for index, id := range []string{"remove-one", "remove-two", "keep-one"} {
		session, openErr := firstOwner.Session(context.Background(), NamedSession(id))
		if openErr != nil {
			t.Fatal(openErr)
		}
		run, runErr := session.Run(context.Background(), Input{Text: id, IdempotencyKey: id})
		if runErr != nil {
			t.Fatal(runErr)
		}
		if _, waitErr := run.Wait(context.Background()); waitErr != nil {
			t.Fatalf("run %d: %v", index, waitErr)
		}
	}
	if err := firstOwner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	secondOwner, err := New(context.Background(), definition(&lifecycleModel{}), WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondOwner.Close(context.Background()) })
	if err := secondOwner.DeleteSessions(context.Background(), SessionSelector{
		Namespace: agentsession.DefaultNamespace, IDPrefix: "remove-",
	}); err != nil {
		t.Fatal(err)
	}
	keys, err := store.List(context.Background(), SessionSelector{Namespace: agentsession.DefaultNamespace})
	if err != nil || len(keys) != 1 || keys[0].ID != "keep-one" {
		t.Fatalf("remaining Sessions=%#v error=%v", keys, err)
	}
}
