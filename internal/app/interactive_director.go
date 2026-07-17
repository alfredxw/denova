package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/session"
)

const (
	interactiveDirectorTaskDirectorPlanUpdate = "director_plan_update"
	interactiveDirectorTaskOpeningPlan        = "opening_plan"
	interactiveDirectorOpeningSourceID        = "story_opening"
)

type interactiveDirectorMaintenanceResult struct {
	Plan interactive.DirectorPlan
}

func startInteractiveDirectorMaintenanceTask(cfg *config.Config, state *book.State, conversation *interactiveConversation, turn interactive.TurnEvent, sessionStore *session.Store, runPlan bool) <-chan struct{} {
	tasks := directorTasksForConversation(conversation)
	schemaDone := startInteractiveStateSchemaTask(cfg, state, conversation, turn, sessionStore)
	done, started := tasks.GoKeyed(interactiveDerivedMaintenanceKey(conversation, turn.BranchID), func(ctx context.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err := fmt.Errorf("互动后台导演 Agent 异常中断: %v", recovered)
				storyID := ""
				if conversation != nil {
					storyID = conversation.storyID
				}
				log.Printf("[interactive-director-agent] maintenance panic recovered story_id=%s branch_id=%s turn_id=%s err=%v", storyID, turn.BranchID, turn.ID, err)
				markInteractiveDirectorMaintenanceFailed(conversation, turn, err)
			}
		}()

		if conversation == nil || conversation.store == nil || cfg == nil {
			return
		}
		select {
		case <-schemaDone:
		case <-ctx.Done():
			return
		}
		if !runPlan {
			return
		}
		conversation.withDirectorTask(interactiveDirectorTaskDirectorPlanUpdate)
		if _, err := runInteractiveDirectorMaintenance(ctx, cfg, state, conversation, turn, sessionStore, interactiveDirectorTaskDirectorPlanUpdate); err != nil {
			log.Printf("[interactive-director-agent] plan maintenance failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
		}
	})
	if !started {
		markInteractiveDirectorMaintenanceFailed(conversation, turn, context.Canceled)
	}
	return done
}

func prepareInteractiveDirectorBeforeOpening(ctx context.Context, cfg *config.Config, state *book.State, conversation *interactiveConversation, openingMessage string, sessionStore *session.Store) (bool, error) {
	if conversation == nil || conversation.store == nil || cfg == nil {
		return false, fmt.Errorf("互动导演开局规划上下文不完整")
	}
	storyCtx, err := conversation.store.StoryContext(conversation.storyID, conversation.branchID)
	if err != nil {
		return false, err
	}
	if len(storyCtx.Snapshot.Turns) > 0 {
		return false, nil
	}
	status, err := conversation.store.DirectorPlanStatus(conversation.storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return false, err
	}
	if status.StartReady {
		return true, nil
	}
	openingContext := firstNonEmptyApp(
		openingMessage,
		storyCtx.Meta.Opening.CustomText,
		storyCtx.Meta.Opening.PresetText,
		storyCtx.Meta.Origin,
		storyCtx.Meta.Title,
	)
	turn := interactive.TurnEvent{
		V:        1,
		Type:     "director_opening",
		ID:       interactiveDirectorOpeningSourceID,
		BranchID: storyCtx.Snapshot.BranchID,
		User:     openingContext,
	}
	conversation.withDirectorTask(interactiveDirectorTaskOpeningPlan)
	if _, err := runInteractiveDirectorMaintenance(ctx, cfg, state, conversation, turn, sessionStore, interactiveDirectorTaskOpeningPlan); err != nil {
		return true, err
	}
	status, err = conversation.store.DirectorPlanStatus(conversation.storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return true, err
	}
	if !status.StartReady {
		return true, fmt.Errorf("开局导演规划未完成: %s", status.Status)
	}
	return true, nil
}

func startInteractiveStateSchemaTask(cfg *config.Config, state *book.State, conversation *interactiveConversation, turn interactive.TurnEvent, sessionStore *session.Store) <-chan struct{} {
	tasks := directorTasksForConversation(conversation)
	done, started := tasks.GoKeyed(interactiveStateSchemaMaintenanceKey(conversation, turn.BranchID), func(ctx context.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err := fmt.Errorf("状态结构初始化异常中断: %v", recovered)
				log.Printf("[interactive-state-schema] panic recovered story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
				_ = conversation.store.MarkStateSchemaInitializationFailed(conversation.storyID, turn.ID, err)
			}
		}()
		if err := runInteractiveStateSchemaInitialization(ctx, cfg, state, conversation, turn, sessionStore); err != nil {
			log.Printf("[interactive-state-schema] manual initialization failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
		}
	})
	if !started {
		_ = conversation.store.MarkStateSchemaInitializationFailed(conversation.storyID, turn.ID, context.Canceled)
	}
	return done
}

func startInteractiveDirectorTask(cfg *config.Config, state *book.State, conversation *interactiveConversation, turn interactive.TurnEvent, sessionStore *session.Store, prestartedTokens ...interactive.DirectorPlanRunToken) <-chan struct{} {
	tasks := directorTasksForConversation(conversation)
	done, started := tasks.GoKeyed(interactiveDerivedMaintenanceKey(conversation, turn.BranchID), func(ctx context.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err := fmt.Errorf("互动导演 Agent 异常中断: %v", recovered)
				storyID := ""
				if conversation != nil {
					storyID = conversation.storyID
				}
				log.Printf("[interactive-director-agent] panic recovered story_id=%s branch_id=%s turn_id=%s err=%v", storyID, turn.BranchID, turn.ID, err)
				markInteractiveDirectorFailed(conversation, turn, err)
			}
		}()

		if conversation == nil || conversation.store == nil || cfg == nil {
			return
		}
		if _, err := runInteractiveDirectorPlan(ctx, cfg, state, conversation, turn, sessionStore, prestartedTokens...); err != nil {
			log.Printf("[interactive-director-agent] run failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
			markInteractiveDirectorFailed(conversation, turn, err)
			return
		}
	})
	if !started {
		markInteractiveDirectorFailed(conversation, turn, context.Canceled)
	}
	return done
}

func interactiveBranchMaintenanceKey(conversation *interactiveConversation, branchID, lane string) string {
	storyID := ""
	if conversation != nil {
		storyID = strings.TrimSpace(conversation.storyID)
	}
	return storyID + ":" + strings.TrimSpace(branchID) + ":" + lane
}

func interactiveStateSchemaMaintenanceKey(conversation *interactiveConversation, branchID string) string {
	return interactiveBranchMaintenanceKey(conversation, branchID, "state_schema")
}

func interactiveDerivedMaintenanceKey(conversation *interactiveConversation, branchID string) string {
	return interactiveBranchMaintenanceKey(conversation, branchID, "derived")
}

func directorTasksForConversation(conversation *interactiveConversation) *workspaceDirectorTaskGroup {
	if conversation != nil && conversation.directorTasks != nil {
		return conversation.directorTasks
	}
	tasks := newWorkspaceDirectorTaskGroup()
	if conversation != nil {
		conversation.directorTasks = tasks
	}
	return tasks
}

func runInteractiveDirectorPlan(ctx context.Context, cfg *config.Config, state *book.State, conversation *interactiveConversation, turn interactive.TurnEvent, sessionStore *session.Store, prestartedTokens ...interactive.DirectorPlanRunToken) (interactive.DirectorPlan, error) {
	result, err := runInteractiveDirectorMaintenance(ctx, cfg, state, conversation, turn, sessionStore, interactiveDirectorTaskDirectorPlanUpdate, prestartedTokens...)
	return result.Plan, err
}

func runInteractiveDirectorMaintenance(ctx context.Context, cfg *config.Config, state *book.State, conversation *interactiveConversation, turn interactive.TurnEvent, sessionStore *session.Store, task string, prestartedTokens ...interactive.DirectorPlanRunToken) (interactiveDirectorMaintenanceResult, error) {
	if conversation == nil || conversation.store == nil || cfg == nil {
		return interactiveDirectorMaintenanceResult{}, fmt.Errorf("互动导演运行上下文不完整")
	}
	task = strings.TrimSpace(task)
	if task == "" {
		task = interactiveDirectorTaskDirectorPlanUpdate
	}
	switch task {
	case interactiveDirectorTaskDirectorPlanUpdate, interactiveDirectorTaskOpeningPlan:
	default:
		return interactiveDirectorMaintenanceResult{}, fmt.Errorf("未知互动导演任务: %s", task)
	}
	runPlan := true
	storyCtx, err := conversation.store.StoryContext(conversation.storyID, turn.BranchID)
	if err != nil {
		return interactiveDirectorMaintenanceResult{}, err
	}
	director := conversation.storyDirectorForMeta(storyCtx.Meta)
	decision := shouldRunInteractiveDirectorAgent(director.Strategy)
	if runPlan && !decision.ShouldRun {
		if err := conversation.store.MarkDirectorPlanRunSkipped(conversation.storyID, turn.BranchID, turn.ID, decision.Reason); err != nil {
			return interactiveDirectorMaintenanceResult{}, err
		}
		runPlan = false
		return interactiveDirectorMaintenanceResult{}, nil
	}
	var token interactive.DirectorPlanRunToken
	if runPlan {
		if len(prestartedTokens) > 0 && prestartedTokens[0].Revision != "" {
			token = prestartedTokens[0]
		} else {
			token, err = conversation.store.DirectorPlanRunToken(conversation.storyID, turn.BranchID)
			if err != nil {
				return interactiveDirectorMaintenanceResult{}, fmt.Errorf("准备导演规划运行版本失败: %w", err)
			}
			if err := conversation.store.MarkDirectorPlanRunStarted(conversation.storyID, turn.BranchID, token, turn.ID); err != nil {
				return interactiveDirectorMaintenanceResult{}, fmt.Errorf("标记导演规划运行状态失败: %w", err)
			}
		}
	}
	baselinePlan, err := conversation.store.DirectorPlan(conversation.storyID, turn.BranchID)
	if err != nil {
		return interactiveDirectorMaintenanceResult{}, fmt.Errorf("读取导演规划 Patch 基线失败: %w", err)
	}
	planDraft := interactive.NewDirectorPlanUpdateDraft(baselinePlan.Docs, token)
	effectiveTask := task
	log.Printf("[interactive-director-agent] maintenance begin story_id=%s branch_id=%s turn_id=%s task=%s revision=%s", conversation.storyID, turn.BranchID, turn.ID, task, token.Revision)
	conversation.withDirectorTask(effectiveTask)
	stableContext, instruction, err := conversation.buildDirectorModelInput(turn)
	if err != nil {
		return interactiveDirectorMaintenanceResult{}, fmt.Errorf("构建后台导演指令失败: %w", err)
	}
	loreSourceRevision := stableContext.Revision
	result := interactiveDirectorMaintenanceResult{}
	var planSubmissionMu sync.Mutex
	var submittedPlanDecision interactive.PlanDecision
	planFinalized := false
	reviewedLoreIDs := map[string]bool{}
	generator := conversation.directorGenerator
	if generator == nil {
		generator = generateInteractiveDirector
	}
	output, err := generator(ctx, cfg, state, agent.InteractiveStoryToolContext{
		Store:                 conversation.store,
		StoryID:               conversation.storyID,
		BranchID:              turn.BranchID,
		TurnID:                turn.ID,
		MaintenanceTask:       effectiveTask,
		StableContextTitle:    stableContext.Title,
		StableContext:         stableContext.Content,
		StableContextMaxBytes: stableContext.MaxBytes,
		DisplayConversation:   conversation,
		OnLoreItemsRead: func(ids []string) {
			planSubmissionMu.Lock()
			defer planSubmissionMu.Unlock()
			for _, id := range ids {
				if id = strings.TrimSpace(id); id != "" {
					reviewedLoreIDs[id] = true
				}
			}
		},
		SubmitDirectorPlanUpdate: func(callCtx context.Context, submission interactive.DirectorPlanUpdateSubmission) (interactive.DirectorPlanUpdateReceipt, error) {
			if !runPlan {
				return interactive.DirectorPlanUpdateReceipt{}, fmt.Errorf("当前维护阶段不允许提交导演规划")
			}
			if err := callCtx.Err(); err != nil {
				return interactive.DirectorPlanUpdateReceipt{}, err
			}
			planSubmissionMu.Lock()
			defer planSubmissionMu.Unlock()
			submission.SourceLoreRevision = loreSourceRevision
			submission.ReviewedLoreIDs = make([]string, 0, len(reviewedLoreIDs))
			for id := range reviewedLoreIDs {
				submission.ReviewedLoreIDs = append(submission.ReviewedLoreIDs, id)
			}
			receipt, err := conversation.store.StageDirectorPlanRunUpdate(conversation.storyID, turn.BranchID, token, turn.ID, planDraft, submission)
			if err != nil {
				return interactive.DirectorPlanUpdateReceipt{}, err
			}
			if receipt.Finalized {
				planFinalized = true
				submittedPlanDecision = receipt.Decision
			}
			return receipt, nil
		},
	}, instruction)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, "执行失败："+err.Error())
		if runPlan {
			markInteractiveDirectorFailed(conversation, turn, err)
		}
		return result, fmt.Errorf("生成后台导演维护失败: %w", err)
	}
	persistedOutput := output
	if runPlan {
		planSubmissionMu.Lock()
		decision := submittedPlanDecision
		finalized := planFinalized
		planSubmissionMu.Unlock()
		if !finalized {
			err = fmt.Errorf("导演规划未通过 submit_director_plan_update finalize Patch 草稿")
			persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, "执行失败："+err.Error())
			markInteractiveDirectorFailed(conversation, turn, err)
			return result, err
		}
		normalizedOutput, marshalErr := json.Marshal(decision)
		if marshalErr != nil {
			err = fmt.Errorf("序列化导演规划决策失败: %w", marshalErr)
			persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, "执行失败："+err.Error())
			markInteractiveDirectorFailed(conversation, turn, err)
			return result, err
		}
		persistedOutput = string(normalizedOutput)
	}
	persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, persistedOutput)
	if runPlan {
		finalDocs, finalized := planDraft.FinalDocs()
		if !finalized {
			err = fmt.Errorf("导演规划 Patch 草稿尚未 finalize")
			markInteractiveDirectorFailed(conversation, turn, err)
			return result, err
		}
		plan, err := conversation.store.CompleteDirectorPlanRunWithDocs(conversation.storyID, turn.BranchID, token, turn.ID, persistedOutput, finalDocs)
		if err != nil {
			markInteractiveDirectorFailed(conversation, turn, err)
			return result, fmt.Errorf("完成导演规划运行失败: %w", err)
		}
		result.Plan = plan
	}
	status := ""
	if result.Plan.Metadata.LastRun != nil {
		status = result.Plan.Metadata.LastRun.Status
	}
	log.Printf("[interactive-director-agent] maintenance done story_id=%s branch_id=%s turn_id=%s task=%s director_status=%s summary=%q", conversation.storyID, turn.BranchID, turn.ID, task, status, strings.TrimSpace(persistedOutput))
	return result, nil
}

func markInteractiveDirectorFailed(conversation *interactiveConversation, turn interactive.TurnEvent, err error) {
	if conversation == nil || conversation.store == nil || err == nil {
		return
	}
	if markErr := conversation.store.MarkDirectorPlanRunFailed(conversation.storyID, turn.BranchID, turn.ID, err); markErr != nil {
		log.Printf("[interactive-director-agent] mark failed director run failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, markErr)
	}
}

func markInteractiveDirectorMaintenanceFailed(conversation *interactiveConversation, turn interactive.TurnEvent, err error) {
	markInteractiveDirectorFailed(conversation, turn, err)
}

func shouldRunInteractiveDirectorAgent(strategy interactive.StoryDirectorStrategy) interactive.DirectorAgentScheduleDecision {
	strategy = interactive.NormalizeStoryDirectorStrategy(strategy)
	if !strategy.Enabled {
		return interactive.DirectorAgentScheduleDecision{Reason: "disabled"}
	}
	if strategy.DirectorAgentMode == interactive.DirectorAgentModeOff {
		return interactive.DirectorAgentScheduleDecision{Reason: "mode_off"}
	}
	return interactive.DirectorAgentScheduleDecision{ShouldRun: true, Reason: "after_persisted_turn"}
}

// shouldScheduleInteractiveDirectorAfterTurn is the low-cost gate for normal
// Game turns. The already-running Game Agent reports material planning impact;
// the Director is not started merely to decide that the plan can be kept.
func shouldScheduleInteractiveDirectorAfterTurn(strategy interactive.StoryDirectorStrategy, turn interactive.TurnEvent) interactive.DirectorAgentScheduleDecision {
	strategy = interactive.NormalizeStoryDirectorStrategy(strategy)
	if !strategy.Enabled {
		return interactive.DirectorAgentScheduleDecision{Reason: "disabled"}
	}
	switch strategy.DirectorAgentMode {
	case interactive.DirectorAgentModeOff:
		return interactive.DirectorAgentScheduleDecision{Reason: "mode_off"}
	case interactive.DirectorAgentModeEveryTurn:
		return interactive.DirectorAgentScheduleDecision{ShouldRun: true, Reason: "every_turn"}
	}
	if turn.TurnResult == nil || turn.TurnResult.DirectorUpdate == nil || !turn.TurnResult.DirectorUpdate.Needed {
		return interactive.DirectorAgentScheduleDecision{Reason: "no_material_update"}
	}
	return interactive.DirectorAgentScheduleDecision{ShouldRun: true, Reason: "game_agent_update"}
}

func firstNonEmptyApp(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
