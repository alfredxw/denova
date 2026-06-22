package agent

import (
	"context"
	"strings"
)

const (
	AgentKindUnknown          = "unknown"
	AgentKindIDE              = "ide"
	AgentKindInteractiveStory = "interactive_story"
	AgentKindConfigManager    = "config_manager"
	AgentKindAutomation       = "automation"
)

// RunOptions identifies one Agent run across runtime, trace, and UI surfaces.
type RunOptions struct {
	AgentKind           string
	TaskID              string
	SessionID           string
	Workspace           string
	Mode                string
	SystemPromptLog     SystemPromptCompositionLog
	OnMutationsVerified func(context.Context, []ToolMutation, PostRunVerification)
}

func (o RunOptions) normalized(defaultWorkspace string) RunOptions {
	o.AgentKind = strings.TrimSpace(o.AgentKind)
	if o.AgentKind == "" {
		o.AgentKind = AgentKindUnknown
	}
	o.TaskID = strings.TrimSpace(o.TaskID)
	o.SessionID = strings.TrimSpace(o.SessionID)
	o.Workspace = strings.TrimSpace(o.Workspace)
	if o.Workspace == "" {
		o.Workspace = strings.TrimSpace(defaultWorkspace)
	}
	o.Mode = strings.TrimSpace(o.Mode)
	return o
}

func (o RunOptions) checkpointID(runID string) string {
	parts := []string{strings.TrimSpace(o.AgentKind)}
	switch {
	case strings.TrimSpace(o.SessionID) != "":
		parts = append(parts, "session", strings.TrimSpace(o.SessionID))
	case strings.TrimSpace(o.TaskID) != "":
		parts = append(parts, "task", strings.TrimSpace(o.TaskID))
	case strings.TrimSpace(runID) != "":
		parts = append(parts, "run", strings.TrimSpace(runID))
	default:
		return ""
	}
	return strings.Join(parts, ":")
}
