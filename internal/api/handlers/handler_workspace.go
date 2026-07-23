package handlers

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	denovaapp "denova/internal/app"
	"denova/internal/book"
	"denova/internal/workspacechange"
)

// handleWorkspaceTree GET /api/workspace/tree — 递归扫描 workspace 目录返回文件树。
func (h *Handlers) HandleWorkspaceTree(ctx context.Context, c *app.RequestContext) {
	if !h.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, []any{})
		return
	}
	tree, err := h.app.BookService().Tree()
	if err != nil {
		writeErrorKey(c, consts.StatusInternalServerError, "api.workspace.scanFailed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, tree)
}

// handleWorkspaceSummary GET /api/workspace/summary — 返回作品章节统计和写作进度。
func (h *Handlers) HandleWorkspaceSummary(ctx context.Context, c *app.RequestContext) {
	if !h.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, map[string]any{
			"title":         "",
			"author":        "",
			"chapter_count": 0,
			"total_words":   0,
			"chapters":      []any{},
		})
		return
	}
	summary, err := h.app.BookService().Summary()
	if err != nil {
		writeErrorKey(c, consts.StatusInternalServerError, "api.workspace.summaryFailed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, summary)
}

// HandleWorkspaceChapterStatus PATCH /api/workspace/chapter-status — 手动确认或撤销章节成章状态。
func (h *Handlers) HandleWorkspaceChapterStatus(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req struct {
		Path      string `json:"path"`
		Confirmed bool   `json:"confirmed"`
	}
	if err := c.BindJSON(&req); err != nil || req.Path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.workspace.chapterStatusPathRequired")
		return
	}
	if err := h.app.BookService().SetChapterConfirmed(req.Path, req.Confirmed); err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.workspace.chapterStatusFailed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"path": req.Path, "confirmed": req.Confirmed, "message": messageKey(c, "api.workspace.chapterStatusSaved")})
}

// handleWorkspaceFile GET /api/workspace/file?path=xxx — 读取文件内容。
func (h *Handlers) HandleWorkspaceFile(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	relPath := c.Query("path")
	if relPath == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.workspace.pathMissing")
		return
	}

	content, revision, workspace, err := h.app.ReadWorkspaceFileWithRevision(relPath)
	if err != nil {
		writeError(c, fileReadStatus(err), err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{
		"content":   content,
		"path":      relPath,
		"revision":  revision,
		"workspace": workspace,
	})
}

// HandleWorkspaceAsset GET /api/workspace/asset?path=... — 读取 workspace 内图像文件。
func (h *Handlers) HandleWorkspaceAsset(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	rawPath := c.Query("path")
	if hasParentPathSegment(rawPath) {
		writeError(c, consts.StatusBadRequest, "图像路径不能包含上级目录")
		return
	}
	relPath := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rawPath)))
	if relPath == "." || relPath == "" {
		writeError(c, consts.StatusBadRequest, "图像路径不能为空")
		return
	}
	contentType := workspaceAssetContentType(relPath)
	if contentType == "" {
		writeError(c, consts.StatusBadRequest, "仅支持读取 png、jpg、jpeg、webp 或 gif 图像")
		return
	}
	absPath, err := book.SafePath(h.app.BookService().Workspace(), relPath)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	info, err := os.Stat(absPath)
	if err != nil {
		writeError(c, fileReadStatus(err), err.Error())
		return
	}
	if info.IsDir() {
		writeError(c, consts.StatusBadRequest, "资产路径是目录")
		return
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		writeError(c, fileReadStatus(err), err.Error())
		return
	}
	c.Data(consts.StatusOK, contentType, data)
}

func hasParentPathSegment(path string) bool {
	for _, part := range strings.Split(filepath.FromSlash(path), string(filepath.Separator)) {
		if part == ".." {
			return true
		}
	}
	return false
}

func workspaceAssetContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return ""
	}
}

// handleWorkspaceSearch GET /api/workspace/search?q=xxx — 搜索当前书籍 workspace 文本内容和文件路径。
func (h *Handlers) HandleWorkspaceSearch(ctx context.Context, c *app.RequestContext) {
	if !h.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, map[string]any{"results": []any{}})
		return
	}
	query := c.Query("q")
	limit := book.DefaultSearchLimit
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 0 {
			writeErrorKey(c, consts.StatusBadRequest, "api.workspace.limitInvalid")
			return
		}
		limit = parsed
	}
	opts := book.SearchOptions{Regex: isTruthyQueryFlag(c.Query("regex"))}

	results, err := h.app.BookService().Search(query, limit, opts)
	if err != nil {
		if errors.Is(err, book.ErrInvalidSearchRegex) {
			writeErrorKey(c, consts.StatusBadRequest, "api.workspace.invalidRegex", "detail", err.Error())
			return
		}
		writeErrorKey(c, consts.StatusInternalServerError, "api.workspace.searchFailed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"results": results})
}

// handleWorkspaceReplace POST /api/workspace/replace — 在整个 workspace 文本文件内全局替换匹配内容。
// 替换前会先做一次只读预扫描，存在匹配时创建可恢复版本，再逐文件按当前 revision CAS 写入；
// 写入期间被并发修改的文件会跳过并在响应中列出。
func (h *Handlers) HandleWorkspaceReplace(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req struct {
		Query       string `json:"query"`
		Replacement string `json:"replacement"`
		Regex       bool   `json:"regex"`
		Workspace   string `json:"workspace"`
	}
	if err := c.BindJSON(&req); err != nil || strings.TrimSpace(req.Query) == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.workspace.queryRequired")
		return
	}
	replacer, err := book.NewReplacer(strings.TrimSpace(req.Query), req.Replacement, book.SearchOptions{Regex: req.Regex})
	if err != nil {
		key := "api.workspace.invalidRegex"
		if errors.Is(err, book.ErrRegexMatchesEmpty) {
			key = "api.workspace.regexMatchesEmpty"
		}
		writeErrorKey(c, consts.StatusBadRequest, key, "detail", err.Error())
		return
	}

	// 只读预扫描：没有任何匹配时直接返回，避免创建无意义的备份版本。
	workspace := strings.TrimSpace(h.app.Workspace())
	hasMatch, err := workspaceHasReplacement(workspace, replacer)
	if err != nil {
		writeErrorKey(c, consts.StatusInternalServerError, "api.workspace.replaceFailed", "detail", err.Error())
		return
	}
	if !hasMatch {
		writeJSON(c, consts.StatusOK, map[string]any{
			"workspace":          workspace,
			"files":              []any{},
			"total_replacements": 0,
			"skipped":            []any{},
		})
		return
	}
	if _, err := h.app.CreateVersion(ctx, "全局替换前自动备份"); err != nil && !errors.Is(err, book.ErrVersionClean) {
		writeErrorKey(c, consts.StatusInternalServerError, "api.workspace.replaceFailed", "detail", err.Error())
		return
	}

	changed := make([]book.ReplaceFileResult, 0)
	skipped := make([]string, 0)
	canonicalWorkspace, err := h.app.WithWorkspaceChangeMutation(
		ctx,
		req.Workspace,
		func(changeService *workspacechange.Service) (denovaapp.WorkspaceChangeMutationHooks, error) {
			candidates, listErr := book.ListReplaceCandidateFiles(changeService.Workspace())
			if listErr != nil {
				return denovaapp.WorkspaceChangeMutationHooks{}, listErr
			}
			for _, rel := range candidates {
				content, revision, readErr := changeService.ReadFile(rel)
				if readErr != nil || !book.IsSearchableContent([]byte(content)) {
					continue
				}
				next, count := replacer.ReplaceAll(content)
				if count == 0 {
					continue
				}
				saveResult, saveErr := changeService.SaveFile(ctx, rel, next, revision)
				if saveErr != nil {
					var changeErr *workspacechange.Error
					if errors.As(saveErr, &changeErr) && changeErr.Code == workspacechange.ErrorCodeRevisionConflict {
						log.Printf("[workspace-replace] 跳过并发修改的文件 path=%q", rel)
						skipped = append(skipped, rel)
						continue
					}
					return denovaapp.WorkspaceChangeMutationHooks{}, saveErr
				}
				// 替换结果与原内容一致时 SaveFile 不产生实际写入，不计入变更。
				if !saveResult.Changed {
					continue
				}
				changed = append(changed, book.ReplaceFileResult{Path: rel, Replacements: count})
			}
			if len(changed) == 0 {
				return denovaapp.WorkspaceChangeMutationHooks{}, nil
			}
			paths := make([]string, 0, len(changed))
			for _, item := range changed {
				paths = append(paths, item.Path)
			}
			return denovaapp.WorkspaceChangeMutationHooks{
				ScheduleAutoVersion: true,
				AutomationSource:    "workspace_replace",
				Paths:               paths,
			}, nil
		},
	)
	if err != nil {
		if errors.Is(err, denovaapp.ErrWorkspaceChanged) {
			writeJSON(c, consts.StatusConflict, map[string]any{
				"error": messageKey(c, "api.workspace.changedDuringRequest"),
				"code":  "workspace_changed",
				"details": map[string]string{
					"expected_workspace": strings.TrimSpace(req.Workspace),
					"actual_workspace":   h.app.Workspace(),
				},
			})
			return
		}
		var changeErr *workspacechange.Error
		if errors.As(err, &changeErr) {
			writeWorkspaceChangeError(c, err)
			return
		}
		writeErrorKey(c, consts.StatusInternalServerError, "api.workspace.replaceFailed", "detail", err.Error())
		return
	}

	total := 0
	for _, item := range changed {
		total += item.Replacements
	}
	log.Printf("[workspace-replace] 全局替换完成 files=%d replacements=%d skipped=%d", len(changed), total, len(skipped))
	writeJSON(c, consts.StatusOK, map[string]any{
		"workspace":          canonicalWorkspace,
		"files":              changed,
		"total_replacements": total,
		"skipped":            skipped,
	})
}

// workspaceHasReplacement 只读扫描 workspace，报告是否存在至少一处可替换匹配。
func workspaceHasReplacement(workspace string, replacer *book.Replacer) (bool, error) {
	candidates, err := book.ListReplaceCandidateFiles(workspace)
	if err != nil {
		return false, err
	}
	for _, rel := range candidates {
		data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(rel)))
		if err != nil || !book.IsSearchableContent(data) {
			continue
		}
		if next, count := replacer.ReplaceAll(string(data)); count > 0 && next != string(data) {
			return true, nil
		}
	}
	return false, nil
}

func isTruthyQueryFlag(raw string) bool {
	return raw == "1" || raw == "true"
}

// handleWorkspaceFileWrite POST /api/workspace/file — 写入文件内容。
func (h *Handlers) HandleWorkspaceFileWrite(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req struct {
		Path         string `json:"path"`
		Content      string `json:"content"`
		BaseRevision string `json:"base_revision"`
		Workspace    string `json:"workspace"`
	}
	if err := c.BindJSON(&req); err != nil || req.Path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.workspace.pathContentRequired")
		return
	}

	var saveResult workspacechange.SaveResult
	canonicalWorkspace, err := h.app.WithWorkspaceChangeMutation(
		ctx,
		req.Workspace,
		func(changeService *workspacechange.Service) (denovaapp.WorkspaceChangeMutationHooks, error) {
			var saveErr error
			saveResult, saveErr = changeService.SaveFile(ctx, req.Path, req.Content, req.BaseRevision)
			if saveErr != nil || !saveResult.Changed {
				return denovaapp.WorkspaceChangeMutationHooks{}, saveErr
			}
			return denovaapp.WorkspaceChangeMutationHooks{
				ScheduleAutoVersion: true,
				AutomationSource:    "workspace_file_write",
				Paths:               []string{req.Path},
			}, nil
		},
	)
	if err != nil {
		if errors.Is(err, denovaapp.ErrWorkspaceChanged) {
			writeJSON(c, consts.StatusConflict, map[string]any{
				"error": messageKey(c, "api.workspace.changedDuringRequest"),
				"code":  "workspace_changed",
				"details": map[string]string{
					"expected_workspace": strings.TrimSpace(req.Workspace),
					"actual_workspace":   h.app.Workspace(),
				},
			})
			return
		}
		var changeErr *workspacechange.Error
		if errors.As(err, &changeErr) {
			writeWorkspaceChangeError(c, err)
			return
		}
		writeErrorKey(c, fileWriteStatus(err), "api.workspace.writeFailed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{
		"workspace": canonicalWorkspace,
		"path":      req.Path,
		"revision":  saveResult.Revision,
		"changed":   saveResult.Changed,
		"message":   messageKey(c, "api.workspace.fileSaved"),
	})
}

// handleWorkspaceCreate POST /api/workspace/create — 新建文件或目录。
func (h *Handlers) HandleWorkspaceCreate(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req struct {
		Path    string `json:"path"`
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	if err := c.BindJSON(&req); err != nil || req.Path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.workspace.pathTypeRequired")
		return
	}

	if err := h.app.CreateWorkspaceItem(ctx, req.Path, req.Type, req.Content); err != nil {
		if errors.Is(err, os.ErrExist) {
			writeErrorKey(c, consts.StatusConflict, "api.workspace.targetExists")
			return
		}
		writeError(c, fileWriteStatus(err), err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"path": req.Path, "message": messageKey(c, "api.workspace.created")})
}

// handleWorkspaceDelete POST /api/workspace/delete — 删除文件或目录。
func (h *Handlers) HandleWorkspaceDelete(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := c.BindJSON(&req); err != nil || req.Path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.pathRequired")
		return
	}

	if err := h.app.DeleteWorkspaceItem(ctx, req.Path); err != nil {
		writeErrorKey(c, fileWriteStatus(err), "api.workspace.deleteFailed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"path": req.Path, "message": messageKey(c, "api.workspace.deleted")})
}

// handleWorkspaceRename POST /api/workspace/rename — 重命名同目录下的文件或目录。
func (h *Handlers) HandleWorkspaceRename(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req struct {
		Path    string `json:"path"`
		NewName string `json:"new_name"`
	}
	if err := c.BindJSON(&req); err != nil || req.Path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.workspace.pathNewNameRequired")
		return
	}

	newPath, err := h.app.RenameWorkspaceItem(ctx, req.Path, req.NewName)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			writeErrorKey(c, consts.StatusConflict, "api.workspace.targetExists")
			return
		}
		writeError(c, fileWriteStatus(err), err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"path": newPath, "message": messageKey(c, "api.workspace.renamed")})
}

// handleWorkspaceCopy POST /api/workspace/copy — 复制文件或目录。
func (h *Handlers) HandleWorkspaceCopy(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := c.BindJSON(&req); err != nil || req.From == "" || req.To == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.workspace.fromToRequired")
		return
	}

	if err := h.app.CopyWorkspaceItem(ctx, req.From, req.To); err != nil {
		if errors.Is(err, os.ErrExist) {
			writeErrorKey(c, consts.StatusConflict, "api.workspace.targetExists")
			return
		}
		writeErrorKey(c, fileWriteStatus(err), "api.workspace.copyFailed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"path": req.To, "message": messageKey(c, "api.workspace.copied")})
}

// handleWorkspaceMove POST /api/workspace/move — 移动文件或目录。
func (h *Handlers) HandleWorkspaceMove(ctx context.Context, c *app.RequestContext) {
	if !h.requireWorkspace(c) {
		return
	}
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := c.BindJSON(&req); err != nil || req.From == "" || req.To == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.workspace.fromToRequired")
		return
	}

	if err := h.app.MoveWorkspaceItem(ctx, req.From, req.To); err != nil {
		if errors.Is(err, os.ErrExist) {
			writeErrorKey(c, consts.StatusConflict, "api.workspace.targetExists")
			return
		}
		writeErrorKey(c, fileWriteStatus(err), "api.workspace.moveFailed", "detail", err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"path": req.To, "message": messageKey(c, "api.workspace.moved")})
}

// handleWorkspaceSwitch POST /api/workspace/switch — 切换工作目录。
func (h *Handlers) HandleWorkspaceSwitch(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.BindJSON(&req); err != nil || req.Path == "" {
		writeErrorKey(c, consts.StatusBadRequest, "api.common.pathRequired")
		return
	}

	workspace, err := h.app.SwitchWorkspace(ctx, req.Path)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{
		"workspace": workspace,
		"message":   messageKey(c, "api.workspace.switched", "workspace", workspace),
	})
}

// handleWorkspaceCurrent GET /api/workspace/current — 获取当前工作目录。
func (h *Handlers) HandleWorkspaceCurrent(ctx context.Context, c *app.RequestContext) {
	hasState, _ := h.app.Status()
	writeJSON(c, consts.StatusOK, map[string]interface{}{
		"workspace": h.app.Workspace(),
		"has_state": hasState,
	})
}

func fileReadStatus(err error) int {
	if os.IsNotExist(err) {
		return consts.StatusNotFound
	}
	if isForbiddenFileError(err) {
		return consts.StatusForbidden
	}
	return consts.StatusBadRequest
}

func fileWriteStatus(err error) int {
	if isForbiddenFileError(err) {
		return consts.StatusForbidden
	}
	if isBadRequestFileError(err) {
		return consts.StatusBadRequest
	}
	return consts.StatusInternalServerError
}

func isForbiddenFileError(err error) bool {
	msg := err.Error()
	return msg == "路径不能为空" ||
		msg == "不允许使用绝对路径" ||
		msg == "路径不在 workspace 范围内" ||
		msg == "不允许操作隐藏文件或隐藏目录"
}

func isBadRequestFileError(err error) bool {
	msg := err.Error()
	return msg == "type 只能是 file 或 dir" ||
		msg == "新名称不能为空" ||
		msg == "新名称不能包含路径分隔符" ||
		msg == "不允许使用隐藏文件名"
}
