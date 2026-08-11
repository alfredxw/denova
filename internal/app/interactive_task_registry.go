package app

import (
	"sort"
	"strings"
)

// InteractiveTaskInfo identifies the game-mode turn owned by a background
// task. The identity is kept separate from the Task event buffer so reconnect
// requests cannot attach a different story or branch by accident.
type InteractiveTaskInfo struct {
	TaskID               string
	Workspace            string
	StoryID              string
	StoryTitle           string
	BranchID             string
	Message              string
	RegenerateFromTurnID string
}

type interactiveTaskRun struct {
	task *Task
	info InteractiveTaskInfo
}

func interactiveTaskKey(workspace, storyID, branchID string) string {
	return canonicalTaskWorkspace(workspace) + "\x00" + strings.TrimSpace(storyID) + "\x00" + strings.TrimSpace(branchID)
}

func (s *InteractiveAppService) bindActiveInteractiveTask(task *Task, info InteractiveTaskInfo) bool {
	if s == nil || s.app == nil || task == nil {
		return false
	}
	info.TaskID = task.ID()
	info.Workspace = strings.TrimSpace(info.Workspace)
	info.StoryID = strings.TrimSpace(info.StoryID)
	info.StoryTitle = strings.TrimSpace(info.StoryTitle)
	info.BranchID = strings.TrimSpace(info.BranchID)
	info.RegenerateFromTurnID = strings.TrimSpace(info.RegenerateFromTurnID)

	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if info.Workspace == "" || info.StoryID == "" || info.BranchID == "" {
		return false
	}
	if a.interactiveTaskRuns == nil {
		a.interactiveTaskRuns = make(map[string]*interactiveTaskRun)
	}
	key := interactiveTaskKey(info.Workspace, info.StoryID, info.BranchID)
	if existing := a.interactiveTaskRuns[key]; existing != nil && existing.task != nil && existing.task.Status() == TaskRunning {
		return false
	}
	a.interactiveTaskRuns[key] = &interactiveTaskRun{task: task, info: info}
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
	if storyID != "" && branchID != "" {
		run := a.interactiveTaskRuns[interactiveTaskKey(a.workspace, storyID, branchID)]
		if run == nil || run.task == nil {
			return nil, InteractiveTaskInfo{}
		}
		return run.task, run.info
	}
	candidates := make([]*interactiveTaskRun, 0)
	for _, run := range a.interactiveTaskRuns {
		if run == nil || run.task == nil || run.info.Workspace != canonicalTaskWorkspace(a.workspace) {
			continue
		}
		if storyID != "" && run.info.StoryID != storyID {
			continue
		}
		if branchID != "" && run.info.BranchID != branchID {
			continue
		}
		candidates = append(candidates, run)
	}
	if len(candidates) == 0 {
		return nil, InteractiveTaskInfo{}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].task.Snapshot().StartedAt.After(candidates[j].task.Snapshot().StartedAt)
	})
	return candidates[0].task, candidates[0].info
}

func (s *InteractiveAppService) interactiveTaskRunning(workspace, storyID, branchID string) bool {
	if s == nil || s.app == nil {
		return false
	}
	a := s.app
	a.mu.RLock()
	run := a.interactiveTaskRuns[interactiveTaskKey(workspace, storyID, branchID)]
	a.mu.RUnlock()
	return run != nil && run.task != nil && run.task.Status() == TaskRunning
}

func (a *App) interactiveTasksSnapshot() []interactiveTaskRun {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]interactiveTaskRun, 0, len(a.interactiveTaskRuns))
	for _, run := range a.interactiveTaskRuns {
		if run == nil || run.task == nil {
			continue
		}
		result = append(result, *run)
	}
	return result
}
