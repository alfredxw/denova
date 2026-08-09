package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"denova/config"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/goal"
	"denova/internal/agents/session"
	agentchatapp "denova/internal/app/agentchat"
	configmanagerapp "denova/internal/app/configmanager"
	interactiveapp "denova/internal/app/interactive"
	appsettings "denova/internal/app/settings"
	"denova/internal/interactive"
)

const (
	ConversationModeWriting       = "writing"
	ConversationModeAgentChat     = "agent_chat"
	ConversationModeInteractive   = "interactive"
	ConversationModeConfigManager = "config_manager"
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

// ConversationConfigPatch is the application-facing mutation contract. The
// transport layer depends on this type instead of reaching through app into
// the Agent implementation package.
type ConversationConfigPatch struct {
	ProfileID     *string                   `json:"profile_id,omitempty"`
	ThinkingLevel *string                   `json:"thinking_level,omitempty"`
	ApprovalMode  *config.AgentApprovalMode `json:"approval_mode,omitempty"`
}

type ConversationGoalMutation struct {
	Action           string `json:"action"`
	Objective        string `json:"objective,omitempty"`
	ExpectedRevision uint64 `json:"expected_revision,omitempty"`
}

func IsConversationGoalRevisionConflict(err error) bool {
	return errors.Is(err, goal.ErrRevisionConflict) || agentchatapp.IsGoalRevisionConflict(err)
}

func IsConversationGoalStateChanged(err error) bool {
	return errors.Is(err, goal.ErrNotFound) || errors.Is(err, goal.ErrNotActive)
}

func (a *App) ConversationGoal(ctx context.Context, binding ConversationConfigBinding) (goal.State, bool, error) {
	switch normalizeConversationMode(binding.Mode) {
	case ConversationModeWriting:
		if err := a.requireForegroundConversationProject(binding.ProjectID); err != nil {
			return goal.State{}, false, err
		}
		service := a.chat()
		service.admission.Lock()
		defer service.admission.Unlock()
		sess, _, err := a.writingGoalSession(binding.SessionID)
		if err != nil {
			return goal.State{}, false, err
		}
		return sess.Goal(ctx)
	case ConversationModeAgentChat:
		return a.AgentChat().ConversationGoal(ctx, agentchatapp.Binding{ProjectID: binding.ProjectID, SessionID: binding.SessionID})
	default:
		return goal.State{}, false, fmt.Errorf("goal is unsupported for conversation mode %q", binding.Mode)
	}
}

func (a *App) MutateConversationGoal(ctx context.Context, binding ConversationConfigBinding, mutation ConversationGoalMutation) (goal.State, error) {
	action := strings.TrimSpace(mutation.Action)
	switch normalizeConversationMode(binding.Mode) {
	case ConversationModeWriting:
		if err := a.requireForegroundConversationProject(binding.ProjectID); err != nil {
			return goal.State{}, err
		}
		service := a.chat()
		service.admission.Lock()
		defer service.admission.Unlock()
		sess, runtimeCfg, err := a.writingGoalSession(binding.SessionID)
		if err != nil {
			return goal.State{}, err
		}
		if (action == "set" || action == "resume") && !config.ResolveAgentTools(&runtimeCfg, config.AgentKindIDE).Allows(config.AgentToolGoal) {
			return goal.State{}, errors.New("conversation goal is disabled for the Writing Agent")
		}
		switch action {
		case "set":
			return sess.SetGoal(ctx, mutation.Objective, mutation.ExpectedRevision)
		case "pause":
			return sess.PauseGoal(ctx, mutation.ExpectedRevision)
		case "resume":
			return sess.ResumeGoal(ctx, mutation.ExpectedRevision)
		case "clear":
			return sess.ClearGoal(ctx, mutation.ExpectedRevision)
		default:
			return goal.State{}, fmt.Errorf("unsupported goal action %q", action)
		}
	case ConversationModeAgentChat:
		return a.AgentChat().MutateConversationGoal(ctx, agentchatapp.Binding{ProjectID: binding.ProjectID, SessionID: binding.SessionID}, action, mutation.Objective, mutation.ExpectedRevision)
	default:
		return goal.State{}, fmt.Errorf("goal is unsupported for conversation mode %q", binding.Mode)
	}
}

func (a *App) writingGoalSession(requestedSessionID string) (*session.Session, config.Config, error) {
	store, runtimeCfg, sessionID, err := a.foregroundConversationRuntime(requestedSessionID)
	if err != nil {
		return nil, config.Config{}, err
	}
	sess, err := store.Get(sessionID)
	return sess, runtimeCfg, err
}

// UnmarshalJSON delegates the strict omitted-versus-null validation to the
// conversation domain and then projects the validated transport value.
func (patch *ConversationConfigPatch) UnmarshalJSON(data []byte) error {
	if patch == nil {
		return errors.New("conversation config patch is nil")
	}
	var parsed conversationconfig.Patch
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*patch = ConversationConfigPatch{
		ProfileID:     parsed.ProfileID,
		ThinkingLevel: parsed.ThinkingLevel,
		ApprovalMode:  parsed.ApprovalMode,
	}
	return nil
}

// IsConversationConfigRevisionConflict keeps transport error classification
// on the application boundary rather than exposing the Agent package.
func IsConversationConfigRevisionConflict(err error) bool {
	return errors.Is(err, conversationconfig.ErrRevisionConflict)
}

func (a *App) ConversationConfig(ctx context.Context, binding ConversationConfigBinding) (conversationconfig.Snapshot, error) {
	if a == nil {
		return conversationconfig.Snapshot{}, ErrNoWorkspace
	}
	switch normalizeConversationMode(binding.Mode) {
	case ConversationModeWriting:
		if err := a.requireForegroundConversationProject(binding.ProjectID); err != nil {
			return conversationconfig.Snapshot{}, err
		}
		return a.writingConversationConfig(binding)
	case ConversationModeAgentChat:
		return a.AgentChat().ConversationConfig(ctx, agentchatapp.Binding{
			ProjectID: binding.ProjectID, SessionID: binding.SessionID,
		})
	case ConversationModeConfigManager:
		return a.ConfigManager().ConversationConfig(configManagerConfigRequest(binding))
	case ConversationModeInteractive:
		if err := a.requireForegroundConversationProject(binding.ProjectID); err != nil {
			return conversationconfig.Snapshot{}, err
		}
		return a.interactiveConversationConfig(binding)
	default:
		return conversationconfig.Snapshot{}, fmt.Errorf("unsupported conversation mode %q", binding.Mode)
	}
}

func (a *App) PatchConversationConfig(ctx context.Context, binding ConversationConfigBinding, patch ConversationConfigPatch, baseRevision uint64) (conversationconfig.Snapshot, error) {
	if patch.ProfileID == nil && patch.ThinkingLevel == nil && patch.ApprovalMode == nil {
		return conversationconfig.Snapshot{}, errors.New("conversation config changes are empty")
	}
	change := conversationconfig.Patch{
		ProfileID:     patch.ProfileID,
		ThinkingLevel: patch.ThinkingLevel,
		ApprovalMode:  patch.ApprovalMode,
	}
	switch normalizeConversationMode(binding.Mode) {
	case ConversationModeWriting:
		if err := a.requireForegroundConversationProject(binding.ProjectID); err != nil {
			return conversationconfig.Snapshot{}, err
		}
		return a.patchWritingConversationConfig(binding, change, baseRevision)
	case ConversationModeAgentChat:
		return a.AgentChat().PatchConversationConfig(ctx, agentchatapp.Binding{
			ProjectID: binding.ProjectID, SessionID: binding.SessionID,
		}, change, baseRevision)
	case ConversationModeConfigManager:
		return a.ConfigManager().PatchConversationConfig(configManagerConfigRequest(binding), change, baseRevision)
	case ConversationModeInteractive:
		if err := a.requireForegroundConversationProject(binding.ProjectID); err != nil {
			return conversationconfig.Snapshot{}, err
		}
		return a.patchInteractiveConversationConfig(binding, change, baseRevision)
	default:
		return conversationconfig.Snapshot{}, fmt.Errorf("unsupported conversation mode %q", binding.Mode)
	}
}

func (a *App) requireForegroundConversationProject(projectID string) error {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return errors.New("Project ID is required for a foreground conversation")
	}
	a.mu.RLock()
	foregroundProjectID := ""
	if a.cfg != nil {
		foregroundProjectID = strings.TrimSpace(a.cfg.ProjectID)
	}
	a.mu.RUnlock()
	if foregroundProjectID == "" || foregroundProjectID != projectID {
		return fmt.Errorf("foreground conversation Project mismatch: requested=%s current=%s", projectID, foregroundProjectID)
	}
	return nil
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
	return agentconversation.EnsureSession(sess, &runtimeCfg, config.AgentKindIDE)
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
	current, err := agentconversation.EnsureSession(sess, &runtimeCfg, config.AgentKindIDE)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	next, err := conversationconfig.Merge(&runtimeCfg, current.Config, patch)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return sess.SetRuntimeConfig(next, baseRevision)
}

func configManagerConfigRequest(binding ConversationConfigBinding) configmanagerapp.Request {
	return configmanagerapp.Request{
		ProjectID: binding.ProjectID, Origin: binding.Origin, ResourceID: binding.ResourceID,
		StoryID: binding.StoryID, BranchID: binding.BranchID,
	}
}

func (a *App) interactiveConversationConfig(binding ConversationConfigBinding) (conversationconfig.Snapshot, error) {
	service := a.interactiveService()
	service.admission.Lock()
	defer service.admission.Unlock()
	store, runtimeCfg, err := a.interactiveConversationRuntime(binding)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return interactiveapp.ApplyConversationConfig(store, &runtimeCfg, binding.StoryID, binding.BranchID)
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
	return appsettings.RefreshProject(runtimeCfg, workspace, stateRoot)
}

func normalizeConversationMode(mode string) string {
	return strings.ToLower(strings.TrimSpace(mode))
}
