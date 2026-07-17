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
	StoryID             string
	BranchID            string
	TurnID              string
	MaintenanceTask     string
	Workspace           string
	Mode                string
	IdleTimeout         time.Duration
	ToolResultMaxBytes  int
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
	o.StoryID = strings.TrimSpace(o.StoryID)
	o.BranchID = strings.TrimSpace(o.BranchID)
	o.TurnID = strings.TrimSpace(o.TurnID)
	o.MaintenanceTask = strings.TrimSpace(o.MaintenanceTask)
	o.Workspace = strings.TrimSpace(o.Workspace)
	if o.Workspace == "" {
		o.Workspace = strings.TrimSpace(defaultWorkspace)
	}
	o.Mode = strings.TrimSpace(o.Mode)
	if o.IdleTimeout < 0 {
		o.IdleTimeout = 0
	}
	if o.ToolResultMaxBytes < 0 {
		o.ToolResultMaxBytes = 0
	}
	return o
}

func rootAgentNameForKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case AgentKindIDE:
		return "DenovaAgent"
	case AgentKindInteractiveStory:
		return "DenovaInteractiveStoryAgent"
	case AgentKindConfigManager:
		return "DenovaConfigManagerAgent"
	case AgentKindImage:
		return "DenovaImageAgent"
	case AgentKindAutomation:
		return "DenovaAutomationAgent"
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

const runTraceMetadataValueMaxBytes = 256

func runTraceMetadataForConversation(options RunOptions, conversation Conversation) RunTraceMetadata {
	metadata := RunTraceMetadata{
		StoryID:         options.StoryID,
		BranchID:        options.BranchID,
		TurnID:          options.TurnID,
		MaintenanceTask: options.MaintenanceTask,
	}
	if reporter, ok := conversation.(RunTraceMetadataReporter); ok {
		reported := reporter.RunTraceMetadata()
		if strings.TrimSpace(reported.StoryID) != "" {
			metadata.StoryID = reported.StoryID
		}
		if strings.TrimSpace(reported.BranchID) != "" {
			metadata.BranchID = reported.BranchID
		}
		if strings.TrimSpace(reported.TurnID) != "" {
			metadata.TurnID = reported.TurnID
		}
		if strings.TrimSpace(reported.MaintenanceTask) != "" {
			metadata.MaintenanceTask = reported.MaintenanceTask
		}
	}
	metadata.StoryID = boundedRunTraceMetadataValue(metadata.StoryID)
	metadata.BranchID = boundedRunTraceMetadataValue(metadata.BranchID)
	metadata.TurnID = boundedRunTraceMetadataValue(metadata.TurnID)
	metadata.MaintenanceTask = boundedRunTraceMetadataValue(metadata.MaintenanceTask)
	return metadata
}

func boundedRunTraceMetadataValue(value string) string {
	return truncateUTF8StringBytes(strings.TrimSpace(value), runTraceMetadataValueMaxBytes)
}

func (m RunTraceMetadata) empty() bool {
	return m.StoryID == "" && m.BranchID == "" && m.TurnID == "" && m.MaintenanceTask == ""
}

func (m RunTraceMetadata) record() map[string]any {
	return map[string]any{
		"story_id":         m.StoryID,
		"branch_id":        m.BranchID,
		"turn_id":          m.TurnID,
		"maintenance_task": m.MaintenanceTask,
	}
}
