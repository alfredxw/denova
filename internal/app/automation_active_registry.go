package app

import (
	"context"
	"strings"

	"denova/internal/automation"
)

// automationRunState and automationRunClaim keep active execution identity
// scoped to a canonical workspace. User-scoped definitions may share task IDs
// across books, but their live agents must remain independent.
type automationRunState struct {
	Run       automation.RunRecord
	TaskID    string
	Workspace string
	TaskKey   string
}

type automationRunClaim struct {
	workspace string
	taskID    string
	runID     string
	run       automation.RunRecord
	task      *Task
	ready     chan struct{}
}

func (s *AutomationAppService) ActiveAutomationRuns() []automation.ActiveRun {
	workspace := canonicalAutomationWorkspace(s.workspace())
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	result := make([]automation.ActiveRun, 0, len(s.app.activeAutomationRuns))
	for _, state := range s.app.activeAutomationRuns {
		if state.Workspace != workspace {
			continue
		}
		task := s.app.activeAutomationTasks[state.TaskKey]
		if task == nil || task.Status() != TaskRunning {
			continue
		}
		result = append(result, automation.ActiveRun{Run: state.Run, TaskID: state.TaskID})
	}
	return result
}

func (a *App) ActiveAutomationRuns() []automation.ActiveRun {
	return a.automation().ActiveAutomationRuns()
}

func (s *AutomationAppService) ActiveAutomationTaskByRunID(runID string) (*Task, automation.RunRecord, bool) {
	workspace := canonicalAutomationWorkspace(s.workspace())
	runKey := automationRunRegistryKey(workspace, runID)
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	if s.app.activeAutomationRuns == nil {
		return nil, automation.RunRecord{}, false
	}
	state, ok := s.app.activeAutomationRuns[runKey]
	if !ok {
		return nil, automation.RunRecord{}, false
	}
	task := s.app.activeAutomationTasks[state.TaskKey]
	if task == nil || task.Status() != TaskRunning {
		return nil, automation.RunRecord{}, false
	}
	return task, state.Run, true
}

func (a *App) ActiveAutomationTaskByRunID(runID string) (*Task, automation.RunRecord, bool) {
	return a.automation().ActiveAutomationTaskByRunID(runID)
}

func (s *AutomationAppService) AbortAutomationRun(runID string) bool {
	task, _, ok := s.ActiveAutomationTaskByRunID(runID)
	if !ok {
		return false
	}
	task.Abort()
	return true
}

func (a *App) AbortAutomationRun(runID string) bool {
	return a.automation().AbortAutomationRun(runID)
}

// reserveActiveAutomationRun performs the check-and-claim transition under
// App.mu. Concurrent trigger checks wait for the owner to either publish its
// Task or release the reservation; they can never start a duplicate run.
func (s *AutomationAppService) reserveActiveAutomationRun(ctx context.Context, taskID string, run automation.RunRecord) (*automationRunClaim, bool, error) {
	workspace := canonicalAutomationWorkspace(s.workspace())
	taskKey := automationTaskRegistryKey(workspace, taskID)
	for {
		s.app.mu.Lock()
		if s.app.activeAutomationClaims == nil {
			s.app.activeAutomationClaims = make(map[string]*automationRunClaim)
		}
		if existing := s.app.activeAutomationClaims[taskKey]; existing != nil {
			if existing.task != nil && existing.task.Status() != TaskRunning {
				s.removeAutomationClaimLocked(taskKey, existing)
				s.app.mu.Unlock()
				continue
			}
			ready := existing.ready
			task := existing.task
			s.app.mu.Unlock()
			if task != nil {
				return existing, false, nil
			}
			select {
			case <-ready:
				continue
			case <-ctx.Done():
				return nil, false, ctx.Err()
			}
		}
		claim := &automationRunClaim{
			workspace: workspace,
			taskID:    taskID,
			runID:     run.ID,
			run:       run,
			ready:     make(chan struct{}),
		}
		s.app.activeAutomationClaims[taskKey] = claim
		s.app.mu.Unlock()
		return claim, true, nil
	}
}

func (s *AutomationAppService) activateAutomationClaim(claim *automationRunClaim, task *Task) bool {
	if claim == nil || task == nil {
		return false
	}
	taskKey := automationTaskRegistryKey(claim.workspace, claim.taskID)
	runKey := automationRunRegistryKey(claim.workspace, claim.runID)
	s.app.mu.Lock()
	defer s.app.mu.Unlock()
	if s.app.activeAutomationClaims[taskKey] != claim {
		return false
	}
	if s.app.activeAutomationTasks == nil {
		s.app.activeAutomationTasks = make(map[string]*Task)
	}
	if s.app.activeAutomationRuns == nil {
		s.app.activeAutomationRuns = make(map[string]automationRunState)
	}
	claim.task = task
	s.app.activeAutomationTasks[taskKey] = task
	s.app.activeAutomationRuns[runKey] = automationRunState{
		Run:       claim.run,
		TaskID:    claim.taskID,
		Workspace: claim.workspace,
		TaskKey:   taskKey,
	}
	close(claim.ready)
	return true
}

func (s *AutomationAppService) releaseAutomationClaim(claim *automationRunClaim) {
	if claim == nil {
		return
	}
	taskKey := automationTaskRegistryKey(claim.workspace, claim.taskID)
	s.app.mu.Lock()
	defer s.app.mu.Unlock()
	if s.app.activeAutomationClaims[taskKey] == claim {
		s.removeAutomationClaimLocked(taskKey, claim)
	}
}

func (s *AutomationAppService) clearActiveAutomationTask(taskID, runID string) {
	workspace := canonicalAutomationWorkspace(s.workspace())
	taskKey := automationTaskRegistryKey(workspace, taskID)
	s.app.mu.Lock()
	defer s.app.mu.Unlock()
	claim := s.app.activeAutomationClaims[taskKey]
	if claim == nil || claim.runID != runID {
		return
	}
	s.removeAutomationClaimLocked(taskKey, claim)
}

func (s *AutomationAppService) removeAutomationClaimLocked(taskKey string, claim *automationRunClaim) {
	delete(s.app.activeAutomationClaims, taskKey)
	if s.app.activeAutomationTasks != nil {
		delete(s.app.activeAutomationTasks, taskKey)
	}
	if s.app.activeAutomationRuns != nil {
		delete(s.app.activeAutomationRuns, automationRunRegistryKey(claim.workspace, claim.runID))
	}
	if claim.task == nil {
		close(claim.ready)
	}
}

func automationTaskRegistryKey(workspace, taskID string) string {
	return canonicalAutomationWorkspace(workspace) + "\x00task\x00" + strings.TrimSpace(taskID)
}

func automationRunRegistryKey(workspace, runID string) string {
	return canonicalAutomationWorkspace(workspace) + "\x00run\x00" + strings.TrimSpace(runID)
}
