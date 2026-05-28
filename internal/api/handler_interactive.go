package api

import (
	"context"
	"log"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"nova/internal/interactive"
)

type tellerAgentRequest struct {
	Instruction string `json:"instruction"`
	TellerID    string `json:"teller_id"`
}

func (s *Server) handleInteractiveStories(ctx context.Context, c *app.RequestContext) {
	index, err := s.app.InteractiveStories()
	if err != nil {
		writeError(c, consts.StatusConflict, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, index)
}

func (s *Server) handleInteractiveStoryCreate(ctx context.Context, c *app.RequestContext) {
	var body interactive.CreateStoryRequest
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	story, err := s.app.CreateInteractiveStory(body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, story)
}

func (s *Server) handleInteractiveStoryUpdate(ctx context.Context, c *app.RequestContext) {
	var body interactive.UpdateStoryRequest
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	story, err := s.app.UpdateInteractiveStory(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, story)
}

func (s *Server) handleInteractiveStoryDelete(ctx context.Context, c *app.RequestContext) {
	if err := s.app.DeleteInteractiveStory(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleInteractiveSnapshot(ctx context.Context, c *app.RequestContext) {
	snapshot, err := s.app.InteractiveSnapshot(c.Param("id"), c.Query("branch"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, snapshot)
}

func (s *Server) handleInteractiveBranches(ctx context.Context, c *app.RequestContext) {
	branches, err := s.app.InteractiveBranches(c.Param("id"))
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"branches": branches})
}

func (s *Server) handleInteractiveBranchCreate(ctx context.Context, c *app.RequestContext) {
	var body interactive.CreateBranchRequest
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	branch, err := s.app.CreateInteractiveBranch(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, branch)
}

func (s *Server) handleInteractiveBranchDelete(ctx context.Context, c *app.RequestContext) {
	if err := s.app.DeleteInteractiveBranch(c.Param("id"), c.Param("branch")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleInteractiveBranchSwitch(ctx context.Context, c *app.RequestContext) {
	var body struct {
		BranchID string `json:"branch_id"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	if err := s.app.SwitchInteractiveBranch(c.Param("id"), body.BranchID); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleInteractiveChat(ctx context.Context, c *app.RequestContext) {
	var body struct {
		Mode    string `json:"mode"`
		StoryID string `json:"story_id"`
		Branch  string `json:"branch"`
		Message string `json:"message"`
	}
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	if strings.TrimSpace(body.Message) == "" {
		writeError(c, consts.StatusBadRequest, "消息不能为空")
		return
	}
	if strings.TrimSpace(body.StoryID) == "" {
		writeError(c, consts.StatusBadRequest, "故事 ID 不能为空")
		return
	}
	if body.Mode != "" && body.Mode != "story" {
		writeError(c, consts.StatusBadRequest, "当前仅支持 story 子模式")
		return
	}

	task := s.app.StartInteractiveTask(body.StoryID, body.Branch, body.Message)
	if task == nil {
		writeError(c, consts.StatusConflict, "尚未选择书籍工作区，请先在书籍管理页选择或创建书籍")
		return
	}
	streamTask(c, task)
}

func (s *Server) handleInteractiveChatAbort(ctx context.Context, c *app.RequestContext) {
	if task := s.app.ActiveInteractiveTask(); task != nil {
		log.Printf("[interactive-agent-sse] abort requested task_id=%s status=%s", task.ID(), task.Status())
	}
	s.app.AbortInteractiveTask()
	c.JSON(consts.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleInteractiveTellers(ctx context.Context, c *app.RequestContext) {
	tellers, err := s.app.InteractiveTellers()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]any{"tellers": tellers})
}

func (s *Server) handleInteractiveTeller(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	teller, err := s.app.InteractiveTeller(id)
	if err != nil {
		writeError(c, consts.StatusNotFound, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, teller)
}

func (s *Server) handleInteractiveTellerCreate(ctx context.Context, c *app.RequestContext) {
	var body interactive.Teller
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	teller, err := s.app.CreateInteractiveTeller(body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, teller)
}

func (s *Server) handleInteractiveTellerUpdate(ctx context.Context, c *app.RequestContext) {
	var body interactive.Teller
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	teller, err := s.app.UpdateInteractiveTeller(c.Param("id"), body)
	if err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, teller)
}

func (s *Server) handleInteractiveTellerDelete(ctx context.Context, c *app.RequestContext) {
	if err := s.app.DeleteInteractiveTeller(c.Param("id")); err != nil {
		writeError(c, consts.StatusBadRequest, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleInteractiveTellerAgentStream(ctx context.Context, c *app.RequestContext) {
	var body tellerAgentRequest
	if err := c.BindJSON(&body); err != nil {
		writeError(c, consts.StatusBadRequest, "请求参数无效: "+err.Error())
		return
	}
	if strings.TrimSpace(body.Instruction) == "" {
		writeError(c, consts.StatusBadRequest, "讲述者编辑指令不能为空")
		return
	}
	task := s.app.StartTellerAgentTask(body.Instruction, body.TellerID)
	if task == nil {
		writeError(c, consts.StatusConflict, "尚未选择书籍工作区，请先在书籍管理页选择或创建书籍")
		return
	}
	streamTask(c, task)
}

func (s *Server) handleInteractiveTellerAgentMessages(ctx context.Context, c *app.RequestContext) {
	if !s.app.HasWorkspace() {
		writeJSON(c, consts.StatusOK, []messageDTO{})
		return
	}
	entries, err := s.app.TellerAgentMessages()
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
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
			Type:    entry.Type,
			Role:    entry.Role,
			Content: entry.Content,
		})
	}
	writeJSON(c, consts.StatusOK, result)
}

func (s *Server) handleInteractiveTellerAgentClear(ctx context.Context, c *app.RequestContext) {
	if !s.requireWorkspace(c) {
		return
	}
	if err := s.app.ClearTellerAgentSession(); err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(c, consts.StatusOK, map[string]string{"status": "ok"})
}
