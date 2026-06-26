package handlers

import (
	"context"
	"log"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/internal/api/sse"
)

func (h *Handlers) HandleUpdateCheck(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.CheckUpdate(ctx)
	if err != nil {
		writeError(c, consts.StatusBadGateway, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleUpdateInstall(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.InstallUpdate(ctx)
	if err != nil {
		writeError(c, consts.StatusBadGateway, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleUpdateApply(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.ApplyUpdate(ctx)
	if err != nil {
		writeError(c, consts.StatusBadGateway, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleUpdateInstallStream(ctx context.Context, c *app.RequestContext) {
	task := h.app.StartInstallUpdateTask()
	log.Printf("[update-sse] attach install task_id=%s", task.ID())
	sse.StreamTask(c, task)
}
