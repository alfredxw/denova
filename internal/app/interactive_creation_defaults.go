package app

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"

	"denova/config"
	imagepreset "denova/internal/image/preset"
	"denova/internal/interactive"
	"denova/internal/interactive/teller"
)

var ErrGameCreationDefaults = errors.New("book game defaults are unavailable")

// Creation choices outrank book defaults, then the book's last selected Story.
// Resume, updates and extension preview Stories never consult these defaults.
func (s *InteractiveAppService) withGameCreationDefaults(req interactive.CreateStoryRequest) (interactive.CreateStoryRequest, error) {
	cfg := s.cfg()
	if req.Preview || cfg == nil || cfg.ProjectStoreDir == "" {
		return req, nil
	}
	settings, err := config.ReadSettingsFile(filepath.Join(cfg.ProjectStoreDir, "config.toml"))
	if err != nil {
		return req, err
	}
	d := settings.GameCreationDefaults
	if err := d.Validate(); err != nil {
		return req, errors.Join(ErrGameCreationDefaults, err)
	}
	if (d == nil || d.NarrativeStyleID == nil) && req.StoryTellerID == "" && (req.ModuleRefs == nil || req.ModuleRefs.NarrativeStyleID == "" && !req.ModuleRefs.NarrativeStyleDisabled) {
		index, err := s.store().Index()
		if err != nil {
			return req, err
		}
		for _, story := range index.Stories {
			if story.ID != index.CurrentStoryID || story.Preview {
				continue
			}
			refs := interactive.StoryDirectorModuleRefs{}
			if req.ModuleRefs != nil {
				refs = *req.ModuleRefs
			}
			refs.NarrativeStyleID = story.StoryTellerID
			if story.ModuleRefs != nil {
				refs.NarrativeStyleDisabled = story.ModuleRefs.NarrativeStyleDisabled
				if story.ModuleRefs.NarrativeStyleID != "" {
					refs.NarrativeStyleID = story.ModuleRefs.NarrativeStyleID
				}
			}
			req.ModuleRefs = &refs
			break
		}
	}
	if d == nil {
		return req, nil
	}
	return applyBookGameDefaults(cfg.DataDir(), req, d)
}

func applyBookGameDefaults(dataDir string, req interactive.CreateStoryRequest, d *config.GameCreationDefaults) (interactive.CreateStoryRequest, error) {
	refs := interactive.StoryDirectorModuleRefs{}
	if req.ModuleRefs != nil {
		refs = *req.ModuleRefs
		refs.EventPackageIDs = slices.Clone(refs.EventPackageIDs)
	}
	builtin := interactive.DefaultStoryDirectorModuleRefs()
	choose := func(field string, id *string, disabled *bool, value *string, fallback string, validate func(string) error) error {
		if *disabled {
			return nil
		}
		if *id == "" && value == nil {
			*id = fallback
			return nil
		}
		if *id == "" && *value == "" {
			*disabled = true
			return nil
		}
		if *id == "" {
			*id = *value
		}
		if err := validate(*id); err != nil {
			slog.Warn("[game-defaults] referenced resource unavailable", "field", field, "id", *id, "error", err)
			return fmt.Errorf("%w: %s", ErrGameCreationDefaults, field)
		}
		return nil
	}
	// The explicit narrator input is an existing API alias and outranks book defaults.
	if req.StoryTellerID != "" && refs.NarrativeStyleID == "" && !refs.NarrativeStyleDisabled {
		refs.NarrativeStyleID = req.StoryTellerID
	}
	for _, selection := range []struct {
		field    string
		id       *string
		disabled *bool
		value    *string
		fallback string
		validate func(string) error
	}{
		{"narrative_style_id", &refs.NarrativeStyleID, &refs.NarrativeStyleDisabled, d.NarrativeStyleID, builtin.NarrativeStyleID, func(id string) error { _, err := teller.NewLibrary(dataDir).Get(id); return err }},
		{"image_preset_id", &refs.ImagePresetID, &refs.ImagePresetDisabled, d.ImagePresetID, builtin.ImagePresetID, func(id string) error { _, err := imagepreset.NewLibrary(dataDir).Get(id); return err }},
		{"actor_state_id", &refs.ActorStateID, &refs.ActorStateDisabled, d.ActorStateID, builtin.ActorStateID, func(id string) error { _, err := interactive.NewActorStateLibrary(dataDir).Get(id); return err }},
		{"rule_system_id", &refs.RuleSystemID, &refs.RuleSystemDisabled, d.RuleSystemID, builtin.RuleSystemID, func(id string) error { _, err := interactive.NewRuleSystemLibrary(dataDir).Get(id); return err }},
	} {
		if err := choose(selection.field, selection.id, selection.disabled, selection.value, selection.fallback, selection.validate); err != nil {
			return req, err
		}
	}
	if refs.ActorStateDisabled && req.StateSchemaPolicy == nil {
		req.StateSchemaPolicy = &interactive.StoryStateSchemaPolicy{Mode: interactive.StoryStateSchemaModeGenerate}
	}
	if refs.EventPackageIDs == nil && !refs.EventPackagesDisabled {
		refs.EventPackageIDs = slices.Clone(builtin.EventPackageIDs)
		if d.EventPackageIDs != nil {
			refs.EventPackageIDs = slices.Clone(*d.EventPackageIDs)
			refs.EventPackagesDisabled = len(refs.EventPackageIDs) == 0
		}
	}
	if req.PlanningTemplateID == "" && d.PlanningTemplateID != nil {
		req.PlanningTemplateID = *d.PlanningTemplateID
	}
	if req.PlanningTemplateID != "" {
		if _, err := interactive.NewGamePlanningTemplateLibrary(dataDir).Get(req.PlanningTemplateID); err != nil {
			return req, fmt.Errorf("%w: planning_template_id", ErrGameCreationDefaults)
		}
	}
	if !refs.EventPackagesDisabled {
		for _, id := range refs.EventPackageIDs {
			if _, err := interactive.NewEventPackageLibrary(dataDir).Get(id); err != nil {
				return req, fmt.Errorf("%w: event_package_ids", ErrGameCreationDefaults)
			}
		}
	}
	if !refs.RuleSystemDisabled && !refs.ActorStateDisabled {
		rules, err := interactive.NewRuleSystemLibrary(dataDir).Get(refs.RuleSystemID)
		if err != nil {
			return req, fmt.Errorf("%w: rule_system_id", ErrGameCreationDefaults)
		}
		if rules.ActorStateID != "" && rules.ActorStateID != refs.ActorStateID {
			return req, fmt.Errorf("%w: rule and state templates differ", ErrGameCreationDefaults)
		}
	}
	if req.PresentationSettings == nil && d.DefaultBackground != nil {
		req.PresentationSettings = interactive.NormalizeStoryPresentationSettings(nil)
		if bg := d.DefaultBackground; bg.Mode == "image" {
			req.PresentationSettings.DefaultBackground = &interactive.PresentationMaterial{ItemID: bg.ItemID, AssetID: bg.AssetID}
		}
	}
	req.ModuleRefs = &refs
	return req, nil
}
