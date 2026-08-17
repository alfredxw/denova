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

// HandleProjectFileEvents streams ephemeral filesystem invalidations. The
// first event is always resync; reconnecting clients never depend on replay.
func (h *Handlers) HandleProjectFileEvents(ctx context.Context, c *app.RequestContext) {
	scope, ok := requireProjectScope(c)
	if !ok {
		return
	}
	events, unsubscribe, subscribeErr := h.app.SubscribeProjectFileChanges(scope.ProjectID)
	if subscribeErr != nil {
		slog.WarnContext(ctx, "[filewatch-sse] Project watcher unavailable; canonical refresh remains active",
			"project_id", scope.ProjectID, "workspace", scope.ContentRoot, "error", subscribeErr)
	}
	projectID := scope.ProjectID
	workspace := scope.ContentRoot
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
				slog.ErrorContext(ctx, fmt.Sprintf("[filewatch-sse] stream panic recovered project_id=%q workspace=%q err=%v", projectID, workspace, recovered))
			}
			unsubscribe()
			_ = writer.Close()
		}()
		slog.InfoContext(ctx, fmt.Sprintf("[filewatch-sse] stream connected project_id=%q workspace=%q", projectID, workspace))
		for {
			select {
			case event, ok := <-events:
				if !ok {
					return
				}
				if err := writeProjectFileEvent(writer, event); err != nil {
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

func writeProjectFileEvent(writer io.Writer, event filewatch.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode Project file event: %w", err)
	}
	if _, err := fmt.Fprintf(writer, "event: workspace-change\ndata: %s\n\n", payload); err != nil {
		return fmt.Errorf("write Project file event: %w", err)
	}
	return nil
}
