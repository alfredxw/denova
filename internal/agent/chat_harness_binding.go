package agent

import (
	"fmt"
	"strings"

	"denova/config"
	runstate "denova/internal/agent/runtime"
)

const imageAgentHarnessSessionID = "image-agent"

func harnessBindingForOptions(options RunOptions) (runstate.Binding, error) {
	options = options.normalized(options.Workspace)
	switch options.AgentKind {
	case AgentKindIDE:
		if options.Workspace == "" || options.SessionID == "" {
			return nil, runstate.ErrInvalidBinding
		}
		return runstate.WritingBinding{Workspace: options.Workspace, SessionID: options.SessionID, Profile: runstate.ProfileWriting}, nil
	case AgentKindInteractiveStory:
		if options.Workspace == "" || options.StoryID == "" || options.BranchID == "" {
			return nil, runstate.ErrInvalidBinding
		}
		return runstate.GameBinding{Workspace: options.Workspace, StoryID: options.StoryID, BranchID: options.BranchID, Profile: runstate.ProfileGame}, nil
	case AgentKindConfigManager:
		if options.Workspace == "" || options.SessionID == "" {
			return nil, runstate.ErrInvalidBinding
		}
		return runstate.WritingBinding{Workspace: options.Workspace, SessionID: options.SessionID, Profile: runstate.ProfileConfigManager}, nil
	case AgentKindImage:
		if options.Workspace == "" {
			return nil, runstate.ErrInvalidBinding
		}
		sessionID := options.SessionID
		if sessionID == "" {
			sessionID = imageAgentHarnessSessionID
		}
		return runstate.WritingBinding{Workspace: options.Workspace, SessionID: sessionID, Profile: runstate.ProfileImage}, nil
	case AgentKindAutomation:
		automationTaskID := options.AutomationTaskID
		if automationTaskID == "" {
			automationTaskID = options.TaskID
		}
		if options.SessionID == "" || automationTaskID == "" {
			return nil, runstate.ErrInvalidBinding
		}
		return runstate.AutomationBinding{Workspace: options.Workspace, SessionID: options.SessionID, TaskID: automationTaskID, Profile: runstate.ProfileAutomation}, nil
	case config.AgentKindInteractiveDirector:
		if options.Workspace == "" || options.StoryID == "" || options.BranchID == "" {
			return nil, runstate.ErrInvalidBinding
		}
		return runstate.GameBinding{Workspace: options.Workspace, StoryID: options.StoryID, BranchID: options.BranchID, Profile: runstate.ProfileDirector}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported agent profile %q", runstate.ErrInvalidBinding, options.AgentKind)
	}
}

func harnessContextRefs(req ChatRequest) []runstate.ContextRef {
	caller := chatRequestCallerView(req)
	refs := make([]runstate.ContextRef, 0, len(caller.References)+len(caller.LoreReferences)+len(caller.StyleScenes)+len(caller.Selections)+len(caller.IDEContext.OpenFiles)+1)
	appendRef := func(source, resource, selector string) {
		resource = strings.TrimSpace(resource)
		if resource == "" {
			return
		}
		refs = append(refs, runstate.ContextRef{Source: source, Resource: resource, Selector: selector, ByteLimit: maxReferenceFileBytes})
	}
	for _, resource := range caller.References {
		appendRef("workspace_file", resource, "")
	}
	for _, resource := range caller.LoreReferences {
		appendRef("lore_item", resource, "")
	}
	for _, scene := range caller.StyleScenes {
		appendRef("style_scene", scene, "")
	}
	for _, selection := range caller.Selections {
		appendRef("editor_selection", selection.FileName, fmt.Sprintf("lines:%d-%d", selection.StartLine, selection.EndLine))
	}
	appendRef("ide_focus", caller.IDEContext.CurrentFile, "current")
	for _, resource := range caller.IDEContext.OpenFiles {
		appendRef("ide_focus", resource, "open")
	}
	return refs
}

func newHarnessIdentity(prefix string) string {
	return strings.TrimSpace(prefix) + "-" + newRunLedgerID()
}
