package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/cloudwego/hertz/pkg/app"

	"denova/internal/workspace/filewatch"
)

const workspaceEventHeartbeatInterval = 15 * time.Second

// HandleWorkspaceFileEvents streams ephemeral filesystem invalidations. The
// first event is always resync; reconnecting clients never depend on replay.
func (h *Handlers) HandleWorkspaceFileEvents(ctx context.Context, c *app.RequestContext) {
	events, unsubscribe := h.app.SubscribeWorkspaceFileChanges()
	workspace := h.app.Workspace()
	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.Header.Set("X-Accel-Buffering", "no")
	c.Response.ImmediateHeaderFlush = true

	reader, writer := io.Pipe()
	go func() {
		heartbeat := time.NewTicker(workspaceEventHeartbeatInterval)
		defer func() {
			heartbeat.Stop()
			if recovered := recover(); recovered != nil {
				slog.ErrorContext(ctx, fmt.Sprintf("[filewatch-sse] stream panic recovered workspace=%q err=%v", workspace, recovered))
			}
			unsubscribe()
			_ = writer.Close()
		}()
		slog.InfoContext(ctx, fmt.Sprintf("[filewatch-sse] stream connected workspace=%q", workspace))
		for {
			select {
			case event, ok := <-events:
				if !ok {
					return
				}
				if err := writeWorkspaceFileEvent(writer, event); err != nil {
					slog.InfoContext(ctx, fmt.Sprintf("[filewatch-sse] stream disconnected workspace=%q err=%v", event.Workspace, err))
					return
				}
			case <-heartbeat.C:
				if _, err := io.WriteString(writer, ": heartbeat\n\n"); err != nil {
					slog.InfoContext(ctx, fmt.Sprintf("[filewatch-sse] stream disconnected workspace=%q phase=heartbeat err=%v", workspace, err))
					return
				}
			}
		}
	}()
	c.Response.SetBodyStream(reader, -1)
}

func writeWorkspaceFileEvent(writer io.Writer, event filewatch.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode workspace file event: %w", err)
	}
	if _, err := fmt.Fprintf(writer, "event: workspace-change\ndata: %s\n\n", payload); err != nil {
		return fmt.Errorf("write workspace file event: %w", err)
	}
	return nil
}
