package agent

import (
	"context"
	"strings"
	"time"
)

const (
	AgentKindUnknown          = "unknown"
	AgentKindIDE              = "ide"
	AgentKindInteractiveStory = "interactive_story"
	AgentKindConfigManager    = "config_manager"
	AgentKindImage            = "image"
	AgentKindAutomation       = "automation"
)

// RunOptions identifies one Agent run across runtime, trace, and UI surfaces.
type RunOptions struct {
	AgentKind           string
	RootAgentName       string
	TaskID              string
	SessionID           string
	Workspace           string
	Mode                string
	IdleTimeout         time.Duration
	SystemPromptLog     SystemPromptCompositionLog
	OnMutationsVerified func(context.Context, []ToolMutation, PostRunVerification)
}

func (o RunOptions) normalized(defaultWorkspace string) RunOptions {
	o.AgentKind = strings.TrimSpace(o.AgentKind)
	if o.AgentKind == "" {
		o.AgentKind = AgentKindUnknown
	}
	o.RootAgentName = strings.TrimSpace(o.RootAgentName)
	if o.RootAgentName == "" {
		o.RootAgentName = rootAgentNameForKind(o.AgentKind)
	}
	o.TaskID = strings.TrimSpace(o.TaskID)
	o.SessionID = strings.TrimSpace(o.SessionID)
	o.Workspace = strings.TrimSpace(o.Workspace)
	if o.Workspace == "" {
		o.Workspace = strings.TrimSpace(defaultWorkspace)
	}
	o.Mode = strings.TrimSpace(o.Mode)
	if o.IdleTimeout < 0 {
		o.IdleTimeout = 0
	}
	return o
}

func rootAgentNameForKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case AgentKindIDE:
		return "NovaAgent"
	case AgentKindInteractiveStory:
		return "NovaInteractiveStoryAgent"
	case AgentKindConfigManager:
		return "NovaConfigManagerAgent"
	case AgentKindImage:
		return "NovaImageAgent"
	case AgentKindAutomation:
		return "NovaAutomationAgent"
	default:
		return ""
	}
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
