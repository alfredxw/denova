package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	"nova/config"
	"nova/internal/agent"
	"nova/internal/book"
	"nova/internal/interactive"
	"nova/internal/prompts"
)

type InteractiveHotChoicesResult struct {
	Enabled bool     `json:"enabled"`
	Choices []string `json:"choices"`
}

func (a *App) GenerateInteractiveHotChoices(ctx context.Context, storyID, branchID string, excludeChoices []string) (InteractiveHotChoicesResult, error) {
	return a.interactiveService().GenerateInteractiveHotChoices(ctx, storyID, branchID, excludeChoices)
}

func (s *InteractiveAppService) GenerateInteractiveHotChoices(ctx context.Context, storyID, branchID string, excludeChoices []string) (InteractiveHotChoicesResult, error) {
	a := s.app
	a.mu.RLock()
	store := a.interactive
	cfg := a.cfg
	workspace := a.workspace
	sessionStore := a.sessionStore
	a.mu.RUnlock()
	if store == nil || cfg == nil {
		return InteractiveHotChoicesResult{}, ErrNoWorkspace
	}

	runtimeCfg := *cfg
	runtimeCfg.Workspace = workspace
	if layered, err := config.LoadLayered(runtimeCfg.NovaDir, workspace); err == nil {
		applyLayeredSettingsToConfig(&runtimeCfg, layered)
	} else {
		log.Printf("[interactive-hot-choices] load settings failed workspace=%s err=%v", workspace, err)
	}
	if !runtimeCfg.InteractiveHotChoices {
		log.Printf("[interactive-hot-choices] disabled by settings story_id=%s branch_id=%s", storyID, branchID)
		return InteractiveHotChoicesResult{Enabled: false, Choices: []string{}}, nil
	}

	storyCtx, err := store.StoryContext(storyID, branchID)
	if err != nil {
		return InteractiveHotChoicesResult{}, err
	}
	cached, hasCached, err := store.HotChoices(storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return InteractiveHotChoicesResult{}, err
	}
	if len(excludeChoices) == 0 && hasCached {
		log.Printf("[interactive-hot-choices] cache hit story_id=%s branch_id=%s parent_id=%s choices=%d", storyID, cached.BranchID, cached.ParentID, len(cached.Choices))
		return InteractiveHotChoicesResult{Enabled: true, Choices: cached.Choices}, nil
	}
	loreItems := hotChoicesLoreContext(workspace)
	turnMemory := buildInteractiveModelVisibleTurnMemory(storyCtx.Snapshot.Turns, storyCtx.Snapshot.ContextCompaction)
	instruction := prompts.InteractiveHotChoicesInstruction(prompts.InteractiveHotChoicesPromptInput{
		Title:          storyCtx.Meta.Title,
		Origin:         storyCtx.Meta.Origin,
		StoryTellerID:  storyCtx.Meta.StoryTellerID,
		BranchID:       storyCtx.Snapshot.BranchID,
		LoreItems:      loreItems,
		TurnHistory:    formatHotChoicesTurnHistory(turnMemory, storyCtx.Snapshot.ContextCompaction),
		ExcludeChoices: formatHotChoicesExcludeChoices(excludeChoices),
	})
	log.Printf(
		"[interactive-hot-choices] context composition story_id=%s branch_id=%s story_title=%s origin=%s teller_id=%s turns=%d model_turns=%d lore=%s instruction=%s",
		storyID,
		storyCtx.Snapshot.BranchID,
		interactivePartSummary(storyCtx.Meta.Title),
		interactivePartSummary(storyCtx.Meta.Origin),
		storyCtx.Meta.StoryTellerID,
		len(storyCtx.Snapshot.Turns),
		len(turnMemory.Turns),
		interactivePartSummary(loreItems),
		interactivePartSummary(instruction),
	)
	choices, err := agent.GenerateInteractiveHotChoices(ctx, &runtimeCfg, instruction)
	if err != nil {
		log.Printf("[interactive-hot-choices] generate failed story_id=%s branch_id=%s err=%v", storyID, storyCtx.Snapshot.BranchID, err)
		persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveHotChoices, instruction, "执行失败："+err.Error())
		return InteractiveHotChoicesResult{}, err
	}
	persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveHotChoices, instruction, formatHotChoicesSessionOutput(choices))
	persistedChoices := mergeHotChoiceLists(cached.Choices, excludeChoices, choices)
	if len(persistedChoices) == 0 {
		return InteractiveHotChoicesResult{}, fmt.Errorf("互动快捷选择模型返回为空")
	}
	event, err := store.SaveHotChoices(storyID, storyCtx.Snapshot.BranchID, persistedChoices)
	if err != nil {
		log.Printf("[interactive-hot-choices] persist failed story_id=%s branch_id=%s err=%v", storyID, storyCtx.Snapshot.BranchID, err)
		return InteractiveHotChoicesResult{}, err
	}
	log.Printf("[interactive-hot-choices] persist done story_id=%s branch_id=%s parent_id=%s choices=%d", storyID, event.BranchID, event.ParentID, len(event.Choices))
	return InteractiveHotChoicesResult{Enabled: true, Choices: event.Choices}, nil
}

func formatHotChoicesSessionOutput(choices []string) string {
	if len(choices) == 0 {
		return "（未生成快捷选项）"
	}
	var sb strings.Builder
	sb.WriteString("快捷选项：\n")
	for _, choice := range choices {
		choice = strings.TrimSpace(choice)
		if choice == "" {
			continue
		}
		fmt.Fprintf(&sb, "- %s\n", choice)
	}
	return strings.TrimSpace(sb.String())
}

func hotChoicesLoreContext(workspace string) string {
	if workspace == "" {
		return ""
	}
	context, err := book.NewLoreStore(workspace).ProgressiveContextMarkdown()
	if err != nil {
		log.Printf("[interactive-hot-choices] load lore context failed workspace=%s err=%v", workspace, err)
		return ""
	}
	return context
}

func formatHotChoicesTurnHistory(turnMemory interactiveTurnMemory, compaction *interactive.ContextCompactionEvent) string {
	return formatInteractiveTurnMemoryHistory(turnMemory, compaction, "（暂无历史回合，请基于开端给出第一步行动建议。）")
}

func formatHotChoicesExcludeChoices(choices []string) string {
	var sb strings.Builder
	for _, choice := range choices {
		choice = strings.TrimSpace(choice)
		if choice == "" {
			continue
		}
		fmt.Fprintf(&sb, "- %s\n", choice)
	}
	return strings.TrimSpace(sb.String())
}

func mergeHotChoiceLists(lists ...[]string) []string {
	merged := make([]string, 0)
	seen := map[string]bool{}
	for _, list := range lists {
		for _, choice := range list {
			choice = strings.TrimSpace(choice)
			if choice == "" || seen[choice] {
				continue
			}
			merged = append(merged, choice)
			seen[choice] = true
			if len(merged) >= 10 {
				return merged
			}
		}
	}
	return merged
}
