package interactive

const (
	ActorStateXiuxianID        = "xiuxian-state"
	ActorStateWesternFantasyID = "western-fantasy-state"
	ActorStateApocalypseID     = "apocalypse-state"
	ActorStateInfiniteFlowID   = "infinite-flow-state"

	ActorStateImportantCharacterTemplateID = "important_character"
	ActorStateOpponentTemplateID           = "opponent"
	ActorStateStoryContextTemplateID       = "story_context"
	DefaultStoryContextActorID             = "story"
)

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
	return actorStatePresetModule(
		ActorStateXiuxianID,
		"修仙状态系统",
		"面向修仙、问道、宗门、秘境和心魔因果的状态表模板集合；默认模板只是起点，可继续添加世界、势力、特定角色等自定义状态表。",
		[]ActorStateTemplate{
			actorStateTemplate("protagonist", "默认主角状态表", "记录主角当前可行动、可检定、可结算的修仙状态；用户可按作品需要新增更具体的状态表。", append(commonProtagonistStateFields(), []ActorStateField{
				textStateField(110, "cultivation.realm", "境界与修为", "记录当前境界、层次、瓶颈或突破状态。", "visible"),
				textStateField(120, "cultivation.practice_status", "修行状态", "记录闭关、破境、经脉受损、灵力紊乱等当前修行状态。", "visible"),
				textStateField(130, "cultivation.qi_status", "灵力状态", "记录灵力是否充盈、枯竭、暴走、受封等。", "visible"),
				textStateField(140, "cultivation.dao_heart", "心魔与道心", "记录道心动摇、心魔隐患、誓愿牵引等精神修行状态。", "spoiler"),
				textStateField(150, "story.karma", "因果牵连", "记录誓言、业力、人情债、师门因果等会影响后续选择的牵连。", "spoiler"),
				textStateField(160, "relations.sect_position", "宗门处境", "记录主角在宗门、家族、盟会或敌对势力中的当前处境。", "spoiler"),
				listStateField(170, "abilities.techniques", "功法与术法", "记录当前可承接的功法、术法、神通、禁术和使用限制。", "spoiler"),
			}...)),
			actorStateTemplate(ActorStateImportantCharacterTemplateID, "默认重要角色状态表", "记录反复登场角色当前会影响互动的修仙状态；特定角色线可以另建独立状态表。", append(commonImportantCharacterStateFields(), []ActorStateField{
				textStateField(110, "cultivation.realm", "境界与修为", "记录该角色已揭示或可推断的境界、压制力和修行状态。", "spoiler"),
				textStateField(120, "relations.faction", "所属势力", "记录宗门、家族、魔门、散修组织等当前身份归属。", "spoiler"),
				textStateField(130, "story.karma_with_protagonist", "与主角因果", "记录承诺、恩怨、师承、交易、命债等因果关系。", "spoiler"),
				listStateField(140, "knowledge.secrets_about_protagonist", "掌握的主角秘密", "记录该角色已经掌握且会影响后续互动的主角秘密。", "hidden"),
			}...)),
			actorStateTemplate(ActorStateOpponentTemplateID, "默认对抗对象状态表", "记录敌修、妖兽、心魔、秘境守卫等当前对抗状态；Boss、秘境或世界危机也可另建状态表。", append(commonOpponentStateFields(), []ActorStateField{
				textStateField(110, "cultivation.realm_pressure", "境界压制", "记录境界差距、威压、领域或规则压制。", "spoiler"),
				textStateField(120, "threat.demonic_qi_status", "妖力与魔气状态", "记录妖力、魔气、邪法、心魔污染等威胁来源。", "spoiler"),
				listStateField(130, "weakness.exploitable_clues", "可利用弱点", "记录玩家已发现或可合理利用的破绽、禁制、命门。", "spoiler"),
			}...)),
		},
	)
}

func westernFantasyActorStateModule() ActorStateModule {
	return actorStatePresetModule(
		ActorStateWesternFantasyID,
		"西幻状态系统",
		"面向剑与魔法、王国、地下城、教会神谕和异族盟约的状态表模板集合；默认模板只是起点，可继续添加世界、势力、基地、特定角色等自定义状态表。",
		[]ActorStateTemplate{
			actorStateTemplate("protagonist", "默认主角状态表", "记录主角当前可行动、可检定、可结算的西幻冒险状态；用户可按作品需要新增更具体的状态表。", append(commonProtagonistStateFields(), []ActorStateField{
				textStateField(110, "magic.status", "魔法状态", "记录法力、神术、元素亲和、施法受限或魔力异常。", "visible"),
				textStateField(120, "assets.equipment_status", "装备状态", "记录武器、护甲、法器、盾牌等当前可用性和损坏情况。", "visible"),
				listStateField(130, "effects.blessings_curses", "祝福与诅咒", "记录持续生效的祝福、诅咒、契约、神谕影响。", "spoiler"),
				textStateField(140, "relations.faction_position", "阵营处境", "记录主角在王国、教会、公会、族群或地下势力中的当前处境。", "spoiler"),
			}...)),
			actorStateTemplate(ActorStateImportantCharacterTemplateID, "默认重要角色状态表", "记录反复登场角色当前会影响互动的西幻状态；特定角色线可以另建独立状态表。", append(commonImportantCharacterStateFields(), []ActorStateField{
				textStateField(110, "identity.class_faction", "职业与阵营", "记录职业、骑士团、公会、教会、王国、族群等当前归属。", "spoiler"),
				textStateField(120, "magic.divine_status", "魔法与神术状态", "记录施法能力、神眷、禁魔、伤病对能力的影响。", "spoiler"),
				listStateField(130, "story.oaths_contracts", "契约与誓言", "记录骑士誓言、魔法契约、神明戒律、血誓等约束。", "spoiler"),
				textStateField(140, "relations.political_stance", "政治与宗教立场", "记录该角色对主角、王国、教会或关键事件的立场。", "spoiler"),
			}...)),
			actorStateTemplate(ActorStateOpponentTemplateID, "默认对抗对象状态表", "记录魔物、亡灵、Boss、敌对骑士等当前对抗状态；Boss、地下城或王国危机也可另建状态表。", append(commonOpponentStateFields(), []ActorStateField{
				listStateField(110, "threat.resistances", "抗性", "记录对武器、元素、神术、毒素等已知或推断抗性。", "spoiler"),
				listStateField(120, "weakness.exposed_weaknesses", "弱点", "记录被玩家确认或合理推断的弱点、禁忌和破防方式。", "spoiler"),
				textStateField(130, "threat.lair_domain", "巢穴与领域影响", "记录巢穴、地形、仪式场、领域效果对战局的影响。", "spoiler"),
			}...)),
		},
	)
}

func apocalypseActorStateModule() ActorStateModule {
	return actorStatePresetModule(
		ActorStateApocalypseID,
		"末世状态系统",
		"面向末世求生、感染异变、基地建设、资源稀缺和幸存者冲突的状态表模板集合；默认模板只是起点，可继续添加基地、世界危机、势力、特定角色等自定义状态表。",
		[]ActorStateTemplate{
			actorStateTemplate("protagonist", "默认主角状态表", "记录主角当前可行动、可检定、可结算的末世生存状态；用户可按作品需要新增更具体的状态表。", append(commonProtagonistStateFields(), []ActorStateField{
				textStateField(110, "survival.hunger_thirst_fatigue", "饥渴与疲劳", "记录饥饿、缺水、睡眠不足、体力透支等生存压力。", "visible"),
				textStateField(120, "conditions.infection_risk", "感染与污染风险", "记录咬伤、接触、污染源、潜伏症状等风险。", "spoiler"),
				objectStateField(130, "resources.survival_supplies", "生存资源", "记录食物、水、药品、燃料、电池等当前关键物资。", "visible"),
				objectStateField(140, "assets.weapons_ammo", "武器弹药", "记录武器、弹药、耐久、备用装备和限制。", "visible"),
				textStateField(150, "story.shelter_status", "避难所处境", "记录基地、车辆、临时营地或安全屋的当前安全状态。", "spoiler"),
			}...)),
			actorStateTemplate(ActorStateImportantCharacterTemplateID, "默认重要角色状态表", "记录反复登场角色当前会影响互动的末世状态；特定角色线可以另建独立状态表。", append(commonImportantCharacterStateFields(), []ActorStateField{
				textStateField(110, "conditions.injury_infection", "伤病与感染", "记录伤势、感染风险、药物依赖和行动限制。", "spoiler"),
				textStateField(120, "team.role", "团队分工", "记录该角色在队伍中的职责、技能、资源控制点。", "visible"),
				textStateField(130, "relations.trust_alertness", "信任与戒备", "记录该角色对主角或团队的信任、怀疑、背叛风险。", "spoiler"),
				textStateField(140, "resources.control", "物资控制权", "记录其掌握的物资、钥匙、路线、武器或基地权限。", "spoiler"),
			}...)),
			actorStateTemplate(ActorStateOpponentTemplateID, "默认对抗对象状态表", "记录感染者、变异体、敌对幸存者、尸潮等当前对抗状态；尸潮、基地或污染源也可另建状态表。", append(commonOpponentStateFields(), []ActorStateField{
				textStateField(110, "threat.sense_method", "感知方式", "记录听觉、嗅觉、视觉、热源、群体联动等追踪方式。", "spoiler"),
				textStateField(120, "conditions.infection_threat", "感染威胁", "记录咬伤、孢子、血液、精神污染等传播风险。", "spoiler"),
				textStateField(130, "threat.tracking_status", "追踪状态", "记录是否正在追踪主角、被诱导、失去目标或聚集。", "spoiler"),
				textStateField(140, "threat.group_scale", "群体规模", "记录尸潮、团伙、兽群或敌对据点的当前规模描述。", "spoiler"),
			}...)),
		},
	)
}

func infiniteFlowActorStateModule() ActorStateModule {
	return actorStatePresetModule(
		ActorStateInfiniteFlowID,
		"无限流状态系统",
		"面向副本规则、主线任务、积分结算、规则污染、队伍博弈和异常实体的状态表模板集合；默认模板只是起点，可继续添加副本、规则、故事倒计时、特定角色等自定义状态表。",
		[]ActorStateTemplate{
			actorStateTemplate("protagonist", "默认主角状态表", "记录主角当前可行动、可检定、可结算的无限流副本状态；用户可按作品需要新增更具体的状态表。", append(commonProtagonistStateFields(), []ActorStateField{
				textStateField(110, "instance.stage", "副本阶段", "记录刚进入、探索中、规则暴露、危机爆发、Boss 前夜、结算后等阶段。", "visible"),
				textStateField(120, "instance.main_task", "主线任务状态", "记录主线任务要求、当前进展、失败条件和结算压力。", "visible"),
				objectStateField(130, "resources.points", "积分与结算资源", "记录积分、奖励、扣罚、兑换资源等结算相关状态。", "visible"),
				listStateField(140, "assets.props", "道具", "记录当前持有且会影响副本行动的道具、限制和消耗。", "visible"),
				listStateField(150, "abilities.skills", "技能与临时能力", "记录技能、临时能力、副本赋予能力和使用代价。", "spoiler"),
				textStateField(160, "conditions.rule_contamination", "规则污染", "记录规则侵蚀、异常标记、认知偏差等风险。", "spoiler"),
				textStateField(170, "conditions.death_marks", "死亡标记", "记录死亡倒计时、失败标记、追杀标记等高危状态。", "hidden"),
			}...)),
			actorStateTemplate(ActorStateImportantCharacterTemplateID, "默认重要角色状态表", "记录反复登场角色当前会影响互动的无限流状态；特定角色线可以另建独立状态表。", append(commonImportantCharacterStateFields(), []ActorStateField{
				textStateField(110, "team.stance", "队伍立场", "记录合作、观望、背刺、交易、保护、竞争等队伍立场。", "spoiler"),
				listStateField(120, "abilities.exposed", "已暴露能力", "记录该角色已经被主角或队伍确认的能力、道具和限制。", "spoiler"),
				textStateField(130, "story.hidden_goal", "隐藏目标", "记录该角色已暴露或可推断的隐藏任务、私心或禁忌。", "hidden"),
				textStateField(140, "knowledge.rule_understanding", "对规则的理解", "记录该角色掌握、误解或隐瞒的副本规则。", "spoiler"),
			}...)),
			actorStateTemplate(ActorStateOpponentTemplateID, "默认对抗对象状态表", "记录副本怪物、规则实体、Boss、追杀者等当前对抗状态；副本、规则实体或结算系统也可另建状态表。", append(commonOpponentStateFields(), []ActorStateField{
				listStateField(110, "rules.triggers", "触发规则", "记录怪物或规则实体触发、追击、变强或退场的规则。", "spoiler"),
				listStateField(120, "rules.avoidance", "规避方式", "记录玩家已确认或合理推断的规避、欺骗、封印方式。", "spoiler"),
				textStateField(130, "rules.death_exit_condition", "死亡与退场条件", "记录击杀、封印、满足规则、拖延到时间点等退场条件。", "spoiler"),
			}...)),
		},
	)
}

func actorStatePresetModule(id, name, description string, templates []ActorStateTemplate) ActorStateModule {
	return normalizeActorStateModule(ActorStateModule{
		Version:     storyDirectorModuleVersion,
		ID:          id,
		Name:        name,
		Description: description,
		ActorState: StoryDirectorActorStateSystem{
			Templates:     appendStoryContextTemplate(templates),
			InitialActors: defaultActorStateInitialActors(),
		},
	})
}

func appendStoryContextTemplate(templates []ActorStateTemplate) []ActorStateTemplate {
	for _, template := range templates {
		if normalizeActorStateID(template.ID) == ActorStateStoryContextTemplateID {
			return templates
		}
	}
	return append(append([]ActorStateTemplate(nil), templates...), defaultStoryContextTemplate())
}

func defaultStoryContextTemplate() ActorStateTemplate {
	return actorStateTemplate(ActorStateStoryContextTemplateID, "默认故事上下文状态表", "记录当前故事线可行动、可检定、可承接的全局时间、地点、场景和压力。", commonStoryContextStateFields())
}

func defaultActorStateInitialActors() []ActorStateInitialActor {
	return []ActorStateInitialActor{
		{
			ID:         DefaultActorID,
			Name:       "主角",
			TemplateID: "protagonist",
			Role:       "protagonist",
		},
		{
			ID:          DefaultStoryContextActorID,
			Name:        "故事上下文",
			TemplateID:  ActorStateStoryContextTemplateID,
			Role:        "story_context",
			Description: "当前分支的剧内时间、地点、场景和全局压力状态。",
		},
	}
}

func actorStateTemplate(id, name, description string, fields []ActorStateField) ActorStateTemplate {
	return ActorStateTemplate{
		ID:          id,
		Name:        name,
		Description: description,
		Fields:      fields,
	}
}

func commonProtagonistStateFields() []ActorStateField {
	return []ActorStateField{
		textStateField(10, "current.body_status", "当前身体状态", "记录该状态对象本回合结束后的身体、伤病、疲劳和行动限制。", "visible"),
		textStateField(20, "current.mental_status", "当前精神与意志状态", "记录该状态对象的情绪、意志、压力、理智或心境变化。", "visible"),
		textStateField(30, "current.situation", "当前处境", "记录该状态对象当前场景中的危险、优势、限制和可行动空间。", "visible"),
		listStateField(40, "abilities.available", "当前可用能力", "记录本阶段可以直接影响行动的能力、手段和限制。", "visible"),
		objectStateField(50, "resources.current", "当前资源", "记录会影响后续行动或结算的资源，具体键和值由剧情决定。", "visible"),
		listStateField(60, "assets.key_items", "关键物品", "记录当前持有且会影响后续行动、检定或分支的物品。", "spoiler"),
		listStateField(70, "effects.ongoing", "持续影响", "记录仍在生效的伤势、增益、削弱、约束或承诺。", "spoiler"),
		listStateField(80, "risks.hidden", "隐藏风险", "记录不宜直接展示给玩家但需要后台承接的风险。", "hidden"),
	}
}

func commonStoryContextStateFields() []ActorStateField {
	return []ActorStateField{
		textStateField(10, "timeline.start_date", "故事开局日期", "记录当前故事线的剧内开局日期；初始化后除非用户明确重置开局时间，否则不要改动。", "visible"),
		textStateField(20, "scene.location", "当前详细地点", "记录主角或当前可玩视角所在的具体场景名称。", "visible"),
		textStateField(30, "scene.previous_time", "上轮场景时间", "记录上一轮交互结束时的剧内时间。", "visible"),
		textStateField(40, "scene.elapsed_time", "经过的时间", "记录当前时间相对上轮场景时间经过了多久。", "visible"),
		textStateField(50, "scene.current_time", "当前剧内时间", "记录当前回合结束后的剧内时间。", "visible"),
		textStateField(60, "scene.current_day", "当前天数", "记录从故事开局日期开始计算的剧内天数，例如第 1 天。", "visible"),
		textStateField(70, "scene.current_event", "当前事件", "记录本轮结束后仍影响下一轮的当前事件，不写下一步选项。", "visible"),
		textStateField(80, "scene.pressure", "当前场景压力", "记录当前场景最影响行动的危机、倒计时、限制或收益窗口。", "spoiler"),
		listStateField(90, "scene.available_hooks", "可承接钩子", "记录已经铺垫且可在后续行动中承接的信息点、冲突口或机会。", "spoiler"),
		listStateField(100, "rules.active_flags", "当前规则标记", "记录会影响后续检定或分支的布尔、枚举或规则标记。", "spoiler"),
	}
}

func commonImportantCharacterStateFields() []ActorStateField {
	return []ActorStateField{
		textStateField(10, "current.status", "当前状态", "记录该状态对象当前行为、处境、伤势、情绪基调或可互动状态。", "visible"),
		textStateField(20, "current.location", "当前地点与去向", "记录该状态对象当前地点、最后确认位置或下一步去向。", "visible"),
		textStateField(30, "relationship.attitude_to_protagonist", "对主角态度", "记录该状态对象当前面对主角的态度和依据。", "spoiler"),
		textStateField(40, "relationship.last_interaction", "与主角最近关键互动", "记录最近一次改变关系、信任、误会或冲突的互动。", "spoiler"),
		listStateField(50, "knowledge.known_about_protagonist", "已知主角信息", "记录该角色已经掌握且影响后续互动的主角信息。", "spoiler"),
		listStateField(60, "knowledge.unknown_about_protagonist", "对主角的误解与未知", "记录该角色仍在误解、怀疑、追查或不知道的主角信息。", "hidden"),
		listStateField(70, "assets.key_items", "持有关键物品", "记录该状态对象持有且会影响剧情、交易、战斗或线索的物品。", "spoiler"),
		textStateField(80, "current.goal_pressure", "当前目标与压力", "记录该状态对象当前最影响行动的目标、压力、倒计时或困境。", "spoiler"),
		listStateField(90, "effects.ongoing", "持续影响", "记录该状态对象身上仍在生效的伤势、增益、削弱、契约或风险。", "spoiler"),
	}
}

func commonOpponentStateFields() []ActorStateField {
	return []ActorStateField{
		textStateField(10, "threat.status", "威胁状态", "记录敌人、怪物或异常实体当前的威胁态势。", "visible"),
		textStateField(20, "conditions.damage", "当前伤势与削弱", "记录已造成的伤害、削弱、封印、控制或失衡状态。", "visible"),
		textStateField(30, "behavior.attack_tendency", "攻击倾向", "记录优先目标、攻击方式、撤退倾向或行为模式。", "spoiler"),
		listStateField(40, "abilities.special", "特殊能力", "记录已知或已展示的特殊能力、招式、异能和限制。", "spoiler"),
		listStateField(50, "weakness.limits", "弱点与限制", "记录已知或可利用的弱点、冷却、禁忌、条件限制。", "spoiler"),
		textStateField(60, "threat.target", "警戒与仇恨目标", "记录当前锁定、追踪、警戒或仇恨的对象。", "spoiler"),
		listStateField(70, "assets.recoverable", "战利品与可回收资源", "记录可能获得、回收、交易或用于推进的资源。", "spoiler"),
		textStateField(80, "threat.exit_condition", "退场条件", "记录击败、驱散、封印、谈判、逃离或暂时摆脱的条件。", "spoiler"),
	}
}

func textStateField(order int, path, name, description, visibility string) ActorStateField {
	return presetStateField(order, path, name, "string", description, visibility)
}

func listStateField(order int, path, name, description, visibility string) ActorStateField {
	return presetStateField(order, path, name, "list", description, visibility)
}

func objectStateField(order int, path, name, description, visibility string) ActorStateField {
	return presetStateField(order, path, name, "object", description, visibility)
}

func presetStateField(order int, path, name, fieldType, description, visibility string) ActorStateField {
	return ActorStateField{
		Path:              path,
		Name:              name,
		Type:              fieldType,
		Visibility:        visibility,
		Description:       description,
		UpdateInstruction: "只记录已经由正文、规则检定或后台导演确认且后续需要承接的状态；状态对象可以是角色、世界、故事、势力、基地、副本等；不要写一次性场景细节或无依据推测。",
		Order:             order,
	}
}
