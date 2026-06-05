package sse

import (
	"encoding/json"
	"fmt"
	"io"
	"log"

	"github.com/cloudwego/hertz/pkg/app"

	"nova/internal/agent"
	novaApp "nova/internal/app"
)

// StreamTask writes a Task event snapshot and live updates as Server-Sent Events.
func StreamTask(c *app.RequestContext, task *novaApp.Task) {
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

		for _, ev := range snapshot {
			if err := writeEvent(pw, ev.Type, ev.Data); err != nil {
				log.Printf("[agent-sse] stream interrupted task_id=%s phase=replay event=%s err=%v", task.ID(), ev.Type, err)
				return
			}
		}

		for ev := range ch {
			if err := writeEvent(pw, ev.Type, ev.Data); err != nil {
				log.Printf("[agent-sse] stream interrupted task_id=%s phase=live event=%s err=%v", task.ID(), ev.Type, err)
				return
			}
		}
		log.Printf("[agent-sse] stream end task_id=%s status=%s", task.ID(), task.Status())
	}()

	c.Response.SetBodyStream(pr, -1)
}

func writeEvent(w io.Writer, eventType string, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, jsonData)
	return err
}
