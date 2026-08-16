package sse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"

	"denova/internal/api/agentui"
	novaApp "denova/internal/app"
	apptask "denova/internal/app/task"
	"denova/internal/observability"
)

const (
	taskCheckpointEventType          = "task_checkpoint"
	taskCheckpointCommittedEventType = "task_checkpoint_committed"
	taskRehydrateRequiredEventType   = "task_rehydrate_required"
)

// StreamTask writes a Task event snapshot and live updates as Server-Sent Events.
func StreamTask(ctx context.Context, c *app.RequestContext, task *apptask.Task) {
	after, ok := requestedTaskCursor(c)
	if !ok {
		return
	}
	replay, subscription, err := task.SubscribeDisplayAfter(after)
	if err != nil {
		writeTaskCursorError(c, task, err)
		return
	}
	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.ImmediateHeaderFlush = true

	pr, pw := io.Pipe()

	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.ErrorContext(ctx, fmt.Sprintf("[agent-sse] stream panic recovered task_id=%s err=%v", task.ID(), recovered))
			}
			task.Unsubscribe(subscription)
			_ = pw.Close()
		}()
		slog.InfoContext(ctx, fmt.Sprintf("[agent-sse] stream start task_id=%s after=%d replay=%d checkpoint=%t", task.ID(), after, len(replay.Events), replay.Checkpoint != nil))
		writeSSE := newSSEWriteHandler(ctx, pw)

		if replay.Checkpoint != nil {
			committed, err := writeTaskCheckpoint(pw, *replay.Checkpoint, writeSSE)
			if err != nil {
				slog.InfoContext(ctx, fmt.Sprintf("[agent-sse] stream interrupted task_id=%s phase=checkpoint cursor=%d err=%v", task.ID(), replay.Checkpoint.Cursor, err))
				return
			}
			if !committed {
				slog.InfoContext(ctx, fmt.Sprintf("[agent-sse] stream requires canonical rehydrate task_id=%s cursor=%d", task.ID(), replay.Checkpoint.Cursor))
				return
			}
		}

		for _, item := range coalesceTaskEvents(replay.Events) {
			if err := writeSSE(item); err != nil {
				slog.InfoContext(ctx, fmt.Sprintf("[agent-sse] stream interrupted task_id=%s phase=replay cursor=%d event=%s err=%v", task.ID(), item.Cursor, item.Event.Type, err))
				return
			}
		}

		if item, err := writeCoalescedTaskEventStream(subscription.Events(), writeSSE); err != nil {
			slog.InfoContext(ctx, fmt.Sprintf("[agent-sse] stream interrupted task_id=%s phase=live cursor=%d event=%s err=%v", task.ID(), item.Cursor, item.Event.Type, err))
			return
		}
		slog.InfoContext(ctx, fmt.Sprintf("[agent-sse] stream end task_id=%s status=%s reason=%s", task.ID(), task.Status(), subscription.EndReason()))
	}()

	c.Response.SetBodyStream(pr, -1)
}

// StreamTaskUI writes Task replay and live updates using the AI SDK UI message
// stream protocol consumed by @ai-sdk/react. A non-zero cursor is accepted only
// for display rehydration: the Writing client first replaces provisional UI
// state with canonical history, then asks for the exact Task suffix after the
// server-issued checkpoint cursor. The UI stream itself intentionally carries
// no Last-Event-ID because one Task event may expand to several AI SDK frames.
func StreamTaskUI(ctx context.Context, c *app.RequestContext, task *apptask.Task) {
	after, ok := requestedTaskCursor(c)
	if !ok {
		return
	}
	replay, subscription, err := task.SubscribeDisplayAfter(after)
	if err != nil {
		writeTaskCursorError(c, task, err)
		return
	}
	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.Header.Set("x-vercel-ai-ui-message-stream", "v1")
	c.Response.ImmediateHeaderFlush = true

	pr, pw := io.Pipe()

	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.ErrorContext(ctx, fmt.Sprintf("[agent-ui-sse] stream panic recovered task_id=%s err=%v", task.ID(), recovered))
			}
			task.Unsubscribe(subscription)
			_ = pw.Close()
		}()
		slog.InfoContext(ctx, fmt.Sprintf("[agent-ui-sse] stream start task_id=%s after=%d replay=%d checkpoint=%t", task.ID(), after, len(replay.Events), replay.Checkpoint != nil))
		writeUI := newUIWriteHandler(ctx, pw)

		if replay.Checkpoint != nil {
			committed, err := writeUITaskCheckpoint(writeUI, *replay.Checkpoint)
			if err != nil {
				slog.InfoContext(ctx, fmt.Sprintf("[agent-ui-sse] stream interrupted task_id=%s phase=checkpoint cursor=%d err=%v", task.ID(), replay.Checkpoint.Cursor, err))
				return
			}
			if !committed {
				slog.InfoContext(ctx, fmt.Sprintf("[agent-ui-sse] stream requires canonical rehydrate task_id=%s cursor=%d", task.ID(), replay.Checkpoint.Cursor))
				return
			}
		}

		for _, item := range coalesceTaskEvents(replay.Events) {
			if err := writeUI.Handle(item); err != nil {
				slog.InfoContext(ctx, fmt.Sprintf("[agent-ui-sse] stream interrupted task_id=%s phase=replay cursor=%d event=%s err=%v", task.ID(), item.Cursor, item.Event.Type, err))
				return
			}
		}

		if item, err := writeCoalescedTaskEventStream(subscription.Events(), writeUI.Handle); err != nil {
			slog.InfoContext(ctx, fmt.Sprintf("[agent-ui-sse] stream interrupted task_id=%s phase=live cursor=%d event=%s err=%v", task.ID(), item.Cursor, item.Event.Type, err))
			return
		}
		if subscription.EndReason() == apptask.SubscriptionTaskFinished {
			_ = writeUI.Finish("stop")
		}
		slog.InfoContext(ctx, fmt.Sprintf("[agent-ui-sse] stream end task_id=%s status=%s reason=%s", task.ID(), task.Status(), subscription.EndReason()))
	}()

	c.Response.SetBodyStream(pr, -1)
}

func writeTaskCheckpoint(w io.Writer, checkpoint apptask.DisplayCheckpoint, writeSSE func(apptask.Event) error) (bool, error) {
	if !checkpoint.Complete {
		// An incomplete projection must never advance Last-Event-ID or attach the
		// client to live events: either would silently certify missing display
		// output. Omitting the SSE id leaves Last-Event-ID unchanged; the client
		// advances only by explicitly using the server-issued checkpoint cursor
		// after canonical rehydration.
		if err := writeEventWithoutCursor(w, taskRehydrateRequiredEventType, taskRehydrateRequiredData(checkpoint)); err != nil {
			return false, err
		}
		return false, nil
	}
	metadata := map[string]any{
		"version":                   checkpoint.Version,
		"task_id":                   checkpoint.TaskID,
		"cursor":                    checkpoint.Cursor,
		"complete":                  checkpoint.Complete,
		"settled":                   checkpoint.Settled,
		"status":                    checkpoint.Status,
		"terminal_reason":           checkpoint.TerminalReason,
		"terminal_reason_truncated": checkpoint.TerminalReasonTruncated,
		"event_count":               len(checkpoint.Events),
	}
	// Checkpoint envelopes deliberately omit an SSE id. The client advances to
	// checkpoint.Cursor only after every projected event was written, so a
	// mid-checkpoint disconnect safely restarts the reset+replay.
	if err := writeEventWithoutCursor(w, taskCheckpointEventType, metadata); err != nil {
		return false, err
	}
	for _, event := range checkpoint.Events {
		if err := writeSSE(apptask.Event{Event: event}); err != nil {
			return false, err
		}
	}
	if err := writeEvent(w, checkpoint.Cursor, taskCheckpointCommittedEventType, map[string]any{
		"version": checkpoint.Version,
		"cursor":  checkpoint.Cursor,
	}); err != nil {
		return false, err
	}
	return true, nil
}

func taskRehydrateRequiredData(checkpoint apptask.DisplayCheckpoint) map[string]any {
	return map[string]any{
		"code":                      "agent_stream.rehydrate_required",
		"message":                   "展示历史超过恢复预算，请重新加载以从持久化历史恢复 / Display history exceeded the recovery budget; reload from canonical history",
		"version":                   checkpoint.Version,
		"task_id":                   checkpoint.TaskID,
		"cursor":                    checkpoint.Cursor,
		"settled":                   checkpoint.Settled,
		"status":                    checkpoint.Status,
		"terminal_reason":           checkpoint.TerminalReason,
		"terminal_reason_truncated": checkpoint.TerminalReasonTruncated,
		"persistence_required":      checkpoint.PersistenceRequired,
	}
}

func writeUITaskCheckpoint(writeUI *uiWriteHandler, checkpoint apptask.DisplayCheckpoint) (bool, error) {
	if !checkpoint.Complete {
		data := taskRehydrateRequiredData(checkpoint)
		// Preserve a typed data part for the Writing client before surfacing the
		// user-visible error. AI SDK errorText alone cannot carry a stable code or
		// the exact Task identity that must no longer be reconnected.
		if err := writeUI.Handle(apptask.Event{Event: novaApp.AgentEvent{Type: taskRehydrateRequiredEventType, Data: data}}); err != nil {
			return false, err
		}
		if err := writeUI.Handle(apptask.Event{Event: novaApp.AgentEvent{Type: "error", Data: data}}); err != nil {
			return false, err
		}
		if err := writeUI.Finish("error"); err != nil {
			return false, err
		}
		return false, nil
	}
	for _, event := range checkpoint.Events {
		if err := writeUI.Handle(apptask.Event{Cursor: checkpoint.Cursor, Event: event}); err != nil {
			return false, err
		}
	}
	return true, nil
}

func writeTaskCursorError(c *app.RequestContext, task *apptask.Task, err error) {
	response := map[string]any{
		"error":           "事件流游标已失效 / Event stream cursor is invalid",
		"code":            "agent_stream.invalid_cursor",
		"earliest_cursor": task.EarliestCursor(),
		"latest_cursor":   task.Cursor(),
	}
	if errors.Is(err, apptask.ErrCursorExpired) {
		response["error"] = "事件流游标已过期，请从规范历史恢复 / Event stream cursor expired; recover from canonical history"
		response["code"] = "agent_stream.cursor_expired"
	}
	c.JSON(409, response)
}

func newSSEWriteHandler(ctx context.Context, w io.Writer) func(apptask.Event) error {
	return func(item apptask.Event) error {
		event := correlateErrorEvent(item.Event, observability.RequestID(ctx))
		if item.Cursor == 0 {
			return writeEventWithoutCursor(w, event.Type, event.Data)
		}
		return writeEvent(w, item.Cursor, event.Type, event.Data)
	}
}

type uiWriteHandler struct {
	encoder *agentui.StreamEncoder
}

func newUIWriteHandler(ctx context.Context, w io.Writer) *uiWriteHandler {
	return &uiWriteHandler{encoder: agentui.NewStreamEncoder(w, observability.RequestID(ctx))}
}

func correlateErrorEvent(event novaApp.AgentEvent, requestID string) novaApp.AgentEvent {
	requestID = strings.TrimSpace(requestID)
	if event.Type != "error" || requestID == "" {
		return event
	}
	payload := map[string]any{}
	if raw, err := json.Marshal(event.Data); err == nil {
		_ = json.Unmarshal(raw, &payload)
	}
	payload[observability.RequestIDField] = requestID
	for _, key := range []string{"message", "error"} {
		if message, ok := payload[key].(string); ok && strings.TrimSpace(message) != "" {
			payload[key] = agentui.CorrelatedErrorMessage(message, requestID)
			break
		}
	}
	event.Data = payload
	return event
}

func (h *uiWriteHandler) Handle(item apptask.Event) error {
	return h.encoder.WriteEvent(item.Event)
}

func (h *uiWriteHandler) Finish(reason string) error {
	return h.encoder.Finish(reason)
}

func writeEvent(w io.Writer, cursor uint64, eventType string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", cursor, eventType, jsonData)
	return err
}

func writeEventWithoutCursor(w io.Writer, eventType string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, jsonData)
	return err
}

func requestedTaskCursor(c *app.RequestContext) (uint64, bool) {
	raw := strings.TrimSpace(c.Query("after"))
	if raw == "" {
		raw = strings.TrimSpace(string(c.Request.Header.Peek("Last-Event-ID")))
	}
	if raw == "" {
		return 0, true
	}
	cursor, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		c.JSON(400, map[string]any{
			"error": "事件流游标格式无效 / Invalid event stream cursor",
			"code":  "agent_stream.invalid_cursor",
		})
		return 0, false
	}
	return cursor, true
}
