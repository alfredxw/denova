package automation

import "strings"

// portableTask removes host routing from Project-owned durable state. The
// owning Project store reconstructs these fields from ProjectID at read time.
func portableTask(task Task) Task {
	if task.Target.Kind != TargetKindWorkspace || strings.TrimSpace(task.Target.ProjectID) == "" {
		return task
	}
	task.Target.Workspace = ""
	if task.LastRun != nil {
		cloned := portableRun(*task.LastRun)
		task.LastRun = &cloned
	}
	task.RecentRuns = append([]RunRecord(nil), task.RecentRuns...)
	for index := range task.RecentRuns {
		task.RecentRuns[index] = portableRun(task.RecentRuns[index])
	}
	if len(task.TriggerState) > 0 {
		states := make(map[string]TriggerState, len(task.TriggerState))
		for key, state := range task.TriggerState {
			if state.Evaluation != nil {
				cloned := *state.Evaluation
				if strings.TrimSpace(cloned.ProjectID) != "" {
					cloned.Workspace = ""
				}
				state.Evaluation = &cloned
			}
			states[key] = state
		}
		task.TriggerState = states
	}
	return task
}

func portableRun(run RunRecord) RunRecord {
	if strings.TrimSpace(run.ProjectID) != "" {
		run.Workspace = ""
	}
	return run
}

func portableInboxItem(item TriggerInboxItem) TriggerInboxItem {
	if strings.TrimSpace(item.ProjectID) != "" {
		item.Workspace = ""
	}
	return item
}

func (s *Store) bindProjectTaskRuntime(task Task) Task {
	if s == nil || strings.TrimSpace(s.projectID) == "" || strings.TrimSpace(s.workspaceStateRoot) == "" || task.Scope != ScopeWorkspace {
		return task
	}
	task.Target.ProjectID = s.projectID
	task.Target.Workspace = s.workspace
	if task.LastRun != nil {
		cloned := s.bindProjectRunRuntime(*task.LastRun)
		task.LastRun = &cloned
	}
	for index := range task.RecentRuns {
		task.RecentRuns[index] = s.bindProjectRunRuntime(task.RecentRuns[index])
	}
	for key, state := range task.TriggerState {
		if state.Evaluation != nil {
			cloned := *state.Evaluation
			cloned.ProjectID = s.projectID
			cloned.Workspace = s.workspace
			state.Evaluation = &cloned
			task.TriggerState[key] = state
		}
	}
	return task
}

func (s *Store) bindProjectRunRuntime(run RunRecord) RunRecord {
	if s != nil && strings.TrimSpace(s.projectID) != "" && strings.TrimSpace(s.workspaceStateRoot) != "" {
		run.ProjectID = s.projectID
		run.Workspace = s.workspace
	}
	return run
}
