package task

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"denova/internal/agents/run"
)

func (t *Task) Emit(ev agentrun.Event) {
	t.mu.Lock()
	if t.finished {
		t.mu.Unlock()
		slog.LogAttrs(t.ctx, slog.LevelWarn, "task_event_ignored", slog.String("component", "agent-task"), slog.String("task_id", t.id), slog.String("event_type", ev.Type), slog.String("reason", "task_finished"))
		return
	}
	item := t.appendRetainedEventLocked(ev)
	t.projectDisplayCheckpointLocked(item)
	if t.status == Running {
		switch ev.Type {
		case "done":
			t.status = Done
		case "error":
			t.status = Failed
		case "aborted":
			t.status = Aborted
		}
		if t.status != Running {
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
			sub.close(SubscriptionLagged)
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
func (t *Task) Finish() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}
	if t.status == Running {
		if t.cancelRequested {
			t.status = Aborted
		} else {
			t.status = Done
		}
	}
	t.finished = true
	for _, ch := range t.subs {
		ch.close(SubscriptionTaskFinished)
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
func (t *Task) Status() Status {
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
func (t *Task) Snapshot() Snapshot {
	if t == nil {
		return Snapshot{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return Snapshot{
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

// Context is the detached lifetime owned by the Task. Callers may use it for
// logging and task-scoped work, but must not cancel it directly.
func (t *Task) Context() context.Context {
	if t == nil || t.ctx == nil {
		return context.Background()
	}
	return t.ctx
}

// BindLifetime replaces the provisional detached request context with the
// lifecycle context admitted by the task's owner. Registration callbacks call
// this before Start so background work keeps request correlation values while
// following Project/App shutdown instead of the initiating HTTP request.
func (t *Task) BindLifetime(ctx context.Context) error {
	if t == nil {
		return fmt.Errorf("task is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	t.mu.Lock()
	if t.started || t.finished {
		t.mu.Unlock()
		return fmt.Errorf("task %s is already started or finished", t.id)
	}
	previousCancel := t.cancel
	t.ctx, t.cancel = context.WithCancel(ctx)
	t.mu.Unlock()
	if previousCancel != nil {
		previousCancel()
	}
	return nil
}

// StartedAt returns the immutable admission time used to order competing
// reconnectable task projections.
func (t *Task) StartedAt() time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.startedAt
}

// ConfigureRetention overrides positive replay limits. It is primarily useful
// to bind a Task to an application-level replay budget before the worker starts.
func (t *Task) ConfigureRetention(eventLimit, byteLimit int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if eventLimit > 0 {
		t.retainedEventLimit = eventLimit
	}
	if byteLimit > 0 {
		t.retainedByteLimit = byteLimit
	}
	t.boundDisplayCheckpointLocked()
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

func taskTerminalReason(event agentrun.Event) (string, bool) {
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
