package configmanager

import (
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	appagentruntime "denova/internal/app/agentruntime"
	apptask "denova/internal/app/task"
	"fmt"
	"strings"
	"sync"
)

const maxRememberedRecoveries = 128

// configManagerRecoveryRun owns the single reconnectable display observer for
// one Config Manager binding after a cold Runtime recovery boundary.
type recoveryRun struct {
	workspace       string
	sessionID       string
	task            *apptask.Task
	recovery        *agentharness.RecoveryObservation
	recoveryActions map[string]agentrun.CommandReceipt
}

type recoveryRegistry struct {
	mu              sync.Mutex
	runs            map[string]*recoveryRun
	order           []string
	replayByteLimit int
}

func recoveryScopeKey(workspace, sessionID string) string {
	return strings.TrimSpace(workspace) + "\x00" + strings.TrimSpace(sessionID)
}

func (r *recoveryRegistry) current(workspace, sessionID string) *recoveryRun {
	if r == nil {
		return nil
	}
	key := recoveryScopeKey(workspace, sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	run := r.runs[key]
	if run != nil {
		r.order = apptask.TouchReplayKey(r.order, key)
	}
	return run
}

func (r *recoveryRegistry) install(run *recoveryRun) error {
	if r == nil || run == nil || run.task == nil {
		return fmt.Errorf("cannot register an empty Config Manager recovery run")
	}
	key := recoveryScopeKey(run.workspace, run.sessionID)
	if key == "\x00" {
		return fmt.Errorf("cannot register Config Manager recovery without a binding")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runs == nil {
		r.runs = make(map[string]*recoveryRun)
	}
	existing := r.runs[key]
	if existing == run {
		r.order = apptask.TouchReplayKey(r.order, key)
		return nil
	}
	if existing != nil && existing.task != nil && !existing.task.Finished() {
		return appagentruntime.ErrOperationActive
	}
	if existing != nil {
		existing.task.ReleaseDisplayReplay()
		delete(r.runs, key)
		r.removeOrderKeyLocked(key)
	}
	for len(r.runs) >= maxRememberedRecoveries {
		if !r.removeOldestSettledLocked() {
			return fmt.Errorf("%w: Config Manager recovery records=%d", apptask.ErrReplayCapacity, len(r.runs))
		}
	}
	r.runs[key] = run
	r.order = apptask.TouchReplayKey(r.order, key)
	r.pruneLocked()
	if r.runs[key] != run {
		return fmt.Errorf("%w: Config Manager recovery display was evicted during admission", apptask.ErrReplayCapacity)
	}
	if r.registryChargeLocked() > apptask.EffectiveRegistryReplayByteLimit(r.replayByteLimit) {
		delete(r.runs, key)
		r.removeOrderKeyLocked(key)
		return fmt.Errorf("%w: Config Manager recovery scope=%q", apptask.ErrReplayCapacity, run.sessionID)
	}
	return nil
}

func (r *recoveryRegistry) rollback(run *recoveryRun) {
	if r == nil || run == nil {
		return
	}
	key := recoveryScopeKey(run.workspace, run.sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runs[key] != run {
		return
	}
	delete(r.runs, key)
	r.removeOrderKeyLocked(key)
}

func (r *recoveryRegistry) releaseScope(workspace, sessionID string) *apptask.Task {
	if r == nil {
		return nil
	}
	key := recoveryScopeKey(workspace, sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[key]
	if run == nil {
		return nil
	}
	delete(r.runs, key)
	r.removeOrderKeyLocked(key)
	if run.task != nil && run.task.Finished() {
		run.task.ReleaseDisplayReplay()
	}
	return run.task
}

func (r *recoveryRegistry) pruneLocked() {
	if r == nil {
		return
	}
	for len(r.runs) > maxRememberedRecoveries {
		if !r.removeOldestSettledLocked() {
			return
		}
	}
	byteLimit := apptask.EffectiveRegistryReplayByteLimit(r.replayByteLimit)
	for r.registryChargeLocked() > byteLimit {
		if !r.removeOldestSettledLocked() {
			return
		}
	}
}

func (r *recoveryRegistry) registryChargeLocked() int {
	total := 0
	for _, run := range r.runs {
		if run != nil && run.task != nil {
			total += run.task.DisplayReplayCharge()
		}
	}
	return total
}

func (r *recoveryRegistry) removeOldestSettledLocked() bool {
	for index, key := range r.order {
		run := r.runs[key]
		if run != nil && run.task != nil && !run.task.Finished() {
			continue
		}
		if run != nil && run.task != nil {
			run.task.ReleaseDisplayReplay()
		}
		delete(r.runs, key)
		r.order = apptask.RemoveReplayKey(r.order, index)
		return true
	}
	return false
}

func (r *recoveryRegistry) removeOrderKeyLocked(key string) {
	for index, candidate := range r.order {
		if candidate == key {
			r.order = apptask.RemoveReplayKey(r.order, index)
			return
		}
	}
}
