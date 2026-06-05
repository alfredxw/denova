package handlers

import (
	"context"
	"log"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/internal/api/sse"
	novaApp "nova/internal/app"
	"nova/internal/interactive"
)

type tellerAgentRequest struct {
	Instruction string   `json:"instruction"`
	TellerID    string   `json:"teller_id"`
	References  []string `json:"references"`
}

func (h *Handlers) HandleInteractiveStories(ctx context.Context, c *app.RequestContext) {
	index, err := h.app.InteractiveStories()
	if err != nil {
		writeError(c, consts.StatusConflict, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, index)
}

func (h *Handlers) HandleInteractiveStoryCreate(ctx context.Context, c *app.RequestContext) {
	var body interactive.CreateStoryRequest
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	story, err := h.app.CreateInteractiveStory(body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, story)
}

func (h *Handlers) HandleInteractiveStoryUpdate(ctx context.Context, c *app.RequestContext) {
	var body interactive.UpdateStoryRequest
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	story, err := h.app.UpdateInteractiveStory(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, story)
}

func (h *Handlers) HandleInteractiveStoryDelete(ctx context.Context, c *app.RequestContext) {
	if err := h.app.DeleteInteractiveStory(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleInteractiveSnapshot(ctx context.Context, c *app.RequestContext) {
	snapshot, err := h.app.InteractiveSnapshot(c.Param("id"), c.Query("branch"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, snapshot)
}

func (h *Handlers) HandleInteractiveBranches(ctx context.Context, c *app.RequestContext) {
	branches, err := h.app.InteractiveBranches(c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"branches": branches})
}

func (h *Handlers) HandleInteractiveBranchCreate(ctx context.Context, c *app.RequestContext) {
	var body interactive.CreateBranchRequest
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	branch, err := h.app.CreateInteractiveBranch(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, branch)
}

func (h *Handlers) HandleInteractiveBranchDelete(ctx context.Context, c *app.RequestContext) {
	if err := h.app.DeleteInteractiveBranch(c.Param("id"), c.Param("branch")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleInteractiveBranchSwitch(ctx context.Context, c *app.RequestContext) {
	var body struct {
		BranchID string `json:"branch_id"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	if err := h.app.SwitchInteractiveBranch(c.Param("id"), body.BranchID); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleInteractiveTurnVersionSwitch(ctx context.Context, c *app.RequestContext) {
	var body interactive.SwitchTurnVersionRequest
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	if err := h.app.SwitchInteractiveTurnVersion(c.Param("id"), body); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleInteractiveHotChoices(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Branch         string   `json:"branch"`
		ExcludeChoices []string `json:"exclude_choices"`
	}
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	result, err := h.app.GenerateInteractiveHotChoices(ctx, c.Param("id"), body.Branch, body.ExcludeChoices)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleInteractiveChat(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Mode               string   `json:"mode"`
		StoryID            string   `json:"story_id"`
		Branch             string   `json:"branch"`
		Message            string   `json:"message"`
		StyleReferences    []string `json:"style_references"`
		RegenerateFromTurn string   `json:"regenerate_from_turn_id"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		writeError(c, consts.StatusBadRequest, "消息不能为空")
		return
	}
	if strings.TrimSpace(body.StoryID) == "" {
		writeError(c, consts.StatusBadRequest, "故事 ID 不能为空")
		return
	}
	if body.Mode != "" && body.Mode != "story" {
		writeError(c, consts.StatusBadRequest, "当前仅支持 story 子模式")
		return
	}

	var task *novaApp.Task
	if strings.TrimSpace(body.RegenerateFromTurn) != "" {
		task = h.app.StartInteractiveRegenerateTask(body.StoryID, body.Branch, body.RegenerateFromTurn, body.Message, body.StyleReferences)
	} else {
		task = h.app.StartInteractiveTask(body.StoryID, body.Branch, body.Message, body.StyleReferences)
	}
	if task == nil {
		writeError(c, consts.StatusConflict, "尚未选择书籍工作区，请先在书籍管理页选择或创建书籍")
		return
	}
	sse.StreamTask(c, task)
}

func (h *Handlers) HandleInteractiveChatAbort(ctx context.Context, c *app.RequestContext) {
	if task := h.app.ActiveInteractiveTask(); task != nil {
		log.Printf("[interactive-agent-sse] abort requested task_id=%s status=%s", task.ID(), task.Status())
	}
	h.app.AbortInteractiveTask()
	c.JSON(consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleInteractiveTellers(ctx context.Context, c *app.RequestContext) {
	tellers, err := h.app.InteractiveTellers()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"tellers": tellers})
}

func (h *Handlers) HandleInteractiveTeller(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	teller, err := h.app.InteractiveTeller(id)
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, teller)
}

func (h *Handlers) HandleInteractiveTellerCreate(ctx context.Context, c *app.RequestContext) {
	var body interactive.Teller
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	teller, err := h.app.CreateInteractiveTeller(body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, teller)
}

func (h *Handlers) HandleInteractiveTellerUpdate(ctx context.Context, c *app.RequestContext) {
	var body interactive.Teller
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	teller, err := h.app.UpdateInteractiveTeller(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, teller)
}

func (h *Handlers) HandleInteractiveTellerDelete(ctx context.Context, c *app.RequestContext) {
	if err := h.app.DeleteInteractiveTeller(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleInteractiveTellerAgentStream(ctx context.Context, c *app.RequestContext) {
	var body tellerAgentRequest
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	if strings.TrimSpace(body.Instruction) == "" {
		writeError(c, consts.StatusBadRequest, "讲述者编辑指令不能为空")
		return
	}
	task := h.app.StartTellerAgentTask(body.Instruction, body.TellerID, body.References)
	if task == nil {
		writeError(c, consts.StatusConflict, "尚未选择书籍工作区，请先在书籍管理页选择或创建书籍")
		return
	}
	sse.StreamTask(c, task)
}

func (h *Handlers) HandleInteractiveTellerAgentMessages(ctx context.Context, c *app.RequestContext) {
	if !h.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, []messageDTO{})
		return
	}
	entries, err := h.app.TellerAgentMessages()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	result := make([]messageDTO, 0, len(entries))
	for _, entry := range entries {
		if entry.Type == "clear" {
			result = append(result, messageDTO{
				Type:      entry.Type,
				CreatedAt: formatTime(entry.CreatedAt),
			})
			continue
		}
		if entry.Content == "" {
			continue
		}
		result = append(result, messageDTO{
			Type:    entry.Type,
			Role:    entry.Role,
			Content: entry.Content,
		})
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleInteractiveTellerAgentClear(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	if err := h.app.ClearTellerAgentSession(); err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}
