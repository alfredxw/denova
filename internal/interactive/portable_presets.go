package interactive

// PreparePortableEventPackage normalizes and validates portable author content
// before its digest becomes a native preset identity. It performs no writes.
func PreparePortableEventPackage(item EventPackageModule) (EventPackageModule, error) {
	if err := validateDirectorModuleWriteBounds(item.Name, item.Description); err != nil {
		return EventPackageModule{}, err
	}
	item = normalizeEventPackageModule(item)
	return item, validateEventPackageModule(item)
}

// PreparePortablePlanningTemplate uses the native editor's normalization and
// bounds without assigning timestamps, file paths or generated identities.
func PreparePortablePlanningTemplate(item GamePlanningTemplate) (GamePlanningTemplate, error) {
	if err := validateGamePlanningTemplateWriteBounds(item); err != nil {
		return GamePlanningTemplate{}, err
	}
	item = normalizeGamePlanningTemplate(item)
	return item, validateGamePlanningTemplateID(item.ID)
}

// PreparePortableStoryState validates a work's state and rule snapshots using
// the same native field, trait and rule-reference checks as opening adaptation.
func PreparePortableStoryState(system StoryDirectorActorStateSystem, rules StoryDirectorTRPGSystem) (*ActorStateSchemaSnapshot, error) {
	for _, rule := range rules.RuleTemplates {
		if err := validateRuleCheck(rule); err != nil {
			return nil, err
		}
	}
	normalized, _, err := ApplyActorStateSchemaAdaptation(system, rules, ActorStateSchemaAdaptation{})
	if err != nil {
		return nil, err
	}
	if err := validateActorTraitSystem(normalized); err != nil {
		return nil, err
	}
	return FreezeActorStateSchemaWithRules(normalized, rules, false), nil
}
