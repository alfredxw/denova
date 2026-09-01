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

func (c *Conversation) StoryRuntimeForMeta(meta interactive.StoryMeta) interactive.StoryDirector {
	return LoadStoryRuntimeForMeta(c.novaDir, meta)
}

// LoadStoryRuntimeForMeta assembles runtime resources from the story's own
// module selections and its independently selected planning outline.
func LoadStoryRuntimeForMeta(novaDir string, meta interactive.StoryMeta) interactive.StoryDirector {
	runtime := interactive.DefaultStoryDirector()
	template := LoadGamePlanningTemplateForMeta(novaDir, meta)
	runtime.ID = template.ID
	runtime.Name = template.Name
	runtime.Description = template.Description
	runtime.Strategy.PromptMarkdown = interactive.RenderGamePlanningTemplateMarkdown(template)
	runtime.Strategy.RuleStateConsumptionMode = meta.CheckSettings.RuleStateConsumptionMode
	runtime.Strategy.RuleVisibilityMode = meta.CheckSettings.RuleVisibilityMode
	refs := interactive.DefaultStoryDirectorModuleRefs()
	if meta.ModuleRefs != nil {
		refs = interactive.NormalizeStoryDirectorModuleRefs(*meta.ModuleRefs)
	}
	runtime.ModuleRefs = refs
	runtime.ResolvedSnapshot = interactive.StoryDirectorResolvedSnapshot{}
	return interactive.ResolveStoryDirectorModules(novaDir, runtime)
}

func LoadGamePlanningTemplateForMeta(novaDir string, meta interactive.StoryMeta) interactive.GamePlanningTemplate {
	return loadGamePlanningTemplate(novaDir, meta.PlanningTemplateID)
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

func loadGamePlanningTemplate(novaDir, templateID string) interactive.GamePlanningTemplate {
	if novaDir == "" {
		return interactive.DefaultGamePlanningTemplate()
	}
	item, err := interactive.NewGamePlanningTemplateLibrary(novaDir).Get(templateID)
	if err == nil {
		return item
	}
	slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] load planning template failed id=%s err=%v", templateID, err))
	fallback, fallbackErr := interactive.NewGamePlanningTemplateLibrary(novaDir).Get(interactive.DefaultGamePlanningTemplateID)
	if fallbackErr != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-agent] load fallback planning template failed err=%v", fallbackErr))
		return interactive.DefaultGamePlanningTemplate()
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
