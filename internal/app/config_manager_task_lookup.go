package app

import (
	apptask "denova/internal/app/task"
	"strings"
)

// configManagerTaskRecord is the reconnectable, process-local display owner
// for one scoped Config Manager binding. Durable Runtime state remains the
// authority when this bounded display record has already been evicted.
type configManagerTaskRecord struct {
	CommandID string
	Task      *apptask.Task
}

// latestConfigManagerTask returns the most recently used display Task for an
// exact workspace/session binding. Config Manager reuses the bounded session
// start index, but never exposes its request fingerprint to the HTTP layer.
func (r *writingStartRegistry) latestSessionTask(workspace, sessionID string) configManagerTaskRecord {
	if r == nil {
		return configManagerTaskRecord{}
	}
	workspace = strings.TrimSpace(workspace)
	sessionID = strings.TrimSpace(sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked()
	for index := len(r.order) - 1; index >= 0; index-- {
		commandID := r.order[index]
		record, ok := r.records[commandID]
		if !ok || record.workspace != workspace || record.sessionID != sessionID || record.task == nil {
			continue
		}
		r.order = touchTaskReplayKey(r.order, commandID)
		return configManagerTaskRecord{CommandID: commandID, Task: record.task}
	}
	return configManagerTaskRecord{}
}

func (r *writingStartRegistry) latestConfigManagerTask(workspace, sessionID string) configManagerTaskRecord {
	return r.latestSessionTask(workspace, sessionID)
}

// releaseConfigManagerScope invalidates display-only replay after /clear.
// Canonical history is cleared separately through an append-only marker, and
// durable Runtime state must already have been drained by the caller.
func (r *writingStartRegistry) releaseConfigManagerScope(workspace, sessionID string) {
	if r == nil {
		return
	}
	workspace = strings.TrimSpace(workspace)
	sessionID = strings.TrimSpace(sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	for index := len(r.order) - 1; index >= 0; index-- {
		commandID := r.order[index]
		record, ok := r.records[commandID]
		if !ok {
			r.order = removeTaskReplayKey(r.order, index)
			continue
		}
		if record.workspace != workspace || record.sessionID != sessionID {
			continue
		}
		if record.task != nil {
			record.task.ReleaseDisplayReplay()
		}
		delete(r.records, commandID)
		r.order = removeTaskReplayKey(r.order, index)
	}
}
