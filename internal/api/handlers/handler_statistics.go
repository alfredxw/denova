package handlers

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // Keep viewer date boundaries available in native release binaries.

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func (h *Handlers) HandleStatistics(ctx context.Context, c *app.RequestContext) {
	days, err := strconv.Atoi(c.DefaultQuery("days", "7"))
	if err != nil || (days != 1 && days != 7 && days != 30 && days != 90) {
		writeErrorKey(c, consts.StatusBadRequest, "api.statistics.invalidRange")
		return
	}
	location, err := time.LoadLocation(c.DefaultQuery("timezone", "UTC"))
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.statistics.invalidRange")
		return
	}
	report, err := h.app.Statistics(ctx, strings.TrimSpace(c.Query("project_id")), days, location)
	if err != nil {
		slog.ErrorContext(ctx, "Load statistics failed", "error", err)
		writeErrorKey(c, consts.StatusInternalServerError, "api.statistics.loadFailed")
		return
	}
	c.Response.Header.Set("Cache-Control", "no-store")
	writeJSON(c, consts.StatusOK, report)
}
