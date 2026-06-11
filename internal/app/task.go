package app

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"nova/internal/agent"
	"nova/internal/observability"
)

// TaskStatus 表示后台任务的执行状态。
type TaskStatus string

const (
	TaskRunning TaskStatus = "running"
	TaskDone    TaskStatus = "done"
	TaskAborted TaskStatus = "aborted"
	TaskError   TaskStatus = "error"
)

var taskSeq atomic.Uint64

// Task 表示一个后台运行的 Agent 任务，独立于 HTTP 连接生命周期。
// 事件缓冲到内存，SSE 客户端作为订阅者消费事件。
type Task struct {
	id        string
	startedAt time.Time
	mu        sync.Mutex
	status    TaskStatus
	events    []agent.Event
	subs      []chan agent.Event
	cancel    context.CancelFunc
}

// NewTask 创建并启动后台任务。run 函数在独立 goroutine 中执行。
func NewTask(run func(ctx context.Context, task *Task, emit func(agent.Event))) *Task {
	ctx, cancel := context.WithCancel(context.Background())
	t := &Task{
		id:        strconv.FormatUint(taskSeq.Add(1), 10),
		startedAt: time.Now(),
		status:    TaskRunning,
		cancel:    cancel,
	}
	observability.Info("agent-task", "task_start", slog.String("task_id", t.id))
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				observability.Error("agent-task", "task_panic_recovered", slog.String("task_id", t.id), slog.Any("error", recovered))
				t.emit(agent.Event{Type: "error", Data: map[string]string{"message": "Agent 后台任务异常中断"}})
			}
			t.finish()
		}()
		run(ctx, t, t.emit)
	}()
	return t
}

// emit 缓冲事件并广播给所有订阅者。
func (t *Task) emit(ev agent.Event) {
	t.mu.Lock()
	t.events = append(t.events, ev)
	if ev.Type == "error" {
		t.status = TaskError
	}
	if ev.Type == "aborted" {
		t.status = TaskAborted
	}
	subs := append([]chan agent.Event(nil), t.subs...)
	eventCount := len(t.events)
	subCount := len(t.subs)
	t.mu.Unlock()
	if shouldLogEvent(ev.Type, eventCount) {
		observability.Info("agent-task", "task_event", slog.String("task_id", t.id), slog.String("event_type", ev.Type), slog.Int("events", eventCount), slog.Int("subscribers", subCount))
	}
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
			// 订阅者消费太慢，丢弃（SSE 是尽力送达）
			observability.Warn("agent-task", "task_event_dropped", slog.String("task_id", t.id), slog.String("event_type", ev.Type), slog.String("reason", "subscriber_slow"))
		}
	}
}

// finish 标记任务完成，关闭所有订阅者 channel。
func (t *Task) finish() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.status == TaskRunning {
		t.status = TaskDone
	}
	for _, ch := range t.subs {
		close(ch)
	}
	t.subs = nil
	observability.Info("agent-task", "task_finish", slog.String("task_id", t.id), slog.String("status", string(t.status)), slog.Int("events", len(t.events)), slog.Duration("duration", time.Since(t.startedAt).Round(time.Millisecond)))
}

// Subscribe 返回已有事件的快照和一个用于接收后续事件的 channel。
// 如果任务已结束，channel 立即关闭。
func (t *Task) Subscribe() ([]agent.Event, <-chan agent.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()

	snapshot := make([]agent.Event, len(t.events))
	copy(snapshot, t.events)

	if t.status != TaskRunning {
		ch := make(chan agent.Event)
		close(ch)
		observability.Info("agent-task", "task_subscribe", slog.String("task_id", t.id), slog.String("status", string(t.status)), slog.Int("replay", len(snapshot)), slog.Bool("live", false))
		return snapshot, ch
	}

	ch := make(chan agent.Event, 256)
	t.subs = append(t.subs, ch)
	observability.Info("agent-task", "task_subscribe", slog.String("task_id", t.id), slog.String("status", string(t.status)), slog.Int("replay", len(snapshot)), slog.Int("subscribers", len(t.subs)), slog.Bool("live", true))
	return snapshot, ch
}

// Unsubscribe 移除订阅者。
func (t *Task) Unsubscribe(ch <-chan agent.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, sub := range t.subs {
		if sub == ch {
			t.subs = append(t.subs[:i], t.subs[i+1:]...)
			observability.Info("agent-task", "task_unsubscribe", slog.String("task_id", t.id), slog.Int("subscribers", len(t.subs)))
			return
		}
	}
}

// Abort 取消任务执行。
func (t *Task) Abort() {
	t.mu.Lock()
	t.status = TaskAborted
	t.mu.Unlock()
	observability.Warn("agent-task", "task_abort", slog.String("task_id", t.id))
	t.cancel()
}

// Status 返回当前状态。
func (t *Task) Status() TaskStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
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
