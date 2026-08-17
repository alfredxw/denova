package automationapp

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

func (s *Service) ActiveAutomationRuns() []automation.ActiveRun {
	snap, err := s.runtimeSnapshot()
	if err != nil {
		// A workspace is optional for user-scoped automations. When no selected
		// runtime can be captured, keep the public projection useful without
		// leaking runs owned by an unrelated workspace.
		return s.activeAutomationRunsForWorkspace("", false)
	}
	return s.activeAutomationRuns(snap)
}

func (s *Service) activeAutomationRuns(snap *automationWorkspaceSnapshot) []automation.ActiveRun {
	if snap == nil {
		// Internal callers use nil explicitly for the catalog-wide diagnostic
		// view. The public method above always chooses a concrete scope.
		return s.activeAutomationRunsForWorkspace("", true)
	}
	return s.activeAutomationRunsForWorkspace(canonicalAutomationWorkspace(snap.workspace), false)
}

func (s *Service) activeAutomationRunsForWorkspace(workspace string, includeAll bool) []automation.ActiveRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]automation.ActiveRun, 0, len(s.activeRuns))
	for _, state := range s.activeRuns {
		// Global runs are relevant in every selected workspace. Workspace runs
		// remain isolated from one another, and a workspace-less public view is
		// deliberately global-only.
		if !includeAll && state.Workspace != "" && state.Workspace != workspace {
			continue
		}
		task := s.activeTasks[state.TaskKey]
		if task == nil || task.Status() != apptask.Running {
			continue
		}
		result = append(result, automation.ActiveRun{Run: state.Run, TaskID: state.TaskID})
	}
	return result
}

func (s *Service) hasActiveAutomationDefinition(taskID string) bool {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, claim := range s.activeClaims {
		if claim != nil && claim.taskID == taskID {
			return true
		}
	}
	return false
}

func (s *Service) ActiveAutomationTaskByRunID(runID string) (*apptask.Task, automation.RunRecord, bool) {
	return s.activeAutomationTaskByRunID(nil, runID)
}

func (s *Service) activeAutomationTaskByRunID(snap *automationWorkspaceSnapshot, runID string) (*apptask.Task, automation.RunRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.activeRuns == nil {
		return nil, automation.RunRecord{}, false
	}
	var state automationRunState
	found := false
	if snap != nil {
		workspace := canonicalAutomationWorkspace(snap.workspace)
		state, found = s.activeRuns[automationRunRegistryKey(workspace, runID)]
	} else {
		for _, candidate := range s.activeRuns {
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
	task := s.activeTasks[state.TaskKey]
	if task == nil || task.Status() != apptask.Running {
		return nil, automation.RunRecord{}, false
	}
	return task, state.Run, true
}

// reserveActiveAutomationRun performs the check-and-claim transition under
// Service.mu. Concurrent trigger checks wait for the owner to either publish its
// Task or release the reservation; they can never start a duplicate run.
func (s *Service) reserveActiveAutomationRun(ctx context.Context, snap *automationWorkspaceSnapshot, taskID string, run automation.RunRecord) (*automationRunClaim, bool, error) {
	workspace := canonicalAutomationWorkspace(snap.workspace)
	taskKey := automationTaskRegistryKey(workspace, taskID)
	for {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return nil, false, concurrency.ErrClosed
		}
		if s.activeClaims == nil {
			s.activeClaims = make(map[string]*automationRunClaim)
		}
		existing := s.activeClaims[taskKey]
		if existing == nil && strings.TrimSpace(run.ID) != "" {
			for _, candidate := range s.activeClaims {
				if candidate.workspace == workspace && candidate.runID == run.ID {
					existing = candidate
					break
				}
			}
		}
		if existing != nil {
			if existing.task != nil && existing.task.Status() != apptask.Running {
				s.removeAutomationClaimLocked(automationTaskRegistryKey(existing.workspace, existing.taskID), existing)
				s.mu.Unlock()
				continue
			}
			ready := existing.ready
			task := existing.task
			s.mu.Unlock()
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
		s.activeClaims[taskKey] = claim
		s.mu.Unlock()
		return claim, true, nil
	}
}

func (s *Service) activateAutomationClaim(claim *automationRunClaim, task *apptask.Task) error {
	if claim == nil || task == nil {
		return fmt.Errorf("automation run claim and Task are required")
	}
	taskKey := automationTaskRegistryKey(claim.workspace, claim.taskID)
	runKey := automationRunRegistryKey(claim.workspace, claim.runID)
	// Host admission is the authoritative fence against App.Close and workspace
	// replacement. Publish the automation identity only after it succeeds.
	if err := s.host.RegisterTask(task, claim.workspace); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		s.host.UnregisterTask(task)
		return concurrency.ErrClosed
	}
	if s.activeClaims[taskKey] != claim {
		s.host.UnregisterTask(task)
		return fmt.Errorf("automation run claim was released before activation")
	}
	if s.activeTasks == nil {
		s.activeTasks = make(map[string]*apptask.Task)
	}
	if s.activeRuns == nil {
		s.activeRuns = make(map[string]automationRunState)
	}
	claim.task = task
	s.activeTasks[taskKey] = task
	s.activeRuns[runKey] = automationRunState{
		Run:       claim.run,
		TaskID:    claim.run.TaskID,
		Workspace: claim.workspace,
		TaskKey:   taskKey,
	}
	close(claim.ready)
	return nil
}

func (s *Service) releaseAutomationClaim(claim *automationRunClaim) {
	if claim == nil {
		return
	}
	taskKey := automationTaskRegistryKey(claim.workspace, claim.taskID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeClaims[taskKey] == claim {
		s.removeAutomationClaimLocked(taskKey, claim)
	}
}

func (s *Service) clearActiveAutomationTask(snap *automationWorkspaceSnapshot, taskID, runID string) {
	workspace := canonicalAutomationWorkspace(snap.workspace)
	taskKey := automationTaskRegistryKey(workspace, taskID)
	s.mu.Lock()
	defer s.mu.Unlock()
	claim := s.activeClaims[taskKey]
	if claim == nil || claim.runID != runID {
		return
	}
	s.removeAutomationClaimLocked(taskKey, claim)
}

// updateActiveAutomationRun publishes a newly accepted successor receipt
// without changing the claim owner. The active API must expose the current
// operation (for exact Abort) while the persisted RunRecord retains its root
// StartTurn receipt separately.
func (s *Service) updateActiveAutomationRun(snap *automationWorkspaceSnapshot, taskID string, run automation.RunRecord) {
	if snap == nil {
		return
	}
	workspace := canonicalAutomationWorkspace(snap.workspace)
	taskKey := automationTaskRegistryKey(workspace, taskID)
	runKey := automationRunRegistryKey(workspace, run.ID)
	s.mu.Lock()
	defer s.mu.Unlock()
	claim := s.activeClaims[taskKey]
	if claim == nil || claim.runID != run.ID || claim.task == nil || claim.task.Status() != apptask.Running {
		return
	}
	claim.run = run
	if state, ok := s.activeRuns[runKey]; ok {
		state.Run = run
		s.activeRuns[runKey] = state
	}
}

func (s *Service) removeAutomationClaimLocked(taskKey string, claim *automationRunClaim) {
	delete(s.activeClaims, taskKey)
	if s.activeTasks != nil {
		delete(s.activeTasks, taskKey)
	}
	if s.activeRuns != nil {
		delete(s.activeRuns, automationRunRegistryKey(claim.workspace, claim.runID))
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
