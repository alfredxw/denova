package app

import (
	"denova/config"
	agents "denova/internal/agents"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

func appRuntimeBindingForTest(binding agents.RuntimeBinding) runstate.BindingRef {
	ref, err := binding.Ref()
	if err != nil {
		panic(err)
	}
	return ref
}

func writingRuntimeBindingForTest(workspace, sessionID string) runstate.BindingRef {
	return appRuntimeBindingForTest(agents.RuntimeBinding{AgentKind: agents.AgentKindIDE, Workspace: workspace, SessionID: sessionID})
}

func configRuntimeBindingForTest(workspace, sessionID string) runstate.BindingRef {
	return appRuntimeBindingForTest(agents.RuntimeBinding{AgentKind: agents.AgentKindConfigManager, Workspace: workspace, SessionID: sessionID})
}

func imageRuntimeBindingForTest(workspace, sessionID string) runstate.BindingRef {
	return appRuntimeBindingForTest(agents.RuntimeBinding{AgentKind: agents.AgentKindImage, Workspace: workspace, SessionID: sessionID})
}

func gameRuntimeBindingForTest(workspace, storyID, branchID string) runstate.BindingRef {
	return appRuntimeBindingForTest(agents.RuntimeBinding{AgentKind: agents.AgentKindInteractiveStory, Workspace: workspace, StoryID: storyID, BranchID: branchID})
}

func directorRuntimeBindingForTest(workspace, storyID, branchID string) runstate.BindingRef {
	return appRuntimeBindingForTest(agents.RuntimeBinding{AgentKind: config.AgentKindInteractiveDirector, Workspace: workspace, StoryID: storyID, BranchID: branchID})
}

func automationRuntimeBindingForTest(workspace, sessionID, taskID string) runstate.BindingRef {
	return appRuntimeBindingForTest(agents.RuntimeBinding{AgentKind: agents.AgentKindAutomation, Workspace: workspace, SessionID: sessionID, TaskID: taskID})
}
