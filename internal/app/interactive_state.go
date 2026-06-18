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
				err := fmt.Errorf("互动记忆 Agent 异常中断: %v", recovered)
				log.Printf("[interactive-memory-agent] panic recovered story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
				markInteractiveStateFailed(conversation, turn, err)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), interactiveStateTimeout)
		defer cancel()

		log.Printf("[interactive-memory-agent] run begin story_id=%s branch_id=%s turn_id=%s", conversation.storyID, turn.BranchID, turn.ID)
		instruction, err := conversation.BuildStateInstruction(turn)
		if err != nil {
			log.Printf("[interactive-memory-agent] build instruction failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
			markInteractiveStateFailed(conversation, turn, err)
			return
		}
		output, err := agent.GenerateInteractiveState(ctx, cfg, instruction)
		if err != nil {
			log.Printf("[interactive-memory-agent] generate failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
			persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveState, instruction, "执行失败："+err.Error())
			markInteractiveStateFailed(conversation, turn, err)
			return
		}
		persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveState, instruction, output)
		result, err := parseInteractiveMemoryOutput(output)
		if err != nil {
			log.Printf("[interactive-memory-agent] parse failed story_id=%s branch_id=%s turn_id=%s err=%v output=%q", conversation.storyID, turn.BranchID, turn.ID, err, output)
			markInteractiveStateFailed(conversation, turn, err)
			return
		}
		if len(result.StateOps) > 0 {
			if _, err := conversation.store.AppendStateDelta(conversation.storyID, interactive.AppendStateDeltaRequest{
				ParentID: turn.ID,
				BranchID: turn.BranchID,
				Ops:      result.StateOps,
			}); err != nil {
				log.Printf("[interactive-memory-agent] persist state failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
				markInteractiveStateFailed(conversation, turn, err)
				return
			}
		}
		if len(result.StoryMemoryPatches) > 0 {
			if _, err := conversation.store.ApplyStoryMemoryPatches(conversation.storyID, turn.BranchID, turn.ID, result.StoryMemoryPatches); err != nil {
				log.Printf("[interactive-memory-agent] persist story memory failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
				markInteractiveStateFailed(conversation, turn, err)
				return
			}
		}
		if err := conversation.store.MarkInteractiveMemoryReady(conversation.storyID, turn.BranchID, turn.ID); err != nil {
			log.Printf("[interactive-memory-agent] mark memory ready failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
			markInteractiveStateFailed(conversation, turn, err)
			return
		}
		log.Printf("[interactive-memory-agent] run done story_id=%s branch_id=%s turn_id=%s state_ops=%d story_memory_patches=%d", conversation.storyID, turn.BranchID, turn.ID, len(result.StateOps), len(result.StoryMemoryPatches))
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
		log.Printf("[interactive-memory-agent] mark failed state failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, markErr)
	}
}
