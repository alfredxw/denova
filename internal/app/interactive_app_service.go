package app

import (
	"context"
	agentattachment "denova/internal/agents/attachment"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	interactiveapp "denova/internal/app/interactive"
	appsettings "denova/internal/app/settings"
	"denova/internal/interactive"
)

// InteractiveAppService owns stories, branches, Game Planning, and Game Agent tasks.
type InteractiveAppService struct {
	app       *App
	admission sync.RWMutex
	starts    interactiveStartRegistry
}

// InteractiveTurnPersistedEvent is emitted after a game-mode turn is durably
// appended, allowing the UI to merge the new turn without a blocking snapshot
// reload.
type InteractiveTurnPersistedEvent struct {
	StoryID           string                                   `json:"story_id"`
	BranchID          string                                   `json:"branch_id"`
	TurnCount         int                                      `json:"turn_count"`
	Turn              interactive.TurnEvent                    `json:"turn"`
	BranchPlan        *interactive.BranchPlan                  `json:"branch_plan,omitempty"`
	State             map[string]any                           `json:"state"`
	Graph             interactive.StoryGraph                   `json:"graph"`
	Branches          []interactive.BranchSummary              `json:"branches"`
	ContextCompaction *interactive.ContextCompactionProjection `json:"context_compaction"`
}

func (a *App) InteractiveStories() (interactive.Index, error) {
	return a.interactiveService().InteractiveStories()
}

func (s *InteractiveAppService) InteractiveStories() (interactive.Index, error) {
	store := s.store()
	if store == nil {
		return interactive.Index{}, ErrNoWorkspace
	}
	return store.Index()
}

func (a *App) SelectInteractiveStory(storyID string) error {
	return a.interactiveService().SelectInteractiveStory(storyID)
}

func (s *InteractiveAppService) SelectInteractiveStory(storyID string) error {
	store := s.store()
	if store == nil {
		return ErrNoWorkspace
	}
	slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-story] persist current story selection story_id=%s", storyID))
	if err := store.SelectStory(storyID); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-story] persist current story selection failed story_id=%s err=%v", storyID, err))
		return err
	}
	return nil
}

func (a *App) CreateInteractiveStory(req interactive.CreateStoryRequest) (interactive.StorySummary, error) {
	return a.interactiveService().CreateInteractiveStoryContext(context.Background(), req)
}

func (s *InteractiveAppService) CreateInteractiveStory(req interactive.CreateStoryRequest) (interactive.StorySummary, error) {
	return s.CreateInteractiveStoryContext(context.Background(), req)
}

func (a *App) CreateInteractiveStoryContext(ctx context.Context, req interactive.CreateStoryRequest) (interactive.StorySummary, error) {
	return a.interactiveService().CreateInteractiveStoryContext(ctx, req)
}

func (s *InteractiveAppService) CreateInteractiveStoryContext(ctx context.Context, req interactive.CreateStoryRequest) (interactive.StorySummary, error) {
	store := s.store()
	if store == nil {
		return interactive.StorySummary{}, ErrNoWorkspace
	}
	var err error
	req.Protagonist, err = s.resolveStoryProtagonist(ctx, req.Protagonist)
	if err != nil {
		return interactive.StorySummary{}, err
	}
	req, err = s.withStoryCreationDefaults(req)
	if err != nil {
		return interactive.StorySummary{}, err
	}
	req.StoryTellerID = s.gameTellerID(req.StoryTellerID)
	if req.RuntimeConfig == nil {
		a := s.app
		a.mu.RLock()
		runtimeCfg := config.Config{}
		workspace := strings.TrimSpace(a.workspace)
		if a.cfg != nil {
			runtimeCfg = *a.cfg
		}
		a.mu.RUnlock()
		if workspace == "" {
			return interactive.StorySummary{}, ErrNoWorkspace
		}
		runtimeCfg, err = refreshConversationRuntimeConfig(runtimeCfg, workspace, runtimeCfg.ProjectStoreDir)
		if err != nil {
			return interactive.StorySummary{}, err
		}
		var seed conversationconfig.Config
		var seedErr error
		if req.CustomAgentID == nil {
			seed, seedErr = interactiveapp.RecentConversationSeed(store, &runtimeCfg, "")
		} else {
			seed, seedErr = conversationconfig.DefaultWithCustomAgent(&runtimeCfg, config.AgentKindInteractiveStory, *req.CustomAgentID)
		}
		if seedErr != nil {
			return interactive.StorySummary{}, seedErr
		}
		profileID := strings.TrimSpace(req.ProfileID)
		thinkingLevel := strings.TrimSpace(req.ThinkingLevel)
		if profileID != "" || thinkingLevel != "" {
			patch := conversationconfig.Patch{}
			if profileID != "" {
				patch.ProfileID = &profileID
			}
			if thinkingLevel != "" {
				patch.ThinkingLevel = &thinkingLevel
			}
			seed, seedErr = conversationconfig.Merge(&runtimeCfg, seed, patch)
			if seedErr != nil {
				return interactive.StorySummary{}, seedErr
			}
		}
		req.RuntimeConfig = &seed
	}
	story, err := store.CreateStory(req)
	if err != nil {
		return interactive.StorySummary{}, err
	}
	return story, nil
}

func (a *App) RollInteractiveActorTraits(req interactive.ActorTraitRollRequest) (interactive.ActorTraitRollResult, error) {
	return a.interactiveService().RollInteractiveActorTraits(req)
}

func (s *InteractiveAppService) RollInteractiveActorTraits(req interactive.ActorTraitRollRequest) (interactive.ActorTraitRollResult, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.ActorTraitRollResult{}, ErrNoWorkspace
	}
	actorStateID := strings.TrimSpace(req.ActorStateID)
	if actorStateID == "" {
		actorStateID = interactive.DefaultActorStateModuleID
	}
	actorState, err := interactive.NewActorStateLibrary(cfg.DataDir()).Get(actorStateID)
	if err != nil {
		return interactive.ActorTraitRollResult{}, err
	}
	req.ActorStateID = actorStateID
	return interactive.RollActorTraits(actorState.ActorState, req)
}

func (s *InteractiveAppService) withStoryCreationDefaults(req interactive.CreateStoryRequest) (interactive.CreateStoryRequest, error) {
	if strings.TrimSpace(req.PlanningMode) == "" {
		req.PlanningMode = interactive.StoryPlanningModeEnabled
	}
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return req, nil
	}
	templateID := interactive.NormalizeGamePlanningTemplateID(req.PlanningTemplateID)
	if templateID == "" {
		templateID = interactive.DefaultGamePlanningTemplateID
	}
	req.PlanningTemplateID = templateID
	if _, err := interactive.NewGamePlanningTemplateLibrary(cfg.DataDir()).Get(templateID); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[game-planning] load template failed planning_template_id=%s err=%v", templateID, err))
		req.PlanningTemplateID = interactive.DefaultGamePlanningTemplateID
	}
	refs := interactive.DefaultStoryDirectorModuleRefs()
	if req.ModuleRefs != nil {
		refs = interactive.NormalizeStoryDirectorModuleRefs(*req.ModuleRefs)
	}
	runtime := interactive.DefaultStoryDirector()
	runtime.ModuleRefs = refs
	runtime.ResolvedSnapshot = interactive.StoryDirectorResolvedSnapshot{}
	runtime = interactive.ResolveStoryDirectorModules(cfg.DataDir(), runtime)
	normalized := interactive.NormalizeStoryDirectorModuleRefs(runtime.ModuleRefs)
	req.ModuleRefs = &normalized
	if interactive.StoryDirectorNarrativeStyleEnabled(runtime) && strings.TrimSpace(req.StoryTellerID) == "" && strings.TrimSpace(runtime.ModuleRefs.NarrativeStyleID) != "" {
		req.StoryTellerID = strings.TrimSpace(runtime.ModuleRefs.NarrativeStyleID)
	}
	if interactive.StoryDirectorImagePresetEnabled(runtime) && strings.TrimSpace(req.ImageSettings.PresetID) == "" && strings.TrimSpace(runtime.ModuleRefs.ImagePresetID) != "" {
		req.ImageSettings.PresetID = strings.TrimSpace(runtime.ModuleRefs.ImagePresetID)
	}
	policy := interactive.StoryStateSchemaPolicy{Mode: interactive.StoryStateSchemaModeAdaptTemplate}
	if req.StateSchemaPolicy != nil {
		policy = interactive.NormalizeStoryStateSchemaPolicy(*req.StateSchemaPolicy)
	}
	req.StateSchemaPolicy = &policy
	actorState := runtime.ActorState
	if policy.Mode == interactive.StoryStateSchemaModeGenerate {
		actorState = interactive.GeneratedStoryActorStateCore()
	}
	if len(actorState.Templates) == 0 && interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(&policy) {
		return req, fmt.Errorf("故事状态模板不可用 / Story state template is unavailable")
	}
	if len(actorState.Templates) > 0 {
		req.ActorState = &actorState
	} else {
		req.ActorState = nil
	}
	req.TRPGSystem = &runtime.TRPGSystem
	status := interactive.StateSchemaInitializationWaitingOpening
	outcome := ""
	if policy.Mode == interactive.StoryStateSchemaModeFixedTemplate {
		status = interactive.StateSchemaInitializationReady
		outcome = "fixed"
	}
	req.StateSchemaInitialization = &interactive.StateSchemaInitializationStatus{
		Mode:         policy.Mode,
		Status:       status,
		Outcome:      outcome,
		BaseRevision: 1,
	}
	if status == interactive.StateSchemaInitializationReady {
		req.StateSchemaInitialization.TargetRevision = 1
	}
	return req, nil
}

func (a *App) UpdateInteractiveStory(storyID string, req interactive.UpdateStoryRequest) (interactive.StorySummary, error) {
	return a.interactiveService().UpdateInteractiveStory(storyID, req)
}

func (s *InteractiveAppService) UpdateInteractiveStory(storyID string, req interactive.UpdateStoryRequest) (interactive.StorySummary, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return interactive.StorySummary{}, ErrNoWorkspace
	}
	if req.Protagonist != nil {
		protagonist, err := s.resolveStoryProtagonist(context.Background(), *req.Protagonist)
		if err != nil {
			return interactive.StorySummary{}, err
		}
		req.Protagonist = &protagonist
	}
	if req.StateSchemaPolicy != nil {
		var err error
		req, err = s.withStoryStateSchemaUpdateDefaults(req)
		if err != nil {
			return interactive.StorySummary{}, err
		}
	}
	if strings.TrimSpace(req.StoryTellerID) != "" {
		req.StoryTellerID = s.gameTellerID(req.StoryTellerID)
	}
	fence, err := s.drainInteractiveBinding(context.Background(), storyID, "")
	if err != nil {
		return interactive.StorySummary{}, err
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := fence.validateLocked(a); err != nil {
		return interactive.StorySummary{}, err
	}
	return store.UpdateStory(storyID, req)
}

func (s *InteractiveAppService) gameTellerID(tellerID string) string {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return strings.TrimSpace(tellerID)
	}
	if teller := interactiveapp.LoadGameTeller(cfg.DataDir(), strings.TrimSpace(tellerID)); teller.ID != "" {
		return teller.ID
	}
	return strings.TrimSpace(tellerID)
}

func (s *InteractiveAppService) withStoryStateSchemaUpdateDefaults(req interactive.UpdateStoryRequest) (interactive.UpdateStoryRequest, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" || req.StateSchemaPolicy == nil {
		return req, nil
	}
	refs := interactive.DefaultStoryDirectorModuleRefs()
	if req.ModuleRefs != nil {
		refs = interactive.NormalizeStoryDirectorModuleRefs(*req.ModuleRefs)
	}
	runtime := interactive.DefaultStoryDirector()
	runtime.ModuleRefs = refs
	runtime.ResolvedSnapshot = interactive.StoryDirectorResolvedSnapshot{}
	runtime = interactive.ResolveStoryDirectorModules(cfg.DataDir(), runtime)
	normalized := interactive.NormalizeStoryDirectorModuleRefs(runtime.ModuleRefs)
	req.ModuleRefs = &normalized
	policy := interactive.NormalizeStoryStateSchemaPolicy(*req.StateSchemaPolicy)
	actorState := runtime.ActorState
	if policy.Mode == interactive.StoryStateSchemaModeGenerate {
		actorState = interactive.GeneratedStoryActorStateCore()
	}
	if len(actorState.Templates) == 0 && interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(&policy) {
		return req, fmt.Errorf("故事状态模板不可用 / Story state template is unavailable")
	}
	req.StateSchemaPolicy = &policy
	if len(actorState.Templates) > 0 {
		req.ActorState = &actorState
	} else {
		req.ActorState = nil
	}
	req.TRPGSystem = &runtime.TRPGSystem
	status := interactive.StateSchemaInitializationWaitingOpening
	outcome := ""
	if policy.Mode == interactive.StoryStateSchemaModeFixedTemplate {
		status = interactive.StateSchemaInitializationReady
		outcome = "fixed"
	}
	req.StateSchemaInitialization = &interactive.StateSchemaInitializationStatus{Mode: policy.Mode, Status: status, Outcome: outcome, BaseRevision: 1}
	if status == interactive.StateSchemaInitializationReady {
		req.StateSchemaInitialization.TargetRevision = 1
	}
	return req, nil
}

func (a *App) DeleteInteractiveStory(storyID string) error {
	return a.interactiveService().DeleteInteractiveStory(storyID)
}

func (s *InteractiveAppService) DeleteInteractiveStory(storyID string) error {
	s.admission.Lock()
	defer s.admission.Unlock()
	fence, err := s.drainInteractiveBinding(context.Background(), storyID, "")
	if err != nil {
		return err
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	store := a.interactive
	sessionStore := a.sessionStore
	stateRoot := ""
	if a.cfg != nil {
		stateRoot = a.cfg.ProjectStoreDir
	}
	if store == nil {
		return ErrNoWorkspace
	}
	if err := fence.validateLocked(a); err != nil {
		return err
	}
	if fence.chat == nil {
		return ErrNoWorkspace
	}
	if err := fence.chat.DeleteStoryBindings(context.Background(), fence.projectID, storyID, ""); err != nil {
		return err
	}
	if err := store.DeleteStory(storyID); err != nil {
		return err
	}
	if err := agentattachment.RemoveScope(stateRoot, agentattachment.StoryScope(storyID)); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-story] remove deleted Story attachments failed story_id=%s error=%v", storyID, err))
	}
	if sessionStore != nil {
		return sessionStore.DeleteByPrefix("interactive-story-" + storyID + "-")
	}
	return nil
}

func (a *App) InteractiveSnapshot(storyID, branchID string) (interactive.Snapshot, error) {
	return a.interactiveService().InteractiveSnapshot(storyID, branchID)
}

func (s *InteractiveAppService) InteractiveSnapshot(storyID, branchID string) (interactive.Snapshot, error) {
	store := s.store()
	if store == nil {
		return interactive.Snapshot{}, ErrNoWorkspace
	}
	snapshot, err := store.Snapshot(storyID, branchID)
	if err != nil {
		return interactive.Snapshot{}, err
	}
	s.app.mu.RLock()
	workspace := s.app.workspace
	projectID := ""
	if s.app.cfg != nil {
		projectID = s.app.cfg.ProjectID
	}
	executionRuntime := s.app.executionRuntime
	s.app.mu.RUnlock()
	// Story Store owns every user-visible game fact. Agent Session contributes
	// optional runtime metadata, so an unavailable or unreadable Agent transcript
	// must never hide already committed turns and state from the user.
	snapshot.ContextCompaction = nil
	if executionRuntime == nil {
		redactInteractiveSnapshotAttachmentPaths(&snapshot)
		return snapshot, nil
	}
	status, projected := projectAgentRuntime(context.Background(), executionRuntime, agentrun.Options{
		AgentKind: agentrun.AgentKindInteractiveStory, ProjectID: projectID, Workspace: workspace,
		StoryID: storyID, BranchID: snapshot.BranchID, Mode: "interactive",
	})
	if !projected {
		redactInteractiveSnapshotAttachmentPaths(&snapshot)
		return snapshot, nil
	}
	snapshot.ContextCompaction, err = interactiveapp.ProjectAgentCompaction(
		status.Compaction, storyID, snapshot.BranchID,
	)
	if err != nil {
		slog.WarnContext(context.Background(), fmt.Sprintf(
			"[interactive-snapshot] ignore invalid optional Agent Compaction projection workspace=%s story_id=%s branch_id=%s err=%v",
			workspace, storyID, snapshot.BranchID, err,
		))
		snapshot.ContextCompaction = nil
	}
	redactInteractiveSnapshotAttachmentPaths(&snapshot)
	return snapshot, nil
}

func (a *App) InteractiveHistoryPage(storyID, branchID, beforeCursor string, limit int) (interactive.StoryHistoryPage, error) {
	return a.interactiveService().InteractiveHistoryPage(storyID, branchID, beforeCursor, limit)
}

func (s *InteractiveAppService) InteractiveHistoryPage(storyID, branchID, beforeCursor string, limit int) (interactive.StoryHistoryPage, error) {
	store := s.store()
	if store == nil {
		return interactive.StoryHistoryPage{}, ErrNoWorkspace
	}
	page, err := store.ReadHistoryPage(storyID, branchID, beforeCursor, limit)
	if err != nil {
		return interactive.StoryHistoryPage{}, err
	}
	for index := range page.Turns {
		page.Turns[index].Attachments = attachmentDescriptors(page.Turns[index].Attachments)
	}
	return page, nil
}

func (a *App) RerollInteractiveRuleResolution(storyID, resolutionID string, req interactive.RuleResolutionRerollRequest) (interactive.RuleResolution, error) {
	return a.interactiveService().RerollInteractiveRuleResolution(storyID, resolutionID, req)
}

func (s *InteractiveAppService) RerollInteractiveRuleResolution(storyID, resolutionID string, req interactive.RuleResolutionRerollRequest) (interactive.RuleResolution, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return interactive.RuleResolution{}, ErrNoWorkspace
	}
	storyCtx, err := store.StoryContext(storyID, req.BranchID)
	if err != nil {
		return interactive.RuleResolution{}, err
	}
	fence, err := s.drainInteractiveBinding(context.Background(), storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return interactive.RuleResolution{}, err
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := fence.validateLocked(a); err != nil {
		return interactive.RuleResolution{}, err
	}
	return store.RerollRuleResolution(storyID, resolutionID, req)
}

// ActiveInteractiveTask 返回当前游戏模式活跃任务（可能为 nil）。
func (a *App) ActiveInteractiveTask() *apptask.Task {
	return a.interactiveService().ActiveInteractiveTask()
}

func (s *InteractiveAppService) ActiveInteractiveTask() *apptask.Task {
	task, _ := s.ActiveInteractiveTaskFor("", "")
	return task
}

func (s *InteractiveAppService) store() *interactive.Store {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.interactive
}

func (s *InteractiveAppService) interactiveRuntimeConfig() (*interactive.Store, config.Config, string, error) {
	a := s.app
	a.mu.RLock()
	if a.interactive == nil || a.cfg == nil {
		a.mu.RUnlock()
		return nil, config.Config{}, "", ErrNoWorkspace
	}
	store := a.interactive
	runtimeCfg := *a.cfg
	workspace := a.workspace
	runtimeCfg.Workspace = workspace
	novaDir := runtimeCfg.DataDir()
	a.mu.RUnlock()

	if layered, err := config.LoadLayeredWithStartupConfigAt(
		novaDir, workspace, config.ProjectConfigPath(runtimeCfg.ProjectStoreDir),
	); err == nil {
		appsettings.ApplyLayered(&runtimeCfg, layered)
	} else {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] load layered settings failed workspace=%s err=%v", workspace, err))
	}
	return store, runtimeCfg, workspace, nil
}

func (s *InteractiveAppService) cfg() *config.Config {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}
