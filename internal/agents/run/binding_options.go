package agentrun

import (
	"fmt"

	"denova/config"
)

const imageAgentSessionID = "image-agent"

// RuntimeBindingForOptions derives and validates the single product identity
// used by public Agent Session admission, recovery, effects, and structural
// mutations. The returned value is normalized through the public Session key,
// so mutable metadata cannot leak back into durable ownership.
func RuntimeBindingForOptions(options Options) (RuntimeBinding, error) {
	options = options.Normalize(options.Workspace)
	var binding RuntimeBinding
	switch options.AgentKind {
	case AgentKindIDE:
		// Foreground Writing remains workspace/session-owned. ProjectID is
		// routing metadata for product events and must not fork the public
		// Agent Session identity when a Project is relinked or reindexed.
		projectID := options.ProjectID
		if options.Mode != ModeAgentChat {
			projectID = ""
		}
		binding = RuntimeBinding{
			AgentKind: options.AgentKind, ProjectID: projectID, Mode: options.Mode,
			Workspace: options.Workspace, SessionID: options.SessionID,
		}
	case AgentKindGeneral, AgentKindHarness:
		binding = RuntimeBinding{
			AgentKind: options.AgentKind, ProjectID: options.ProjectID, Mode: options.Mode,
			Workspace: options.Workspace, SessionID: options.SessionID,
		}
	case AgentKindInteractiveStory:
		binding = RuntimeBinding{
			AgentKind: options.AgentKind, Workspace: options.Workspace,
			StoryID: options.StoryID, BranchID: options.BranchID,
		}
	case AgentKindConfigManager:
		binding = RuntimeBinding{AgentKind: options.AgentKind, Workspace: options.Workspace, SessionID: options.SessionID}
	case AgentKindImage:
		sessionID := options.SessionID
		if sessionID == "" {
			sessionID = imageAgentSessionID
		}
		binding = RuntimeBinding{AgentKind: options.AgentKind, Workspace: options.Workspace, SessionID: sessionID}
	case AgentKindAutomation:
		taskID := options.AutomationTaskID
		if taskID == "" {
			taskID = options.TaskID
		}
		binding = RuntimeBinding{
			AgentKind: options.AgentKind, ProjectID: options.ProjectID, Workspace: options.Workspace,
			SessionID: options.SessionID, TaskID: taskID,
		}
	case config.AgentKindInteractiveDirector:
		binding = RuntimeBinding{
			AgentKind: options.AgentKind, Workspace: options.Workspace,
			StoryID: options.StoryID, BranchID: options.BranchID,
		}
	default:
		return RuntimeBinding{}, fmt.Errorf("%w: unsupported agent profile %q", ErrInvalidBinding, options.AgentKind)
	}
	key, err := binding.AgentSessionKey()
	if err != nil {
		return RuntimeBinding{}, err
	}
	return RuntimeBindingFromAgentSessionKey(key)
}
