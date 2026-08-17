package task

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

	"denova/internal/agents/run"
)

// Status 表示后台任务的执行状态。
type Status string

const (
	Running Status = "running"
	Done    Status = "done"
	Aborted Status = "aborted"
	Failed  Status = "error"
)

var (
	taskSeq          atomic.Uint64
	taskProcessNonce = newTaskProcessNonce()
)

// ErrCursorAhead means a reconnect cursor points beyond the latest event
// admitted by the task. Treating it as an explicit protocol error avoids
// silently attaching a client to an unrelated or reset stream.
var ErrCursorAhead = errors.New("task event cursor is ahead of the stream")

// Snapshot is one internally consistent read of the public Task
// identity and stream state. Active endpoints must not assemble these fields
// through separate locks because finish may occur between reads.
type Snapshot struct {
	ID                      string
	Status                  Status
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
	status    Status
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
	events             []Event
	eventBytes         []int
	retainedBytes      int
	eventBaseCursor    uint64
	nextCursor         uint64
	retainedEventLimit int
	retainedByteLimit  int
	checkpointEvents   []agentrun.Event
	checkpointBytes    []int
	checkpointSize     int
	checkpointCursor   uint64
	checkpointComplete bool
	// gameTurnPersistenceRequired is semantic Task state, not a projection
	// detail. It must survive checkpoint event eviction so Game reconnects do not
	// treat an unpersisted Agent cycle as a completed structural operation.
	gameTurnPersistenceRequired bool
	subs                        []*Subscription
	cancel                      context.CancelFunc
	done                        chan struct{}
}

// New 创建并启动后台任务。run 函数在独立 goroutine 中执行。
func New(run func(ctx context.Context, task *Task, emit func(agentrun.Event))) *Task {
	task, err := NewRegistered(nil, run)
	if err != nil {
		panic(err)
	}
	return task
}

// NewRegistered atomically publishes a fully initialized Task through
// register before its goroutine may run. This closes the historical window in
// which a fast task could emit, finish, or receive a command before App had
// bound it to the matching workspace/session/story.
func NewRegistered(register func(*Task) error, run func(ctx context.Context, task *Task, emit func(agentrun.Event))) (*Task, error) {
	return NewRegisteredWithContext(context.Background(), register, run)
}

// NewRegisteredWithContext preserves correlation values from the request
// that created the detached task without coupling its lifetime to that request.
func NewRegisteredWithContext(ctx context.Context, register func(*Task) error, run func(ctx context.Context, task *Task, emit func(agentrun.Event))) (*Task, error) {
	t, err := NewDeferredWithContext(ctx, register)
	if err != nil {
		return nil, err
	}
	if err := t.Start(run); err != nil {
		t.RejectStart(err)
		return nil, err
	}
	return t, nil
}

// NewDeferred reserves a fully initialized display Task without
// starting its worker. Root Agent API paths use the task as an event sink while
// synchronously crossing durable StartTurn acceptance, then call Start only
// after a Receipt exists.
func NewDeferred(register func(*Task) error) (*Task, error) {
	return NewDeferredWithContext(context.Background(), register)
}

// NewDeferredWithContext creates a detached task while retaining
// context values such as request_id. Cancellation remains Task-owned because
// accepted Agent work intentionally outlives the initiating HTTP connection.
func NewDeferredWithContext(ctx context.Context, register func(*Task) error) (*Task, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	t := &Task{
		id:                 newTaskID(),
		startedAt:          time.Now(),
		ctx:                ctx,
		status:             Running,
		retainedEventLimit: DefaultRetainedEventLimit,
		retainedByteLimit:  DefaultRetainedByteLimit,
		cancel:             cancel,
		done:               make(chan struct{}),
	}
	if register != nil {
		if err := register(t); err != nil {
			cancel()
			t.mu.Lock()
			t.status = Failed
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
func (t *Task) Start(run func(ctx context.Context, task *Task, emit func(agentrun.Event))) error {
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
				t.Emit(agentrun.Event{Type: "error", Data: map[string]string{"message": "Agent 后台任务异常中断 / Agent background task stopped unexpectedly"}})
			}
			t.Finish()
		}()
		if run != nil {
			run(ctx, t, t.Emit)
		}
	}()
	return nil
}

func (t *Task) RejectStart(err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.started || t.finished {
		t.mu.Unlock()
		return
	}
	t.status = Failed
	message := "task start rejected"
	if err != nil {
		message = err.Error()
	}
	t.terminalReason, t.terminalReasonTruncated = boundedTaskTerminalReason(message)
	t.finished = true
	for _, sub := range t.subs {
		sub.close(SubscriptionTaskFinished)
	}
	t.subs = nil
	// This cancel is created by NewDeferred, so it has no
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

// AcceptanceContext is canceled by either the synchronous API caller or a
// structural abort of the already-registered display task. After StartTurn is
// accepted, the Task worker switches to its own background lifetime.
func AcceptanceContext(caller context.Context, task *Task) (context.Context, func()) {
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
