package app

import (
	"context"
	"fmt"
	"strings"

	agentchat "denova/internal/agents/chat"
	agentcompaction "denova/internal/agents/context/compaction"
	agentstructural "denova/internal/agents/context/structural"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	compactionapp "denova/internal/app/compaction"
)

func (s *ChatAppService) executeWritingContextCompaction(ctx context.Context, requestedCommandID string) (agentcompaction.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	fence, err := s.drainWritingBinding(ctx, "")
	if err != nil {
		return agentcompaction.Result{}, err
	}
	if fence.selected == nil {
		return agentcompaction.Result{}, ErrNoWorkspace
	}
	commandID, err := compactionapp.ResolveCommandID(
		requestedCommandID,
		agentstructural.CommandID(
			"writing-compact", fence.workspace, fence.selected.ID,
			fmt.Sprint(fence.selected.ContextCursor().Revision),
		),
	)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	cycle, err := s.prepareWritingStructuralCycle(ctx, fence)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	result, err := fence.chat.ExecuteStructuralOperation(ctx, cycle, agentstructural.Spec{
		CommandID: commandID,
		Action:    agentstructural.Compact,
		Ref:       agentrun.ContextCompactionRef{Force: true},
	})
	if err != nil {
		return result.Compaction, err
	}
	if !result.Compaction.Triggered {
		return result.Compaction, fmt.Errorf("没有可压缩的上下文 / No context is available for compaction")
	}
	return result.Compaction, nil
}

func (s *ChatAppService) executeWritingContextCompactionRemoval(ctx context.Context, requestedCommandID string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	fence, err := s.drainWritingBinding(ctx, "")
	if err != nil {
		return false, err
	}
	if fence.selected == nil {
		return false, ErrNoWorkspace
	}
	commandID, err := compactionapp.ResolveCommandID(
		strings.TrimSpace(requestedCommandID),
		agentstructural.CommandID(
			"writing-remove-compaction", fence.workspace, fence.selected.ID,
			fmt.Sprint(fence.selected.ContextCursor().Revision),
		),
	)
	if err != nil {
		return false, err
	}
	cycle, err := s.prepareWritingStructuralCycle(ctx, fence)
	if err != nil {
		return false, err
	}
	result, err := fence.chat.ExecuteStructuralOperation(ctx, cycle, agentstructural.Spec{
		CommandID: commandID,
		Action:    agentstructural.Remove,
	})
	return result.Removed, err
}

func (s *ChatAppService) prepareWritingStructuralCycle(ctx context.Context, fence writingStructuralFence) (agentexecution.Cycle, error) {
	cycle, runtime, err := s.prepareWritingCycle(ctx, agentchat.ChatRequest{}, "")
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	s.app.mu.RLock()
	err = fence.validateLocked(s.app, true)
	s.app.mu.RUnlock()
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	if runtime.projectID != fence.projectID || runtime.workspace != fence.workspace ||
		runtime.sess != fence.selected || runtime.executionRuntime != fence.chat {
		return agentexecution.Cycle{}, ErrAgentContextChanged
	}
	return cycle, nil
}
