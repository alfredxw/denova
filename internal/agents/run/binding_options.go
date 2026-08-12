package agentrun

import (
	"fmt"

	"denova/config"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

const imageAgentSessionID = "image-agent"

// BindingForOptions derives the single durable runtime identity used by turn
// admission, recovery, host-effect reconciliation, and structural mutations.
func BindingForOptions(options Options) (runstate.BindingRef, error) {
	options = options.Normalize(options.Workspace)
	switch options.AgentKind {
	case AgentKindGeneral, AgentKindIDE:
		return (RuntimeBinding{
			AgentKind: options.AgentKind, ProjectID: options.ProjectID, Mode: options.Mode,
			Workspace: options.Workspace, SessionID: options.SessionID,
		}).Ref()
	case AgentKindInteractiveStory:
		return (RuntimeBinding{
			AgentKind: options.AgentKind, Workspace: options.Workspace,
			StoryID: options.StoryID, BranchID: options.BranchID,
		}).Ref()
	case AgentKindConfigManager:
		return (RuntimeBinding{AgentKind: options.AgentKind, Workspace: options.Workspace, SessionID: options.SessionID}).Ref()
	case AgentKindHarnessOptimizer:
		return (RuntimeBinding{AgentKind: options.AgentKind, SessionID: options.SessionID}).Ref()
	case AgentKindImage:
		sessionID := options.SessionID
		if sessionID == "" {
			sessionID = imageAgentSessionID
		}
		return (RuntimeBinding{AgentKind: options.AgentKind, Workspace: options.Workspace, SessionID: sessionID}).Ref()
	case AgentKindAutomation:
		taskID := options.AutomationTaskID
		if taskID == "" {
			taskID = options.TaskID
		}
		return (RuntimeBinding{
			AgentKind: options.AgentKind, ProjectID: options.ProjectID, Workspace: options.Workspace,
			SessionID: options.SessionID, TaskID: taskID,
		}).Ref()
	case config.AgentKindInteractiveDirector:
		return (RuntimeBinding{
			AgentKind: options.AgentKind, Workspace: options.Workspace,
			StoryID: options.StoryID, BranchID: options.BranchID,
		}).Ref()
	default:
		return runstate.BindingRef{}, fmt.Errorf("%w: unsupported agent profile %q", runstate.ErrInvalidBinding, options.AgentKind)
	}
}
