package interactive

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func (s *Store) SaveStoryMemoryStructure(storyID string, req StoryMemoryStructureRequest) (StoryMemoryStructure, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, _, err := s.readStoryLocked(storyID); err != nil {
		return StoryMemoryStructure{}, err
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return StoryMemoryStructure{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	structure := normalizeStoryMemoryStructure(req, now)
	if structure.ID == "" {
		structure.ID = newID("sm")
	}
	if err := validateStoryMemoryStructure(structure); err != nil {
		return StoryMemoryStructure{}, err
	}
	for i := range book.Structures {
		if book.Structures[i].ID != structure.ID {
			continue
		}
		if book.Structures[i].ReadOnly {
			return StoryMemoryStructure{}, fmt.Errorf("故事记忆结构为只读派生表，不能编辑: %s", structure.ID)
		}
		structure.BuiltIn = book.Structures[i].BuiltIn
		structure.ReadOnly = book.Structures[i].ReadOnly
		structure.Derived = book.Structures[i].Derived
		structure.CreatedAt = firstMemoryText(book.Structures[i].CreatedAt, now)
		structure.UpdatedAt = now
		book.Structures[i] = structure
		if err := s.writeMemoryBookLocked(storyID, book); err != nil {
			return StoryMemoryStructure{}, err
		}
		return structure, nil
	}
	structure.CreatedAt = now
	structure.UpdatedAt = now
	book.Structures = append(book.Structures, structure)
	sortStoryMemoryStructures(book.Structures)
	if err := s.writeMemoryBookLocked(storyID, book); err != nil {
		return StoryMemoryStructure{}, err
	}
	return structure, nil
}

func (s *Store) DeleteStoryMemoryStructure(storyID, structureID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, _, err := s.readStoryLocked(storyID); err != nil {
		return err
	}
	structureID = strings.TrimSpace(structureID)
	if structureID == "" {
		return fmt.Errorf("故事记忆结构 ID 不能为空")
	}
	book, err := s.readMemoryBookLocked(storyID)
	if err != nil {
		return err
	}
	next := book.Structures[:0]
	removed := false
	for _, structure := range book.Structures {
		if structure.ID == structureID {
			if structure.ReadOnly {
				return fmt.Errorf("故事记忆结构为只读派生表，不能删除: %s", structureID)
			}
			removed = true
			continue
		}
		next = append(next, structure)
	}
	if !removed {
		return fmt.Errorf("故事记忆结构不存在: %s", structureID)
	}
	book.Structures = next
	for i := range book.Records {
		if book.Records[i].StructureID == structureID {
			book.Records[i].Archived = true
			book.Records[i].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
	}
	return s.writeMemoryBookLocked(storyID, book)
}

func normalizeMemoryBook(book interactiveMemoryBook) interactiveMemoryBook {
	if book.V <= 0 {
		book.V = 1
	}
	book.Settings = normalizeStoryMemorySettings(book.Settings, book.V)
	if len(book.Structures) == 0 {
		book.Structures = defaultStoryMemoryStructures()
	} else {
		book.Structures = refreshBuiltInStoryMemoryStructures(book.Structures)
	}
	if len(book.Records) == 0 && len(book.Entries) > 0 {
		book.Records = migrateInteractiveEntriesToStoryMemoryRecords(book.Entries)
	}
	for i := range book.Structures {
		book.Structures[i] = normalizeStoryMemoryStructureFromStored(book.Structures[i])
	}
	sortStoryMemoryStructures(book.Structures)
	if book.Records == nil {
		book.Records = []StoryMemoryRecord{}
	}
	return book
}

func normalizeStoryMemorySettings(settings StoryMemorySettings, version int) StoryMemorySettings {
	if settings.AutoIntervalTurns <= 0 {
		settings.AutoIntervalTurns = defaultStoryMemoryInterval
	}
	if version < 2 && !settings.Enabled {
		settings.Enabled = true
	}
	return settings
}

func normalizeStoryMemoryInterval(value int) int {
	if value <= 0 {
		return defaultStoryMemoryInterval
	}
	if value > 50 {
		return 50
	}
	return value
}

func defaultStoryMemoryStructures() []StoryMemoryStructure {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	structures := []StoryMemoryStructure{
		defaultStoryMemoryStructure("current_state", "当前状态", "旧版派生摘要：记录当前剧情线的全局时间、地点和场景状态。默认关闭；运行态真源已迁移到状态系统的 story_context。", "仅在用户显式启用旧版派生摘要时维护；当前时间、地点、天数和当前事件必须先写入状态系统 story_context，不能把此表作为状态真源。", "singleton", "", false, 10, []StoryMemoryField{
			defaultStoryMemoryField("story_start_date", "故事开局日期", "当前故事线的剧内开局日期。", "格式尽量使用 YYYY-MM-DD；初始化后除非用户明确重置开局时间，否则不要改动。", true, 10),
			defaultStoryMemoryField("location", "当前详细地点", "主角当前所在的具体场景名称。", "填写具体场景名，不要只写宽泛区域。", true, 20),
			defaultStoryMemoryField("previous_time", "上轮场景时间", "上一轮交互结束时的剧内时间。", "格式尽量与当前时间一致；首轮没有上一轮时填本轮开始前的合理时间。", true, 30),
			defaultStoryMemoryField("elapsed_time", "经过的时间", "当前时间相对上轮场景时间经过了多久。", "用自然语言描述时间跨度，例如“几分钟”“半个时辰”“两天”。", true, 40),
			defaultStoryMemoryField("time", "当前时间", "当前回合结束后的剧内时间。", "必须填写明确日期和时间；若正文未给出，需根据世界观和场景推进推定。", true, 50),
			defaultStoryMemoryField("current_day", "当前天数", "从故事开局日期开始计算的剧内天数。", "开局日期当天为第 1 天；只有剧内日期跨日时才变化。", true, 60),
			defaultStoryMemoryField("event", "当前事件", "当前场景正在承接的核心事件。", "一句话写清本轮结束后仍影响下一轮的当前事件，不写下一步选项。", false, 70),
		}, now),
		defaultStoryMemoryStructure("protagonist", "主角信息", "记录主角的核心身份、背景、长期经历和叙事承接信息。此表有且仅有一行。", "只记录主角长期叙事事实；可计算资源、持续状态、当前位置、当前伤势、关系数值和规则标记必须写入状态系统，不得以本表替代 Actor State 真源。", "singleton", "", true, 20, []StoryMemoryField{
			defaultStoryMemoryField("name", "人物名称", "主角的名字或稳定称呼。", "使用剧情中最稳定的称呼。", true, 10),
			defaultStoryMemoryField("gender_age", "性别/年龄", "主角的性别和年龄或年龄阶段。", "没有明确年龄时根据设定给出合理阶段或估计年龄。", false, 20),
			defaultStoryMemoryField("appearance", "外貌特征", "主角相对稳定的外貌特征。", "只写客观可观察特征；不要把临时姿态、表情或单轮状态写成外貌。", false, 30),
			defaultStoryMemoryField("identity", "职业/身份", "主角在社会、组织或世界规则中的主要身份。", "填写当前最主要身份，可包含门派、职位、阶层、公开身份或隐藏身份。", false, 40),
			defaultStoryMemoryField("current_condition", "叙事近况", "主角近期处境的叙事摘要，不作为可计算状态。", "只写长期需要承接的一句话近况；当前伤势、压力、资源、位置等运行态变化必须写入状态系统。", false, 50),
			defaultStoryMemoryField("location", "长期关联地点", "主角稳定居所、据点或长期关联地点。", "不要用本字段追踪当前所在地点；当前地点必须写入状态系统 story_context 或对应状态对象。", false, 60),
			defaultStoryMemoryField("abilities", "稳定能力/特有能力", "主角长期稳定能力、身份能力或已确认特殊能力。", "用分号分隔多项；只记录长期设定和已证实能力，临时增益、资源数值和检定修正写入状态系统。", false, 70),
			defaultStoryMemoryField("skills", "技能列表", "主角掌握的技能。", "按“技能名称｜类型｜等级/阶段｜效果”维护多项；无技能时留空。", false, 80),
			defaultStoryMemoryField("items", "重要物品/资源", "主角持有的重要物品、装备、资源或线索。", "按“名称｜数量/规模｜用途/意义｜状态”维护多项；只记录会影响后续剧情的内容。", false, 90),
			defaultStoryMemoryField("relationships", "与其他人物关系", "主角与重要角色之间的关系和最近互动。", "每行一个人物，格式建议“人物：关系及最近关键互动”；只写已发生或已证实内容。", false, 100),
			defaultStoryMemoryField("experience", "关键经历", "主角背景故事和剧情推进后的关键经历。", "随剧情增量更新，不超过 400 字；超过时压缩，只保留影响后续剧情的事实。", false, 110),
		}, now),
		defaultStoryMemoryStructure("important_character", "重要角色", "记录会影响后续剧情的关键角色档案和长期叙事事实。", "每个关键角色一行；只记录身份、关系来由、长期经历和后续承接事实。当前伤势、当前位置、态度、目标压力等运行态变化应写入状态系统。", "keyed", "name", true, 30, []StoryMemoryField{
			defaultStoryMemoryField("name", "姓名", "角色姓名或稳定称呼。", "使用角色最稳定的正式姓名或常用称呼。", true, 10),
			defaultStoryMemoryField("gender_age", "性别/年龄", "角色性别和年龄或年龄阶段。", "没有明确年龄时根据设定给出合理阶段或估计年龄。", false, 20),
			defaultStoryMemoryField("brief", "一句话介绍", "角色身份背景的一句话概括。", "不超过 30 字；只写身份背景，不写好坏强弱等主观评价。", false, 30),
			defaultStoryMemoryField("appearance", "外貌特征", "角色相对稳定的外貌特征。", "只写客观可观察特征；临时衣着、姿态和表情放到当前状态。", false, 40),
			defaultStoryMemoryField("identity", "身份", "角色职业、阵营、社会身份或剧情身份。", "只写已设定或已揭示身份；疑似身份写明待确认。", false, 50),
			defaultStoryMemoryField("location", "长期关联地点", "角色稳定居所、势力范围或长期关联地点。", "不要用本字段追踪实时位置；当前地点或最后确认去向应写入状态系统的重要角色状态对象。", false, 60),
			defaultStoryMemoryField("current_status", "叙事近况", "角色近期对剧情有长期影响的处境摘要。", "只写长期需要承接的事实；当前行为、伤势、情绪基调、目标压力等运行态写入状态系统。", false, 70),
			defaultStoryMemoryField("relationship_to_protagonist", "与主角关系", "该角色与主角的关系和最近关键互动。", "避免只写“朋友/敌人”等标签，要补一句具体依据。", false, 80),
			defaultStoryMemoryField("relationships", "与其他重要角色关系", "该角色与其他重要角色的关系网络。", "每行一个人物，格式建议“人物：关系及最近互动”；只写已接触或已设定关系。", false, 90),
			defaultStoryMemoryField("known_about_protagonist", "对主角已知信息", "该角色已经知道的主角相关情报。", "上限 5 项，保留最影响后续互动的情报。", false, 100),
			defaultStoryMemoryField("unknown_about_protagonist", "对主角未知/误解", "该角色仍想探明或误解的主角相关情报。", "上限 5 项；没有明确误解或未知点时留空。", false, 110),
			defaultStoryMemoryField("important_items", "持有关键物品", "角色持有的重要物品、资源或线索。", "多项用分号分隔；只记录关键物品。", false, 120),
			defaultStoryMemoryField("experience", "关键经历", "角色背景与登场后的关键经历。", "随剧情增量更新，不超过 350 字；超过时压缩，只保留影响后续剧情的事实。", false, 130),
			defaultStoryMemoryField("left_scene", "是否离场", "该角色是否已经离开当前可互动场景。", "只能填写“是”或“否”。", false, 140),
		}, now),
		defaultStoryMemoryStructure("world_context", "世界上下文", "记录地点、势力、组织、阵营、关键场景和世界规则节点的长期叙事事实。", "本表记录外部结构如何影响剧情，不重复记录角色完整档案；可计算世界状态、倒计时、当前危机压力和规则标记写入状态系统。", "keyed", "name", true, 40, []StoryMemoryField{
			defaultStoryMemoryField("name", "节点名称", "地点、势力、组织、规则或关键场景名称。", "使用稳定可复用名称。", true, 10),
			defaultStoryMemoryField("type", "节点类型", "节点类别。", "可填地点、势力、组织、规则、场景、家族、阵营等。", true, 20),
			defaultStoryMemoryField("scope", "所属范围", "上级区域、所属世界、阵营范围或适用范围。", "没有明确范围时填“未知”或留空。", false, 30),
			defaultStoryMemoryField("description", "描述", "该节点的性质、环境、规则或结构说明。", "写对后续剧情有用的事实，不写百科式长篇设定。", false, 40),
			defaultStoryMemoryField("related_characters", "相关角色", "与该节点有关的重要角色。", "多名角色用分号分隔。", false, 50),
			defaultStoryMemoryField("plot_relation", "与主角/主线关系", "该节点如何影响主角、主线或关键关系。", "写推动、阻碍、保护、监视、误导、交易、压迫等具体作用。", false, 60),
			defaultStoryMemoryField("stance", "当前立场", "节点对主角或当前事件的立场。", "没有明确立场时填“未知”。", false, 70),
			defaultStoryMemoryField("status", "当前状态", "节点当前状态。", "记录开放、封锁、覆灭、隐藏、紧张、待调查等状态。", false, 80),
		}, now),
		defaultStoryMemoryStructure("open_threads", "进行中事项", "记录任务、备忘录、承诺、伏笔、计划、未解决误会和待办。", "只维护仍需后续承接的事项；结束、失效或不再参与判断时归档。", "keyed", "title", true, 50, []StoryMemoryField{
			defaultStoryMemoryField("title", "标题", "事项的稳定短标题。", "用可复用短标题，不要每轮改名。", true, 10),
			defaultStoryMemoryField("type", "事项类型", "任务、备忘、承诺、伏笔、计划、误会、纪念日、调查等。", "选择最贴近的一类；不要新增无意义分类。", true, 20),
			defaultStoryMemoryField("related", "相关对象", "相关角色、地点、势力或物品。", "多项用分号分隔。", false, 30),
			defaultStoryMemoryField("detail", "详细内容", "事项来由、关键细节和当前卡点。", "写清楚为什么需要后续承接，避免一句话空泛概括。", false, 40),
			defaultStoryMemoryField("progress", "当前进度/状态", "已完成事项、当前阻碍或当前状态。", "简要描述已发生变化；没有进展时沿用旧值。", false, 50),
			defaultStoryMemoryField("deadline", "时限", "完成、兑现或爆发的时间限制。", "没有明确时限时填“暂无明确时限”。", false, 60),
			defaultStoryMemoryField("stakes", "风险/收益", "事项成功、失败或拖延的后果。", "没有明确风险收益时填“暂无明确风险收益”。", false, 70),
			defaultStoryMemoryField("result", "后续结果", "事项完结或状态变更后的结果。", "未完结时留空；完结时写具体结果，不写“已解决”等空泛收束。", false, 80),
		}, now),
		defaultStoryMemoryStructure("rule_state_summary", "规则与状态摘要", "旧版派生摘要：记录 TRPG 检定和状态系统的叙事摘要。默认关闭；资源、属性、持续状态和关系数值以状态系统为真源。", "仅在用户显式启用旧版派生摘要时维护；资源、属性、持续状态、关系数值和规则标记必须先写入状态系统，不能通过本表改变真实状态。", "singleton", "", false, 60, []StoryMemoryField{
			defaultStoryMemoryField("resources", "资源数值", "生命、体力、灵力、金钱、声望等资源状态。", "按“资源：当前值/上限｜变化原因”维护；未知上限时写当前状态和来源。", false, 10),
			defaultStoryMemoryField("attributes", "属性/境界", "力量、敏捷、修为、经营等级等稳定属性。", "按“属性：数值或阶段｜来源”维护；只写已确认属性。", false, 20),
			defaultStoryMemoryField("conditions", "持续状态", "伤势、中毒、疲劳、增益、诅咒、通缉等会跨回合影响行动的状态。", "每项写清持续条件、影响和解除方式；过期状态应移除或标记已结束。", false, 30),
			defaultStoryMemoryField("relationship_scores", "关系值", "好感、信任、敌意、债务等可量化关系。", "按“角色/势力：指标=值｜变化原因”维护；没有已确认关系值时可留空。", false, 40),
			defaultStoryMemoryField("flags", "规则标记", "会影响后续检定或分支的布尔/枚举标记。", "按“标记：值｜来源”维护，例如“已暴露身份：否”。", false, 50),
			defaultStoryMemoryField("last_rule_checks", "最近规则检定", "最近关键规则检定及结果。", "记录 3-5 个影响后续叙事的检定，格式建议“回合/检定：成功等级｜代价｜影响”。", false, 60),
		}, now),
		defaultStoryMemoryStructure("relationship_state", "关系状态", "可选叙事关系线索：记录普通互动、恋爱、误会、敌意、债务和同盟等可推进关系。默认关闭；关系数值和当前态度以状态系统为真源。", "用户显式启用后只记录已经被剧情证实的长期关系线索；好感、信任、敌意等可计算或当前态度字段必须写入状态系统。", "keyed", "name", false, 70, []StoryMemoryField{
			defaultStoryMemoryField("name", "姓名/对象", "角色、势力或关系对象名称。", "使用重要角色、势力或稳定称呼。", true, 10),
			defaultStoryMemoryField("relationship_type", "关系类型", "同盟、师徒、竞争、恋爱、误会、债务、敌对等。", "选择最影响后续互动的一类或两类。", true, 20),
			defaultStoryMemoryField("affection", "好感/亲近叙事", "对象对主角亲近、好感或抗拒的长期叙事线索。", "只写自然语言依据；数值、阶段和当前态度必须写入状态系统。", false, 30),
			defaultStoryMemoryField("trust", "信任/戒备叙事", "对象对主角信任、戒备或怀疑的长期叙事线索。", "只写触发原因和长期风险；可计算信任值或当前态度必须写入状态系统。", false, 40),
			defaultStoryMemoryField("tension", "张力/冲突", "暧昧、敌意、竞争、亏欠或压力。", "记录会推动下一次互动的张力，不写泛泛情绪。", false, 50),
			defaultStoryMemoryField("misunderstanding", "误会/秘密", "仍未消解的误会、隐瞒、秘密或错认。", "没有时留空；有时写清谁误会了什么。", false, 60),
			defaultStoryMemoryField("last_interaction", "最近关键互动", "最近一次改变关系状态的互动。", "一句话记录事件和结果。", false, 70),
			defaultStoryMemoryField("next_hook", "后续关系钩子", "下一次关系推进可承接的入口。", "只记录已经被当前剧情合理铺垫的入口。", false, 80),
		}, now),
		defaultStoryMemoryStructure("foreshadowing_resolved", "伏笔与回收", "记录已埋下、推进中、已回收或已失效的伏笔。", "伏笔必须有来源事件、可见线索和回收条件；回收后写具体回收结果，避免只写“已回收”。", "keyed", "title", true, 80, []StoryMemoryField{
			defaultStoryMemoryField("title", "伏笔标题", "伏笔的稳定短标题。", "用可复用短标题，不要每轮改名。", true, 10),
			defaultStoryMemoryField("status", "状态", "seeded、developing、ready、paid_off、void 等状态。", "根据剧情事实选择；不确定时保持上一状态。", true, 20),
			defaultStoryMemoryField("seeded_at", "埋设来源", "伏笔首次出现的事件、回合或场景。", "写清来源，方便后续回查。", false, 30),
			defaultStoryMemoryField("clues", "已露线索", "用户或角色已经看见的线索。", "列出 1-5 条关键线索；不要加入未揭示真相。", false, 40),
			defaultStoryMemoryField("payoff_condition", "回收条件", "触发回收需要满足的条件。", "写成可判断条件，例如“主角交出残卷且长老在场”。", false, 50),
			defaultStoryMemoryField("payoff_result", "回收结果", "伏笔回收、反转或失效后的具体结果。", "未回收时留空；已回收时写对主线、关系或世界状态的影响。", false, 60),
			defaultStoryMemoryField("related_events", "关联事件", "相关事件、角色、物品或长期弧线。", "多项用分号分隔。", false, 70),
		}, now),
		defaultStoryMemoryStructure("long_term_arc_progress", "长期弧线进度", "记录逆袭、复仇、种田、经营、修炼、恋爱等长期情节的目标、阶段、压力和回报。", "只维护仍在推进或需要后续承接的长期弧线；阶段变化必须由剧情或规则结果支撑。", "keyed", "arc_name", true, 90, []StoryMemoryField{
			defaultStoryMemoryField("arc_name", "弧线名称", "长期情节的稳定名称。", "例如“青云宗逆袭线”“北境种田线”。", true, 10),
			defaultStoryMemoryField("arc_type", "弧线类型", "逆袭、复仇、种田、经营、修炼、恋爱、学院比拼等。", "选择主要类型，可用逗号补充副类型。", true, 20),
			defaultStoryMemoryField("goal", "长期目标", "此弧线当前阶段的长期目标。", "写成可推进目标，不写抽象愿望。", false, 30),
			defaultStoryMemoryField("current_phase", "当前阶段", "铺垫、升级、挫败、反转、收获、收束等阶段。", "用一个短语描述当前节奏阶段。", false, 40),
			defaultStoryMemoryField("pressure", "当前压力/危机", "推进此弧线的外部压力、倒计时或危机。", "写清压力来源和失败后果。", false, 50),
			defaultStoryMemoryField("milestones", "已达成节点", "此弧线已完成的重要节点。", "按时间顺序压缩记录，只保留影响后续的节点。", false, 60),
			defaultStoryMemoryField("setbacks", "失败/代价", "此弧线中已经付出的代价、失败或错失机会。", "失败后果要保留，不要下一轮自然消失。", false, 70),
			defaultStoryMemoryField("next_pressure", "下一压力点", "后续可触发的压力、比拼、危机或收益窗口。", "只记录已铺垫且合理的下一压力点。", false, 80),
			defaultStoryMemoryField("terminal_risk", "终局风险", "可能导致主线失败、死亡或弧线中断的风险。", "没有明确风险时留空；有风险时写触发条件。", false, 90),
		}, now),
		defaultStoryMemoryStructure("plot_summary", "剧情纪要", "轮次日志，每轮或每批整理追加一条新记录。", "纪要以第三人称客观记录正文明确发生的事实，不生成下一步行动选项，不加入推测、情绪化语言或主观判断。", "append", "", true, 100, []StoryMemoryField{
			defaultStoryMemoryField("code_index", "编码索引", "本条纪要的唯一顺序索引。", "格式建议 AM0001 起递增；无法确认时根据已有纪要顺序推定。", true, 10),
			defaultStoryMemoryField("time_span", "时间跨度", "本轮事件发生的精确时间范围。", "格式尽量与状态系统 story_context 的当前时间一致。", true, 20),
			defaultStoryMemoryField("place", "地点", "本轮事件发生的地点。", "按从大到小的层级描述地点。", true, 30),
			defaultStoryMemoryField("summary", "概览", "30 字以内的一句话概括。", "不超过 30 字，客观概括本轮事实。", false, 40),
			defaultStoryMemoryField("event", "详细纪要", "以第三人称客观记录本轮事件。", "必须基于正文明确发生的事实；记录关键因果、对话、移动、物品交互和状态变化；不少于 300 字；结尾禁止总结或升华。", true, 50),
			defaultStoryMemoryField("key_dialogue", "重要对话", "造成事实重点或后续影响的重要原文对话。", "摘录 2-4 句并标明说话者；没有关键对话时留空。", false, 60),
			defaultStoryMemoryField("current_day", "当前天数", "本轮结束时对应的剧内天数。", "必须与状态系统 story_context 的当前天数一致。", true, 70),
		}, now),
		defaultStoryMemoryStructure("romance_profile", "恋爱关系档案", "记录恋爱对象或潜在恋爱对象的关系阶段和情感变化。", "默认关闭；用户启用后才参与自动整理。只记录已发生或已表现出的关系变化，不替代重要角色表。", "keyed", "name", false, 110, []StoryMemoryField{
			defaultStoryMemoryField("name", "姓名", "恋爱对象或潜在恋爱对象姓名。", "必须对应重要角色表中的角色。", true, 10),
			defaultStoryMemoryField("relationship_stage", "关系阶段", "该角色与主角的关系阶段。", "用具体短语描述当前阶段，并写出依据。", false, 20),
			defaultStoryMemoryField("affection", "好感/亲近度", "该角色对主角的亲近、好感或抗拒状态。", "用自然语言描述，不强制数值。", false, 30),
			defaultStoryMemoryField("trust", "信任度", "该角色对主角的信任状态。", "用自然语言描述信任依据和风险。", false, 40),
			defaultStoryMemoryField("attitude", "当前态度", "该角色当前面对主角的态度。", "只基于正文表现和已知设定，不主观脑补。", false, 50),
			defaultStoryMemoryField("key_experience", "关键经历", "影响关系发展的关键经历。", "不超过 300 字；只保留影响后续互动的节点。", false, 60),
		}, now),
		defaultStoryMemoryStructure("romance_diary", "恋爱日记", "记录特定角色视角下值得长期保留的情感节点。", "默认关闭；只记录明显改变关系、误会、期待、后悔、动摇或无法说出口想法的节点，不记录普通互动流水账。", "append", "", false, 120, []StoryMemoryField{
			defaultStoryMemoryField("writer", "写作角色", "日记视角角色。", "必须是已建档的重要角色或恋爱档案角色。", true, 10),
			defaultStoryMemoryField("related", "关联角色", "该情感节点关联的角色。", "通常为主角，也可包含关键第三人。", false, 20),
			defaultStoryMemoryField("content", "日记内容", "该角色视角下的情感节点。", "100-200 字；聚焦内心变化、误解、期待、动摇或确认。", true, 30),
			defaultStoryMemoryField("time", "发生时间", "该节点发生的剧内时间。", "尽量与状态系统 story_context 的当前时间一致。", false, 40),
			defaultStoryMemoryField("event_type", "事件类型", "情感节点类型。", "可填初次相遇、日常互动、感情升温、冲突矛盾、和解修复、亲密接触等。", false, 50),
			defaultStoryMemoryField("impact", "影响判断", "该节点对后续关系的影响。", "写具体影响，不写空泛总结。", false, 60),
		}, now),
		defaultStoryMemoryStructure("mature_relationship_profile", "成人向关系档案", "记录用户主动启用后的成人向关系扩展信息。", "默认关闭；作为可配置扩展结构存在，不照搬外部模板的私有字段。启用后只记录用户作品设定中明确允许且后续需要承接的内容。", "keyed", "name", false, 130, []StoryMemoryField{
			defaultStoryMemoryField("name", "姓名", "角色姓名。", "必须对应重要角色表中的角色。", true, 10),
			defaultStoryMemoryField("boundary", "边界与偏好", "角色在成人向互动中的边界、偏好或禁忌。", "只记录已设定或已明确表达的内容。", false, 20),
			defaultStoryMemoryField("relationship_context", "关系语境", "成人向内容与主角关系、权力结构或剧情状态的关联。", "必须服务后续剧情承接，不写一次性场景细节。", false, 30),
			defaultStoryMemoryField("continuity_notes", "连续性备注", "需要长期保持一致的成人向连续性信息。", "压缩记录，避免露骨流水账。", false, 40),
		}, now),
	}
	for i := range structures {
		if isDerivedStoryMemoryStructureID(structures[i].ID) {
			structures[i].ReadOnly = true
			structures[i].Derived = true
		}
	}
	return structures
}

func isDerivedStoryMemoryStructureID(id string) bool {
	switch sanitizeMemoryID(id) {
	case "current_state", "rule_state_summary":
		return true
	default:
		return false
	}
}

func defaultStoryMemoryStructure(id, name, description, generationInstruction, mode, keyFieldID string, enabled bool, order int, fields []StoryMemoryField, now string) StoryMemoryStructure {
	return StoryMemoryStructure{ID: id, Name: name, Description: description, GenerationInstruction: generationInstruction, Mode: mode, KeyFieldID: keyFieldID, Fields: fields, Enabled: boolPtr(enabled), Order: order, BuiltIn: true, CreatedAt: now, UpdatedAt: now}
}

func defaultStoryMemoryField(id, name, description, generationInstruction string, required bool, order int) StoryMemoryField {
	return StoryMemoryField{ID: id, Name: name, Description: description, GenerationInstruction: generationInstruction, Required: required, Order: order}
}

func boolPtr(value bool) *bool {
	return &value
}

func refreshBuiltInStoryMemoryStructures(structures []StoryMemoryStructure) []StoryMemoryStructure {
	defaults := defaultStoryMemoryStructures()
	storedBuiltInByID := make(map[string]StoryMemoryStructure, len(structures))
	custom := make([]StoryMemoryStructure, 0, len(structures))
	for _, structure := range structures {
		if structure.BuiltIn {
			storedBuiltInByID[structure.ID] = structure
			continue
		}
		custom = append(custom, structure)
	}
	out := make([]StoryMemoryStructure, 0, len(defaults)+len(custom))
	for _, preset := range defaults {
		next := preset
		if stored, ok := storedBuiltInByID[preset.ID]; ok {
			next.CreatedAt = firstMemoryText(stored.CreatedAt, preset.CreatedAt)
			next.UpdatedAt = firstMemoryText(stored.UpdatedAt, preset.UpdatedAt)
			if stored.Enabled != nil {
				next.Enabled = stored.Enabled
			}
			next.Fields = mergeBuiltInStoryMemoryFields(preset.Fields, stored.Fields)
		}
		out = append(out, next)
	}
	out = append(out, custom...)
	return out
}

func mergeBuiltInStoryMemoryFields(defaults, stored []StoryMemoryField) []StoryMemoryField {
	storedByID := make(map[string]StoryMemoryField, len(stored))
	for _, field := range stored {
		storedByID[field.ID] = field
	}
	out := make([]StoryMemoryField, 0, len(defaults))
	for _, field := range defaults {
		if storedField, ok := storedByID[field.ID]; ok && storedField.Enabled != nil {
			field.Enabled = storedField.Enabled
		}
		out = append(out, field)
	}
	return out
}

func normalizeStoryMemoryStructure(req StoryMemoryStructureRequest, now string) StoryMemoryStructure {
	structure := StoryMemoryStructure{
		ID:                    sanitizeMemoryID(req.ID),
		Name:                  trimMemoryText(req.Name),
		Description:           trimMemoryText(req.Description),
		GenerationInstruction: trimMemoryText(req.GenerationInstruction),
		Mode:                  strings.TrimSpace(req.Mode),
		KeyFieldID:            sanitizeMemoryID(req.KeyFieldID),
		Enabled:               req.Enabled,
		Order:                 req.Order,
		ReadOnly:              req.ReadOnly,
		Derived:               req.Derived,
		Fields:                normalizeStoryMemoryFields(req.Fields),
		UpdatedAt:             now,
	}
	if structure.Mode == "" {
		structure.Mode = "append"
	}
	if isDerivedStoryMemoryStructureID(structure.ID) {
		structure.ReadOnly = true
		structure.Derived = true
	}
	return structure
}

func normalizeStoryMemoryStructureFromStored(structure StoryMemoryStructure) StoryMemoryStructure {
	structure.ID = sanitizeMemoryID(structure.ID)
	structure.Name = trimMemoryText(structure.Name)
	structure.Description = trimMemoryText(structure.Description)
	structure.GenerationInstruction = trimMemoryText(structure.GenerationInstruction)
	structure.Mode = strings.TrimSpace(structure.Mode)
	if structure.Mode == "" {
		structure.Mode = "append"
	}
	structure.KeyFieldID = sanitizeMemoryID(structure.KeyFieldID)
	if isDerivedStoryMemoryStructureID(structure.ID) {
		structure.ReadOnly = true
		structure.Derived = true
	}
	structure.Fields = normalizeStoryMemoryFields(structure.Fields)
	return structure
}

func normalizeStoryMemoryFields(fields []StoryMemoryField) []StoryMemoryField {
	out := make([]StoryMemoryField, 0, len(fields))
	for i, field := range fields {
		field.ID = sanitizeMemoryID(field.ID)
		if field.ID == "" {
			field.ID = fmt.Sprintf("field_%d", i+1)
		}
		field.Name = trimMemoryText(field.Name)
		if field.Name == "" {
			field.Name = field.ID
		}
		field.Description = trimMemoryText(field.Description)
		field.GenerationInstruction = trimMemoryText(field.GenerationInstruction)
		if field.Order == 0 {
			field.Order = (i + 1) * 10
		}
		out = append(out, field)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Order == out[j].Order {
			return out[i].ID < out[j].ID
		}
		return out[i].Order < out[j].Order
	})
	return out
}

func validateStoryMemoryStructure(structure StoryMemoryStructure) error {
	if strings.TrimSpace(structure.ID) == "" {
		return fmt.Errorf("故事记忆结构 ID 不能为空")
	}
	if strings.TrimSpace(structure.Name) == "" {
		return fmt.Errorf("故事记忆结构名称不能为空")
	}
	switch structure.Mode {
	case "singleton", "keyed", "append":
	default:
		return fmt.Errorf("故事记忆结构模式无效: %s", structure.Mode)
	}
	if len(structure.Fields) == 0 {
		return fmt.Errorf("故事记忆结构至少需要一个字段")
	}
	if structure.Mode == "keyed" && structure.KeyFieldID == "" {
		return fmt.Errorf("keyed 结构必须配置 key_field_id")
	}
	return nil
}

func sortStoryMemoryStructures(structures []StoryMemoryStructure) {
	sort.SliceStable(structures, func(i, j int) bool {
		if structures[i].Order == structures[j].Order {
			return structures[i].ID < structures[j].ID
		}
		return structures[i].Order < structures[j].Order
	})
}

func storyMemoryStructureEnabled(structure StoryMemoryStructure) bool {
	return structure.Enabled == nil || *structure.Enabled
}

func enabledStoryMemoryStructures(structures []StoryMemoryStructure) []StoryMemoryStructure {
	out := make([]StoryMemoryStructure, 0, len(structures))
	for _, structure := range structures {
		if storyMemoryStructureEnabled(structure) {
			out = append(out, structure)
		}
	}
	return out
}

func storyMemoryFieldEnabled(field StoryMemoryField) bool {
	return field.Enabled == nil || *field.Enabled
}

func migrateInteractiveEntriesToStoryMemoryRecords(entries []InteractiveMemoryEntry) []StoryMemoryRecord {
	records := make([]StoryMemoryRecord, 0, len(entries))
	for _, entry := range entries {
		values := map[string]string{
			"event": firstMemoryText(entry.Summary, entry.Content, entry.Title),
		}
		if strings.TrimSpace(entry.Content) != "" {
			values["detail"] = trimMemoryText(entry.Content)
		}
		if len(entry.Places) > 0 {
			values["place"] = strings.Join(entry.Places, "，")
		}
		record := StoryMemoryRecord{
			ID:           firstMemoryText(entry.ID, newID("mem")),
			StructureID:  "plot_summary",
			BranchID:     entry.BranchID,
			TurnID:       entry.TurnID,
			AnchorTurnID: entry.TurnID,
			Key:          entry.Title,
			Values:       values,
			Archived:     entry.Archived,
			Manual:       entry.Manual,
			Source:       "legacy",
			CreatedAt:    entry.CreatedAt,
			UpdatedAt:    entry.UpdatedAt,
		}
		if record.CreatedAt == "" {
			record.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if record.UpdatedAt == "" {
			record.UpdatedAt = record.CreatedAt
		}
		records = append(records, record)
	}
	return records
}

func storyMemoryStructureByID(structures []StoryMemoryStructure, id string) StoryMemoryStructure {
	for _, structure := range structures {
		if structure.ID == id {
			return structure
		}
	}
	return StoryMemoryStructure{ID: id, Name: id, Mode: "append"}
}

func sanitizeStoryMemoryValues(values map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		key = sanitizeMemoryID(key)
		value = trimMemoryText(value)
		if key != "" && value != "" {
			out[key] = value
		}
	}
	return out
}

func sanitizeMemoryID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "-", "_")
	return value
}

func firstMemoryText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func trimMemoryText(value string) string {
	return strings.TrimSpace(value)
}

func sanitizeStringList(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		out = append(out, value)
		seen[value] = true
		if len(out) >= 20 {
			break
		}
	}
	return out
}
