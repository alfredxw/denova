package api

import (
	"context"
	"errors"
	"os"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/internal/book"
)

// handleWorkspaceTree GET /api/workspace/tree — 递归扫描 workspace 目录返回文件树。
func (s *Server) handleWorkspaceTree(ctx context.Context, c *app.RequestContext) {
	if !s.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, []any{})
		return
	}
	tree, err := s.app.BookService().Tree()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "扫描目录失败: "+err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, tree)
}

// handleWorkspaceSummary GET /api/workspace/summary — 返回作品章节统计和写作进度。
func (s *Server) handleWorkspaceSummary(ctx context.Context, c *app.RequestContext) {
	if !s.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, map[string]any{
			"title":         "",
			"author":        "",
			"chapter_count": 0,
			"total_words":   0,
			"chapters":      []any{},
		})
		return
	}
	summary, err := s.app.BookService().Summary()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "统计作品进度失败: "+err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, summary)
}

// handleWorkspaceFile GET /api/workspace/file?path=xxx — 读取文件内容。
func (s *Server) handleWorkspaceFile(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	relPath := c.Query("path")
	if relPath == "" {
		writeError(c, consts.StatusBadRequest, "缺少 path 参数")
		return
	}

	content, err := s.app.BookService().ReadFile(relPath)
	if err != nil {
		writeError(c, fileReadStatus(err), err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{
		"content": content,
		"path":    relPath,
	})
}

// handleWorkspaceSearch GET /api/workspace/search?q=xxx — 搜索当前书籍 workspace 文本内容和文件路径。
func (s *Server) handleWorkspaceSearch(ctx context.Context, c *app.RequestContext) {
	if !s.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, map[string]any{"results": []any{}})
		return
	}
	query := c.Query("q")
	limit := book.DefaultSearchLimit
	if rawLimit := c.Query("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 0 {
			writeError(c, consts.StatusBadRequest, "limit 必须是非负整数")
			return
		}
		limit = parsed
	}

	results, err := s.app.BookService().Search(query, limit)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, "搜索失败: "+err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"results": results})
}

// handleWorkspaceFileWrite POST /api/workspace/file — 写入文件内容。
func (s *Server) handleWorkspaceFileWrite(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := c.BindJSON(&req); err != nil || req.Path == "" {
		writeError(c, consts.StatusBadRequest, "请提供 path 和 content 参数")
		return
	}

	if err := s.app.BookService().WriteFile(req.Path, req.Content); err != nil {
		writeError(c, fileWriteStatus(err), "写入文件失败: "+err.Error())
		return
	}
	s.app.MaybeCreateTimedVersion(ctx)
	writeJSON(c, consts.StatusOK, map[string]string{
		"path":    req.Path,
		"message": "文件已保存",
	})
}

// handleWorkspaceCreate POST /api/workspace/create — 新建文件或目录。
func (s *Server) handleWorkspaceCreate(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	var req struct {
		Path    string `json:"path"`
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	if err := c.BindJSON(&req); err != nil || req.Path == "" {
		writeError(c, consts.StatusBadRequest, "请提供 path 和 type 参数")
		return
	}

	if err := s.app.BookService().Create(req.Path, req.Type, req.Content); err != nil {
		if errors.Is(err, os.ErrExist) {
			writeError(c, consts.StatusConflict, "目标已存在")
			return
		}
		writeError(c, fileWriteStatus(err), err.Error())
		return
	}
	s.app.MaybeCreateTimedVersion(ctx)
	writeJSON(c, consts.StatusOK, map[string]string{"path": req.Path, "message": "创建成功"})
}

// handleWorkspaceDelete POST /api/workspace/delete — 删除文件或目录。
func (s *Server) handleWorkspaceDelete(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := c.BindJSON(&req); err != nil || req.Path == "" {
		writeError(c, consts.StatusBadRequest, "请提供 path 参数")
		return
	}

	if err := s.app.BookService().Delete(req.Path); err != nil {
		writeError(c, fileWriteStatus(err), "删除失败: "+err.Error())
		return
	}
	s.app.MaybeCreateTimedVersion(ctx)
	writeJSON(c, consts.StatusOK, map[string]string{"path": req.Path, "message": "删除成功"})
}

// handleWorkspaceRename POST /api/workspace/rename — 重命名同目录下的文件或目录。
func (s *Server) handleWorkspaceRename(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	var req struct {
		Path    string `json:"path"`
		NewName string `json:"new_name"`
	}
	if err := c.BindJSON(&req); err != nil || req.Path == "" {
		writeError(c, consts.StatusBadRequest, "请提供 path 和 new_name 参数")
		return
	}

	newPath, err := s.app.BookService().Rename(req.Path, req.NewName)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			writeError(c, consts.StatusConflict, "目标已存在")
			return
		}
		writeError(c, fileWriteStatus(err), err.Error())
		return
	}
	s.app.MaybeCreateTimedVersion(ctx)
	writeJSON(c, consts.StatusOK, map[string]string{"path": newPath, "message": "重命名成功"})
}

// handleWorkspaceCopy POST /api/workspace/copy — 复制文件或目录。
func (s *Server) handleWorkspaceCopy(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := c.BindJSON(&req); err != nil || req.From == "" || req.To == "" {
		writeError(c, consts.StatusBadRequest, "请提供 from 和 to 参数")
		return
	}

	if err := s.app.BookService().Copy(req.From, req.To); err != nil {
		if errors.Is(err, os.ErrExist) {
			writeError(c, consts.StatusConflict, "目标已存在")
			return
		}
		writeError(c, fileWriteStatus(err), "复制失败: "+err.Error())
		return
	}
	s.app.MaybeCreateTimedVersion(ctx)
	writeJSON(c, consts.StatusOK, map[string]string{"path": req.To, "message": "复制成功"})
}

// handleWorkspaceMove POST /api/workspace/move — 移动文件或目录。
func (s *Server) handleWorkspaceMove(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := c.BindJSON(&req); err != nil || req.From == "" || req.To == "" {
		writeError(c, consts.StatusBadRequest, "请提供 from 和 to 参数")
		return
	}

	if err := s.app.BookService().Move(req.From, req.To); err != nil {
		if errors.Is(err, os.ErrExist) {
			writeError(c, consts.StatusConflict, "目标已存在")
			return
		}
		writeError(c, fileWriteStatus(err), "移动失败: "+err.Error())
		return
	}
	s.app.MaybeCreateTimedVersion(ctx)
	writeJSON(c, consts.StatusOK, map[string]string{"path": req.To, "message": "移动成功"})
}

// handleWorkspaceSwitch POST /api/workspace/switch — 切换工作目录。
func (s *Server) handleWorkspaceSwitch(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.BindJSON(&req); err != nil || req.Path == "" {
		writeError(c, consts.StatusBadRequest, "请提供 path 参数")
		return
	}

	workspace, err := s.app.SwitchWorkspace(ctx, req.Path)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{
		"workspace": workspace,
		"message":   "已切换到: " + workspace,
	})
}

// handleWorkspaceCurrent GET /api/workspace/current — 获取当前工作目录。
func (s *Server) handleWorkspaceCurrent(ctx context.Context, c *app.RequestContext) {
	hasState, _ := s.app.Status()
	writeJSON(c, consts.StatusOK, map[string]interface{}{
		"workspace": s.app.Workspace(),
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
