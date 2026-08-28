package handlers

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

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
	if err := decodeStrictJSONRequest(c.Request.Body(), &request); err != nil {
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

func (h *Handlers) HandleContinualLearningPublish(ctx context.Context, c *app.RequestContext) {
	var request continuallearningapp.StatePublishRequest
	if err := decodeStrictJSONRequest(c.Request.Body(), &request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	result, err := h.app.ContinualLearning().Publish(ctx, request)
	if err != nil {
		writeContinualLearningError(c, err)
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleContinualLearningDebug(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.ContinualLearning().Debug(ctx, c.Query("agent_kind"), c.Query("revision"))
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
	var validation *continuallearningapp.StateValidationError
	switch {
	case errors.Is(err, continuallearningapp.ErrDisabled):
		writeErrorKey(c, consts.StatusNotFound, "api.continualLearning.disabled")
	case errors.Is(err, continuallearningapp.ErrStateVersionNotFound):
		writeErrorKey(c, consts.StatusNotFound, "api.continualLearning.versionNotFound")
	case errors.Is(err, continuallearningapp.ErrStateConflict):
		writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
	case errors.As(err, &validation):
		writeJSON(c, consts.StatusUnprocessableEntity, map[string]any{
			"error":   messageKey(c, "api.common.invalidRequestWithDetail", "detail", validation.Error()),
			"code":    "harness_state_validation_failed",
			"details": map[string]any{"diagnostics": validation.Diagnostics},
		})
	default:
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
	}
}
