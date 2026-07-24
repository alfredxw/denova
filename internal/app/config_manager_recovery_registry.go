package app

import (
	"fmt"
	"strings"
	"sync"

	agents "denova/internal/agents"
)

const maxRememberedConfigManagerRecoveries = 128

// configManagerRecoveryRun owns the single reconnectable display observer for
// one Config Manager binding after a cold Runtime recovery boundary.
type configManagerRecoveryRun struct {
	workspace       string
	sessionID       string
	task            *Task
	recovery        *agents.RecoveryObservation
	recoveryActions map[string]agents.CommandReceipt
}

type configManagerRecoveryRegistry struct {
	mu              sync.Mutex
	runs            map[string]*configManagerRecoveryRun
	order           []string
	replayByteLimit int
}

func configManagerRecoveryScopeKey(workspace, sessionID string) string {
	return strings.TrimSpace(workspace) + "\x00" + strings.TrimSpace(sessionID)
}

func (r *configManagerRecoveryRegistry) current(workspace, sessionID string) *configManagerRecoveryRun {
	if r == nil {
		return nil
	}
	key := configManagerRecoveryScopeKey(workspace, sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	run := r.runs[key]
	if run != nil {
		r.order = touchTaskReplayKey(r.order, key)
	}
	return run
}

func (r *configManagerRecoveryRegistry) install(run *configManagerRecoveryRun) error {
	if r == nil || run == nil || run.task == nil {
		return fmt.Errorf("cannot register an empty Config Manager recovery run")
	}
	key := configManagerRecoveryScopeKey(run.workspace, run.sessionID)
	if key == "\x00" {
		return fmt.Errorf("cannot register Config Manager recovery without a binding")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runs == nil {
		r.runs = make(map[string]*configManagerRecoveryRun)
	}
	existing := r.runs[key]
	if existing == run {
		r.order = touchTaskReplayKey(r.order, key)
		return nil
	}
	if existing != nil && existing.task != nil && !existing.task.Finished() {
		return ErrAgentOperationActive
	}
	if existing != nil {
		existing.task.releaseDisplayReplay()
		delete(r.runs, key)
		r.removeOrderKeyLocked(key)
	}
	for len(r.runs) >= maxRememberedConfigManagerRecoveries {
		if !r.removeOldestSettledLocked() {
			return fmt.Errorf("%w: Config Manager recovery records=%d", ErrAgentReplayCapacity, len(r.runs))
		}
	}
	r.runs[key] = run
	r.order = touchTaskReplayKey(r.order, key)
	r.pruneLocked()
	if r.runs[key] != run {
		return fmt.Errorf("%w: Config Manager recovery display was evicted during admission", ErrAgentReplayCapacity)
	}
	if r.registryChargeLocked() > effectiveTaskRegistryReplayByteLimit(r.replayByteLimit) {
		delete(r.runs, key)
		r.removeOrderKeyLocked(key)
		return fmt.Errorf("%w: Config Manager recovery scope=%q", ErrAgentReplayCapacity, run.sessionID)
	}
	return nil
}

func (r *configManagerRecoveryRegistry) rollback(run *configManagerRecoveryRun) {
	if r == nil || run == nil {
		return
	}
	key := configManagerRecoveryScopeKey(run.workspace, run.sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runs[key] != run {
		return
	}
	delete(r.runs, key)
	r.removeOrderKeyLocked(key)
}

func (r *configManagerRecoveryRegistry) releaseScope(workspace, sessionID string) *Task {
	if r == nil {
		return nil
	}
	key := configManagerRecoveryScopeKey(workspace, sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[key]
	if run == nil {
		return nil
	}
	delete(r.runs, key)
	r.removeOrderKeyLocked(key)
	if run.task != nil && run.task.Finished() {
		run.task.releaseDisplayReplay()
	}
	return run.task
}

func (r *configManagerRecoveryRegistry) pruneLocked() {
	if r == nil {
		return
	}
	for len(r.runs) > maxRememberedConfigManagerRecoveries {
		if !r.removeOldestSettledLocked() {
			return
		}
	}
	byteLimit := effectiveTaskRegistryReplayByteLimit(r.replayByteLimit)
	for r.registryChargeLocked() > byteLimit {
		if !r.removeOldestSettledLocked() {
			return
		}
	}
}

func (r *configManagerRecoveryRegistry) registryChargeLocked() int {
	total := 0
	for _, run := range r.runs {
		if run != nil && run.task != nil {
			total += run.task.displayReplayRegistryCharge()
		}
	}
	return total
}

func (r *configManagerRecoveryRegistry) removeOldestSettledLocked() bool {
	for index, key := range r.order {
		run := r.runs[key]
		if run != nil && run.task != nil && !run.task.Finished() {
			continue
		}
		if run != nil && run.task != nil {
			run.task.releaseDisplayReplay()
		}
		delete(r.runs, key)
		r.order = removeTaskReplayKey(r.order, index)
		return true
	}
	return false
}

func (r *configManagerRecoveryRegistry) removeOrderKeyLocked(key string) {
	for index, candidate := range r.order {
		if candidate == key {
			r.order = removeTaskReplayKey(r.order, index)
			return
		}
	}
}
