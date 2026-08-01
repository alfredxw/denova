package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	agents "denova/internal/agents"
)

// TaskStatus 表示后台任务的执行状态。
type TaskStatus string

const (
	TaskRunning TaskStatus = "running"
	TaskDone    TaskStatus = "done"
	TaskAborted TaskStatus = "aborted"
	TaskError   TaskStatus = "error"
)

var (
	taskSeq          atomic.Uint64
	taskProcessNonce = newTaskProcessNonce()
)

// ErrTaskCursorAhead means a reconnect cursor points beyond the latest event
// admitted by the task. Treating it as an explicit protocol error avoids
// silently attaching a client to an unrelated or reset stream.
var ErrTaskCursorAhead = errors.New("task event cursor is ahead of the stream")

// TaskStateSnapshot is one internally consistent read of the public Task
// identity and stream state. Active endpoints must not assemble these fields
// through separate locks because finish may occur between reads.
type TaskStateSnapshot struct {
	ID                      string
	Status                  TaskStatus
	TerminalReason          string
	TerminalReasonTruncated bool
	Finished                bool
	CancelRequested         bool
	Cursor                  uint64
}

// Task 表示一个后台运行的 Agent 任务，独立于 HTTP 连接生命周期。
// 事件缓冲到内存，SSE 客户端作为订阅者消费事件。
type Task struct {
	id        string
	startedAt time.Time
	mu        sync.Mutex
	ctx       context.Context
	status    TaskStatus
	// terminalReason is bounded semantic settlement state. Unlike replay
	// buffers, it survives display eviction so reconnects can distinguish an
	// evicted failure/abort from normal completion.
	terminalReason          string
	terminalReasonTruncated bool
	started                 bool
	finished                bool
	// cancelRequested records caller intent only. Durable terminal events remain
	// authoritative for the public status, so a commit that already won can
	// still settle this Task as done.
	cancelRequested    bool
	events             []TaskEvent
	eventBytes         []int
	retainedBytes      int
	eventBaseCursor    uint64
	nextCursor         uint64
	retainedEventLimit int
	retainedByteLimit  int
	checkpointEvents   []agents.Event
	checkpointBytes    []int
	checkpointSize     int
	checkpointCursor   uint64
	checkpointComplete bool
	// gameTurnPersistenceRequired is semantic Task state, not a projection
	// detail. It must survive checkpoint event eviction so Game reconnects do not
	// treat an unpersisted Agent cycle as a completed structural operation.
	gameTurnPersistenceRequired bool
	subs                        []*TaskSubscription
	cancel                      context.CancelFunc
	done                        chan struct{}
}

// NewTask 创建并启动后台任务。run 函数在独立 goroutine 中执行。
func NewTask(run func(ctx context.Context, task *Task, emit func(agents.Event))) *Task {
	task, err := NewRegisteredTask(nil, run)
	if err != nil {
		panic(err)
	}
	return task
}

// NewRegisteredTask atomically publishes a fully initialized Task through
// register before its goroutine may run. This closes the historical window in
// which a fast task could emit, finish, or receive a command before App had
// bound it to the matching workspace/session/story.
func NewRegisteredTask(register func(*Task) error, run func(ctx context.Context, task *Task, emit func(agents.Event))) (*Task, error) {
	return NewRegisteredTaskWithContext(context.Background(), register, run)
}

// NewRegisteredTaskWithContext preserves correlation values from the request
// that created the detached task without coupling its lifetime to that request.
func NewRegisteredTaskWithContext(ctx context.Context, register func(*Task) error, run func(ctx context.Context, task *Task, emit func(agents.Event))) (*Task, error) {
	t, err := NewDeferredRegisteredTaskWithContext(ctx, register)
	if err != nil {
		return nil, err
	}
	if err := t.Start(run); err != nil {
		t.failBeforeStart(err)
		return nil, err
	}
	return t, nil
}

// NewDeferredRegisteredTask reserves a fully initialized display Task without
// starting its worker. Root Agent API paths use the task as an event sink while
// synchronously crossing durable StartTurn acceptance, then call Start only
// after a Receipt exists.
func NewDeferredRegisteredTask(register func(*Task) error) (*Task, error) {
	return NewDeferredRegisteredTaskWithContext(context.Background(), register)
}

// NewDeferredRegisteredTaskWithContext creates a detached task while retaining
// context values such as request_id. Cancellation remains Task-owned because
// accepted Agent work intentionally outlives the initiating HTTP connection.
func NewDeferredRegisteredTaskWithContext(ctx context.Context, register func(*Task) error) (*Task, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	t := &Task{
		id:                 newTaskID(),
		startedAt:          time.Now(),
		ctx:                ctx,
		status:             TaskRunning,
		retainedEventLimit: defaultTaskRetainedEventLimit,
		retainedByteLimit:  defaultTaskRetainedByteLimit,
		cancel:             cancel,
		done:               make(chan struct{}),
	}
	if register != nil {
		if err := register(t); err != nil {
			cancel()
			t.mu.Lock()
			t.status = TaskError
			t.terminalReason, t.terminalReasonTruncated = boundedTaskTerminalReason(err.Error())
			t.finished = true
			close(t.done)
			t.mu.Unlock()
			return nil, err
		}
	}
	return t, nil
}

// Start launches the Task worker exactly once.
func (t *Task) Start(run func(ctx context.Context, task *Task, emit func(agents.Event))) error {
	if t == nil {
		return fmt.Errorf("task is nil")
	}
	t.mu.Lock()
	if t.started || t.finished {
		t.mu.Unlock()
		return fmt.Errorf("task %s is already started or finished", t.id)
	}
	t.started = true
	ctx := t.ctx
	t.mu.Unlock()
	slog.LogAttrs(ctx, slog.LevelInfo, "task_start", slog.String("component", "agent-task"), slog.String("task_id", t.id))
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.LogAttrs(ctx, slog.LevelError, "task_panic_recovered", slog.String("component", "agent-task"), slog.String("task_id", t.id), slog.Any("error", recovered))
				t.emit(agents.Event{Type: "error", Data: map[string]string{"message": "Agent 后台任务异常中断 / Agent background task stopped unexpectedly"}})
			}
			t.finish()
		}()
		if run != nil {
			run(ctx, t, t.emit)
		}
	}()
	return nil
}

func (t *Task) failBeforeStart(err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.started || t.finished {
		t.mu.Unlock()
		return
	}
	t.status = TaskError
	t.terminalReason, t.terminalReasonTruncated = boundedTaskTerminalReason(err.Error())
	t.finished = true
	for _, sub := range t.subs {
		sub.close(TaskSubscriptionTaskFinished)
	}
	t.subs = nil
	// This cancel is created by NewDeferredRegisteredTask, so it has no
	// user-defined blocking implementation. Cancel before Done becomes visible:
	// observers may then rely on terminal Task state and lifecycle state agreeing.
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	close(t.done)
	t.mu.Unlock()
	slog.LogAttrs(t.ctx, slog.LevelError, "task_start_failed", slog.String("component", "agent-task"), slog.String("task_id", t.id), slog.Any("error", err))
}

// taskAcceptanceContext is canceled by either the synchronous API caller or a
// structural abort of the already-registered display task. After StartTurn is
// accepted, the Task worker switches to its own background lifetime.
func taskAcceptanceContext(caller context.Context, task *Task) (context.Context, func()) {
	if caller == nil {
		caller = context.Background()
	}
	ctx, cancel := context.WithCancel(caller)
	if task == nil || task.ctx == nil {
		return ctx, cancel
	}
	stop := context.AfterFunc(task.ctx, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

// newTaskID remains opaque to clients and cannot be reused after a process
// restart. Exact task IDs are the authority for reconnecting a display stream,
// so a process-local counter alone would let a stale browser attach a new run.
func newTaskID() string {
	return "task-" + taskProcessNonce + "-" + strconv.FormatUint(taskSeq.Add(1), 36)
}

func newTaskProcessNonce() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err == nil {
		return hex.EncodeToString(raw[:])
	}
	// crypto/rand failure must not prevent local Agent use. PID plus wall-clock
	// nanoseconds still gives the reconnect identity a restart-scoped namespace.
	return strings.Join([]string{
		strconv.FormatInt(time.Now().UTC().UnixNano(), 36),
		strconv.Itoa(os.Getpid()),
	}, "-")
}

// emit 缓冲事件并广播给所有订阅者。
//
// 广播与订阅者关闭必须在同一把锁内完成，否则 finish 可能在发送前关闭
// channel，导致后台 goroutine 因 send-on-closed-channel 再次 panic。慢订阅者
// 会被断开，使客户端通过重连和事件快照恢复；继续静默丢单个事件会让客户端
// 保持连接却得到不可恢复的残缺事件流。
func (t *Task) emit(ev agents.Event) {
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		slog.LogAttrs(t.ctx, slog.LevelWarn, "task_event_ignored", slog.String("component", "agent-task"), slog.String("task_id", t.id), slog.String("event_type", ev.Type), slog.String("reason", "task_finished"))
		return
	}
	item := t.appendRetainedEventLocked(ev)
	t.projectDisplayCheckpointLocked(item)
	if t.status == TaskRunning {
		switch ev.Type {
		case "done":
			t.status = TaskDone
		case "error":
			t.status = TaskError
		case "aborted":
			t.status = TaskAborted
		}
		if t.status != TaskRunning {
			t.terminalReason, t.terminalReasonTruncated = taskTerminalReason(ev)
		}
	}
	eventCount := int(t.nextCursor)
	laggedSubscribers := 0
	remainingSubscribers := t.subs[:0]
	for _, sub := range t.subs {
		select {
		case sub.events <- item:
			remainingSubscribers = append(remainingSubscribers, sub)
		default:
			sub.close(TaskSubscriptionLagged)
			laggedSubscribers++
		}
	}
	t.subs = remainingSubscribers
	subCount := len(remainingSubscribers)
	t.mu.Unlock()

	if shouldLogEvent(ev.Type, eventCount) {
		slog.LogAttrs(t.ctx, slog.LevelInfo, "task_event", slog.String("component", "agent-task"), slog.String("task_id", t.id), slog.String("event_type", ev.Type), slog.Int("events", eventCount), slog.Int("subscribers", subCount))
	}
	if laggedSubscribers > 0 {
		slog.LogAttrs(t.ctx, slog.LevelWarn, "task_subscriber_disconnected", slog.String("component", "agent-task"), slog.String("task_id", t.id), slog.String("event_type", ev.Type), slog.String("reason", "subscriber_slow"), slog.Int("disconnected", laggedSubscribers), slog.Int("subscribers", subCount))
	}
}

// finish 标记任务完成，关闭所有订阅者 channel。
func (t *Task) finish() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}
	if t.status == TaskRunning {
		if t.cancelRequested {
			t.status = TaskAborted
		} else {
			t.status = TaskDone
		}
	}
	t.finished = true
	for _, ch := range t.subs {
		ch.close(TaskSubscriptionTaskFinished)
	}
	t.subs = nil
	// This is the Task-owned standard-library cancel. Calling it while the Task
	// lock is held is non-blocking, and lets Done publish one atomic terminal
	// boundary where child contexts are already canceled.
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	close(t.done)
	slog.LogAttrs(t.ctx, slog.LevelInfo, "task_finish", slog.String("component", "agent-task"), slog.String("task_id", t.id), slog.String("status", string(t.status)), slog.Uint64("events", t.nextCursor), slog.Int("retained_events", len(t.events)), slog.Duration("duration", time.Since(t.startedAt).Round(time.Millisecond)))
}

// Abort 取消任务执行。
func (t *Task) Abort() {
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		return
	}
	t.cancelRequested = true
	cancel := t.cancel
	t.mu.Unlock()
	slog.LogAttrs(t.ctx, slog.LevelWarn, "task_abort_requested", slog.String("component", "agent-task"), slog.String("task_id", t.id))
	if cancel != nil {
		cancel()
	}
}

// Status 返回当前状态。
func (t *Task) Status() TaskStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// Finished reports whether the task goroutine has fully exited.
func (t *Task) Finished() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.finished
}

// Done is closed only after the task goroutine has fully exited and all live
// subscriptions have received their terminal reason.
func (t *Task) Done() <-chan struct{} {
	return t.done
}

// Cursor returns the latest task-local display cursor.
func (t *Task) Cursor() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.nextCursor
}

// EarliestCursor returns the first display cursor still available for replay.
// It is one past Cursor when the task has not emitted any events.
func (t *Task) EarliestCursor() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.eventBaseCursor + 1
}

// Snapshot reads status, settlement, and cursor under one Task lock.
func (t *Task) Snapshot() TaskStateSnapshot {
	if t == nil {
		return TaskStateSnapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return TaskStateSnapshot{
		ID:                      t.id,
		Status:                  t.status,
		TerminalReason:          t.terminalReason,
		TerminalReasonTruncated: t.terminalReasonTruncated,
		Finished:                t.finished,
		CancelRequested:         t.cancelRequested,
		Cursor:                  t.nextCursor,
	}
}

// ID 返回任务编号，用于关联后端日志。
func (t *Task) ID() string {
	return t.id
}

func shouldLogEvent(eventType string, eventCount int) bool {
	switch eventType {
	case "chunk", "thinking", "tool_args_delta":
		return eventCount == 1 || eventCount%100 == 0
	default:
		return true
	}
}

// Keep one terminal diagnostic after replay eviction without letting an
// arbitrary provider error turn each retained Task identity into an unbounded
// allocation. The durable runtime uses the same terminal-reason ceiling.
const maxTaskTerminalReasonBytes = 16 << 10

func taskTerminalReason(event agents.Event) (string, bool) {
	data, ok := taskDisplayDataMap(event.Data)
	if !ok {
		return "", false
	}
	keys := []string{"message", "error", "reason"}
	if event.Type == "aborted" {
		keys = []string{"reason", "message", "error"}
	}
	for _, key := range keys {
		value, ok := data[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		return boundedTaskTerminalReason(value)
	}
	return "", false
}

func boundedTaskTerminalReason(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) <= maxTaskTerminalReasonBytes {
		return value, false
	}
	end := 0
	for index := range value {
		if index > maxTaskTerminalReasonBytes {
			break
		}
		end = index
	}
	return strings.TrimSpace(value[:end]), true
}
