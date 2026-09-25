package handlers

import (
	"context"
	"log/slog"

	"denova/internal/agents/skills"
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

func (h *Handlers) HandleSkillUpdates(ctx context.Context, c *app.RequestContext) {
	var request struct {
		Scope  skills.Scope        `json:"scope"`
		Name   string              `json:"name"`
		Action skills.UpdateAction `json:"action"`
	}
	if err := c.BindJSON(&request); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.updateFailed", "detail", err.Error())
		return
	}
	results, err := h.app.ResourceCatalog().RefreshSkillUpdates(ctx, skillTarget(c), request.Scope, request.Name, request.Action)
	if err != nil {
		slog.ErrorContext(ctx, "Refresh Skill updates failed", "error", err)
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.updateFailed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, results)
}
