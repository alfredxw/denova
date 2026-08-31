package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	runstate "github.com/alfredxw/denova/agent/internal/runstate"
	agentsession "github.com/alfredxw/denova/agent/session"
)

type lifecycleModel struct {
	mu        sync.Mutex
	responses []*Message
	inputs    [][]*Message
	options   []*Options
}

type observingSessionStore struct {
	agentsession.Store
	mu    sync.Mutex
	kinds []string
}

func newObservingSessionStore() *observingSessionStore {
	return &observingSessionStore{Store: agentsession.Memory()}
}

func (store *observingSessionStore) Open(ctx context.Context, key agentsession.Key) (agentsession.Log, error) {
	log, err := store.Store.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	return &observingSessionLog{Log: log, store: store}, nil
}

func (store *observingSessionStore) count(kind string) int {
	store.mu.Lock()
	defer store.mu.Unlock()
	count := 0
	for _, candidate := range store.kinds {
		if candidate == kind {
			count++
		}
	}
	return count
}

type observingSessionLog struct {
	agentsession.Log
	store *observingSessionStore
}

func (log *observingSessionLog) Append(ctx context.Context, expected agentsession.Revision, records ...agentsession.Record) (agentsession.Revision, error) {
	revision, err := log.Log.Append(ctx, expected, records...)
	if err != nil {
		return revision, err
	}
	log.store.mu.Lock()
	for _, record := range records {
		log.store.kinds = append(log.store.kinds, record.Kind)
	}
	log.store.mu.Unlock()
	return revision, nil
}

func (model *lifecycleModel) Generate(_ context.Context, input []*Message, options ...ModelOption) (*Message, error) {
	return model.next(input, options...)
}

func (model *lifecycleModel) Stream(_ context.Context, input []*Message, options ...ModelOption) (*StreamReader[*Message], error) {
	message, err := model.next(input, options...)
	if err != nil {
		return nil, err
	}
	return StreamReaderFromArray([]*Message{message}), nil
}

func (model *lifecycleModel) next(input []*Message, options ...ModelOption) (*Message, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.inputs = append(model.inputs, cloneMessages(input))
	model.options = append(model.options, GetCommonOptions(&Options{}, options...))
	if len(model.responses) == 0 {
		return nil, errors.New("lifecycle model exhausted")
	}
	message := CloneMessage(model.responses[0])
	model.responses = model.responses[1:]
	return message, nil
}

func (model *lifecycleModel) calls() [][]*Message {
	model.mu.Lock()
	defer model.mu.Unlock()
	result := make([][]*Message, len(model.inputs))
	for index := range model.inputs {
		result[index] = cloneMessages(model.inputs[index])
	}
	return result
}

func TestLightweightSessionRetainsTranscriptAcrossRuns(t *testing.T) {
	store := agentsession.Memory()
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("first answer", nil), AssistantMessage("second answer", nil),
	}}
	owner, err := New(context.Background(), Definition{Name: "test", Model: model}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	session, err := owner.Session(context.Background(), NamedSession("main"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := session.Run(context.Background(), Text("first question"))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := first.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("first result = %#v, err = %v", result, err)
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}

	owner, err = New(context.Background(), Definition{Name: "test", Model: model}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err = owner.Session(context.Background(), NamedSession("main"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.Run(context.Background(), Text("second question"))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := second.Wait(context.Background()); err != nil || result.Status != ResultCompleted {
		t.Fatalf("second result = %#v, err = %v", result, err)
	}
	calls := model.calls()
	if len(calls) != 2 || len(calls[1]) != 3 || calls[1][0].Content != "first question" || calls[1][1].Content != "first answer" {
		t.Fatalf("second model transcript = %#v", calls)
	}
}

func TestActiveRunDoesNotPersistPartialTranscript(t *testing.T) {
	store := newObservingSessionStore()
	model := &gatedLifecycleModel{started: make(chan struct{}), release: make(chan struct{})}
	owner, err := New(context.Background(), Definition{Name: "test", Model: model}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("partial-transcript"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Text("wait"))
	if err != nil {
		t.Fatal(err)
	}
	<-model.started
	if count := store.count(sessionTranscriptRecord); count != 0 {
		t.Fatalf("active Run persisted %d partial transcripts", count)
	}
	close(model.release)
	if _, err := run.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if count := store.count(sessionTranscriptRecord); count != 1 {
		t.Fatalf("completed Run persisted %d transcripts, want 1", count)
	}
}

func TestCloseSessionsClosesActiveChildSessionTree(t *testing.T) {
	ctx := context.Background()
	model := &gatedLifecycleModel{started: make(chan struct{}), release: make(chan struct{})}
	owner, err := New(ctx, Definition{Name: "test", Model: model}, WithSessionStore(agentsession.Memory()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	parent, err := owner.Session(ctx, NamedSession("parent"))
	if err != nil {
		t.Fatal(err)
	}
	attributes, err := ChildSessionAttributes(parent.Key())
	if err != nil {
		t.Fatal(err)
	}
	attributes["agent"] = "researcher"
	child, err := owner.Session(ctx, SessionKey{Namespace: "task.researcher", ID: "child", Attributes: attributes})
	if err != nil {
		t.Fatal(err)
	}
	run, err := child.Run(ctx, Text("wait"))
	if err != nil {
		t.Fatal(err)
	}
	<-model.started
	childSelector := SessionSelector{Namespace: "task.researcher", Attributes: map[string]string{"agent": "researcher"}}
	if count, err := owner.CountActiveSessions(ctx, childSelector); err != nil || count != 1 {
		t.Fatalf("active child count = %d, err = %v", count, err)
	}
	parentKey := parent.Key()
	if err := owner.CloseSessions(ctx, SessionSelector{Namespace: parentKey.Namespace, ID: parentKey.ID}); err != nil {
		t.Fatal(err)
	}
	result, err := run.Wait(ctx)
	if err != nil || result.Status != ResultAborted {
		t.Fatalf("closed child result = %#v, err = %v", result, err)
	}
	if count, err := owner.CountActiveSessions(ctx, childSelector); err != nil || count != 0 {
		t.Fatalf("active child count after close = %d, err = %v", count, err)
	}
}

type gatedLifecycleModel struct {
	mu      sync.Mutex
	started chan struct{}
	release chan struct{}
	once    sync.Once
	calls   int
}

func (model *gatedLifecycleModel) wait(ctx context.Context) (*Message, error) {
	model.mu.Lock()
	model.calls++
	model.mu.Unlock()
	model.once.Do(func() { close(model.started) })
	select {
	case <-model.release:
		return AssistantMessage("done", nil), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (model *gatedLifecycleModel) Generate(ctx context.Context, _ []*Message, _ ...ModelOption) (*Message, error) {
	return model.wait(ctx)
}

func (model *gatedLifecycleModel) Stream(ctx context.Context, _ []*Message, _ ...ModelOption) (*StreamReader[*Message], error) {
	message, err := model.wait(ctx)
	if err != nil {
		return nil, err
	}
	return StreamReaderFromArray([]*Message{message}), nil
}

func (model *gatedLifecycleModel) callCount() int {
	model.mu.Lock()
	defer model.mu.Unlock()
	return model.calls
}

func TestSessionSnapshotProjectsOnlyLiveCoordination(t *testing.T) {
	model := &gatedLifecycleModel{started: make(chan struct{}), release: make(chan struct{})}
	owner, err := New(context.Background(), Definition{Name: "test", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("main"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Text("first"))
	if err != nil {
		t.Fatal(err)
	}
	<-model.started
	queued, err := run.Queue(context.Background(), Text("same run"))
	if err != nil {
		t.Fatal(err)
	}
	next, err := run.FollowUp(context.Background(), Text("next run"))
	if err != nil {
		t.Fatal(err)
	}
	if err := run.handleEngineEvent(runstate.EngineToolStarted{
		CallID: "tool-1", Name: "read", ExecutionAuthorized: true,
	}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := session.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.QueuedRuns) != 2 || snapshot.QueuedRuns[0].CommandID != queued.ID() ||
		snapshot.QueuedRuns[0].ID != run.ID() || snapshot.QueuedRuns[1].ID != next.ID() {
		t.Fatalf("queued runs = %#v", snapshot.QueuedRuns)
	}
	if len(snapshot.OpenTools) != 1 || snapshot.OpenTools[0].CallID != "tool-1" || snapshot.OpenTools[0].RunID != run.ID() {
		t.Fatalf("open tools = %#v", snapshot.OpenTools)
	}
	if err := run.handleEngineEvent(runstate.EngineToolFinished{CallID: "tool-1", Name: "read"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = session.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.OpenTools) != 0 {
		t.Fatalf("finished tool remained open: %#v", snapshot.OpenTools)
	}
	close(model.release)
	if _, err := run.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := next.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRunStartedAtBeginsOnlyWhenAQueuedRunActivates(t *testing.T) {
	model := &gatedLifecycleModel{started: make(chan struct{}), release: make(chan struct{})}
	owner, err := New(context.Background(), Definition{Name: "test", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("run-started-at"))
	if err != nil {
		t.Fatal(err)
	}
	active, err := session.Run(context.Background(), Text("active"))
	if err != nil {
		t.Fatal(err)
	}
	<-model.started
	if active.startedAtValue().IsZero() {
		t.Fatal("active Run has no activation timestamp")
	}
	queued, err := active.FollowUp(context.Background(), Text("queued"))
	if err != nil {
		t.Fatal(err)
	}
	if !queued.startedAtValue().IsZero() {
		t.Fatalf("queued Run started during wait: %s", queued.startedAtValue())
	}

	close(model.release)
	if _, err := active.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := queued.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	startedAt := queued.startedAtValue()
	if startedAt.IsZero() {
		t.Fatal("queued Run did not record its activation timestamp")
	}
	for event := range queued.Events() {
		if started, ok := event.Payload.(RunStarted); ok {
			if !started.StartedAt.Equal(startedAt) {
				t.Fatalf("RunStarted timestamp = %s, want %s", started.StartedAt, startedAt)
			}
			return
		}
	}
	t.Fatal("queued Run did not publish RunStarted")
}

func TestInspectUsesUTCStartedAtLikeAdmittedRuns(t *testing.T) {
	owner, err := New(context.Background(), Definition{Name: "test", Model: &lifecycleModel{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("inspection-started-at"))
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := session.Inspect(context.Background(), Text("inspect"))
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Run.StartedAt.Location() != time.UTC {
		t.Fatalf("inspection StartedAt location = %s, want UTC", inspection.Run.StartedAt.Location())
	}
}

func TestAbortingPendingRunDoesNotStartItsSuccessor(t *testing.T) {
	model := &gatedLifecycleModel{started: make(chan struct{}), release: make(chan struct{})}
	owner, err := New(context.Background(), Definition{Name: "test", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("pending-abort"))
	if err != nil {
		t.Fatal(err)
	}
	active, err := session.Run(context.Background(), Text("active"))
	if err != nil {
		t.Fatal(err)
	}
	<-model.started
	pending, err := active.FollowUp(context.Background(), Text("pending"))
	if err != nil {
		t.Fatal(err)
	}
	successor, err := active.FollowUp(context.Background(), Text("successor"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pending.Abort(context.Background(), AbortRequest{Reason: "no longer needed"}); err != nil {
		t.Fatal(err)
	}
	if result, err := pending.Wait(context.Background()); err != nil || result.Status != ResultAborted || result.Reason != "no longer needed" {
		t.Fatalf("pending result = %#v, err = %v", result, err)
	}
	if calls := model.callCount(); calls != 1 {
		t.Fatalf("successor started before active Run settled: model calls = %d", calls)
	}
	close(model.release)
	if _, err := active.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := successor.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAbortPreservesCallerReason(t *testing.T) {
	model := &gatedLifecycleModel{started: make(chan struct{}), release: make(chan struct{})}
	owner, err := New(context.Background(), Definition{Name: "test", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("abort-reason"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Text("wait"))
	if err != nil {
		t.Fatal(err)
	}
	<-model.started
	if _, err := run.Abort(context.Background(), AbortRequest{Reason: "cancelled by user"}); err != nil {
		t.Fatal(err)
	}
	result, err := run.Wait(context.Background())
	if err != nil || result.Status != ResultAborted || result.Reason != "cancelled by user" {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}

type activeStreamingLifecycleModel struct {
	started chan struct{}
	once    sync.Once
}

func (*activeStreamingLifecycleModel) Generate(context.Context, []*Message, ...ModelOption) (*Message, error) {
	return nil, errors.New("unexpected Generate")
}

func (model *activeStreamingLifecycleModel) Stream(ctx context.Context, _ []*Message, _ ...ModelOption) (*StreamReader[*Message], error) {
	first := true
	return &StreamReader[*Message]{recvFn: func() (*Message, error) {
		if first {
			first = false
			model.once.Do(func() { close(model.started) })
			return AssistantMessage("partial", nil), nil
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}}, nil
}

func TestAbortWhileStreamingPreservesCallerReason(t *testing.T) {
	model := &activeStreamingLifecycleModel{started: make(chan struct{})}
	owner, err := New(context.Background(), Definition{Name: "test", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("abort-stream-reason"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Text("stream"))
	if err != nil {
		t.Fatal(err)
	}
	<-model.started
	if _, err := run.Abort(context.Background(), AbortRequest{Reason: "user_requested"}); err != nil {
		t.Fatal(err)
	}
	result, err := run.Wait(context.Background())
	if err != nil || result.Status != ResultAborted || result.Reason != "user_requested" {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}

func TestCancelledControlContextDoesNotMutateRun(t *testing.T) {
	model := &gatedLifecycleModel{started: make(chan struct{}), release: make(chan struct{})}
	owner, err := New(context.Background(), Definition{Name: "test", Model: model})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("cancelled-controls"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := session.Run(context.Background(), Text("wait"))
	if err != nil {
		t.Fatal(err)
	}
	<-model.started

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := run.Queue(cancelled, Text("queue")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Queue error = %v", err)
	}
	if _, err := run.Steer(cancelled, Text("steer")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Steer error = %v", err)
	}
	if _, err := run.FollowUp(cancelled, Text("follow up")); !errors.Is(err, context.Canceled) {
		t.Fatalf("FollowUp error = %v", err)
	}
	if _, err := run.Abort(cancelled, AbortRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Abort error = %v", err)
	}
	if snapshot, err := session.Snapshot(context.Background()); err != nil {
		t.Fatal(err)
	} else if len(snapshot.QueuedRuns) != 0 {
		t.Fatalf("cancelled commands changed queue: %#v", snapshot.QueuedRuns)
	}

	queued, err := run.Queue(nil, Text("accepted"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queued.Cancel(cancelled, QueueControlRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Cancel error = %v", err)
	}
	if _, err := queued.Interrupt(cancelled, QueueControlRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Interrupt error = %v", err)
	}
	if snapshot, err := session.Snapshot(context.Background()); err != nil {
		t.Fatal(err)
	} else if len(snapshot.QueuedRuns) != 1 || snapshot.QueuedRuns[0].Delivery != DeliveryFollowUp {
		t.Fatalf("cancelled queue controls changed queue: %#v", snapshot.QueuedRuns)
	}
	if _, err := queued.Cancel(nil, QueueControlRequest{}); err != nil {
		t.Fatal(err)
	}

	close(model.release)
	if _, err := run.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}
