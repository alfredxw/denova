package handlers

import (
	"context"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaskills "nova/internal/skills"
)

type skillCreateRequest struct {
	Scope       novaskills.Scope `json:"scope"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Agents      []string         `json:"agents"`
}

type skillSaveRequest struct {
	Scope       novaskills.Scope `json:"scope"`
	Name        string           `json:"name"`
	Content     string           `json:"content"`
	TargetScope novaskills.Scope `json:"target_scope"`
	TargetName  string           `json:"target_name"`
}

func (h *Handlers) HandleSkills(ctx context.Context, c *app.RequestContext) {
	snapshot, err := h.app.SkillSnapshot(ctx)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, snapshot)
}

func (h *Handlers) HandleSkillDocument(ctx context.Context, c *app.RequestContext) {
	scope := novaskills.Scope(strings.TrimSpace(c.Query("scope")))
	name := strings.TrimSpace(c.Query("name"))
	if scope == "" || name == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.scopeNameRequired")
		return
	}
	doc, err := h.app.SkillDocument(ctx, scope, name)
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
	body.Scope = novaskills.Scope(strings.TrimSpace(string(body.Scope)))
	body.Name = strings.TrimSpace(body.Name)
	doc, err := h.app.CreateSkillDocument(ctx, body.Scope, body.Name, body.Description, body.Agents)
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
	body.Scope = novaskills.Scope(strings.TrimSpace(string(body.Scope)))
	body.Name = strings.TrimSpace(body.Name)
	body.TargetScope = novaskills.Scope(strings.TrimSpace(string(body.TargetScope)))
	body.TargetName = strings.TrimSpace(body.TargetName)
	if body.TargetScope == "" {
		body.TargetScope = body.Scope
	}
	if body.TargetName == "" {
		body.TargetName = body.Name
	}
	doc, err := h.app.SaveSkillDocumentAs(ctx, body.Scope, body.Name, body.TargetScope, body.TargetName, body.Content)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, doc)
}

func (h *Handlers) HandleSkillDelete(ctx context.Context, c *app.RequestContext) {
	scope := novaskills.Scope(strings.TrimSpace(c.Query("scope")))
	name := strings.TrimSpace(c.Query("name"))
	if scope == "" || name == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.scopeNameRequired")
		return
	}
	if err := h.app.DeleteSkillDocument(ctx, scope, name); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}
