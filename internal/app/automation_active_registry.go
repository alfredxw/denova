package app

import (
	"context"
	apptask "denova/internal/app/task"
	"fmt"
	"strings"

	"denova/internal/automation"
	"denova/internal/concurrency"
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
	task      *apptask.Task
	ready     chan struct{}
}

func (s *AutomationAppService) ActiveAutomationRuns() []automation.ActiveRun {
	snap, err := s.runtimeSnapshot()
	if err != nil {
		// A workspace is optional for user-scoped automations. When no selected
		// runtime can be captured, keep the public projection useful without
		// leaking runs owned by an unrelated workspace.
		return s.activeAutomationRunsForWorkspace("", false)
	}
	return s.activeAutomationRuns(snap)
}

func (s *AutomationAppService) activeAutomationRuns(snap *automationWorkspaceSnapshot) []automation.ActiveRun {
	if snap == nil {
		// Internal callers use nil explicitly for the catalog-wide diagnostic
		// view. The public method above always chooses a concrete scope.
		return s.activeAutomationRunsForWorkspace("", true)
	}
	return s.activeAutomationRunsForWorkspace(canonicalAutomationWorkspace(snap.workspace), false)
}

func (s *AutomationAppService) activeAutomationRunsForWorkspace(workspace string, includeAll bool) []automation.ActiveRun {
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	result := make([]automation.ActiveRun, 0, len(s.app.activeAutomationRuns))
	for _, state := range s.app.activeAutomationRuns {
		// Global runs are relevant in every selected workspace. Workspace runs
		// remain isolated from one another, and a workspace-less public view is
		// deliberately global-only.
		if !includeAll && state.Workspace != "" && state.Workspace != workspace {
			continue
		}
		task := s.app.activeAutomationTasks[state.TaskKey]
		if task == nil || task.Status() != apptask.Running {
			continue
		}
		result = append(result, automation.ActiveRun{Run: state.Run, TaskID: state.TaskID})
	}
	return result
}

func (a *App) ActiveAutomationRuns() []automation.ActiveRun {
	return a.automation().ActiveAutomationRuns()
}

func (s *AutomationAppService) hasActiveAutomationDefinition(taskID string) bool {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false
	}
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	for _, claim := range s.app.activeAutomationClaims {
		if claim != nil && claim.taskID == taskID {
			return true
		}
	}
	return false
}

func (s *AutomationAppService) ActiveAutomationTaskByRunID(runID string) (*apptask.Task, automation.RunRecord, bool) {
	return s.activeAutomationTaskByRunID(nil, runID)
}

func (s *AutomationAppService) activeAutomationTaskByRunID(snap *automationWorkspaceSnapshot, runID string) (*apptask.Task, automation.RunRecord, bool) {
	s.app.mu.RLock()
	defer s.app.mu.RUnlock()
	if s.app.activeAutomationRuns == nil {
		return nil, automation.RunRecord{}, false
	}
	var state automationRunState
	found := false
	if snap != nil {
		workspace := canonicalAutomationWorkspace(snap.workspace)
		state, found = s.app.activeAutomationRuns[automationRunRegistryKey(workspace, runID)]
	} else {
		for _, candidate := range s.app.activeAutomationRuns {
			if candidate.Run.ID == strings.TrimSpace(runID) {
				state = candidate
				found = true
				break
			}
		}
	}
	if !found {
		return nil, automation.RunRecord{}, false
	}
	task := s.app.activeAutomationTasks[state.TaskKey]
	if task == nil || task.Status() != apptask.Running {
		return nil, automation.RunRecord{}, false
	}
	return task, state.Run, true
}

func (a *App) ActiveAutomationTaskByRunID(runID string) (*apptask.Task, automation.RunRecord, bool) {
	return a.automation().ActiveAutomationTaskByRunID(runID)
}

// reserveActiveAutomationRun performs the check-and-claim transition under
// App.mu. Concurrent trigger checks wait for the owner to either publish its
// Task or release the reservation; they can never start a duplicate run.
func (s *AutomationAppService) reserveActiveAutomationRun(ctx context.Context, snap *automationWorkspaceSnapshot, taskID string, run automation.RunRecord) (*automationRunClaim, bool, error) {
	workspace := canonicalAutomationWorkspace(snap.workspace)
	taskKey := automationTaskRegistryKey(workspace, taskID)
	for {
		s.app.mu.Lock()
		if s.app.closed {
			s.app.mu.Unlock()
			return nil, false, concurrency.ErrClosed
		}
		if s.app.activeAutomationClaims == nil {
			s.app.activeAutomationClaims = make(map[string]*automationRunClaim)
		}
		existing := s.app.activeAutomationClaims[taskKey]
		if existing == nil && strings.TrimSpace(run.ID) != "" {
			for _, candidate := range s.app.activeAutomationClaims {
				if candidate.workspace == workspace && candidate.runID == run.ID {
					existing = candidate
					break
				}
			}
		}
		if existing != nil {
			if existing.task != nil && existing.task.Status() != apptask.Running {
				s.removeAutomationClaimLocked(automationTaskRegistryKey(existing.workspace, existing.taskID), existing)
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

func (s *AutomationAppService) activateAutomationClaim(claim *automationRunClaim, task *apptask.Task) error {
	if claim == nil || task == nil {
		return fmt.Errorf("automation run claim and Task are required")
	}
	taskKey := automationTaskRegistryKey(claim.workspace, claim.taskID)
	runKey := automationRunRegistryKey(claim.workspace, claim.runID)
	s.app.mu.Lock()
	defer s.app.mu.Unlock()
	if s.app.closed {
		return concurrency.ErrClosed
	}
	if s.app.activeAutomationClaims[taskKey] != claim {
		return fmt.Errorf("automation run claim was released before activation")
	}
	if err := s.registerAutomationTaskLocked(task, claim.workspace); err != nil {
		return err
	}
	if s.app.activeAutomationTasks == nil {
		s.app.activeAutomationTasks = make(map[string]*apptask.Task)
	}
	if s.app.activeAutomationRuns == nil {
		s.app.activeAutomationRuns = make(map[string]automationRunState)
	}
	claim.task = task
	s.app.activeAutomationTasks[taskKey] = task
	s.app.activeAutomationRuns[runKey] = automationRunState{
		Run:       claim.run,
		TaskID:    claim.run.TaskID,
		Workspace: claim.workspace,
		TaskKey:   taskKey,
	}
	close(claim.ready)
	return nil
}

// registerAutomationTaskLocked admits the Task before its active-run identity
// becomes observable. Workspace automations own that exact workspace scope;
// user-wide automations own the App root. The caller holds App.mu, making
// lifecycle admission and claim publication one atomic transition with Close.
func (s *AutomationAppService) registerAutomationTaskLocked(task *apptask.Task, workspace string) error {
	if strings.TrimSpace(workspace) != "" {
		return s.app.registerWorkspaceTaskLocked(task, workspace, false)
	}
	if err := s.app.initializeLifecycleLocked(); err != nil {
		return err
	}
	return s.app.registerOwnedTaskLocked(task, "", s.app.rootScope)
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

func (s *AutomationAppService) clearActiveAutomationTask(snap *automationWorkspaceSnapshot, taskID, runID string) {
	workspace := canonicalAutomationWorkspace(snap.workspace)
	taskKey := automationTaskRegistryKey(workspace, taskID)
	s.app.mu.Lock()
	defer s.app.mu.Unlock()
	claim := s.app.activeAutomationClaims[taskKey]
	if claim == nil || claim.runID != runID {
		return
	}
	s.removeAutomationClaimLocked(taskKey, claim)
}

// updateActiveAutomationRun publishes a newly accepted successor receipt
// without changing the claim owner. The active API must expose the current
// operation (for exact Abort) while the persisted RunRecord retains its root
// StartTurn receipt separately.
func (s *AutomationAppService) updateActiveAutomationRun(snap *automationWorkspaceSnapshot, taskID string, run automation.RunRecord) {
	if snap == nil {
		return
	}
	workspace := canonicalAutomationWorkspace(snap.workspace)
	taskKey := automationTaskRegistryKey(workspace, taskID)
	runKey := automationRunRegistryKey(workspace, run.ID)
	s.app.mu.Lock()
	defer s.app.mu.Unlock()
	claim := s.app.activeAutomationClaims[taskKey]
	if claim == nil || claim.runID != run.ID || claim.task == nil || claim.task.Status() != apptask.Running {
		return
	}
	claim.run = run
	if state, ok := s.app.activeAutomationRuns[runKey]; ok {
		state.Run = run
		s.app.activeAutomationRuns[runKey] = state
	}
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
