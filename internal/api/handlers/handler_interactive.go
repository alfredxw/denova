package handlers

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	appsvc "denova/internal/app"
	imageapp "denova/internal/app/image"
	imagepreset "denova/internal/image/preset"
	"denova/internal/interactive"
	"denova/internal/interactive/teller"
	"denova/internal/style"
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
		if errors.Is(err, appsvc.ErrAgentOperationActive) {
			writeErrorKey(c, consts.StatusConflict, "api.interactive.storyStructureBusy")
			return
		}
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

func (h *Handlers) HandleInteractiveHistory(ctx context.Context, c *app.RequestContext) {
	limit := defaultInteractiveHistoryPageSize
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidQuery")
			return
		}
		limit = parsed
	}
	page, err := h.app.InteractiveHistoryPage(c.Param("id"), c.Query("branch"), c.Query("before"), limit)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, page)
}

const defaultInteractiveHistoryPageSize = 100

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

func (h *Handlers) HandleInteractiveImageGenerate(ctx context.Context, c *app.RequestContext) {
	var body interactive.InteractiveImageGenerateRequest
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	if strings.TrimSpace(body.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试请求 / command_id is required for safe request retries", nil)
		return
	}
	if err := appsvc.ValidateAgentCommandID(body.CommandID); err != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "command_id 请求标识无效 / invalid request identifier command_id", nil)
		return
	}
	if body.BranchID == "" {
		body.BranchID = c.Query("branch")
	}
	result, err := h.app.Images().GenerateInteractiveImage(ctx, c.Param("id"), body)
	if err != nil {
		switch {
		case errors.Is(err, appsvc.ErrAgentCommandIDRequired):
			writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id，无法安全重试请求 / command_id is required for safe request retries", nil)
		case errors.Is(err, appsvc.ErrInvalidAgentCommand):
			writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "command_id 请求标识无效 / invalid request identifier command_id", nil)
		case errors.Is(err, appsvc.ErrAgentCommandConflict):
			writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.command_conflict", "command_id 已用于其他请求 / command_id was already used for a different request", nil)
		case errors.Is(err, imageapp.ErrExecution):
			writeError(c, consts.StatusInternalServerError, err.Error())
		default:
			writeError(c, consts.StatusBadRequest, err.Error())
		}
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

func (h *Handlers) HandleInteractiveBranchPlanUpdate(ctx context.Context, c *app.RequestContext) {
	var body interactive.UpdateBranchPlanRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	body.BranchID = c.Param("branch")
	if strings.TrimSpace(body.Markdown) == "" {
		writeError(c, consts.StatusBadRequest, "规划文档不能为空 / Branch plan cannot be empty")
		return
	}
	result, err := h.app.UpdateInteractiveBranchPlan(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusConflict, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleInteractiveContextCompaction(ctx context.Context, c *app.RequestContext) {
	var body struct {
		CommandID string `json:"command_id"`
		BranchID  string `json:"branch_id"`
		Branch    string `json:"branch"`
	}
	if err := c.BindJSON(&body); err != nil && len(c.Request.Body()) > 0 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	branchID := body.BranchID
	if strings.TrimSpace(branchID) == "" {
		branchID = body.Branch
	}
	body.CommandID = strings.TrimSpace(body.CommandID)
	if err := appsvc.ValidateAgentCommandID(body.CommandID); err != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少或无效的 command_id，无法安全重试 / command_id is required and must be valid for safe retries", nil)
		return
	}
	result, err := h.app.CompactInteractiveContextCommand(ctx, c.Param("id"), branchID, body.CommandID)
	if err != nil {
		writeError(c, consts.StatusConflict, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleInteractiveContextCompactionRemove(ctx context.Context, c *app.RequestContext) {
	commandID := strings.TrimSpace(c.Query("command_id"))
	if err := appsvc.ValidateAgentCommandID(commandID); err != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少或无效的 command_id，无法安全重试 / command_id is required and must be valid for safe retries", nil)
		return
	}
	removed, err := h.app.RemoveInteractiveContextCompactionCommand(ctx, c.Param("id"), c.Query("branch"), commandID)
	if err != nil {
		writeError(c, consts.StatusConflict, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]bool{"removed": removed})
}

func (h *Handlers) HandleInteractiveTellers(ctx context.Context, c *app.RequestContext) {
	tellers, err := h.app.ResourceCatalog().Tellers()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"tellers": tellers, "default_id": style.DefaultID})
}

func (h *Handlers) HandleInteractiveTeller(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	teller, err := h.app.ResourceCatalog().Teller(id)
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, teller)
}

func (h *Handlers) HandleInteractiveTellerCreate(ctx context.Context, c *app.RequestContext) {
	var body teller.Definition
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	teller, err := h.app.ResourceCatalog().CreateTeller(body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, teller)
}

func (h *Handlers) HandleInteractiveTellerUpdate(ctx context.Context, c *app.RequestContext) {
	var body struct {
		teller.Definition
		BaseRevision string `json:"base_revision"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	updated, err := h.app.ResourceCatalog().UpdateTeller(c.Param("id"), body.Definition, body.BaseRevision)
	if err != nil {
		if errors.Is(err, teller.ErrRevisionConflict) {
			writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, updated)
}

func (h *Handlers) HandleInteractiveTellerDelete(ctx context.Context, c *app.RequestContext) {
	if err := h.app.ResourceCatalog().DeleteTeller(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleGamePlanningTemplates(ctx context.Context, c *app.RequestContext) {
	items, err := h.app.ResourceCatalog().GamePlanningTemplates()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"game_planning_templates": items})
}

func (h *Handlers) HandleGamePlanningTemplate(ctx context.Context, c *app.RequestContext) {
	item, err := h.app.ResourceCatalog().GamePlanningTemplate(c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleGamePlanningTemplateCreate(ctx context.Context, c *app.RequestContext) {
	var body interactive.GamePlanningTemplate
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	item, err := h.app.ResourceCatalog().CreateGamePlanningTemplate(body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleGamePlanningTemplateUpdate(ctx context.Context, c *app.RequestContext) {
	var body struct {
		interactive.GamePlanningTemplate
		BaseRevision string `json:"base_revision"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	item, err := h.app.ResourceCatalog().UpdateGamePlanningTemplate(c.Param("id"), body.GamePlanningTemplate, body.BaseRevision)
	if err != nil {
		if errors.Is(err, interactive.ErrGamePlanningTemplateRevisionConflict) {
			writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleGamePlanningTemplateDelete(ctx context.Context, c *app.RequestContext) {
	if err := h.app.ResourceCatalog().DeleteGamePlanningTemplate(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleEventPackages(ctx context.Context, c *app.RequestContext) {
	items, err := h.app.ResourceCatalog().EventPackages()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"event_packages": items})
}

func (h *Handlers) HandleEventPackage(ctx context.Context, c *app.RequestContext) {
	item, err := h.app.ResourceCatalog().EventPackage(c.Param("id"))
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
	item, err := h.app.ResourceCatalog().CreateEventPackage(body)
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
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	item, err := h.app.ResourceCatalog().UpdateEventPackage(c.Param("id"), body.EventPackageModule, body.BaseRevision)
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
	if err := h.app.ResourceCatalog().DeleteEventPackage(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleRuleSystems(ctx context.Context, c *app.RequestContext) {
	items, err := h.app.ResourceCatalog().RuleSystems()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"rule_systems": items})
}

func (h *Handlers) HandleRuleSystem(ctx context.Context, c *app.RequestContext) {
	item, err := h.app.ResourceCatalog().RuleSystem(c.Param("id"))
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
	item, err := h.app.ResourceCatalog().CreateRuleSystem(body)
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
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	item, err := h.app.ResourceCatalog().UpdateRuleSystem(c.Param("id"), body.RuleSystemModule, body.BaseRevision)
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
	if err := h.app.ResourceCatalog().DeleteRuleSystem(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleActorStates(ctx context.Context, c *app.RequestContext) {
	items, err := h.app.ResourceCatalog().ActorStates()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"actor_states": items})
}

func (h *Handlers) HandleActorState(ctx context.Context, c *app.RequestContext) {
	item, err := h.app.ResourceCatalog().ActorState(c.Param("id"))
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
	item, err := h.app.ResourceCatalog().CreateActorState(body)
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
	}
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	item, err := h.app.ResourceCatalog().UpdateActorState(c.Param("id"), body.ActorStateModule, body.BaseRevision)
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
	if err := h.app.ResourceCatalog().DeleteActorState(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleImagePresets(ctx context.Context, c *app.RequestContext) {
	presets, err := h.app.ResourceCatalog().ImagePresets()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"presets": presets})
}

func (h *Handlers) HandleImagePreset(ctx context.Context, c *app.RequestContext) {
	preset, err := h.app.ResourceCatalog().ImagePreset(c.Param("id"))
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
	preset, err := h.app.ResourceCatalog().CreateImagePreset(body)
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
	preset, err := h.app.ResourceCatalog().UpdateImagePreset(c.Param("id"), body.Preset, body.BaseRevision)
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
	if err := h.app.ResourceCatalog().DeleteImagePreset(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}
