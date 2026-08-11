package app

import (
	"errors"
	"fmt"
	"strings"
)

var ErrAgentTaskRunning = errors.New("当前 Agent 会话已有运行中的任务")

// AgentTaskInfo identifies the project and conversation that own an Agent run.
type AgentTaskInfo struct {
	TaskID       string
	Workspace    string
	SessionID    string
	SessionTitle string
}

type agentTaskRun struct {
	task *Task
	info AgentTaskInfo
}

func agentTaskKey(workspace, sessionID string) string {
	return canonicalTaskWorkspace(workspace) + "\x00" + strings.TrimSpace(sessionID)
}

func (s *ChatAppService) bindAgentTask(task *Task, info AgentTaskInfo) error {
	if s == nil || s.app == nil || task == nil {
		return errors.New("Agent 任务不能为空")
	}
	info.TaskID = task.ID()
	info.Workspace = canonicalTaskWorkspace(info.Workspace)
	info.SessionID = strings.TrimSpace(info.SessionID)
	info.SessionTitle = strings.TrimSpace(info.SessionTitle)
	if info.Workspace == "" || info.SessionID == "" {
		return fmt.Errorf("Agent 任务缺少来源项目或会话")
	}

	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.agentTaskRuns == nil {
		a.agentTaskRuns = make(map[string]*agentTaskRun)
	}
	key := agentTaskKey(info.Workspace, info.SessionID)
	if existing := a.agentTaskRuns[key]; existing != nil && existing.task != nil && existing.task.Status() == TaskRunning {
		return ErrAgentTaskRunning
	}
	a.agentTaskRuns[key] = &agentTaskRun{task: task, info: info}
	return nil
}

// ActiveTaskFor returns the latest run for a session in the open project. An
// empty session ID resolves to the current conversation.
func (a *App) ActiveTaskFor(sessionID string) (*Task, AgentTaskInfo) {
	return a.chat().ActiveTaskFor(sessionID)
}

func (s *ChatAppService) ActiveTaskFor(sessionID string) (*Task, AgentTaskInfo) {
	if s == nil || s.app == nil {
		return nil, AgentTaskInfo{}
	}
	a := s.app
	a.mu.RLock()
	workspace := a.workspace
	if strings.TrimSpace(sessionID) == "" && a.session != nil {
		sessionID = a.session.ID
	}
	run := a.agentTaskRuns[agentTaskKey(workspace, sessionID)]
	a.mu.RUnlock()
	if run == nil || run.task == nil {
		return nil, AgentTaskInfo{}
	}
	return run.task, run.info
}

func (s *ChatAppService) agentTaskRunning(workspace, sessionID string) bool {
	if s == nil || s.app == nil {
		return false
	}
	a := s.app
	a.mu.RLock()
	run := a.agentTaskRuns[agentTaskKey(workspace, sessionID)]
	a.mu.RUnlock()
	return run != nil && run.task != nil && run.task.Status() == TaskRunning
}

func (s *ChatAppService) forgetAgentTasks(workspace, sessionID string) {
	if s == nil || s.app == nil {
		return
	}
	a := s.app
	a.mu.Lock()
	delete(a.agentTaskRuns, agentTaskKey(workspace, sessionID))
	a.mu.Unlock()
}

func (a *App) agentTasksSnapshot() []agentTaskRun {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]agentTaskRun, 0, len(a.agentTaskRuns))
	for _, run := range a.agentTaskRuns {
		if run == nil || run.task == nil {
			continue
		}
		result = append(result, *run)
	}
	return result
}
