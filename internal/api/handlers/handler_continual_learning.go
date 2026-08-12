package handlers

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/api/agentui"
	"denova/internal/api/sse"
	appsvc "denova/internal/app"
	continuallearningapp "denova/internal/app/continuallearning"
)

func (h *Handlers) HandleContinualLearningState(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.ContinualLearning().State(ctx)
	if err != nil {
		writeContinualLearningError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleContinualLearningStateUpdate(ctx context.Context, c *app.RequestContext) {
	var request continuallearningapp.StateUpdateRequest
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	result, err := h.app.ContinualLearning().UpdateState(ctx, request)
	if err != nil {
		writeContinualLearningError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleContinualLearningVersions(ctx context.Context, c *app.RequestContext) {
	limit, ok := optionalPositiveLimit(c, 100)
	if !ok {
		return
	}
	result, err := h.app.ContinualLearning().Versions(ctx, limit)
	if err != nil {
		writeContinualLearningError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleContinualLearningVersionDiff(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.ContinualLearning().Diff(
		ctx, continuallearningapp.StateVersionID(strings.TrimSpace(c.Query("from"))), continuallearningapp.StateVersionID(strings.TrimSpace(c.Query("to"))),
	)
	if err != nil {
		writeContinualLearningError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleContinualLearningVersionRestore(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.ContinualLearning().Restore(ctx, continuallearningapp.StateVersionID(strings.TrimSpace(c.Param("id"))))
	if err != nil {
		writeContinualLearningError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleHarnessOptimizerStream(ctx context.Context, c *app.RequestContext) {
	var request continuallearningapp.Request
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	if strings.TrimSpace(request.CommandID) == "" {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "缺少 command_id / command_id is required", nil)
		return
	}
	if err := appsvc.ValidateAgentCommandID(request.CommandID); err != nil {
		writeAgentRuntimeError(c, consts.StatusBadRequest, "agent_runtime.invalid_command", "command_id 请求标识无效 / invalid command_id", nil)
		return
	}
	request.Trigger = continuallearningapp.TriggerManual
	request.Locale = requestLocale(c)
	task, err := h.app.ContinualLearning().StartTask(ctx, request)
	if err != nil {
		h.writeChatPreparationError(c, err)
		return
	}
	sse.StreamTaskUI(ctx, c, task)
}

func (h *Handlers) HandleHarnessOptimizerTaskStream(ctx context.Context, c *app.RequestContext) {
	task := h.app.ContinualLearning().DisplayTask(strings.TrimSpace(c.Query("task_id")))
	if task == nil {
		writeAgentRuntimeError(c, consts.StatusConflict, "agent_runtime.rehydrate_required", "任务流已失效 / The task stream is stale", nil)
		return
	}
	sse.StreamTaskUI(ctx, c, task)
}

func (h *Handlers) HandleHarnessOptimizerActive(ctx context.Context, c *app.RequestContext) {
	if err := h.app.ContinualLearning().CheckEnabled(); err != nil {
		writeContinualLearningError(c, err)
		return
	}
	response := map[string]any{"active": false}
	if task := h.app.ContinualLearning().ActiveTask(); task != nil {
		snapshot := task.Snapshot()
		response["active"] = !snapshot.Finished
		response["status"] = snapshot.Status
		response["task_id"] = snapshot.ID
		response["stream_cursor"] = snapshot.Cursor
	}
	if ask := h.app.ContinualLearning().PendingAsk(); ask != nil {
		response["pending_ask"] = ask
	}
	status, available := h.app.ContinualLearning().RuntimeStatus(ctx)
	addAgentRuntimeProjection(response, status, agentRuntimeProjectionOptions{Available: available, StreamAttached: response["active"] == true})
	writeJSON(c, consts.StatusOK, response)
}

func (h *Handlers) HandleHarnessOptimizerMessages(ctx context.Context, c *app.RequestContext) {
	limit, ok := optionalPositiveLimit(c, maxSessionMessagePageSize)
	if !ok {
		return
	}
	before := -1
	if raw := strings.TrimSpace(c.Query("before")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidQuery")
			return
		}
		before = parsed
	}
	page, err := h.app.ContinualLearning().Messages(ctx, before, min(limit, maxSessionMessagePageSize))
	if err != nil {
		writeContinualLearningError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, sessionMessagesPageDTO{
		Messages: agentui.MessagesFromHistoryAtOffset(page.Entries, page.NextBefore),
		Page:     sessionMessagePageMeta{NextBefore: strconv.Itoa(page.NextBefore), HasMore: page.HasMore, Total: page.Total},
	})
}

func (h *Handlers) HandleHarnessOptimizerClear(ctx context.Context, c *app.RequestContext) {
	if err := h.app.ContinualLearning().Clear(ctx); err != nil {
		writeContinualLearningError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) HandleHarnessOptimizerAskAnswer(ctx context.Context, c *app.RequestContext) {
	var request askAnswerRequest
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	result, err := h.app.ContinualLearning().AnswerAsk(ctx, strings.TrimSpace(c.Param("ask_id")), request.Answers)
	if err != nil {
		writeAskResolutionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleHarnessOptimizerAskCancel(ctx context.Context, c *app.RequestContext) {
	var request askCancelRequest
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	result, err := h.app.ContinualLearning().CancelAsk(ctx, strings.TrimSpace(c.Param("ask_id")), request.Reason)
	if err != nil {
		writeAskResolutionError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleContinualLearningSchedule(_ context.Context, c *app.RequestContext) {
	result, err := h.app.ContinualLearning().ScheduleStatus()
	if err != nil {
		writeContinualLearningError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleContinualLearningOutcomes(_ context.Context, c *app.RequestContext) {
	limit, ok := optionalPositiveLimit(c, 50)
	if !ok {
		return
	}
	result, err := h.app.ContinualLearning().Outcomes(limit)
	if err != nil {
		writeContinualLearningError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleContinualLearningOutcomeCreate(_ context.Context, c *app.RequestContext) {
	var outcome continuallearningapp.Outcome
	if err := c.BindJSON(&outcome); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	result, err := h.app.ContinualLearning().RecordOutcome(outcome)
	if err != nil {
		writeContinualLearningError(c, err)
		return
	}
	writeJSON(c, consts.StatusCreated, result)
}

func optionalPositiveLimit(c *app.RequestContext, fallback int) (int, bool) {
	raw := strings.TrimSpace(c.Query("limit"))
	if raw == "" {
		return fallback, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 || limit > 500 {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidQuery")
		return 0, false
	}
	return limit, true
}

func writeContinualLearningError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, continuallearningapp.ErrDisabled):
		writeErrorKey(c, consts.StatusNotFound, "api.continualLearning.disabled")
	case errors.Is(err, continuallearningapp.ErrStateVersionNotFound):
		writeErrorKey(c, consts.StatusNotFound, "api.continualLearning.versionNotFound")
	case errors.Is(err, continuallearningapp.ErrStateConflict):
		writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
	default:
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
	}
}
