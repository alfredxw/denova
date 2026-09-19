package automationapp

import (
	"context"
	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
	agenttoolruntime "denova/internal/agents/toolruntime"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"denova/internal/automation"
)

const admittedToolMutationVersion = 1

// admittedToolMutationPayload is the application-owned representation of a
// canonical tool mutation. The Automation store owns its later scheduling.
type admittedToolMutationPayload struct {
	Version          int                                 `json:"version"`
	Binding          agentrun.RuntimeBinding             `json:"binding"`
	RuntimeOperation agentrun.OperationID                `json:"runtime_operation"`
	RuntimeCycle     int                                 `json:"runtime_cycle"`
	ToolCallID       string                              `json:"tool_call_id"`
	Origin           agenttoolruntime.ToolMutationOrigin `json:"origin"`
	Mutation         agenttool.Mutation                  `json:"mutation"`
}

// ApplyToolMutation durably admits a canonical mutation before returning
// success to Agent.
func (s *Service) ApplyToolMutation(ctx context.Context, committed agenttoolruntime.CommittedToolMutation) error {
	if s == nil || s.host == nil {
		return fmt.Errorf("admit agent host effect: app configuration is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	projectID, workspace, portablePayload := portableAdmittedToolMutation(committed)
	payload, err := json.Marshal(portablePayload)
	if err != nil {
		return fmt.Errorf("encode agent host effect %q: %w", committed.EffectID, err)
	}
	catalog, err := s.host.Catalog()
	if err != nil {
		return fmt.Errorf("resolve automation catalog for host effect: %w", err)
	}
	store := automation.NewStore(catalog.DataDir, "")
	_, err = store.AdmitHostEffect(ctx, automation.HostEffectObligation{
		ID: string(committed.EffectID), Kind: agentrun.HostEffectToolMutationCommitted,
		ProjectID: projectID, Workspace: workspace, Payload: payload,
	})
	if err != nil {
		return fmt.Errorf("admit agent host effect %q: %w", committed.EffectID, err)
	}
	s.SignalReconciliation()
	return nil
}

// portableAdmittedToolMutation separates durable Project identity from the
// runtime filesystem projection. It is the last boundary before the global
// host-effect outbox is written.
func portableAdmittedToolMutation(committed agenttoolruntime.CommittedToolMutation) (string, string, admittedToolMutationPayload) {
	projectID := strings.TrimSpace(committed.Binding.ProjectID)
	if projectID == "" {
		projectID = strings.TrimSpace(committed.Origin.ProjectID)
	}
	origin := committed.Origin
	binding := committed.Binding
	mutation := committed.Mutation
	workspace := strings.TrimSpace(mutation.Workspace)
	if workspace == "" {
		workspace = strings.TrimSpace(origin.Workspace)
	}
	if projectID != "" {
		origin.ProjectID = projectID
		origin.Workspace = ""
		binding.ProjectID = projectID
		binding.Workspace = ""
		mutation.Workspace = ""
		workspace = ""
	}
	return projectID, workspace, admittedToolMutationPayload{
		Version: admittedToolMutationVersion, Binding: binding,
		RuntimeOperation: committed.RuntimeOperation, RuntimeCycle: committed.RuntimeCycle,
		ToolCallID: committed.ToolCallID, Origin: origin, Mutation: mutation,
	}
}

func (s *Service) reconcilePersistedHostEffects(ctx context.Context) {
	if s == nil || s.host == nil {
		return
	}
	store := s.storeAllWorkspaces()
	effects, err := store.ListHostEffects()
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[automation-host-effect] list durable obligations failed err=%v", err))
		return
	}
	for _, effect := range effects {
		queued, reconcileErr := s.reconcilePersistedHostEffect(ctx, effect)
		if reconcileErr != nil {
			slog.InfoContext(ctx, fmt.Sprintf("[automation-host-effect] obligation remains pending effect_id=%s workspace=%q err=%v", effect.ID, effect.Workspace, reconcileErr))
			continue
		}
		if queued {
			slog.InfoContext(ctx, fmt.Sprintf("[automation-host-effect] obligation transferred effect_id=%s workspace=%q", effect.ID, effect.Workspace))
		}
	}
}

// reconcilePersistedHostEffect transfers every Agent mutation through the same
// workspace trigger coordinator after Agent settlement. The bool
// reports successful admission/acknowledgement; false with nil means an async
// trigger pass now owns completion.
func (s *Service) reconcilePersistedHostEffect(ctx context.Context, effect automation.HostEffectObligation) (bool, error) {
	payload, err := decodeAdmittedToolMutation(effect)
	if err != nil {
		return false, err
	}

	// A released build could crash between transfer into its old outbox and
	// global acknowledgement. Preserve that single owner while draining it.
	if payload.Origin.AutomationTaskID != "" {
		_, legacy, lookupErr := s.storeAllWorkspaces().GetRunByID(payload.Origin.TaskID)
		if lookupErr != nil && !errors.Is(lookupErr, automation.ErrRunNotFound) {
			return false, lookupErr
		}
		for _, transferredID := range legacy.CompletionMutationEffectIDs {
			if transferredID == effect.ID {
				return true, s.storeAllWorkspaces().AcknowledgeHostEffect(ctx, effect)
			}
		}
	}
	if s.hostEffectOperationActive(ctx, payload) {
		return false, fmt.Errorf("agent operation %s is still active", payload.RuntimeOperation)
	}
	paths := automationCompletionMutationPaths([]agenttool.Mutation{payload.Mutation})
	workspace := strings.TrimSpace(effect.Workspace)
	if (effect.ProjectID == "" && workspace == "") || len(paths) == 0 {
		return true, s.storeAllWorkspaces().AcknowledgeHostEffect(ctx, effect)
	}
	snap, operation, err := s.acquireTargetRuntime(ctx, automation.ExecutionTarget{
		Kind: automation.TargetKindWorkspace, ProjectID: effect.ProjectID, Workspace: workspace,
	})
	if err != nil {
		return false, err
	}
	targets := s.chapterContentMutationPaths(snap, paths)
	if len(targets) == 0 {
		operation.Release()
		return true, s.storeAllWorkspaces().AcknowledgeHostEffect(ctx, effect)
	}
	if s.triggers == nil {
		operation.Release()
		return false, fmt.Errorf("automation mutation-effect coordinator is unavailable")
	}
	enqueued := s.triggers.EnqueueWithCompletion(
		s,
		snap,
		"agent_host_effect:"+effect.ID,
		targets,
		func(processErr error) {
			if processErr != nil {
				slog.ErrorContext(ctx, fmt.Sprintf("[automation-host-effect] trigger pass failed effect_id=%s workspace=%q err=%v", effect.ID, effect.Workspace, processErr))
				return
			}
			if ackErr := s.storeAllWorkspaces().AcknowledgeHostEffect(context.Background(), effect); ackErr != nil {
				slog.ErrorContext(ctx, fmt.Sprintf("[automation-host-effect] persist trigger receipt failed effect_id=%s workspace=%q err=%v", effect.ID, effect.Workspace, ackErr))
			}
		},
	)
	operation.Release()
	if !enqueued {
		return false, fmt.Errorf("workspace trigger pass could not be admitted")
	}
	return false, nil
}

func (s *Service) hostEffectOperationActive(ctx context.Context, payload admittedToolMutationPayload) bool {
	if s == nil || s.host == nil {
		return true
	}
	executionRuntime := s.host.BaseRuntime().ExecutionRuntime
	if executionRuntime == nil {
		return true
	}
	status, found, err := executionRuntime.OperationProjection(ctx, agentrun.Options{
		AgentKind: payload.Origin.AgentKind, ProjectID: payload.Origin.ProjectID,
		TaskID:           payload.Origin.TaskID,
		AutomationTaskID: payload.Origin.AutomationTaskID, SessionID: payload.Origin.SessionID,
		ReviewThreadID: payload.Origin.ReviewThreadID, StoryID: payload.Origin.StoryID,
		BranchID: payload.Origin.BranchID, TurnID: payload.Origin.TurnID,
		MaintenanceTask: payload.Origin.MaintenanceTask, Workspace: payload.Origin.Workspace,
		Mode: payload.Origin.Mode,
	}, string(payload.RuntimeOperation))
	if err != nil {
		slog.WarnContext(ctx, fmt.Sprintf("[automation-host-effect] runtime projection unavailable effect_operation=%s err=%v", payload.RuntimeOperation, err))
		return true
	}
	return !found || status.Outcome == nil
}

func decodeAdmittedToolMutation(effect automation.HostEffectObligation) (admittedToolMutationPayload, error) {
	if effect.Kind != agentrun.HostEffectToolMutationCommitted {
		return admittedToolMutationPayload{}, fmt.Errorf("unsupported admitted host effect kind %q", effect.Kind)
	}
	var payload admittedToolMutationPayload
	if err := json.Unmarshal(effect.Payload, &payload); err != nil {
		return admittedToolMutationPayload{}, fmt.Errorf("decode admitted host effect %q: %w", effect.ID, err)
	}
	if payload.Version != admittedToolMutationVersion || strings.TrimSpace(payload.ToolCallID) == "" ||
		strings.TrimSpace(payload.Mutation.ToolName) == "" || payload.Mutation.ToolCallID != payload.ToolCallID ||
		strings.TrimSpace(string(payload.RuntimeOperation)) == "" {
		return admittedToolMutationPayload{}, fmt.Errorf("admitted host effect %q has invalid identity", effect.ID)
	}
	return payload, nil
}
