package app

import (
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"sync"
)

// writingTaskRun binds the reconnectable display task to the exact immutable
// runtime snapshot admitted for its root operation. Typed commands never
// reconstruct a binding from whichever session happens to be selected later.
type writingTaskRun struct {
	task            *apptask.Task
	runtime         ideChatRuntime
	recovery        *agentexecution.RecoveryObservation
	recoveryActions map[string]agentrun.CommandReceipt

	recoveryMutationMu sync.Mutex
	recoveryMutations  []writingRecoveryMutationBatch
}

func (run *writingTaskRun) matchesCurrent(a *App) bool {
	if run == nil || run.task == nil || a == nil || a.session == nil {
		return false
	}
	return run.runtime.workspace == a.workspace && run.runtime.sess == a.session
}

var (
	// ErrAgentCommandIDRequired is returned before any display task, model, or
	// canonical side effect is allocated for a root Agent request without caller identity.
	ErrAgentCommandIDRequired = apptask.ErrCommandIDRequired
	// ErrAgentCommandConflict means one caller identity was reused for a
	// different payload or lifecycle binding.
	ErrAgentCommandConflict = apptask.ErrCommandConflict
	// ErrAgentReplayCapacity means every bounded display replay slot is owned by
	// live work. Admission fails before the durable Runtime command is submitted.
	ErrAgentReplayCapacity = apptask.ErrReplayCapacity
)

func agentStartIdentity(commandID, scope, sessionID, fingerprint string) apptask.StartIdentity {
	return apptask.StartIdentity{
		CommandID: commandID, Scope: scope, SessionID: sessionID, Fingerprint: fingerprint,
	}
}

func agentStartRecord(commandID, scope, sessionID, fingerprint string, task *apptask.Task) apptask.StartRecord {
	return apptask.StartRecord{
		Identity: agentStartIdentity(commandID, scope, sessionID, fingerprint),
		Task:     task,
	}
}
