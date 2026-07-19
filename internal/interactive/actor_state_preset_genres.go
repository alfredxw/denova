package interactive

func xiuxianActorStatePresetSpec() actorStatePresetSpec {
	return actorStatePresetSpec{
		ID:                     ActorStateXiuxianID,
		Name:                   "修仙状态系统",
		Description:            "面向境界、灵根与特殊体质、功法法宝、宗门关系和天地异变；面板只保留设定真正量化的修行项，不套用通用六维。",
		PanelDescription:       "记录会影响检定的当前境界显示与作品明确量化的修行面板；灵根、特殊体质和血脉仍使用文本维护。",
		PanelUpdateInstruction: "默认只保留境界显示。只有作品明确提供数值规则时，才按基础值、当前值、修正说明增加神识、术法强度等检定项；不要自行把身法、体魄或根骨数值化。",
		PanelDefault:           map[string]any{"境界": map[string]any{"显示值": "未定", "修正说明": ""}},
		StateDescription:       "维护境界进度、灵力等动态资源，以及伤势、封印、增益、异常和功法法宝冷却。",
		StateUpdateInstruction: "固定使用资源、效果、冷却三个分区。境界进度使用当前值、上限与单位；其他资源只在设定存在时增加。伤势、封印和增益写入效果；功法、术法或法宝冷却使用对应 ability/item ID。",
		StateDefault: map[string]any{
			"资源": map[string]any{"境界进度": map[string]any{"当前值": float64(0), "上限": float64(100), "单位": "%"}},
			"效果": map[string]any{},
			"冷却": map[string]any{},
		},
		ProtagonistFields: []ActorStateField{
			textStateField("cultivation.foundation", "灵根与特殊体质", "记录灵根、血脉、特殊体质、道基等会决定修行路径的根本特征；没有对应设定时保持空白。", "visible", "题材设定", "block"),
		},
		ImportantCharacterFields: []ActorStateField{
			textStateField("cultivation.profile", "根基与传承", "记录已知灵根、特殊体质和主要传承；境界显示放入面板，伤势与临时异常放入状态，未知部分不估算。", "spoiler", "题材设定", "block"),
		},
		OpponentFields: []ActorStateField{
			textStateField("cultivation.threat_profile", "本相与传承特征", "记录已确认的妖身、法相、传承特征和境界压制方式；境界显示放入面板，具体招式放入技能与能力。", "spoiler", "题材设定", "block"),
		},
		StoryFields: []ActorStateField{
			textStateField("cultivation_world.state", "修行世界状态", "合并记录天地灵气、天道或飞升规则、宗门秩序及正在影响多地的修行界异变。", "spoiler", "题材状态", "block"),
		},
		AbilityGuidance:      "类型可使用功法、术法、神通、秘术、剑诀、身法或天赋；阶段与效果按正文设定表达，不换算成统一数值。",
		ItemGuidance:         "类型可使用法宝、法器、丹药、符箓、阵盘、灵材、灵石或传承物；记录品阶、认主、耐久或剩余次数等实际存在的状态。",
		RelationshipGuidance: "关系按师徒、同门、道侣、盟友、因果、恩怨、主从或敌对等修仙语义表达。",
		QuestGuidance:        "任务类型可使用修炼、突破、宗门、历练、秘境、因果或主线。",
		LocationGuidance:     "地点类型可使用洞府、宗门、坊市、秘境、禁地、城池、洞天或战场。",
		FactionGuidance:      "势力类型可使用宗门、世家、仙朝、魔门、妖族、商会、散修盟或上界势力。",
	}
}

func westernFantasyActorStatePresetSpec() actorStatePresetSpec {
	return actorStatePresetSpec{
		ID:                     ActorStateWesternFantasyID,
		Name:                   "西幻状态系统",
		Description:            "面向职业与超凡位阶、魔法神术、种族血脉、装备、阵营势力和冒险任务；面板只提供职业等级与 AC/DC 骨架，其他属性按作品规则增加。",
		PanelDescription:       "记录职业等级、作品实际采用的检定属性、攻击 AC 与防御 DC 等结算后有效值。",
		PanelUpdateInstruction: "职业等级、攻击 AC 与防御 DC 使用基础值、当前值、修正说明。力量等属性只有当前作品明确采用时才增加；装备、祝福或诅咒改变有效值时同步更新当前值与来源。",
		PanelDefault: map[string]any{
			"职业等级": panelNumber(1, "1–4 初阶；5–10 中阶；11–16 高阶；17 及以上传奇。作品另有等级规则时以作品为准。"),
			"攻击AC": panelNumber(10, "1–5 极低；6–9 偏低；10–13 常规；14–17 高；18 及以上极高。"),
			"防御DC": panelNumber(10, "1–5 极低；6–9 偏低；10–13 常规；14–17 高；18 及以上极高。"),
		},
		StateDescription:       "维护生命、法力或法术位等动态资源，以及祝福、诅咒、专注和技能物品冷却。",
		StateUpdateInstruction: "固定使用资源、效果、冷却三个分区。生命默认存在；法力、法术位和充能仅在设定采用时增加。祝福、诅咒与专注写入效果；能力和物品冷却使用对应 ability/item ID。",
		StateDefault: map[string]any{
			"资源": map[string]any{"生命": map[string]any{"当前值": float64(10), "上限": float64(10)}},
			"效果": map[string]any{},
			"冷却": map[string]any{},
		},
		ProtagonistFields: []ActorStateField{
			textStateField("fantasy.progression", "超凡来源与血脉契约", "记录魔法、神术或其他超凡来源，以及会持续影响能力的血脉与契约；职业等级放入面板。", "visible", "题材设定", "block"),
		},
		ImportantCharacterFields: []ActorStateField{
			textStateField("fantasy.profile", "超凡来源与阵营职责", "记录已知超凡来源、血脉契约和其在阵营中的职责；职业等级与检定项放入面板。", "spoiler", "题材设定", "block"),
		},
		OpponentFields: []ActorStateField{
			textStateField("fantasy.threat_profile", "生物特性与抗性", "记录已确认的种族或生物类型、抗性与免疫；阶位与检定项放入面板，具体能力放入技能与能力。", "spoiler", "题材设定", "block"),
		},
		StoryFields: []ActorStateField{
			textStateField("fantasy_world.order", "魔法、信仰与政治秩序", "合并记录当前世界的魔法环境、神祇或教会规则、王国秩序及跨地区冲突。", "spoiler", "题材状态", "block"),
		},
		AbilityGuidance:      "类型可使用职业能力、法术、神术、战技、专长、血脉能力或仪式；记录法术环阶、次数、冷却、专注等作品实际采用的限制。",
		ItemGuidance:         "类型可使用武器、护甲、饰品、药剂、卷轴、材料、货币或任务物；记录品质、充能、耐久、诅咒或绑定等实际状态。",
		RelationshipGuidance: "关系按家族、同伴、效忠、契约、教会、雇佣、盟友、竞争或敌对等语义表达。",
		QuestGuidance:        "任务类型可使用主线、委托、探索、阵营、誓约、讨伐或个人任务。",
		LocationGuidance:     "地点类型可使用王国、城市、村镇、城堡、地城、遗迹、荒野、位面或神域。",
		FactionGuidance:      "势力类型可使用王国、贵族、教会、公会、学院、商会、部族、军团或邪教。",
	}
}

func apocalypseActorStatePresetSpec() actorStatePresetSpec {
	return actorStatePresetSpec{
		ID:                     ActorStateApocalypseID,
		Name:                   "末世状态系统",
		Description:            "面向灾变求生、感染异变、稀缺资源、基地与幸存者冲突；不为饥饿、口渴、疲劳逐项造字段，也不预设通用六维。",
		PanelDescription:       "只记录当前作品规则明确量化、会参与生存或战斗检定的固定面板；没有数值规则时保持空 object。",
		PanelUpdateInstruction: "不要默认创建通用六维或生存属性。只有规则明确给出等级、命中、防护或专长数值时，才按基础值、当前值、修正说明增加对应项。",
		PanelDefault:           map[string]any{},
		StateDescription:       "集中维护生命、感染、污染、伤势、临时增益与技能物品冷却；普通生理压力只在确实影响行动时合并记录。",
		StateUpdateInstruction: "固定使用资源、效果、冷却三个分区。资源仅保留规则实际消耗的聚合资源；感染、污染、伤势与异常写入效果并注明阶段和影响；不为饥饿、口渴、疲劳各建一项。",
		StateDefault:           map[string]any{"资源": map[string]any{}, "效果": map[string]any{}, "冷却": map[string]any{}},
		ImportantCharacterFields: []ActorStateField{
			textStateField("survival.profile", "生存专长与职责", "记录关键生存专长，以及其在队伍或基地中的职责；感染、污染和伤势放入状态。", "spoiler", "题材设定", "block"),
		},
		OpponentFields: []ActorStateField{
			textStateField("survival.mutation_profile", "感染与变异特征", "合并记录传播方式、变异形态、感知方式和群体行为；具体攻击能力放入技能与能力。", "spoiler", "题材设定", "block"),
		},
		StoryFields: []ActorStateField{
			textStateField("apocalypse.situation", "灾变与生存局势", "合并记录灾变类型、感染或变异扩散、基础设施、区域污染和跨地点资源压力。", "spoiler", "题材状态", "block"),
			objectStateFieldWithInstruction("apocalypse.base", "基地状态", "基地存在时记录其当前运行状态；未建立基地时保持空 object。", "visible", "题材状态", "object 只写基地名称与地点ID、存续与安全状态、人员、关键储备、设施能力、当前威胁和紧急需求。安全状态使用崩溃、危险、勉强、稳定、安全等文字等级；删除基地时 replace 为空 object。"),
		},
		AbilityGuidance:      "类型可使用生存、医疗、维修、战斗、驾驶、侦察、制造或谈判。",
		ItemGuidance:         "类型可使用武器、弹药、食物、饮水、药品、燃料、工具、防具或任务物；记录数量、耐久、弹药、保质或污染等实际状态。",
		RelationshipGuidance: "关系按团队信任、资源互助、保护、服从、竞争、交易或敌对等末世语义表达。",
		QuestGuidance:        "任务类型可使用生存、搜救、补给、撤离、调查、基地、势力或主线。",
		LocationGuidance:     "地点类型可使用安全屋、基地、城市、郊区、野外、设施、道路或污染区。",
		FactionGuidance:      "势力类型可使用幸存者团队、聚居地、军方、公司、邪教、匪帮或感染群体。",
	}
}

func infiniteFlowActorStatePresetSpec() actorStatePresetSpec {
	return actorStatePresetSpec{
		ID:                     ActorStateInfiniteFlowID,
		Name:                   "无限流状态系统",
		Description:            "面向副本规则、任务结算、空间资源、规则污染、队伍博弈和异常实体；面板只承接空间明确公布的评级或属性。",
		PanelDescription:       "只记录轮回空间或作品规则明确公布、会参与检定的等级、评级或属性；没有公开量化规则时保持空 object。",
		PanelUpdateInstruction: "不要自行创建六维或战力评分。空间明确公布数值后，按基础值、当前值、修正说明维护；装备、血统与规则污染造成的修正必须注明来源。",
		PanelDefault:           map[string]any{},
		StateDescription:       "维护生命、精神、积分等作品实际采用的动态资源，以及规则污染、死亡标记、增益异常和技能物品冷却。",
		StateUpdateInstruction: "固定使用资源、效果、冷却三个分区。资源只增加当前作品明确采用的生命、精神或积分等项；规则污染与死亡标记写入效果；能力和道具冷却使用对应 ability/item ID。",
		StateDefault:           map[string]any{"资源": map[string]any{}, "效果": map[string]any{}, "冷却": map[string]any{}},
		ProtagonistFields: []ActorStateField{
			textStateField("infinite_space.profile", "空间身份", "记录权限、队伍身份和已完成副本等长期信息；评级放入面板，积分与规则污染放入状态。", "visible", "题材设定", "block"),
		},
		ImportantCharacterFields: []ActorStateField{
			textStateField("infinite_space.role", "队伍角色与空间身份", "合并记录当前队伍职责和空间权限；已公开的个人任务统一写入当前目标与处境。", "spoiler", "题材设定", "block"),
		},
		OpponentFields: []ActorStateField{
			textStateField("infinite_space.rule_profile", "触发、规避与规则影响", "合并记录触发条件、规避方式、规则领域和追击阶段；持续污染放入状态，只写已经确认或有充分线索支持的内容。", "spoiler", "题材设定", "block"),
		},
		StoryFields: []ActorStateField{
			textStateField("infinite_space.status", "轮回空间状态", "记录跨副本长期生效的空间规则、权限体系、结算秩序和当前整体局势。", "spoiler", "题材状态", "block"),
			objectStateFieldWithInstruction("infinite_space.current_instance", "当前副本", "记录当前副本的阶段、规则、时限和结算条件。", "visible", "题材状态", "object 只写副本名称、类型与难度、当前阶段与区域、剩余时间、任务ID、已确认规则、违规记录、核心威胁、结算和逃离条件。稳定程度使用稳定、波动、崩坏等文字状态；副本结算后 replace 为空 object。"),
		},
		AbilityGuidance:      "类型可使用主动技能、被动技能、血统、天赋、临时能力或职业能力；记录次数、冷却、积分代价和规则限制等实际存在的约束。",
		ItemGuidance:         "类型可使用道具、消耗品、诅咒物、线索物、兑换物、任务物或装备；记录剩余次数、绑定、污染和诅咒等实际状态。",
		RelationshipGuidance: "关系按合作、竞争、资源债务、救命债、队伍承诺或敌对等副本语义表达。",
		QuestGuidance:        "任务类型可使用副本主线、支线、隐藏、生存、团队、个人或结算。",
		LocationGuidance:     "地点类型可使用空间大厅、副本区域、房间、规则节点、安全区、禁区或出口。",
		FactionGuidance:      "势力类型可使用轮回队伍、空间组织、原住民阵营、规则实体或敌对小队。",
	}
}
