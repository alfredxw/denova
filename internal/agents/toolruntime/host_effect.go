package toolruntime

import (
	"context"

	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
)

// ToolMutationOrigin is the stable host identity captured at durable turn
// admission. It intentionally excludes process-local callbacks and model/tool
// implementations so a cold Agent Session can route the same effect after
// restart.
type ToolMutationOrigin struct {
	AgentKind        string `json:"agent_kind"`
	ProjectID        string `json:"project_id,omitempty"`
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
// host reconciler after the public Agent canonical effect barrier. EffectID is
// the idempotency key; returning nil means the host has durably admitted the
// obligation, not merely queued process work.
type CommittedToolMutation struct {
	EffectID         agentrun.HostEffectID
	Binding          agentrun.RuntimeBinding
	RuntimeOperation agentrun.OperationID
	RuntimeCycle     int
	ToolCallID       string
	Origin           ToolMutationOrigin
	Mutation         agenttool.Mutation
}

// HostEffectReconciler durably admits one Agent-owned host effect.
// Implementations must be idempotent by EffectID and reject conflicting reuse.
type HostEffectReconciler func(context.Context, CommittedToolMutation) error
