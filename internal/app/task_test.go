package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	agents "denova/internal/agents"
	"denova/internal/observability"
)

func TestTaskDisconnectsSlowSubscriberInsteadOfDroppingEvents(t *testing.T) {
	started := make(chan struct{})
	emitAll := make(chan struct{})
	emitted := make(chan struct{})
	finish := make(chan struct{})

	task := NewTask(func(_ context.Context, _ *Task, emit func(agents.Event)) {
		close(started)
		<-emitAll
		for i := 0; i < 257; i++ {
			emit(agents.Event{Type: "chunk"})
		}
		close(emitted)
		<-finish
	})
	<-started

	initial, slow := task.Subscribe()
	if len(initial) != 0 {
		t.Fatalf("initial replay length = %d, want 0", len(initial))
	}
	close(emitAll)
	<-emitted

	received := 0
	for range slow.Events() {
		received++
	}
	if slow.EndReason() != TaskSubscriptionLagged {
		t.Fatalf("slow subscription ended with %q, want %q", slow.EndReason(), TaskSubscriptionLagged)
	}
	if received != 256 {
		t.Fatalf("slow subscriber received %d events, want its complete buffered prefix of 256", received)
	}

	replay, live := task.Subscribe()
	if len(replay) != 257 {
		t.Fatalf("replay length = %d, want all 257 events", len(replay))
	}
	close(finish)
	for range live.Events() {
	}
}

func TestTaskIgnoresEventsAfterFinish(t *testing.T) {
	task := NewTask(func(_ context.Context, _ *Task, emit func(agents.Event)) {
		emit(agents.Event{Type: "done"})
	})

	_, live := task.Subscribe()
	for range live.Events() {
	}

	task.emit(agents.Event{Type: "error"})
	task.Abort()
	replay, closed := task.Subscribe()
	if len(replay) != 1 || replay[0].Cursor != 1 || replay[0].Event.Type != "done" {
		t.Fatalf("replay after finish = %#v, want only the original done event", replay)
	}
	if status := task.Status(); status != TaskDone {
		t.Fatalf("status after late emit and abort = %q, want %q", status, TaskDone)
	}
	if _, ok := <-closed.Events(); ok {
		t.Fatal("finished task returned an open subscription")
	}
	if closed.EndReason() != TaskSubscriptionTaskFinished {
		t.Fatalf("finished subscription ended with %q", closed.EndReason())
	}
}

func TestTaskDonePublishesCanceledLifecycle(t *testing.T) {
	started := make(chan context.Context, 1)
	task := NewTask(func(ctx context.Context, _ *Task, _ func(agents.Event)) {
		started <- ctx
	})
	ctx := <-started
	<-task.Done()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("Task.Done closed before the Task-owned context was canceled")
	}
}

func TestTaskStartFailurePublishesCanceledLifecycle(t *testing.T) {
	task, err := NewDeferredRegisteredTask(nil)
	if err != nil {
		t.Fatalf("create deferred Task: %v", err)
	}
	ctx := task.ctx
	task.failBeforeStart(errors.New("rejected"))
	<-task.Done()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("failed Task published Done before canceling its lifecycle")
	}
}

func TestDetachedTaskRetainsRequestIDWithoutCallerCancellation(t *testing.T) {
	caller, cancelCaller := context.WithCancel(observability.WithRequestID(context.Background(), "request-123"))
	task, err := NewDeferredRegisteredTaskWithContext(caller, nil)
	if err != nil {
		t.Fatal(err)
	}
	cancelCaller()

	observed := make(chan context.Context, 1)
	release := make(chan struct{})
	if err := task.Start(func(ctx context.Context, _ *Task, _ func(agents.Event)) {
		observed <- ctx
		<-release
	}); err != nil {
		t.Fatal(err)
	}
	taskContext := <-observed
	if got := observability.RequestID(taskContext); got != "request-123" {
		t.Fatalf("detached Task request_id = %q, want request-123", got)
	}
	if err := taskContext.Err(); err != nil {
		t.Fatalf("caller cancellation leaked into accepted Task: %v", err)
	}
	close(release)
	<-task.Done()
}

func TestTaskAbortRequestDoesNotOverrideLaterDurableCompletion(t *testing.T) {
	settle := make(chan struct{})
	task := NewTask(func(ctx context.Context, _ *Task, emit func(agents.Event)) {
		<-ctx.Done()
		<-settle
		emit(agents.Event{Type: "done"})
	})

	task.Abort()
	snapshot := task.Snapshot()
	if !snapshot.CancelRequested || snapshot.Finished || snapshot.Status != TaskRunning {
		t.Fatalf("abort request must not project a terminal outcome before settlement: %#v", snapshot)
	}
	close(settle)
	<-task.Done()
	snapshot = task.Snapshot()
	if !snapshot.CancelRequested || !snapshot.Finished || snapshot.Status != TaskDone {
		t.Fatalf("durable completion must win over a late cancel request: %#v", snapshot)
	}
}

func TestTaskAcceptedAbortSettlesFromAbortedEvent(t *testing.T) {
	task := NewTask(func(ctx context.Context, _ *Task, emit func(agents.Event)) {
		<-ctx.Done()
		emit(agents.Event{Type: "aborted"})
	})

	task.Abort()
	<-task.Done()
	snapshot := task.Snapshot()
	if !snapshot.CancelRequested || !snapshot.Finished || snapshot.Status != TaskAborted {
		t.Fatalf("accepted abort did not settle atomically from its terminal event: %#v", snapshot)
	}
}

func TestTaskPanicEmitsErrorAndSettlesAsTaskError(t *testing.T) {
	task := NewTask(func(context.Context, *Task, func(agents.Event)) {
		panic("worker exploded")
	})
	<-task.Done()

	snapshot := task.Snapshot()
	if !snapshot.Finished || snapshot.Status != TaskError {
		t.Fatalf("panic settlement = %#v, want finished TaskError", snapshot)
	}
	replay, subscription, err := task.SubscribeAfter(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay) != 1 || replay[0].Event.Type != "error" {
		t.Fatalf("panic replay = %#v, want one error event", replay)
	}
	data, ok := taskDisplayDataMap(replay[0].Event.Data)
	if !ok || data["message"] == "" {
		t.Fatalf("panic error event has no user-visible message: %#v", replay[0].Event.Data)
	}
	if _, open := <-subscription.Events(); open || subscription.EndReason() != TaskSubscriptionTaskFinished {
		t.Fatalf("panic subscription remained open or had wrong reason: %q", subscription.EndReason())
	}
}

func TestTaskIDsAreRestartScopedAndOpaque(t *testing.T) {
	first := newTaskID()
	second := newTaskID()
	if first == second {
		t.Fatalf("task IDs must be unique within one process: %q", first)
	}
	if !strings.HasPrefix(first, "task-"+taskProcessNonce+"-") || !strings.HasPrefix(second, "task-"+taskProcessNonce+"-") {
		t.Fatalf("task IDs do not carry the process namespace: first=%q second=%q", first, second)
	}
	otherBoot := "00000000000000000000000000000000"
	if otherBoot == taskProcessNonce {
		otherBoot = "ffffffffffffffffffffffffffffffff"
	}
	stale := strings.Replace(first, taskProcessNonce, otherBoot, 1)
	if stale == first || strings.HasPrefix(stale, "task-"+taskProcessNonce+"-") {
		t.Fatalf("a previous process namespace could alias the current task: stale=%q current=%q", stale, first)
	}
}

func TestTaskSubscribeAfterResumesAtExactDisplayCursor(t *testing.T) {
	release := make(chan struct{})
	emitted := make(chan struct{})
	task := NewTask(func(_ context.Context, _ *Task, emit func(agents.Event)) {
		emit(agents.Event{Type: "chunk", Data: "one"})
		emit(agents.Event{Type: "chunk", Data: "two"})
		close(emitted)
		<-release
	})

	<-emitted
	replay, subscription, err := task.SubscribeAfter(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay) != 1 || replay[0].Cursor != 2 || replay[0].Event.Data != "two" {
		t.Fatalf("resumed replay = %#v", replay)
	}
	if _, _, err := task.SubscribeAfter(3); !errors.Is(err, ErrTaskCursorAhead) {
		t.Fatalf("ahead cursor error = %v, want ErrTaskCursorAhead", err)
	}
	close(release)
	for range subscription.Events() {
	}
}

func TestTaskRetentionBoundsMemoryAndRejectsExpiredDisplayCursor(t *testing.T) {
	task, err := NewDeferredRegisteredTask(nil)
	if err != nil {
		t.Fatal(err)
	}
	task.retainedEventLimit = 3
	task.retainedByteLimit = 1 << 20
	for index := 1; index <= 5; index++ {
		task.emit(agents.Event{Type: "chunk", Data: fmt.Sprintf("chunk-%d", index)})
	}

	if got := task.Cursor(); got != 5 {
		t.Fatalf("latest cursor = %d, want 5", got)
	}
	if got := len(task.events); got != 3 {
		t.Fatalf("retained events = %d, want 3", got)
	}
	if _, _, err := task.SubscribeAfter(1); !errors.Is(err, ErrTaskCursorExpired) {
		t.Fatalf("expired cursor error = %v, want ErrTaskCursorExpired", err)
	}
	replay, subscription, err := task.SubscribeAfter(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay) != 3 || replay[0].Cursor != 3 || replay[2].Cursor != 5 {
		t.Fatalf("retained replay = %#v", replay)
	}
	task.failBeforeStart(errors.New("test complete"))
	for range subscription.Events() {
	}
}

func TestTaskRetentionDoesNotKeepOneEventLargerThanByteBudget(t *testing.T) {
	task, err := NewDeferredRegisteredTask(nil)
	if err != nil {
		t.Fatal(err)
	}
	task.retainedEventLimit = 3
	task.retainedByteLimit = 32
	task.emit(agents.Event{Type: "chunk", Data: strings.Repeat("x", 128)})

	if got := len(task.events); got != 0 {
		t.Fatalf("oversized replay event retained = %d, want 0", got)
	}
	if task.retainedBytes != 0 {
		t.Fatalf("retained bytes = %d, want 0", task.retainedBytes)
	}
	if _, _, err := task.SubscribeAfter(0); !errors.Is(err, ErrTaskCursorExpired) {
		t.Fatalf("oversized event cursor error = %v, want ErrTaskCursorExpired", err)
	}
	if replay, subscription, err := task.SubscribeAfter(1); err != nil || len(replay) != 0 {
		t.Fatalf("latest cursor replay = %#v err=%v, want empty replay", replay, err)
	} else {
		task.failBeforeStart(errors.New("test complete"))
		for range subscription.Events() {
		}
	}
}

func TestTaskDisplayCheckpointRecoversActiveTaskAfterRawRetentionOverflow(t *testing.T) {
	task, err := NewDeferredRegisteredTask(nil)
	if err != nil {
		t.Fatal(err)
	}
	task.retainedEventLimit = 3
	task.retainedByteLimit = 1 << 20
	task.emit(agents.Event{Type: "agent_cycle_started", Data: map[string]any{
		"command_id": "command-1", "delivery": "start_turn", "message": "继续写", "operation_id": "operation-1", "cycle": 1,
	}})
	for _, fragment := range []string{"一", "段", "完整", "思考", "。"} {
		task.emit(agents.Event{Type: "thinking", Data: map[string]any{"content": fragment, "run_id": "run-1"}})
	}

	replay, subscription, err := task.SubscribeDisplayAfter(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay.Events) != 0 || replay.Checkpoint == nil {
		t.Fatalf("overflow replay = %#v, want checkpoint only", replay)
	}
	checkpoint := replay.Checkpoint
	if checkpoint.Cursor != 6 || !checkpoint.Complete || checkpoint.Settled || len(checkpoint.Events) != 2 {
		t.Fatalf("checkpoint metadata/events = %#v", checkpoint)
	}
	thinking, ok := taskDisplayDataMap(checkpoint.Events[1].Data)
	if !ok || thinking["content"] != "一段完整思考。" {
		t.Fatalf("checkpoint thinking = %#v, want complete semantic content", checkpoint.Events[1].Data)
	}

	task.emit(agents.Event{Type: "chunk", Data: map[string]any{"content": "正文", "run_id": "run-1"}})
	live := <-subscription.Events()
	if live.Cursor != 7 || live.Event.Type != "chunk" {
		t.Fatalf("live event after checkpoint = %#v", live)
	}
	task.failBeforeStart(errors.New("test complete"))
	for range subscription.Events() {
	}
}

func TestTaskDisplayCheckpointReplaysFinishedTaskAndAssemblesToolArguments(t *testing.T) {
	task, err := NewDeferredRegisteredTask(nil)
	if err != nil {
		t.Fatal(err)
	}
	task.retainedEventLimit = 4
	task.retainedByteLimit = 1 << 20
	if err := task.Start(func(_ context.Context, _ *Task, emit func(agents.Event)) {
		emit(agents.Event{Type: "agent_cycle_started", Data: map[string]any{
			"command_id": "command-1", "delivery": "start_turn", "message": "读取", "operation_id": "operation-1", "cycle": 1,
		}})
		emit(agents.Event{Type: "tool_call", Data: map[string]any{"id": "call-1", "name": "read", "args": "", "run_id": "run-1"}})
		for _, delta := range []string{`{"path"`, `:"chapter.md"`, `}`} {
			// Some providers omit the repeated tool name on delta frames.
			emit(agents.Event{Type: "tool_args_delta", Data: map[string]any{"id": "call-1", "delta": delta, "run_id": "run-1"}})
		}
		emit(agents.Event{Type: "done", Data: map[string]any{}})
	}); err != nil {
		t.Fatal(err)
	}
	<-task.Done()

	replay, subscription, err := task.SubscribeDisplayAfter(0)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Checkpoint == nil || !replay.Checkpoint.Complete || !replay.Checkpoint.Settled || replay.Checkpoint.Cursor != 6 {
		t.Fatalf("finished checkpoint = %#v", replay.Checkpoint)
	}
	if len(replay.Checkpoint.Events) != 3 {
		t.Fatalf("finished checkpoint events = %#v", replay.Checkpoint.Events)
	}
	tool, ok := taskDisplayDataMap(replay.Checkpoint.Events[1].Data)
	if !ok || tool["args"] != `{"path":"chapter.md"}` {
		t.Fatalf("assembled tool call = %#v", replay.Checkpoint.Events[1].Data)
	}
	if replay.Checkpoint.Events[2].Type != "done" {
		t.Fatalf("checkpoint terminal event = %#v", replay.Checkpoint.Events[2])
	}
	if _, open := <-subscription.Events(); open || subscription.EndReason() != TaskSubscriptionTaskFinished {
		t.Fatalf("finished checkpoint returned live subscription: %q", subscription.EndReason())
	}
}

func TestTaskDisplayCheckpointMarksWholeEventOmissionIncomplete(t *testing.T) {
	task, err := NewDeferredRegisteredTask(nil)
	if err != nil {
		t.Fatal(err)
	}
	task.retainedEventLimit = 1
	task.retainedByteLimit = 1 << 20
	task.emit(agents.Event{Type: "agent_cycle_started", Data: map[string]any{
		"command_id": "command-1", "delivery": "start_turn", "message": "继续", "operation_id": "operation-1", "cycle": 1,
	}})
	task.emit(agents.Event{Type: "thinking", Data: map[string]any{"content": "这段思考只能完整保留或完整省略"}})

	replay, subscription, err := task.SubscribeDisplayAfter(0)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Checkpoint == nil || replay.Checkpoint.Complete || replay.Checkpoint.Settled || replay.Checkpoint.Cursor != 2 {
		t.Fatalf("bounded checkpoint = %#v, want explicit incomplete cursor 2", replay.Checkpoint)
	}
	if len(replay.Checkpoint.Events) != 1 || replay.Checkpoint.Events[0].Type != "agent_cycle_started" {
		t.Fatalf("checkpoint sliced or partially retained semantic output: %#v", replay.Checkpoint.Events)
	}
	task.failBeforeStart(errors.New("test complete"))
	for range subscription.Events() {
	}
}

func TestTaskDisplayCheckpointKeepsPersistenceBarrierAfterCycleAnchorEviction(t *testing.T) {
	task, err := NewDeferredRegisteredTask(nil)
	if err != nil {
		t.Fatal(err)
	}
	task.retainedEventLimit = 1
	task.retainedByteLimit = 64
	task.emit(agents.Event{Type: "agent_cycle_started", Data: map[string]any{
		"command_id": "command-1", "delivery": "start_turn", "message": strings.Repeat("x", 256), "operation_id": "operation-1", "cycle": 1,
	}})

	replay, subscription, err := task.SubscribeDisplayAfter(0)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Checkpoint == nil || replay.Checkpoint.Complete || !replay.Checkpoint.PersistenceRequired || len(replay.Checkpoint.Events) != 0 {
		t.Fatalf("evicted cycle anchor checkpoint = %#v", replay.Checkpoint)
	}
	task.Unsubscribe(subscription)

	task.emit(agents.Event{Type: "interactive_turn_persisted", Data: map[string]any{"turn_id": "turn-1"}})
	replay, subscription, err = task.SubscribeDisplayAfter(0)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Checkpoint == nil || replay.Checkpoint.PersistenceRequired {
		t.Fatalf("persisted cycle retained stale checkpoint barrier = %#v", replay.Checkpoint)
	}
	task.failBeforeStart(errors.New("test complete"))
	for range subscription.Events() {
	}
}

func TestTaskDisplayCheckpointKeepsTerminalOutcomeAfterSettledReplayEviction(t *testing.T) {
	tests := []struct {
		name       string
		event      agents.Event
		wantStatus TaskStatus
		wantReason string
	}{
		{
			name:       "error",
			event:      agents.Event{Type: "error", Data: map[string]string{"message": "provider failed after acceptance"}},
			wantStatus: TaskError,
			wantReason: "provider failed after acceptance",
		},
		{
			name:       "aborted",
			event:      agents.Event{Type: "aborted", Data: map[string]string{"reason": "user_requested"}},
			wantStatus: TaskAborted,
			wantReason: "user_requested",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			task := NewTask(func(_ context.Context, _ *Task, emit func(agents.Event)) {
				emit(test.event)
			})
			<-task.Done()
			if released := task.releaseDisplayReplay(); released == 0 {
				t.Fatal("fixture did not release settled display history")
			}

			replay, subscription, err := task.SubscribeDisplayAfter(0)
			if err != nil {
				t.Fatal(err)
			}
			if replay.Checkpoint == nil || replay.Checkpoint.Complete || !replay.Checkpoint.Settled {
				t.Fatalf("evicted settled checkpoint = %#v", replay.Checkpoint)
			}
			if replay.Checkpoint.Status != test.wantStatus || replay.Checkpoint.TerminalReason != test.wantReason {
				t.Fatalf("evicted terminal outcome = status:%q reason:%q, want status:%q reason:%q", replay.Checkpoint.Status, replay.Checkpoint.TerminalReason, test.wantStatus, test.wantReason)
			}
			if _, open := <-subscription.Events(); open || subscription.EndReason() != TaskSubscriptionTaskFinished {
				t.Fatalf("evicted settled Task returned a live subscription: %q", subscription.EndReason())
			}
		})
	}
}

func TestNewRegisteredTaskPublishesBeforeRun(t *testing.T) {
	var registered *Task
	ran := make(chan struct{})
	task, err := NewRegisteredTask(func(task *Task) error {
		registered = task
		return nil
	}, func(_ context.Context, task *Task, _ func(agents.Event)) {
		if registered != task {
			t.Errorf("task ran before registration")
		}
		close(ran)
	})
	if err != nil {
		t.Fatal(err)
	}
	<-ran
	<-task.Done()
	if registered != task {
		t.Fatal("registered a different task")
	}
}
