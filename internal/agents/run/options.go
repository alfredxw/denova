// Package agentrun defines the stable execution contract shared by Agent
// composition, durable coordination, conversations, and application services.
package agentrun

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
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

// InputCommitEffect is an idempotent product side effect coupled to accepted
// canonical input.
type InputCommitEffectRequest struct {
	CommandID   string
	OperationID string
	Cycle       int
	Hash        string
}

func (request InputCommitEffectRequest) ID() (string, error) {
	request.CommandID = strings.TrimSpace(request.CommandID)
	request.OperationID = strings.TrimSpace(request.OperationID)
	request.Hash = strings.TrimSpace(request.Hash)
	if request.CommandID == "" || request.OperationID == "" || request.Cycle <= 0 || request.Hash == "" {
		return "", fmt.Errorf("input commit effect requires command, operation, cycle, and hash")
	}
	digest := sha256.Sum256([]byte(request.CommandID + "\x00" + request.OperationID + "\x00" + fmt.Sprint(request.Cycle) + "\x00" + request.Hash))
	return fmt.Sprintf("agent-input-%x", digest[:16]), nil
}

type InputCommitEffect interface {
	Apply(context.Context, InputCommitEffectRequest) error
}

// InputCommitEffectFuncs adapts small host implementations.
type InputCommitEffectFuncs struct {
	ApplyFunc func(context.Context, InputCommitEffectRequest) error
}

func (effect InputCommitEffectFuncs) Apply(ctx context.Context, request InputCommitEffectRequest) error {
	if effect.ApplyFunc == nil {
		return nil
	}
	return effect.ApplyFunc(ctx, request)
}

const (
	AgentKindUnknown          = "unknown"
	AgentKindGeneral          = "general"
	AgentKindIDE              = "ide"
	AgentKindInteractiveStory = "interactive_story"
	AgentKindHarness          = "harness"
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
	Controls            <-chan Control
	SystemPromptLog     prompts.SystemPromptComposition
	OnMutationsVerified func(context.Context, []agenttool.Mutation, agenttool.Verification)
	InputCommitEffect   InputCommitEffect
	RestoreData         *RestoreData
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
	case AgentKindHarness:
		return "DenovaHarnessAgent"
	case AgentKindImage:
		return "DenovaImageAgent"
	case AgentKindAutomation:
		return "DenovaAutomationAgent"
	default:
		return ""
	}
}
