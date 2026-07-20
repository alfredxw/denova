package handlers

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/api/sse"
	novaApp "denova/internal/app"
	"denova/internal/imagepreset"
	"denova/internal/interactive"
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
	story, err := h.app.CreateInteractiveStoryContext(ctx, body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, story)
}

func (h *Handlers) HandleInteractiveActorTraitRoll(ctx context.Context, c *app.RequestContext) {
	var body interactive.ActorTraitRollRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	result, err := h.app.RollInteractiveActorTraits(body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
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

func (h *Handlers) HandleInteractiveStorySelect(ctx context.Context, c *app.RequestContext) {
	if err := h.app.SelectInteractiveStory(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
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

func (h *Handlers) HandleInteractiveRuleResolutionReroll(ctx context.Context, c *app.RequestContext) {
	var body interactive.RuleResolutionRerollRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if body.BranchID == "" {
		body.BranchID = c.Query("branch")
	}
	resolution, err := h.app.RerollInteractiveRuleResolution(c.Param("id"), c.Param("resolution_id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, resolution)
}

func (h *Handlers) HandleInteractiveDirector(ctx context.Context, c *app.RequestContext) {
	plan, err := h.app.InteractiveDirectorPlan(c.Param("id"), c.Query("branch"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, plan)
}

func (h *Handlers) HandleInteractiveDirectorStatus(ctx context.Context, c *app.RequestContext) {
	status, err := h.app.InteractiveDirectorPlanStatus(c.Param("id"), c.Query("branch"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, status)
}

func (h *Handlers) HandleInteractiveDirectorUpdate(ctx context.Context, c *app.RequestContext) {
	var body interactive.UpdateDirectorPlanRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if body.BranchID == "" {
		body.BranchID = c.Query("branch")
	}
	plan, err := h.app.UpdateInteractiveDirectorPlan(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, plan)
}

func (h *Handlers) HandleInteractiveDirectorRebuild(ctx context.Context, c *app.RequestContext) {
	var body interactive.RebuildDirectorPlanRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if body.BranchID == "" {
		body.BranchID = c.Query("branch")
	}
	plan, err := h.app.RebuildInteractiveDirectorPlan(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, plan)
}

func (h *Handlers) HandleInteractiveDirectorRun(ctx context.Context, c *app.RequestContext) {
	var body interactive.RunDirectorPlanRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if body.BranchID == "" {
		body.BranchID = c.Query("branch")
	}
	status, err := h.app.RunInteractiveDirectorPlan(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, status)
}

func (h *Handlers) HandleInteractiveDirectorContextAnalysis(ctx context.Context, c *app.RequestContext) {
	var body struct {
		BranchID string `json:"branch_id"`
		Branch   string `json:"branch"`
		TurnID   string `json:"turn_id"`
	}
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	branchID := body.BranchID
	if strings.TrimSpace(branchID) == "" {
		branchID = body.Branch
	}
	if strings.TrimSpace(branchID) == "" {
		branchID = c.Query("branch")
	}
	analysis, err := h.app.AnalyzeInteractiveDirectorContext(c.Param("id"), branchID, body.TurnID, requestLocale(c))
	if err != nil {
		writeError(c, consts.StatusConflict, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, analysis)
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

func (h *Handlers) HandleInteractiveTurnNarrativeUpdate(ctx context.Context, c *app.RequestContext) {
	var body interactive.UpdateTurnNarrativeRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	body.TurnID = c.Param("turn_id")
	if strings.TrimSpace(body.Narrative) == "" {
		writeError(c, consts.StatusBadRequest, "AI 回复不能为空 / AI reply cannot be empty")
		return
	}
	result, err := h.app.UpdateInteractiveTurnNarrative(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusConflict, err.Error())
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
		task = h.app.StartInteractiveRegenerateTask(ctx, body.StoryID, body.Branch, body.RegenerateFromTurn, body.Message, body.StyleScenes, locale)
	} else {
		task = h.app.StartInteractiveTask(ctx, body.StoryID, body.Branch, body.Message, body.StyleScenes, locale)
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
	task, info := h.app.ActiveInteractiveTaskFor(storyID, branchID)
	if task == nil || (taskID != "" && info.TaskID != taskID) {
		writeErrorKey(c, consts.StatusNotFound, "api.chat.noActiveTask")
		return
	}
	log.Printf("[interactive-agent-sse] attach active task_id=%s story_id=%s branch_id=%s status=%s", task.ID(), info.StoryID, info.BranchID, task.Status())
	sse.StreamTask(c, task)
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
	task, info := h.app.ActiveInteractiveTaskFor(storyID, branchID)
	if task == nil {
		writeJSON(c, consts.StatusOK, map[string]any{"active": false})
		return
	}
	status := task.Status()
	writeJSON(c, consts.StatusOK, map[string]any{
		"active":                  status == novaApp.TaskRunning,
		"status":                  status,
		"task_id":                 info.TaskID,
		"story_id":                info.StoryID,
		"branch_id":               info.BranchID,
		"message":                 info.Message,
		"regenerate_from_turn_id": info.RegenerateFromTurnID,
	})
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
		Workspace    string `json:"workspace"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if !h.ensurePresetMutationWorkspace(c, body.Workspace) {
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

func (h *Handlers) HandleStoryDirectors(ctx context.Context, c *app.RequestContext) {
	directors, err := h.app.StoryDirectors()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"directors": directors})
}

func (h *Handlers) HandleStoryDirector(ctx context.Context, c *app.RequestContext) {
	director, err := h.app.StoryDirector(c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, director)
}

func (h *Handlers) HandleStoryDirectorCreate(ctx context.Context, c *app.RequestContext) {
	var body interactive.StoryDirector
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	director, err := h.app.CreateStoryDirector(body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, director)
}

func (h *Handlers) HandleStoryDirectorUpdate(ctx context.Context, c *app.RequestContext) {
	var body struct {
		interactive.StoryDirector
		BaseRevision string `json:"base_revision"`
		Workspace    string `json:"workspace"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if !h.ensurePresetMutationWorkspace(c, body.Workspace) {
		return
	}
	director, err := h.app.UpdateStoryDirector(c.Param("id"), body.StoryDirector, body.BaseRevision)
	if err != nil {
		if errors.Is(err, interactive.ErrStoryDirectorRevisionConflict) {
			writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, director)
}

func (h *Handlers) HandleStoryDirectorDelete(ctx context.Context, c *app.RequestContext) {
	if err := h.app.DeleteStoryDirector(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleEventPackages(ctx context.Context, c *app.RequestContext) {
	items, err := h.app.EventPackages()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"event_packages": items})
}

func (h *Handlers) HandleEventPackage(ctx context.Context, c *app.RequestContext) {
	item, err := h.app.EventPackage(c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleEventPackageCreate(ctx context.Context, c *app.RequestContext) {
	var body interactive.EventPackageModule
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	item, err := h.app.CreateEventPackage(body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleEventPackageUpdate(ctx context.Context, c *app.RequestContext) {
	var body struct {
		interactive.EventPackageModule
		BaseRevision string `json:"base_revision"`
		Workspace    string `json:"workspace"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if !h.ensurePresetMutationWorkspace(c, body.Workspace) {
		return
	}
	item, err := h.app.UpdateEventPackage(c.Param("id"), body.EventPackageModule, body.BaseRevision)
	if err != nil {
		if errors.Is(err, interactive.ErrEventPackageRevisionConflict) {
			writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleEventPackageDelete(ctx context.Context, c *app.RequestContext) {
	if err := h.app.DeleteEventPackage(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleRuleSystems(ctx context.Context, c *app.RequestContext) {
	items, err := h.app.RuleSystems()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"rule_systems": items})
}

func (h *Handlers) HandleRuleSystem(ctx context.Context, c *app.RequestContext) {
	item, err := h.app.RuleSystem(c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleRuleSystemCreate(ctx context.Context, c *app.RequestContext) {
	var body interactive.RuleSystemModule
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	item, err := h.app.CreateRuleSystem(body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleRuleSystemUpdate(ctx context.Context, c *app.RequestContext) {
	var body struct {
		interactive.RuleSystemModule
		BaseRevision string `json:"base_revision"`
		Workspace    string `json:"workspace"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if !h.ensurePresetMutationWorkspace(c, body.Workspace) {
		return
	}
	item, err := h.app.UpdateRuleSystem(c.Param("id"), body.RuleSystemModule, body.BaseRevision)
	if err != nil {
		if errors.Is(err, interactive.ErrRuleSystemRevisionConflict) {
			writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleRuleSystemDelete(ctx context.Context, c *app.RequestContext) {
	if err := h.app.DeleteRuleSystem(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleActorStates(ctx context.Context, c *app.RequestContext) {
	items, err := h.app.ActorStates()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"actor_states": items})
}

func (h *Handlers) HandleActorState(ctx context.Context, c *app.RequestContext) {
	item, err := h.app.ActorState(c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleActorStateCreate(ctx context.Context, c *app.RequestContext) {
	var body interactive.ActorStateModule
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	item, err := h.app.CreateActorState(body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleActorStateUpdate(ctx context.Context, c *app.RequestContext) {
	var body struct {
		interactive.ActorStateModule
		BaseRevision string `json:"base_revision"`
		Workspace    string `json:"workspace"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if !h.ensurePresetMutationWorkspace(c, body.Workspace) {
		return
	}
	item, err := h.app.UpdateActorState(c.Param("id"), body.ActorStateModule, body.BaseRevision)
	if err != nil {
		if errors.Is(err, interactive.ErrActorStateRevisionConflict) {
			writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleActorStateDelete(ctx context.Context, c *app.RequestContext) {
	if err := h.app.DeleteActorState(c.Param("id")); err != nil {
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
		Workspace    string `json:"workspace"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if !h.ensurePresetMutationWorkspace(c, body.Workspace) {
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

func (h *Handlers) ensurePresetMutationWorkspace(c *app.RequestContext, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	current := strings.TrimSpace(h.app.Workspace())
	if current != "" && filepath.Clean(current) == filepath.Clean(expected) {
		return true
	}
	writeErrorKey(c, consts.StatusConflict, "api.workspace.changedDuringRequest")
	return false
}

func (h *Handlers) HandleImagePresetDelete(ctx context.Context, c *app.RequestContext) {
	if err := h.app.DeleteImagePreset(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}
