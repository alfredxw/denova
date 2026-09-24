package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"denova/internal/interactive"
	"denova/internal/platform"
)

func platformStoryModuleRefs(refs *interactive.StoryDirectorModuleRefs) *platform.StoryModuleRefs {
	if refs == nil {
		return nil
	}
	return &platform.StoryModuleRefs{
		NarrativeStyleID: refs.NarrativeStyleID, NarrativeStyleDisabled: refs.NarrativeStyleDisabled,
		EventPackageIDs: refs.EventPackageIDs, EventPackagesDisabled: refs.EventPackagesDisabled,
		RuleSystemID: refs.RuleSystemID, RuleSystemDisabled: refs.RuleSystemDisabled,
		ActorStateID: refs.ActorStateID, ActorStateDisabled: refs.ActorStateDisabled,
		ImagePresetID: refs.ImagePresetID, ImagePresetDisabled: refs.ImagePresetDisabled,
	}
}

func (h Stories) Tune(ctx context.Context, scope platform.Scope, input platform.StoryPacing) (platform.StorySnapshot, error) {
	operation, err := h.host.AcquireStory(ctx, scope.ProjectID)
	if err != nil {
		return platform.StorySnapshot{}, err
	}
	defer operation.Release()
	ctx = operation.Context()
	if input.ReplyTargetChars <= 0 {
		return platform.StorySnapshot{}, platformStoryError("INVALID_ARGUMENT", "Reply target must be positive")
	}
	_, err = h.host.UpdateInteractiveStory(scope.StoryID, interactive.UpdateStoryRequest{ReplyTargetChars: &input.ReplyTargetChars})
	if err != nil {
		slog.WarnContext(ctx, "platform_story_pacing_failed", "story", scope.StoryID, "error", err)
		return platform.StorySnapshot{}, err
	}
	slog.InfoContext(ctx, "platform_story_pacing_updated", "story", scope.StoryID, "reply_target_chars", input.ReplyTargetChars)
	return h.Snapshot(ctx, scope)
}

func nativeStoryModuleRefs(refs *platform.StoryModuleRefs) *interactive.StoryDirectorModuleRefs {
	if refs == nil {
		return nil
	}
	return &interactive.StoryDirectorModuleRefs{
		NarrativeStyleID: refs.NarrativeStyleID, NarrativeStyleDisabled: refs.NarrativeStyleDisabled,
		EventPackageIDs: refs.EventPackageIDs, EventPackagesDisabled: refs.EventPackagesDisabled,
		RuleSystemID: refs.RuleSystemID, RuleSystemDisabled: refs.RuleSystemDisabled,
		ActorStateID: refs.ActorStateID, ActorStateDisabled: refs.ActorStateDisabled,
		ImagePresetID: refs.ImagePresetID, ImagePresetDisabled: refs.ImagePresetDisabled,
	}
}

func platformStoryConfiguration(meta interactive.StoryMeta) platform.StoryConfiguration {
	p := meta.Protagonist
	result := platform.StoryConfiguration{
		Origin: meta.Origin, Protagonist: &platform.StoryProtagonist{Mode: p.Mode, Name: p.Name, Profile: p.Profile, SourceLoreItemID: p.SourceLoreItemID, SourceLoreUpdatedAt: p.SourceLoreUpdatedAt},
		ModuleRefs: platformStoryModuleRefs(meta.ModuleRefs), PlanningTemplateID: meta.PlanningTemplateID,
		PlanningMode: meta.PlanningMode, ReplyTargetChars: meta.ReplyTargetChars, ChoiceCount: meta.ChoiceCount,
	}
	if meta.StateSchemaPolicy != nil {
		result.StateSchemaMode = meta.StateSchemaPolicy.Mode
	}
	if meta.ActorStateSchema != nil {
		result.StatePreset, _ = json.Marshal(meta.ActorStateSchema.System)
		result.RulePreset, _ = json.Marshal(meta.ActorStateSchema.TRPGSystem)
		for _, actor := range meta.ActorStateSchema.System.InitialActors {
			result.InitialActors = append(result.InitialActors, platform.StoryInitialActor{ID: actor.ID, Name: actor.Name, TemplateID: actor.TemplateID, Role: actor.Role, Description: actor.Description, State: actor.State})
		}
	}
	return result
}

// Configure shares the native admission and opening-update path. Only the
// extension's public input is adapted here; schema normalization, validation,
// initial state reduction and persistence remain native Story operations.
func (h Stories) Configure(ctx context.Context, scope platform.Scope, input platform.StoryConfiguration) (platform.StorySnapshot, error) {
	operation, err := h.host.AcquireStory(ctx, scope.ProjectID)
	if err != nil {
		return platform.StorySnapshot{}, err
	}
	defer operation.Release()
	ctx = operation.Context()
	if len(input.Origin) > 256*1024 {
		return platform.StorySnapshot{}, platformStoryError("INVALID_ARGUMENT", "Story origin exceeds 256 KiB")
	}
	dataDir := h.host.DataDir()
	if err := validatePlatformStoryPresetSelection(dataDir, input); err != nil {
		slog.WarnContext(ctx, "platform_story_preset_unavailable", "story", scope.StoryID, "error", err)
		return platform.StorySnapshot{}, platformStoryError("INVALID_ARGUMENT", "Selected Story preset is unavailable")
	}
	err = h.host.WithStoryOpening(ctx, scope.StoryID, func(opening Opening) error {
		return configureStoryOpening(ctx, scope, opening, input)
	})
	if err != nil {
		slog.WarnContext(ctx, "platform_story_configuration_rejected", "story", scope.StoryID, "error", err)
		return platform.StorySnapshot{}, err
	}
	slog.InfoContext(ctx, "platform_story_configured", "story", scope.StoryID, "instance", scope.InstanceID, "actors", len(input.InitialActors))
	return h.Snapshot(ctx, scope)
}

func configureStoryOpening(ctx context.Context, scope platform.Scope, opening Opening, input platform.StoryConfiguration) error {
	store := opening.Store()
	snapshot, err := store.Snapshot(scope.StoryID, "")
	if err != nil {
		return err
	}
	if snapshot.TurnCount != 0 || len(snapshot.Graph.Branches) != 1 {
		return platformStoryError("DOCUMENT_CONFLICT", "Opening configuration requires a Story without turns or additional branches")
	}
	mode := input.StateSchemaMode
	if mode == "" {
		mode = interactive.StoryStateSchemaModeFixedTemplate
	}
	switch mode {
	case interactive.StoryStateSchemaModeFixedTemplate, interactive.StoryStateSchemaModeAdaptTemplate, interactive.StoryStateSchemaModeGenerate:
	default:
		return platformStoryError("INVALID_ARGUMENT", "Unknown stateSchemaMode")
	}
	request := interactive.UpdateStoryRequest{Origin: &input.Origin, ModuleRefs: nativeStoryModuleRefs(input.ModuleRefs), PlanningTemplateID: input.PlanningTemplateID, StateSchemaPolicy: &interactive.StoryStateSchemaPolicy{Mode: mode}}
	if len(input.StatePreset) > 0 || len(input.RulePreset) > 0 {
		if mode != interactive.StoryStateSchemaModeFixedTemplate {
			return platformStoryError("INVALID_ARGUMENT", "Portable state and rule snapshots require fixed_template mode")
		}
		if request.ModuleRefs == nil {
			refs := interactive.DefaultStoryDirectorModuleRefs()
			request.ModuleRefs = &refs
		}
		// Frozen snapshots supersede author-local IDs. Native loading still
		// resolves the other modules before the per-Story freeze below.
		if len(input.StatePreset) > 0 {
			if request.ModuleRefs.ActorStateDisabled {
				return platformStoryError("INVALID_ARGUMENT", "A state snapshot cannot enable an explicitly disabled state system")
			}
			request.ModuleRefs.ActorStateID = ""
			request.ModuleRefs.ActorStateDisabled = false
		}
		if len(input.RulePreset) > 0 {
			request.ModuleRefs.RuleSystemID = ""
			request.ModuleRefs.RuleSystemDisabled = false
		}
	}
	if input.PlanningMode != "" {
		request.PlanningMode = &input.PlanningMode
	}
	if input.ReplyTargetChars != 0 {
		request.ReplyTargetChars = &input.ReplyTargetChars
	}
	if input.ChoiceCount != 0 {
		request.ChoiceCount = &input.ChoiceCount
	}
	if p := input.Protagonist; p != nil {
		protagonist, err := opening.ResolveProtagonist(ctx, interactive.StoryProtagonist{Mode: p.Mode, Name: p.Name, Profile: p.Profile, SourceLoreItemID: p.SourceLoreItemID, SourceLoreUpdatedAt: p.SourceLoreUpdatedAt})
		if err != nil {
			return platformStoryError("INVALID_ARGUMENT", err.Error())
		}
		request.Protagonist = &protagonist
	}
	request, err = opening.PrepareUpdate(request)
	if err != nil {
		return platformStoryError("INVALID_ARGUMENT", err.Error())
	}
	if len(input.StatePreset) > 0 || len(input.RulePreset) > 0 {
		var state interactive.StoryDirectorActorStateSystem
		var rules interactive.StoryDirectorTRPGSystem
		if request.ActorState != nil {
			state = *request.ActorState
		}
		if request.TRPGSystem != nil {
			rules = *request.TRPGSystem
		}
		if len(input.StatePreset) > 0 {
			state = interactive.StoryDirectorActorStateSystem{}
			if err := decodePortableStoryContent(input.StatePreset, &state); err != nil {
				return err
			}
		}
		if len(input.RulePreset) > 0 {
			rules = interactive.StoryDirectorTRPGSystem{}
			if err := decodePortableStoryContent(input.RulePreset, &rules); err != nil {
				return err
			}
			if input.ModuleRefs != nil && input.ModuleRefs.RuleSystemDisabled && len(rules.RuleTemplates) > 0 {
				return platformStoryError("INVALID_ARGUMENT", "A nonempty rule snapshot cannot enable an explicitly disabled rule system")
			}
		}
		if len(state.Templates) == 0 {
			return platformStoryError("INVALID_ARGUMENT", "Portable state requires Actor templates")
		}
		frozen, err := interactive.PreparePortableStoryState(state, rules)
		if err != nil {
			return platformStoryError("INVALID_ARGUMENT", err.Error())
		}
		request.ActorState, request.TRPGSystem = &frozen.System, &frozen.TRPGSystem
		if len(input.RulePreset) > 0 && len(frozen.TRPGSystem.RuleTemplates) == 0 {
			// Native runtime falls back to the selected library when a frozen
			// rule list is empty. Explicitly disable it to preserve this snapshot.
			request.ModuleRefs.RuleSystemDisabled = true
		}
	}
	if len(input.InitialActors) > 0 {
		if request.ActorState == nil {
			return platformStoryError("INVALID_ARGUMENT", "Initial Actors require an enabled state template")
		}
		if mode != interactive.StoryStateSchemaModeFixedTemplate {
			return platformStoryError("INVALID_ARGUMENT", "Explicit initial Actors require fixed_template mode")
		}
		actors := request.ActorState.InitialActors
		byID := map[string]int{}
		for index, actor := range actors {
			byID[actor.ID] = index
		}
		seen := map[string]bool{}
		for _, actor := range input.InitialActors {
			if actor.ID == "" || strings.TrimSpace(actor.ID) != actor.ID || seen[actor.ID] {
				return platformStoryError("INVALID_ARGUMENT", "Initial Actor IDs must be nonempty and unique")
			}
			seen[actor.ID] = true
			if strings.TrimSpace(actor.Name) == "" || len(actor.Name) > 128 || len(actor.Role) > 128 || len(actor.Description) > 4000 {
				return platformStoryError("INVALID_ARGUMENT", "Initial Actor name requires 1..128 UTF-8 bytes; role allows 128 and description 4000 bytes. Put full profiles in origin")
			}
			// Validate all fields with the existing Actor compiler before schema
			// normalization, which intentionally tolerates reusable preset edits.
			patch := interactive.ActorStatePatch{ActorID: actor.ID, ActorName: actor.Name, TemplateID: actor.TemplateID, Role: actor.Role, Description: actor.Description, State: actor.State}
			validated, err := interactive.ValidateActorStatePatches(*request.ActorState, []interactive.ActorStatePatch{patch}, "")
			if err != nil {
				return platformStoryError("INVALID_ARGUMENT", fmt.Sprintf("Invalid initial Actor %q: %v", actor.ID, err))
			}
			if len(validated.AppliedActors) != 1 || validated.AppliedActors[0] != actor.ID {
				return platformStoryError("INVALID_ARGUMENT", "Initial Actor IDs must already use their exact native identity without dots, controls or normalization changes")
			}
			value := interactive.ActorStateInitialActor{ID: actor.ID, Name: actor.Name, TemplateID: actor.TemplateID, Role: actor.Role, Description: actor.Description, State: actor.State}
			if index, exists := byID[actor.ID]; exists {
				actors[index] = value
			} else {
				byID[actor.ID] = len(actors)
				actors = append(actors, value)
			}
		}
		request.ActorState.InitialActors = actors
	}
	return opening.Commit(ctx, scope.StoryID, request)
}
