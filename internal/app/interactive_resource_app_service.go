package app

import (
	"denova/internal/imagepreset"
	"denova/internal/interactive"
)

func (a *App) InteractiveTellers() ([]interactive.Teller, error) {
	return a.interactiveService().InteractiveTellers()
}

func (s *InteractiveAppService) InteractiveTellers() ([]interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return nil, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.DataDir()).List()
}

func (a *App) InteractiveTeller(id string) (interactive.Teller, error) {
	return a.interactiveService().InteractiveTeller(id)
}

func (s *InteractiveAppService) InteractiveTeller(id string) (interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.Teller{}, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.DataDir()).Get(id)
}

func (a *App) CreateInteractiveTeller(teller interactive.Teller) (interactive.Teller, error) {
	return a.interactiveService().CreateInteractiveTeller(teller)
}

func (s *InteractiveAppService) CreateInteractiveTeller(teller interactive.Teller) (interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.Teller{}, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.DataDir()).Create(teller)
}

func (a *App) UpdateInteractiveTeller(id string, teller interactive.Teller, baseRevision ...string) (interactive.Teller, error) {
	return a.interactiveService().UpdateInteractiveTeller(id, teller, firstRevision(baseRevision))
}

func (s *InteractiveAppService) UpdateInteractiveTeller(id string, teller interactive.Teller, baseRevision string) (interactive.Teller, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.Teller{}, ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.DataDir()).Update(id, teller, baseRevision)
}

func (a *App) DeleteInteractiveTeller(id string) error {
	return a.interactiveService().DeleteInteractiveTeller(id)
}

func (s *InteractiveAppService) DeleteInteractiveTeller(id string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return ErrNoWorkspace
	}
	return interactive.NewTellerLibrary(cfg.DataDir()).Delete(id)
}

func (a *App) StoryDirectors() ([]interactive.StoryDirector, error) {
	return a.interactiveService().StoryDirectors()
}

func (s *InteractiveAppService) StoryDirectors() ([]interactive.StoryDirector, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return nil, ErrNoWorkspace
	}
	return interactive.NewStoryDirectorLibrary(cfg.DataDir()).List()
}

func (a *App) StoryDirector(id string) (interactive.StoryDirector, error) {
	return a.interactiveService().StoryDirector(id)
}

func (s *InteractiveAppService) StoryDirector(id string) (interactive.StoryDirector, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.StoryDirector{}, ErrNoWorkspace
	}
	return interactive.NewStoryDirectorLibrary(cfg.DataDir()).Get(id)
}

func (a *App) CreateStoryDirector(director interactive.StoryDirector) (interactive.StoryDirector, error) {
	return a.interactiveService().CreateStoryDirector(director)
}

func (s *InteractiveAppService) CreateStoryDirector(director interactive.StoryDirector) (interactive.StoryDirector, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.StoryDirector{}, ErrNoWorkspace
	}
	return interactive.NewStoryDirectorLibrary(cfg.DataDir()).Create(director)
}

func (a *App) UpdateStoryDirector(id string, director interactive.StoryDirector, baseRevision ...string) (interactive.StoryDirector, error) {
	return a.interactiveService().UpdateStoryDirector(id, director, firstRevision(baseRevision))
}

func (s *InteractiveAppService) UpdateStoryDirector(id string, director interactive.StoryDirector, baseRevision string) (interactive.StoryDirector, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.StoryDirector{}, ErrNoWorkspace
	}
	return interactive.NewStoryDirectorLibrary(cfg.DataDir()).Update(id, director, baseRevision)
}

func (a *App) DeleteStoryDirector(id string) error {
	return a.interactiveService().DeleteStoryDirector(id)
}

func (s *InteractiveAppService) DeleteStoryDirector(id string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return ErrNoWorkspace
	}
	return interactive.NewStoryDirectorLibrary(cfg.DataDir()).Delete(id)
}

func (a *App) EventPackages() ([]interactive.EventPackageModule, error) {
	return a.interactiveService().EventPackages()
}

func (s *InteractiveAppService) EventPackages() ([]interactive.EventPackageModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return nil, ErrNoWorkspace
	}
	return interactive.NewEventPackageLibrary(cfg.DataDir()).List()
}

func (a *App) EventPackage(id string) (interactive.EventPackageModule, error) {
	return a.interactiveService().EventPackage(id)
}

func (s *InteractiveAppService) EventPackage(id string) (interactive.EventPackageModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.EventPackageModule{}, ErrNoWorkspace
	}
	return interactive.NewEventPackageLibrary(cfg.DataDir()).Get(id)
}

func (a *App) CreateEventPackage(item interactive.EventPackageModule) (interactive.EventPackageModule, error) {
	return a.interactiveService().CreateEventPackage(item)
}

func (s *InteractiveAppService) CreateEventPackage(item interactive.EventPackageModule) (interactive.EventPackageModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.EventPackageModule{}, ErrNoWorkspace
	}
	return interactive.NewEventPackageLibrary(cfg.DataDir()).Create(item)
}

func (a *App) UpdateEventPackage(id string, item interactive.EventPackageModule, baseRevision ...string) (interactive.EventPackageModule, error) {
	return a.interactiveService().UpdateEventPackage(id, item, firstRevision(baseRevision))
}

func (s *InteractiveAppService) UpdateEventPackage(id string, item interactive.EventPackageModule, baseRevision string) (interactive.EventPackageModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.EventPackageModule{}, ErrNoWorkspace
	}
	return interactive.NewEventPackageLibrary(cfg.DataDir()).Update(id, item, baseRevision)
}

func (a *App) DeleteEventPackage(id string) error {
	return a.interactiveService().DeleteEventPackage(id)
}

func (s *InteractiveAppService) DeleteEventPackage(id string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return ErrNoWorkspace
	}
	return interactive.NewEventPackageLibrary(cfg.DataDir()).Delete(id)
}

func (a *App) RuleSystems() ([]interactive.RuleSystemModule, error) {
	return a.interactiveService().RuleSystems()
}

func (s *InteractiveAppService) RuleSystems() ([]interactive.RuleSystemModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return nil, ErrNoWorkspace
	}
	return interactive.NewRuleSystemLibrary(cfg.DataDir()).List()
}

func (a *App) RuleSystem(id string) (interactive.RuleSystemModule, error) {
	return a.interactiveService().RuleSystem(id)
}

func (s *InteractiveAppService) RuleSystem(id string) (interactive.RuleSystemModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.RuleSystemModule{}, ErrNoWorkspace
	}
	return interactive.NewRuleSystemLibrary(cfg.DataDir()).Get(id)
}

func (a *App) CreateRuleSystem(item interactive.RuleSystemModule) (interactive.RuleSystemModule, error) {
	return a.interactiveService().CreateRuleSystem(item)
}

func (s *InteractiveAppService) CreateRuleSystem(item interactive.RuleSystemModule) (interactive.RuleSystemModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.RuleSystemModule{}, ErrNoWorkspace
	}
	return interactive.NewRuleSystemLibrary(cfg.DataDir()).Create(item)
}

func (a *App) UpdateRuleSystem(id string, item interactive.RuleSystemModule, baseRevision ...string) (interactive.RuleSystemModule, error) {
	return a.interactiveService().UpdateRuleSystem(id, item, firstRevision(baseRevision))
}

func (s *InteractiveAppService) UpdateRuleSystem(id string, item interactive.RuleSystemModule, baseRevision string) (interactive.RuleSystemModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.RuleSystemModule{}, ErrNoWorkspace
	}
	return interactive.NewRuleSystemLibrary(cfg.DataDir()).Update(id, item, baseRevision)
}

func (a *App) DeleteRuleSystem(id string) error {
	return a.interactiveService().DeleteRuleSystem(id)
}

func (s *InteractiveAppService) DeleteRuleSystem(id string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return ErrNoWorkspace
	}
	return interactive.NewRuleSystemLibrary(cfg.DataDir()).Delete(id)
}

func (a *App) ActorStates() ([]interactive.ActorStateModule, error) {
	return a.interactiveService().ActorStates()
}

func (s *InteractiveAppService) ActorStates() ([]interactive.ActorStateModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return nil, ErrNoWorkspace
	}
	return interactive.NewActorStateLibrary(cfg.DataDir()).List()
}

func (a *App) ActorState(id string) (interactive.ActorStateModule, error) {
	return a.interactiveService().ActorState(id)
}

func (s *InteractiveAppService) ActorState(id string) (interactive.ActorStateModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.ActorStateModule{}, ErrNoWorkspace
	}
	return interactive.NewActorStateLibrary(cfg.DataDir()).Get(id)
}

func (a *App) CreateActorState(item interactive.ActorStateModule) (interactive.ActorStateModule, error) {
	return a.interactiveService().CreateActorState(item)
}

func (s *InteractiveAppService) CreateActorState(item interactive.ActorStateModule) (interactive.ActorStateModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.ActorStateModule{}, ErrNoWorkspace
	}
	return interactive.NewActorStateLibrary(cfg.DataDir()).Create(item)
}

func (a *App) UpdateActorState(id string, item interactive.ActorStateModule, baseRevision ...string) (interactive.ActorStateModule, error) {
	return a.interactiveService().UpdateActorState(id, item, firstRevision(baseRevision))
}

func (s *InteractiveAppService) UpdateActorState(id string, item interactive.ActorStateModule, baseRevision string) (interactive.ActorStateModule, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return interactive.ActorStateModule{}, ErrNoWorkspace
	}
	return interactive.NewActorStateLibrary(cfg.DataDir()).Update(id, item, baseRevision)
}

func (a *App) DeleteActorState(id string) error {
	return a.interactiveService().DeleteActorState(id)
}

func (s *InteractiveAppService) DeleteActorState(id string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return ErrNoWorkspace
	}
	return interactive.NewActorStateLibrary(cfg.DataDir()).Delete(id)
}

func (a *App) ImagePresets() ([]imagepreset.Preset, error) {
	return a.interactiveService().ImagePresets()
}

func (s *InteractiveAppService) ImagePresets() ([]imagepreset.Preset, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return nil, ErrNoWorkspace
	}
	return imagepreset.NewLibrary(cfg.DataDir()).List()
}

func (a *App) ImagePreset(id string) (imagepreset.Preset, error) {
	return a.interactiveService().ImagePreset(id)
}

func (s *InteractiveAppService) ImagePreset(id string) (imagepreset.Preset, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return imagepreset.Preset{}, ErrNoWorkspace
	}
	return imagepreset.NewLibrary(cfg.DataDir()).Get(id)
}

func (a *App) CreateImagePreset(preset imagepreset.Preset) (imagepreset.Preset, error) {
	return a.interactiveService().CreateImagePreset(preset)
}

func (s *InteractiveAppService) CreateImagePreset(preset imagepreset.Preset) (imagepreset.Preset, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return imagepreset.Preset{}, ErrNoWorkspace
	}
	return imagepreset.NewLibrary(cfg.DataDir()).Create(preset)
}

func (a *App) UpdateImagePreset(id string, preset imagepreset.Preset, baseRevision ...string) (imagepreset.Preset, error) {
	return a.interactiveService().UpdateImagePreset(id, preset, firstRevision(baseRevision))
}

func (s *InteractiveAppService) UpdateImagePreset(id string, preset imagepreset.Preset, baseRevision string) (imagepreset.Preset, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return imagepreset.Preset{}, ErrNoWorkspace
	}
	return imagepreset.NewLibrary(cfg.DataDir()).Update(id, preset, baseRevision)
}

func (a *App) DeleteImagePreset(id string) error {
	return a.interactiveService().DeleteImagePreset(id)
}

func (s *InteractiveAppService) DeleteImagePreset(id string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.DataDir() == "" {
		return ErrNoWorkspace
	}
	return imagepreset.NewLibrary(cfg.DataDir()).Delete(id)
}
