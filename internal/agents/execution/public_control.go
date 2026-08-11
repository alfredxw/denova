package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"

	agentrun "denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

func (backend *publicBackend) openSession(ctx context.Context, options agentrun.Options) (*agent.Session, agentrun.RuntimeBinding, error) {
	if backend == nil || backend.agent == nil {
		return nil, agentrun.RuntimeBinding{}, ErrRuntimeProjectionUnavailable
	}
	options = options.Normalize(options.Workspace)
	ref, err := agentrun.BindingForOptions(options)
	if err != nil {
		return nil, agentrun.RuntimeBinding{}, err
	}
	binding, err := agentrun.ParseRuntimeBinding(ref)
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
	return publicRuntimeStatus(binding, snapshot), nil
}

func publicRuntimeStatus(binding agentrun.RuntimeBinding, snapshot agent.SessionSnapshot) agentrun.RuntimeStatus {
	status := agentrun.RuntimeStatus{
		Binding: binding, Cursor: agentrun.Cursor(snapshot.Cursor), Phase: agentrun.RunPhaseIdle,
		ActiveCommandID:          agentrun.CommandID(snapshot.ActiveCommandID),
		ActiveCommandFingerprint: snapshot.ActiveCommandFingerprint,
		ActiveReceiptCursor:      agentrun.Cursor(snapshot.ActiveReceiptCursor),
		ActiveOperation:          agentrun.OperationID(snapshot.ActiveRunID), ActiveCycle: snapshot.ActiveCycle,
		RecoveryPaused: snapshot.RecoveryPaused, RecoveryPending: snapshot.RecoveryPending,
		ActiveOutput: agentrun.ActiveOutput{
			OperationID: agentrun.OperationID(snapshot.ActiveRunID), Cycle: snapshot.ActiveCycle,
			Content: snapshot.ActiveOutput.Content, Thinking: snapshot.ActiveOutput.Thinking,
			ContentTruncated:  snapshot.ActiveOutput.ContentTruncated,
			ThinkingTruncated: snapshot.ActiveOutput.ThinkingTruncated,
			RehydrateRequired: snapshot.ActiveOutput.RehydrateRequired,
		},
		AgentRecoveryActions: make([]agentrun.AgentRecoveryAction, 0, len(snapshot.RecoveryActions)),
	}
	if snapshot.ActiveRunID != "" {
		status.Phase = agentrun.RunPhaseRunning
	}
	commandForRun := make(map[string]string, len(snapshot.QueuedRuns)+1)
	if snapshot.ActiveRunID != "" {
		commandForRun[snapshot.ActiveRunID] = snapshot.ActiveCommandID
	}
	for _, item := range snapshot.QueuedRuns {
		commandForRun[item.ID] = item.CommandID
		status.Queue = append(status.Queue, agentrun.QueuedCommand{
			CommandID: agentrun.CommandID(item.CommandID), OperationID: agentrun.OperationID(item.ID),
			Delivery: publicDeliveryKind(item.Delivery),
		})
	}
	for _, tool := range snapshot.OpenTools {
		status.OpenToolCalls = append(status.OpenToolCalls, agentrun.OpenToolCall{
			CallID: tool.CallID, Name: tool.Name,
			OperationID: agentrun.OperationID(snapshot.ActiveRunID), Cycle: snapshot.ActiveCycle,
		})
	}
	for _, run := range snapshot.RecentRuns {
		summary := agentrun.OperationSummary{
			OperationID: agentrun.OperationID(run.ID), CommandID: agentrun.CommandID(run.CommandID),
			CommandFingerprint: run.CommandFingerprint, ReceiptCursor: agentrun.Cursor(run.ReceiptCursor),
			Status: publicOperationStatus(run.Status), Reason: run.Reason,
		}
		status.RecentOperations = append(status.RecentOperations, summary)
	}
	if len(status.RecentOperations) > 0 {
		last := status.RecentOperations[len(status.RecentOperations)-1]
		status.LastOperation = &last
	}
	for _, recovery := range snapshot.RecoveryActions {
		status.AgentRecoveryActions = append(status.AgentRecoveryActions, agentrun.AgentRecoveryAction{
			ID: recovery.ID, Kind: string(recovery.Kind), RunID: recovery.RunID,
			CommandID: commandForRun[recovery.RunID], Delivery: string(recovery.Delivery),
			Compaction: string(recovery.Compaction),
		})
		if recovery.Kind == agent.RecoveryResumeCompaction {
			status.Phase = agentrun.RunPhaseCompacting
		}
	}
	if snapshot.Compaction != nil {
		projected := &agentrun.AgentCompactionState{
			ID: snapshot.Compaction.ID, Revision: snapshot.Compaction.Revision,
			Summary: snapshot.Compaction.Summary, TokenEstimate: snapshot.Compaction.TokenEstimate,
			ReplacementFrom: snapshot.Compaction.ReplacementFrom,
			ReplacementTo:   snapshot.Compaction.ReplacementTo,
		}
		if snapshot.Compaction.ContextData != nil {
			projected.ContextData = &agentrun.RestoreData{
				Type: snapshot.Compaction.ContextData.Type, Version: snapshot.Compaction.ContextData.Version,
				Data: append([]byte(nil), snapshot.Compaction.ContextData.Data...),
			}
		}
		status.Compaction = projected
	}
	return status
}

func publicDeliveryKind(delivery agent.RecoveryInputDelivery) agentrun.DeliveryKind {
	switch delivery {
	case agent.RecoveryDeliverySteer:
		return agentrun.DeliverySteer
	case agent.RecoveryDeliveryFollowUp:
		return agentrun.DeliveryFollowUp
	case agent.RecoveryDeliveryNextTurn:
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

func (backend *publicBackend) closeSessions(ctx context.Context, selector runstate.BindingSelector) error {
	if backend == nil || backend.agent == nil {
		return ErrRuntimeProjectionUnavailable
	}
	publicSelector := agent.SessionSelector{
		ID: selector.Key, Attributes: clonePublicLabels(selector.Labels),
	}
	if selector.Kind != "" || selector.Profile != "" {
		if selector.Kind == "" || selector.Profile == "" {
			return errors.New("Denova public Agent close requires an exact kind/profile pair")
		}
		publicSelector.Namespace = "denova." + strings.TrimSpace(selector.Kind) + "." + strings.TrimSpace(selector.Profile)
	}
	if err := publicSelector.Validate(); err != nil {
		return fmt.Errorf("derive public Agent Session selector: %w", err)
	}
	return backend.agent.CloseSessions(ctx, publicSelector)
}

func clonePublicLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	result := make(map[string]string, len(labels))
	for key, value := range labels {
		result[key] = value
	}
	return result
}
