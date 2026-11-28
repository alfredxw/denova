package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/agents/modelio"
	"denova/internal/agents/modeltask"
	"denova/internal/api/sse"
	appsvc "denova/internal/app"
)

type inlineEditStreamRequest struct {
	Mode           string   `json:"mode"`
	Instruction    string   `json:"instruction"`
	Selection      string   `json:"selection,omitempty"`
	Before         string   `json:"before,omitempty"`
	After          string   `json:"after,omitempty"`
	PreviousResult string   `json:"previous_result,omitempty"`
	LoreIDs        []string `json:"lore_ids,omitempty"`
	IncludeLore    *bool    `json:"include_lore,omitempty"`
	IncludeStyle   *bool    `json:"include_style,omitempty"`
	StoryID        string   `json:"story_id,omitempty"`
}

type inlineEditDeltaEvent struct {
	Text string `json:"text"`
}

type inlineEditDoneEvent struct {
	Text string `json:"text"`
}

type inlineEditErrorEvent struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

// HandleProjectInlineEditStream POST /api/projects/:project_id/inline-edit/stream
// streams one stateless localized prose edit or insertion without creating a
// chat session turn or writing to workspace files.
func (h *Handlers) HandleProjectInlineEditStream(ctx context.Context, c *app.RequestContext) {
	layout, ok := requireProjectScope(c)
	if !ok {
		return
	}
	var body inlineEditStreamRequest
	if err := decodeStrictJSONRequest(c.Request.Body(), &body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.inlineEdit.invalidRequest", "detail", err.Error())
		return
	}

	prepared, err := h.app.PrepareProjectInlineEdit(ctx, layout, appsvc.InlineEditInput{
		Mode:           modeltask.InlineEditMode(body.Mode),
		Instruction:    body.Instruction,
		Selection:      body.Selection,
		Before:         body.Before,
		After:          body.After,
		PreviousResult: body.PreviousResult,
		LoreIDs:        body.LoreIDs,
		IncludeLore:    body.IncludeLore,
		IncludeStyle:   body.IncludeStyle,
		StoryID:        body.StoryID,
		Locale:         requestLocale(c),
	})
	if err != nil {
		writeInlineEditPrepareError(c, err)
		return
	}

	localizer := requestLocalizer(c)
	streamCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	writer := sse.NewSubscriptionStream(ctx, c, cancel)

	slog.InfoContext(ctx, "[api/inline-edit] stream open",
		"project_id", layout.ProjectID,
		"mode", body.Mode,
		"model", prepared.Meta.Model,
		"lore_count", prepared.Meta.LoreCount,
		"has_style", prepared.Meta.StyleName != "",
	)

	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.ErrorContext(streamCtx, "[api/inline-edit] stream panic recovered",
					"project_id", layout.ProjectID,
					"error", recovered,
				)
				_ = writeInlineEditSSE(writer, "error", inlineEditErrorEvent{
					Code:  "api.inlineEdit.failed",
					Error: localizer.T("api.inlineEdit.failed", "detail", fmt.Sprint(recovered)),
				})
			}
			cancel()
			_ = writer.Close()
		}()

		if err := writeInlineEditSSE(writer, "meta", prepared.Meta); err != nil {
			return
		}

		finalText, streamErr := h.app.StreamPreparedInlineEdit(streamCtx, prepared, func(delta string) error {
			return writeInlineEditSSE(writer, "delta", inlineEditDeltaEvent{Text: delta})
		})
		if streamErr != nil {
			if errors.Is(streamErr, context.Canceled) || errors.Is(streamErr, io.ErrClosedPipe) || streamCtx.Err() != nil {
				slog.DebugContext(streamCtx, "[api/inline-edit] stream cancelled", "project_id", layout.ProjectID)
				return
			}
			slog.WarnContext(streamCtx, "[api/inline-edit] stream failed",
				"project_id", layout.ProjectID,
				"error", streamErr,
			)
			code := "api.inlineEdit.failed"
			msg := localizer.T(code, "detail", streamErr.Error())
			if errors.Is(streamErr, modeltask.ErrInlineEditModelNotConfigured) {
				code = "api.inlineEdit.modelRequired"
				msg = localizer.T(code)
			} else if cls := modelio.ClassifyModelError(streamErr); cls.Class == modelio.ModelErrorAuthentication {
				code = "api.inlineEdit.authFailed"
				msg = localizer.T(code, "detail", streamErr.Error())
			}
			_ = writeInlineEditSSE(writer, "error", inlineEditErrorEvent{Code: code, Error: msg})
			return
		}

		_ = writeInlineEditSSE(writer, "done", inlineEditDoneEvent{Text: finalText})
	}()
}

func writeInlineEditPrepareError(c *app.RequestContext, err error) {
	switch {
	case errors.Is(err, modeltask.ErrInlineEditInvalidMode):
		writeErrorKey(c, consts.StatusBadRequest, "api.inlineEdit.invalidMode")
	case errors.Is(err, modeltask.ErrInlineEditInstructionRequired):
		writeErrorKey(c, consts.StatusBadRequest, "api.inlineEdit.instructionRequired")
	case errors.Is(err, modeltask.ErrInlineEditSelectionTooLong):
		writeErrorKey(c, consts.StatusBadRequest, "api.inlineEdit.selectionTooLong")
	case errors.Is(err, modeltask.ErrInlineEditModelNotConfigured):
		writeErrorKey(c, consts.StatusUnprocessableEntity, "api.inlineEdit.modelRequired")
	default:
		writeErrorKey(c, consts.StatusBadRequest, "api.inlineEdit.failed", "detail", err.Error())
	}
}

func writeInlineEditSSE(w io.Writer, event string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
	return err
}
