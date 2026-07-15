package interactive

import (
	"fmt"
	"strings"
)

const (
	GenreXuanhuanEventPackageID   = "xuanhuan-core"
	GenreXiuxianEventPackageID    = "xiuxian-core"
	GenreApocalypseEventPackageID = "apocalypse-core"
	GenreWesternEventPackageID    = "western-fantasy-core"
	GenreUrbanEventPackageID      = "urban-core"
	GenreTRPGEventPackageID       = "trpg-core"
)

type genreEventCardPreset struct {
	ID         string
	TypeName   string
	Category   string
	Trigger    string
	Logic      string
	Payoff     string
	RewardCost string
	Guardrail  string
	Intensity  string
	Tags       []string
}

func builtinEventPackageModules() []EventPackageModule {
	return []EventPackageModule{
		DefaultEventPackageModule(),
		builtinGenreEventPackageModule(
			GenreXuanhuanEventPackageID,
			"玄幻核心事件包",
			"面向东方玄幻、热血升级、家族宗门冲突和大世界奇遇的事件卡包。",
			xuanhuanEventCards(),
		),
		builtinGenreEventPackageModule(
			GenreXiuxianEventPackageID,
			"修仙核心事件包",
			"面向修仙、问道、宗门任务、心魔天劫和因果机缘的事件卡包。",
			xiuxianEventCards(),
		),
		builtinGenreEventPackageModule(
			GenreApocalypseEventPackageID,
			"末世核心事件包",
			"面向末世求生、基地建设、感染异变、资源稀缺和幸存者冲突的事件卡包。",
			apocalypseEventCards(),
		),
		builtinGenreEventPackageModule(
			GenreWesternEventPackageID,
			"西幻核心事件包",
			"面向剑与魔法、王国纷争、地下城、神谕教会和异族盟约的事件卡包。",
			westernFantasyEventCards(),
		),
		builtinGenreEventPackageModule(
			GenreUrbanEventPackageID,
			"都市核心事件包",
			"面向都市成长、职场商业、家庭关系、舆论案件和情感拉扯的事件卡包。",
			urbanEventCards(),
		),
		builtinGenreEventPackageModule(
			GenreTRPGEventPackageID,
			"TRPG核心事件包",
			"面向桌面角色扮演式互动叙事，强调任务钩子、线索、检定、遭遇和失败前进。",
			trpgEventCards(),
		),
	}
}

func builtinGenreEventPackageModule(id, name, description string, cards []genreEventCardPreset) EventPackageModule {
	pkg := builtinGenreEventPackage(id, name, cards)
	return normalizeEventPackageModule(EventPackageModule{
		Version:     storyDirectorModuleVersion,
		ID:          id,
		Name:        name,
		Description: description,
		Events:      pkg.Events,
	})
}

func builtinGenreEventPackage(id, name string, cards []genreEventCardPreset) TellerEventPackage {
	events := make([]TellerEventCard, 0, len(cards))
	for _, card := range cards {
		events = append(events, builtinGenreEventCard(card))
	}
	return TellerEventPackage{
		ID:      id,
		Name:    name,
		Enabled: true,
		Events:  events,
	}
}

func builtinGenreEventCard(card genreEventCardPreset) TellerEventCard {
	return TellerEventCard{
		ID:                  card.ID,
		TypeName:            card.TypeName,
		DescriptionMarkdown: builtinGenreEventMarkdown(card),
		Enabled:             true,
		Category:            card.Category,
		Tags:                card.Tags,
		Intensity:           card.Intensity,
	}
}

func builtinGenreEventMarkdown(card genreEventCardPreset) string {
	guardrail := strings.TrimSpace(card.Guardrail)
	if guardrail == "" {
		guardrail = "只在当前设定、角色动机和玩家行动支持时触发；不要替玩家做决定，不要把事件硬插到无关场景。"
	}
	return strings.TrimSpace(fmt.Sprintf(`## 触发场景

%s

## 大致事件逻辑

%s

## 事件回收

%s

## 奖励 / 代价

%s

## 避免生硬的约束

%s`, card.Trigger, card.Logic, card.Payoff, card.RewardCost, guardrail))
}

func xuanhuanEventCards() []genreEventCardPreset {
	return []genreEventCardPreset{
		{
			ID:         "xuanhuan-bloodline-awakening",
			TypeName:   "血脉觉醒",
			Category:   "血脉",
			Trigger:    "主角遭遇压制、濒危、祖器共鸣或亲族线索时，体内血脉出现异常反应。",
			Logic:      "先给出失控征兆和旁人误判，再让血脉力量解决一个局部困境，同时暴露更高层级的传承或敌意。",
			Payoff:     "记录觉醒阶段、血脉副作用、知情者名单和可能被追查的痕迹。",
			RewardCost: "奖励可以是临时战力、传承片段或身份线索；代价是消耗、失控风险、血脉敌人的注意。",
			Intensity:  "high",
			Tags:       []string{"玄幻", "血脉", "成长"},
		},
		{
			ID:         "xuanhuan-clan-pressure",
			TypeName:   "家族压迫",
			Category:   "家族",
			Trigger:    "家族资源分配、婚约、继承、族规处罚或旁系挑衅影响当前目标。",
			Logic:      "让压迫方提出清晰要求或期限，给主角留下谈判、反击、暂避或借势的可行动入口。",
			Payoff:     "沉淀家族派系态度、公开评价、未偿人情和下一次族内节点。",
			RewardCost: "奖励是名望、资源、保护或话语权；代价是敌意升级、亲情裂痕或暴露底牌。",
			Intensity:  "medium",
			Tags:       []string{"玄幻", "家族", "冲突"},
		},
		{
			ID:         "xuanhuan-secret-realm-contest",
			TypeName:   "秘境争夺",
			Category:   "秘境",
			Trigger:    "地图碎片、令牌、天象、宗门名额或敌对队伍把角色推向封闭高风险场景。",
			Logic:      "设置入口规则、竞争者、环境限制和一处可争夺资源，让探索、背叛、合作都能成立。",
			Payoff:     "回收秘境规则、已得资源、未解机关、幸存竞争者和被带出的危险。",
			RewardCost: "奖励是功法、材料、情报或盟友；代价是伤势、消耗、结仇或秘境诅咒。",
			Intensity:  "high",
			Tags:       []string{"玄幻", "秘境", "竞争"},
		},
		{
			ID:         "xuanhuan-genius-ranking",
			TypeName:   "天骄榜变动",
			Category:   "排行",
			Trigger:    "比试、战绩、传闻或榜单组织更新评价，让主角进入公众视野。",
			Logic:      "用排名、评语和挑战者外化成长压力，并让名次变化带来资源与麻烦。",
			Payoff:     "记录当前排名、公开评价、挑战邀约和被榜单牵动的势力。",
			RewardCost: "奖励是声望、邀约和资源优先权；代价是被针对、被试探或行动自由下降。",
			Intensity:  "medium",
			Tags:       []string{"玄幻", "排行", "名望"},
		},
		{
			ID:         "xuanhuan-ancient-inheritance",
			TypeName:   "远古传承",
			Category:   "传承",
			Trigger:    "遗迹、梦境、古物、残魂或禁地让主角接触失落体系。",
			Logic:      "传承不直接白送，先提出试炼、契约、残缺条件或伦理问题，再给可分阶段兑现的线索。",
			Payoff:     "记录传承条件、已解锁部分、传承意志态度和后续试炼。",
			RewardCost: "奖励是功法、知识或身份背书；代价是承诺、因果、敌人或修炼隐患。",
			Intensity:  "high",
			Tags:       []string{"玄幻", "传承", "试炼"},
		},
		{
			ID:         "xuanhuan-auction-gamble",
			TypeName:   "拍卖赌斗",
			Category:   "交易",
			Trigger:    "稀缺材料、残卷、情报或伪装身份把角色带到拍卖、赌石、黑市或公开竞价。",
			Logic:      "让资金、人情、眼力和隐藏身份产生张力，设置可被玩家选择影响的竞价或识宝节点。",
			Payoff:     "记录成交物、欠款、人情债、被盯上的原因和竞争买家。",
			RewardCost: "奖励是稀缺资源或情报；代价是财力消耗、暴露财富、欠债或招惹强者。",
			Intensity:  "medium",
			Tags:       []string{"玄幻", "交易", "资源"},
		},
		{
			ID:         "xuanhuan-beast-tide",
			TypeName:   "兽潮压境",
			Category:   "灾变",
			Trigger:    "边境、山脉、秘境出口或城镇防线出现妖兽异常聚集。",
			Logic:      "把个人目标和公共危机绑在一起，给出救援、撤离、斩首、诱敌或查因的行动路径。",
			Payoff:     "记录伤亡、守城贡献、异常源头、妖兽材料和后续追查方向。",
			RewardCost: "奖励是功勋、材料或民望；代价是伤势、资源消耗、守护对象风险。",
			Intensity:  "high",
			Tags:       []string{"玄幻", "妖兽", "灾变"},
		},
	}
}

func xiuxianEventCards() []genreEventCardPreset {
	return []genreEventCardPreset{
		{
			ID:         "xiuxian-bottleneck-breakthrough",
			TypeName:   "瓶颈突破",
			Category:   "境界",
			Trigger:    "修炼停滞、战后顿悟、灵气环境变化或关键心结被触动。",
			Logic:      "先呈现瓶颈原因，再让突破需要资源、心境、风险或外部护法配合，而不是无条件升级。",
			Payoff:     "记录境界变化、根基隐患、护法人情和突破时暴露的气机。",
			RewardCost: "奖励是境界、术法或感知提升；代价是灵力亏空、雷劫痕迹、心魔苗头或资源消耗。",
			Intensity:  "high",
			Tags:       []string{"修仙", "境界", "突破"},
		},
		{
			ID:         "xiuxian-heart-demon",
			TypeName:   "心魔问道",
			Category:   "道心",
			Trigger:    "杀伐、背叛、执念、突破前夕或幻境触发角色内在矛盾。",
			Logic:      "心魔应围绕已发生事实和角色欲望发问，提供承认、拒绝、交易或自毁的行动空间。",
			Payoff:     "记录道心变化、执念残留、被心魔利用的弱点和后续修行影响。",
			RewardCost: "奖励是心境稳固、术法领悟或自我认识；代价是短期失控、关系伤害或道心裂痕。",
			Intensity:  "high",
			Tags:       []string{"修仙", "心魔", "道心"},
		},
		{
			ID:         "xiuxian-sect-mission",
			TypeName:   "宗门任务",
			Category:   "宗门",
			Trigger:    "宗门贡献、师门命令、外门考核或内务危机需要主角执行任务。",
			Logic:      "任务要有目标、约束、同行者和隐藏变量，让执行方式影响宗门评价和人际关系。",
			Payoff:     "记录贡献、任务评价、同行者态度、未解决异常和宗门后续安排。",
			RewardCost: "奖励是贡献点、师承信任或资源；代价是卷入派系、耽误私事或承担失败责任。",
			Intensity:  "medium",
			Tags:       []string{"修仙", "宗门", "任务"},
		},
		{
			ID:         "xiuxian-alchemy-opportunity",
			TypeName:   "丹药灵草",
			Category:   "资源",
			Trigger:    "伤势、突破需求、委托、秘境采药或丹方线索引出炼丹资源。",
			Logic:      "围绕药材真伪、火候风险、丹毒、竞争采摘或丹师交易制造选择。",
			Payoff:     "记录丹方、药材余量、丹毒状态、交易对象和炼制结果。",
			RewardCost: "奖励是丹药、药材、人脉或修复伤势；代价是失败损耗、丹毒、欠债或引来争抢。",
			Intensity:  "medium",
			Tags:       []string{"修仙", "丹药", "资源"},
		},
		{
			ID:         "xiuxian-dharma-treasure-recognition",
			TypeName:   "法宝认主",
			Category:   "法宝",
			Trigger:    "古宝、残器、拍卖所得或危机中法宝主动回应。",
			Logic:      "让法宝有性格、限制或前任因果，认主过程需要承诺、试炼或代价。",
			Payoff:     "记录法宝能力、限制、器灵态度、前任因果和温养进度。",
			RewardCost: "奖励是新能力或防护；代价是灵力供养、因果牵连、暴露宝物或被器灵试探。",
			Intensity:  "medium",
			Tags:       []string{"修仙", "法宝", "因果"},
		},
		{
			ID:         "xiuxian-oath-karma",
			TypeName:   "誓约因果",
			Category:   "因果",
			Trigger:    "交易、救命、师承、背叛或跨势力合作需要稳定承诺。",
			Logic:      "誓约要有明确条款、见证方式、违背代价和可利用漏洞，推动长期因果线。",
			Payoff:     "记录誓约内容、见证者、约束范围、潜在漏洞和偿还节点。",
			RewardCost: "奖励是信任、资源或临时同盟；代价是行动受限、因果债或违约惩罚。",
			Intensity:  "medium",
			Tags:       []string{"修仙", "因果", "交易"},
		},
		{
			ID:         "xiuxian-tribulation-warning",
			TypeName:   "天劫预兆",
			Category:   "天劫",
			Trigger:    "境界临界、逆天改命、杀孽累积或天地规则被触碰。",
			Logic:      "先以异象、灵压或卦象给出倒计时，让玩家选择准备、压制、借劫或避劫。",
			Payoff:     "记录劫数类型、准备进度、牵连人物和可能借劫解决的外部问题。",
			RewardCost: "奖励是突破、淬体或威慑；代价是重伤、环境破坏、旁人受牵连或根基受损。",
			Intensity:  "high",
			Tags:       []string{"修仙", "天劫", "倒计时"},
		},
		{
			ID:         "xiuxian-immortal-clue",
			TypeName:   "飞升线索",
			Category:   "大境界",
			Trigger:    "古籍、上界遗物、散仙残念或大势变化透露更高世界规则。",
			Logic:      "线索应扩大格局但不立刻兑现，给出可追查地点、条件或敌对封锁。",
			Payoff:     "记录线索来源、可信度、解锁条件、封锁势力和长期主线影响。",
			RewardCost: "奖励是方向、秘闻或高阶坐标；代价是被上层势力注意、认知冲击或路线分歧。",
			Intensity:  "medium",
			Tags:       []string{"修仙", "飞升", "主线"},
		},
	}
}

func apocalypseEventCards() []genreEventCardPreset {
	return []genreEventCardPreset{
		{
			ID:         "apocalypse-resource-shortage",
			TypeName:   "资源短缺",
			Category:   "生存",
			Trigger:    "食物、药品、燃料、弹药或安全住所不足以支撑下一阶段行动。",
			Logic:      "明确缺口和剩余时间，给出搜寻、交易、抢夺、节省或放弃目标的路线。",
			Payoff:     "记录库存变化、消耗速度、欠债、牺牲和暴露的补给点。",
			RewardCost: "奖励是续航和主动权；代价是时间、风险、道德负担或队伍关系恶化。",
			Intensity:  "medium",
			Tags:       []string{"末世", "资源", "生存"},
		},
		{
			ID:         "apocalypse-safe-zone-entry",
			TypeName:   "安全区门槛",
			Category:   "基地",
			Trigger:    "队伍抵达或听闻安全区、避难所、基地、军方据点或私人堡垒。",
			Logic:      "设置入场规则、检查、费用、隔离、派系审查或黑市入口，让安全本身带条件。",
			Payoff:     "记录安全区制度、入场身份、欠下条件、敌友派系和可用设施。",
			RewardCost: "奖励是庇护、医疗、情报或交易；代价是自由受限、资源缴纳、政治站队。",
			Intensity:  "medium",
			Tags:       []string{"末世", "基地", "派系"},
		},
		{
			ID:         "apocalypse-infected-teammate",
			TypeName:   "队友感染",
			Category:   "感染",
			Trigger:    "战斗、搜救、隐瞒伤口或污染区行动后，队友出现感染迹象。",
			Logic:      "把诊断、隔离、隐瞒、治疗、处置和队伍信任放在同一场景内，让玩家选择承担后果。",
			Payoff:     "记录感染阶段、知情者、处理决定、队伍裂痕和治疗线索。",
			RewardCost: "奖励是保住同伴、获得样本或强化信任；代价是感染扩散、资源消耗、道德创伤。",
			Guardrail:  "不要把感染直接写成必死；除非已有规则或玩家选择导致，应保留治疗、拖延或代价交换空间。",
			Intensity:  "high",
			Tags:       []string{"末世", "感染", "队伍"},
		},
		{
			ID:         "apocalypse-mutant-horde",
			TypeName:   "尸潮异变",
			Category:   "威胁",
			Trigger:    "噪音、气味、天气、巢穴迁移或敌方引诱导致大规模感染体逼近。",
			Logic:      "提供防守、转移、诱导、潜行或斩首路线，并让环境和资源决定难度。",
			Payoff:     "记录尸潮规模、变异特征、防线损耗、被毁区域和残留样本。",
			RewardCost: "奖励是样本、地盘、声望或安全窗口；代价是弹药、伤亡、设施损坏。",
			Intensity:  "high",
			Tags:       []string{"末世", "尸潮", "战斗"},
		},
		{
			ID:         "apocalypse-hostile-survivors",
			TypeName:   "敌对幸存者",
			Category:   "人性",
			Trigger:    "路线、补给、情报或避难点与另一支幸存者队伍冲突。",
			Logic:      "敌对方应有可理解目标和底线，允许谈判、威慑、交易、潜入或正面冲突。",
			Payoff:     "记录对方领袖、损失、仇恨或同盟可能、被抢/交换的资源。",
			RewardCost: "奖励是资源、地盘或情报；代价是结仇、伤亡、声誉下降或复仇伏笔。",
			Intensity:  "medium",
			Tags:       []string{"末世", "幸存者", "冲突"},
		},
		{
			ID:         "apocalypse-power-restart",
			TypeName:   "电力重启",
			Category:   "设施",
			Trigger:    "基地建设、医院、通讯站、冷库或防线需要恢复关键设施。",
			Logic:      "把技术步骤、零件缺口、噪音风险和时间窗口结合，形成可分工执行的目标。",
			Payoff:     "记录恢复设施、维护需求、暴露信号、可用设备和下一处技术瓶颈。",
			RewardCost: "奖励是照明、通讯、医疗或生产能力；代价是燃料、零件、引怪或被人定位。",
			Intensity:  "medium",
			Tags:       []string{"末世", "设施", "建设"},
		},
		{
			ID:         "apocalypse-moral-tradeoff",
			TypeName:   "生存道德困境",
			Category:   "抉择",
			Trigger:    "救援、分配、撤离、感染处置或交易要求迫使队伍在价值与生存间取舍。",
			Logic:      "至少给出两条有代价的路，明确每条路会影响谁、损失什么、留下什么后患。",
			Payoff:     "记录选择、受影响人物、队伍评价、心理负担和未来报偿或报复。",
			RewardCost: "奖励是资源、时间或人心；代价是道德债、信任裂缝、创伤或外部敌意。",
			Intensity:  "high",
			Tags:       []string{"末世", "道德", "抉择"},
		},
		{
			ID:         "apocalypse-extreme-weather",
			TypeName:   "极端天气",
			Category:   "环境",
			Trigger:    "暴雨、寒潮、沙尘、酸雨、热浪或辐射云改变路线和资源需求。",
			Logic:      "让天气限制视野、移动、感染体活动或设施稳定性，迫使改变计划。",
			Payoff:     "记录天气持续时间、环境损伤、消耗提升和被天气揭露的新路线。",
			RewardCost: "奖励是掩护、意外通道或敌人削弱；代价是疾病、装备损耗、行程延误。",
			Intensity:  "medium",
			Tags:       []string{"末世", "天气", "环境"},
		},
	}
}

func westernFantasyEventCards() []genreEventCardPreset {
	return []genreEventCardPreset{
		{
			ID:         "western-tavern-quest",
			TypeName:   "酒馆委托",
			Category:   "委托",
			Trigger:    "角色进入城镇、酒馆、行会大厅或旅店时，委托人与传闻汇聚。",
			Logic:      "委托要有雇主、目标、报酬、隐瞒信息和竞争者，适合导入短线冒险。",
			Payoff:     "记录委托条款、真实动机、已收报酬、雇主信誉和后续追索。",
			RewardCost: "奖励是金币、名声、线索或通行权；代价是卷入地方冲突、违约风险。",
			Intensity:  "medium",
			Tags:       []string{"西幻", "委托", "城镇"},
		},
		{
			ID:         "western-dungeon-delving",
			TypeName:   "地下城探索",
			Category:   "地下城",
			Trigger:    "遗迹入口、怪物巢穴、古堡地下层或地图传闻引导队伍进入封闭空间。",
			Logic:      "设置房间目标、陷阱、怪物、谜题和撤退路线，让资源消耗和风险逐步升高。",
			Payoff:     "记录已探索区域、机关状态、战利品、未开门和逃出的怪物。",
			RewardCost: "奖励是宝物、经验、秘密或救援对象；代价是伤势、法术位、补给和诅咒。",
			Intensity:  "high",
			Tags:       []string{"西幻", "地下城", "探索"},
		},
		{
			ID:         "western-oracle-church",
			TypeName:   "神谕与教会",
			Category:   "神权",
			Trigger:    "神殿、圣物、瘟疫、异端审判或祈祷回应把角色卷入宗教权力。",
			Logic:      "神谕应含糊但可行动，教会提供资源也提出义务或审查。",
			Payoff:     "记录神谕措辞、教会派系、圣物状态、信仰声望和异端风险。",
			RewardCost: "奖励是治疗、庇护、神术或合法性；代价是誓言、审查、敌对教派关注。",
			Intensity:  "medium",
			Tags:       []string{"西幻", "教会", "神谕"},
		},
		{
			ID:         "western-royal-intrigue",
			TypeName:   "王国阴谋",
			Category:   "王权",
			Trigger:    "贵族宴会、继承争议、边境军报或密信让角色接触王国权力斗争。",
			Logic:      "把公开礼仪和私下交易并置，给出站队、中立、揭发或利用的路线。",
			Payoff:     "记录贵族立场、把柄、承诺、宫廷传闻和被误会的证据。",
			RewardCost: "奖励是封赏、情报或通行权；代价是政治敌人、名誉风险或被当作棋子。",
			Intensity:  "medium",
			Tags:       []string{"西幻", "王国", "阴谋"},
		},
		{
			ID:         "western-dragon-shadow",
			TypeName:   "龙影临近",
			Category:   "巨兽",
			Trigger:    "天空异象、古老巢穴、龙裔传闻或被劫掠村庄指向巨龙威胁。",
			Logic:      "巨龙应先作为区域压力存在，通过贡品、谈判、寻宝或猎龙准备逐步推进。",
			Payoff:     "记录龙的类型、领地、欲望、可谈条件、弱点线索和被影响地区。",
			RewardCost: "奖励是龙财宝、盟约或威望；代价是毁灭风险、恐慌、巨额准备成本。",
			Intensity:  "high",
			Tags:       []string{"西幻", "龙", "大事件"},
		},
		{
			ID:         "western-magic-school",
			TypeName:   "魔法学院",
			Category:   "学院",
			Trigger:    "入学、考试、导师委托、禁书、法术事故或学生派系冲突。",
			Logic:      "让规则、导师、同辈和实验风险共同施压，形成学习与冒险并行的场景。",
			Payoff:     "记录课程进度、导师态度、禁忌知识、同学关系和校规处罚。",
			RewardCost: "奖励是法术、资源、导师信任；代价是处分、实验后遗症或学院敌意。",
			Intensity:  "medium",
			Tags:       []string{"西幻", "魔法", "学院"},
		},
		{
			ID:         "western-ancestral-curse",
			TypeName:   "古老诅咒",
			Category:   "诅咒",
			Trigger:    "家族旧债、墓穴、魔法物品、被遗忘誓言或怪病显露诅咒迹象。",
			Logic:      "诅咒要有源头、症状、传播或触发规则，解除需要调查和代价。",
			Payoff:     "记录诅咒规则、受害者、缓解方法、解除条件和幕后受益者。",
			RewardCost: "奖励是解除威胁、获得秘密或净化物品；代价是时间压力、牺牲或新债务。",
			Intensity:  "high",
			Tags:       []string{"西幻", "诅咒", "调查"},
		},
		{
			ID:         "western-ancestry-alliance",
			TypeName:   "异族盟约",
			Category:   "异族",
			Trigger:    "精灵、矮人、兽人、半身人或其他族群的利益与当前任务交叉。",
			Logic:      "把文化差异、旧怨、共同敌人和交换条件写清楚，让协商比单纯战斗更有价值。",
			Payoff:     "记录盟约条款、族群态度、禁忌、共享资源和可能破裂的条件。",
			RewardCost: "奖励是盟友、工艺、路径或情报；代价是遵守习俗、卷入旧战或牺牲利益。",
			Intensity:  "medium",
			Tags:       []string{"西幻", "异族", "盟约"},
		},
	}
}

func urbanEventCards() []genreEventCardPreset {
	return []genreEventCardPreset{
		{
			ID:         "urban-career-trap",
			TypeName:   "职场机会与陷阱",
			Category:   "职场",
			Trigger:    "升职、项目、竞标、背锅、空降领导或关键客户改变主角工作局面。",
			Logic:      "机会背后要有利益相关人和隐含成本，让主角能选择承担、反击、谈判或另辟路线。",
			Payoff:     "记录职位变化、项目风险、同事态度、证据和后续绩效节点。",
			RewardCost: "奖励是晋升、收入、人脉或作品成果；代价是时间、名誉风险、职场敌人。",
			Intensity:  "medium",
			Tags:       []string{"都市", "职场", "成长"},
		},
		{
			ID:         "urban-business-rivalry",
			TypeName:   "商业竞争",
			Category:   "商业",
			Trigger:    "创业、投资、供应链、合同、竞品或资本方要求推动商业冲突。",
			Logic:      "把现金流、信息差、法律边界和竞争对手策略写清楚，避免只靠口号逆袭。",
			Payoff:     "记录合同、资金、关键客户、竞对动作和未解决法律/舆论风险。",
			RewardCost: "奖励是利润、市场、资源或股权；代价是债务、被收购压力、合规风险。",
			Intensity:  "medium",
			Tags:       []string{"都市", "商业", "竞争"},
		},
		{
			ID:         "urban-family-pressure",
			TypeName:   "家庭压力",
			Category:   "家庭",
			Trigger:    "婚恋、赡养、债务、亲戚攀比、旧事翻出或家庭成员求助影响主线。",
			Logic:      "家庭压力要兼具情感和现实约束，给主角沟通、划界、帮助或拒绝的空间。",
			Payoff:     "记录家人立场、承诺、旧矛盾、经济往来和情感温度。",
			RewardCost: "奖励是支持、和解或家庭资源；代价是负担、误解、牺牲个人机会。",
			Intensity:  "medium",
			Tags:       []string{"都市", "家庭", "关系"},
		},
		{
			ID:         "urban-public-opinion",
			TypeName:   "舆论反转",
			Category:   "舆论",
			Trigger:    "偷拍视频、热搜、爆料、误会、粉丝争执或媒体报道扭曲事实。",
			Logic:      "先明确公众看到的版本，再给证据收集、回应时机、沉默成本和反转风险。",
			Payoff:     "记录公开叙事、关键证据、支持者/攻击者、平台影响和后续声誉。",
			RewardCost: "奖励是名声、流量或清白；代价是隐私暴露、二次伤害、关系破裂。",
			Intensity:  "high",
			Tags:       []string{"都市", "舆论", "反转"},
		},
		{
			ID:         "urban-old-acquaintance",
			TypeName:   "旧识重逢",
			Category:   "关系",
			Trigger:    "同学会、商务场合、医院、街头偶遇或线上联系让旧人回到当前生活。",
			Logic:      "旧识应带来未结清的情感、利益或秘密，而不是只负责寒暄。",
			Payoff:     "记录旧关系历史、当前态度、未说出口的信息和下次见面理由。",
			RewardCost: "奖励是线索、人脉、情感推进；代价是旧伤复发、误会、现实冲突。",
			Intensity:  "medium",
			Tags:       []string{"都市", "旧识", "情感"},
		},
		{
			ID:         "urban-case-commission",
			TypeName:   "案件委托",
			Category:   "案件",
			Trigger:    "失踪、诈骗、纠纷、事故、法律委托或私人调查需要主角介入。",
			Logic:      "案件要有委托人、表面事实、矛盾证词和一条可追的证据链。",
			Payoff:     "记录证据、嫌疑人、动机、委托人可信度和未解问题。",
			RewardCost: "奖励是报酬、真相、人脉或正义感；代价是危险、得罪人、法律风险。",
			Intensity:  "medium",
			Tags:       []string{"都市", "案件", "调查"},
		},
		{
			ID:         "urban-skill-breakthrough",
			TypeName:   "技能突破",
			Category:   "成长",
			Trigger:    "训练、比赛、项目压线、导师点拨或失败复盘让主角掌握新能力。",
			Logic:      "突破来自具体练习、反馈和选择，需写清新能力的边界和下一步验证场景。",
			Payoff:     "记录技能等级、适用条件、导师评价、短板和可展示机会。",
			RewardCost: "奖励是能力、作品、认可或收入；代价是疲劳、机会成本、被模仿或嫉妒。",
			Intensity:  "medium",
			Tags:       []string{"都市", "技能", "成长"},
		},
	}
}

func trpgEventCards() []genreEventCardPreset {
	return []genreEventCardPreset{
		{
			ID:         "trpg-quest-hook",
			TypeName:   "任务钩子",
			Category:   "任务",
			Trigger:    "玩家抵达新地点、完成旧目标、休整或询问传闻时，出现可接取任务。",
			Logic:      "任务要明确目标、雇主、报酬、时限、未知风险和拒绝后的世界变化。",
			Payoff:     "记录任务状态、雇主、目标地点、奖励承诺和倒计时。",
			RewardCost: "奖励是经验、金币、线索或盟友；代价是时间、危险、阵营牵连。",
			Intensity:  "medium",
			Tags:       []string{"TRPG", "任务", "钩子"},
		},
		{
			ID:         "trpg-investigation-clue",
			TypeName:   "调查线索",
			Category:   "调查",
			Trigger:    "玩家搜索、询问、观察异常或复盘信息时，场景中出现可验证线索。",
			Logic:      "线索分为表层信息、深挖信息和误导信息；检定失败也应给出方向但附带代价。",
			Payoff:     "记录线索来源、可信度、已排除假设和下一处调查入口。",
			RewardCost: "奖励是真相进展、优势或避免危险；代价是时间流逝、暴露、误判。",
			Intensity:  "medium",
			Tags:       []string{"TRPG", "调查", "线索"},
		},
		{
			ID:         "trpg-social-check",
			TypeName:   "社交检定",
			Category:   "社交",
			Trigger:    "说服、威吓、欺瞒、洞察、谈判或安抚 NPC 会改变局面时。",
			Logic:      "先确定 NPC 目标、底线和可交换筹码，再让检定结果影响态度和条件。",
			Payoff:     "记录 NPC 态度、承诺、怀疑点、欠下的人情和未来反应。",
			RewardCost: "奖励是信息、通行、折扣或盟友；代价是敌意、误解、额外条件。",
			Guardrail:  "不要把高检定写成精神控制；成功也应尊重 NPC 的利益和底线。",
			Intensity:  "medium",
			Tags:       []string{"TRPG", "社交", "检定"},
		},
		{
			ID:         "trpg-combat-encounter",
			TypeName:   "战斗遭遇",
			Category:   "战斗",
			Trigger:    "敌人伏击、守卫阻拦、怪物巡逻、谈判破裂或玩家主动开战。",
			Logic:      "明确敌人目标、战场要素、可利用环境、撤退路线和非战斗解决可能。",
			Payoff:     "记录敌我损耗、战利品、逃脱敌人、噪音后果和战场变化。",
			RewardCost: "奖励是战利品、通路、威慑或保护对象；代价是伤害、资源消耗、增援。",
			Intensity:  "high",
			Tags:       []string{"TRPG", "战斗", "遭遇"},
		},
		{
			ID:         "trpg-random-encounter",
			TypeName:   "随机遭遇",
			Category:   "遭遇",
			Trigger:    "旅行、扎营、长时间探索、等待或噪音吸引外界反应。",
			Logic:      "随机遭遇应服务当前区域生态或主线压力，可以是危险、机会、路人或环境变化。",
			Payoff:     "记录遭遇对象、区域生态信息、消耗、获得线索和路线变化。",
			RewardCost: "奖励是发现、补给、传闻或捷径；代价是消耗、延误、伤害或暴露。",
			Intensity:  "medium",
			Tags:       []string{"TRPG", "随机", "旅行"},
		},
		{
			ID:         "trpg-faction-reputation",
			TypeName:   "派系声望",
			Category:   "派系",
			Trigger:    "玩家行动被组织、城镇、教派、帮会或敌对阵营得知。",
			Logic:      "用声望变化、通缉、折扣、委托或封锁体现阵营反馈。",
			Payoff:     "记录各派系态度、声望分、已知行为、赏金或保护关系。",
			RewardCost: "奖励是资源、庇护、情报或身份；代价是敌对阵营追杀、限制或政治负担。",
			Intensity:  "medium",
			Tags:       []string{"TRPG", "派系", "声望"},
		},
		{
			ID:         "trpg-rest-and-resource",
			TypeName:   "休整与资源",
			Category:   "资源",
			Trigger:    "队伍要求休息、治疗、补给、制作、升级或等待时。",
			Logic:      "休整不是无事发生，应结算恢复、消耗、守夜风险、营地互动和外界倒计时。",
			Payoff:     "记录恢复量、消耗物、营地安全、角色互动和推进的倒计时。",
			RewardCost: "奖励是恢复、制作、计划和关系推进；代价是时间流逝、随机遭遇、目标变化。",
			Intensity:  "low",
			Tags:       []string{"TRPG", "休整", "资源"},
		},
		{
			ID:         "trpg-fail-forward",
			TypeName:   "失败前进",
			Category:   "裁定",
			Trigger:    "关键检定失败、计划失误或玩家选择高风险行动后，故事需要继续推进。",
			Logic:      "失败应改变条件而不是堵死剧情：给出代价、暴露、损耗、误会或更危险的新入口。",
			Payoff:     "记录失败后果、增加的压力、新路线、失去的机会和可弥补条件。",
			RewardCost: "奖励是获得有限信息或新入口；代价是资源、时间、伤害、敌意或复杂化局势。",
			Guardrail:  "不要用失败前进抹消失败；必须让代价在后续回合继续可见。",
			Intensity:  "medium",
			Tags:       []string{"TRPG", "失败", "裁定"},
		},
	}
}
