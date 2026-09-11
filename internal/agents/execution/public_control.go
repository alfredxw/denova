package execution

import (
	"context"
	"errors"

	agentrun "denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
)

func (backend *publicBackend) openSession(ctx context.Context, options agentrun.Options) (*agent.Session, agentrun.RuntimeBinding, error) {
	if backend == nil || backend.agent == nil {
		return nil, agentrun.RuntimeBinding{}, ErrRuntimeProjectionUnavailable
	}
	options = options.Normalize(options.Workspace)
	binding, err := agentrun.RuntimeBindingForOptions(options)
	if err != nil {
		return nil, agentrun.RuntimeBinding{}, err
	}
	key, err := binding.AgentSessionKey()
	if err != nil {
		return nil, agentrun.RuntimeBinding{}, err
	}
	session, err := backend.agent.Session(ctx, key)
	if err != nil {
		return nil, agentrun.RuntimeBinding{}, err
	}
	return session, binding, nil
}

func (backend *publicBackend) status(ctx context.Context, options agentrun.Options) (agentrun.RuntimeStatus, error) {
	session, binding, err := backend.openSession(ctx, options)
	if err != nil {
		return agentrun.RuntimeStatus{}, err
	}
	snapshot, err := session.Snapshot(ctx)
	if err != nil {
		return agentrun.RuntimeStatus{}, err
	}
	status := publicRuntimeStatus(binding, snapshot)
	sessions, err := backend.taskSessions(ctx, session)
	if err != nil {
		return agentrun.RuntimeStatus{}, err
	}
	for _, child := range sessions[1:] {
		childSnapshot, err := child.Snapshot(ctx)
		if err != nil {
			return agentrun.RuntimeStatus{}, err
		}
		for _, request := range childSnapshot.PendingInteractions {
			if request.Verification != nil {
				status.PendingInteractions = append(status.PendingInteractions, request)
			}
		}
	}
	return status, nil
}

func (backend *publicBackend) goal(ctx context.Context, options agentrun.Options) (agent.GoalState, bool, error) {
	session, _, err := backend.openSession(ctx, options)
	if err != nil {
		return agent.GoalState{}, false, err
	}
	return session.Goal(ctx)
}

func (backend *publicBackend) updateGoal(ctx context.Context, options agentrun.Options, mutation agent.GoalMutation) (agent.GoalState, error) {
	session, _, err := backend.openSession(ctx, options)
	if err != nil {
		return agent.GoalState{}, err
	}
	return session.UpdateGoal(ctx, mutation)
}

func (backend *publicBackend) clearSession(ctx context.Context, options agentrun.Options) error {
	session, _, err := backend.openSession(ctx, options)
	if err != nil {
		return err
	}
	return session.Clear(ctx)
}

func publicRuntimeStatus(binding agentrun.RuntimeBinding, snapshot agent.SessionSnapshot) agentrun.RuntimeStatus {
	status := agentrun.RuntimeStatus{
		Binding: binding, Cursor: agentrun.Cursor(snapshot.Cursor), Phase: agentrun.RunPhaseIdle,
		ActiveCommandID:     agentrun.CommandID(snapshot.ActiveCommandID),
		ActiveReceiptCursor: agentrun.Cursor(snapshot.ActiveReceiptCursor),
		ActiveOperation:     agentrun.OperationID(snapshot.ActiveRunID), ActiveCycle: snapshot.ActiveCycle,
		ActiveOutput: agentrun.ActiveOutput{
			OperationID: agentrun.OperationID(snapshot.ActiveRunID), Cycle: snapshot.ActiveCycle,
			Content: snapshot.ActiveOutput.Content, Thinking: snapshot.ActiveOutput.Thinking,
			ContentTruncated:  snapshot.ActiveOutput.ContentTruncated,
			ThinkingTruncated: snapshot.ActiveOutput.ThinkingTruncated,
			RehydrateRequired: snapshot.ActiveOutput.RehydrateRequired,
		},
	}
	if snapshot.ActiveRunID != "" {
		status.Phase = agentrun.RunPhaseRunning
		if snapshot.ActiveStatus == agent.ResultSuspended {
			status.Phase = agentrun.RunPhaseSuspended
		}
	}
	for _, item := range snapshot.QueuedRuns {
		status.Queue = append(status.Queue, agentrun.QueuedCommand{
			CommandID: agentrun.CommandID(item.CommandID), OperationID: agentrun.OperationID(item.ID),
			Delivery: publicDeliveryKind(item.Delivery), Message: item.Text,
			MessageTruncated: item.TextTruncated, SteerRequested: item.InterruptRequested,
		})
	}
	for _, tool := range snapshot.OpenTools {
		status.OpenToolCalls = append(status.OpenToolCalls, agentrun.OpenToolCall{
			CallID: tool.CallID, Name: tool.Name,
			OperationID: agentrun.OperationID(tool.RunID), Cycle: tool.Cycle,
		})
	}
	for _, run := range snapshot.RecentRuns {
		summary := agentrun.OperationSummary{
			OperationID: agentrun.OperationID(run.ID), CommandID: agentrun.CommandID(run.CommandID),
			ReceiptCursor: agentrun.Cursor(run.ReceiptCursor),
			Status:        publicOperationStatus(run.Status), Reason: run.Reason, ReasonTruncated: run.ReasonTruncated,
		}
		status.RecentOperations = append(status.RecentOperations, summary)
	}
	if len(status.RecentOperations) > 0 {
		last := status.RecentOperations[len(status.RecentOperations)-1]
		status.LastOperation = &last
	}
	if snapshot.Compaction != nil {
		projected := &agentrun.AgentCompactionState{
			ID: snapshot.Compaction.ID, Revision: snapshot.Compaction.Revision,
			Summary: snapshot.Compaction.Summary, TokensBefore: snapshot.Compaction.TokensBefore, TokensAfter: snapshot.Compaction.TokensAfter,
			SourceMessageCount: snapshot.Compaction.SourceMessageCount,
		}
		if snapshot.Compaction.ContextData != nil {
			projected.ContextData = &agentrun.RestoreData{
				Type: snapshot.Compaction.ContextData.Type, Version: snapshot.Compaction.ContextData.Version,
				Data: append([]byte(nil), snapshot.Compaction.ContextData.Data...),
			}
		}
		status.Compaction = projected
	}
	status.PendingInteractions = append([]agent.InteractionRequest(nil), snapshot.PendingInteractions...)
	return status
}

func (backend *publicBackend) resolveInteraction(
	ctx context.Context,
	options agentrun.Options,
	interactionID string,
	response agent.InteractionResponse,
) (agent.InteractionRequest, agent.InteractionResolution, error) {
	session, _, err := backend.openSession(ctx, options)
	if err != nil {
		return agent.InteractionRequest{}, agent.InteractionResolution{}, err
	}
	sessions, err := backend.taskSessions(ctx, session)
	if err != nil {
		return agent.InteractionRequest{}, agent.InteractionResolution{}, err
	}
	for _, candidate := range sessions {
		request, resolution, err := candidate.Respond(ctx, interactionID, response)
		if errors.Is(err, agent.ErrInteractionStale) {
			continue
		}
		return request, resolution, err
	}
	return agent.InteractionRequest{}, agent.InteractionResolution{}, agent.ErrInteractionStale
}

func publicDeliveryKind(delivery agent.InputDelivery) agentrun.DeliveryKind {
	switch delivery {
	case agent.DeliverySteer:
		return agentrun.DeliverySteer
	case agent.DeliveryFollowUp:
		return agentrun.DeliveryFollowUp
	case agent.DeliveryNextTurn:
		return agentrun.DeliveryNextTurn
	default:
		return ""
	}
}

func publicOperationStatus(status agent.ResultStatus) agentrun.OperationStatus {
	switch status {
	case agent.ResultCompleted:
		return agentrun.OperationSucceeded
	case agent.ResultAborted:
		return agentrun.OperationAborted
	default:
		return agentrun.OperationFailed
	}
}

func (backend *publicBackend) closeSessions(ctx context.Context, selector agent.SessionSelector) error {
	if backend == nil || backend.agent == nil {
		return ErrRuntimeProjectionUnavailable
	}
	return backend.agent.CloseSessions(ctx, selector)
}

func (backend *publicBackend) deleteSessions(ctx context.Context, selector agent.SessionSelector) error {
	if backend == nil || backend.agent == nil {
		return ErrRuntimeProjectionUnavailable
	}
	return backend.agent.DeleteSessions(ctx, selector)
}
