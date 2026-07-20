package interactive

import "fmt"

const actorStateFavorabilityBands = "0–20 敌视或强烈排斥；21–40 戒备或疏远；41–60 中立、熟悉或可合作；61–80 友好且愿意信任；81–90 亲近并会主动支持；91–100 深厚羁绊，除非发生根本冲突通常不会动摇。"

func defaultActorStatePresetSpec() actorStatePresetSpec {
	return actorStatePresetSpec{
		ID:                   DefaultActorStateModuleID,
		Name:                 "默认状态系统",
		Description:          "以故事、主角和世界实体三个初始 Actor 集中维护关键状态；默认角色采用 TRPG 检定面板与动态状态字段，均可直接分组展示和单独配置。",
		PanelFields:          defaultTRPGPanelFields(),
		StateFields:          defaultTRPGStateFields(),
		AbilityGuidance:      "类型使用战斗、社交、探索、制造或其他符合故事设定的分类。",
		ItemGuidance:         "类型使用装备、消耗品、资源、任务物、线索或其他符合故事设定的分类。",
		RelationshipGuidance: "关系类型按故事实际使用亲属、朋友、同伴、竞争、敌对、恋爱、主从或师徒等自然语言。",
		QuestGuidance:        "任务类型按故事实际使用主线、支线、个人、委托或世界事件。",
		LocationGuidance:     "地点类型按世界观使用城市、建筑、野外、遗迹、设施或房间等自然语言。",
		FactionGuidance:      "势力类型按世界观使用组织、国家、公司、教会、帮派或其他自然语言。",
	}
}

func protagonistStateTemplate(spec actorStatePresetSpec) ActorStateTemplate {
	fields := []ActorStateField{
		textStateField("identity.name", "姓名", "主角姓名；尚未确定时保持空白，确认后只在正式改名时更新。", "visible", "人物设定", "inline"),
		textStateField("identity.profile", "基本身份", "合并记录年龄、性别、职业、种族、社会身份等当前故事确实使用的信息。", "visible", "人物设定", "block"),
		textStateField("identity.appearance_style", "外貌与装扮", "合并记录有辨识度的外貌、衣着、装备外观和当前伪装；忽略无承接价值的临时描写。", "spoiler", "人物设定", "block"),
		textStateField("identity.character_background", "性格与背景", "记录会持续影响选择的性格倾向与已确认背景，不复述回合经历。", "spoiler", "人物设定", "block"),
	}
	fields = append(fields, actorPanelAndStateFields(spec)...)
	fields = append(fields, textStateField("current.situation", "当前处境", "记录当前优势、危险、限制和直接压力；具体地点由故事状态维护。", "visible", "状态", "block"))
	fields = append(fields, spec.ProtagonistFields...)
	fields = append(fields, actorOwnedRecordFields(spec, 12, 16, 12, "visible", "")...)
	return actorStatePresetTemplate(
		DefaultActorID,
		"主角状态表",
		"集中维护主角人物设定、检定面板、动态状态、技能、重要物品和有方向的关系；不假设 Lore 已包含完整角色资料。",
		fields,
	)
}

func importantCharacterStateTemplate(spec actorStatePresetSpec) ActorStateTemplate {
	fields := []ActorStateField{
		textStateField("identity.profile", "基本身份", "合并记录年龄、性别、职业、种族、组织与公开身份等有用信息。", "visible", "人物设定", "block"),
		textStateField("identity.appearance_style", "外貌与装扮", "合并记录辨识特征、常见装束、装备外观和当前伪装。", "spoiler", "人物设定", "block"),
		textStateField("identity.character_background", "性格与背景", "记录稳定性格、关键来历和会持续影响行为的背景。", "spoiler", "人物设定", "block"),
		textStateField("current.presence_location", "出场与位置", "用一句话记录在场、离场、失联、死亡等存续状态及当前位置或最后确认位置。", "visible", "状态", "inline"),
	}
	fields = append(fields, actorPanelAndStateFields(spec)...)
	fields = append(fields,
		textStateField("current.goal_situation", "当前目标与处境", "记录已经由行为、对话或设定确认的近期目标、压力、优势与限制，不猜测幕后计划。", "spoiler", "状态", "block"),
		textStateField("protagonist_relation.summary", "与主角关系", "记录关系类型、当前态度、关系阶段、重要承诺、边界和主要矛盾。", "visible", "主角关系", "block"),
		favorabilityStateField("protagonist_relation.favorability", "对主角好感度", "该角色对主角的总体情感倾向，不代表信任、服从或恋爱关系。", "visible", "主角关系"),
		listStateField("knowledge.about_protagonist", "对主角的已知信息", "只列会影响该角色判断或行动、且该角色确实已经得知的信息，最多 6 项。", "spoiler", "主角关系"),
	)
	fields = append(fields, spec.ImportantCharacterFields...)
	fields = append(fields, actorOwnedRecordFields(spec, 6, 6, 6, "spoiler", "对主角的关系已由专用字段维护，此处只记录该角色对其他 Actor 或势力的关系。")...)
	return actorStatePresetTemplate(
		ActorStateImportantCharacterTemplateID,
		"重要角色状态表",
		"每个反复登场的重要角色使用独立 Actor；重点维护当前状态、对主角好感与已知信息，以及自身技能、物品和其他关系。",
		fields,
	)
}

func opponentStateTemplate(spec actorStatePresetSpec) ActorStateTemplate {
	fields := []ActorStateField{
		textStateField("identity.profile", "类型与外观", "合并记录敌人、怪物、Boss、陷阱或异常实体的类型、身份与辨识特征。", "visible", "威胁", "block"),
		textStateField("current.lifecycle_location", "存续与位置", "用一句话记录活跃、撤退、被俘、失效、消灭等状态及所在或最后确认位置。", "visible", "威胁", "inline"),
		textStateFieldWithInstruction("threat.assessment", "威胁态势", "合并记录威胁级别、警戒或追击状态和当前目标。", "visible", "威胁", "block", "使用低、中、高、致命四档：低=通常无需专门准备；中=会造成明确损失；高=需要资源、协作或针对性策略；致命=正面对抗很可能导致死亡或不可逆后果。只有能力、数量、环境或目标发生实质变化时调整。"),
	}
	fields = append(fields, actorPanelAndStateFields(spec)...)
	fields = append(fields,
		textStateField("behavior.pattern", "行动模式", "记录已经观察或可靠确认的攻击倾向、触发方式和行为规律。", "spoiler", "状态", "block"),
		textStateField("weakness.exit_condition", "弱点与退场条件", "合并记录已确认弱点，以及击败、驱散、封印、谈判或摆脱条件。", "spoiler", "状态", "block"),
	)
	fields = append(fields, spec.OpponentFields...)
	fields = append(fields, actorOwnedRecordFields(spec, 6, 4, 4, "spoiler", "")...)
	return actorStatePresetTemplate(
		ActorStateOpponentTemplateID,
		"敌对对象状态表",
		"每个需要持续追踪的敌人、怪物、Boss 或异常实体使用独立 Actor；相近威胁信息合并维护。",
		fields,
	)
}

func storyContextStateTemplate(spec actorStatePresetSpec) ActorStateTemplate {
	fields := []ActorStateField{
		textStateField("scene.current_time", "当前时间", "按世界观记录当前日期、时段或阶段；只保留对行动有意义的精度。", "visible", "当前场景", "inline"),
		textStateField("scene.location", "当前详细地点", "用一个字段表达当前具体地点及必要的上级范围，不再拆分大区、区域和房间。", "visible", "当前场景", "inline"),
		textStateField("scene.current_event", "当前事件", "合并记录正在发生的事件、直接压力和下一步必须面对的问题。", "visible", "当前场景", "block"),
		listStateField("scene.present_actors", "在场角色", "使用 Actor 状态中已有的精确 ID；新建 Actor 直接使用故事语言中的角色名称，Actor 名称即 ID。只保留当前可感知或正在互动的角色。", "visible", "当前场景"),
		textStateFieldWithInstruction("scene.continuation_hook", "可承接钩子", "保留一个下一段可以直接承接的未完成动作、对话、发现或迫近威胁；它不是剧情总结或选项列表。", "visible", "当前场景", "block", "只写一个当前最直接的承接点。原钩子已经兑现、失效或被更强的新钩子替代时更新；使用具体对象与动作，不写泛泛的“继续探索”或多个备选项。"),
		textStateField("world.situation", "世界局势", "合并记录当前阶段、环境变化、跨场景威胁与倒计时；只写仍会影响后续剧情的变化。", "spoiler", "世界状态", "block"),
		objectStateFieldWithInstruction("tasks.current", "当前任务", "记录已经成立且仍需推进的任务。", "visible", "当前任务", questRecordUpdateInstruction(spec)),
	}
	fields = append(fields, spec.StoryFields...)
	return actorStatePresetTemplate(
		ActorStateStoryContextTemplateID,
		"故事状态表",
		"集中维护当前场景、世界局势和当前任务；不保存纪要、总结、行动选项或幕后计划。",
		fields,
	)
}

func worldEntitiesStateTemplate(spec actorStatePresetSpec) ActorStateTemplate {
	fields := []ActorStateField{
		objectStateFieldWithInstruction("world.locations", "地点记录", "记录仍有探索、通行、资源或剧情价值的重要地点。", "visible", "地点", locationRecordUpdateInstruction(spec)),
		objectStateFieldWithInstruction("world.factions", "势力记录", "记录仍在影响主角、任务或世界局势的重要势力。", "visible", "势力", factionRecordUpdateInstruction(spec)),
	}
	return actorStatePresetTemplate(
		ActorStateWorldEntitiesTemplateID,
		"世界实体表",
		"地点与势力集中在同一个世界 Actor 中，以两个 object 字段保持不同记录结构。",
		fields,
	)
}

func actorOwnedRecordFields(spec actorStatePresetSpec, abilityLimit, itemLimit, relationshipLimit int, visibility, relationshipScope string) []ActorStateField {
	return []ActorStateField{
		objectStateFieldWithInstruction("abilities.records", "技能与能力", "记录属于当前 Actor、会再次使用或仍有可变状态的能力。", visibility, "持有与关系", abilityRecordUpdateInstruction(spec, abilityLimit)),
		objectStateFieldWithInstruction("assets.important_items", "重要物品", "记录由当前 Actor 持有、会影响后续行动的物品。", visibility, "持有与关系", itemRecordUpdateInstruction(spec, itemLimit)),
		objectStateFieldWithInstruction("relations.records", "关系", "记录当前 Actor 对目标 Actor 或势力的有方向关系。", visibility, "持有与关系", relationshipRecordUpdateInstruction(spec, relationshipLimit, relationshipScope)),
	}
}

func abilityRecordUpdateInstruction(spec actorStatePresetSpec, limit int) string {
	return fmt.Sprintf("%s每项只写名称、类型、掌握或当前状态、效果、代价与限制、来源，最多 %d 条。删除记录时 replace 整个 object。%s", statePanelRecordNameIDInstruction("名称"), limit, spec.AbilityGuidance)
}

func itemRecordUpdateInstruction(spec actorStatePresetSpec, limit int) string {
	return fmt.Sprintf("%s当前 Actor 即持有者；每项只写名称、类型、数量或状态、作用、限制与来源，最多 %d 条。物品转移时同轮从原持有者移除并写入新持有者；删除记录时 replace 整个 object。%s", statePanelRecordNameIDInstruction("名称"), limit, spec.ItemGuidance)
}

func relationshipRecordUpdateInstruction(spec actorStatePresetSpec, limit int, scope string) string {
	instruction := fmt.Sprintf("键使用目标 Actor 或势力的精确 ID；新建目标直接使用故事语言中的名称，目标 Actor 或势力的名称即 ID，不得翻译成英文、转写拼音或生成 slug。只表示当前 Actor 对目标的单向关系，不自动镜像；每项只写关系类型、阶段、好感度、当前态度、边界或承诺、主要矛盾，最多 %d 条。好感度区间：%s 普通同场或闲聊不自动增加；删除记录时 replace 整个 object。%s", limit, actorStateFavorabilityBands, spec.RelationshipGuidance)
	if scope != "" {
		instruction += " " + scope
	}
	return instruction
}

func questRecordUpdateInstruction(spec actorStatePresetSpec) string {
	return statePanelRecordNameIDInstruction("任务名称") + "每项只写任务名称、类型、状态、目标与进度、时限、明确的奖惩和关联对象名称/ID，最多 8 条；关联对象引用已有状态面板名称/ID。任务结算且结果已进入正文后删除；删除记录时 replace 整个 object。" + spec.QuestGuidance
}

func locationRecordUpdateInstruction(spec actorStatePresetSpec) string {
	return statePanelRecordNameIDInstruction("地点名称") + "每项只写地点名称、类型、所属范围、当前状态、风险、关键特征与通路，最多 16 条。风险使用低、中、高、致命等文字等级，不保存画布坐标；删除记录时 replace 整个 object。" + spec.LocationGuidance
}

func factionRecordUpdateInstruction(spec actorStatePresetSpec) string {
	return statePanelRecordNameIDInstruction("势力名称") + "每项只写势力名称、类型、核心特征、存续状态、对主角立场、主要范围和当前行动，最多 8 条。删除记录时 replace 整个 object。" + spec.FactionGuidance
}

func statePanelRecordNameIDInstruction(nameField string) string {
	return fmt.Sprintf("键直接使用故事语言中的“%s”，名称即 ID；不得翻译成英文、转写拼音或生成 slug。", nameField)
}

func actorPanelAndStateFields(spec actorStatePresetSpec) []ActorStateField {
	fields := make([]ActorStateField, 0, len(spec.PanelFields)+len(spec.StateFields))
	fields = append(fields, spec.PanelFields...)
	fields = append(fields, spec.StateFields...)
	return fields
}

func defaultTRPGPanelFields() []ActorStateField {
	fields := []ActorStateField{
		scaledNumberStateField("panel.level", "等级", "角色当前规则等级。", "面板", 1, 1, 20, "1–4 初阶；5–10 中阶；11–16 高阶；17–20 传奇。", "等级只随明确的升级、降级或规则结算改变。"),
	}
	for _, attribute := range []struct {
		path string
		name string
	}{
		{path: "panel.strength", name: "力量"},
		{path: "panel.dexterity", name: "敏捷"},
		{path: "panel.constitution", name: "体质"},
		{path: "panel.intelligence", name: "智力"},
		{path: "panel.wisdom", name: "感知"},
		{path: "panel.charisma", name: "魅力"},
	} {
		fields = append(fields, scaledNumberStateField(attribute.path, attribute.name, "参与相关检定的有效属性值，已经包含装备与持续状态修正。", "面板", 10, 1, 30, "1–5 极弱；6–9 偏弱；10–11 常人；12–15 优秀；16–19 卓越；20–30 超凡。", "基础能力、装备或持续效果改变时更新有效值；临时一次性加值不写入。"))
	}
	return append(fields,
		scaledNumberStateField("panel.attack_ac", "攻击 AC", "参与攻击相关检定的有效值。", "面板", 10, 0, 30, "0–5 极低；6–9 偏低；10–13 常规；14–17 高；18–30 极高。", "装备、等级或持续效果改变攻击结算时更新；一次性加值不写入。"),
		scaledNumberStateField("panel.defense_dc", "防御 DC", "参与防御相关检定的有效值。", "面板", 10, 0, 30, "0–5 极低；6–9 偏低；10–13 常规；14–17 高；18–30 极高。", "装备、等级或持续效果改变防御结算时更新；一次性加值不写入。"),
	)
}

func defaultTRPGStateFields() []ActorStateField {
	return []ActorStateField{
		textStateFieldWithDefaultInstruction("state.health", "生命", "使用“当前值/上限”表达可恢复的生命资源；例如 8/12。", "visible", "状态", "inline", "10/10", "受伤、治疗或上限改变时更新；没有生命数值规则时改用作品中的等价状态。"),
		textStateFieldWithDefaultInstruction("state.mana", "法力", "使用“当前值/上限”表达法力、能量或等价施法资源；例如 3/6。", "visible", "状态", "inline", "0/0", "消耗、恢复或上限改变时更新；作品没有此类资源时保持 0/0。"),
		listStateFieldWithInstruction("state.effects", "持续效果", "只列仍在生效且会影响后续行动的增益、减益、异常或伤势。", "visible", "状态", "每项使用“名称｜影响｜剩余条件或时长”；结束后立即移除，瞬时效果不保留。"),
		listStateFieldWithInstruction("state.cooldowns", "冷却状态", "只列尚未恢复的技能或物品及其剩余冷却。", "visible", "状态", "每项使用稳定技能或物品名称及剩余回合、时间或恢复条件；冷却结束后移除。"),
	}
}

func textStateField(path, name, description, visibility, group, display string) ActorStateField {
	return presetStateField(path, name, "string", description, visibility, group, display)
}

func textStateFieldWithInstruction(path, name, description, visibility, group, display, instruction string) ActorStateField {
	field := textStateField(path, name, description, visibility, group, display)
	field.UpdateInstruction = instruction
	return field
}

func textStateFieldWithDefaultInstruction(path, name, description, visibility, group, display, defaultValue, instruction string) ActorStateField {
	field := textStateFieldWithInstruction(path, name, description, visibility, group, display, instruction)
	field.Default = defaultValue
	return field
}

func listStateField(path, name, description, visibility, group string) ActorStateField {
	field := presetStateField(path, name, "list", description, visibility, group, "list")
	field.Default = []any{}
	return field
}

func listStateFieldWithInstruction(path, name, description, visibility, group, instruction string) ActorStateField {
	field := listStateField(path, name, description, visibility, group)
	field.UpdateInstruction = instruction
	return field
}

func objectStateFieldWithInstruction(path, name, description, visibility, group, instruction string) ActorStateField {
	field := presetStateField(path, name, "object", description, visibility, group, "")
	field.Default = map[string]any{}
	field.UpdateInstruction = instruction
	return field
}

func favorabilityStateField(path, name, description, visibility, group string) ActorStateField {
	field := presetStateField(path, name, "number", description+" 区间含义："+actorStateFavorabilityBands, visibility, group, "stat")
	field.Min = presetFloatPointer(0)
	field.Max = presetFloatPointer(100)
	field.UpdateInstruction = "只有会改变双方关系的有效互动或事件才调整；一般事件变化 1–5，重大选择变化 6–15，跨越区间必须有明确剧情依据。"
	return field
}

func scaledNumberStateField(path, name, description, group string, defaultValue, minValue, maxValue float64, scale, instruction string) ActorStateField {
	field := presetStateField(path, name, "number", description+" 数值区间："+scale, "visible", group, "stat")
	field.Default = defaultValue
	field.Min = presetFloatPointer(minValue)
	field.Max = presetFloatPointer(maxValue)
	field.UpdateInstruction = instruction
	return field
}

func presetStateField(path, name, fieldType, description, visibility, group, display string) ActorStateField {
	return ActorStateField{
		Path:        path,
		Name:        name,
		Type:        fieldType,
		Visibility:  visibility,
		Description: description,
		Group:       group,
		Display:     display,
	}
}

func presetFloatPointer(value float64) *float64 {
	return &value
}
