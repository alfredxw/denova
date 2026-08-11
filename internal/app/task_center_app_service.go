package app

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"denova/internal/automation"
	"denova/internal/taskcenter"
)

// Tasks returns the cross-project task catalog used by the task center. Each
// source remains responsible for its full history and recovery workflow.
func (a *App) Tasks() (taskcenter.ListResult, error) {
	definitions, err := a.Automations()
	if err != nil {
		return taskcenter.ListResult{}, err
	}
	inbox, err := a.AutomationInbox()
	if err != nil {
		return taskcenter.ListResult{}, err
	}

	projectNames := make(map[string]string)
	for _, project := range a.Books() {
		projectNames[canonicalTaskWorkspace(project.Path)] = project.Name
	}
	definitionsByID := make(map[string]automation.Task, len(definitions)*2)
	result := taskcenter.ListResult{Tasks: make([]taskcenter.Task, 0)}
	for _, run := range a.agentTasksSnapshot() {
		task, taskErr := agentRunTask(run, projectNames)
		if taskErr != nil {
			return taskcenter.ListResult{}, taskErr
		}
		result.Tasks = append(result.Tasks, task)
		if taskcenter.IsActionRequired(task.Status) {
			result.ActionRequiredCount++
		}
	}
	for _, run := range a.interactiveTasksSnapshot() {
		task, taskErr := interactiveRunTask(run, projectNames)
		if taskErr != nil {
			return taskcenter.ListResult{}, taskErr
		}
		result.Tasks = append(result.Tasks, task)
		if taskcenter.IsActionRequired(task.Status) {
			result.ActionRequiredCount++
		}
	}
	for _, definition := range definitions {
		definitionsByID[definition.ID] = definition
		if definition.CatalogID != "" {
			definitionsByID[definition.CatalogID] = definition
		}
		for _, run := range definition.RecentRuns {
			task, taskErr := automationRunTask(definition, run, projectNames, a.Workspace())
			if taskErr != nil {
				return taskcenter.ListResult{}, taskErr
			}
			result.Tasks = append(result.Tasks, task)
			if taskcenter.IsActionRequired(task.Status) {
				result.ActionRequiredCount++
			}
		}
	}
	for _, item := range inbox {
		if item.Status != automation.InboxStatusPending {
			continue
		}
		definition := definitionsByID[item.TaskID]
		task := automationInboxTask(definition, item, projectNames, a.Workspace())
		result.Tasks = append(result.Tasks, task)
		result.ActionRequiredCount++
	}
	a.mu.RLock()
	loreImageTask := a.activeLoreImageTask
	a.mu.RUnlock()
	if loreImageTask != nil && !loreImageTask.Finished() {
		task, taskErr := loreImageGenerationTask(loreImageTask, projectNames, a.Workspace())
		if taskErr != nil {
			return taskcenter.ListResult{}, taskErr
		}
		result.Tasks = append(result.Tasks, task)
		if taskcenter.IsActionRequired(task.Status) {
			result.ActionRequiredCount++
		}
	}
	for _, run := range a.novelImportTasksSnapshot() {
		task, taskErr := novelImportTaskCenterTask(run, projectNames)
		if taskErr != nil {
			return taskcenter.ListResult{}, taskErr
		}
		result.Tasks = append(result.Tasks, task)
		if taskcenter.IsActionRequired(task.Status) {
			result.ActionRequiredCount++
		}
	}
	for _, run := range a.configManagerTaskRunsSnapshot() {
		task, taskErr := configManagerTaskCenterTask(run, projectNames)
		if taskErr != nil {
			return taskcenter.ListResult{}, taskErr
		}
		result.Tasks = append(result.Tasks, task)
		if taskcenter.IsActionRequired(task.Status) {
			result.ActionRequiredCount++
		}
	}
	sort.SliceStable(result.Tasks, func(i, j int) bool {
		if result.Tasks[i].UpdatedAt.Equal(result.Tasks[j].UpdatedAt) {
			return result.Tasks[i].ID < result.Tasks[j].ID
		}
		return result.Tasks[i].UpdatedAt.After(result.Tasks[j].UpdatedAt)
	})
	return result, nil
}

func configManagerTaskCenterTask(run configManagerTaskRun, projectNames map[string]string) (taskcenter.Task, error) {
	snapshot := run.task.Snapshot()
	status, err := agentTaskStatus(snapshot.Status)
	if err != nil {
		return taskcenter.Task{}, fmt.Errorf("config-manager task %s: %w", snapshot.ID, err)
	}
	workspace := run.workspace
	return taskcenter.Task{
		ID:        "agent:" + snapshot.ID,
		Type:      taskcenter.TaskTypeAgent,
		Status:    status,
		Title:     configManagerTaskTitle(run.req),
		Project:   taskProjectRef(workspace, projectNames),
		StartedAt: snapshot.StartedAt,
		UpdatedAt: snapshot.UpdatedAt,
		Recovery: taskcenter.RecoveryTarget{
			Kind:       taskcenter.RecoveryConfigManager,
			Workspace:  workspace,
			TaskID:     snapshot.ID,
			SessionID:  run.sessionID,
			Origin:     run.req.Origin,
			ResourceID: run.req.ResourceID,
			StoryID:    run.req.StoryID,
			BranchID:   run.req.BranchID,
		},
		Error: snapshot.Error,
	}, nil
}

func novelImportTaskCenterTask(run novelImportTaskSnapshot, projectNames map[string]string) (taskcenter.Task, error) {
	snapshot := run.task
	status, err := agentTaskStatus(snapshot.Status)
	if err != nil {
		return taskcenter.Task{}, fmt.Errorf("novel import task %s: %w", snapshot.ID, err)
	}
	workspace := run.sourceWorkspace
	title := run.title
	message := snapshot.Error
	if run.result != nil {
		if run.result.workspace != "" {
			workspace = run.result.workspace
		}
		title = run.result.title
		if status == taskcenter.StatusFailed {
			message = run.result.error
		}
	}
	return taskcenter.Task{
		ID:        "import-export:" + snapshot.ID,
		Type:      taskcenter.TaskTypeImportExport,
		Status:    status,
		Title:     title,
		Project:   taskProjectRef(workspace, projectNames),
		StartedAt: snapshot.StartedAt,
		UpdatedAt: snapshot.UpdatedAt,
		Recovery: taskcenter.RecoveryTarget{
			Kind:      taskcenter.RecoveryImportExport,
			Workspace: workspace,
			TaskID:    snapshot.ID,
		},
		Error: message,
	}, nil
}

func loreImageGenerationTask(task *Task, projectNames map[string]string, workspace string) (taskcenter.Task, error) {
	snapshot := task.Snapshot()
	status, err := agentTaskStatus(snapshot.Status)
	if err != nil {
		return taskcenter.Task{}, fmt.Errorf("image generation task %s: %w", snapshot.ID, err)
	}
	project := taskProjectRef(workspace, projectNames)
	return taskcenter.Task{
		ID:        "image-generation:" + snapshot.ID,
		Type:      taskcenter.TaskTypeImageGeneration,
		Status:    status,
		Title:     project.Name,
		Project:   project,
		StartedAt: snapshot.StartedAt,
		UpdatedAt: snapshot.UpdatedAt,
		Recovery: taskcenter.RecoveryTarget{
			Kind:      taskcenter.RecoveryImageGeneration,
			Workspace: workspace,
			TaskID:    snapshot.ID,
		},
		Error: snapshot.Error,
	}, nil
}

func agentRunTask(run agentTaskRun, projectNames map[string]string) (taskcenter.Task, error) {
	snapshot := run.task.Snapshot()
	status, err := agentTaskStatus(snapshot.Status)
	if err != nil {
		return taskcenter.Task{}, fmt.Errorf("agent task %s: %w", snapshot.ID, err)
	}
	title := strings.TrimSpace(run.info.SessionTitle)
	if title == "" {
		title = "Agent"
	}
	return taskcenter.Task{
		ID:        "agent:" + snapshot.ID,
		Type:      taskcenter.TaskTypeAgent,
		Status:    status,
		Title:     title,
		Project:   taskProjectRef(run.info.Workspace, projectNames),
		StartedAt: snapshot.StartedAt,
		UpdatedAt: snapshot.UpdatedAt,
		Recovery: taskcenter.RecoveryTarget{
			Kind:      taskcenter.RecoveryAgentSession,
			Workspace: run.info.Workspace,
			TaskID:    snapshot.ID,
			SessionID: run.info.SessionID,
		},
		Error: snapshot.Error,
	}, nil
}

func agentTaskStatus(status TaskStatus) (taskcenter.Status, error) {
	switch status {
	case TaskRunning:
		return taskcenter.StatusRunning, nil
	case TaskDone:
		return taskcenter.StatusCompleted, nil
	case TaskAborted:
		return taskcenter.StatusStopped, nil
	case TaskError:
		return taskcenter.StatusFailed, nil
	}
	return "", fmt.Errorf("unknown status %q", status)
}

func interactiveRunTask(run interactiveTaskRun, projectNames map[string]string) (taskcenter.Task, error) {
	snapshot := run.task.Snapshot()
	status, err := agentTaskStatus(snapshot.Status)
	if err != nil {
		return taskcenter.Task{}, fmt.Errorf("interactive task %s: %w", snapshot.ID, err)
	}
	title := strings.TrimSpace(run.info.StoryTitle)
	if title == "" {
		title = "Interactive Story"
	}
	return taskcenter.Task{
		ID:        "interactive-story:" + snapshot.ID,
		Type:      taskcenter.TaskTypeInteractiveStory,
		Status:    status,
		Title:     title,
		Project:   taskProjectRef(run.info.Workspace, projectNames),
		StartedAt: snapshot.StartedAt,
		UpdatedAt: snapshot.UpdatedAt,
		Recovery: taskcenter.RecoveryTarget{
			Kind:      taskcenter.RecoveryInteractiveStory,
			Workspace: run.info.Workspace,
			TaskID:    snapshot.ID,
			StoryID:   run.info.StoryID,
			BranchID:  run.info.BranchID,
		},
		Error: snapshot.Error,
	}, nil
}

func automationRunTask(definition automation.Task, run automation.RunRecord, projectNames map[string]string, currentWorkspace string) (taskcenter.Task, error) {
	status, err := automationTaskStatus(run.Status)
	if err != nil {
		return taskcenter.Task{}, fmt.Errorf("automation run %s: %w", run.ID, err)
	}
	workspace := automationTaskWorkspace(run.Workspace, definition.Target.Workspace, currentWorkspace)
	updatedAt := run.FinishedAt
	if updatedAt.IsZero() {
		updatedAt = run.StartedAt
	}
	return taskcenter.Task{
		ID:        "automation:" + run.ID,
		Type:      taskcenter.TaskTypeAutomation,
		Status:    status,
		Title:     definition.Name,
		Project:   taskProjectRef(workspace, projectNames),
		StartedAt: run.StartedAt,
		UpdatedAt: updatedAt,
		Recovery: taskcenter.RecoveryTarget{
			Kind:      taskcenter.RecoveryAutomationRun,
			Workspace: workspace,
			TaskID:    definition.ID,
			RunID:     run.ID,
		},
		Error: run.Error,
	}, nil
}

func automationInboxTask(definition automation.Task, item automation.TriggerInboxItem, projectNames map[string]string, currentWorkspace string) taskcenter.Task {
	workspace := automationTaskWorkspace(item.Workspace, definition.Target.Workspace, currentWorkspace)
	title := strings.TrimSpace(item.Title)
	if title == "" {
		title = definition.Name
	}
	updatedAt := item.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = item.CreatedAt
	}
	return taskcenter.Task{
		ID:        "automation-inbox:" + item.ID,
		Type:      taskcenter.TaskTypeAutomation,
		Status:    taskcenter.StatusWaitingUser,
		Title:     title,
		Project:   taskProjectRef(workspace, projectNames),
		StartedAt: item.CreatedAt,
		UpdatedAt: updatedAt,
		Recovery: taskcenter.RecoveryTarget{
			Kind:      taskcenter.RecoveryAutomationInbox,
			Workspace: workspace,
			TaskID:    definition.ID,
			InboxID:   item.ID,
		},
	}
}

func automationTaskStatus(status string) (taskcenter.Status, error) {
	switch status {
	case automation.RunStatusRunning:
		return taskcenter.StatusRunning, nil
	case automation.RunStatusSuccess:
		return taskcenter.StatusCompleted, nil
	case automation.RunStatusFailed:
		return taskcenter.StatusFailed, nil
	case automation.RunStatusAborted:
		return taskcenter.StatusStopped, nil
	}
	return "", fmt.Errorf("unknown status %q", status)
}

func automationTaskWorkspace(candidates ...string) string {
	for _, candidate := range candidates {
		if workspace := canonicalTaskWorkspace(candidate); workspace != "" {
			return workspace
		}
	}
	return ""
}

func canonicalTaskWorkspace(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return filepath.Clean(workspace)
	}
	return filepath.Clean(absolute)
}

func taskProjectRef(workspace string, projectNames map[string]string) taskcenter.ProjectRef {
	workspace = canonicalTaskWorkspace(workspace)
	name := strings.TrimSpace(projectNames[workspace])
	if name == "" && workspace != "" {
		name = filepath.Base(workspace)
	}
	return taskcenter.ProjectRef{Name: name, Path: workspace}
}
