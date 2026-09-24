package execution

import (
	"context"
	agentrun "denova/internal/agents/run"
	agent "github.com/alfredxw/denova/agent"
	"time"
)

// OperationProjection reads one exact Agent Run, including runs outside bounded
// recent history. It contains no durable product state and never starts work.
type OperationProjection struct {
	Receipt    agentrun.CommandReceipt
	Phase      agentrun.RunPhase
	Queued     bool
	Outcome    *agentrun.Outcome
	Content    string
	FinishedAt time.Time
}

func projectOperation(snapshot agent.RunSnapshot) OperationProjection {
	view := OperationProjection{
		Receipt: agentrun.CommandReceipt{CommandID: agentrun.CommandID(snapshot.Receipt.CommandID), OperationID: agentrun.OperationID(snapshot.Receipt.RunID), Cursor: agentrun.Cursor(snapshot.Receipt.Cursor)},
		Phase:   agentrun.RunPhaseRunning, Queued: !snapshot.Started, Content: snapshot.Output, FinishedAt: snapshot.FinishedAt,
	}
	if snapshot.Suspended {
		view.Phase = agentrun.RunPhaseSuspended
	}
	if snapshot.Result != nil {
		view.Phase = agentrun.RunPhaseIdle
		outcome := publicResultOutcome(*snapshot.Result, nil, snapshot.Output, "")
		view.Outcome = &outcome
	}
	return view
}

// CommandProjection locates a root command by caller identity in its canonical
// Session. Callers must propagate errors; unavailable storage is not absence.
func (s *Runtime) CommandProjection(ctx context.Context, options agentrun.Options, commandID string) (OperationProjection, bool, error) {
	if s == nil || s.public == nil {
		return OperationProjection{}, false, ErrRuntimeProjectionUnavailable
	}
	session, _, err := s.public.openSession(ctx, options)
	if err != nil {
		return OperationProjection{}, false, err
	}
	run, found, err := session.CommandSnapshot(ctx, commandID)
	if err != nil || !found {
		return OperationProjection{}, false, err
	}
	return projectOperation(run), true, nil
}

// OperationProjection locates an operation in the product Session or its exact
// delegated descendants. Mutation outboxes use this as their settlement barrier.
func (s *Runtime) OperationProjection(ctx context.Context, options agentrun.Options, operationID string) (OperationProjection, bool, error) {
	if s == nil || s.public == nil {
		return OperationProjection{}, false, ErrRuntimeProjectionUnavailable
	}
	session, _, err := s.public.openSession(ctx, options)
	if err != nil {
		return OperationProjection{}, false, err
	}
	run, found, err := session.RunSnapshot(ctx, operationID)
	if err != nil {
		return OperationProjection{}, false, err
	}
	if found {
		return projectOperation(run), true, nil
	}
	// Delegated mutations retain the parent product identity and the child's
	// Run ID. Reuse canonical descendant discovery, including after reopening.
	sessions, err := s.public.taskSessions(ctx, session)
	if err != nil {
		return OperationProjection{}, false, err
	}
	for _, child := range sessions[1:] {
		run, found, err := child.RunSnapshot(ctx, operationID)
		if err != nil {
			return OperationProjection{}, false, err
		}
		if found {
			return projectOperation(run), true, nil
		}
	}
	return OperationProjection{}, false, nil
}
