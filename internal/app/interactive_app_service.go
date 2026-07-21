package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/imagepreset"
	"denova/internal/interactive"
)

// InteractiveAppService 负责互动故事、剧情分支、导演和互动 Agent 任务。
type InteractiveAppService struct {
	app *App
}

func generateInteractiveDirector(ctx context.Context, cfg *config.Config, state *book.State, toolContext agent.InteractiveStoryToolContext, instruction string) (string, error) {
	return agent.GenerateInteractiveDirectorWithTools(ctx, cfg, state, toolContext, instruction)
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
	return store.UpdateStory(storyID, req)
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
	a := s.app
	a.mu.RLock()
	store := a.interactive
	sessionStore := a.sessionStore
	a.mu.RUnlock()
	if store == nil {
		return ErrNoWorkspace
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

func (a *App) RerollInteractiveRuleResolution(storyID, resolutionID string, req interactive.RuleResolutionRerollRequest) (interactive.RuleResolution, error) {
	return a.interactiveService().RerollInteractiveRuleResolution(storyID, resolutionID, req)
}

func (s *InteractiveAppService) RerollInteractiveRuleResolution(storyID, resolutionID string, req interactive.RuleResolutionRerollRequest) (interactive.RuleResolution, error) {
	store := s.store()
	if store == nil {
		return interactive.RuleResolution{}, ErrNoWorkspace
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
	store := s.store()
	if store == nil {
		return interactive.DirectorPlan{}, ErrNoWorkspace
	}
	return store.UpdateDirectorPlan(storyID, req)
}

func (a *App) RebuildInteractiveDirectorPlan(storyID string, req interactive.RebuildDirectorPlanRequest) (interactive.DirectorPlan, error) {
	return a.interactiveService().RebuildInteractiveDirectorPlan(storyID, req)
}

func (s *InteractiveAppService) RebuildInteractiveDirectorPlan(storyID string, req interactive.RebuildDirectorPlanRequest) (interactive.DirectorPlan, error) {
	store := s.store()
	if store == nil {
		return interactive.DirectorPlan{}, ErrNoWorkspace
	}
	seed := interactive.DirectorPlanSeed{Templates: interactive.DefaultStoryDirectorPlanningTemplates(), BranchPlanningTurns: 5, Source: firstNonEmptyApp(req.Source, "manual_rebuild")}
	if cfg := s.cfg(); cfg != nil && cfg.DataDir() != "" {
		if storyCtx, err := store.StoryContext(storyID, req.BranchID); err == nil {
			if director := loadStoryDirectorForMeta(cfg.DataDir(), storyCtx.Meta); director.ID != "" {
				seed.Templates = director.Strategy.PlanningTemplates
				seed.BranchPlanningTurns = director.Strategy.BranchPlanningTurns
			}
		} else {
			log.Printf("[interactive-director] load story context for rebuild failed story_id=%s branch_id=%s err=%v", storyID, req.BranchID, err)
		}
	}
	return store.RebuildDirectorPlan(storyID, req, seed)
}

func (a *App) RunInteractiveDirectorPlan(storyID string, req interactive.RunDirectorPlanRequest) (interactive.DirectorPlanStatus, error) {
	return a.interactiveService().RunInteractiveDirectorPlan(storyID, req)
}

func (s *InteractiveAppService) RunInteractiveDirectorPlan(storyID string, req interactive.RunDirectorPlanRequest) (interactive.DirectorPlanStatus, error) {
	a := s.app
	a.mu.RLock()
	if a.interactive == nil || a.bookState == nil || a.cfg == nil {
		a.mu.RUnlock()
		return interactive.DirectorPlanStatus{}, ErrNoWorkspace
	}
	store := a.interactive
	state := a.bookState
	sessionStore := a.sessionStore
	runtimeCfg := *a.cfg
	workspace := a.workspace
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
	log.Printf("[interactive-director-agent] manual run scheduled story_id=%s branch_id=%s turn_id=%s source=%s", storyID, storyCtx.Snapshot.BranchID, turn.ID, firstNonEmptyApp(req.Source, "manual_retry"))
	conversation := newInteractiveConversation(store, novaDir, workspace, storyID, storyCtx.Snapshot.BranchID, turn.User, storyCtx.Meta.ReplyTargetChars, &runtimeCfg).bindDirectorRuntime(a.directorTasksForWorkspace(workspace), a.interactiveDirectorGenerator())
	startInteractiveDirectorTask(&runtimeCfg, state, conversation, turn, sessionStore, token)
	return store.DirectorPlanStatus(storyID, storyCtx.Snapshot.BranchID)
}

func (a *App) CreateInteractiveBranch(storyID string, req interactive.CreateBranchRequest) (interactive.BranchSummary, error) {
	return a.interactiveService().CreateInteractiveBranch(storyID, req)
}

func (s *InteractiveAppService) CreateInteractiveBranch(storyID string, req interactive.CreateBranchRequest) (interactive.BranchSummary, error) {
	store := s.store()
	if store == nil {
		return interactive.BranchSummary{}, ErrNoWorkspace
	}
	return store.CreateBranch(storyID, req)
}

func (a *App) SwitchInteractiveBranch(storyID, branchID string) error {
	return a.interactiveService().SwitchInteractiveBranch(storyID, branchID)
}

func (s *InteractiveAppService) SwitchInteractiveBranch(storyID, branchID string) error {
	store := s.store()
	if store == nil {
		return ErrNoWorkspace
	}
	return store.SwitchBranch(storyID, branchID)
}

func (a *App) SwitchInteractiveTurnVersion(storyID string, req interactive.SwitchTurnVersionRequest) error {
	return a.interactiveService().SwitchInteractiveTurnVersion(storyID, req)
}

func (s *InteractiveAppService) SwitchInteractiveTurnVersion(storyID string, req interactive.SwitchTurnVersionRequest) error {
	store := s.store()
	if store == nil {
		return ErrNoWorkspace
	}
	return store.SwitchTurnVersion(storyID, req)
}

func (a *App) UpdateInteractiveTurnNarrative(storyID string, req interactive.UpdateTurnNarrativeRequest) (interactive.UpdateTurnNarrativeResult, error) {
	return a.interactiveService().UpdateInteractiveTurnNarrative(storyID, req)
}

func (s *InteractiveAppService) UpdateInteractiveTurnNarrative(storyID string, req interactive.UpdateTurnNarrativeRequest) (interactive.UpdateTurnNarrativeResult, error) {
	store := s.store()
	if store == nil {
		return interactive.UpdateTurnNarrativeResult{}, ErrNoWorkspace
	}
	result, err := store.UpdateTurnNarrative(storyID, req)
	if err != nil {
		log.Printf("[interactive-turn-edit] update failed story_id=%s branch_id=%s turn_id=%s err=%v", storyID, req.BranchID, req.TurnID, err)
		return interactive.UpdateTurnNarrativeResult{}, err
	}
	log.Printf(
		"[interactive-turn-edit] narrative updated story_id=%s branch_id=%s turn_id=%s narrative_bytes=%d context_compaction_invalidated=%t",
		storyID,
		result.Turn.BranchID,
		result.Turn.ID,
		len([]byte(result.Turn.Narrative)),
		result.ContextCompactionInvalidated,
	)
	return result, nil
}

func (a *App) DeleteInteractiveBranch(storyID, branchID string) error {
	return a.interactiveService().DeleteInteractiveBranch(storyID, branchID)
}

func (s *InteractiveAppService) DeleteInteractiveBranch(storyID, branchID string) error {
	store := s.store()
	if store == nil {
		return ErrNoWorkspace
	}
	return store.DeleteBranch(storyID, branchID)
}

func (a *App) InteractiveBranches(storyID string) ([]interactive.BranchSummary, error) {
	return a.interactiveService().InteractiveBranches(storyID)
}

func (s *InteractiveAppService) InteractiveBranches(storyID string) ([]interactive.BranchSummary, error) {
	store := s.store()
	if store == nil {
		return nil, ErrNoWorkspace
	}
	return store.Branches(storyID)
}

func (a *App) AppendInteractiveTurn(storyID, branchID, user, narrative string) (interactive.TurnEvent, error) {
	return a.interactiveService().AppendInteractiveTurn(storyID, branchID, user, narrative)
}

func (s *InteractiveAppService) AppendInteractiveTurn(storyID, branchID, user, narrative string) (interactive.TurnEvent, error) {
	store := s.store()
	if store == nil {
		return interactive.TurnEvent{}, ErrNoWorkspace
	}
	return store.AppendTurn(storyID, interactive.AppendTurnRequest{
		BranchID:  branchID,
		User:      user,
		Narrative: narrative,
	})
}

// StartInteractiveTask 启动游戏模式 Agent 任务，输出写回 interactive/story。
func (a *App) StartInteractiveTask(ctx context.Context, storyID, branchID, message string, styleScenes []string, locale string) *Task {
	return a.interactiveService().StartInteractiveTask(ctx, storyID, branchID, message, styleScenes, locale)
}

func (s *InteractiveAppService) StartInteractiveTask(ctx context.Context, storyID, branchID, message string, styleScenes []string, locale string) *Task {
	return s.startInteractiveTask(ctx, storyID, branchID, message, styleScenes, "", locale)
}

func (a *App) StartInteractiveRegenerateTask(ctx context.Context, storyID, branchID, turnID, message string, styleScenes []string, locale string) *Task {
	return a.interactiveService().StartInteractiveRegenerateTask(ctx, storyID, branchID, turnID, message, styleScenes, locale)
}

func (s *InteractiveAppService) StartInteractiveRegenerateTask(ctx context.Context, storyID, branchID, turnID, message string, styleScenes []string, locale string) *Task {
	return s.startInteractiveTask(ctx, storyID, branchID, message, styleScenes, turnID, locale)
}

func (a *App) AnalyzeInteractiveContext(storyID, branchID, message string, styleScenes []string, locale string) (agent.ContextAnalysis, error) {
	return a.interactiveService().AnalyzeInteractiveContext(storyID, branchID, message, styleScenes, locale)
}

func (s *InteractiveAppService) AnalyzeInteractiveContext(storyID, branchID, message string, styleScenes []string, locale string) (agent.ContextAnalysis, error) {
	a := s.app
	a.mu.RLock()
	if a.interactive == nil || a.bookState == nil || a.cfg == nil {
		a.mu.RUnlock()
		return agent.ContextAnalysis{}, ErrNoWorkspace
	}
	store := a.interactive
	state := a.bookState
	bookService := a.bookService
	runtimeCfg := *a.cfg
	workspace := a.workspace
	runtimeCfg.Workspace = workspace
	novaDir := runtimeCfg.DataDir()
	a.mu.RUnlock()

	if layered, err := config.LoadLayeredWithStartupConfig(novaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
	} else {
		log.Printf("[interactive-agent-analysis] load interactive settings failed workspace=%s err=%v", workspace, err)
	}
	applyRequestLocaleToConfig(&runtimeCfg, locale)

	storyCtx, err := store.StoryContext(storyID, branchID)
	if err != nil {
		return agent.ContextAnalysis{}, err
	}
	teller := loadInteractiveTeller(novaDir, storyCtx.Meta.StoryTellerID)
	runtimeCfg.InteractiveReplyTargetChars = storyCtx.Meta.ReplyTargetChars
	styleRules := convertTellerStyleRules(novaDir, teller.StyleRefs, teller.StyleRules, styleScenes)
	req := agent.ChatRequest{
		Message:     message,
		StyleScenes: styleScenes,
		StyleRules:  styleRules,
		Locale:      locale,
	}
	conversation := newInteractiveConversation(store, novaDir, workspace, storyID, branchID, message, runtimeCfg.InteractiveReplyTargetChars, &runtimeCfg).bindDirectorRuntime(a.directorTasksForWorkspace(workspace), a.interactiveDirectorGenerator())
	return agent.BuildInteractiveStoryContextAnalysis(&runtimeCfg, state, interactiveStoryTellerSystemInput(teller, styleRules), bookService, req, storyCtx.Snapshot.ContextCompaction, conversation.PrepareMessages)
}

func (a *App) AnalyzeInteractiveDirectorContext(storyID, branchID, turnID string, locale string) (agent.ContextAnalysis, error) {
	return a.interactiveService().AnalyzeInteractiveDirectorContext(storyID, branchID, turnID, locale)
}

func (s *InteractiveAppService) AnalyzeInteractiveDirectorContext(storyID, branchID, turnID string, locale string) (agent.ContextAnalysis, error) {
	a := s.app
	a.mu.RLock()
	if a.interactive == nil || a.bookState == nil || a.cfg == nil {
		a.mu.RUnlock()
		return agent.ContextAnalysis{}, ErrNoWorkspace
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
		log.Printf("[interactive-director-analysis] load interactive settings failed workspace=%s err=%v", workspace, err)
	}
	applyRequestLocaleToConfig(&runtimeCfg, locale)

	storyCtx, err := store.StoryContext(storyID, branchID)
	if err != nil {
		return agent.ContextAnalysis{}, err
	}
	turn, err := interactiveDirectorAnalysisTurn(storyCtx.Snapshot, turnID)
	if err != nil {
		return agent.ContextAnalysis{}, err
	}
	conversation := newInteractiveConversation(store, novaDir, workspace, storyID, storyCtx.Snapshot.BranchID, turn.User, storyCtx.Meta.ReplyTargetChars, &runtimeCfg).bindDirectorRuntime(a.directorTasksForWorkspace(workspace), a.interactiveDirectorGenerator())
	stableContext, instruction, err := conversation.buildDirectorModelInput(turn)
	if err != nil {
		return agent.ContextAnalysis{}, err
	}
	log.Printf("[interactive-director-analysis] built context story_id=%s branch_id=%s turn_id=%s instruction=%s", storyID, storyCtx.Snapshot.BranchID, turn.ID, interactivePartSummary(instruction))
	return agent.BuildInteractiveDirectorContextAnalysisWithStableContext(&runtimeCfg, stableContext.Title, stableContext.Content, stableContext.MaxBytes, instruction)
}

func interactiveDirectorAnalysisTurn(snapshot interactive.Snapshot, turnID string) (interactive.TurnEvent, error) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		if snapshot.CurrentTurn == nil {
			return interactive.TurnEvent{}, fmt.Errorf("开局尚未完成，无法分析导演上下文")
		}
		return *snapshot.CurrentTurn, nil
	}
	for _, turn := range snapshot.Turns {
		if turn.ID == turnID {
			return turn, nil
		}
	}
	return interactive.TurnEvent{}, fmt.Errorf("回合不存在: %s", turnID)
}

func (a *App) CompactInteractiveContext(ctx context.Context, storyID, branchID string) (agent.ContextCompactionResult, error) {
	return a.interactiveService().CompactInteractiveContext(ctx, storyID, branchID)
}

func (s *InteractiveAppService) CompactInteractiveContext(ctx context.Context, storyID, branchID string) (agent.ContextCompactionResult, error) {
	store, runtimeCfg, workspace, err := s.interactiveRuntimeConfig()
	if err != nil {
		return agent.ContextCompactionResult{}, err
	}
	storyCtx, err := store.StoryContext(storyID, branchID)
	if err != nil {
		return agent.ContextCompactionResult{}, err
	}
	source, existingCheckpoint := interactiveCompactionSource(storyCtx.Snapshot.Turns, storyCtx.Snapshot.ContextCompaction)
	epoch := 1
	if storyCtx.Snapshot.ContextCompaction != nil {
		epoch = storyCtx.Snapshot.ContextCompaction.Epoch + 1
	}
	_, result, err := agent.BuildContextCompaction(ctx, &runtimeCfg, config.AgentKindInteractiveStory, agent.ContextCompactionInput{
		Messages:           source,
		SourceMessages:     source,
		Phase:              "manual",
		Force:              true,
		ExistingCheckpoint: existingCheckpoint,
		KeepLatestUser:     true,
	}, epoch)
	if err != nil {
		return result, err
	}
	if !result.Triggered {
		return result, fmt.Errorf("没有可压缩的互动上下文")
	}
	event := interactive.ContextCompactionEvent{
		AgentKind:           config.AgentKindInteractiveStory,
		Epoch:               result.Epoch,
		Summary:             result.Summary,
		SourceTurnCount:     len(storyCtx.Snapshot.Turns),
		RetainedTurns:       result.RetainedTurns,
		TokensBefore:        result.TokensBefore,
		TokensAfter:         result.TokensAfter,
		TargetRatio:         result.TargetRatio,
		ContextWindowTokens: result.ContextWindowTokens,
		Strategy:            result.Strategy,
		Threshold:           result.Threshold,
		Reason:              "manual",
		Phase:               result.Phase,
	}
	event, err = store.AppendContextCompaction(storyID, storyCtx.Snapshot.BranchID, event)
	if err != nil {
		return result, err
	}
	result.Epoch = event.Epoch
	log.Printf("[interactive-agent] manual context compaction completed workspace=%s story_id=%s branch_id=%s epoch=%d source_turns=%d", workspace, storyID, storyCtx.Snapshot.BranchID, result.Epoch, len(storyCtx.Snapshot.Turns))
	return result, nil
}

func (a *App) RemoveInteractiveContextCompaction(storyID, branchID string) (bool, error) {
	return a.interactiveService().RemoveInteractiveContextCompaction(storyID, branchID)
}

func (s *InteractiveAppService) RemoveInteractiveContextCompaction(storyID, branchID string) (bool, error) {
	store := s.store()
	if store == nil {
		return false, ErrNoWorkspace
	}
	storyCtx, err := store.StoryContext(storyID, branchID)
	if err != nil {
		return false, err
	}
	if storyCtx.Snapshot.ContextCompaction == nil {
		return false, nil
	}
	_, err = store.AppendContextCompactionRemoval(storyID, storyCtx.Snapshot.BranchID, interactive.ContextCompactionRemovalEvent{
		AgentKind:       config.AgentKindInteractiveStory,
		CompactionID:    storyCtx.Snapshot.ContextCompaction.ID,
		SourceTurnCount: storyCtx.Snapshot.ContextCompaction.SourceTurnCount,
		Reason:          "user_removed",
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *InteractiveAppService) startInteractiveTask(ctx context.Context, storyID, branchID, message string, styleScenes []string, rewindTurnID string, locale string) *Task {
	a := s.app
	a.mu.Lock()
	if a.interactive == nil || a.bookState == nil || a.cfg == nil {
		a.mu.Unlock()
		log.Printf("[interactive-agent-task] 未选择 workspace，无法启动任务")
		return nil
	}
	if a.activeInteractiveRun != nil && a.activeInteractiveRun.task != nil && a.activeInteractiveRun.task.Status() == TaskRunning {
		log.Printf("[interactive-agent-task] replace running task id=%s", a.activeInteractiveRun.task.ID())
		a.activeInteractiveRun.task.Abort()
	}

	store := a.interactive
	state := a.bookState
	bookService := a.bookService
	chatService := a.chatService
	sessionStore := a.sessionStore
	runtimeCfg := *a.cfg
	workspace := a.workspace
	runtimeCfg.Workspace = workspace
	novaDir := runtimeCfg.DataDir()
	a.mu.Unlock()

	if layered, err := config.LoadLayeredWithStartupConfig(novaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
		log.Printf("[interactive-agent-task] load interactive settings workspace=%s", workspace)
	} else {
		log.Printf("[interactive-agent-task] load interactive settings failed workspace=%s err=%v", workspace, err)
	}
	applyRequestLocaleToConfig(&runtimeCfg, locale)

	storyCtx, err := store.StoryContext(storyID, branchID)
	if err != nil {
		log.Printf("[interactive-agent-task] 读取互动故事上下文失败 story_id=%s branch_id=%s err=%v", storyID, branchID, err)
		return nil
	}
	teller := loadInteractiveTeller(novaDir, storyCtx.Meta.StoryTellerID)
	runtimeCfg.InteractiveReplyTargetChars = storyCtx.Meta.ReplyTargetChars
	styleRules := convertTellerStyleRules(novaDir, teller.StyleRefs, teller.StyleRules, styleScenes)
	if len(styleRules) > 0 {
		log.Printf("[interactive-agent-task] inject teller style rules teller_id=%s scenes=%q count=%d rules=%q", teller.ID, styleScenes, len(styleRules), appStyleRuleNames(styleRules))
	}
	log.Printf("[interactive-agent-task] use story settings story_id=%s teller_id=%s target_chars=%d style_rules=%d", storyID, teller.ID, runtimeCfg.InteractiveReplyTargetChars, len(styleRules))
	tellerSystemInput := interactiveStoryTellerSystemInput(teller, styleRules)
	tellerSystemInput.ChoiceCount = storyCtx.Meta.ChoiceCount
	baseParentID := storyCtx.Meta.Branches[storyCtx.Snapshot.BranchID].Head
	conversation := newInteractiveConversation(store, novaDir, workspace, storyID, branchID, message, runtimeCfg.InteractiveReplyTargetChars, &runtimeCfg).bindDirectorRuntime(a.directorTasksForWorkspace(workspace), a.interactiveDirectorGenerator()).withBaseParentID(baseParentID).withOpeningStateSchema(storyCtx)
	var submitOpeningStateSchema func(context.Context, interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error)
	if interactive.StoryStateSchemaPolicyUsesOpeningGameAgent(storyCtx.Meta.StateSchemaPolicy) && storyCtx.Meta.StateSchemaInitialization != nil && storyCtx.Meta.StateSchemaInitialization.Status == interactive.StateSchemaInitializationWaitingOpening && len(storyCtx.Snapshot.Turns) == 0 {
		submitOpeningStateSchema = conversation.SubmitOpeningStateSchemaBatch
	}
	runner, err := buildInteractiveStoryRunner(ctx, &runtimeCfg, state, tellerSystemInput, agent.InteractiveStoryToolContext{
		Store:                  store,
		StoryID:                storyID,
		BranchID:               storyCtx.Snapshot.BranchID,
		SubmitStateSchemaBatch: submitOpeningStateSchema,
		PrepareTurn:            conversation.PrepareInteractiveTurn,
		SubmitTurnResult:       conversation.SubmitTurnResult,
		TurnResultReady:        conversation.InteractiveNarrativeReady,
	})
	if err != nil {
		log.Printf("[interactive-agent-task] 刷新互动故事 Agent Runner 失败 workspace=%s err=%v", workspace, err)
		return nil
	}
	a.mu.Lock()
	if a.workspace == workspace {
		a.interactiveStoryRunner = runner
	}
	a.mu.Unlock()

	req := agent.ChatRequest{
		Message:     message,
		StyleScenes: styleScenes,
		StyleRules:  styleRules,
		Locale:      locale,
	}
	task := NewTask(func(ctx context.Context, task *Task, emit func(agent.Event)) {
		log.Printf("[interactive-agent-task] run begin id=%s story_id=%s branch_id=%s rewind_turn_id=%s message_len=%d style_scenes=%d", task.ID(), storyID, branchID, rewindTurnID, len(message), len(styleScenes))
		if strings.TrimSpace(rewindTurnID) != "" {
			if err := store.RewindToTurnParent(storyID, interactive.RewindTurnRequest{BranchID: branchID, TurnID: rewindTurnID}); err != nil {
				log.Printf("[interactive-agent-task] 回退互动故事分支失败 story_id=%s branch_id=%s turn_id=%s err=%v", storyID, branchID, rewindTurnID, err)
				emit(agent.Event{Type: "error", Data: map[string]string{"message": "回退互动故事分支失败：" + err.Error()}})
				return
			}
			rewoundContext, err := store.StoryContext(storyID, branchID)
			if err != nil {
				log.Printf("[interactive-agent-task] 重新读取回退后的互动故事上下文失败 story_id=%s branch_id=%s err=%v", storyID, branchID, err)
				emit(agent.Event{Type: "error", Data: map[string]string{"message": "读取回退后的互动故事失败：" + err.Error()}})
				return
			}
			conversation.withBaseParentID(rewoundContext.Meta.Branches[rewoundContext.Snapshot.BranchID].Head)
			log.Printf("[interactive-agent-task] rewind branch for regeneration story_id=%s branch_id=%s turn_id=%s base_parent_id=%s", storyID, branchID, rewindTurnID, rewoundContext.Meta.Branches[rewoundContext.Snapshot.BranchID].Head)
		}
		persistedEmitted := false
		maintenanceScheduled := false
		scheduleMaintenance := func(turn interactive.TurnEvent, persistedSnapshot *interactive.Snapshot) {
			director := conversation.storyDirectorForMeta(storyCtx.Meta)
			policy := interactive.ResolveStoryDirectorRunPolicy(storyCtx.Meta.DirectorRunPolicy, director.Strategy)
			if persistedSnapshot == nil {
				loaded, loadErr := store.Snapshot(storyID, turn.BranchID)
				if loadErr != nil {
					log.Printf("[interactive-director-agent] load scheduling snapshot failed story_id=%s branch_id=%s turn_id=%s err=%v", storyID, turn.BranchID, turn.ID, loadErr)
				} else {
					persistedSnapshot = &loaded
				}
			}
			committedTurns := len(storyCtx.Snapshot.Turns) + 1
			planStatus := ""
			if storyCtx.Snapshot.DirectorPlanStatus != nil {
				planStatus = storyCtx.Snapshot.DirectorPlanStatus.Status
			}
			if persistedSnapshot != nil {
				committedTurns = len(persistedSnapshot.Turns)
				if persistedSnapshot.DirectorPlanStatus != nil {
					planStatus = persistedSnapshot.DirectorPlanStatus.Status
				}
			}
			materialUpdate := turn.TurnResult != nil && turn.TurnResult.DirectorUpdate != nil && turn.TurnResult.DirectorUpdate.Needed
			decision := interactive.DecideDirectorRunAfterTurn(director.Strategy.Enabled, policy, interactive.DirectorRunScheduleContext{
				CommittedTurns: committedTurns,
				PlanStatus:     planStatus,
				MaterialUpdate: materialUpdate,
			})
			log.Printf("[interactive-director-agent] maintenance decision story_id=%s branch_id=%s turn_id=%s policy_mode=%s interval_turns=%d committed_turns=%d plan_status=%s run_plan=%t reason=%s", storyID, turn.BranchID, turn.ID, policy.Mode, policy.IntervalTurns, committedTurns, planStatus, decision.ShouldRun, decision.Reason)
			startInteractiveDirectorMaintenanceTask(&runtimeCfg, state, conversation, turn, sessionStore, decision.ShouldRun)
			maintenanceScheduled = true
		}
		interactiveEmit := func(event agent.Event) {
			if event.Type == "done" && !persistedEmitted && ctx.Err() == nil {
				persistedEmitted = true
				persistedSnapshot := emitInteractiveTurnPersisted(store, storyID, conversation, emit)
				if turn, _, ok := conversation.LastTurnForState(); ok {
					scheduleMaintenance(turn, persistedSnapshot)
				}
			}
			emit(event)
		}
		chatService.RunWithOptions(ctx, runner, conversation, bookService, req, agent.RunOptions{
			AgentKind:           agent.AgentKindInteractiveStory,
			TaskID:              task.ID(),
			StoryID:             storyID,
			BranchID:            conversation.branchID,
			Workspace:           workspace,
			Mode:                "interactive",
			IdleTimeout:         agentIdleTimeout(runtimeCfg),
			ToolResultMaxBytes:  agentToolResultMaxBytes(runtimeCfg),
			SystemPromptLog:     agent.BuildInteractiveStoryInstructionComposition(&runtimeCfg, state, tellerSystemInput),
			OnMutationsVerified: a.automationMutationCallback("interactive_agent_post_run"),
		}, interactiveEmit)
		if turn, _, ok := conversation.LastTurnForState(); ok && ctx.Err() == nil && !maintenanceScheduled {
			scheduleMaintenance(turn, nil)
		}
		log.Printf("[interactive-agent-task] run end id=%s status=%s", task.ID(), task.Status())
	})

	if !s.bindActiveInteractiveTask(task, InteractiveTaskInfo{
		Workspace:            workspace,
		StoryID:              storyID,
		BranchID:             storyCtx.Snapshot.BranchID,
		Message:              message,
		RegenerateFromTurnID: rewindTurnID,
	}) {
		log.Printf("[interactive-agent-task] skip active task binding after workspace changed id=%s workspace=%s story_id=%s branch_id=%s", task.ID(), workspace, storyID, storyCtx.Snapshot.BranchID)
	}

	return task
}

func emitInteractiveTurnPersisted(store *interactive.Store, storyID string, conversation *interactiveConversation, emit func(agent.Event)) *interactive.Snapshot {
	if store == nil || conversation == nil || emit == nil {
		return nil
	}
	turn, _, ok := conversation.LastTurnForState()
	if !ok || strings.TrimSpace(turn.ID) == "" {
		return nil
	}
	snapshot, err := store.Snapshot(storyID, turn.BranchID)
	if err != nil {
		log.Printf("[interactive-agent-task] load persisted turn snapshot failed story_id=%s branch_id=%s turn_id=%s err=%v", storyID, turn.BranchID, turn.ID, err)
		return nil
	}
	persistedTurn := turn
	for _, snapshotTurn := range snapshot.Turns {
		if snapshotTurn.ID == turn.ID {
			persistedTurn = snapshotTurn
			break
		}
	}
	event := InteractiveTurnPersistedEvent{
		StoryID:                  storyID,
		BranchID:                 snapshot.BranchID,
		Turn:                     persistedTurn,
		DirectorPlanStatus:       snapshot.DirectorPlanStatus,
		State:                    snapshot.State,
		Graph:                    snapshot.Graph,
		Branches:                 snapshot.Graph.Branches,
		ContextCompaction:        snapshot.ContextCompaction,
		ContextCompactionRemoval: snapshot.ContextCompactionRemoval,
	}
	emit(agent.Event{Type: "interactive_turn_persisted", Data: event})
	log.Printf("[interactive-agent-task] emitted persisted turn story_id=%s branch_id=%s turn_id=%s", storyID, snapshot.BranchID, persistedTurn.ID)
	return &snapshot
}

func (a *App) InteractiveTellers() ([]interactive.Teller, error) {
	return a.interactiveService().InteractiveTellers()
}

func (s *InteractiveAppService) InteractiveTellers() ([]interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return nil, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.DataDir()).List()
}

func (a *App) InteractiveTeller(id string) (interactive.Teller, error) {
	return a.interactiveService().InteractiveTeller(id)
}

func (s *InteractiveAppService) InteractiveTeller(id string) (interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.Teller{}, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.DataDir()).Get(id)
}

func (a *App) CreateInteractiveTeller(teller interactive.Teller) (interactive.Teller, error) {
	return a.interactiveService().CreateInteractiveTeller(teller)
}

func (s *InteractiveAppService) CreateInteractiveTeller(teller interactive.Teller) (interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.Teller{}, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.DataDir()).Create(teller)
}

func (a *App) UpdateInteractiveTeller(id string, teller interactive.Teller, baseRevision ...string) (interactive.Teller, error) {
	return a.interactiveService().UpdateInteractiveTeller(id, teller, firstRevision(baseRevision))
}

func (s *InteractiveAppService) UpdateInteractiveTeller(id string, teller interactive.Teller, baseRevision string) (interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.Teller{}, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.DataDir()).Update(id, teller, baseRevision)
}

func (a *App) DeleteInteractiveTeller(id string) error {
	return a.interactiveService().DeleteInteractiveTeller(id)
}

func (s *InteractiveAppService) DeleteInteractiveTeller(id string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.DataDir()).Delete(id)
}

func (a *App) StoryDirectors() ([]interactive.StoryDirector, error) {
	return a.interactiveService().StoryDirectors()
}

func (s *InteractiveAppService) StoryDirectors() ([]interactive.StoryDirector, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return nil, ErrNoWorkspace
	}
	return interactive.NewStoryDirectorLibrary(cfg.DataDir()).List()
}

func (a *App) StoryDirector(id string) (interactive.StoryDirector, error) {
	return a.interactiveService().StoryDirector(id)
}

func (s *InteractiveAppService) StoryDirector(id string) (interactive.StoryDirector, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.StoryDirector{}, ErrNoWorkspace
	}
	return interactive.NewStoryDirectorLibrary(cfg.DataDir()).Get(id)
}

func (a *App) CreateStoryDirector(director interactive.StoryDirector) (interactive.StoryDirector, error) {
	return a.interactiveService().CreateStoryDirector(director)
}

func (s *InteractiveAppService) CreateStoryDirector(director interactive.StoryDirector) (interactive.StoryDirector, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.StoryDirector{}, ErrNoWorkspace
	}
	return interactive.NewStoryDirectorLibrary(cfg.DataDir()).Create(director)
}

func (a *App) UpdateStoryDirector(id string, director interactive.StoryDirector, baseRevision ...string) (interactive.StoryDirector, error) {
	return a.interactiveService().UpdateStoryDirector(id, director, firstRevision(baseRevision))
}

func (s *InteractiveAppService) UpdateStoryDirector(id string, director interactive.StoryDirector, baseRevision string) (interactive.StoryDirector, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.StoryDirector{}, ErrNoWorkspace
	}
	return interactive.NewStoryDirectorLibrary(cfg.DataDir()).Update(id, director, baseRevision)
}

func (a *App) DeleteStoryDirector(id string) error {
	return a.interactiveService().DeleteStoryDirector(id)
}

func (s *InteractiveAppService) DeleteStoryDirector(id string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return ErrNoWorkspace
	}
	return interactive.NewStoryDirectorLibrary(cfg.DataDir()).Delete(id)
}

func (a *App) EventPackages() ([]interactive.EventPackageModule, error) {
	return a.interactiveService().EventPackages()
}

func (s *InteractiveAppService) EventPackages() ([]interactive.EventPackageModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return nil, ErrNoWorkspace
	}
	return interactive.NewEventPackageLibrary(cfg.DataDir()).List()
}

func (a *App) EventPackage(id string) (interactive.EventPackageModule, error) {
	return a.interactiveService().EventPackage(id)
}

func (s *InteractiveAppService) EventPackage(id string) (interactive.EventPackageModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.EventPackageModule{}, ErrNoWorkspace
	}
	return interactive.NewEventPackageLibrary(cfg.DataDir()).Get(id)
}

func (a *App) CreateEventPackage(item interactive.EventPackageModule) (interactive.EventPackageModule, error) {
	return a.interactiveService().CreateEventPackage(item)
}

func (s *InteractiveAppService) CreateEventPackage(item interactive.EventPackageModule) (interactive.EventPackageModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.EventPackageModule{}, ErrNoWorkspace
	}
	return interactive.NewEventPackageLibrary(cfg.DataDir()).Create(item)
}

func (a *App) UpdateEventPackage(id string, item interactive.EventPackageModule, baseRevision ...string) (interactive.EventPackageModule, error) {
	return a.interactiveService().UpdateEventPackage(id, item, firstRevision(baseRevision))
}

func (s *InteractiveAppService) UpdateEventPackage(id string, item interactive.EventPackageModule, baseRevision string) (interactive.EventPackageModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.EventPackageModule{}, ErrNoWorkspace
	}
	return interactive.NewEventPackageLibrary(cfg.DataDir()).Update(id, item, baseRevision)
}

func (a *App) DeleteEventPackage(id string) error {
	return a.interactiveService().DeleteEventPackage(id)
}

func (s *InteractiveAppService) DeleteEventPackage(id string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return ErrNoWorkspace
	}
	return interactive.NewEventPackageLibrary(cfg.DataDir()).Delete(id)
}

func (a *App) RuleSystems() ([]interactive.RuleSystemModule, error) {
	return a.interactiveService().RuleSystems()
}

func (s *InteractiveAppService) RuleSystems() ([]interactive.RuleSystemModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return nil, ErrNoWorkspace
	}
	return interactive.NewRuleSystemLibrary(cfg.DataDir()).List()
}

func (a *App) RuleSystem(id string) (interactive.RuleSystemModule, error) {
	return a.interactiveService().RuleSystem(id)
}

func (s *InteractiveAppService) RuleSystem(id string) (interactive.RuleSystemModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.RuleSystemModule{}, ErrNoWorkspace
	}
	return interactive.NewRuleSystemLibrary(cfg.DataDir()).Get(id)
}

func (a *App) CreateRuleSystem(item interactive.RuleSystemModule) (interactive.RuleSystemModule, error) {
	return a.interactiveService().CreateRuleSystem(item)
}

func (s *InteractiveAppService) CreateRuleSystem(item interactive.RuleSystemModule) (interactive.RuleSystemModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.RuleSystemModule{}, ErrNoWorkspace
	}
	return interactive.NewRuleSystemLibrary(cfg.DataDir()).Create(item)
}

func (a *App) UpdateRuleSystem(id string, item interactive.RuleSystemModule, baseRevision ...string) (interactive.RuleSystemModule, error) {
	return a.interactiveService().UpdateRuleSystem(id, item, firstRevision(baseRevision))
}

func (s *InteractiveAppService) UpdateRuleSystem(id string, item interactive.RuleSystemModule, baseRevision string) (interactive.RuleSystemModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.RuleSystemModule{}, ErrNoWorkspace
	}
	return interactive.NewRuleSystemLibrary(cfg.DataDir()).Update(id, item, baseRevision)
}

func (a *App) DeleteRuleSystem(id string) error {
	return a.interactiveService().DeleteRuleSystem(id)
}

func (s *InteractiveAppService) DeleteRuleSystem(id string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return ErrNoWorkspace
	}
	return interactive.NewRuleSystemLibrary(cfg.DataDir()).Delete(id)
}

func (a *App) ActorStates() ([]interactive.ActorStateModule, error) {
	return a.interactiveService().ActorStates()
}

func (s *InteractiveAppService) ActorStates() ([]interactive.ActorStateModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return nil, ErrNoWorkspace
	}
	return interactive.NewActorStateLibrary(cfg.DataDir()).List()
}

func (a *App) ActorState(id string) (interactive.ActorStateModule, error) {
	return a.interactiveService().ActorState(id)
}

func (s *InteractiveAppService) ActorState(id string) (interactive.ActorStateModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.ActorStateModule{}, ErrNoWorkspace
	}
	return interactive.NewActorStateLibrary(cfg.DataDir()).Get(id)
}

func (a *App) CreateActorState(item interactive.ActorStateModule) (interactive.ActorStateModule, error) {
	return a.interactiveService().CreateActorState(item)
}

func (s *InteractiveAppService) CreateActorState(item interactive.ActorStateModule) (interactive.ActorStateModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.ActorStateModule{}, ErrNoWorkspace
	}
	return interactive.NewActorStateLibrary(cfg.DataDir()).Create(item)
}

func (a *App) UpdateActorState(id string, item interactive.ActorStateModule, baseRevision ...string) (interactive.ActorStateModule, error) {
	return a.interactiveService().UpdateActorState(id, item, firstRevision(baseRevision))
}

func (s *InteractiveAppService) UpdateActorState(id string, item interactive.ActorStateModule, baseRevision string) (interactive.ActorStateModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.ActorStateModule{}, ErrNoWorkspace
	}
	return interactive.NewActorStateLibrary(cfg.DataDir()).Update(id, item, baseRevision)
}

func (a *App) DeleteActorState(id string) error {
	return a.interactiveService().DeleteActorState(id)
}

func (s *InteractiveAppService) DeleteActorState(id string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return ErrNoWorkspace
	}
	return interactive.NewActorStateLibrary(cfg.DataDir()).Delete(id)
}

func (a *App) ImagePresets() ([]imagepreset.Preset, error) {
	return a.interactiveService().ImagePresets()
}

func (s *InteractiveAppService) ImagePresets() ([]imagepreset.Preset, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return nil, ErrNoWorkspace
	}
	return imagepreset.NewLibrary(cfg.DataDir()).List()
}

func (a *App) ImagePreset(id string) (imagepreset.Preset, error) {
	return a.interactiveService().ImagePreset(id)
}

func (s *InteractiveAppService) ImagePreset(id string) (imagepreset.Preset, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return imagepreset.Preset{}, ErrNoWorkspace
	}
	return imagepreset.NewLibrary(cfg.DataDir()).Get(id)
}

func (a *App) CreateImagePreset(preset imagepreset.Preset) (imagepreset.Preset, error) {
	return a.interactiveService().CreateImagePreset(preset)
}

func (s *InteractiveAppService) CreateImagePreset(preset imagepreset.Preset) (imagepreset.Preset, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return imagepreset.Preset{}, ErrNoWorkspace
	}
	return imagepreset.NewLibrary(cfg.DataDir()).Create(preset)
}

func (a *App) UpdateImagePreset(id string, preset imagepreset.Preset, baseRevision ...string) (imagepreset.Preset, error) {
	return a.interactiveService().UpdateImagePreset(id, preset, firstRevision(baseRevision))
}

func (s *InteractiveAppService) UpdateImagePreset(id string, preset imagepreset.Preset, baseRevision string) (imagepreset.Preset, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return imagepreset.Preset{}, ErrNoWorkspace
	}
	return imagepreset.NewLibrary(cfg.DataDir()).Update(id, preset, baseRevision)
}

func (a *App) DeleteImagePreset(id string) error {
	return a.interactiveService().DeleteImagePreset(id)
}

func (s *InteractiveAppService) DeleteImagePreset(id string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return ErrNoWorkspace
	}
	return imagepreset.NewLibrary(cfg.DataDir()).Delete(id)
}

// ActiveInteractiveTask 返回当前游戏模式活跃任务（可能为 nil）。
func (a *App) ActiveInteractiveTask() *Task {
	return a.interactiveService().ActiveInteractiveTask()
}

func (s *InteractiveAppService) ActiveInteractiveTask() *Task {
	task, _ := s.ActiveInteractiveTaskFor("", "")
	return task
}

// AbortInteractiveTask 终止当前游戏模式活跃任务。
func (a *App) AbortInteractiveTask() {
	a.interactiveService().AbortInteractiveTask()
}

func (s *InteractiveAppService) AbortInteractiveTask() {
	task, _ := s.ActiveInteractiveTaskFor("", "")
	if task != nil {
		task.Abort()
	}
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
