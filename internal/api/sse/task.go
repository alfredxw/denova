package sse

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"

	"denova/internal/agent"
	agentmiddleware "denova/internal/agent/middleware"
	"denova/internal/api/agentui"
	novaApp "denova/internal/app"
)

type StreamOptions struct {
	HideChapterBodyLiveOutput bool
}

type StreamOption struct {
	F func(*StreamOptions)
}

func WithHideChapterBodyLiveOutput(enabled bool) StreamOption {
	return StreamOption{F: func(o *StreamOptions) {
		o.HideChapterBodyLiveOutput = enabled
	}}
}

const (
	taskCheckpointEventType          = "task_checkpoint"
	taskCheckpointCommittedEventType = "task_checkpoint_committed"
	taskRehydrateRequiredEventType   = "task_rehydrate_required"
)

// StreamTask writes a Task event snapshot and live updates as Server-Sent Events.
func StreamTask(c *app.RequestContext, task *novaApp.Task, options ...StreamOption) {
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
				log.Printf("[agent-sse] stream panic recovered task_id=%s err=%v", task.ID(), recovered)
			}
			task.Unsubscribe(subscription)
			_ = pw.Close()
		}()
		log.Printf("[agent-sse] stream start task_id=%s after=%d replay=%d checkpoint=%t", task.ID(), after, len(replay.Events), replay.Checkpoint != nil)
		writeSSE := newSSEWriteHandler(pw, options...)

		if replay.Checkpoint != nil {
			committed, err := writeTaskCheckpoint(pw, *replay.Checkpoint, writeSSE)
			if err != nil {
				log.Printf("[agent-sse] stream interrupted task_id=%s phase=checkpoint cursor=%d err=%v", task.ID(), replay.Checkpoint.Cursor, err)
				return
			}
			if !committed {
				log.Printf("[agent-sse] stream requires canonical rehydrate task_id=%s cursor=%d", task.ID(), replay.Checkpoint.Cursor)
				return
			}
		}

		for _, item := range replay.Events {
			if err := writeSSE(item); err != nil {
				log.Printf("[agent-sse] stream interrupted task_id=%s phase=replay cursor=%d event=%s err=%v", task.ID(), item.Cursor, item.Event.Type, err)
				return
			}
		}

		for item := range subscription.Events() {
			if err := writeSSE(item); err != nil {
				log.Printf("[agent-sse] stream interrupted task_id=%s phase=live cursor=%d event=%s err=%v", task.ID(), item.Cursor, item.Event.Type, err)
				return
			}
		}
		log.Printf("[agent-sse] stream end task_id=%s status=%s reason=%s", task.ID(), task.Status(), subscription.EndReason())
	}()

	c.Response.SetBodyStream(pr, -1)
}

// StreamTaskUI writes Task replay and live updates using the AI SDK UI message
// stream protocol consumed by @ai-sdk/react. A non-zero cursor is accepted only
// for display rehydration: the Writing client first replaces provisional UI
// state with canonical history, then asks for the exact Task suffix after the
// server-issued checkpoint cursor. The UI stream itself intentionally carries
// no Last-Event-ID because one Task event may expand to several AI SDK frames.
func StreamTaskUI(c *app.RequestContext, task *novaApp.Task, options ...StreamOption) {
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
				log.Printf("[agent-ui-sse] stream panic recovered task_id=%s err=%v", task.ID(), recovered)
			}
			task.Unsubscribe(subscription)
			_ = pw.Close()
		}()
		log.Printf("[agent-ui-sse] stream start task_id=%s after=%d replay=%d checkpoint=%t", task.ID(), after, len(replay.Events), replay.Checkpoint != nil)
		writeUI := newUIWriteHandler(pw, options...)

		if replay.Checkpoint != nil {
			committed, err := writeUITaskCheckpoint(writeUI, *replay.Checkpoint)
			if err != nil {
				log.Printf("[agent-ui-sse] stream interrupted task_id=%s phase=checkpoint cursor=%d err=%v", task.ID(), replay.Checkpoint.Cursor, err)
				return
			}
			if !committed {
				log.Printf("[agent-ui-sse] stream requires canonical rehydrate task_id=%s cursor=%d", task.ID(), replay.Checkpoint.Cursor)
				return
			}
		}

		for _, item := range replay.Events {
			if err := writeUI.Handle(item); err != nil {
				log.Printf("[agent-ui-sse] stream interrupted task_id=%s phase=replay cursor=%d event=%s err=%v", task.ID(), item.Cursor, item.Event.Type, err)
				return
			}
		}

		for item := range subscription.Events() {
			if err := writeUI.Handle(item); err != nil {
				log.Printf("[agent-ui-sse] stream interrupted task_id=%s phase=live cursor=%d event=%s err=%v", task.ID(), item.Cursor, item.Event.Type, err)
				return
			}
		}
		if subscription.EndReason() == novaApp.TaskSubscriptionTaskFinished {
			_ = writeUI.Finish("stop")
		}
		log.Printf("[agent-ui-sse] stream end task_id=%s status=%s reason=%s", task.ID(), task.Status(), subscription.EndReason())
	}()

	c.Response.SetBodyStream(pr, -1)
}

func writeTaskCheckpoint(w io.Writer, checkpoint novaApp.TaskDisplayCheckpoint, writeSSE func(novaApp.TaskEvent) error) (bool, error) {
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
		if err := writeSSE(novaApp.TaskEvent{Event: event}); err != nil {
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

func taskRehydrateRequiredData(checkpoint novaApp.TaskDisplayCheckpoint) map[string]any {
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

func writeUITaskCheckpoint(writeUI *uiWriteHandler, checkpoint novaApp.TaskDisplayCheckpoint) (bool, error) {
	if !checkpoint.Complete {
		data := taskRehydrateRequiredData(checkpoint)
		// Preserve a typed data part for the Writing client before surfacing the
		// user-visible error. AI SDK errorText alone cannot carry a stable code or
		// the exact Task identity that must no longer be reconnected.
		if err := writeUI.Handle(novaApp.TaskEvent{Event: agent.Event{Type: taskRehydrateRequiredEventType, Data: data}}); err != nil {
			return false, err
		}
		if err := writeUI.Handle(novaApp.TaskEvent{Event: agent.Event{Type: "error", Data: data}}); err != nil {
			return false, err
		}
		if err := writeUI.Finish("error"); err != nil {
			return false, err
		}
		return false, nil
	}
	for _, event := range checkpoint.Events {
		if err := writeUI.Handle(novaApp.TaskEvent{Cursor: checkpoint.Cursor, Event: event}); err != nil {
			return false, err
		}
	}
	return true, nil
}

func writeTaskCursorError(c *app.RequestContext, task *novaApp.Task, err error) {
	response := map[string]any{
		"error":           "事件流游标已失效 / Event stream cursor is invalid",
		"code":            "agent_stream.invalid_cursor",
		"earliest_cursor": task.EarliestCursor(),
		"latest_cursor":   task.Cursor(),
	}
	if errors.Is(err, novaApp.ErrTaskCursorExpired) {
		response["error"] = "事件流游标已过期，请从规范历史恢复 / Event stream cursor expired; recover from canonical history"
		response["code"] = "agent_stream.cursor_expired"
	}
	c.JSON(409, response)
}

func newSSEWriteHandler(w io.Writer, options ...StreamOption) func(novaApp.TaskEvent) error {
	opts := applyStreamOptions(options...)
	var cursor uint64
	chain := agentmiddleware.NewSSEEventMiddlewareChain(
		agentmiddleware.WithHideChapterBodyLiveOutput(opts.HideChapterBodyLiveOutput),
	)
	handler := chain.Next(func(ev agent.Event) error {
		if cursor == 0 {
			return writeEventWithoutCursor(w, ev.Type, ev.Data)
		}
		return writeEvent(w, cursor, ev.Type, ev.Data)
	})
	return func(item novaApp.TaskEvent) error {
		cursor = item.Cursor
		return handler(item.Event)
	}
}

type uiWriteHandler struct {
	encoder *agentui.StreamEncoder
	handler agentmiddleware.SSEEventHandler
}

func newUIWriteHandler(w io.Writer, options ...StreamOption) *uiWriteHandler {
	opts := applyStreamOptions(options...)
	encoder := agentui.NewStreamEncoder(w)
	chain := agentmiddleware.NewSSEEventMiddlewareChain(
		agentmiddleware.WithHideChapterBodyLiveOutput(opts.HideChapterBodyLiveOutput),
	)
	h := &uiWriteHandler{encoder: encoder}
	h.handler = chain.Next(func(ev agent.Event) error {
		return encoder.WriteEvent(ev)
	})
	return h
}

func (h *uiWriteHandler) Handle(item novaApp.TaskEvent) error {
	return h.handler(item.Event)
}

func (h *uiWriteHandler) Finish(reason string) error {
	return h.encoder.Finish(reason)
}

func applyStreamOptions(options ...StreamOption) StreamOptions {
	var out StreamOptions
	for _, option := range options {
		if option.F != nil {
			option.F(&out)
		}
	}
	return out
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
