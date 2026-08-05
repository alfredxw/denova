package automationapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"denova/config"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/conversationconfig"
	"denova/internal/automation"
)

// ConversationBinding identifies the durable conversation owned by one run.
type ConversationBinding struct {
	ProjectID string
	SessionID string
	RunID     string
}

// ConversationConfig returns the immutable model selection for one run.
func (s *Service) ConversationConfig(ctx context.Context, binding ConversationBinding) (conversationconfig.Snapshot, error) {
	snap, operation, run, task, err := s.conversationRuntime(ctx, binding)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	defer operation.Release()
	runtimeCfg := runtimeConfigForTask(snap, task)
	_, snapshot, err := agentconversation.GetOrCreateSession(snap.sessionStore, run.SessionID, &runtimeCfg, config.AgentKindAutomation)
	return snapshot, err
}

// PatchConversationConfig atomically rejects changes while a successor is
// active and applies a revision-checked model selection otherwise.
func (s *Service) PatchConversationConfig(
	ctx context.Context,
	binding ConversationBinding,
	patch conversationconfig.Patch,
	baseRevision uint64,
) (conversationconfig.Snapshot, error) {
	s.followUpAdmission.Lock()
	defer s.followUpAdmission.Unlock()
	snap, operation, run, task, err := s.conversationRuntime(ctx, binding)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	defer operation.Release()
	if active, _, ok := s.activeAutomationTaskByRunID(snap, run.ID); ok && active != nil && !active.Finished() {
		return conversationconfig.Snapshot{}, ErrOperationActive
	}
	runtimeCfg := runtimeConfigForTask(snap, task)
	sess, current, err := agentconversation.GetOrCreateSession(snap.sessionStore, run.SessionID, &runtimeCfg, config.AgentKindAutomation)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	next, err := conversationconfig.Merge(&runtimeCfg, current.Config, patch)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return sess.SetRuntimeConfig(next, baseRevision)
}

func (s *Service) conversationRuntime(
	ctx context.Context,
	binding ConversationBinding,
) (*automationWorkspaceSnapshot, Operation, automation.RunRecord, automation.Task, error) {
	runID := strings.TrimSpace(binding.RunID)
	if runID == "" {
		return nil, nil, automation.RunRecord{}, automation.Task{}, errors.New("automation run is required")
	}
	_, activeRun, active := s.ActiveAutomationTaskByRunID(runID)
	run := activeRun
	var err error
	if !active {
		run, err = s.automationRunByID(nil, runID)
		if err != nil {
			return nil, nil, automation.RunRecord{}, automation.Task{}, err
		}
	}
	if sessionID := strings.TrimSpace(binding.SessionID); sessionID != "" && sessionID != strings.TrimSpace(run.SessionID) {
		return nil, nil, automation.RunRecord{}, automation.Task{}, errors.New("automation conversation does not match the run")
	}
	if projectID := strings.TrimSpace(binding.ProjectID); projectID != "" && projectID != strings.TrimSpace(run.ProjectID) {
		return nil, nil, automation.RunRecord{}, automation.Task{}, errors.New("automation project does not match the run")
	}
	if strings.TrimSpace(run.SessionID) == "" {
		return nil, nil, automation.RunRecord{}, automation.Task{}, fmt.Errorf("automation run %s has no session history", run.ID)
	}
	target := automation.ExecutionTarget{Kind: automation.TargetKindUser}
	if strings.TrimSpace(run.Workspace) != "" {
		target = automation.ExecutionTarget{
			Kind: automation.TargetKindWorkspace, ProjectID: run.ProjectID, Workspace: run.Workspace,
		}
	}
	snap, operation, err := s.acquireTargetRuntime(ctx, target)
	if err != nil {
		return nil, nil, automation.RunRecord{}, automation.Task{}, err
	}
	task, err := storeForSnapshot(snap).Get(run.TaskID)
	if err != nil {
		operation.Release()
		return nil, nil, automation.RunRecord{}, automation.Task{}, err
	}
	return snap, operation, run, task, nil
}
