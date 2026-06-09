package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"nova/config"
	"nova/internal/agent"
	"nova/internal/interactive"
	"nova/internal/session"
)

const interactiveStateTimeout = 2 * time.Minute

func startInteractiveStateTask(cfg *config.Config, conversation *interactiveConversation, turn interactive.TurnEvent, sessionStore *session.Store) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				err := fmt.Errorf("状态 Agent 异常中断: %v", recovered)
				log.Printf("[interactive-state-agent] panic recovered story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
				markInteractiveStateFailed(conversation, turn, err)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), interactiveStateTimeout)
		defer cancel()

		log.Printf("[interactive-state-agent] run begin story_id=%s branch_id=%s turn_id=%s", conversation.storyID, turn.BranchID, turn.ID)
		instruction, err := conversation.BuildStateInstruction(turn)
		if err != nil {
			log.Printf("[interactive-state-agent] build instruction failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
			markInteractiveStateFailed(conversation, turn, err)
			return
		}
		output, err := agent.GenerateInteractiveState(ctx, cfg, instruction)
		if err != nil {
			log.Printf("[interactive-state-agent] generate failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
			persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveState, instruction, "执行失败："+err.Error())
			markInteractiveStateFailed(conversation, turn, err)
			return
		}
		persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveState, instruction, output)
		ops, err := parseInteractiveStateOps(output)
		if err != nil {
			log.Printf("[interactive-state-agent] parse failed story_id=%s branch_id=%s turn_id=%s err=%v output=%q", conversation.storyID, turn.BranchID, turn.ID, err, output)
			markInteractiveStateFailed(conversation, turn, err)
			return
		}
		if _, err := conversation.store.AppendStateDelta(conversation.storyID, interactive.AppendStateDeltaRequest{
			ParentID: turn.ID,
			BranchID: turn.BranchID,
			Ops:      ops,
		}); err != nil {
			log.Printf("[interactive-state-agent] persist failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
			markInteractiveStateFailed(conversation, turn, err)
			return
		}
		log.Printf("[interactive-state-agent] run done story_id=%s branch_id=%s turn_id=%s ops=%d", conversation.storyID, turn.BranchID, turn.ID, len(ops))
	}()
}

func markInteractiveStateFailed(conversation *interactiveConversation, turn interactive.TurnEvent, err error) {
	if conversation == nil || conversation.store == nil {
		return
	}
	if markErr := conversation.store.MarkStateFailed(conversation.storyID, interactive.MarkStateFailedRequest{
		ParentID: turn.ID,
		BranchID: turn.BranchID,
		Error:    err.Error(),
	}); markErr != nil {
		log.Printf("[interactive-state-agent] mark failed state failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, markErr)
	}
}
