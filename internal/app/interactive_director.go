package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/session"
)

const (
	interactiveDirectorTaskTurnMaintenance    = "turn_maintenance"
	interactiveDirectorTaskMemoryUpdate       = "memory_update"
	interactiveDirectorTaskDirectorPlanUpdate = "director_plan_update"
)

type interactiveDirectorMaintenanceResult struct {
	Plan                      interactive.DirectorPlan
	AppliedStoryMemoryPatches int
}

func startInteractiveDirectorMaintenanceTask(cfg *config.Config, state *book.State, conversation *interactiveConversation, turn interactive.TurnEvent, sessionStore *session.Store) <-chan struct{} {
	tasks := directorTasksForConversation(conversation)
	done, started := tasks.GoKeyed(interactiveMaintenanceKey(conversation, turn.BranchID), func(ctx context.Context) {
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
		shouldRecordMemory, _, err := conversation.store.ShouldGenerateStoryMemory(conversation.storyID, turn.BranchID)
		if err != nil {
			markInteractiveMemoryFailed(conversation, turn, err)
			log.Printf("[interactive-memory-recorder] schedule check failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
		} else if !shouldRecordMemory {
			if err := conversation.store.MarkInteractiveMemoryReady(conversation.storyID, turn.BranchID, turn.ID); err != nil {
				markInteractiveMemoryFailed(conversation, turn, err)
			}
		} else if claimed, claimErr := conversation.store.ClaimInteractiveMemoryRun(conversation.storyID, turn.BranchID, turn.ID, false); claimErr != nil {
			markInteractiveMemoryFailed(conversation, turn, claimErr)
			log.Printf("[interactive-memory-recorder] claim failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, claimErr)
		} else if claimed {
			conversation.withDirectorTask(interactiveDirectorTaskMemoryUpdate)
			if _, err := runInteractiveDirectorMaintenance(ctx, cfg, state, conversation, turn, sessionStore, interactiveDirectorTaskMemoryUpdate); err != nil {
				log.Printf("[interactive-memory-recorder] maintenance failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
			}
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

func startInteractiveDirectorTask(cfg *config.Config, state *book.State, conversation *interactiveConversation, turn interactive.TurnEvent, sessionStore *session.Store, prestartedTokens ...interactive.DirectorPlanRunToken) <-chan struct{} {
	tasks := directorTasksForConversation(conversation)
	done, started := tasks.GoKeyed(interactiveMaintenanceKey(conversation, turn.BranchID), func(ctx context.Context) {
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

func interactiveMaintenanceKey(conversation *interactiveConversation, branchID string) string {
	storyID := ""
	if conversation != nil {
		storyID = strings.TrimSpace(conversation.storyID)
	}
	return storyID + ":" + strings.TrimSpace(branchID)
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
		task = interactiveDirectorTaskTurnMaintenance
	}
	if task == interactiveDirectorTaskTurnMaintenance {
		memoryResult, memoryErr := runInteractiveDirectorMaintenance(ctx, cfg, state, conversation.withDirectorTask(interactiveDirectorTaskMemoryUpdate), turn, sessionStore, interactiveDirectorTaskMemoryUpdate)
		planResult, planErr := runInteractiveDirectorMaintenance(ctx, cfg, state, conversation.withDirectorTask(interactiveDirectorTaskDirectorPlanUpdate), turn, sessionStore, interactiveDirectorTaskDirectorPlanUpdate, prestartedTokens...)
		return interactiveDirectorMaintenanceResult{
			Plan:                      planResult.Plan,
			AppliedStoryMemoryPatches: memoryResult.AppliedStoryMemoryPatches,
		}, errors.Join(memoryErr, planErr)
	}
	runMemory := task != interactiveDirectorTaskDirectorPlanUpdate
	runPlan := task != interactiveDirectorTaskMemoryUpdate
	storyCtx, err := conversation.store.StoryContext(conversation.storyID, turn.BranchID)
	if err != nil {
		return interactiveDirectorMaintenanceResult{}, err
	}
	director := conversation.storyDirector(storyCtx.Meta.StoryDirectorID)
	decision := shouldRunInteractiveDirectorAgent(director.Strategy)
	if runPlan && !decision.ShouldRun {
		if err := conversation.store.MarkDirectorPlanRunSkipped(conversation.storyID, turn.BranchID, turn.ID, decision.Reason); err != nil {
			return interactiveDirectorMaintenanceResult{}, err
		}
		runPlan = false
		if !runMemory {
			return interactiveDirectorMaintenanceResult{}, nil
		}
	}
	var token interactive.DirectorPlanRunToken
	var allowedPaths []string
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
		allowedPaths = conversation.store.DirectorPlanAllowedPaths(conversation.storyID, turn.BranchID)
	}
	effectiveTask := task
	if runMemory && !runPlan {
		effectiveTask = interactiveDirectorTaskMemoryUpdate
	} else if runPlan && !runMemory {
		effectiveTask = interactiveDirectorTaskDirectorPlanUpdate
	} else if runMemory && runPlan {
		effectiveTask = interactiveDirectorTaskTurnMaintenance
	}
	log.Printf("[interactive-director-agent] maintenance begin story_id=%s branch_id=%s turn_id=%s task=%s effective_task=%s memory=%t plan=%t revision=%s allowed_paths=%d", conversation.storyID, turn.BranchID, turn.ID, task, effectiveTask, runMemory, runPlan, token.Revision, len(allowedPaths))
	conversation.withDirectorTask(effectiveTask)
	instruction, err := conversation.BuildDirectorInstruction(turn)
	if err != nil {
		return interactiveDirectorMaintenanceResult{}, fmt.Errorf("构建后台导演指令失败: %w", err)
	}
	result := interactiveDirectorMaintenanceResult{}
	var memoryMaintenanceErr error
	generator := conversation.directorGenerator
	if generator == nil {
		generator = generateInteractiveDirector
	}
	output, err := generator(ctx, cfg, state, agent.InteractiveStoryToolContext{
		Store:                    conversation.store,
		StoryID:                  conversation.storyID,
		BranchID:                 turn.BranchID,
		TurnID:                   turn.ID,
		MaintenanceTask:          effectiveTask,
		DirectorPlanAllowedPaths: allowedPaths,
		DisplayConversation:      conversation,
		OnStoryMemoryApplied: func(applied int) {
			result.AppliedStoryMemoryPatches += applied
		},
		OnStateMaintenanceFailed: func(err error) {
			if err != nil {
				memoryMaintenanceErr = errors.Join(memoryMaintenanceErr, err)
			}
		},
	}, instruction)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, "执行失败："+err.Error())
		if runMemory {
			if memoryMaintenanceErr == nil && result.AppliedStoryMemoryPatches > 0 {
				if readyErr := conversation.store.MarkInteractiveMemoryRunReady(conversation.storyID, turn.BranchID, turn.ID); readyErr != nil {
					markInteractiveMemoryFailed(conversation, turn, readyErr)
				}
			} else {
				markInteractiveMemoryFailed(conversation, turn, errors.Join(memoryMaintenanceErr, err))
			}
		}
		if runPlan {
			markInteractiveDirectorFailed(conversation, turn, err)
		}
		return result, fmt.Errorf("生成后台导演维护失败: %w", err)
	}
	persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, output)
	var errs []error
	if runPlan {
		plan, err := conversation.store.CompleteDirectorPlanRun(conversation.storyID, turn.BranchID, token, turn.ID, strings.TrimSpace(output))
		if err != nil {
			errs = append(errs, fmt.Errorf("完成导演规划运行失败: %w", err))
		} else {
			result.Plan = plan
		}
	}
	if runMemory {
		if memoryMaintenanceErr != nil {
			markInteractiveMemoryFailed(conversation, turn, memoryMaintenanceErr)
			errs = append(errs, fmt.Errorf("故事记忆或状态系统工具失败: %w", memoryMaintenanceErr))
		} else if err := conversation.store.MarkInteractiveMemoryRunReady(conversation.storyID, turn.BranchID, turn.ID); err != nil {
			markInteractiveMemoryFailed(conversation, turn, err)
			errs = append(errs, fmt.Errorf("标记故事记忆完成失败: %w", err))
		}
	}
	if len(errs) > 0 {
		return result, errors.Join(errs...)
	}
	status := ""
	if result.Plan.Metadata.LastRun != nil {
		status = result.Plan.Metadata.LastRun.Status
	}
	log.Printf("[interactive-director-agent] maintenance done story_id=%s branch_id=%s turn_id=%s task=%s effective_task=%s memory_patches=%d director_status=%s summary=%q", conversation.storyID, turn.BranchID, turn.ID, task, effectiveTask, result.AppliedStoryMemoryPatches, status, strings.TrimSpace(output))
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

func markInteractiveMemoryFailed(conversation *interactiveConversation, turn interactive.TurnEvent, err error) {
	if conversation == nil || conversation.store == nil || err == nil {
		return
	}
	if markErr := conversation.store.MarkInteractiveMemoryFailed(conversation.storyID, interactive.MarkStateFailedRequest{
		ParentID: turn.ID,
		BranchID: turn.BranchID,
		Error:    err.Error(),
	}); markErr != nil {
		log.Printf("[interactive-director-agent] mark failed memory failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, markErr)
	}
}

func markInteractiveDirectorMaintenanceFailed(conversation *interactiveConversation, turn interactive.TurnEvent, err error) {
	if conversation != nil && strings.TrimSpace(conversation.directorTask) == interactiveDirectorTaskMemoryUpdate {
		markInteractiveMemoryFailed(conversation, turn, err)
		return
	}
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

func firstNonEmptyApp(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
