package interactiveapp

import (
	"context"
	"fmt"
	"log/slog"

	"denova/internal/agents/prompts"
	"denova/internal/interactive"
	"denova/internal/interactive/teller"
	"denova/internal/style"
)

func (c *Conversation) teller(tellerID string) teller.Definition {
	return LoadGameTeller(c.novaDir, tellerID)
}

func (c *Conversation) StoryDirectorForMeta(meta interactive.StoryMeta) interactive.StoryDirector {
	return LoadStoryDirectorForMeta(c.novaDir, meta)
}

// LoadStoryDirectorForMeta resolves the legacy storage name as the current
// game preset. The preset supplies rules, state, narrative style, images, and
// optional planning guidance; it no longer represents a separate Agent.
func LoadStoryDirectorForMeta(novaDir string, meta interactive.StoryMeta) interactive.StoryDirector {
	preset := loadGamePreset(novaDir, meta.StoryDirectorID)
	if meta.ModuleRefs == nil {
		return preset
	}
	preset.ModuleRefs = interactive.NormalizeStoryDirectorModuleRefs(*meta.ModuleRefs)
	preset.ResolvedSnapshot = interactive.StoryDirectorResolvedSnapshot{}
	return interactive.ResolveStoryDirectorModules(novaDir, preset)
}

func storyDirectorForSnapshot(preset interactive.StoryDirector, snapshot *interactive.ActorStateSchemaSnapshot) interactive.StoryDirector {
	if snapshot == nil || len(snapshot.System.Templates) == 0 {
		return preset
	}
	preset.ActorState = snapshot.System
	if !preset.ModuleRefs.RuleSystemDisabled && len(snapshot.TRPGSystem.RuleTemplates) > 0 {
		preset.TRPGSystem = snapshot.TRPGSystem
	}
	return preset
}

func LoadWritingTeller(novaDir, tellerID string) teller.Definition {
	return loadInteractiveTeller(novaDir, tellerID)
}

func LoadGameTeller(novaDir, tellerID string) teller.Definition {
	return loadInteractiveTeller(novaDir, tellerID)
}

func loadInteractiveTeller(novaDir, tellerID string) teller.Definition {
	if novaDir == "" {
		return teller.Definition{}
	}
	library := teller.NewLibrary(novaDir)
	selected, err := library.Get(tellerID)
	if err == nil {
		return selected
	}
	slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] load narrative style failed id=%s err=%v", tellerID, err))
	fallback, fallbackErr := library.Get(style.DefaultID)
	if fallbackErr != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] load default narrative style failed id=%s err=%v", style.DefaultID, fallbackErr))
		return teller.Definition{}
	}
	return fallback
}

func loadGamePreset(novaDir, presetID string) interactive.StoryDirector {
	if novaDir == "" {
		return interactive.DefaultStoryDirector()
	}
	preset, err := interactive.NewStoryDirectorLibrary(novaDir).Get(presetID)
	if err == nil {
		return preset
	}
	slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] load game preset failed id=%s err=%v", presetID, err))
	fallback, fallbackErr := interactive.NewStoryDirectorLibrary(novaDir).Get(interactive.DefaultStoryDirectorID)
	if fallbackErr != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] load fallback game preset failed err=%v", fallbackErr))
		return interactive.DefaultStoryDirector()
	}
	return fallback
}

func StoryTellerSystemInput(teller teller.Definition, styleRules ...[]prompts.StyleRule) prompts.InteractiveStorySystemInstructionInput {
	var rules []prompts.StyleRule
	if len(styleRules) > 0 {
		rules = styleRules[0]
	}
	return prompts.InteractiveStorySystemInstructionInput{
		StoryTellerID:           teller.ID,
		StoryTellerName:         teller.Name,
		StoryTellerDescription:  teller.Description,
		StoryTellerSystemPrompt: teller.PromptForTargets("system"),
		StyleRules:              rules,
	}
}
