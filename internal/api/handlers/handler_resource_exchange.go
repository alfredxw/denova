package handlers

import (
	"context"
	"denova/internal/revisionfile"
	"errors"
	"io"
	"log/slog"
	"strings"

	"denova/internal/app/resourceexchange"
	"denova/internal/platform"
	"github.com/cloudwego/hertz/pkg/app"
)

func exchangeError(ctx context.Context, c *app.RequestContext, err error) {
	slog.WarnContext(ctx, "resource_exchange_failed", "error", err)
	key, status := "market.errors.operationFailed", 400
	var conflict *revisionfile.ConflictError
	switch {
	case errors.As(err, &conflict):
		key, status = "api.resource.revisionConflict", 409
	case errors.Is(err, resourceexchange.ErrLocalModified):
		key, status = "market.errors.localModified", 409
	case errors.Is(err, resourceexchange.ErrResourceOwned):
		key, status = "market.errors.resourceOwned", 409
	case errors.Is(err, resourceexchange.ErrSkillExists):
		key, status = "market.errors.skillExists", 409
	case errors.Is(err, resourceexchange.ErrSourceChanged):
		key, status = "market.errors.sourceChanged", 409
	case errors.Is(err, resourceexchange.ErrBundleOwned):
		key, status = "market.errors.bundleOwned", 409
	case errors.Is(err, resourceexchange.ErrResourcesBusy):
		key, status = "market.errors.busy", 409
	case errors.Is(err, resourceexchange.ErrReferenceChanged):
		key, status = "market.errors.referenceChanged", 409
	}
	writeErrorKey(c, status, key)
}
func (h *Handlers) HandleResourcePreview(ctx context.Context, c *app.RequestContext) {
	source := resourceexchange.Source{}
	var raw []byte
	if strings.HasPrefix(string(c.ContentType()), "multipart/form-data") {
		header, err := c.FormFile("file")
		if err != nil || header.Size > platform.MaxPackageBytes {
			writeErrorKey(c, 400, "market.errors.invalidFile")
			return
		}
		file, err := header.Open()
		if err != nil {
			exchangeError(ctx, c, err)
			return
		}
		defer file.Close()
		raw, err = io.ReadAll(io.LimitReader(file, platform.MaxPackageBytes+1))
		if err != nil || len(raw) > platform.MaxPackageBytes {
			writeErrorKey(c, 400, "market.errors.invalidFile")
			return
		}
		source = resourceexchange.Source{Kind: "file", Filename: header.Filename}
	} else {
		if err := c.BindJSON(&source); err != nil {
			exchangeError(ctx, c, err)
			return
		}
		source.Commit = ""
		source.Filename = ""
	}
	result, err := h.app.ResourceExchange().Preview(ctx, source, raw)
	if err != nil {
		exchangeError(ctx, c, err)
		return
	}
	writeJSON(c, 200, result)
}
func (h *Handlers) HandleResourcePlan(ctx context.Context, c *app.RequestContext) {
	var request resourceexchange.PlanRequest
	if err := c.BindJSON(&request); err != nil {
		exchangeError(ctx, c, err)
		return
	}
	result, err := h.app.ResourceExchange().Plan(ctx, request)
	if err != nil {
		exchangeError(ctx, c, err)
		return
	}
	writeJSON(c, 200, result.PublicPlan())
}
func (h *Handlers) HandleResourceApply(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.ApplyResourcePlan(ctx, c.Param("id"))
	if err != nil {
		exchangeError(ctx, c, err)
		return
	}
	writeJSON(c, 200, result)
}
func (h *Handlers) HandleResourceInstallations(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.ResourceExchange().Installations(ctx)
	if err != nil {
		exchangeError(ctx, c, err)
		return
	}
	writeJSON(c, 200, result)
}
func (h *Handlers) HandleResourceDetach(ctx context.Context, c *app.RequestContext) {
	if err := h.app.ResourceExchange().Detach(ctx, c.Param("id")); err != nil {
		exchangeError(ctx, c, err)
		return
	}
	writeJSON(c, 200, map[string]bool{"ok": true})
}
func (h *Handlers) HandleResourceCheckUpdate(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.ResourceExchange().CheckUpdate(ctx, c.Param("id"))
	if err != nil {
		exchangeError(ctx, c, err)
		return
	}
	writeJSON(c, 200, result)
}
func (h *Handlers) HandleResourceDiscard(ctx context.Context, c *app.RequestContext) {
	if err := h.app.ResourceExchange().DiscardPreview(c.Param("id")); err != nil {
		exchangeError(ctx, c, err)
		return
	}
	writeJSON(c, 200, map[string]bool{"ok": true})
}

func (h *Handlers) HandleResourceExportChoices(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.ResourceExchange().ExportResources(ctx, c.Query("project_id"))
	if err != nil {
		exchangeError(ctx, c, err)
		return
	}
	writeJSON(c, 200, result)
}
func (h *Handlers) HandleResourceExport(ctx context.Context, c *app.RequestContext) {
	var request resourceexchange.ExportRequest
	if err := c.BindJSON(&request); err != nil {
		exchangeError(ctx, c, err)
		return
	}
	raw, err := h.app.ResourceExchange().Export(ctx, request)
	if err != nil {
		exchangeError(ctx, c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=denova-resources.zip")
	c.Data(200, "application/zip", raw)
}

func (h *Handlers) HandleResourceUpdateMode(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Mode string `json:"update_mode"`
	}
	if err := c.BindJSON(&body); err != nil {
		exchangeError(ctx, c, err)
		return
	}
	if err := h.app.ResourceExchange().SetUpdateMode(ctx, c.Param("id"), body.Mode); err != nil {
		exchangeError(ctx, c, err)
		return
	}
	writeJSON(c, 200, map[string]bool{"ok": true})
}

func (h *Handlers) HandleResourceExportPlan(ctx context.Context, c *app.RequestContext) {
	var request resourceexchange.ExportRequest
	if err := c.BindJSON(&request); err != nil {
		exchangeError(ctx, c, err)
		return
	}
	plan, err := h.app.ResourceExchange().PrepareExport(ctx, request)
	if err != nil {
		exchangeError(ctx, c, err)
		return
	}
	writeJSON(c, 200, plan)
}
func (h *Handlers) HandleResourceExportDownload(ctx context.Context, c *app.RequestContext) {
	raw, err := h.app.ResourceExchange().ReadExport(ctx, c.Param("id"))
	if err != nil {
		exchangeError(ctx, c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=denova-resources.zip")
	c.Data(200, "application/zip", raw)
}
func (h *Handlers) HandleResourceExportDefinitions(ctx context.Context, c *app.RequestContext) {
	if string(c.Method()) == "GET" {
		items, err := h.app.ResourceExchange().ExportDefinitions(ctx)
		if err != nil {
			exchangeError(ctx, c, err)
			return
		}
		writeJSON(c, 200, items)
		return
	}
	var definition resourceexchange.ExportDefinition
	if err := c.BindJSON(&definition); err != nil {
		exchangeError(ctx, c, err)
		return
	}
	result, err := h.app.ResourceExchange().SaveExportDefinition(ctx, definition)
	if err != nil {
		exchangeError(ctx, c, err)
		return
	}
	writeJSON(c, 200, result)
}
func (h *Handlers) HandleResourceExportDefinitionDelete(ctx context.Context, c *app.RequestContext) {
	if err := h.app.ResourceExchange().DeleteExportDefinition(ctx, c.Param("id"), c.Query("revision")); err != nil {
		exchangeError(ctx, c, err)
		return
	}
	writeJSON(c, 200, map[string]bool{"ok": true})
}

func (h *Handlers) HandleResourceBackups(ctx context.Context, c *app.RequestContext) {
	items, err := h.app.ResourceExchange().Backups(ctx, c.Param("id"))
	if err != nil {
		exchangeError(ctx, c, err)
		return
	}
	writeJSON(c, 200, items)
}
func (h *Handlers) HandleResourceBackupDownload(ctx context.Context, c *app.RequestContext) {
	raw, err := h.app.ResourceExchange().DownloadBackup(ctx, c.Param("id"))
	if err != nil {
		exchangeError(ctx, c, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=denova-installation-backup.zip")
	c.Data(200, "application/zip", raw)
}
func (h *Handlers) HandleResourceBackupPlan(ctx context.Context, c *app.RequestContext) {
	plan, err := h.app.ResourceExchange().PlanRestore(ctx, c.Param("id"))
	if err != nil {
		exchangeError(ctx, c, err)
		return
	}
	writeJSON(c, 200, plan.PublicPlan())
}

func (h *Handlers) HandleResourcePreviewFiles(ctx context.Context, c *app.RequestContext) {
	result, err := h.app.ResourceExchange().PreviewFiles(ctx, c.Param("id"), c.Query("candidate_id"), c.Query("resource_id"), c.Query("path"))
	if err != nil {
		exchangeError(ctx, c, err)
		return
	}
	writeJSON(c, 200, result)
}
