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
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
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
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
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

func (h *Handlers) HandleInteractiveMemory(ctx context.Context, c *app.RequestContext) {
	includeHidden := strings.EqualFold(c.Query("hidden"), "true") || strings.EqualFold(c.Query("include_hidden"), "true")
	state, err := h.app.InteractiveMemory(c.Param("id"), c.Query("branch"), includeHidden)
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, state)
}

func (h *Handlers) HandleStoryMemory(ctx context.Context, c *app.RequestContext) {
	includeHidden := strings.EqualFold(c.Query("hidden"), "true") || strings.EqualFold(c.Query("include_hidden"), "true")
	state, err := h.app.StoryMemory(c.Param("id"), c.Query("branch"), includeHidden)
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, state)
}

func (h *Handlers) HandleStoryMemorySettingsUpdate(ctx context.Context, c *app.RequestContext) {
	var body interactive.StoryMemorySettingsUpdateRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	settings, err := h.app.UpdateStoryMemorySettings(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, settings)
}

func (h *Handlers) HandleStoryMemoryStructureSave(ctx context.Context, c *app.RequestContext) {
	var body interactive.StoryMemoryStructureRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if id := strings.TrimSpace(c.Param("structure_id")); id != "" {
		body.ID = id
	}
	structure, err := h.app.SaveStoryMemoryStructure(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, structure)
}

func (h *Handlers) HandleStoryMemoryStructureDelete(ctx context.Context, c *app.RequestContext) {
	if err := h.app.DeleteStoryMemoryStructure(c.Param("id"), c.Param("structure_id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleStoryMemoryRecordSave(ctx context.Context, c *app.RequestContext) {
	var body interactive.StoryMemoryRecordRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if id := strings.TrimSpace(c.Param("record_id")); id != "" {
		body.ID = id
	}
	record, err := h.app.SaveStoryMemoryRecord(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, record)
}

func (h *Handlers) HandleStoryMemoryRecordHide(ctx context.Context, c *app.RequestContext) {
	var body interactive.StoryMemoryRecordHideRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	hidden := true
	if body.Hidden != nil {
		hidden = *body.Hidden
	}
	record, err := h.app.SetStoryMemoryRecordHidden(c.Param("id"), c.Param("record_id"), c.Query("branch"), hidden)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, record)
}

func (h *Handlers) HandleStoryMemoryGenerate(ctx context.Context, c *app.RequestContext) {
	var body interactive.StoryMemoryGenerateRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if body.BranchID == "" {
		body.BranchID = c.Query("branch")
	}
	state, err := h.app.GenerateStoryMemory(ctx, c.Param("id"), body.BranchID)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, state)
}

func (h *Handlers) HandleInteractiveMemoryCreate(ctx context.Context, c *app.RequestContext) {
	var body interactive.InteractiveMemoryCreateRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	entry, err := h.app.CreateInteractiveMemory(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, entry)
}

func (h *Handlers) HandleInteractiveMemoryUpdate(ctx context.Context, c *app.RequestContext) {
	var body interactive.InteractiveMemoryUpdateRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	entry, err := h.app.UpdateInteractiveMemory(c.Param("id"), c.Param("memory_id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, entry)
}

func (h *Handlers) HandleInteractiveMemoryHide(ctx context.Context, c *app.RequestContext) {
	var body interactive.InteractiveMemoryHideRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	hidden := true
	if body.Hidden != nil {
		hidden = *body.Hidden
	}
	entry, err := h.app.SetInteractiveMemoryHidden(c.Param("id"), c.Param("memory_id"), hidden)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, entry)
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
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
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
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
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
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
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
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
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

	var task *novaApp.Task
	if strings.TrimSpace(body.RegenerateFromTurn) != "" {
		task = h.app.StartInteractiveRegenerateTask(body.StoryID, body.Branch, body.RegenerateFromTurn, body.Message, body.StyleReferences)
	} else {
		task = h.app.StartInteractiveTask(body.StoryID, body.Branch, body.Message, body.StyleReferences)
	}
	if task == nil {
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
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
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
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
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
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
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if strings.TrimSpace(body.Instruction) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.interactive.tellerInstructionEmpty")
		return
	}
	task := h.app.StartTellerAgentTask(body.Instruction, body.TellerID, body.References)
	if task == nil {
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
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
