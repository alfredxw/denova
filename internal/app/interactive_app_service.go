package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	"nova/config"
	"nova/internal/agent"
	"nova/internal/imagepreset"
	"nova/internal/interactive"
	"nova/internal/session"
)

// InteractiveAppService 负责互动故事、剧情分支、导演和互动 Agent 任务。
type InteractiveAppService struct {
	app *App
}

const (
	storyMemoryGenerateSourceManual = "manual"
	storyMemoryGenerateSourceAuto   = "auto"
)

var generateInteractiveStateForStoryMemory = agent.GenerateInteractiveState

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
	state, _, err := s.runStoryMemoryGenerate(ctx, storyID, branchID, storyMemoryGenerateSourceManual, nil)
	return state, err
}

func (a *App) StartStoryMemoryGenerateTask(storyID, branchID, source string) *Task {
	return a.interactiveService().StartStoryMemoryGenerateTask(storyID, branchID, source)
}

func (s *InteractiveAppService) StartStoryMemoryGenerateTask(storyID, branchID, source string) *Task {
	source = normalizeStoryMemoryGenerateSource(source)
	return NewTask(func(ctx context.Context, task *Task, emit func(agent.Event)) {
		log.Printf("[interactive-memory-agent] stream begin task_id=%s story_id=%s branch_id=%s source=%s", task.ID(), storyID, branchID, source)
		emit(agent.Event{Type: "thinking", Data: map[string]string{"content": "正在读取当前剧情线和历史回合，准备整理故事记忆。"}})
		state, patchCount, err := s.runStoryMemoryGenerate(ctx, storyID, branchID, source, emit)
		if err != nil {
			log.Printf("[interactive-memory-agent] stream failed task_id=%s story_id=%s branch_id=%s source=%s err=%v", task.ID(), storyID, branchID, source, err)
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
		log.Printf("[interactive-memory-agent] stream done task_id=%s story_id=%s branch_id=%s source=%s patches=%d records=%d", task.ID(), storyID, state.BranchID, source, patchCount, len(state.Records))
	})
}

func (s *InteractiveAppService) runStoryMemoryGenerate(ctx context.Context, storyID, branchID, source string, emit func(agent.Event)) (interactive.StoryMemoryState, int, error) {
	source = normalizeStoryMemoryGenerateSource(source)
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
	generate := generateInteractiveStateForStoryMemory
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
		if source == storyMemoryGenerateSourceAuto {
			return skipAutoStoryMemoryGenerate(store, storyID, snapshot.BranchID, snapshot.CurrentTurn.ID, err, emit)
		}
		_ = store.MarkInteractiveMemoryFailed(storyID, interactive.MarkStateFailedRequest{ParentID: snapshot.CurrentTurn.ID, BranchID: snapshot.BranchID, Error: err.Error()})
		return interactive.StoryMemoryState{}, 0, err
	}
	if len(result.StateOps) > 0 && snapshot.CurrentTurn.StateStatus == "pending" {
		if _, err := store.AppendStateDelta(storyID, interactive.AppendStateDeltaRequest{
			ParentID: snapshot.CurrentTurn.ID,
			BranchID: snapshot.BranchID,
			Ops:      result.StateOps,
		}); err != nil {
			if source == storyMemoryGenerateSourceAuto {
				return skipAutoStoryMemoryGenerate(store, storyID, snapshot.BranchID, snapshot.CurrentTurn.ID, err, emit)
			}
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

func normalizeStoryMemoryGenerateSource(source string) string {
	if strings.TrimSpace(source) == storyMemoryGenerateSourceAuto {
		return storyMemoryGenerateSourceAuto
	}
	return storyMemoryGenerateSourceManual
}

func skipAutoStoryMemoryGenerate(store *interactive.Store, storyID, branchID, turnID string, cause error, emit func(agent.Event)) (interactive.StoryMemoryState, int, error) {
	log.Printf("[interactive-memory-agent] auto generate skipped story_id=%s branch_id=%s turn_id=%s err=%v", storyID, branchID, turnID, cause)
	if emit != nil {
		emit(agent.Event{Type: "thinking", Data: map[string]string{"content": "故事记忆自动整理暂时不可用，本回合已先跳过；你可以稍后手动重新整理。"}})
	}
	if err := store.MarkInteractiveMemoryReady(storyID, branchID, turnID); err != nil {
		return interactive.StoryMemoryState{}, 0, err
	}
	state, err := store.StoryMemory(storyID, branchID, true)
	if err != nil {
		return interactive.StoryMemoryState{}, 0, err
	}
	return state, 0, nil
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

// StartInteractiveTask 启动游戏模式 Agent 任务，输出写回 interactive/story。
func (a *App) StartInteractiveTask(storyID, branchID, message string, styleScenes []string, locale string) *Task {
	return a.interactiveService().StartInteractiveTask(storyID, branchID, message, styleScenes, locale)
}

func (s *InteractiveAppService) StartInteractiveTask(storyID, branchID, message string, styleScenes []string, locale string) *Task {
	return s.startInteractiveTask(storyID, branchID, message, styleScenes, "", locale)
}

func (a *App) StartInteractiveRegenerateTask(storyID, branchID, turnID, message string, styleScenes []string, locale string) *Task {
	return a.interactiveService().StartInteractiveRegenerateTask(storyID, branchID, turnID, message, styleScenes, locale)
}

func (s *InteractiveAppService) StartInteractiveRegenerateTask(storyID, branchID, turnID, message string, styleScenes []string, locale string) *Task {
	return s.startInteractiveTask(storyID, branchID, message, styleScenes, turnID, locale)
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
	novaDir := runtimeCfg.NovaDir
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
	styleRules := convertTellerStyleRules(teller.StyleRules, styleScenes)
	req := agent.ChatRequest{
		Message:     message,
		StyleScenes: styleScenes,
		StyleRules:  styleRules,
		Locale:      locale,
	}
	conversation := newInteractiveConversation(store, novaDir, workspace, storyID, branchID, message, runtimeCfg.InteractiveReplyTargetChars, &runtimeCfg)
	return agent.BuildInteractiveStoryContextAnalysis(&runtimeCfg, state, interactiveStoryTellerSystemInput(teller, styleRules), bookService, req, storyCtx.Snapshot.ContextCompaction, conversation.PrepareMessages)
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
	source, existingMemory := interactiveCompactionSource(storyCtx.Snapshot.Turns, storyCtx.Snapshot.ContextCompaction)
	referenceContext := interactiveCompactionReferenceContext(store, storyID, storyCtx.Snapshot.BranchID)
	epoch := 1
	if storyCtx.Snapshot.ContextCompaction != nil {
		epoch = storyCtx.Snapshot.ContextCompaction.Epoch + 1
	}
	_, result, err := agent.BuildContextCompaction(ctx, &runtimeCfg, config.AgentKindInteractiveStory, agent.ContextCompactionInput{
		Messages:         source,
		SourceMessages:   source,
		Phase:            "manual",
		Force:            true,
		ExistingMemory:   existingMemory,
		ReferenceContext: referenceContext,
		KeepLatestUser:   true,
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

func (s *InteractiveAppService) startInteractiveTask(storyID, branchID, message string, styleScenes []string, rewindTurnID string, locale string) *Task {
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
	styleRules := convertTellerStyleRules(teller.StyleRules, styleScenes)
	if len(styleRules) > 0 {
		log.Printf("[interactive-agent-task] inject teller style rules teller_id=%s scenes=%q count=%d rules=%q", teller.ID, styleScenes, len(styleRules), appStyleRuleNames(styleRules))
	}
	log.Printf("[interactive-agent-task] use story settings story_id=%s teller_id=%s target_chars=%d style_rules=%d", storyID, teller.ID, runtimeCfg.InteractiveReplyTargetChars, len(styleRules))
	tellerSystemInput := interactiveStoryTellerSystemInput(teller, styleRules)
	runner, err := buildInteractiveStoryRunner(context.Background(), &runtimeCfg, state, tellerSystemInput, agent.InteractiveStoryToolContext{
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
		Message:     message,
		StyleScenes: styleScenes,
		StyleRules:  styleRules,
		Locale:      locale,
	}
	conversation := newInteractiveConversation(store, novaDir, workspace, storyID, branchID, message, runtimeCfg.InteractiveReplyTargetChars, &runtimeCfg)
	task := NewTask(func(ctx context.Context, task *Task, emit func(agent.Event)) {
		log.Printf("[interactive-agent-task] run begin id=%s story_id=%s branch_id=%s rewind_turn_id=%s message_len=%d style_scenes=%d", task.ID(), storyID, branchID, rewindTurnID, len(message), len(styleScenes))
		chatService.RunWithOptions(ctx, runner, conversation, bookService, req, agent.RunOptions{
			AgentKind:           agent.AgentKindInteractiveStory,
			TaskID:              task.ID(),
			Workspace:           workspace,
			Mode:                "interactive",
			IdleTimeout:         agentIdleTimeout(runtimeCfg),
			ToolResultMaxBytes:  agentToolResultMaxBytes(runtimeCfg),
			SystemPromptLog:     agent.BuildInteractiveStoryInstructionComposition(&runtimeCfg, state, tellerSystemInput),
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

func (a *App) UpdateInteractiveTeller(id string, teller interactive.Teller, baseRevision ...string) (interactive.Teller, error) {
	return a.interactiveService().UpdateInteractiveTeller(id, teller, firstRevision(baseRevision))
}

func (s *InteractiveAppService) UpdateInteractiveTeller(id string, teller interactive.Teller, baseRevision string) (interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return interactive.Teller{}, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.NovaDir).Update(id, teller, baseRevision)
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

func (a *App) ImagePresets() ([]imagepreset.Preset, error) {
	return a.interactiveService().ImagePresets()
}

func (s *InteractiveAppService) ImagePresets() ([]imagepreset.Preset, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return nil, ErrNoWorkspace
	}
	return imagepreset.NewLibrary(cfg.NovaDir).List()
}

func (a *App) ImagePreset(id string) (imagepreset.Preset, error) {
	return a.interactiveService().ImagePreset(id)
}

func (s *InteractiveAppService) ImagePreset(id string) (imagepreset.Preset, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return imagepreset.Preset{}, ErrNoWorkspace
	}
	return imagepreset.NewLibrary(cfg.NovaDir).Get(id)
}

func (a *App) CreateImagePreset(preset imagepreset.Preset) (imagepreset.Preset, error) {
	return a.interactiveService().CreateImagePreset(preset)
}

func (s *InteractiveAppService) CreateImagePreset(preset imagepreset.Preset) (imagepreset.Preset, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return imagepreset.Preset{}, ErrNoWorkspace
	}
	return imagepreset.NewLibrary(cfg.NovaDir).Create(preset)
}

func (a *App) UpdateImagePreset(id string, preset imagepreset.Preset, baseRevision ...string) (imagepreset.Preset, error) {
	return a.interactiveService().UpdateImagePreset(id, preset, firstRevision(baseRevision))
}

func (s *InteractiveAppService) UpdateImagePreset(id string, preset imagepreset.Preset, baseRevision string) (imagepreset.Preset, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return imagepreset.Preset{}, ErrNoWorkspace
	}
	return imagepreset.NewLibrary(cfg.NovaDir).Update(id, preset, baseRevision)
}

func (a *App) DeleteImagePreset(id string) error {
	return a.interactiveService().DeleteImagePreset(id)
}

func (s *InteractiveAppService) DeleteImagePreset(id string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.NovaDir == "" {
		return ErrNoWorkspace
	}
	return imagepreset.NewLibrary(cfg.NovaDir).Delete(id)
}

// ActiveInteractiveTask 返回当前游戏模式活跃任务（可能为 nil）。
func (a *App) ActiveInteractiveTask() *Task {
	return a.interactiveService().ActiveInteractiveTask()
}

func (s *InteractiveAppService) ActiveInteractiveTask() *Task {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.activeInteractiveTask
}

// AbortInteractiveTask 终止当前游戏模式活跃任务。
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
	novaDir := runtimeCfg.NovaDir
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

func (s *InteractiveAppService) sessionStore() *session.Store {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionStore
}
