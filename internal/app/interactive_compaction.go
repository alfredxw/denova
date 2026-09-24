package app

import (
	"context"
	"fmt"
	"log/slog"

	agentcompaction "denova/internal/agents/context/compaction"
	agentstructural "denova/internal/agents/context/structural"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	agentruntime "denova/internal/agents/runtime"
	compactionapp "denova/internal/app/compaction"
	interactiveapp "denova/internal/app/interactive"
)

func (s *InteractiveAppService) executeInteractiveContextCompaction(
	ctx context.Context,
	storyID, branchID, requestedCommandID string,
) (agentcompaction.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return agentcompaction.Result{}, ErrNoWorkspace
	}
	storyContext, err := store.StoryContext(storyID, branchID)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	branchID = storyContext.Snapshot.BranchID
	fence, err := s.drainInteractiveBinding(ctx, storyID, branchID)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	storyContext, err = store.StoryContext(storyID, branchID)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	expectedParent := storyContext.Meta.Branches[branchID].Head
	commandID, err := compactionapp.ResolveCommandID(
		requestedCommandID,
		agentstructural.CommandID(
			"game-compact", fence.workspace, storyID, branchID, expectedParent,
		),
	)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	prepared, err := s.prepareInteractiveAgentCycle(ctx, interactiveAgentCycleRequest{StoryID: storyID, BranchID: branchID})
	if err != nil {
		return agentcompaction.Result{}, err
	}
	s.app.mu.RLock()
	err = fence.validateLocked(s.app)
	s.app.mu.RUnlock()
	if err != nil {
		return agentcompaction.Result{}, err
	}
	if prepared.externalAssembly != nil {
		state, err := agentruntime.GameState(prepared.options(""), prepared.store)
		if err != nil {
			return agentcompaction.Result{}, err
		}
		control, err := s.app.AgentEngines().ExternalControl(prepared.options(""), state)
		if err != nil {
			return agentcompaction.Result{}, err
		}
		return control.Maintain(ctx, commandID, func() (agentcompaction.Result, error) {
			return interactiveapp.CompactExternal(ctx, interactiveapp.ExternalTurnConfig{Conversation: prepared.conversation, Request: prepared.request, Config: prepared.runtimeCfg, Assembly: *prepared.externalAssembly, BookService: prepared.bookService, Runtime: s.app.AgentEngines().ExternalRuntime(prepared.runtimeCfg), PrepareHistory: state.PrepareExternalHistory})
		})
	}
	cycle := agentexecution.Cycle{Definition: prepared.definition, Conversation: prepared.conversation, BookService: prepared.bookService, Request: prepared.request, Options: prepared.options("")}
	result, err := fence.chat.ExecuteStructuralOperation(ctx, cycle, agentstructural.Spec{
		CommandID: commandID,
		Action:    agentstructural.Compact,
		Ref:       agentrun.ContextCompactionRef{Force: true},
	})
	if err != nil {
		return result.Compaction, err
	}
	if !result.Compaction.Triggered {
		return result.Compaction, fmt.Errorf("没有可压缩的互动上下文 / No interactive context is available for compaction")
	}
	slog.InfoContext(ctx, "[interactive-agent] manual Agent Session compaction completed",
		"workspace", fence.workspace, "story_id", storyID, "branch_id", branchID,
		"revision", result.Compaction.Revision, "source_messages", result.Compaction.SourceMessageCount,
	)
	return result.Compaction, nil
}

func (s *InteractiveAppService) executeInteractiveContextCompactionRemoval(
	ctx context.Context,
	storyID, branchID, requestedCommandID string,
) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return false, ErrNoWorkspace
	}
	storyContext, err := store.StoryContext(storyID, branchID)
	if err != nil {
		return false, err
	}
	branchID = storyContext.Snapshot.BranchID
	fence, err := s.drainInteractiveBinding(ctx, storyID, branchID)
	if err != nil {
		return false, err
	}
	storyContext, err = store.StoryContext(storyID, branchID)
	if err != nil {
		return false, err
	}
	expectedParent := storyContext.Meta.Branches[branchID].Head
	commandID, err := compactionapp.ResolveCommandID(
		requestedCommandID,
		agentstructural.CommandID(
			"game-remove-compaction", fence.workspace, storyID, branchID, expectedParent,
		),
	)
	if err != nil {
		return false, err
	}
	cycle, err := s.prepareInteractiveStructuralCycle(ctx, fence, storyID, branchID)
	if err != nil {
		return false, err
	}
	result, err := fence.chat.ExecuteStructuralOperation(ctx, cycle, agentstructural.Spec{
		CommandID: commandID,
		Action:    agentstructural.Remove,
	})
	return result.Removed, err
}

func (s *InteractiveAppService) prepareInteractiveStructuralCycle(ctx context.Context, fence interactiveStructuralFence, storyID, branchID string) (agentexecution.Cycle, error) {
	cycle, err := s.prepareInteractiveAgentCycle(ctx, interactiveAgentCycleRequest{StoryID: storyID, BranchID: branchID})
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	s.app.mu.RLock()
	err = fence.validateLocked(s.app)
	s.app.mu.RUnlock()
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	if cycle.runtimeCfg.ProjectID != fence.projectID || cycle.workspace != fence.workspace ||
		cycle.storyID != storyID || cycle.branchID != branchID || cycle.executionRuntime != fence.chat {
		return agentexecution.Cycle{}, ErrAgentContextChanged
	}
	return agentexecution.Cycle{
		Definition: cycle.definition, Conversation: cycle.conversation, BookService: cycle.bookService,
		Request: cycle.request, Options: cycle.options(""),
	}, nil
}
