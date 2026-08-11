package app

import "strings"

// configManagerTaskRun keeps a config-manager execution visible to the task
// center and to stopBackgroundTasks after the originating page disconnects.
// The request scope is retained so recovery can land on the owning surface.
type configManagerTaskRun struct {
	task      *Task
	workspace string
	sessionID string
	req       ConfigManagerRequest
}

func (s *ConfigManagerAppService) bindConfigManagerTask(task *Task, workspace, sessionID string, req ConfigManagerRequest) {
	if s == nil || s.app == nil || task == nil {
		return
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.configManagerTaskRuns == nil {
		a.configManagerTaskRuns = make(map[string]*configManagerTaskRun)
	}
	a.configManagerTaskRuns[task.ID()] = &configManagerTaskRun{
		task:      task,
		workspace: canonicalTaskWorkspace(workspace),
		sessionID: strings.TrimSpace(sessionID),
		req:       req,
	}
}

func (a *App) configManagerTaskRunsSnapshot() []configManagerTaskRun {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]configManagerTaskRun, 0, len(a.configManagerTaskRuns))
	for _, run := range a.configManagerTaskRuns {
		if run == nil || run.task == nil {
			continue
		}
		result = append(result, *run)
	}
	return result
}

func configManagerTaskTitle(req ConfigManagerRequest) string {
	title, _ := trimStringToUTF8Bytes(strings.TrimSpace(req.Instruction), 60)
	if title == "" {
		return "Config Manager Agent"
	}
	return title
}
