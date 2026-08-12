// Package agentrun defines the stable execution contract shared by Agent
// composition, durable coordination, conversations, and application services.
package agentrun

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"denova/internal/agents/prompts"
	"denova/internal/agents/tool"
)

// RestoreData is bounded product-owned input required to rebuild a dynamic
// Definition after process restart. It never enters model context implicitly.
type RestoreData struct {
	Type    string
	Version uint16
	Data    json.RawMessage
}

const (
	AgentKindUnknown          = "unknown"
	AgentKindGeneral          = "general"
	AgentKindIDE              = "ide"
	AgentKindInteractiveStory = "interactive_story"
	AgentKindConfigManager    = "config_manager"
	AgentKindHarnessOptimizer = "harness_optimizer"
	AgentKindImage            = "image"
	AgentKindAutomation       = "automation"
)

const WriteModeReadOnly = "read_only"

// Options identifies one Agent run across runtime, trace, and UI surfaces.
type Options struct {
	AgentKind     string
	ProjectID     string
	StateRoot     string
	RootAgentName string
	TaskID        string
	// AutomationTaskID is the stable automation definition identity used by
	// the durable binding. TaskID remains the individual run/trace identity.
	AutomationTaskID   string
	SessionID          string
	ReviewThreadID     string
	StoryID            string
	BranchID           string
	TurnID             string
	MaintenanceTask    string
	Workspace          string
	Mode               string
	WriteMode          string
	WriteScope         string
	IdleTimeout        time.Duration
	ToolResultMaxBytes int
	// Controls carries lifecycle requests for this run only. Closing it is a no-op.
	Controls               <-chan Control
	SystemPromptLog        prompts.SystemPromptComposition
	OnMutationsVerified    func(context.Context, []agenttool.Mutation, agenttool.Verification)
	OnUserMessageCommitted func(context.Context) error
	RestoreData            *RestoreData
}

// Normalize applies defaults and canonical string forms without changing the
// caller-owned execution identity.
func (o Options) Normalize(defaultWorkspace string) Options {
	o.AgentKind = strings.TrimSpace(o.AgentKind)
	if o.AgentKind == "" {
		o.AgentKind = AgentKindUnknown
	}
	o.RootAgentName = strings.TrimSpace(o.RootAgentName)
	o.ProjectID = strings.TrimSpace(o.ProjectID)
	o.StateRoot = strings.TrimSpace(o.StateRoot)
	if o.RootAgentName == "" {
		o.RootAgentName = RootAgentName(o.AgentKind)
	}
	o.TaskID = strings.TrimSpace(o.TaskID)
	o.AutomationTaskID = strings.TrimSpace(o.AutomationTaskID)
	o.SessionID = strings.TrimSpace(o.SessionID)
	o.ReviewThreadID = strings.TrimSpace(o.ReviewThreadID)
	o.StoryID = strings.TrimSpace(o.StoryID)
	o.BranchID = strings.TrimSpace(o.BranchID)
	o.TurnID = strings.TrimSpace(o.TurnID)
	o.MaintenanceTask = strings.TrimSpace(o.MaintenanceTask)
	o.Workspace = strings.TrimSpace(o.Workspace)
	if o.Workspace == "" {
		o.Workspace = strings.TrimSpace(defaultWorkspace)
	}
	o.Mode = strings.TrimSpace(o.Mode)
	o.WriteMode = strings.ToLower(strings.TrimSpace(o.WriteMode))
	o.WriteScope = strings.ToLower(strings.TrimSpace(o.WriteScope))
	if o.IdleTimeout < 0 {
		o.IdleTimeout = 0
	}
	if o.ToolResultMaxBytes < 0 {
		o.ToolResultMaxBytes = 0
	}
	if o.RestoreData != nil {
		cloned := *o.RestoreData
		cloned.Type = strings.TrimSpace(cloned.Type)
		cloned.Data = append(json.RawMessage(nil), cloned.Data...)
		o.RestoreData = &cloned
	}
	return o
}

// RootAgentName returns the stable root name for a product Agent kind.
func RootAgentName(kind string) string {
	switch strings.TrimSpace(kind) {
	case AgentKindGeneral:
		return "DenovaGeneralAgent"
	case AgentKindIDE:
		return "DenovaAgent"
	case AgentKindInteractiveStory:
		return "DenovaInteractiveStoryAgent"
	case AgentKindConfigManager:
		return "DenovaConfigManagerAgent"
	case AgentKindHarnessOptimizer:
		return "DenovaHarnessOptimizer"
	case AgentKindImage:
		return "DenovaImageAgent"
	case AgentKindAutomation:
		return "DenovaAutomationAgent"
	default:
		return ""
	}
}
