package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/api/sse"
	novaApp "denova/internal/app"
)

func (h *Handlers) HandleInteractiveChat(ctx context.Context, c *app.RequestContext) {
	var body struct {
		CommandID          string                          `json:"command_id"`
		Mode               string                          `json:"mode"`
		StoryID            string                          `json:"story_id"`
		Branch             string                          `json:"branch"`
		Message            string                          `json:"message"`
		StyleScenes        []string                        `json:"style_scenes"`
		RegenerateFromTurn string                          `json:"regenerate_from_turn_id"`
		Attachments        []novaApp.AgentAttachmentUpload `json:"attachments,omitempty"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if strings.TrimSpace(body.Message) == "" && len(body.Attachments) == 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.messageRequired")
		return
	}
	if strings.TrimSpace(body.StoryID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	if body.Mode != "" && body.Mode != "story" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyModeOnly")
		return
	}
	if strings.TrimSpace(body.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试请求 / command_id is required for safe request retries", nil)
		return
	}
	chatRequest := novaApp.AgentChatRequest{CommandID: body.CommandID, Message: body.Message, AttachmentUploads: body.Attachments}
	if err := h.app.MaterializeInteractiveAttachments(body.StoryID, body.CommandID, &chatRequest); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}

	task, err := h.app.StartInteractiveTaskWithError(ctx, novaApp.InteractiveAgentStartRequest{
		CommandID: body.CommandID, StoryID: body.StoryID, BranchID: body.Branch,
		Message: body.Message, StyleScenes: body.StyleScenes,
		RegenerateFromTurnID: body.RegenerateFromTurn, Locale: requestLocale(c),
		AttachmentIDs: chatRequest.AttachmentIDs, AttachedFiles: chatRequest.AttachedFiles,
	})
	if err != nil {
		h.writeChatPreparationError(c, err)
		return
	}
	sse.StreamTask(ctx, c, task)
}

func (h *Handlers) HandleInteractiveChatContextAnalysis(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Mode        string   `json:"mode"`
		StoryID     string   `json:"story_id"`
		Branch      string   `json:"branch"`
		Message     string   `json:"message"`
		StyleScenes []string `json:"style_scenes"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.messageRequired")
		return
	}
	if strings.TrimSpace(body.StoryID) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	if body.Mode != "" && body.Mode != "story" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyModeOnly")
		return
	}
	analysis, err := h.app.AnalyzeInteractiveContext(body.StoryID, body.Branch, body.Message, body.StyleScenes, requestLocale(c))
	if err != nil {
		writeError(c, consts.StatusConflict, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, analysis)
}

// HandleInteractiveChatStream reconnects to the active game-mode turn and
// replays its buffered SSE events before following live output.
func (h *Handlers) HandleInteractiveChatStream(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Query("story_id"))
	branchID := strings.TrimSpace(c.Query("branch"))
	taskID := strings.TrimSpace(c.Query("task_id"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	if taskID == "" {
		writeError(c, consts.StatusBadRequest, "缺少 task_id，无法精确恢复 Agent 流 / task_id is required for exact Agent stream recovery")
		return
	}
	task, info := h.app.ActiveInteractiveTaskFor(storyID, branchID)
	if task == nil || info.TaskID != taskID {
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.rehydrate_required", "旧的任务流已失效，请从 active projection 重新挂接 / The old task stream is stale; rehydrate from the active projection", map[string]any{"task_id": taskID})
		return
	}
	slog.InfoContext(ctx, fmt.Sprintf("[interactive-agent-sse] attach active task_id=%s command_id=%s story_id=%s branch_id=%s status=%s", task.ID(), info.CommandID, info.StoryID, info.BranchID, task.Status()))
	sse.StreamTask(ctx, c, task)
}

// HandleInteractiveChatActive reports the active turn identity and original
// player message so a refreshed stage can reconstruct its optimistic turn.
func (h *Handlers) HandleInteractiveChatActive(ctx context.Context, c *app.RequestContext) {
	storyID := strings.TrimSpace(c.Query("story_id"))
	branchID := strings.TrimSpace(c.Query("branch"))
	if storyID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.storyIDRequired")
		return
	}
	view := h.app.InteractiveAgentActiveView(ctx, storyID, branchID)
	if view.Task == nil {
		response := map[string]any{"active": false}
		addAgentRuntimeProjection(response, view.Runtime, agentRuntimeProjectionOptions{Available: view.RuntimeProjectionOK})
		writeJSON(c, consts.StatusOK, response)
		return
	}
	response := map[string]any{
		"active":                  !view.Task.Finished,
		"status":                  view.Task.Status,
		"task_id":                 view.Info.TaskID,
		"command_id":              view.Info.CommandID,
		"stream_cursor":           view.Task.Cursor,
		"story_id":                view.Info.StoryID,
		"branch_id":               view.Info.BranchID,
		"message":                 view.Info.Message,
		"regenerate_from_turn_id": view.Info.RegenerateFromTurnID,
	}
	if len(view.Info.Attachments) > 0 {
		response["attachments"] = view.Info.Attachments
	}
	addAgentRuntimeProjection(response, view.Runtime, agentRuntimeProjectionOptions{
		Available: view.RuntimeProjectionOK, StreamAttached: true,
	})
	writeJSON(c, consts.StatusOK, response)
}
