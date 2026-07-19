package interactive

const (
	ActorStateXiuxianID        = "xiuxian-state"
	ActorStateWesternFantasyID = "western-fantasy-state"
	ActorStateApocalypseID     = "apocalypse-state"
	ActorStateInfiniteFlowID   = "infinite-flow-state"

	ActorStateImportantCharacterTemplateID = "important_character"
	ActorStateOpponentTemplateID           = "opponent"
	ActorStateStoryContextTemplateID       = "story_context"
	ActorStateWorldEntitiesTemplateID      = "world_entities"
	DefaultStoryContextActorID             = "story"
	DefaultWorldEntitiesActorID            = "world"
)

// actorStatePresetSpec contains only the deltas that genuinely vary by genre.
// All presets keep the same five-template boundary so genre additions do not
// reintroduce fragmented global ledgers.
type actorStatePresetSpec struct {
	ID                     string
	Name                   string
	Description            string
	PanelDescription       string
	PanelUpdateInstruction string
	PanelDefault           map[string]any
	StateDescription       string
	StateUpdateInstruction string
	StateDefault           map[string]any

	ProtagonistFields        []ActorStateField
	ImportantCharacterFields []ActorStateField
	OpponentFields           []ActorStateField
	StoryFields              []ActorStateField

	AbilityGuidance      string
	ItemGuidance         string
	RelationshipGuidance string
	QuestGuidance        string
	LocationGuidance     string
	FactionGuidance      string
}

func builtinActorStateModules() []ActorStateModule {
	return []ActorStateModule{
		DefaultActorStateModule(),
		xiuxianActorStateModule(),
		westernFantasyActorStateModule(),
		apocalypseActorStateModule(),
		infiniteFlowActorStateModule(),
	}
}

func builtinActorStateModuleByID(id string) (ActorStateModule, bool) {
	id = normalizeDirectorModuleID(id)
	for _, item := range builtinActorStateModules() {
		if item.ID == id {
			return item, true
		}
	}
	return ActorStateModule{}, false
}

func xiuxianActorStateModule() ActorStateModule {
	return actorStatePresetModule(xiuxianActorStatePresetSpec())
}

func westernFantasyActorStateModule() ActorStateModule {
	return actorStatePresetModule(westernFantasyActorStatePresetSpec())
}

func apocalypseActorStateModule() ActorStateModule {
	return actorStatePresetModule(apocalypseActorStatePresetSpec())
}

func infiniteFlowActorStateModule() ActorStateModule {
	return actorStatePresetModule(infiniteFlowActorStatePresetSpec())
}

func actorStatePresetModule(spec actorStatePresetSpec) ActorStateModule {
	return normalizeActorStateModule(ActorStateModule{
		Version:     storyDirectorModuleVersion,
		ID:          spec.ID,
		Name:        spec.Name,
		Description: spec.Description,
		ActorState:  actorStateSystemForPreset(spec),
	})
}

func actorStateSystemForPreset(spec actorStatePresetSpec) StoryDirectorActorStateSystem {
	return normalizeActorStateSystem(StoryDirectorActorStateSystem{
		Templates: []ActorStateTemplate{
			protagonistStateTemplate(spec),
			storyContextStateTemplate(spec),
			importantCharacterStateTemplate(spec),
			opponentStateTemplate(spec),
			worldEntitiesStateTemplate(spec),
		},
		InitialActors: defaultActorStateInitialActors(),
	})
}

func defaultActorStateInitialActors() []ActorStateInitialActor {
	return []ActorStateInitialActor{
		{
			ID:          DefaultActorID,
			Name:        "主角",
			TemplateID:  DefaultActorID,
			Role:        "protagonist",
			Description: "当前故事的可玩主角；身份、状态、技能、重要物品和关系集中维护在此 Actor。",
		},
		{
			ID:          DefaultStoryContextActorID,
			Name:        "故事状态",
			TemplateID:  ActorStateStoryContextTemplateID,
			Role:        "story_context",
			Description: "当前场景、世界局势与当前任务的统一状态对象。",
		},
		{
			ID:          DefaultWorldEntitiesActorID,
			Name:        "世界实体",
			TemplateID:  ActorStateWorldEntitiesTemplateID,
			Role:        "world",
			Description: "仍对当前故事有行动价值的重要地点与势力。",
		},
	}
}

func actorStatePresetTemplate(id, name, description string, fields []ActorStateField) ActorStateTemplate {
	return ActorStateTemplate{
		ID:          id,
		Name:        name,
		Description: description,
		Fields:      fields,
	}
}
