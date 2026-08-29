package handlers

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	agentattachment "denova/internal/agents/attachment"
)

// HandleAttachmentImage serves one project- and conversation-scoped native
// image without exposing its application-owned filesystem path.
func (h *Handlers) HandleAttachmentImage(ctx context.Context, c *app.RequestContext) {
	project, ok := requireProjectScope(c)
	if !ok {
		return
	}
	scopeID := strings.TrimSpace(c.Query("scope_id"))
	attachmentID := strings.TrimSpace(c.Param("attachment_id"))
	if scopeID == "" || attachmentID == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.attachments.scopeRequired")
		return
	}
	var scope agentattachment.Scope
	switch strings.TrimSpace(c.Query("scope")) {
	case "session":
		scope = agentattachment.SessionScope(scopeID)
	case "story":
		scope = agentattachment.StoryScope(scopeID)
	default:
		writeErrorKey(c, consts.StatusBadRequest, "api.attachments.scopeRequired")
		return
	}
	image, err := agentattachment.ReadImage(project.StateRoot, scope, attachmentID)
	if err != nil {
		status := consts.StatusInternalServerError
		if errors.Is(err, agentattachment.ErrImageNotFound) || errors.Is(err, agentattachment.ErrImagePreviewDisabled) {
			status = consts.StatusNotFound
		}
		slog.WarnContext(ctx, "[internal/api/handlers/handler_attachments.go] attachment image preview failed",
			"project_id", project.ProjectID,
			"scope", scope.Kind,
			"scope_id", scope.ID,
			"attachment_id", attachmentID,
			"error", err,
		)
		writeErrorKey(c, status, "api.attachments.previewUnavailable")
		return
	}
	c.Response.Header.Set("Cache-Control", "private, max-age=31536000, immutable")
	c.Response.Header.Set("ETag", `"`+image.SHA256+`"`)
	c.Response.Header.Set("X-Content-Type-Options", "nosniff")
	c.Data(consts.StatusOK, image.MediaType, image.Data)
}
