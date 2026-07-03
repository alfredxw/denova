package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/session"
)

func startInteractiveDirectorTask(cfg *config.Config, state *book.State, conversation *interactiveConversation, turn interactive.TurnEvent, sessionStore *session.Store) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				err := fmt.Errorf("互动导演 Agent 异常中断: %v", recovered)
				log.Printf("[interactive-director-agent] panic recovered story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
				markInteractiveDirectorFailed(conversation, turn, err)
			}
		}()

		if conversation == nil || conversation.store == nil || cfg == nil {
			return
		}
		if _, err := runInteractiveDirectorPlan(context.Background(), cfg, state, conversation, turn, sessionStore); err != nil {
			log.Printf("[interactive-director-agent] run failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
			markInteractiveDirectorFailed(conversation, turn, err)
			return
		}
	}()
}

func runInteractiveDirectorPlan(ctx context.Context, cfg *config.Config, state *book.State, conversation *interactiveConversation, turn interactive.TurnEvent, sessionStore *session.Store) (interactive.DirectorPlan, error) {
	if conversation == nil || conversation.store == nil || cfg == nil {
		return interactive.DirectorPlan{}, fmt.Errorf("互动导演运行上下文不完整")
	}
	token, err := conversation.store.DirectorPlanRunToken(conversation.storyID, turn.BranchID)
	if err != nil {
		return interactive.DirectorPlan{}, fmt.Errorf("准备导演规划运行版本失败: %w", err)
	}
	allowedPaths := conversation.store.DirectorPlanAllowedPaths(conversation.storyID, turn.BranchID)
	log.Printf("[interactive-director-agent] run begin story_id=%s branch_id=%s turn_id=%s revision=%s allowed_paths=%d", conversation.storyID, turn.BranchID, turn.ID, token.Revision, len(allowedPaths))
	instruction, err := conversation.BuildDirectorInstruction(turn)
	if err != nil {
		return interactive.DirectorPlan{}, fmt.Errorf("构建导演规划指令失败: %w", err)
	}
	output, err := generateInteractiveDirectorForPlan(ctx, cfg, state, agent.InteractiveStoryToolContext{
		Store:                    conversation.store,
		StoryID:                  conversation.storyID,
		BranchID:                 turn.BranchID,
		DirectorPlanAllowedPaths: allowedPaths,
	}, instruction)
	if err != nil {
		persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, "执行失败："+err.Error())
		return interactive.DirectorPlan{}, fmt.Errorf("生成导演规划失败: %w", err)
	}
	persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, output)
	plan, err := conversation.store.CompleteDirectorPlanRun(conversation.storyID, turn.BranchID, token, turn.ID, strings.TrimSpace(output))
	if err != nil {
		return interactive.DirectorPlan{}, fmt.Errorf("完成导演规划运行失败: %w", err)
	}
	status := ""
	if plan.Metadata.LastRun != nil {
		status = plan.Metadata.LastRun.Status
	}
	log.Printf("[interactive-director-agent] run done story_id=%s branch_id=%s turn_id=%s status=%s summary=%q", conversation.storyID, turn.BranchID, turn.ID, status, strings.TrimSpace(output))
	return plan, nil
}

func markInteractiveDirectorFailed(conversation *interactiveConversation, turn interactive.TurnEvent, err error) {
	if conversation == nil || conversation.store == nil || err == nil {
		return
	}
	if markErr := conversation.store.MarkDirectorPlanRunFailed(conversation.storyID, turn.BranchID, turn.ID, err); markErr != nil {
		log.Printf("[interactive-director-agent] mark failed director run failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, markErr)
	}
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
