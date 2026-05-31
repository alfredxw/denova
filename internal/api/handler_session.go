package api

import (
	"context"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/internal/session"
)

// messageDTO 消息 DTO，type=clear 时表示上下文清理分界。
type messageDTO struct {
	Type      string `json:"type"`
	ID        string `json:"id,omitempty"`
	Role      string `json:"role,omitempty"`
	Content   string `json:"content,omitempty"`
	Name      string `json:"name,omitempty"`
	Status    string `json:"status,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// sessionDTO 会话摘要 DTO。
type sessionDTO struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	Active       bool   `json:"active"`
	MessageCount int    `json:"message_count"`
}

type sessionCreateRequest struct {
	Title string `json:"title"`
}

type sessionIDRequest struct {
	ID string `json:"id"`
}

type sessionRenameRequest struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// handleSessionMessages GET /api/session/messages — 返回当前或指定会话历史消息。
func (s *Server) handleSessionMessages(ctx context.Context, c *app.RequestContext) {
	if !s.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, []messageDTO{})
		return
	}
	id := strings.TrimSpace(c.Query("session_id"))
	entries, err := s.app.SessionMessages(id)
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}

	result := make([]messageDTO, 0, len(entries))
	for _, entry := range entries {
		if entry.Type == "clear" {
			result = append(result, messageDTO{
				Type:      entry.Type,
				CreatedAt: formatTime(entry.CreatedAt),
			})
			continue
		}
		if entry.Content == "" {
			continue
		}
		result = append(result, messageDTO{
			Type:      entry.Type,
			ID:        entry.ID,
			Role:      entry.Role,
			Content:   entry.Content,
			Name:      entry.Name,
			Status:    entry.Status,
			CreatedAt: formatTime(entry.CreatedAt),
		})
	}

	writeJSON(c, consts.StatusOK, result)
}

// handleSessions GET /api/sessions — 返回当前 workspace 下的会话列表。
func (s *Server) handleSessions(ctx context.Context, c *app.RequestContext) {
	if !s.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, map[string]any{"sessions": []sessionDTO{}})
		return
	}
	metas, err := s.app.Sessions()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"sessions": toSessionDTOs(metas)})
}

// handleSessionCreate POST /api/sessions — 创建并激活新会话。
func (s *Server) handleSessionCreate(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	var req sessionCreateRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "无效请求体")
		return
	}
	sess, err := s.app.CreateSession(req.Title)
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, sessionDTOFromSession(sess, true))
}

// handleSessionSwitch POST /api/sessions/switch — 切换当前激活会话。
func (s *Server) handleSessionSwitch(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	var req sessionIDRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "无效请求体")
		return
	}
	sess, err := s.app.SwitchSession(req.ID)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, sessionDTOFromSession(sess, true))
}

// handleSessionRename POST /api/sessions/rename — 重命名会话。
func (s *Server) handleSessionRename(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	var req sessionRenameRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "无效请求体")
		return
	}
	if err := s.app.RenameSession(req.ID, req.Title); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

// handleSessionDelete POST /api/sessions/delete — 删除会话并返回新的激活会话。
func (s *Server) handleSessionDelete(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	var req sessionIDRequest
	if err := c.BindJSON(&req); err != nil {
		writeError(c, consts.StatusBadRequest, "无效请求体")
		return
	}
	sess, err := s.app.DeleteSession(req.ID)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, sessionDTOFromSession(sess, true))
}

// statusResponse 状态响应。
type statusResponse struct {
	HasState bool   `json:"has_state"`
	Context  string `json:"context"`
}

// handleStatus GET /api/status — 返回作品状态。
func (s *Server) handleStatus(ctx context.Context, c *app.RequestContext) {
	hasState, stateCtx := s.app.Status()
	writeJSON(c, consts.StatusOK, statusResponse{
		HasState: hasState,
		Context:  stateCtx,
	})
}

func toSessionDTOs(metas []session.SessionMeta) []sessionDTO {
	result := make([]sessionDTO, 0, len(metas))
	for _, meta := range metas {
		result = append(result, sessionDTO{
			ID:           meta.ID,
			Title:        meta.Title,
			CreatedAt:    formatTime(meta.CreatedAt),
			UpdatedAt:    formatTime(meta.UpdatedAt),
			Active:       meta.Active,
			MessageCount: meta.MessageCount,
		})
	}
	return result
}

func sessionDTOFromSession(sess *session.Session, active bool) sessionDTO {
	return sessionDTO{
		ID:           sess.ID,
		Title:        sess.Title(),
		CreatedAt:    formatTime(sess.CreatedAt),
		UpdatedAt:    formatTime(sess.UpdatedAt),
		Active:       active,
		MessageCount: sess.MessageCount(),
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
