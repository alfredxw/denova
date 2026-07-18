package app

import "strings"

// InteractiveTaskInfo identifies the game-mode turn owned by a background
// task. The identity is kept separate from the Task event buffer so reconnect
// requests cannot attach a different story or branch by accident.
type InteractiveTaskInfo struct {
	TaskID               string
	Workspace            string
	StoryID              string
	BranchID             string
	Message              string
	RegenerateFromTurnID string
}

type interactiveTaskRun struct {
	task *Task
	info InteractiveTaskInfo
}

func (s *InteractiveAppService) bindActiveInteractiveTask(task *Task, info InteractiveTaskInfo) bool {
	if s == nil || s.app == nil || task == nil {
		return false
	}
	info.TaskID = task.ID()
	info.Workspace = strings.TrimSpace(info.Workspace)
	info.StoryID = strings.TrimSpace(info.StoryID)
	info.BranchID = strings.TrimSpace(info.BranchID)
	info.RegenerateFromTurnID = strings.TrimSpace(info.RegenerateFromTurnID)

	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if info.Workspace == "" || a.workspace != info.Workspace {
		return false
	}
	a.activeInteractiveRun = &interactiveTaskRun{task: task, info: info}
	return true
}

// ActiveInteractiveTaskFor returns the reconnectable task only when the
// current workspace, story, and branch all match the request.
func (a *App) ActiveInteractiveTaskFor(storyID, branchID string) (*Task, InteractiveTaskInfo) {
	return a.interactiveService().ActiveInteractiveTaskFor(storyID, branchID)
}

func (s *InteractiveAppService) ActiveInteractiveTaskFor(storyID, branchID string) (*Task, InteractiveTaskInfo) {
	if s == nil || s.app == nil {
		return nil, InteractiveTaskInfo{}
	}
	storyID = strings.TrimSpace(storyID)
	branchID = strings.TrimSpace(branchID)
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	run := a.activeInteractiveRun
	if run == nil || run.task == nil || run.info.Workspace == "" || run.info.Workspace != a.workspace {
		return nil, InteractiveTaskInfo{}
	}
	if storyID != "" && run.info.StoryID != storyID {
		return nil, InteractiveTaskInfo{}
	}
	if branchID != "" && run.info.BranchID != branchID {
		return nil, InteractiveTaskInfo{}
	}
	return run.task, run.info
}
