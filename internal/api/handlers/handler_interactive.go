package handlers

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/internal/api/sse"
	novaApp "nova/internal/app"
	"nova/internal/imagepreset"
	"nova/internal/interactive"
)

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
	includeArchived := strings.EqualFold(c.Query("archived"), "true") || strings.EqualFold(c.Query("include_archived"), "true")
	state, err := h.app.InteractiveMemory(c.Param("id"), c.Query("branch"), includeArchived)
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, state)
}

func (h *Handlers) HandleStoryMemory(ctx context.Context, c *app.RequestContext) {
	includeArchived := strings.EqualFold(c.Query("archived"), "true") || strings.EqualFold(c.Query("include_archived"), "true")
	state, err := h.app.StoryMemory(c.Param("id"), c.Query("branch"), includeArchived)
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

func (h *Handlers) HandleStoryMemoryRecordArchive(ctx context.Context, c *app.RequestContext) {
	var body interactive.StoryMemoryRecordArchiveRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	archived := true
	if body.Archived != nil {
		archived = *body.Archived
	}
	record, err := h.app.SetStoryMemoryRecordArchived(c.Param("id"), c.Param("record_id"), c.Query("branch"), archived)
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

func (h *Handlers) HandleStoryMemoryGenerateStream(ctx context.Context, c *app.RequestContext) {
	var body interactive.StoryMemoryGenerateRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if body.BranchID == "" {
		body.BranchID = c.Query("branch")
	}
	task := h.app.StartStoryMemoryGenerateTask(c.Param("id"), body.BranchID, body.Source)
	if task == nil {
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
		return
	}
	sse.StreamTask(c, task)
}

func (h *Handlers) HandleInteractiveImageGenerate(ctx context.Context, c *app.RequestContext) {
	var body interactive.InteractiveImageGenerateRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if body.BranchID == "" {
		body.BranchID = c.Query("branch")
	}
	result, err := h.app.GenerateInteractiveImage(ctx, c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
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

func (h *Handlers) HandleInteractiveMemoryArchive(ctx context.Context, c *app.RequestContext) {
	var body interactive.InteractiveMemoryArchiveRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	archived := true
	if body.Archived != nil {
		archived = *body.Archived
	}
	entry, err := h.app.SetInteractiveMemoryArchived(c.Param("id"), c.Param("memory_id"), archived)
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
		StyleScenes        []string `json:"style_scenes"`
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
	locale := requestLocale(c)
	if strings.TrimSpace(body.RegenerateFromTurn) != "" {
		task = h.app.StartInteractiveRegenerateTask(body.StoryID, body.Branch, body.RegenerateFromTurn, body.Message, body.StyleScenes, locale)
	} else {
		task = h.app.StartInteractiveTask(body.StoryID, body.Branch, body.Message, body.StyleScenes, locale)
	}
	if task == nil {
		writeErrorKey(c, consts.StatusConflict, "api.workspace.noWorkspace")
		return
	}
	sse.StreamTask(c, task)
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

func (h *Handlers) HandleInteractiveContextCompaction(ctx context.Context, c *app.RequestContext) {
	var body struct {
		BranchID string `json:"branch_id"`
		Branch   string `json:"branch"`
	}
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	branchID := body.BranchID
	if strings.TrimSpace(branchID) == "" {
		branchID = body.Branch
	}
	result, err := h.app.CompactInteractiveContext(ctx, c.Param("id"), branchID)
	if err != nil {
		writeError(c, consts.StatusConflict, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleInteractiveContextCompactionRemove(ctx context.Context, c *app.RequestContext) {
	removed, err := h.app.RemoveInteractiveContextCompaction(c.Param("id"), c.Query("branch"))
	if err != nil {
		writeError(c, consts.StatusConflict, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]bool{"removed": removed})
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
	var body struct {
		interactive.Teller
		BaseRevision string `json:"base_revision"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	teller, err := h.app.UpdateInteractiveTeller(c.Param("id"), body.Teller, body.BaseRevision)
	if err != nil {
		if errors.Is(err, interactive.ErrTellerRevisionConflict) {
			writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
			return
		}
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

func (h *Handlers) HandleImagePresets(ctx context.Context, c *app.RequestContext) {
	presets, err := h.app.ImagePresets()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"presets": presets})
}

func (h *Handlers) HandleImagePreset(ctx context.Context, c *app.RequestContext) {
	preset, err := h.app.ImagePreset(c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, preset)
}

func (h *Handlers) HandleImagePresetCreate(ctx context.Context, c *app.RequestContext) {
	var body imagepreset.Preset
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	preset, err := h.app.CreateImagePreset(body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, preset)
}

func (h *Handlers) HandleImagePresetUpdate(ctx context.Context, c *app.RequestContext) {
	var body struct {
		imagepreset.Preset
		BaseRevision string `json:"base_revision"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	preset, err := h.app.UpdateImagePreset(c.Param("id"), body.Preset, body.BaseRevision)
	if err != nil {
		if errors.Is(err, imagepreset.ErrPresetRevisionConflict) {
			writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, preset)
}

func (h *Handlers) HandleImagePresetDelete(ctx context.Context, c *app.RequestContext) {
	if err := h.app.DeleteImagePreset(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}
