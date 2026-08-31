package tools

import (
	"context"
	"encoding/json"
	"fmt"

	agent "github.com/alfredxw/denova/agent"
)

const selectStoryProtagonistToolName = "select_story_protagonist"

type selectStoryProtagonistInput struct {
	LoreItemID string `json:"lore_item_id" jsonschema_description:"Exact stable ID of any enabled character entry from the provided Lore character catalog. A protagonist tag is not required."`
}

func newInteractiveProtagonistTools(ctx InteractiveContext) ([]agent.ToolDefinition, error) {
	if ctx.SelectStoryProtagonist == nil {
		return nil, nil
	}
	tool, err := agent.InferTool(
		selectStoryProtagonistToolName,
		"Select the player-controlled protagonist during an unresolved opening. Choose any enabled Lore character that best fits the story premise, not only a character tagged protagonist. Call this exactly once before opening prose; the backend validates the ID and freezes a story-owned snapshot.",
		func(callCtx context.Context, input selectStoryProtagonistInput) (string, error) {
			selected, err := ctx.SelectStoryProtagonist(callCtx, input.LoreItemID)
			if err != nil {
				return "", err
			}
			data, err := json.MarshalIndent(selected, "", "  ")
			if err != nil {
				return "", fmt.Errorf("serialize selected story protagonist: %w", err)
			}
			return string(data), nil
		},
	)
	if err != nil {
		return nil, err
	}
	definition, err := defineTool(tool, interactiveStoryWorkflowDescriptor())
	if err != nil {
		return nil, err
	}
	return []agent.ToolDefinition{definition}, nil
}
