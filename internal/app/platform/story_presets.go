package platform

import (
	"context"
	"fmt"
	"log/slog"

	imagepreset "denova/internal/image/preset"
	"denova/internal/interactive"
	"denova/internal/interactive/teller"
	"denova/internal/platform"
)

func validatePlatformStoryPresetSelection(dataDir string, input platform.StoryConfiguration) error {
	if input.PlanningTemplateID != "" {
		if _, err := interactive.NewGamePlanningTemplateLibrary(dataDir).Get(input.PlanningTemplateID); err != nil {
			return err
		}
	}
	refs := input.ModuleRefs
	if refs == nil {
		return nil
	}
	if refs.ActorStateID != "" && !refs.ActorStateDisabled && len(input.StatePreset) == 0 {
		if _, err := interactive.NewActorStateLibrary(dataDir).Get(refs.ActorStateID); err != nil {
			return err
		}
	}
	if refs.RuleSystemID != "" && !refs.RuleSystemDisabled && len(input.RulePreset) == 0 {
		if _, err := interactive.NewRuleSystemLibrary(dataDir).Get(refs.RuleSystemID); err != nil {
			return err
		}
	}
	if !refs.EventPackagesDisabled {
		for _, id := range refs.EventPackageIDs {
			if id == "" {
				return fmt.Errorf("Event preset ID is empty")
			}
			if _, err := interactive.NewEventPackageLibrary(dataDir).Get(id); err != nil {
				return err
			}
		}
	}
	if refs.NarrativeStyleID != "" && !refs.NarrativeStyleDisabled {
		if _, err := teller.NewLibrary(dataDir).Get(refs.NarrativeStyleID); err != nil {
			return err
		}
	}
	if refs.ImagePresetID != "" && !refs.ImagePresetDisabled {
		if _, err := imagepreset.NewLibrary(dataDir).Get(refs.ImagePresetID); err != nil {
			return err
		}
	}
	return nil
}

// Presets exposes the same libraries as native Story creation. Invalid local
// files are omitted; paths and parser errors never become extension content.
func (h Stories) Presets(ctx context.Context, scope platform.Scope) ([]platform.StoryPreset, error) {
	operation, err := h.host.AcquireStory(ctx, scope.ProjectID)
	if err != nil {
		return nil, err
	}
	defer operation.Release()
	dataDir := h.host.DataDir()
	items := []platform.StoryPreset{}
	states, err := interactive.NewActorStateLibrary(dataDir).List()
	if err != nil {
		return nil, err
	}
	for _, item := range states {
		if !item.Invalid {
			items = append(items, platform.StoryPreset{Kind: "state", ID: item.ID, Name: item.Name, Description: item.Description, Revision: item.Revision, Content: item.ActorState})
		}
	}
	rules, err := interactive.NewRuleSystemLibrary(dataDir).List()
	if err != nil {
		return nil, err
	}
	for _, item := range rules {
		if !item.Invalid {
			items = append(items, platform.StoryPreset{Kind: "rules", ID: item.ID, Name: item.Name, Description: item.Description, Revision: item.Revision, Content: item.TRPGSystem})
		}
	}
	events, err := interactive.NewEventPackageLibrary(dataDir).List()
	if err != nil {
		return nil, err
	}
	for _, item := range events {
		if !item.Invalid {
			items = append(items, platform.StoryPreset{Kind: "events", ID: item.ID, Name: item.Name, Description: item.Description, Revision: item.Revision, Content: item.Events})
		}
	}
	plans, err := interactive.NewGamePlanningTemplateLibrary(dataDir).List()
	if err != nil {
		return nil, err
	}
	for _, item := range plans {
		if !item.Invalid {
			items = append(items, platform.StoryPreset{Kind: "planning", ID: item.ID, Name: item.Name, Description: item.Description, Revision: item.Revision, Content: item.Sections})
		}
	}
	narratives, err := teller.NewLibrary(dataDir).List()
	if err != nil {
		return nil, err
	}
	for _, item := range narratives {
		if !item.Invalid {
			content, err := portableStoryNarrative(dataDir, item)
			if err != nil {
				slog.WarnContext(ctx, "platform_story_preset_unavailable", "kind", "narrative", "preset", item.ID, "error", err)
				continue
			}
			items = append(items, platform.StoryPreset{Kind: "narrative", ID: item.ID, Name: item.Name, Description: item.Description, Revision: item.Revision, Content: content})
		}
	}
	images, err := imagepreset.NewLibrary(dataDir).List()
	if err != nil {
		return nil, err
	}
	for _, item := range images {
		if !item.Invalid {
			items = append(items, platform.StoryPreset{Kind: "image", ID: item.ID, Name: item.Name, Description: item.Description, Revision: item.Revision, Content: map[string]any{"prompt": item.Prompt, "slots": item.Slots}})
		}
	}
	return items, nil
}
