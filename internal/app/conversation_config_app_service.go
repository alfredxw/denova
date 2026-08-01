package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"denova/config"
	"denova/internal/agents/session"
	"denova/internal/automation"
	"denova/internal/conversationconfig"
	"denova/internal/interactive"
)

const (
	ConversationModeWriting       = "writing"
	ConversationModeAgentChat     = "agent_chat"
	ConversationModeInteractive   = "interactive"
	ConversationModeConfigManager = "config_manager"
	ConversationModeAutomation    = "automation"
)

// ConversationConfigBinding is the stable transport identity for every
// creator-visible conversation surface. AgentKind is always derived server
// side from the owning project/mode and is never caller-controlled.
type ConversationConfigBinding struct {
	Mode       string `json:"mode"`
	ProjectID  string `json:"project_id,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	StoryID    string `json:"story_id,omitempty"`
	BranchID   string `json:"branch_id,omitempty"`
	Origin     string `json:"origin,omitempty"`
	ResourceID string `json:"resource_id,omitempty"`
	RunID      string `json:"run_id,omitempty"`
}

func (a *App) ConversationConfig(ctx context.Context, binding ConversationConfigBinding) (conversationconfig.Snapshot, error) {
	if a == nil {
		return conversationconfig.Snapshot{}, ErrNoWorkspace
	}
	switch normalizeConversationMode(binding.Mode) {
	case ConversationModeWriting:
		return a.writingConversationConfig(binding)
	case ConversationModeAgentChat:
		return a.agentChatConversationConfig(ctx, binding)
	case ConversationModeConfigManager:
		return a.configManagerConversationConfig(binding)
	case ConversationModeInteractive:
		return a.interactiveConversationConfig(binding)
	case ConversationModeAutomation:
		return a.automationConversationConfig(ctx, binding)
	default:
		return conversationconfig.Snapshot{}, fmt.Errorf("unsupported conversation mode %q", binding.Mode)
	}
}

func (a *App) PatchConversationConfig(ctx context.Context, binding ConversationConfigBinding, patch conversationconfig.Patch, baseRevision uint64) (conversationconfig.Snapshot, error) {
	if patch.ProfileID == nil && patch.ThinkingLevel == nil && patch.ApprovalMode == nil {
		return conversationconfig.Snapshot{}, errors.New("conversation config changes are empty")
	}
	switch normalizeConversationMode(binding.Mode) {
	case ConversationModeWriting:
		return a.patchWritingConversationConfig(binding, patch, baseRevision)
	case ConversationModeAgentChat:
		return a.patchAgentChatConversationConfig(ctx, binding, patch, baseRevision)
	case ConversationModeConfigManager:
		return a.patchConfigManagerConversationConfig(binding, patch, baseRevision)
	case ConversationModeInteractive:
		return a.patchInteractiveConversationConfig(binding, patch, baseRevision)
	case ConversationModeAutomation:
		return a.patchAutomationConversationConfig(ctx, binding, patch, baseRevision)
	default:
		return conversationconfig.Snapshot{}, fmt.Errorf("unsupported conversation mode %q", binding.Mode)
	}
}

func (a *App) writingConversationConfig(binding ConversationConfigBinding) (conversationconfig.Snapshot, error) {
	service := a.chat()
	service.admission.Lock()
	defer service.admission.Unlock()
	store, runtimeCfg, sessionID, err := a.foregroundConversationRuntime(binding.SessionID)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return ensureExistingSessionConfig(sess, &runtimeCfg, config.AgentKindIDE)
}

func (a *App) patchWritingConversationConfig(binding ConversationConfigBinding, patch conversationconfig.Patch, baseRevision uint64) (conversationconfig.Snapshot, error) {
	service := a.chat()
	service.admission.Lock()
	defer service.admission.Unlock()
	store, runtimeCfg, sessionID, err := a.foregroundConversationRuntime(binding.SessionID)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	a.mu.RLock()
	active := writingTaskForSessionLocked(a, a.workspace, sessionID)
	a.mu.RUnlock()
	if active != nil && !active.Finished() {
		return conversationconfig.Snapshot{}, ErrAgentOperationActive
	}
	sess, err := store.Get(sessionID)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	current, err := ensureExistingSessionConfig(sess, &runtimeCfg, config.AgentKindIDE)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	next, err := conversationconfig.Merge(&runtimeCfg, current.Config, patch)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return sess.SetRuntimeConfig(next, baseRevision)
}

func (a *App) agentChatConversationConfig(ctx context.Context, binding ConversationConfigBinding) (conversationconfig.Snapshot, error) {
	service := a.agentChat()
	service.admission.Lock()
	defer service.admission.Unlock()
	resolved, project, runtimeCfg, err := service.conversationRuntime(ctx, binding)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return previewConversationSessionConfig(project.store, resolved.SessionID, &runtimeCfg, resolved.agentKind)
}

func (a *App) patchAgentChatConversationConfig(ctx context.Context, binding ConversationConfigBinding, patch conversationconfig.Patch, baseRevision uint64) (conversationconfig.Snapshot, error) {
	service := a.agentChat()
	service.admission.Lock()
	defer service.admission.Unlock()
	resolved, project, runtimeCfg, err := service.conversationRuntime(ctx, binding)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	if run := service.activeRun(resolved); run != nil && run.task != nil && !run.task.Finished() {
		return conversationconfig.Snapshot{}, ErrAgentOperationActive
	}
	if !project.store.Exists(resolved.SessionID) {
		current, previewErr := previewConversationSessionConfig(project.store, resolved.SessionID, &runtimeCfg, resolved.agentKind)
		if previewErr != nil {
			return conversationconfig.Snapshot{}, previewErr
		}
		if baseRevision != 0 {
			return conversationconfig.Snapshot{}, fmt.Errorf("%w: conversation is not initialized", conversationconfig.ErrRevisionConflict)
		}
		next, mergeErr := conversationconfig.Merge(&runtimeCfg, current.Config, patch)
		if mergeErr != nil {
			return conversationconfig.Snapshot{}, mergeErr
		}
		sess, createErr := project.store.GetOrCreateWithRuntimeConfig(resolved.SessionID, next)
		if createErr != nil {
			return conversationconfig.Snapshot{}, createErr
		}
		created, ok := sess.RuntimeConfig()
		if !ok || created.Config != next || created.Revision != 1 {
			return conversationconfig.Snapshot{}, fmt.Errorf("%w: conversation was initialized concurrently", conversationconfig.ErrRevisionConflict)
		}
		return created, nil
	}
	sess, current, err := getOrCreateConversationSession(project.store, resolved.SessionID, &runtimeCfg, resolved.agentKind)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	next, err := conversationconfig.Merge(&runtimeCfg, current.Config, patch)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return sess.SetRuntimeConfig(next, baseRevision)
}

func (s *AgentChatAppService) conversationRuntime(ctx context.Context, binding ConversationConfigBinding) (AgentChatBinding, *agentChatProjectRuntime, config.Config, error) {
	resolved, err := s.resolveBinding(AgentChatBinding{ProjectID: binding.ProjectID, SessionID: binding.SessionID})
	if err != nil {
		return AgentChatBinding{}, nil, config.Config{}, err
	}
	project, err := s.projectRuntime(ctx, resolved.ProjectID)
	if err != nil {
		return AgentChatBinding{}, nil, config.Config{}, err
	}
	runtimeCfg, err := refreshConversationRuntimeConfig(project.cfg, project.workspace, project.stateRoot)
	if err != nil {
		return AgentChatBinding{}, nil, config.Config{}, err
	}
	return resolved, project, runtimeCfg, nil
}

func (a *App) configManagerConversationConfig(binding ConversationConfigBinding) (conversationconfig.Snapshot, error) {
	service := a.configManager()
	service.admission.Lock()
	defer service.admission.Unlock()
	store, runtimeCfg, sessionID, _, err := a.configManagerConversationRuntime(binding)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	_, snapshot, err := getOrCreateConversationSession(store, sessionID, &runtimeCfg, config.AgentKindConfigManager)
	return snapshot, err
}

func (a *App) patchConfigManagerConversationConfig(binding ConversationConfigBinding, patch conversationconfig.Patch, baseRevision uint64) (conversationconfig.Snapshot, error) {
	service := a.configManager()
	service.admission.Lock()
	defer service.admission.Unlock()
	store, runtimeCfg, sessionID, workspace, err := a.configManagerConversationRuntime(binding)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	if active := service.starts.latestConfigManagerTask(workspace, sessionID).Task; active != nil && !active.Finished() {
		return conversationconfig.Snapshot{}, ErrAgentOperationActive
	}
	if recovery := service.recoveries.current(workspace, sessionID); recovery != nil && recovery.task != nil && !recovery.task.Finished() {
		return conversationconfig.Snapshot{}, ErrAgentOperationActive
	}
	sess, current, err := getOrCreateConversationSession(store, sessionID, &runtimeCfg, config.AgentKindConfigManager)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	next, err := conversationconfig.Merge(&runtimeCfg, current.Config, patch)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return sess.SetRuntimeConfig(next, baseRevision)
}

func (a *App) configManagerConversationRuntime(binding ConversationConfigBinding) (*session.Store, config.Config, string, string, error) {
	request := ConfigManagerRequest{Origin: binding.Origin, ResourceID: binding.ResourceID, StoryID: binding.StoryID, BranchID: binding.BranchID}
	sessionID, err := configManagerSessionID(request)
	if err != nil {
		return nil, config.Config{}, "", "", err
	}
	a.mu.RLock()
	store := a.sessionStore
	workspace := strings.TrimSpace(a.workspace)
	runtimeCfg := config.Config{}
	if a.cfg != nil {
		runtimeCfg = *a.cfg
	}
	a.mu.RUnlock()
	if store == nil || workspace == "" {
		return nil, config.Config{}, "", "", ErrNoWorkspace
	}
	runtimeCfg, err = refreshConversationRuntimeConfig(runtimeCfg, workspace, runtimeCfg.ProjectStateDir)
	return store, runtimeCfg, sessionID, workspace, err
}

func (a *App) automationConversationConfig(ctx context.Context, binding ConversationConfigBinding) (conversationconfig.Snapshot, error) {
	snap, operation, run, task, err := a.automationConversationRuntime(ctx, binding)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	defer operation.Release()
	runtimeCfg := runtimeConfigForTask(snap, task)
	_, snapshot, err := getOrCreateConversationSession(snap.sessionStore, run.SessionID, &runtimeCfg, config.AgentKindAutomation)
	return snapshot, err
}

func (a *App) patchAutomationConversationConfig(ctx context.Context, binding ConversationConfigBinding, patch conversationconfig.Patch, baseRevision uint64) (conversationconfig.Snapshot, error) {
	service := a.automation()
	// ContinueRun owns the same admission lock. The active-run check and CAS
	// update therefore form one boundary with follow-up Agent admission.
	service.followUpAdmission.Lock()
	defer service.followUpAdmission.Unlock()
	snap, operation, run, task, err := a.automationConversationRuntime(ctx, binding)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	defer operation.Release()
	if active, _, ok := service.activeAutomationTaskByRunID(snap, run.ID); ok && active != nil && !active.Finished() {
		return conversationconfig.Snapshot{}, ErrAgentOperationActive
	}
	runtimeCfg := runtimeConfigForTask(snap, task)
	sess, current, err := getOrCreateConversationSession(snap.sessionStore, run.SessionID, &runtimeCfg, config.AgentKindAutomation)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	next, err := conversationconfig.Merge(&runtimeCfg, current.Config, patch)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return sess.SetRuntimeConfig(next, baseRevision)
}

// automationConversationRuntime resolves the durable run first, then admits
// its exact user/workspace lifecycle. Callers must release the operation.
func (a *App) automationConversationRuntime(ctx context.Context, binding ConversationConfigBinding) (*automationWorkspaceSnapshot, *appOperation, automation.RunRecord, automation.Task, error) {
	service := a.automation()
	runID := strings.TrimSpace(binding.RunID)
	if runID == "" {
		return nil, nil, automation.RunRecord{}, automation.Task{}, errors.New("automation run is required")
	}
	_, activeRun, active := service.ActiveAutomationTaskByRunID(runID)
	run := activeRun
	var err error
	if !active {
		run, err = service.automationRunByID(nil, runID)
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
		target = automation.ExecutionTarget{Kind: automation.TargetKindWorkspace, WorkspaceID: run.ProjectID, Workspace: run.Workspace}
	}
	snap, operation, err := service.acquireTargetRuntime(ctx, target)
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

func (a *App) interactiveConversationConfig(binding ConversationConfigBinding) (conversationconfig.Snapshot, error) {
	service := a.interactiveService()
	service.admission.Lock()
	defer service.admission.Unlock()
	store, runtimeCfg, err := a.interactiveConversationRuntime(binding)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return applyInteractiveConversationConfig(store, &runtimeCfg, binding.StoryID, binding.BranchID)
}

func (a *App) patchInteractiveConversationConfig(binding ConversationConfigBinding, patch conversationconfig.Patch, baseRevision uint64) (conversationconfig.Snapshot, error) {
	service := a.interactiveService()
	service.admission.Lock()
	defer service.admission.Unlock()
	store, runtimeCfg, err := a.interactiveConversationRuntime(binding)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	if active, _ := service.ActiveInteractiveTaskFor(binding.StoryID, binding.BranchID); active != nil && !active.Finished() {
		return conversationconfig.Snapshot{}, ErrAgentOperationActive
	}
	current, ok, err := store.BranchRuntimeConfig(binding.StoryID, binding.BranchID)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	if !ok {
		seed := conversationconfig.Default(&runtimeCfg, config.AgentKindInteractiveStory)
		current, err = store.EnsureBranchRuntimeConfig(binding.StoryID, binding.BranchID, seed)
		if err != nil {
			return conversationconfig.Snapshot{}, err
		}
	}
	next, err := conversationconfig.Merge(&runtimeCfg, current.Config, patch)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return store.SetBranchRuntimeConfig(binding.StoryID, binding.BranchID, next, baseRevision)
}

func (a *App) foregroundConversationRuntime(requestedSessionID string) (*session.Store, config.Config, string, error) {
	a.mu.RLock()
	store := a.sessionStore
	runtimeCfg := config.Config{}
	if a.cfg != nil {
		runtimeCfg = *a.cfg
	}
	sessionID := strings.TrimSpace(requestedSessionID)
	if sessionID == "" && a.session != nil {
		sessionID = a.session.ID
	}
	workspace := strings.TrimSpace(a.workspace)
	a.mu.RUnlock()
	if store == nil || workspace == "" || sessionID == "" || isAgentSessionID(sessionID) {
		return nil, config.Config{}, "", ErrNoWorkspace
	}
	fresh, err := refreshConversationRuntimeConfig(runtimeCfg, workspace, runtimeCfg.ProjectStateDir)
	return store, fresh, sessionID, err
}

func (a *App) interactiveConversationRuntime(binding ConversationConfigBinding) (*interactive.Store, config.Config, error) {
	if strings.TrimSpace(binding.StoryID) == "" {
		return nil, config.Config{}, errors.New("interactive story is required")
	}
	a.mu.RLock()
	store := a.interactive
	workspace := strings.TrimSpace(a.workspace)
	runtimeCfg := config.Config{}
	if a.cfg != nil {
		runtimeCfg = *a.cfg
	}
	a.mu.RUnlock()
	if store == nil || workspace == "" {
		return nil, config.Config{}, ErrNoWorkspace
	}
	fresh, err := refreshConversationRuntimeConfig(runtimeCfg, workspace, runtimeCfg.ProjectStateDir)
	return store, fresh, err
}

func refreshConversationRuntimeConfig(runtimeCfg config.Config, workspace, stateRoot string) (config.Config, error) {
	layered, err := config.LoadLayeredWithStartupConfigAt(runtimeCfg.DataDir(), workspace, config.ProjectConfigPath(stateRoot))
	if err != nil {
		return config.Config{}, err
	}
	applyLayeredSettingsToConfig(&runtimeCfg, layered)
	runtimeCfg.Workspace = workspace
	runtimeCfg.ProjectStateDir = stateRoot
	return runtimeCfg, nil
}

func normalizeConversationMode(mode string) string {
	return strings.ToLower(strings.TrimSpace(mode))
}
