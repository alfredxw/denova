package app

import (
	"denova/config"
	agentrun "denova/internal/agents/run"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

func appRuntimeBindingForTest(binding agentrun.RuntimeBinding) runstate.BindingRef {
	ref, err := binding.Ref()
	if err != nil {
		panic(err)
	}
	return ref
}

func writingRuntimeBindingForTest(workspace, sessionID string) runstate.BindingRef {
	return appRuntimeBindingForTest(agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Workspace: workspace, SessionID: sessionID})
}

func configRuntimeBindingForTest(workspace, sessionID string) runstate.BindingRef {
	return appRuntimeBindingForTest(agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindConfigManager, Workspace: workspace, SessionID: sessionID})
}

func imageRuntimeBindingForTest(workspace, sessionID string) runstate.BindingRef {
	return appRuntimeBindingForTest(agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindImage, Workspace: workspace, SessionID: sessionID})
}

func gameRuntimeBindingForTest(workspace, storyID, branchID string) runstate.BindingRef {
	return appRuntimeBindingForTest(agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace, StoryID: storyID, BranchID: branchID})
}

func directorRuntimeBindingForTest(workspace, storyID, branchID string) runstate.BindingRef {
	return appRuntimeBindingForTest(agentrun.RuntimeBinding{AgentKind: config.AgentKindInteractiveDirector, Workspace: workspace, StoryID: storyID, BranchID: branchID})
}

func automationRuntimeBindingForTest(workspace, sessionID, taskID string, projectIDs ...string) runstate.BindingRef {
	projectID := ""
	if len(projectIDs) > 0 {
		projectID = projectIDs[0]
	}
	return appRuntimeBindingForTest(agentrun.RuntimeBinding{
		AgentKind: agentrun.AgentKindAutomation,
		ProjectID: projectID,
		Workspace: workspace,
		SessionID: sessionID,
		TaskID:    taskID,
	})
}
