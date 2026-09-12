package agent

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	agentsession "github.com/alfredxw/denova/agent/session"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
)

type interruptedTreeDiscovery struct {
	agentsession.Store
	probe            atomic.Bool
	entered, release chan struct{}
}

func (store *interruptedTreeDiscovery) List(ctx context.Context, selector agentsession.Selector) ([]agentsession.Key, error) {
	if store.probe.CompareAndSwap(true, false) {
		close(store.entered)
		<-store.release
		return nil, errors.New("injected descendant discovery failure")
	}
	return store.Store.List(ctx, selector)
}

func TestPartialTreePauseKeepsAdmissionClosedUntilRetryFinishes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	store := &interruptedTreeDiscovery{Store: agentsession.Memory(), entered: make(chan struct{}), release: make(chan struct{})}
	rootKey, childKey := NamedSession("partial-root"), NamedSession("partial-child")
	var err error
	childKey.Attributes, err = ChildSessionAttributes(rootKey)
	if err != nil {
		t.Fatal(err)
	}
	rootModel := &gatedLifecycleModel{started: make(chan struct{}), release: make(chan struct{})}
	owner, err := New(ctx, Definition{Name: "test", Model: rootModel}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	root, err := owner.Session(ctx, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	run, err := root.Run(ctx, Text("root task"))
	if err != nil {
		t.Fatal(err)
	}
	<-rootModel.started
	child, err := owner.Session(ctx, childKey)
	if err != nil {
		t.Fatal(err)
	}
	store.probe.Store(true)
	stopped := make(chan error, 1)
	safeGo(func() {
		_, err := owner.SuspendTree(ctx, rootKey, SuspendRequest{RunID: run.ID(), IdempotencyKey: "stop-tree"})
		stopped <- err
	}, func(err error) { stopped <- err })
	<-store.entered
	if _, err := child.Run(ctx, Text("concurrent child start")); !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("concurrent admission=%v", err)
	}
	queued, err := child.FollowUp(ctx, Input{Text: "received while stopping", IdempotencyKey: "pending-child"})
	if err != nil {
		t.Fatal(err)
	}
	close(store.release)
	if err := <-stopped; err == nil {
		t.Fatal("partial tree pause reported success")
	}
	if _, err := child.Run(ctx, Text("start after partial failure")); !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("partial failure released admission: %v", err)
	}
	if _, err := owner.SuspendTree(ctx, rootKey, SuspendRequest{RunID: run.ID(), IdempotencyKey: "stop-tree"}); err != nil {
		t.Fatal(err)
	}
	if result, err := run.Wait(ctx); err != nil || result.Status != ResultSuspended {
		t.Fatalf("root=%#v error=%v", result, err)
	}
	child, err = owner.Session(ctx, childKey)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := child.Snapshot(ctx)
	if err != nil || len(snapshot.QueuedRuns) != 1 || snapshot.QueuedRuns[0].ID != queued.RunID || rootModel.callCount() != 1 {
		t.Fatalf("stopped pending work=%#v error=%v", snapshot.QueuedRuns, err)
	}
	close(rootModel.release)
	resumed, err := owner.ResumeTree(ctx, rootKey, ResumeRequest{RunID: run.ID(), IdempotencyKey: "resume-tree"})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := resumed.Wait(ctx); err != nil || result.Status != ResultCompleted {
		t.Fatalf("resumed root=%#v error=%v", result, err)
	}
	attached, found, err := child.AttachRun(ctx, queued.RunID)
	if err != nil || !found {
		t.Fatalf("pending receipt disappeared: %v", err)
	}
	if result, err := attached.Wait(ctx); err != nil || result.Status != ResultCompleted {
		t.Fatalf("resumed pending=%#v error=%v", result, err)
	}
	if rootModel.callCount() != 3 {
		t.Fatal("pending work executed more than once")
	}
}

func TestTaskTreePauseReopenResumesParticipantsAndPendingWork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store, err := sessionfile.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rootKey := NamedSession("tree")
	attributes, err := ChildSessionAttributes(rootKey)
	if err != nil {
		t.Fatal(err)
	}
	childKey, independentKey := NamedSession("child"), NamedSession("independent")
	childKey.Attributes, independentKey.Attributes = attributes, attributes
	models := map[string]*gatedLifecycleModel{}
	for _, key := range []SessionKey{rootKey, childKey, independentKey} {
		models[key.ID] = &gatedLifecycleModel{started: make(chan struct{}), release: make(chan struct{})}
	}
	owner, err := New(ctx, SourceFunc(func(_ context.Context, request PrepareRequest) (Definition, error) {
		return Definition{Name: "test", Model: models[request.Session.Key.ID]}, nil
	}), WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	runs := map[string]*Run{}
	for _, key := range []SessionKey{rootKey, childKey, independentKey} {
		session, err := owner.Session(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		runs[key.ID], err = session.Run(ctx, Input{Text: key.ID, IdempotencyKey: "start"})
		if err != nil {
			t.Fatal(err)
		}
		<-models[key.ID].started
	}
	independent, _ := owner.Session(ctx, independentKey)
	if _, err := independent.SuspendAndClose(ctx, SuspendRequest{IdempotencyKey: "own-pause"}); err != nil {
		t.Fatal(err)
	}
	child, _ := owner.Session(ctx, childKey)
	pending, err := child.FollowUp(ctx, Input{Text: "next child task", IdempotencyKey: "next"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.SuspendTree(ctx, rootKey, SuspendRequest{RunID: runs[rootKey.ID].ID(), IdempotencyKey: "pause-tree"}); err != nil {
		t.Fatal(err)
	}
	for _, run := range runs {
		if result, err := run.Wait(ctx); err != nil || result.Status != ResultSuspended {
			t.Fatalf("paused result=%#v err=%v", result, err)
		}
	}
	if err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	for _, model := range models {
		close(model.release)
	}
	resumedModels := map[string]*lifecycleModel{}
	for _, key := range []SessionKey{rootKey, childKey, independentKey} {
		resumedModels[key.ID] = &lifecycleModel{responses: []*Message{AssistantMessage("first", nil), AssistantMessage("second", nil)}}
	}
	owner, err = New(ctx, SourceFunc(func(_ context.Context, request PrepareRequest) (Definition, error) {
		return Definition{Name: "test", Model: resumedModels[request.Session.Key.ID]}, nil
	}), WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	child, err = owner.Session(ctx, childKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := child.ResumeRun(ctx, ResumeRequest{RunID: runs[childKey.ID].ID(), IdempotencyKey: "bypass"}); !errors.Is(err, ErrSessionBusy) {
		t.Fatalf("child bypass error=%v", err)
	}
	root, err := owner.ResumeTree(ctx, rootKey, ResumeRequest{RunID: runs[rootKey.ID].ID(), IdempotencyKey: "resume-tree"})
	if err != nil {
		t.Fatal(err)
	}
	if root.ID() != runs[rootKey.ID].ID() {
		t.Fatal("root Run identity changed")
	}
	if result, err := root.Wait(ctx); err != nil || result.Status != ResultCompleted {
		t.Fatalf("root=%#v err=%v", result, err)
	}
	child, err = owner.Session(ctx, childKey)
	if err != nil {
		t.Fatal(err)
	}
	next, found, err := child.AttachRun(ctx, pending.RunID)
	if err != nil || !found {
		t.Fatalf("pending child missing: %v", err)
	}
	if result, err := next.Wait(ctx); err != nil || result.Status != ResultCompleted {
		t.Fatalf("pending=%#v err=%v", result, err)
	}
	if len(resumedModels[childKey.ID].calls()) != 2 || len(resumedModels[independentKey.ID].calls()) != 0 {
		t.Fatal("tree resumed the wrong tasks")
	}
}

func TestAbortTreeReleasesAdmissionForLaterTasks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	model := &gatedLifecycleModel{started: make(chan struct{}), release: make(chan struct{})}
	owner, err := New(ctx, Definition{Name: "test", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	key := NamedSession("abort-tree")
	session, err := owner.Session(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(ctx, Text("first"))
	if err != nil {
		t.Fatal(err)
	}
	<-model.started
	if _, err := owner.AbortTree(ctx, key, AbortRequest{IdempotencyKey: "abort-tree"}); err != nil {
		t.Fatal(err)
	}
	if result, err := run.Wait(ctx); err != nil || result.Status != ResultAborted {
		t.Fatalf("aborted=%#v err=%v", result, err)
	}
	close(model.release)
	session, err = owner.Session(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	next, err := session.Run(ctx, Text("new task"))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := next.Wait(ctx); err != nil || result.Status != ResultCompleted {
		t.Fatalf("next=%#v err=%v", result, err)
	}
}
