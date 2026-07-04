package handlers

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	novaskills "denova/internal/skills"
)

// MaxSkillInstallUploadBytes limits Skill ZIP uploads.
const MaxSkillInstallUploadBytes = novaskills.MaxInstallArchiveBytes

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

type skillFileSaveRequest struct {
	Scope   novaskills.Scope `json:"scope"`
	Name    string           `json:"name"`
	Path    string           `json:"path"`
	Content string           `json:"content"`
}

type skillInstallGitHubRequest struct {
	URL          string           `json:"url"`
	Ref          string           `json:"ref"`
	Subdir       string           `json:"subdir"`
	Scope        novaskills.Scope `json:"scope"`
	CandidateIDs []string         `json:"candidate_ids"`
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

func (h *Handlers) HandleSkillFileDocument(ctx context.Context, c *app.RequestContext) {
	scope := novaskills.Scope(strings.TrimSpace(c.Query("scope")))
	name := strings.TrimSpace(c.Query("name"))
	path := strings.TrimSpace(c.Query("path"))
	if scope == "" || name == "" || path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.scopeNamePathRequired")
		return
	}
	doc, err := h.app.SkillFileDocument(ctx, scope, name, path)
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

func (h *Handlers) HandleSkillFileSave(ctx context.Context, c *app.RequestContext) {
	var body skillFileSaveRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	body.Scope = novaskills.Scope(strings.TrimSpace(string(body.Scope)))
	body.Name = strings.TrimSpace(body.Name)
	body.Path = strings.TrimSpace(body.Path)
	if body.Scope == "" || body.Name == "" || body.Path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.scopeNamePathRequired")
		return
	}
	doc, err := h.app.SaveSkillFileDocument(ctx, body.Scope, body.Name, body.Path, body.Content)
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

func (h *Handlers) HandleSkillInstallZipPreview(ctx context.Context, c *app.RequestContext) {
	scope := normalizeSkillInstallScope(string(c.FormValue("scope")))
	_, data, ok := readSkillInstallUpload(c)
	if !ok {
		return
	}
	preview, err := h.app.PreviewSkillZip(ctx, scope, data)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, preview)
}

func (h *Handlers) HandleSkillInstallZip(ctx context.Context, c *app.RequestContext) {
	scope := normalizeSkillInstallScope(string(c.FormValue("scope")))
	_, data, ok := readSkillInstallUpload(c)
	if !ok {
		return
	}
	candidateIDs := parseCandidateIDs(string(c.FormValue("candidate_ids")))
	result, err := h.app.InstallSkillZip(ctx, scope, data, candidateIDs)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func (h *Handlers) HandleSkillInstallGitHubPreview(ctx context.Context, c *app.RequestContext) {
	var body skillInstallGitHubRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	source := novaskills.GitHubSource{URL: body.URL, Ref: body.Ref, Subdir: body.Subdir}
	preview, err := h.app.PreviewSkillGitHub(ctx, normalizeSkillInstallScope(string(body.Scope)), source)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, preview)
}

func (h *Handlers) HandleSkillInstallGitHub(ctx context.Context, c *app.RequestContext) {
	var body skillInstallGitHubRequest
	if err := c.BindJSON(&body); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.invalidRequestWithDetail", "detail", err.Error())
		return
	}
	source := novaskills.GitHubSource{URL: body.URL, Ref: body.Ref, Subdir: body.Subdir}
	result, err := h.app.InstallSkillGitHub(ctx, normalizeSkillInstallScope(string(body.Scope)), source, body.CandidateIDs)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, result)
}

func readSkillInstallUpload(c *app.RequestContext) (string, []byte, bool) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.uploadRequired")
		return "", nil, false
	}
	if fileHeader.Size > MaxSkillInstallUploadBytes {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.tooLarge")
		return "", nil, false
	}

	file, err := fileHeader.Open()
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.readFailed", "detail", err.Error())
		return "", nil, false
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, MaxSkillInstallUploadBytes+1))
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.readFailed", "detail", err.Error())
		return "", nil, false
	}
	if int64(len(data)) > MaxSkillInstallUploadBytes {
		writeErrorKey(c, consts.StatusBadRequest, "api.skills.tooLarge")
		return "", nil, false
	}
	return fileHeader.Filename, data, true
}

func normalizeSkillInstallScope(scope string) novaskills.Scope {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return novaskills.ScopeUser
	}
	return novaskills.Scope(scope)
}

func parseCandidateIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var ids []string
	if strings.HasPrefix(raw, "[") {
		if err := json.Unmarshal([]byte(raw), &ids); err == nil {
			return normalizeCandidateIDs(ids)
		}
	}
	return normalizeCandidateIDs(strings.Split(raw, ","))
}

func normalizeCandidateIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
