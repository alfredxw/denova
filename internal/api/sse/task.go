package sse

import (
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/cloudwego/hertz/pkg/app"

	"nova/internal/agent"
	agentmiddleware "nova/internal/agent/middleware"
	novaApp "nova/internal/app"
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

// StreamTask writes a Task event snapshot and live updates as Server-Sent Events.
func StreamTask(c *app.RequestContext, task *novaApp.Task, options ...StreamOption) {
	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.ImmediateHeaderFlush = true

	pr, pw := io.Pipe()

	go func() {
		var ch <-chan agent.Event
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[agent-sse] stream panic recovered task_id=%s err=%v", task.ID(), recovered)
			}
			if ch != nil {
				task.Unsubscribe(ch)
			}
			_ = pw.Close()
		}()
		var snapshot []agent.Event
		snapshot, ch = task.Subscribe()
		log.Printf("[agent-sse] stream start task_id=%s replay=%d", task.ID(), len(snapshot))
		writeSSE := newSSEWriteHandler(pw, options...)

		for _, ev := range snapshot {
			if err := writeSSE(ev); err != nil {
				log.Printf("[agent-sse] stream interrupted task_id=%s phase=replay event=%s err=%v", task.ID(), ev.Type, err)
				return
			}
		}

		for ev := range ch {
			if err := writeSSE(ev); err != nil {
				log.Printf("[agent-sse] stream interrupted task_id=%s phase=live event=%s err=%v", task.ID(), ev.Type, err)
				return
			}
		}
		log.Printf("[agent-sse] stream end task_id=%s status=%s", task.ID(), task.Status())
	}()

	c.Response.SetBodyStream(pr, -1)
}

func newSSEWriteHandler(w io.Writer, options ...StreamOption) agentmiddleware.SSEEventHandler {
	opts := applyStreamOptions(options...)
	chain := agentmiddleware.NewSSEEventMiddlewareChain(
		agentmiddleware.WithHideChapterBodyLiveOutput(opts.HideChapterBodyLiveOutput),
	)
	return chain.Next(func(ev agent.Event) error {
		return writeEvent(w, ev.Type, ev.Data)
	})
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

func writeEvent(w io.Writer, eventType string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, jsonData)
	return err
}
