package handlers

import (
	"context"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/agents/trajectory"
)

func (h *Handlers) HandleTrajectoryOutcomes(_ context.Context, c *app.RequestContext) {
	limit, ok := optionalPositiveLimit(c, 50)
	if !ok {
		return
	}
	result, err := h.app.TrajectoryOutcomes().List(limit)
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleTrajectoryOutcomeCreate(_ context.Context, c *app.RequestContext) {
	var outcome trajectory.Outcome
	if err := decodeStrictJSONRequest(c.Request.Body(), &outcome); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidBody")
		return
	}
	result, err := h.app.TrajectoryOutcomes().Append(outcome)
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
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
		writeErrorKey(c, consts.StatusBadRequest, "api.trajectory.invalidLimit")
		return 0, false
	}
	return limit, true
}
