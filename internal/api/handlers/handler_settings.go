package handlers

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/config"
)

// handleSettingsGet GET /api/settings — 返回三层配置快照。
func (h *Handlers) HandleSettingsGet(ctx context.Context, c *app.RequestContext) {
	layered, err := h.app.Settings()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, layered)
}

// handleSettingsUserUpdate PUT /api/settings/user — 持久化用户级配置。
func (h *Handlers) HandleSettingsUserUpdate(ctx context.Context, c *app.RequestContext) {
	var body config.Settings
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	layered, err := h.app.UpdateUserSettings(body)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, layered)
}

// handleSettingsWorkspaceUpdate PUT /api/settings/workspace — 持久化工作区级配置。
func (h *Handlers) HandleSettingsWorkspaceUpdate(ctx context.Context, c *app.RequestContext) {
	var body config.Settings
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	layered, err := h.app.UpdateWorkspaceSettings(body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, layered)
}
