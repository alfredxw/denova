package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentdelegation "denova/internal/agents/delegation"
	agentexecution "denova/internal/agents/execution"
	agentlifecycle "denova/internal/agents/lifecycle"
	agentrun "denova/internal/agents/run"
	appagentruntime "denova/internal/app/agentruntime"
)

func (a *App) prepareChildDefinition(
	ctx context.Context,
	request agentexecution.ChildDefinitionRequest,
) (agent.Definition, error) {
	binding, err := agentrun.RuntimeBindingFromAgentSessionKey(request.Parent)
	if err != nil {
		return agent.Definition{}, fmt.Errorf("decode delegated parent Session: %w", err)
	}
	turn, err := agentlifecycle.DecodeTurnHostData(agent.Input{Text: childParentText(request.HostData), HostData: request.HostData})
	if err != nil {
		return agent.Definition{}, fmt.Errorf("decode delegated parent input: %w", err)
	}
	parentRequest := turn.ChatRequest()
	finalize := func(definition agent.Definition, definitionErr error) (agent.Definition, error) {
		if definitionErr != nil {
			return agent.Definition{}, definitionErr
		}
		if a == nil || a.projectRegistry == nil {
			return agent.Definition{}, agentexecution.ErrCyclePreparationUnavailable
		}
		record, err := a.projectRegistry.Get(binding.ProjectID)
		if err != nil {
			return agent.Definition{}, err
		}
		layout, err := a.projectRegistry.EnsureStore(record)
		if err != nil {
			return agent.Definition{}, err
		}
		definition.AttachmentRoot = layout.StoreRoot
		return definition, nil
	}
	switch binding.AgentKind {
	case agentrun.AgentKindGeneral:
		return finalize(a.AgentChat().PrepareChildDefinition(ctx, binding, request.Child, parentRequest))
	case agentrun.AgentKindIDE:
		if binding.Mode == agentrun.ModeAgentChat {
			return finalize(a.AgentChat().PrepareChildDefinition(ctx, binding, request.Child, parentRequest))
		}
		runtime, _, err := a.chat().prepareIDEChatRuntime(ctx, parentRequest)
		if err != nil {
			return agent.Definition{}, err
		}
		if runtime.projectID != strings.TrimSpace(binding.ProjectID) || runtime.sess == nil || runtime.sess.ID != strings.TrimSpace(binding.SessionID) {
			return agent.Definition{}, fmt.Errorf("%w: delegated Writing parent is not the active Session", agentexecution.ErrCyclePreparationUnavailable)
		}
		agentHost, err := a.AgentHostCapabilities(ctx, &runtime.cfg, config.AgentKindIDE)
		if err != nil {
			return agent.Definition{}, err
		}
		built, err := appagentruntime.BuildConversationAgent(
			ctx, &runtime.cfg, runtime.state, runtime.ideTeller, config.AgentKindIDE,
			agentHost,
		)
		if err != nil {
			return agent.Definition{}, err
		}
		return finalize(agentdelegation.ChildDefinition(built.Definition, request.Child))
	case agentrun.AgentKindInteractiveStory:
		cycle, err := a.interactiveService().prepareInteractiveAgentCycle(ctx, interactiveAgentCycleRequest{
			CommandID: parentRequest.CommandID, StoryID: binding.StoryID, BranchID: binding.BranchID,
			Message: parentRequest.Message, StyleScenes: parentRequest.StyleScenes, Locale: parentRequest.Locale,
			InputVisibility: parentRequest.InputVisibility,
		})
		if err != nil {
			return agent.Definition{}, err
		}
		return finalize(agentdelegation.ChildDefinition(cycle.definition, request.Child))
	default:
		return agent.Definition{}, fmt.Errorf("%w: Agent kind %q does not support delegation", agentexecution.ErrCyclePreparationUnavailable, binding.AgentKind)
	}
}

func childParentText(data *agent.HostData) string {
	if data == nil {
		return ""
	}
	var envelope struct {
		Caller struct {
			Message string `json:"message"`
		} `json:"caller"`
	}
	_ = json.Unmarshal(data.Data, &envelope)
	return envelope.Caller.Message
}
