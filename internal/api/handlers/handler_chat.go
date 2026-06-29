package handlers

import (
	"context"
	"log"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/internal/agent"
	"nova/internal/api/sse"
	novaApp "nova/internal/app"
)

// handleChat 处理聊天请求：启动后台 Task，然后以 SSE 流订阅事件。
func (h *Handlers) HandleChat(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req agent.ChatRequest
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.messageRequired")
		return
	}
	req.Locale = requestLocale(c)

	task := h.app.StartTask(req)
	if task == nil {
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
		return
	}
	log.Printf("[agent-sse] attach new chat task_id=%s", task.ID())
	sse.StreamTask(c, task, h.chatSSEStreamOptions()...)
}

// HandleChatContextAnalysis 模拟一次聊天请求，返回真实 SystemPrompt 和上下文组成，不启动 LLM。
func (h *Handlers) HandleChatContextAnalysis(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req agent.ChatRequest
	if err := c.BindJSON(&req); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.messageRequired")
		return
	}
	req.Locale = requestLocale(c)
	analysis, err := h.app.AnalyzeContext(req)
	if err != nil {
		writeError(c, consts.StatusConflict, err.Error())
		return
	}
	c.JSON(consts.StatusOK, analysis)
}

func (h *Handlers) HandleChatContextCompaction(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	result, err := h.app.CompactContext(ctx)
	if err != nil {
		writeError(c, consts.StatusConflict, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleChatContextCompactionRemove(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	removed, err := h.app.RemoveContextCompaction()
	if err != nil {
		writeError(c, consts.StatusConflict, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]bool{"removed": removed})
}

// handleChatStream 重连到当前活跃任务的事件流（回放已有事件 + 继续接收新事件）。
func (h *Handlers) HandleChatStream(ctx context.Context, c *app.RequestContext) {
	task := h.app.ActiveTask()
	if task == nil {
		writeErrorKey(c, consts.StatusNotFound, "api.chat.noActiveTask")
		return
	}
	log.Printf("[agent-sse] attach active chat task_id=%s status=%s", task.ID(), task.Status())
	sse.StreamTask(c, task, h.chatSSEStreamOptions()...)
}

// handleChatActive 查询当前是否有活跃任务。
func (h *Handlers) HandleChatActive(ctx context.Context, c *app.RequestContext) {
	task := h.app.ActiveTask()
	if task == nil {
		c.JSON(consts.StatusOK, map[string]interface{}{
			"active": false,
		})
		return
	}
	status := task.Status()
	c.JSON(consts.StatusOK, map[string]interface{}{
		"active": status == novaApp.TaskRunning,
		"status": status,
	})
}

// handleChatAbort 终止当前活跃任务。
func (h *Handlers) HandleChatAbort(ctx context.Context, c *app.RequestContext) {
	if task := h.app.ActiveTask(); task != nil {
		log.Printf("[agent-sse] abort requested task_id=%s status=%s", task.ID(), task.Status())
	}
	h.app.AbortTask()
	c.JSON(consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) chatSSEStreamOptions() []sse.StreamOption {
	return []sse.StreamOption{
		sse.WithHideChapterBodyLiveOutput(h.app.HideChapterBodyLiveOutput()),
	}
}
