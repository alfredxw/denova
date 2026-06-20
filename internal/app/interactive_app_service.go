package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	"nova/config"
	"nova/internal/agent"
	"nova/internal/interactive"
	"nova/internal/session"
)

// InteractiveAppService 负责互动故事、剧情分支、导演和互动 Agent 任务。
type InteractiveAppService struct {
	app *App
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

func (a *App) CreateInteractiveStory(req interactive.CreateStoryRequest) (interactive.StorySummary, error) {
	return a.interactiveService().CreateInteractiveStory(req)
}

func (s *InteractiveAppService) CreateInteractiveStory(req interactive.CreateStoryRequest) (interactive.StorySummary, error) {
	store := s.store()
	if store == nil {
		return interactive.StorySummary{}, ErrNoWorkspace
	}
	return store.CreateStory(req)
}

func (a *App) UpdateInteractiveStory(storyID string, req interactive.UpdateStoryRequest) (interactive.StorySummary, error) {
	return a.interactiveService().UpdateInteractiveStory(storyID, req)
}

func (s *InteractiveAppService) UpdateInteractiveStory(storyID string, req interactive.UpdateStoryRequest) (interactive.StorySummary, error) {
	store := s.store()
	if store == nil {
		return interactive.StorySummary{}, ErrNoWorkspace
	}
	return store.UpdateStory(storyID, req)
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

func (a *App) InteractiveMemory(storyID, branchID string, includeArchived bool) (interactive.InteractiveMemoryState, error) {
	return a.interactiveService().InteractiveMemory(storyID, branchID, includeArchived)
}

func (s *InteractiveAppService) InteractiveMemory(storyID, branchID string, includeArchived bool) (interactive.InteractiveMemoryState, error) {
	store := s.store()
	if store == nil {
		return interactive.InteractiveMemoryState{}, ErrNoWorkspace
	}
	return store.InteractiveMemory(storyID, branchID, includeArchived)
}

func (a *App) StoryMemory(storyID, branchID string, includeArchived bool) (interactive.StoryMemoryState, error) {
	return a.interactiveService().StoryMemory(storyID, branchID, includeArchived)
}

func (s *InteractiveAppService) StoryMemory(storyID, branchID string, includeArchived bool) (interactive.StoryMemoryState, error) {
	store := s.store()
	if store == nil {
		return interactive.StoryMemoryState{}, ErrNoWorkspace
	}
	return store.StoryMemory(storyID, branchID, includeArchived)
}

func (a *App) UpdateStoryMemorySettings(storyID string, req interactive.StoryMemorySettingsUpdateRequest) (interactive.StoryMemorySettings, error) {
	return a.interactiveService().UpdateStoryMemorySettings(storyID, req)
}

func (s *InteractiveAppService) UpdateStoryMemorySettings(storyID string, req interactive.StoryMemorySettingsUpdateRequest) (interactive.StoryMemorySettings, error) {
	store := s.store()
	if store == nil {
		return interactive.StoryMemorySettings{}, ErrNoWorkspace
	}
	return store.UpdateStoryMemorySettings(storyID, req)
}

func (a *App) SaveStoryMemoryStructure(storyID string, req interactive.StoryMemoryStructureRequest) (interactive.StoryMemoryStructure, error) {
	return a.interactiveService().SaveStoryMemoryStructure(storyID, req)
}

func (s *InteractiveAppService) SaveStoryMemoryStructure(storyID string, req interactive.StoryMemoryStructureRequest) (interactive.StoryMemoryStructure, error) {
	store := s.store()
	if store == nil {
		return interactive.StoryMemoryStructure{}, ErrNoWorkspace
	}
	return store.SaveStoryMemoryStructure(storyID, req)
}

func (a *App) DeleteStoryMemoryStructure(storyID, structureID string) error {
	return a.interactiveService().DeleteStoryMemoryStructure(storyID, structureID)
}

func (s *InteractiveAppService) DeleteStoryMemoryStructure(storyID, structureID string) error {
	store := s.store()
	if store == nil {
		return ErrNoWorkspace
	}
	return store.DeleteStoryMemoryStructure(storyID, structureID)
}

func (a *App) SaveStoryMemoryRecord(storyID string, req interactive.StoryMemoryRecordRequest) (interactive.StoryMemoryRecord, error) {
	return a.interactiveService().SaveStoryMemoryRecord(storyID, req)
}

func (s *InteractiveAppService) SaveStoryMemoryRecord(storyID string, req interactive.StoryMemoryRecordRequest) (interactive.StoryMemoryRecord, error) {
	store := s.store()
	if store == nil {
		return interactive.StoryMemoryRecord{}, ErrNoWorkspace
	}
	return store.SaveStoryMemoryRecord(storyID, req)
}

func (a *App) SetStoryMemoryRecordArchived(storyID, recordID, branchID string, archived bool) (interactive.StoryMemoryRecord, error) {
	return a.interactiveService().SetStoryMemoryRecordArchived(storyID, recordID, branchID, archived)
}

func (s *InteractiveAppService) SetStoryMemoryRecordArchived(storyID, recordID, branchID string, archived bool) (interactive.StoryMemoryRecord, error) {
	store := s.store()
	if store == nil {
		return interactive.StoryMemoryRecord{}, ErrNoWorkspace
	}
	return store.SetStoryMemoryRecordArchived(storyID, recordID, branchID, archived)
}

func (a *App) GenerateStoryMemory(ctx context.Context, storyID, branchID string) (interactive.StoryMemoryState, error) {
	return a.interactiveService().GenerateStoryMemory(ctx, storyID, branchID)
}

func (s *InteractiveAppService) GenerateStoryMemory(ctx context.Context, storyID, branchID string) (interactive.StoryMemoryState, error) {
	state, _, err := s.runStoryMemoryGenerate(ctx, storyID, branchID, nil)
	return state, err
}

func (a *App) StartStoryMemoryGenerateTask(storyID, branchID string) *Task {
	return a.interactiveService().StartStoryMemoryGenerateTask(storyID, branchID)
}

func (s *InteractiveAppService) StartStoryMemoryGenerateTask(storyID, branchID string) *Task {
	return NewTask(func(ctx context.Context, task *Task, emit func(agent.Event)) {
		log.Printf("[interactive-memory-agent] manual stream begin task_id=%s story_id=%s branch_id=%s", task.ID(), storyID, branchID)
		emit(agent.Event{Type: "thinking", Data: map[string]string{"content": "正在读取当前剧情线和最近回合，准备整理故事记忆。"}})
		state, patchCount, err := s.runStoryMemoryGenerate(ctx, storyID, branchID, emit)
		if err != nil {
			log.Printf("[interactive-memory-agent] manual stream failed task_id=%s story_id=%s branch_id=%s err=%v", task.ID(), storyID, branchID, err)
			emit(agent.Event{Type: "error", Data: map[string]string{"message": err.Error()}})
			return
		}
		emit(agent.Event{Type: "story_memory_result", Data: map[string]any{
			"story_id":     state.StoryID,
			"branch_id":    state.BranchID,
			"records":      len(state.Records),
			"patches":      patchCount,
			"sync_status":  state.SyncStatus,
			"sync_error":   state.SyncError,
			"next_auto_in": state.NextAutoInTurns,
		}})
		emit(agent.Event{Type: "done", Data: map[string]string{"status": "ok"}})
		log.Printf("[interactive-memory-agent] manual stream done task_id=%s story_id=%s branch_id=%s patches=%d records=%d", task.ID(), storyID, state.BranchID, patchCount, len(state.Records))
	})
}

func (s *InteractiveAppService) runStoryMemoryGenerate(ctx context.Context, storyID, branchID string, emit func(agent.Event)) (interactive.StoryMemoryState, int, error) {
	a := s.app
	a.mu.Lock()
	store := a.interactive
	cfg := a.cfg
	workspace := a.workspace
	sessionStore := a.sessionStore
	a.mu.Unlock()
	if store == nil || cfg == nil {
		return interactive.StoryMemoryState{}, 0, ErrNoWorkspace
	}
	snapshot, err := store.Snapshot(storyID, branchID)
	if err != nil {
		return interactive.StoryMemoryState{}, 0, err
	}
	if snapshot.CurrentTurn == nil {
		return interactive.StoryMemoryState{}, 0, fmt.Errorf("当前分支还没有可整理的互动回合")
	}
	runtimeCfg := *cfg
	runtimeCfg.Workspace = workspace
	conversation := newInteractiveConversation(store, runtimeCfg.NovaDir, workspace, storyID, snapshot.BranchID, snapshot.CurrentTurn.User, runtimeCfg.InteractiveReplyTargetChars, &runtimeCfg)
	instruction, err := conversation.BuildStateInstruction(*snapshot.CurrentTurn)
	if err != nil {
		return interactive.StoryMemoryState{}, 0, err
	}
	runCtx, cancel := context.WithTimeout(ctx, interactiveStateTimeout)
	defer cancel()
	if emit != nil {
		emit(agent.Event{Type: "tool_call", Data: map[string]string{
			"id":   "story_memory_context",
			"name": "build_story_memory_context",
			"args": fmt.Sprintf("story_id=%s branch_id=%s turn_id=%s", storyID, snapshot.BranchID, snapshot.CurrentTurn.ID),
		}})
		emit(agent.Event{Type: "tool_result", Data: map[string]string{
			"id":      "story_memory_context",
			"name":    "build_story_memory_context",
			"content": "已读取当前剧情线、当前回合和有界故事记忆上下文。",
		}})
	}
	generate := agent.GenerateInteractiveState
	if emit != nil {
		generate = func(ctx context.Context, cfg *config.Config, instruction string) (string, error) {
			return agent.StreamInteractiveState(ctx, cfg, instruction, emit)
		}
	}
	var patchCount int
	result, err := runInteractiveMemoryAgentWithRetry(runCtx, &runtimeCfg, instruction, sessionStore, generate, func(result interactiveMemoryAgentResult) error {
		patchCount = len(result.StoryMemoryPatches)
		if len(result.StoryMemoryPatches) == 0 {
			return nil
		}
		if emit != nil {
			emit(agent.Event{Type: "tool_call", Data: map[string]string{
				"id":   "story_memory_apply",
				"name": "apply_story_memory_patches",
				"args": fmt.Sprintf("patches=%d branch_id=%s", patchCount, snapshot.BranchID),
			}})
		}
		appliedRecords, err := store.ApplyStoryMemoryPatches(storyID, snapshot.BranchID, snapshot.CurrentTurn.ID, result.StoryMemoryPatches)
		if err != nil {
			return err
		}
		patchCount = len(appliedRecords)
		if emit != nil {
			emit(agent.Event{Type: "tool_result", Data: map[string]string{
				"id":      "story_memory_apply",
				"name":    "apply_story_memory_patches",
				"content": fmt.Sprintf("已写入 %d 条故事记忆更新。", patchCount),
			}})
		}
		return nil
	})
	if err != nil {
		_ = store.MarkInteractiveMemoryFailed(storyID, interactive.MarkStateFailedRequest{ParentID: snapshot.CurrentTurn.ID, BranchID: snapshot.BranchID, Error: err.Error()})
		return interactive.StoryMemoryState{}, 0, err
	}
	if len(result.StateOps) > 0 && snapshot.CurrentTurn.StateStatus == "pending" {
		if _, err := store.AppendStateDelta(storyID, interactive.AppendStateDeltaRequest{
			ParentID: snapshot.CurrentTurn.ID,
			BranchID: snapshot.BranchID,
			Ops:      result.StateOps,
		}); err != nil {
			_ = store.MarkInteractiveMemoryFailed(storyID, interactive.MarkStateFailedRequest{ParentID: snapshot.CurrentTurn.ID, BranchID: snapshot.BranchID, Error: err.Error()})
			return interactive.StoryMemoryState{}, patchCount, err
		}
	}
	if err := store.MarkInteractiveMemoryReady(storyID, snapshot.BranchID, snapshot.CurrentTurn.ID); err != nil {
		return interactive.StoryMemoryState{}, patchCount, err
	}
	state, err := store.StoryMemory(storyID, snapshot.BranchID, true)
	if err != nil {
		return interactive.StoryMemoryState{}, patchCount, err
	}
	return state, patchCount, nil
}

func (a *App) CreateInteractiveMemory(storyID string, req interactive.InteractiveMemoryCreateRequest) (interactive.InteractiveMemoryEntry, error) {
	return a.interactiveService().CreateInteractiveMemory(storyID, req)
}

func (s *InteractiveAppService) CreateInteractiveMemory(storyID string, req interactive.InteractiveMemoryCreateRequest) (interactive.InteractiveMemoryEntry, error) {
	store := s.store()
	if store == nil {
		return interactive.InteractiveMemoryEntry{}, ErrNoWorkspace
	}
	return store.CreateInteractiveMemory(storyID, req)
}

func (a *App) UpdateInteractiveMemory(storyID, memoryID string, req interactive.InteractiveMemoryUpdateRequest) (interactive.InteractiveMemoryEntry, error) {
	return a.interactiveService().UpdateInteractiveMemory(storyID, memoryID, req)
}

func (s *InteractiveAppService) UpdateInteractiveMemory(storyID, memoryID string, req interactive.InteractiveMemoryUpdateRequest) (interactive.InteractiveMemoryEntry, error) {
	store := s.store()
	if store == nil {
		return interactive.InteractiveMemoryEntry{}, ErrNoWorkspace
	}
	return store.UpdateInteractiveMemory(storyID, memoryID, req)
}

func (a *App) SetInteractiveMemoryArchived(storyID, memoryID string, archived bool) (interactive.InteractiveMemoryEntry, error) {
	return a.interactiveService().SetInteractiveMemoryArchived(storyID, memoryID, archived)
}

func (s *InteractiveAppService) SetInteractiveMemoryArchived(storyID, memoryID string, archived bool) (interactive.InteractiveMemoryEntry, error) {
	store := s.store()
	if store == nil {
		return interactive.InteractiveMemoryEntry{}, ErrNoWorkspace
	}
	return store.SetInteractiveMemoryArchived(storyID, memoryID, archived)
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

// StartInteractiveTask 启动互动模式 Agent 任务，输出写回 interactive/story。
func (a *App) StartInteractiveTask(storyID, branchID, message string, styleReferences []string) *Task {
	return a.interactiveService().StartInteractiveTask(storyID, branchID, message, styleReferences)
}

func (s *InteractiveAppService) StartInteractiveTask(storyID, branchID, message string, styleReferences []string) *Task {
	return s.startInteractiveTask(storyID, branchID, message, styleReferences, "")
}

func (a *App) StartInteractiveRegenerateTask(storyID, branchID, turnID, message string, styleReferences []string) *Task {
	return a.interactiveService().StartInteractiveRegenerateTask(storyID, branchID, turnID, message, styleReferences)
}

func (s *InteractiveAppService) StartInteractiveRegenerateTask(storyID, branchID, turnID, message string, styleReferences []string) *Task {
	return s.startInteractiveTask(storyID, branchID, message, styleReferences, turnID)
}

func (a *App) AnalyzeInteractiveContext(storyID, branchID, message string, styleReferences []string) (agent.ContextAnalysis, error) {
	return a.interactiveService().AnalyzeInteractiveContext(storyID, branchID, message, styleReferences)
}

func (s *InteractiveAppService) AnalyzeInteractiveContext(storyID, branchID, message string, styleReferences []string) (agent.ContextAnalysis, error) {
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
	novaDir := runtimeCfg.NovaDir
	a.mu.RUnlock()

	if layered, err := config.LoadLayered(novaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
		runtimeCfg.InteractiveMaxTokens = appSettingsInt(layered.Effective.InteractiveMaxTokens, 0)
	} else {
		log.Printf("[interactive-agent-analysis] load interactive settings failed workspace=%s err=%v", workspace, err)
	}

	storyCtx, err := store.StoryContext(storyID, branchID)
	if err != nil {
		return agent.ContextAnalysis{}, err
	}
	teller := loadInteractiveTeller(novaDir, storyCtx.Meta.StoryTellerID)
	runtimeCfg.InteractiveReplyTargetChars = storyCtx.Meta.ReplyTargetChars
	var styleRules []agent.StyleRule
	if len(styleReferences) == 0 {
		styleRules = convertTellerStyleRules(novaDir, teller.StyleRules)
	}
	req := agent.ChatRequest{
		Message:         message,
		StyleReferences: styleReferences,
		StyleRules:      styleRules,
	}
	conversation := newInteractiveConversation(store, novaDir, workspace, storyID, branchID, message, runtimeCfg.InteractiveReplyTargetChars, &runtimeCfg)
	return agent.BuildInteractiveStoryContextAnalysis(&runtimeCfg, state, interactiveStoryTellerSystemInput(teller), bookService, req, conversation.PrepareMessages)
}

func (s *InteractiveAppService) startInteractiveTask(storyID, branchID, message string, styleReferences []string, rewindTurnID string) *Task {
	a := s.app
	a.mu.Lock()
	if a.interactive == nil || a.bookState == nil || a.cfg == nil {
		a.mu.Unlock()
		log.Printf("[interactive-agent-task] 未选择 workspace，无法启动任务")
		return nil
	}
	if a.activeInteractiveTask != nil && a.activeInteractiveTask.Status() == TaskRunning {
		log.Printf("[interactive-agent-task] replace running task id=%s", a.activeInteractiveTask.ID())
		a.activeInteractiveTask.Abort()
	}

	store := a.interactive
	state := a.bookState
	bookService := a.bookService
	chatService := a.chatService
	runtimeCfg := *a.cfg
	workspace := a.workspace
	runtimeCfg.Workspace = workspace
	novaDir := runtimeCfg.NovaDir
	a.mu.Unlock()

	if layered, err := config.LoadLayered(novaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
		runtimeCfg.InteractiveMaxTokens = appSettingsInt(layered.Effective.InteractiveMaxTokens, 0)
		log.Printf("[interactive-agent-task] load interactive settings max_tokens=%d workspace=%s", runtimeCfg.InteractiveMaxTokens, workspace)
	} else {
		log.Printf("[interactive-agent-task] load interactive settings failed workspace=%s err=%v", workspace, err)
	}

	storyCtx, err := store.StoryContext(storyID, branchID)
	if err != nil {
		log.Printf("[interactive-agent-task] 读取互动故事上下文失败 story_id=%s branch_id=%s err=%v", storyID, branchID, err)
		return nil
	}
	teller := loadInteractiveTeller(novaDir, storyCtx.Meta.StoryTellerID)
	runtimeCfg.InteractiveReplyTargetChars = storyCtx.Meta.ReplyTargetChars
	var styleRules []agent.StyleRule
	if len(styleReferences) == 0 {
		styleRules = convertTellerStyleRules(novaDir, teller.StyleRules)
		if len(styleRules) > 0 {
			log.Printf("[interactive-agent-task] inject teller style rules teller_id=%s count=%d rules=%q", teller.ID, len(styleRules), appStyleRuleNames(styleRules))
		}
	}
	log.Printf("[interactive-agent-task] use story settings story_id=%s teller_id=%s target_chars=%d style_rules=%d", storyID, teller.ID, runtimeCfg.InteractiveReplyTargetChars, len(styleRules))
	runner, err := buildInteractiveStoryRunner(context.Background(), &runtimeCfg, state, interactiveStoryTellerSystemInput(teller), agent.InteractiveStoryToolContext{
		Store:    store,
		StoryID:  storyID,
		BranchID: storyCtx.Snapshot.BranchID,
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

	if strings.TrimSpace(rewindTurnID) != "" {
		if err := store.RewindToTurnParent(storyID, interactive.RewindTurnRequest{BranchID: branchID, TurnID: rewindTurnID}); err != nil {
			log.Printf("[interactive-agent-task] 回退互动故事分支失败 story_id=%s branch_id=%s turn_id=%s err=%v", storyID, branchID, rewindTurnID, err)
			return nil
		}
		log.Printf("[interactive-agent-task] rewind branch for regeneration story_id=%s branch_id=%s turn_id=%s", storyID, branchID, rewindTurnID)
	}

	req := agent.ChatRequest{
		Message:         message,
		StyleReferences: styleReferences,
		StyleRules:      styleRules,
	}
	conversation := newInteractiveConversation(store, novaDir, workspace, storyID, branchID, message, runtimeCfg.InteractiveReplyTargetChars, &runtimeCfg)
	task := NewTask(func(ctx context.Context, task *Task, emit func(agent.Event)) {
		log.Printf("[interactive-agent-task] run begin id=%s story_id=%s branch_id=%s rewind_turn_id=%s message_len=%d style_references=%d", task.ID(), storyID, branchID, rewindTurnID, len(message), len(styleReferences))
		chatService.RunWithOptions(ctx, runner, conversation, bookService, req, agent.RunOptions{
			AgentKind:           agent.AgentKindInteractiveStory,
			TaskID:              task.ID(),
			Workspace:           workspace,
			Mode:                "interactive",
			OnMutationsVerified: a.automationMutationCallback("interactive_agent_post_run"),
		}, emit)
		if turn, stateReady, ok := conversation.LastTurnForState(); ok && !stateReady && ctx.Err() == nil {
			shouldGenerate, nextAuto, err := store.ShouldGenerateStoryMemory(storyID, turn.BranchID)
			if err != nil {
				log.Printf("[interactive-memory-agent] auto decision failed story_id=%s branch_id=%s turn_id=%s err=%v", storyID, turn.BranchID, turn.ID, err)
				markInteractiveStateFailed(conversation, turn, err)
			} else if shouldGenerate {
				log.Printf("[interactive-memory-agent] auto pending for stream story_id=%s branch_id=%s turn_id=%s", storyID, turn.BranchID, turn.ID)
			} else if err := store.MarkInteractiveMemoryReady(storyID, turn.BranchID, turn.ID); err != nil {
				log.Printf("[interactive-memory-agent] mark skipped turn ready failed story_id=%s branch_id=%s turn_id=%s err=%v", storyID, turn.BranchID, turn.ID, err)
				markInteractiveStateFailed(conversation, turn, err)
			} else {
				log.Printf("[interactive-memory-agent] auto skipped story_id=%s branch_id=%s turn_id=%s next_auto_in_turns=%d", storyID, turn.BranchID, turn.ID, nextAuto)
			}
		}
		log.Printf("[interactive-agent-task] run end id=%s status=%s", task.ID(), task.Status())
	})

	a.mu.Lock()
	a.activeInteractiveTask = task
	a.mu.Unlock()

	return task
}

func (a *App) InteractiveTellers() ([]interactive.Teller, error) {
	return a.interactiveService().InteractiveTellers()
}

func (s *InteractiveAppService) InteractiveTellers() ([]interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return nil, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.NovaDir).List()
}

func (a *App) InteractiveTeller(id string) (interactive.Teller, error) {
	return a.interactiveService().InteractiveTeller(id)
}

func (s *InteractiveAppService) InteractiveTeller(id string) (interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return interactive.Teller{}, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.NovaDir).Get(id)
}

func (a *App) CreateInteractiveTeller(teller interactive.Teller) (interactive.Teller, error) {
	return a.interactiveService().CreateInteractiveTeller(teller)
}

func (s *InteractiveAppService) CreateInteractiveTeller(teller interactive.Teller) (interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return interactive.Teller{}, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.NovaDir).Create(teller)
}

func (a *App) UpdateInteractiveTeller(id string, teller interactive.Teller) (interactive.Teller, error) {
	return a.interactiveService().UpdateInteractiveTeller(id, teller)
}

func (s *InteractiveAppService) UpdateInteractiveTeller(id string, teller interactive.Teller) (interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return interactive.Teller{}, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.NovaDir).Update(id, teller)
}

func (a *App) DeleteInteractiveTeller(id string) error {
	return a.interactiveService().DeleteInteractiveTeller(id)
}

func (s *InteractiveAppService) DeleteInteractiveTeller(id string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.NovaDir).Delete(id)
}

// ActiveInteractiveTask 返回当前互动模式活跃任务（可能为 nil）。
func (a *App) ActiveInteractiveTask() *Task {
	return a.interactiveService().ActiveInteractiveTask()
}

func (s *InteractiveAppService) ActiveInteractiveTask() *Task {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.activeInteractiveTask
}

// AbortInteractiveTask 终止当前互动模式活跃任务。
func (a *App) AbortInteractiveTask() {
	a.interactiveService().AbortInteractiveTask()
}

func (s *InteractiveAppService) AbortInteractiveTask() {
	a := s.app
	a.mu.RLock()
	task := a.activeInteractiveTask
	a.mu.RUnlock()
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

func (s *InteractiveAppService) cfg() *config.Config {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

func (s *InteractiveAppService) sessionStore() *session.Store {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionStore
}
