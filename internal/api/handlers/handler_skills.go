package handlers

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	appsvc "denova/internal/app"
	resourcecatalogapp "denova/internal/app/resourcecatalog"
)

type skillCreateRequest struct {
	Scope        appsvc.SkillScope `json:"scope"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Agents       []string          `json:"agents"`
	Category     string            `json:"category"`
	Capabilities []string          `json:"capabilities"`
}

type skillSaveRequest struct {
	Scope        appsvc.SkillScope `json:"scope"`
	Name         string            `json:"name"`
	Content      string            `json:"content"`
	TargetScope  appsvc.SkillScope `json:"target_scope"`
	TargetName   string            `json:"target_name"`
	BaseRevision string            `json:"base_revision"`
}

type skillFileSaveRequest struct {
	Scope        appsvc.SkillScope `json:"scope"`
	Name         string            `json:"name"`
	Path         string            `json:"path"`
	Content      string            `json:"content"`
	BaseRevision string            `json:"base_revision"`
}

func (h *Handlers) HandleSkills(ctx context.Context, c *app.RequestContext) {
	snapshot, err := h.app.ResourceCatalog().SkillSnapshot(ctx, skillTarget(c))
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, snapshot)
}

func (h *Handlers) HandleSkillDocument(ctx context.Context, c *app.RequestContext) {
	scope := appsvc.SkillScope(strings.TrimSpace(c.Query("scope")))
	name := strings.TrimSpace(c.Query("name"))
	if scope == "" || name == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.scopeNameRequired")
		return
	}
	doc, err := h.app.ResourceCatalog().SkillDocument(ctx, skillTarget(c), scope, name)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, doc)
}

func (h *Handlers) HandleSkillFileDocument(ctx context.Context, c *app.RequestContext) {
	scope := appsvc.SkillScope(strings.TrimSpace(c.Query("scope")))
	name := strings.TrimSpace(c.Query("name"))
	path := strings.TrimSpace(c.Query("path"))
	if scope == "" || name == "" || path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.scopeNamePathRequired")
		return
	}
	doc, err := h.app.ResourceCatalog().SkillFileDocument(ctx, skillTarget(c), scope, name, path)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, doc)
}

func (h *Handlers) HandleSkillCreate(ctx context.Context, c *app.RequestContext) {
	var body skillCreateRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	body.Scope = appsvc.SkillScope(strings.TrimSpace(string(body.Scope)))
	body.Name = strings.TrimSpace(body.Name)
	doc, err := h.app.ResourceCatalog().CreateSkill(ctx, skillTarget(c), body.Scope, body.Name, appsvc.SkillCreateMetadata{
		Description:  body.Description,
		Agents:       body.Agents,
		Category:     body.Category,
		Capabilities: body.Capabilities,
	})
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, doc)
}

func (h *Handlers) HandleSkillSave(ctx context.Context, c *app.RequestContext) {
	var body skillSaveRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	body.Scope = appsvc.SkillScope(strings.TrimSpace(string(body.Scope)))
	body.Name = strings.TrimSpace(body.Name)
	body.TargetScope = appsvc.SkillScope(strings.TrimSpace(string(body.TargetScope)))
	body.TargetName = strings.TrimSpace(body.TargetName)
	if body.TargetScope == "" {
		body.TargetScope = body.Scope
	}
	if body.TargetName == "" {
		body.TargetName = body.Name
	}
	doc, err := h.app.ResourceCatalog().SaveSkillAs(ctx, skillTarget(c), body.Scope, body.Name, body.TargetScope, body.TargetName, body.Content, strings.TrimSpace(body.BaseRevision))
	if err != nil {
		if errors.Is(err, appsvc.ErrSkillRevisionConflict) {
			writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, doc)
}

func (h *Handlers) HandleSkillFileSave(ctx context.Context, c *app.RequestContext) {
	var body skillFileSaveRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	body.Scope = appsvc.SkillScope(strings.TrimSpace(string(body.Scope)))
	body.Name = strings.TrimSpace(body.Name)
	body.Path = strings.TrimSpace(body.Path)
	if body.Scope == "" || body.Name == "" || body.Path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.scopeNamePathRequired")
		return
	}
	doc, err := h.app.ResourceCatalog().SaveSkillFile(ctx, skillTarget(c), body.Scope, body.Name, body.Path, body.Content, strings.TrimSpace(body.BaseRevision))
	if err != nil {
		if errors.Is(err, appsvc.ErrSkillRevisionConflict) {
			writeErrorKey(c, consts.StatusConflict, "api.resource.revisionConflict")
			return
		}
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, doc)
}

func (h *Handlers) HandleSkillDelete(ctx context.Context, c *app.RequestContext) {
	scope := appsvc.SkillScope(strings.TrimSpace(c.Query("scope")))
	name := strings.TrimSpace(c.Query("name"))
	if scope == "" || name == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.scopeNameRequired")
		return
	}
	if err := h.app.ResourceCatalog().DeleteSkill(ctx, skillTarget(c), scope, name); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func skillTarget(c *app.RequestContext) resourcecatalogapp.SkillTarget {
	if layout := projectScope(c); layout.ProjectID != "" {
		return resourcecatalogapp.ProjectSkills(layout.ProjectID)
	}
	return resourcecatalogapp.GlobalSkills()
}
