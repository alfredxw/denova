package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"denova/internal/agentruntime"
)

const toolMutationHostEffectVersion = 1

// ToolMutationOrigin is the stable host identity captured at durable turn
// admission. It intentionally excludes process-local callbacks and model/tool
// implementations so a cold Runtime can route the same effect after restart.
type ToolMutationOrigin struct {
	AgentKind        string `json:"agent_kind"`
	TaskID           string `json:"task_id,omitempty"`
	AutomationTaskID string `json:"automation_task_id,omitempty"`
	SessionID        string `json:"session_id,omitempty"`
	ReviewThreadID   string `json:"review_thread_id,omitempty"`
	StoryID          string `json:"story_id,omitempty"`
	BranchID         string `json:"branch_id,omitempty"`
	TurnID           string `json:"turn_id,omitempty"`
	MaintenanceTask  string `json:"maintenance_task,omitempty"`
	Workspace        string `json:"workspace,omitempty"`
	Mode             string `json:"mode,omitempty"`
}

// CommittedToolMutation is delivered at least once through the process-level
// host reconciler. EffectID is the idempotency key; returning nil means the
// host has durably admitted the obligation, not merely queued process work.
type CommittedToolMutation struct {
	EffectID         agentruntime.HostEffectID
	Binding          agentruntime.BindingRef
	RuntimeOperation agentruntime.OperationID
	RuntimeCycle     int
	ToolCallID       string
	Origin           ToolMutationOrigin
	Mutation         ToolMutation
}

// HarnessHostEffectReconciler durably admits one Runtime-owned host effect.
// Implementations must be idempotent by EffectID and reject conflicting reuse.
type HarnessHostEffectReconciler func(context.Context, CommittedToolMutation) error

type toolMutationHostEffectPayload struct {
	Version  int                `json:"version"`
	Origin   ToolMutationOrigin `json:"origin"`
	Mutation ToolMutation       `json:"mutation"`
}

func toolMutationOrigin(options RunOptions) ToolMutationOrigin {
	options = options.normalized(options.Workspace)
	return ToolMutationOrigin{
		AgentKind: options.AgentKind, TaskID: options.TaskID, AutomationTaskID: options.AutomationTaskID,
		SessionID: options.SessionID, ReviewThreadID: options.ReviewThreadID,
		StoryID: options.StoryID, BranchID: options.BranchID, TurnID: options.TurnID,
		MaintenanceTask: options.MaintenanceTask, Workspace: options.Workspace, Mode: options.Mode,
	}
}

func newCommittedToolMutationHostEffect(
	binding agentruntime.BindingRef,
	operationID agentruntime.OperationID,
	cycle int,
	record ToolExecutionRecord,
	options RunOptions,
) (agentruntime.HostEffect, bool, error) {
	mutation, ok := toolMutationFromExecutionRecord(record)
	if !ok {
		return agentruntime.HostEffect{}, false, nil
	}
	origin := toolMutationOrigin(options)
	if strings.TrimSpace(mutation.Workspace) == "" {
		mutation.Workspace = origin.Workspace
	}
	payload, err := json.Marshal(toolMutationHostEffectPayload{
		Version: toolMutationHostEffectVersion, Origin: origin, Mutation: mutation,
	})
	if err != nil {
		return agentruntime.HostEffect{}, false, fmt.Errorf("encode committed tool mutation: %w", err)
	}
	effect, err := agentruntime.NewToolHostEffect(
		binding, operationID, cycle, record.ToolCallID, 0,
		agentruntime.HostEffectToolMutationCommitted, payload,
	)
	if err != nil {
		return agentruntime.HostEffect{}, false, fmt.Errorf("build committed tool mutation host effect: %w", err)
	}
	return effect, true, nil
}

func decodeCommittedToolMutationHostEffect(binding agentruntime.BindingRef, effect agentruntime.HostEffect) (CommittedToolMutation, error) {
	if effect.Kind != agentruntime.HostEffectToolMutationCommitted {
		return CommittedToolMutation{}, fmt.Errorf("unsupported agent host effect kind %q", effect.Kind)
	}
	var payload toolMutationHostEffectPayload
	if err := json.Unmarshal(effect.Payload, &payload); err != nil {
		return CommittedToolMutation{}, fmt.Errorf("decode committed tool mutation host effect: %w", err)
	}
	if payload.Version != toolMutationHostEffectVersion {
		return CommittedToolMutation{}, fmt.Errorf("unsupported committed tool mutation version %d", payload.Version)
	}
	if strings.TrimSpace(payload.Mutation.ToolCallID) == "" {
		payload.Mutation.ToolCallID = effect.CallID
	}
	if payload.Mutation.ToolCallID != effect.CallID || strings.TrimSpace(payload.Mutation.ToolName) == "" {
		return CommittedToolMutation{}, fmt.Errorf("committed tool mutation does not match effect call identity")
	}
	options := RunOptions{
		AgentKind: payload.Origin.AgentKind, TaskID: payload.Origin.TaskID,
		AutomationTaskID: payload.Origin.AutomationTaskID, SessionID: payload.Origin.SessionID,
		ReviewThreadID: payload.Origin.ReviewThreadID, StoryID: payload.Origin.StoryID,
		BranchID: payload.Origin.BranchID, TurnID: payload.Origin.TurnID,
		MaintenanceTask: payload.Origin.MaintenanceTask, Workspace: payload.Origin.Workspace,
		Mode: payload.Origin.Mode,
	}
	resolved, err := harnessBindingForOptions(options)
	if err != nil {
		return CommittedToolMutation{}, fmt.Errorf("resolve committed tool mutation binding: %w", err)
	}
	ref, err := agentruntime.BindingReference(resolved)
	if err != nil || ref != binding {
		return CommittedToolMutation{}, fmt.Errorf("committed tool mutation binding does not match runtime")
	}
	payload.Mutation.LoreItemIDs = append([]string(nil), payload.Mutation.LoreItemIDs...)
	payload.Mutation.DeletedLoreItemIDs = append([]string(nil), payload.Mutation.DeletedLoreItemIDs...)
	return CommittedToolMutation{
		EffectID: effect.ID, Binding: binding, RuntimeOperation: effect.OperationID,
		RuntimeCycle: effect.Cycle, ToolCallID: effect.CallID,
		Origin: payload.Origin, Mutation: payload.Mutation,
	}, nil
}
