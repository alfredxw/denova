package app

import (
	"context"
	"crypto/sha256"
	agentinteractive "denova/internal/agents/interactive"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"denova/config"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/interactive/director"
)

type interactiveDirectorCommandDescriptor struct {
	Token           interactive.DirectorPlanRunToken `json:"token"`
	SourceTurnID    string                           `json:"source_turn_id"`
	MaintenanceTask string                           `json:"maintenance_task"`
}

// interactiveDirectorCommandID is fixed length but fully determined by the
// durable plan token and the exact maintenance operation it authorizes.
func interactiveDirectorCommandID(token interactive.DirectorPlanRunToken, sourceTurnID, maintenanceTask string) (string, error) {
	descriptor := interactiveDirectorCommandDescriptor{
		Token: token, SourceTurnID: strings.TrimSpace(sourceTurnID), MaintenanceTask: strings.TrimSpace(maintenanceTask),
	}
	data, err := json.Marshal(descriptor)
	if err != nil {
		return "", fmt.Errorf("serialize interactive Director command identity: %w", err)
	}
	digest := sha256.Sum256(data)
	return fmt.Sprintf("interactive-director:%x", digest[:]), nil
}

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
	done, started := tasks.GoKeyed(interactiveDerivedMaintenanceKey(conversation, turn.BranchID), func(ctx context.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err := fmt.Errorf("互动后台导演 Agent 异常中断: %v", recovered)
				storyID := ""
				if conversation != nil {
					storyID = conversation.storyID
				}
				slog.ErrorContext(ctx, fmt.Sprintf("[interactive-director-agent] maintenance panic recovered story_id=%s branch_id=%s turn_id=%s err=%v", storyID, turn.BranchID, turn.ID, err))
				markInteractiveDirectorMaintenanceFailed(conversation, turn, err)
			}
		}()

		if conversation == nil || conversation.store == nil || cfg == nil {
			return
		}
		if !runPlan {
			acknowledgeInteractiveDirectorDerivedTurn(conversation, turn)
			return
		}
		conversation.withDirectorTask(interactiveDirectorTaskDirectorPlanUpdate)
		if _, err := runInteractiveDirectorMaintenance(ctx, cfg, state, conversation, turn, sessionStore, interactiveDirectorTaskDirectorPlanUpdate); err != nil {
			slog.ErrorContext(ctx, fmt.Sprintf("[interactive-director-agent] plan maintenance failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err))
			return
		}
		acknowledgeInteractiveDirectorDerivedTurn(conversation, turn)
	})
	if !started {
		markInteractiveDirectorMaintenanceFailed(conversation, turn, context.Canceled)
	}
	return done
}

func acknowledgeInteractiveDirectorDerivedTurn(conversation *interactiveConversation, turn interactive.TurnEvent) {
	if conversation == nil || conversation.store == nil || strings.TrimSpace(turn.ID) == "" {
		return
	}
	if err := conversation.store.MarkDirectorTurnDerived(conversation.storyID, turn.BranchID, turn.ID); err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-director-agent] persist derived receipt failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err))
		return
	}
	slog.InfoContext(context.Background(), fmt.Sprintf("[interactive-director-agent] persisted derived receipt story_id=%s branch_id=%s turn_id=%s", conversation.storyID, turn.BranchID, turn.ID))
}

func prepareInteractiveDirectorBeforeOpening(ctx context.Context, cfg *config.Config, state *book.State, conversation *interactiveConversation, openingMessage string, sessionStore *session.Store) (bool, error) {
	if conversation == nil || conversation.store == nil || cfg == nil {
		return false, fmt.Errorf("互动导演开局规划上下文不完整")
	}
	storyCtx, err := conversation.store.StoryContext(conversation.storyID, conversation.branchID)
	if err != nil {
		return false, err
	}
	if interactiveSnapshotTurnCount(storyCtx.Snapshot) > 0 {
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
				slog.ErrorContext(ctx, fmt.Sprintf("[interactive-director-agent] panic recovered story_id=%s branch_id=%s turn_id=%s err=%v", storyID, turn.BranchID, turn.ID, err))
				markInteractiveDirectorFailed(conversation, turn, err)
			}
		}()

		if conversation == nil || conversation.store == nil || cfg == nil {
			return
		}
		if _, err := runInteractiveDirectorPlan(ctx, cfg, state, conversation, turn, sessionStore, prestartedTokens...); err != nil {
			slog.ErrorContext(ctx, fmt.Sprintf("[interactive-director-agent] run failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err))
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

func interactiveDerivedMaintenanceKey(conversation *interactiveConversation, branchID string) string {
	return interactiveBranchMaintenanceKey(conversation, branchID, "derived")
}

func directorTasksForConversation(conversation *interactiveConversation) *workspaceDirectorTaskGroup {
	if conversation != nil && conversation.directorTasks != nil {
		return conversation.directorTasks
	}
	// Director work must always be owned by the App workspace generation that
	// supplied its store/config. Creating a fallback group here would leave an
	// untracked goroutine able to write after workspace switch or App.Close.
	return nil
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
	commandID, err := interactiveDirectorCommandID(token, turn.ID, task)
	if err != nil {
		return interactiveDirectorMaintenanceResult{}, fmt.Errorf("派生互动导演命令标识失败: %w", err)
	}
	baselinePlan, err := conversation.store.DirectorPlan(conversation.storyID, turn.BranchID)
	if err != nil {
		return interactiveDirectorMaintenanceResult{}, fmt.Errorf("读取导演规划 Patch 基线失败: %w", err)
	}
	planDraft := interactive.NewDirectorPlanUpdateDraft(baselinePlan.Docs, token)
	effectiveTask := task
	slog.InfoContext(ctx, fmt.Sprintf("[interactive-director-agent] maintenance begin story_id=%s branch_id=%s turn_id=%s task=%s revision=%s", conversation.storyID, turn.BranchID, turn.ID, task, token.Revision))
	conversation.withDirectorTask(effectiveTask)
	stableContext, instruction, err := conversation.buildDirectorModelInput(turn)
	if err != nil {
		return interactiveDirectorMaintenanceResult{}, fmt.Errorf("构建后台导演指令失败: %w", err)
	}
	loreSourceRevision := stableContext.Revision
	result := interactiveDirectorMaintenanceResult{}
	var planSubmissionMu sync.Mutex
	reviewedLoreIDs := map[string]bool{}
	planCommit := newInteractiveDirectorPlanCommit(
		conversation.store, conversation.storyID, turn.BranchID, turn.ID, token, planDraft, &planSubmissionMu,
	)
	generator := conversation.directorGenerator
	if generator == nil {
		err = fmt.Errorf("互动导演缺少 App-owned durable runtime")
		persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, "执行失败："+err.Error())
		markInteractiveDirectorFailed(conversation, turn, err)
		return result, err
	}
	_, err = generator(ctx, cfg, state, agentinteractive.InteractiveStoryToolContext{
		Store:                   conversation.store,
		CommandID:               commandID,
		StoryID:                 conversation.storyID,
		BranchID:                turn.BranchID,
		TurnID:                  turn.ID,
		MaintenanceTask:         effectiveTask,
		StableContextTitle:      stableContext.Title,
		StableContext:           stableContext.Content,
		StableContextMaxBytes:   stableContext.MaxBytes,
		DisplayConversation:     conversation,
		DomainCommitParticipant: planCommit,
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
			return conversation.store.StageDirectorPlanRunUpdate(conversation.storyID, turn.BranchID, token, turn.ID, planDraft, submission)
		},
	}, instruction)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		if committedPlan, persistedOutput, committed := planCommit.committedResult(); committed {
			// The canonical output commit won before a late transport/runtime
			// error. Never replace its durable receipt with a failed projection.
			result.Plan = committedPlan
			persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, persistedOutput)
			slog.ErrorContext(ctx, fmt.Sprintf("[interactive-director-agent] reconciled committed plan after late runtime error story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err))
			return result, nil
		}
		if committedPlan, persistedOutput, committed, reconcileErr := interactiveDirectorCanonicalResult(
			conversation.store, conversation.storyID, turn.BranchID, commandID,
		); reconcileErr == nil && committed {
			result.Plan = committedPlan
			persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, persistedOutput)
			slog.ErrorContext(ctx, fmt.Sprintf("[interactive-director-agent] recovered canonical plan after replay error story_id=%s branch_id=%s turn_id=%s command_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, commandID, err))
			return result, nil
		}
		persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, "执行失败："+err.Error())
		if runPlan {
			markInteractiveDirectorFailed(conversation, turn, err)
		}
		return result, fmt.Errorf("生成后台导演维护失败: %w", err)
	}
	if conversation.customDirectorGenerator || conversation.directorChatService == nil {
		if err = planCommit.commitCustomGenerator(ctx); err != nil {
			persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, "执行失败："+err.Error())
			markInteractiveDirectorFailed(conversation, turn, err)
			return result, err
		}
	}
	committedPlan, persistedOutput, committed := planCommit.committedResult()
	if !committed {
		committedPlan, persistedOutput, committed, err = interactiveDirectorCanonicalResult(
			conversation.store, conversation.storyID, turn.BranchID, commandID,
		)
		if err != nil {
			return result, fmt.Errorf("读取互动导演 canonical output commit 失败: %w", err)
		}
		if !committed {
			err = fmt.Errorf("互动导演 durable runtime 未返回 canonical output commit receipt")
			persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, "执行失败："+err.Error())
			markInteractiveDirectorFailed(conversation, turn, err)
			return result, err
		}
	}
	result.Plan = committedPlan
	persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, persistedOutput)
	status := ""
	if result.Plan.Metadata.LastRun != nil {
		status = result.Plan.Metadata.LastRun.Status
	}
	slog.InfoContext(ctx, fmt.Sprintf("[interactive-director-agent] maintenance done story_id=%s branch_id=%s turn_id=%s task=%s director_status=%s summary=%q", conversation.storyID, turn.BranchID, turn.ID, task, status, strings.TrimSpace(persistedOutput)))
	return result, nil
}

func interactiveDirectorCanonicalResult(store *interactive.Store, storyID, branchID, commandID string) (interactive.DirectorPlan, string, bool, error) {
	if store == nil || strings.TrimSpace(commandID) == "" {
		return interactive.DirectorPlan{}, "", false, nil
	}
	plan, err := store.DirectorPlan(storyID, branchID)
	if err != nil {
		return interactive.DirectorPlan{}, "", false, err
	}
	run := plan.Metadata.LastRun
	if run == nil || run.DomainCommit == nil || strings.TrimSpace(run.DomainCommit.Identity.CommandID) != strings.TrimSpace(commandID) {
		return interactive.DirectorPlan{}, "", false, nil
	}
	return plan, strings.TrimSpace(run.Summary), true, nil
}

func markInteractiveDirectorFailed(conversation *interactiveConversation, turn interactive.TurnEvent, err error) {
	if conversation == nil || conversation.store == nil || err == nil {
		return
	}
	if markErr := conversation.store.MarkDirectorPlanRunFailed(conversation.storyID, turn.BranchID, turn.ID, err); markErr != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-director-agent] mark failed director run failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, markErr))
	}
}

func markInteractiveDirectorMaintenanceFailed(conversation *interactiveConversation, turn interactive.TurnEvent, err error) {
	markInteractiveDirectorFailed(conversation, turn, err)
}

func shouldRunInteractiveDirectorAgent(strategy interactive.StoryDirectorStrategy) director.ScheduleDecision {
	strategy = interactive.NormalizeStoryDirectorStrategy(strategy)
	if !strategy.Enabled {
		return director.ScheduleDecision{Reason: "disabled"}
	}
	return director.ScheduleDecision{ShouldRun: true, Reason: "after_persisted_turn"}
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
