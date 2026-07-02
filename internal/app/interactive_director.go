package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/session"
)

const interactiveDirectorPatchSource = "interactive_director"

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
		snapshot, err := conversation.store.Snapshot(conversation.storyID, turn.BranchID)
		if err != nil {
			log.Printf("[interactive-director-agent] load snapshot failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
			markInteractiveDirectorFailed(conversation, turn, err)
			return
		}
		if !snapshot.DirectorState.Enabled {
			log.Printf("[interactive-director-agent] skipped disabled story_id=%s branch_id=%s turn_id=%s", conversation.storyID, turn.BranchID, turn.ID)
			return
		}

		log.Printf("[interactive-director-agent] run begin story_id=%s branch_id=%s turn_id=%s", conversation.storyID, turn.BranchID, turn.ID)
		instruction, err := conversation.BuildDirectorInstruction(turn)
		if err != nil {
			log.Printf("[interactive-director-agent] build instruction failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
			markInteractiveDirectorFailed(conversation, turn, err)
			return
		}
		output, err := generateInteractiveDirectorForPlan(context.Background(), cfg, state, agent.InteractiveStoryToolContext{
			Store:    conversation.store,
			StoryID:  conversation.storyID,
			BranchID: turn.BranchID,
		}, instruction)
		if err != nil {
			persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, "执行失败："+err.Error())
			log.Printf("[interactive-director-agent] generate failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
			markInteractiveDirectorFailed(conversation, turn, err)
			return
		}
		persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, output)
		patch, err := parseInteractiveDirectorPatch(output)
		if err != nil {
			log.Printf("[interactive-director-agent] parse failed story_id=%s branch_id=%s turn_id=%s err=%v output=%q", conversation.storyID, turn.BranchID, turn.ID, err, output)
			markInteractiveDirectorFailed(conversation, turn, err)
			return
		}
		patch.BranchID = turn.BranchID
		patch.Source = interactiveDirectorPatchSource
		patch.Summary = firstNonEmptyApp(patch.Summary, "后台导演已根据本回合审计更新叙事计划。")
		patch.LastDirectorRun = &interactive.DirectorRunStatus{
			Status:    "ready",
			Summary:   patch.Summary,
			UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
		if _, err := conversation.store.UpdateDirectorState(conversation.storyID, patch); err != nil {
			log.Printf("[interactive-director-agent] persist patch failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, err)
			markInteractiveDirectorFailed(conversation, turn, err)
			return
		}
		log.Printf("[interactive-director-agent] run done story_id=%s branch_id=%s turn_id=%s summary=%q", conversation.storyID, turn.BranchID, turn.ID, patch.Summary)
	}()
}

func markInteractiveDirectorFailed(conversation *interactiveConversation, turn interactive.TurnEvent, err error) {
	if conversation == nil || conversation.store == nil || err == nil {
		return
	}
	run := interactive.DirectorRunStatus{
		Status:    "failed",
		Summary:   "后台导演更新失败，已保留本回合正文和规则结算。",
		Error:     err.Error(),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if _, markErr := conversation.store.UpdateDirectorState(conversation.storyID, interactive.UpdateDirectorStateRequest{
		BranchID:        turn.BranchID,
		Source:          interactiveDirectorPatchSource,
		Summary:         run.Summary,
		LastDirectorRun: &run,
	}); markErr != nil {
		log.Printf("[interactive-director-agent] mark failed director run failed story_id=%s branch_id=%s turn_id=%s err=%v", conversation.storyID, turn.BranchID, turn.ID, markErr)
	}
}

func parseInteractiveDirectorPatch(content string) (interactive.UpdateDirectorStateRequest, error) {
	content = strings.TrimSpace(extractDirectorJSONContent(content))
	if content == "" {
		return interactive.UpdateDirectorStateRequest{}, fmt.Errorf("互动导演 Agent 返回为空")
	}
	var req interactive.UpdateDirectorStateRequest
	if err := json.Unmarshal([]byte(content), &req); err != nil {
		return interactive.UpdateDirectorStateRequest{}, fmt.Errorf("解析互动导演 patch 失败: %w", err)
	}
	if !interactiveDirectorPatchHasChanges(req) {
		var wrapped struct {
			Summary       string                    `json:"summary,omitempty"`
			DirectorState interactive.DirectorState `json:"director_state,omitempty"`
		}
		if err := json.Unmarshal([]byte(content), &wrapped); err != nil {
			return interactive.UpdateDirectorStateRequest{}, fmt.Errorf("解析互动导演 director_state 失败: %w", err)
		}
		if !wrapped.DirectorState.Enabled && strings.TrimSpace(wrapped.DirectorState.MainArc) == "" && strings.TrimSpace(wrapped.DirectorState.StagePlan) == "" {
			return interactive.UpdateDirectorStateRequest{}, fmt.Errorf("互动导演 Agent 未返回可应用的 DirectorState patch")
		}
		req = updateDirectorRequestFromState(wrapped.DirectorState)
		req.Summary = wrapped.Summary
	}
	return req, nil
}

func extractDirectorJSONContent(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSpace(content)
		content = strings.TrimSuffix(content, "```")
	}
	return strings.TrimSpace(content)
}

func interactiveDirectorPatchHasChanges(req interactive.UpdateDirectorStateRequest) bool {
	return req.Enabled != nil ||
		req.SpoilerMode != nil ||
		req.MainArc != nil ||
		req.StagePlan != nil ||
		req.BeatQueue != nil ||
		req.EventQueue != nil ||
		req.Foreshadowing != nil ||
		req.PotentialCharacters != nil ||
		req.BranchPatches != nil ||
		req.ForcedEvents != nil ||
		req.DisabledEvents != nil ||
		strings.TrimSpace(req.Summary) != ""
}

func updateDirectorRequestFromState(state interactive.DirectorState) interactive.UpdateDirectorStateRequest {
	enabled := state.Enabled
	spoilerMode := state.SpoilerMode
	mainArc := state.MainArc
	stagePlan := state.StagePlan
	beatQueue := state.BeatQueue
	eventQueue := state.EventQueue
	foreshadowing := state.Foreshadowing
	potentialCharacters := state.PotentialCharacters
	branchPatches := state.BranchPatches
	forcedEvents := state.ForcedEvents
	disabledEvents := state.DisabledEvents
	return interactive.UpdateDirectorStateRequest{
		Enabled:             &enabled,
		SpoilerMode:         &spoilerMode,
		MainArc:             &mainArc,
		StagePlan:           &stagePlan,
		BeatQueue:           &beatQueue,
		EventQueue:          &eventQueue,
		Foreshadowing:       &foreshadowing,
		PotentialCharacters: &potentialCharacters,
		BranchPatches:       &branchPatches,
		ForcedEvents:        &forcedEvents,
		DisabledEvents:      &disabledEvents,
	}
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
