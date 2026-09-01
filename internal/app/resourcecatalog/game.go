package resourcecatalog

import "denova/internal/interactive"

func (s *Service) GamePlanningTemplates() ([]interactive.GamePlanningTemplate, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.planning.List()
}

func (s *Service) GamePlanningTemplate(id string) (interactive.GamePlanningTemplate, error) {
	if err := s.ready(); err != nil {
		return interactive.GamePlanningTemplate{}, err
	}
	return s.planning.Get(id)
}

func (s *Service) CreateGamePlanningTemplate(item interactive.GamePlanningTemplate) (interactive.GamePlanningTemplate, error) {
	if err := s.ready(); err != nil {
		return interactive.GamePlanningTemplate{}, err
	}
	return s.planning.Create(item)
}

func (s *Service) UpdateGamePlanningTemplate(id string, item interactive.GamePlanningTemplate, baseRevision string) (interactive.GamePlanningTemplate, error) {
	if err := s.ready(); err != nil {
		return interactive.GamePlanningTemplate{}, err
	}
	return s.planning.Update(id, item, baseRevision)
}

func (s *Service) DeleteGamePlanningTemplate(id string) error {
	if err := s.ready(); err != nil {
		return err
	}
	return s.planning.Delete(id)
}

func (s *Service) EventPackages() ([]interactive.EventPackageModule, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.eventPackages.List()
}

func (s *Service) EventPackage(id string) (interactive.EventPackageModule, error) {
	if err := s.ready(); err != nil {
		return interactive.EventPackageModule{}, err
	}
	return s.eventPackages.Get(id)
}

func (s *Service) CreateEventPackage(item interactive.EventPackageModule) (interactive.EventPackageModule, error) {
	if err := s.ready(); err != nil {
		return interactive.EventPackageModule{}, err
	}
	return s.eventPackages.Create(item)
}

func (s *Service) UpdateEventPackage(id string, item interactive.EventPackageModule, baseRevision string) (interactive.EventPackageModule, error) {
	if err := s.ready(); err != nil {
		return interactive.EventPackageModule{}, err
	}
	return s.eventPackages.Update(id, item, baseRevision)
}

func (s *Service) DeleteEventPackage(id string) error {
	if err := s.ready(); err != nil {
		return err
	}
	return s.eventPackages.Delete(id)
}

func (s *Service) RuleSystems() ([]interactive.RuleSystemModule, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.ruleSystems.List()
}

func (s *Service) RuleSystem(id string) (interactive.RuleSystemModule, error) {
	if err := s.ready(); err != nil {
		return interactive.RuleSystemModule{}, err
	}
	return s.ruleSystems.Get(id)
}

func (s *Service) CreateRuleSystem(item interactive.RuleSystemModule) (interactive.RuleSystemModule, error) {
	if err := s.ready(); err != nil {
		return interactive.RuleSystemModule{}, err
	}
	return s.ruleSystems.Create(item)
}

func (s *Service) UpdateRuleSystem(id string, item interactive.RuleSystemModule, baseRevision string) (interactive.RuleSystemModule, error) {
	if err := s.ready(); err != nil {
		return interactive.RuleSystemModule{}, err
	}
	return s.ruleSystems.Update(id, item, baseRevision)
}

func (s *Service) DeleteRuleSystem(id string) error {
	if err := s.ready(); err != nil {
		return err
	}
	return s.ruleSystems.Delete(id)
}

func (s *Service) ActorStates() ([]interactive.ActorStateModule, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.actorStates.List()
}

func (s *Service) ActorState(id string) (interactive.ActorStateModule, error) {
	if err := s.ready(); err != nil {
		return interactive.ActorStateModule{}, err
	}
	return s.actorStates.Get(id)
}

func (s *Service) CreateActorState(item interactive.ActorStateModule) (interactive.ActorStateModule, error) {
	if err := s.ready(); err != nil {
		return interactive.ActorStateModule{}, err
	}
	return s.actorStates.Create(item)
}

func (s *Service) UpdateActorState(id string, item interactive.ActorStateModule, baseRevision string) (interactive.ActorStateModule, error) {
	if err := s.ready(); err != nil {
		return interactive.ActorStateModule{}, err
	}
	return s.actorStates.Update(id, item, baseRevision)
}

func (s *Service) DeleteActorState(id string) error {
	if err := s.ready(); err != nil {
		return err
	}
	return s.actorStates.Delete(id)
}
