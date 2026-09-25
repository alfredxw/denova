package handlers

import (
	"context"
	"log/slog"

	"denova/internal/app/resourcecatalog"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func (h *Handlers) HandleSkillPreference(ctx context.Context, c *app.RequestContext) {
	var change resourcecatalog.SkillPreferenceChange
	if err := c.BindJSON(&change); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.preferenceFailed", "detail", err.Error())
		return
	}
	snapshot, err := h.app.ResourceCatalog().SetSkillPreference(ctx, skillTarget(c), change)
	if err != nil {
		slog.ErrorContext(ctx, "Change Skill preference failed", "error", err)
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.preferenceFailed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, snapshot)
}
