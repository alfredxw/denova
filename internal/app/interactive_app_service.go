package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"denova/config"
	"denova/internal/interactive"
)

// InteractiveAppService 负责互动故事、剧情分支、导演和互动 Agent 任务。
type InteractiveAppService struct {
	app       *App
	admission sync.RWMutex
	starts    interactiveStartRegistry
}

// InteractiveTurnPersistedEvent is emitted after a game-mode turn is durably
// appended, allowing the UI to merge the new turn without a blocking snapshot
// reload.
type InteractiveTurnPersistedEvent struct {
	StoryID                  string                                     `json:"story_id"`
	BranchID                 string                                     `json:"branch_id"`
	Turn                     interactive.TurnEvent                      `json:"turn"`
	DirectorPlanStatus       *interactive.DirectorPlanStatus            `json:"director_plan_status,omitempty"`
	State                    map[string]any                             `json:"state"`
	Graph                    interactive.StoryGraph                     `json:"graph"`
	Branches                 []interactive.BranchSummary                `json:"branches"`
	ContextCompaction        *interactive.ContextCompactionEvent        `json:"context_compaction,omitempty"`
	ContextCompactionRemoval *interactive.ContextCompactionRemovalEvent `json:"context_compaction_removal,omitempty"`
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
	log.Printf("[interactive-story] persist current story selection story_id=%s", storyID)
	if err := store.SelectStory(storyID); err != nil {
		log.Printf("[interactive-story] persist current story selection failed story_id=%s err=%v", storyID, err)
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
	req, err = s.withStoryDirectorDefaults(req)
	if err != nil {
		return interactive.StorySummary{}, err
	}
	req.StoryTellerID = s.gameTellerID(req.StoryTellerID)
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
	directorID := interactive.NormalizeStoryDirectorID(req.StoryDirectorID)
	if directorID == "" {
		directorID = interactive.DefaultStoryDirectorID
	}
	director, err := interactive.NewStoryDirectorLibrary(cfg.DataDir()).Get(directorID)
	if err != nil {
		return interactive.ActorTraitRollResult{}, err
	}
	req.StoryDirectorID = directorID
	return interactive.RollActorTraits(director.ActorState, req)
}

func (s *InteractiveAppService) withStoryDirectorDefaults(req interactive.CreateStoryRequest) (interactive.CreateStoryRequest, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return req, nil
	}
	directorID := interactive.NormalizeStoryDirectorID(req.StoryDirectorID)
	if directorID == "" {
		directorID = interactive.DefaultStoryDirectorID
	}
	req.StoryDirectorID = directorID
	director, err := interactive.NewStoryDirectorLibrary(cfg.DataDir()).Get(directorID)
	if err != nil {
		log.Printf("[interactive-director] load story director failed story_director_id=%s err=%v", directorID, err)
		return req, nil
	}
	if req.ModuleRefs != nil {
		director.ModuleRefs = interactive.NormalizeStoryDirectorModuleRefs(*req.ModuleRefs)
		director.ResolvedSnapshot = interactive.StoryDirectorResolvedSnapshot{}
		director = interactive.ResolveStoryDirectorModules(cfg.DataDir(), director)
		normalized := interactive.NormalizeStoryDirectorModuleRefs(director.ModuleRefs)
		req.ModuleRefs = &normalized
	}
	if interactive.StoryDirectorNarrativeStyleEnabled(director) && strings.TrimSpace(req.StoryTellerID) == "" && strings.TrimSpace(director.ModuleRefs.NarrativeStyleID) != "" {
		req.StoryTellerID = strings.TrimSpace(director.ModuleRefs.NarrativeStyleID)
	}
	if interactive.StoryDirectorImagePresetEnabled(director) && strings.TrimSpace(req.ImageSettings.PresetID) == "" && strings.TrimSpace(director.ModuleRefs.ImagePresetID) != "" {
		req.ImageSettings.PresetID = strings.TrimSpace(director.ModuleRefs.ImagePresetID)
	}
	directorRunPolicy := interactive.ResolveStoryDirectorRunPolicy(req.DirectorRunPolicy, director.Strategy)
	req.DirectorRunPolicy = &directorRunPolicy
	openingSummary := openingSummaryFromStateOps(req.InitialStateOps)
	req.DirectorPlanSeed = &interactive.DirectorPlanSeed{
		Templates:           director.Strategy.PlanningTemplates,
		BranchPlanningTurns: director.Strategy.BranchPlanningTurns,
		Source:              "story_create",
		OpeningSummary:      openingSummary,
		InitialStatus:       interactive.DirectorPlanStatusWaitingOpening,
		InitialSummary:      "等待玩家开局完成后由后台导演规划。",
	}
	decision := shouldRunInteractiveDirectorAgent(director.Strategy)
	if !decision.ShouldRun {
		req.DirectorPlanSeed.InitialStatus = interactive.DirectorPlanStatusSkipped
		req.DirectorPlanSeed.InitialSummary = "后台导演已关闭，跳过开局规划。"
		req.DirectorPlanSeed.StartReady = true
	} else if directorRunPolicy.Mode == interactive.DirectorRunModeManual {
		req.DirectorPlanSeed.InitialStatus = interactive.DirectorPlanStatusSkipped
		req.DirectorPlanSeed.InitialSummary = "后台导演设为仅手动运行。"
		req.DirectorPlanSeed.StartReady = true
	}
	policy := interactive.StoryStateSchemaPolicy{Mode: interactive.StoryStateSchemaModeAdaptTemplate}
	if req.StateSchemaPolicy != nil {
		policy = interactive.NormalizeStoryStateSchemaPolicy(*req.StateSchemaPolicy)
	}
	req.StateSchemaPolicy = &policy
	actorState := director.ActorState
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
	req.TRPGSystem = &director.TRPGSystem
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
	if req.DirectorPlanSeed.OpeningSummary == "" {
		req.DirectorPlanSeed.OpeningSummary = openingSummaryFromStateOps(req.InitialStateOps)
	}
	return req, nil
}

func openingSummaryFromStateOps(ops []interactive.StateOp) string {
	if len(ops) == 0 {
		return ""
	}
	data, err := json.Marshal(ops)
	if err != nil {
		return ""
	}
	return "开局状态操作：" + string(data)
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
	if teller := loadGameTeller(cfg.DataDir(), strings.TrimSpace(tellerID)); teller.ID != "" {
		return teller.ID
	}
	return strings.TrimSpace(tellerID)
}

func (s *InteractiveAppService) withStoryStateSchemaUpdateDefaults(req interactive.UpdateStoryRequest) (interactive.UpdateStoryRequest, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" || req.StateSchemaPolicy == nil {
		return req, nil
	}
	directorID := interactive.NormalizeStoryDirectorID(req.StoryDirectorID)
	if directorID == "" {
		directorID = interactive.DefaultStoryDirectorID
	}
	director, err := interactive.NewStoryDirectorLibrary(cfg.DataDir()).Get(directorID)
	if err != nil {
		return req, fmt.Errorf("读取故事导演失败 / Failed to load story director: %w", err)
	}
	if req.ModuleRefs != nil {
		director.ModuleRefs = interactive.NormalizeStoryDirectorModuleRefs(*req.ModuleRefs)
		director.ResolvedSnapshot = interactive.StoryDirectorResolvedSnapshot{}
		director = interactive.ResolveStoryDirectorModules(cfg.DataDir(), director)
		normalized := interactive.NormalizeStoryDirectorModuleRefs(director.ModuleRefs)
		req.ModuleRefs = &normalized
	}
	policy := interactive.NormalizeStoryStateSchemaPolicy(*req.StateSchemaPolicy)
	actorState := director.ActorState
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
	req.TRPGSystem = &director.TRPGSystem
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
	if store == nil {
		return ErrNoWorkspace
	}
	if err := fence.validateLocked(a); err != nil {
		return err
	}
	if err := store.DeleteStory(storyID); err != nil {
		return err
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
	return store.Snapshot(storyID, branchID)
}

func (a *App) InteractiveHistoryPage(storyID, branchID, beforeCursor string, limit int) (interactive.StoryHistoryPage, error) {
	return a.interactiveService().InteractiveHistoryPage(storyID, branchID, beforeCursor, limit)
}

func (s *InteractiveAppService) InteractiveHistoryPage(storyID, branchID, beforeCursor string, limit int) (interactive.StoryHistoryPage, error) {
	store := s.store()
	if store == nil {
		return interactive.StoryHistoryPage{}, ErrNoWorkspace
	}
	return store.ReadHistoryPage(storyID, branchID, beforeCursor, limit)
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

func (a *App) InteractiveDirectorPlan(storyID, branchID string) (interactive.DirectorPlan, error) {
	return a.interactiveService().InteractiveDirectorPlan(storyID, branchID)
}

func (s *InteractiveAppService) InteractiveDirectorPlan(storyID, branchID string) (interactive.DirectorPlan, error) {
	store := s.store()
	if store == nil {
		return interactive.DirectorPlan{}, ErrNoWorkspace
	}
	return store.DirectorPlan(storyID, branchID)
}

func (a *App) InteractiveDirectorPlanStatus(storyID, branchID string) (interactive.DirectorPlanStatus, error) {
	return a.interactiveService().InteractiveDirectorPlanStatus(storyID, branchID)
}

func (s *InteractiveAppService) InteractiveDirectorPlanStatus(storyID, branchID string) (interactive.DirectorPlanStatus, error) {
	store := s.store()
	if store == nil {
		return interactive.DirectorPlanStatus{}, ErrNoWorkspace
	}
	return store.DirectorPlanStatus(storyID, branchID)
}

func (a *App) UpdateInteractiveDirectorPlan(storyID string, req interactive.UpdateDirectorPlanRequest) (interactive.DirectorPlan, error) {
	return a.interactiveService().UpdateInteractiveDirectorPlan(storyID, req)
}

func (s *InteractiveAppService) UpdateInteractiveDirectorPlan(storyID string, req interactive.UpdateDirectorPlanRequest) (interactive.DirectorPlan, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return interactive.DirectorPlan{}, ErrNoWorkspace
	}
	storyCtx, err := store.StoryContext(storyID, req.BranchID)
	if err != nil {
		return interactive.DirectorPlan{}, err
	}
	fence, err := s.drainInteractiveBinding(context.Background(), storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return interactive.DirectorPlan{}, err
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := fence.validateLocked(a); err != nil {
		return interactive.DirectorPlan{}, err
	}
	return store.UpdateDirectorPlan(storyID, req)
}

func (a *App) RebuildInteractiveDirectorPlan(storyID string, req interactive.RebuildDirectorPlanRequest) (interactive.DirectorPlan, error) {
	return a.interactiveService().RebuildInteractiveDirectorPlan(storyID, req)
}

func (s *InteractiveAppService) RebuildInteractiveDirectorPlan(storyID string, req interactive.RebuildDirectorPlanRequest) (interactive.DirectorPlan, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return interactive.DirectorPlan{}, ErrNoWorkspace
	}
	storyCtx, err := store.StoryContext(storyID, req.BranchID)
	if err != nil {
		return interactive.DirectorPlan{}, err
	}
	fence, err := s.drainInteractiveBinding(context.Background(), storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return interactive.DirectorPlan{}, err
	}
	seed := interactive.DirectorPlanSeed{Templates: interactive.DefaultStoryDirectorPlanningTemplates(), BranchPlanningTurns: 5, Source: firstNonEmptyApp(req.Source, "manual_rebuild")}
	if cfg := s.cfg(); cfg != nil && cfg.DataDir() != "" {
		if currentStoryCtx, contextErr := store.StoryContext(storyID, storyCtx.Snapshot.BranchID); contextErr == nil {
			if director := loadStoryDirectorForMeta(cfg.DataDir(), currentStoryCtx.Meta); director.ID != "" {
				seed.Templates = director.Strategy.PlanningTemplates
				seed.BranchPlanningTurns = director.Strategy.BranchPlanningTurns
			}
		} else {
			log.Printf("[interactive-director] load story context for rebuild failed story_id=%s branch_id=%s err=%v", storyID, req.BranchID, contextErr)
		}
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := fence.validateLocked(a); err != nil {
		return interactive.DirectorPlan{}, err
	}
	return store.RebuildDirectorPlan(storyID, req, seed)
}

func (a *App) RunInteractiveDirectorPlan(storyID string, req interactive.RunDirectorPlanRequest) (interactive.DirectorPlanStatus, error) {
	return a.interactiveService().RunInteractiveDirectorPlan(storyID, req)
}

func (s *InteractiveAppService) RunInteractiveDirectorPlan(storyID string, req interactive.RunDirectorPlanRequest) (interactive.DirectorPlanStatus, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	a := s.app
	a.mu.RLock()
	workspace := a.workspace
	a.mu.RUnlock()
	operation, err := a.acquireWorkspaceOperation(context.Background(), workspace, true)
	if err != nil {
		return interactive.DirectorPlanStatus{}, err
	}
	defer operation.Release()

	a.mu.RLock()
	if a.interactive == nil || a.bookState == nil || a.cfg == nil || a.workspace != workspace {
		a.mu.RUnlock()
		return interactive.DirectorPlanStatus{}, ErrNoWorkspace
	}
	store := a.interactive
	state := a.bookState
	sessionStore := a.sessionStore
	chatService := a.chatService
	runtimeCfg := *a.cfg
	directorTasks := a.workspaceDirectorTasks
	directorGenerator := a.directorGenerator
	runtimeCfg.Workspace = workspace
	novaDir := runtimeCfg.DataDir()
	a.mu.RUnlock()

	if layered, err := config.LoadLayeredWithStartupConfig(novaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
	} else {
		log.Printf("[interactive-director-agent] load settings for manual run failed workspace=%s err=%v", workspace, err)
	}
	storyCtx, err := store.StoryContext(storyID, req.BranchID)
	if err != nil {
		return interactive.DirectorPlanStatus{}, err
	}
	a.mu.RLock()
	active := interactiveTaskForScopeLocked(a, workspace, storyID, storyCtx.Snapshot.BranchID)
	a.mu.RUnlock()
	if active != nil {
		return interactive.DirectorPlanStatus{}, ErrAgentOperationActive
	}
	if storyCtx.Snapshot.CurrentTurn == nil {
		return interactive.DirectorPlanStatus{}, fmt.Errorf("开局尚未完成，无法运行导演规划")
	}
	turn := *storyCtx.Snapshot.CurrentTurn
	director := loadStoryDirectorForMeta(novaDir, storyCtx.Meta)
	decision := shouldRunInteractiveDirectorAgent(director.Strategy)
	if !decision.ShouldRun {
		if err := store.MarkDirectorPlanRunSkipped(storyID, storyCtx.Snapshot.BranchID, turn.ID, decision.Reason); err != nil {
			return interactive.DirectorPlanStatus{}, err
		}
		return store.DirectorPlanStatus(storyID, storyCtx.Snapshot.BranchID)
	}
	token, err := store.DirectorPlanRunToken(storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return interactive.DirectorPlanStatus{}, fmt.Errorf("准备导演规划运行版本失败: %w", err)
	}
	if err := store.MarkDirectorPlanRunStarted(storyID, storyCtx.Snapshot.BranchID, token, turn.ID, req.ForceEventEvaluation); err != nil {
		return interactive.DirectorPlanStatus{}, fmt.Errorf("标记导演规划运行状态失败: %w", err)
	}
	if err := operation.Context().Err(); err != nil {
		_ = store.MarkDirectorPlanRunFailed(storyID, storyCtx.Snapshot.BranchID, turn.ID, ErrWorkspaceTransition)
		return interactive.DirectorPlanStatus{}, ErrWorkspaceTransition
	}
	log.Printf("[interactive-director-agent] manual run scheduled story_id=%s branch_id=%s turn_id=%s source=%s", storyID, storyCtx.Snapshot.BranchID, turn.ID, firstNonEmptyApp(req.Source, "manual_retry"))
	conversation := newInteractiveConversation(store, novaDir, workspace, storyID, storyCtx.Snapshot.BranchID, turn.User, storyCtx.Meta.ReplyTargetChars, &runtimeCfg).bindDirectorRuntime(directorTasks, directorGenerator, chatService)
	startInteractiveDirectorTask(&runtimeCfg, state, conversation, turn, sessionStore, token)
	return store.DirectorPlanStatus(storyID, storyCtx.Snapshot.BranchID)
}

// ActiveInteractiveTask 返回当前游戏模式活跃任务（可能为 nil）。
func (a *App) ActiveInteractiveTask() *Task {
	return a.interactiveService().ActiveInteractiveTask()
}

func (s *InteractiveAppService) ActiveInteractiveTask() *Task {
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

	if layered, err := config.LoadLayeredWithStartupConfig(novaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
	} else {
		log.Printf("[interactive-agent] load layered settings failed workspace=%s err=%v", workspace, err)
	}
	return store, runtimeCfg, workspace, nil
}

func (s *InteractiveAppService) cfg() *config.Config {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}
