package toolruntime

import (
	"context"

	agentrun "denova/internal/agents/run"
	agenttool "denova/internal/agents/tool"
)

// ToolMutationOrigin is the stable product identity attached to a canonical
// tool effect. It intentionally excludes process-local callbacks and model or
// tool implementations.
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

// CommittedToolMutation is delivered through Agent's canonical effect boundary.
// EffectID is the idempotency key; returning nil means the product accepted the
// mutation into its own authoritative state.
type CommittedToolMutation struct {
	EffectID         agentrun.HostEffectID
	Binding          agentrun.RuntimeBinding
	RuntimeOperation agentrun.OperationID
	RuntimeCycle     int
	ToolCallID       string
	Origin           ToolMutationOrigin
	Mutation         agenttool.Mutation
}

// ToolMutationApplier commits one canonical Agent tool mutation.
// Implementations must be idempotent by EffectID and reject conflicting reuse.
type ToolMutationApplier func(context.Context, CommittedToolMutation) error
