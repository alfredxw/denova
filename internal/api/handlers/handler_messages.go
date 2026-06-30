package handlers

import (
	"context"
	"net/url"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
)

func (h *Handlers) HandleMessages(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.Messages()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleMessageRead(ctx context.Context, c *app.RequestContext) {
	id, unescapeErr := url.PathUnescape(c.Param("id"))
	if unescapeErr != nil {
		writeError(c, consts.StatusBadRequest, unescapeErr.Error())
		return
	}
	item, err := h.app.MarkMessageRead(id)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, item)
}

func (h *Handlers) HandleMessagesReadAll(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.MarkAllMessagesRead()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}
