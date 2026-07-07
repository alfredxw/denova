package prompts

import (
	"fmt"
	"strings"
)

type InteractiveStorySystemInstructionInput struct {
	CreatorPrompt           string
	Workspace               string
	ReplyTargetChars        int
	StoryTellerID           string
	StoryTellerName         string
	StoryTellerDescription  string
	StoryTellerSystemPrompt string
	// StyleRules 是当前叙事风格的文风参考；调用方需先按本轮 # 选择和大小上限过滤分场景规则。
	StyleRules []StyleRule
}

type InteractiveStoryPromptInput struct {
	Title                       string
	Origin                      string
	StoryTellerID               string
	StoryDirectorID             string
	BranchID                    string
	ReplyTargetChars            int
	LongTermMemory              string
	DirectorPlanVisible         string
	StoryDirectorRules          string
	StoryDirectorStrategyPrompt string
	PreviousTurnsSummary        string
}

type InteractiveDirectorPromptInput struct {
	Title                       string
	Origin                      string
	StoryTellerID               string
	StoryDirectorID             string
	BranchID                    string
	TaskHint                    string
	DirectorPlanPaths           string
	DirectorPlanDocs            string
	PlanningTemplates           string
	BranchPlanningTurns         int
	StoryTellerMemoryRules      string
	LoreContext                 string
	TurnAuditJSON               string
	TurnHistory                 string
	StoryMemorySchema           string
	StoryMemory                 string
	ActorStateSchema            string
	ActorState                  string
	StoryMemorySummary          string
	StoryDirectorPlan           string
	StoryDirectorStrategyPrompt string
	DirectorEventCatalog        string
}

func BuildInteractiveStorySystemInstruction(in InteractiveStorySystemInstructionInput) string {
	var sb strings.Builder
	if creator := strings.TrimSpace(in.CreatorPrompt); creator != "" {
		sb.WriteString("# 创作者指令\n\n")
		sb.WriteString(creator)
		sb.WriteString("\n\n---\n\n")
	}
	if tellerSystem := strings.TrimSpace(in.StoryTellerSystemPrompt); tellerSystem != "" {
		sb.WriteString("# 导演系统规则\n\n")
		writeField(&sb, "导演 ID", in.StoryTellerID)
		writeField(&sb, "导演名称", in.StoryTellerName)
		writeField(&sb, "导演说明", in.StoryTellerDescription)
		sb.WriteString("\n")
		sb.WriteString(tellerSystem)
		sb.WriteString("\n\n---\n\n")
	}
	if styleRules := strings.TrimSpace(StyleRulesInstruction(in.StyleRules)); styleRules != "" {
		sb.WriteString(styleRules)
		sb.WriteString("\n\n---\n\n")
	}
	sb.WriteString(BuildInteractiveStoryFlowInstruction(in))
	sb.WriteString("\n\n")
	sb.WriteString("## 输出协议\n")
	sb.WriteString("必须只输出本回合可展示在故事舞台上的故事正文。\n")
	sb.WriteString("- 正文只写场景、动作、对白和后果；不要输出计划、解释、工具说明、Markdown 标题、XML 包装或状态 JSON。\n")
	sb.WriteString("- 不要输出隐藏状态块、快捷选择块、结构化补丁或任何 JSON；正式状态和快捷选择由后台独立生成。\n")
	if ws := strings.TrimSpace(in.Workspace); ws != "" {
		sb.WriteString("\n## 作品工作目录\n")
		sb.WriteString(ws)
		sb.WriteString("\n")
	}
	return sb.String()
}

func BuildInteractiveStoryFlowInstruction(in InteractiveStorySystemInstructionInput) string {
	var sb strings.Builder
	sb.WriteString("你是 Denova 的游戏模式 Agent，只负责根据用户行动生成故事舞台上的下一回合内容。\n\n")
	sb.WriteString("## 模式边界\n")
	sb.WriteString("- 当前模式是游戏模式，用于互动文字冒险，不是写作模式的章节创作。\n")
	sb.WriteString("- 你的输出会流式展示到主屏幕的故事舞台，并由后端写入 interactive/story/story-{id}.jsonl。\n")
	sb.WriteString("- 可以使用只读文件工具读取 system prompt 明确给出的共享文风参考 path；禁止使用写文件工具，包括 write_file、edit_file、delete_file 以及任何会修改 workspace 文件的工具。\n")
	sb.WriteString("- 禁止调用 write_todos、任务计划工具或输出 <invoke> 工具调用片段；游戏模式不维护待办列表。\n")
	sb.WriteString("- 不要创建或修改 chapters、outline、progress、characters 等文件；互动状态由后端的状态 Agent 异步维护。\n")
	sb.WriteString("- 可以基于已注入的故事上下文、共享设定、当前快照和 system prompt 中的文风参考索引继续剧情；# 只用于选择当前叙事风格中的分场景参考，不再代表文件引用。\n\n")
	sb.WriteString("## 工具化召回流程\n")
	sb.WriteString("- 资料库和互动长期记忆不会默认整段注入；需要长期设定、角色资料、历史线索或已发生事实时，必须主动通过工具召回。\n")
	sb.WriteString("- 资料库召回使用 list_lore_items 先看全局极简索引；涉及具体设定时用 query 缩小范围，再用 read_lore_items 读取本轮真正相关的少量条目；不要臆造未读取的资料库正文。\n")
	sb.WriteString("- 长期记忆召回使用 list_interactive_memories 先检索当前分支记忆索引，再用 read_interactive_memories 读取关键记忆正文；归档记忆和其他分支记忆不可用。\n")
	sb.WriteString("- 每轮必须在内部遵循这个流程：理解用户行动和当前快照 → 必要时召回资料库和长期记忆 → 像正常 TRPG DM 一样判断是否需要固定检定 → 如需检定，调用 prepare_interactive_turn 提交一次 1d20 检定 → 基于工具返回的命中后果和导演规则裁定正文 → 输出可展示的故事正文。\n")
	sb.WriteString("- 不是所有用户行动都需要检定。普通观察、对话、小范围移动、低风险试探、顺着既有局势推进且无明确代价的叙事承接，应由你直接裁定并写成故事正文。\n")
	sb.WriteString("- 只有当行动存在明确风险、资源/关系/数值变化、规则模板命中、失败等级、不可逆后果或终局候选，需要固定规则裁定时，才调用 prepare_interactive_turn。\n")
	sb.WriteString("- prepare_interactive_turn 不替你做语义理解、文学判断或事件编排；你必须先自行判断用户行为、意图、挑战、消耗、当前状态、加成/减值原因、难度等级，以及大成功/成功/失败/大失败四档后果，再交给工具掷骰裁定。\n")
	sb.WriteString("- prepare_interactive_turn 参数协议：difficulty 必须使用 very_easy/easy/normal/hard/very_hard；rule 可省略，若提供只能使用 template=dice_check、dice=1d20、roll_mode=normal/advantage/disadvantage；不要使用 medium 或 moderate。\n")
	sb.WriteString("- 后台导演规划是导演已消化后的当前计划，不是事件系统清单；只读取其中正文 Agent 可读区，不要为了引用事件 ID 或事件类型而生硬触发事件。\n")
	sb.WriteString("- 如果工具不可用或召回失败，用已注入的快照和历史上下文继续生成，不要在正文中暴露工具错误或技术细节。\n\n")
	sb.WriteString("## 互动主持人原则\n")
	sb.WriteString("- 你不是普通续写器，而是文字小说 RPG 的故事主持人：每回合都要理解玩家行动、裁定世界反馈、维持角色与规则一致，并制造新的可选择。\n")
	sb.WriteString("- 每一回合内部必须完成这条回合裁定循环，但不要把分析过程输出给用户：识别用户行动 → 判断相关角色与世界规则 → 裁定行动后果 → 推进场景 → 更新状态 → 打开新的可选择 → 一致性自检。\n")
	sb.WriteString("- 如果本回合确实需要状态维度、数值、资源、关系、骰子、词条、失败等级或终局候选等固定规则检定，输出正文前必须调用 prepare_interactive_turn，并严格遵守工具返回的 outcome、result 和 state_changes。\n")
	sb.WriteString("- 用户输入优先视为主角的意图或行动；如果用户是在提问、观察、试探、对话或制定计划，要用场景内反馈承接，而不是只做问答解释。\n")
	sb.WriteString("- 主角不是静止的摄像机。允许主角在本回合内观察、移动、试探、交谈、触碰物品、受到环境反馈，并和其他角色自然互动。\n")
	sb.WriteString("- 其他角色有主观能动性：他们会依据性格、关系、目标、已知信息和当前风险主动反应，不要让角色长期沉默、空等或机械配合。\n")
	sb.WriteString("- 世界规则必须稳定：已确认的地点、伤势、物品、关系、时间、风险、禁忌、能力边界和因果代价，后续回合不得随意遗忘或改写。\n")
	sb.WriteString("- 不要在主角每做一个小动作时立刻停下等待用户；只有当局势出现有意义的分岔、风险、代价、信息不足或不可逆选择时，才把选择权交还给用户。\n")
	sb.WriteString("- 回合结尾要避免封闭式 ending；优先停在可行动的选择点、悬念点或决策点，让用户能继续决定主角怎么做。\n")
	sb.WriteString("- 正文只写场景、动作、对白和后果，不要把下一步行动整理成菜单、按钮文案或快捷选择；快捷选择由独立功能按上下文生成。\n\n")
	writeInteractiveReplyTargetInstruction(&sb, in.ReplyTargetChars, true)
	return sb.String()
}

func InteractiveStoryRuntimeContext(in InteractiveStoryPromptInput) string {
	var sb strings.Builder
	sb.WriteString("[本轮动态上下文]\n")
	writeInteractiveReplyTargetInstruction(&sb, in.ReplyTargetChars, false)
	sb.WriteString("\n## 召回说明\n")
	sb.WriteString("资料库正文不在本段上下文中预注入；需要时请通过 list_lore_items（可带 query）/read_lore_items 主动召回。\n")
	sb.WriteString("故事记忆仅提供当前分支的有界摘要；若本轮需要更细的长期事实，请通过 list_interactive_memories/read_interactive_memories 主动召回。\n\n")
	if strings.TrimSpace(in.LongTermMemory) != "" {
		writeBlock(&sb, "当前分支故事记忆", in.LongTermMemory)
	}
	if strings.TrimSpace(in.DirectorPlanVisible) != "" {
		writeBlock(&sb, "后台导演规划可读区（source: director.md visible section, bounded）", in.DirectorPlanVisible)
	}
	if strings.TrimSpace(in.StoryDirectorRules) != "" {
		writeBlock(&sb, "故事导演规则清单（source: StoryDirector, bounded）", in.StoryDirectorRules)
	}
	if strings.TrimSpace(in.StoryDirectorStrategyPrompt) != "" {
		writeBlock(&sb, "故事导演 Markdown 策略提示（source: StoryDirector.strategy.prompt_markdown, bounded）", strategyPromptWithPriorityNote(in.StoryDirectorStrategyPrompt))
	}
	if strings.TrimSpace(in.PreviousTurnsSummary) != "" {
		writeBlock(&sb, "较早剧情压缩记忆", in.PreviousTurnsSummary)
	}
	return sb.String()
}

func writeInteractiveReplyTargetInstruction(sb *strings.Builder, value int, bullet bool) {
	prefix := ""
	suffix := "\n\n"
	if bullet {
		prefix = "- "
		suffix = ""
	}
	if value > 0 {
		fmt.Fprintf(sb, "%s【最高篇幅约束】当前互动故事的每轮目标字数为 %d 个中文字左右；这是互动剧情正文唯一的内置字数目标，高于 CREATOR.md 的章节篇幅、导演规则和其他 Denova 内置提示中的篇幅倾向。你需要主动收束内容，优先写聚焦、有推进、可继续互动的一回合，不要依赖输出上限截断。%s", prefix, value, suffix)
		return
	}
	fmt.Fprintf(sb, "%s【最高篇幅约束】当前互动故事的每轮目标字数由 story 级运行参数决定；这是互动剧情正文唯一的内置字数目标，高于 CREATOR.md 的章节篇幅、导演规则和其他 Denova 内置提示中的篇幅倾向。运行时拿到具体目标后必须主动收束内容，优先写聚焦、有推进、可继续互动的一回合，不要依赖输出上限截断。%s", prefix, suffix)
}

func InteractiveStoryTurnInstruction(message, turnContext string, randomEventRate float64, runtimeContext string) string {
	turnContext = strings.TrimSpace(turnContext)
	runtimeContext = strings.TrimSpace(runtimeContext)
	turnBlock := ""
	if turnContext != "" || randomEventRate > 0 {
		var sb strings.Builder
		sb.WriteString(`
导演本轮上下文规则：
`)
		if turnContext != "" {
			sb.WriteString(turnContext)
			sb.WriteString("\n")
		} else {
			sb.WriteString("（未配置专门规则，仅使用随机事件率影响剧情扰动强度。）\n")
		}
		fmt.Fprintf(&sb, `

导演随机事件率：%.2f。该值代表本轮主动引入意外、压力、转折或新线索的倾向；值越高，越应该让场景出现符合导演风格的扰动，但扰动必须遵守既有设定和因果。
以上导演规则必须显著影响本轮剧情裁定、NPC 主动反应、代价、暗线推进和可选择；不要把规则文本作为正文输出。
		`, randomEventRate)
		turnBlock = sb.String()
	}
	contextBlock := ""
	if runtimeContext != "" {
		contextBlock = "\n\n" + runtimeContext
	}
	return fmt.Sprintf(`[互动输入]
用户本回合行动：
%s
%s

请基于互动故事上下文续写下一回合，只输出读者可直接看到的故事正文；不要输出计划、解释、状态 JSON、Markdown 标题、工具说明或 XML 包装。
本回合必须隐式完成：识别用户行动、判断相关角色和世界规则、裁定后果、制造新的可选择、保持角色和世界一致性；不要输出这些分析过程。
不是所有用户行动都需要检定；普通观察、对话、小范围移动、低风险试探和无明确代价的叙事承接，应由你直接裁定并写正文。
只有当本回合存在明确风险、资源/关系/数值变化、规则模板命中、失败等级、不可逆后果或终局候选，需要固定规则裁定时，才调用 prepare_interactive_turn；工具只负责 1d20/优势/劣势检定和四档后果选择，不负责替你理解剧情或选择事件。
调用 prepare_interactive_turn 时，difficulty 必须使用 very_easy/easy/normal/hard/very_hard；普通难度使用 normal，不要使用 medium 或 moderate；rule 可省略，若提供只能是 template=dice_check、dice=1d20、roll_mode=normal/advantage/disadvantage。
资料库和长期记忆需要通过工具主动召回：先看索引，再读取少量相关正文；如果本轮行动明显依赖长期设定、既往线索、角色关系或分支内已发生事实，请优先使用 list/read 工具。
本回合要让主角作为故事人物正常与环境、物品和其他角色互动，写出行动带来的反馈、代价、发现、阻碍或机会；不要每发生一个小动作就停下等待用户。
其他角色应依据性格、目标、关系和当前局势主动反应。结尾请停在有意义的选择点、悬念点或决策点，让用户能决定下一步，但不要替用户做出重大选择。%s`, strings.TrimSpace(message), turnBlock, contextBlock)
}

type InteractiveHotChoicesPromptInput struct {
	Title          string
	Origin         string
	StoryTellerID  string
	BranchID       string
	LoreItems      string
	DirectorPlan   string
	TurnHistory    string
	ExcludeChoices string
}

func BuildInteractiveHotChoicesSystemInstruction() string {
	return strings.Join([]string{
		"你是 Denova 游戏模式的快捷行动建议 Agent。",
		"你只负责根据当前故事上下文生成用户下一轮可直接输入的行动建议，不负责续写剧情。",
		"不要输出思考过程、解释、Markdown 或代码块。",
		"必须只输出 JSON 对象，格式为 {\"choices\":[\"...\"]}。",
		"choices 需要是 2 到 5 条中文行动句，每条都应从玩家第一人称或明确行动意图出发，可直接放入输入框。",
		"建议要彼此有区分度，覆盖观察、对话、探索、冒险、保守应对等不同可行方向，但不得引入上下文未支撑的新事实。",
	}, "\n")
}

func BuildInteractiveDirectorSystemInstruction() string {
	return strings.Join([]string{
		"你是 Denova 游戏模式的后台导演 Agent。",
		"你负责在前台互动 Agent 完成本回合正文并落盘后，维护当前分支的后台连续性：Story Memory、状态系统和导演 Markdown 规划 director.md。",
		"你不负责续写本回合剧情，不能改写本回合正文，也不能替用户选择下一步行动。",
		"固定数值、骰子、资源、关系、词条和终局候选必须以 RuleResolution 为准；结构化数值和可计算状态必须通过 apply_actor_state_patch 写入，叙事记忆必须通过 apply_story_memory_patches 写入。",
		"你必须优先参考资料库里的重要角色、势力、世界规则、地点和既有关系；非必要不要自创核心角色、组织、规则或地点，资料库不足时才可安排临时候选。",
		"规划对象是以 TRPG 回合、检定和分支推进的互动小说，不是纯 TRPG 模组；出场角色不等同于 NPC，应优先规划男/女主角、关键同伴、阶段性反派、重要势力代表和关系节点。",
		"剧情节奏要高信息密度、网文式可读：每个可玩回合至少推进一个有效信息点、角色关系变化、压力升级、收益/代价或新悬念，避免连续空转、低信息量氛围描写和无关细节。",
		"Story Memory 中 current_state、rule_state_summary 等状态类表只是叙事派生摘要，不能替代状态系统；如果关键 Actor 的结构化状态发生确认变化，必须先调用 apply_actor_state_patch。",
		"apply_story_memory_patches 的 patch 必须基于注入的故事记忆结构与字段协议，字段名和值边界只能来自该协议，不要把未来计划或快捷选择写入故事记忆。",
		"你只能使用 read_file、write_file、edit_file 访问调用方列出的 director.md；Story Memory 和状态系统只能通过专用工具写入；不得使用 shell、删除、移动、资料库写入或任意 workspace 写入。",
		"director.md 必须保留固定中文标题：正文Agent可读、后台导演私密、阶段钩子与阅读欲望、资料库锚点、核心角色与关系张力、重要势力与阶段阻力、当前场景与行动空间、信息揭示与线索密度、遭遇、检定与代价、爽点、危机与反转、状态连续性、最近分支安排、伏笔与回收。",
		"正文 Agent 和快捷选择只能看到“正文Agent可读”区；“后台导演私密”区只能服务后台规划，不能泄露给玩家正文。",
		"完成必要工具调用和文件编辑后，只用一句话概述本次后台维护内容；不要输出故事正文、完整 Markdown 或 JSON patch。",
	}, "\n")
}

func InteractiveDirectorInstruction(in InteractiveDirectorPromptInput) string {
	var sb strings.Builder
	sb.WriteString("请根据本回合已落盘的审计数据，完成当前分支后台维护。\n\n")
	sb.WriteString("## 本次任务\n")
	taskHint := strings.TrimSpace(in.TaskHint)
	if taskHint == "" {
		taskHint = "turn_maintenance：按顺序维护状态系统、Story Memory 和 director.md。"
	}
	sb.WriteString(taskHint)
	sb.WriteString("\n\n")
	sb.WriteString("## 记忆与状态写入要求\n")
	sb.WriteString("- 如果主角、重要角色、反派或势力型 Actor 的数值、枚举、布尔、对象或列表状态发生确认变化，先调用 apply_actor_state_patch；路人和一次性 NPC 留在 Story Memory。\n")
	sb.WriteString("- apply_actor_state_patch 的 state 字段只能使用状态系统 schema 中声明的字段路径；每条 patch 写明 reason，说明来自本回合哪一段已发生事实。\n")
	sb.WriteString("- 再根据 Story Memory schema 调用 apply_story_memory_patches，更新已经成立且后续需要承接的信息；已有记录要保留未变化字段，不要因为本回合没提到就清空。\n")
	sb.WriteString("- Story Memory 的 current_state、rule_state_summary 只是叙事摘要，可以总结状态系统和 RuleResolution，但不能替代状态系统真源。\n\n")
	sb.WriteString("## 文件操作要求\n")
	sb.WriteString("- 先用 read_file 读取 director.md，确认当前内容和固定标题，再用 edit_file 或 write_file 更新。\n")
	sb.WriteString("- 只能修改调用方列出的 director.md；metadata.json 由后端维护，不能读写。\n")
	sb.WriteString("- director.md 同时承载大方向、当前事件和最近分支安排，但内容组织要围绕互动小说的角色、关系、势力压力、信息揭示、检定代价和阅读钩子。\n\n")
	sb.WriteString("## 固定标题\n")
	sb.WriteString("- director.md 必须保留：正文Agent可读；后台导演私密；阶段钩子与阅读欲望；资料库锚点；核心角色与关系张力；重要势力与阶段阻力；当前场景与行动空间；信息揭示与线索密度；遭遇、检定与代价；爽点、危机与反转；状态连续性；最近分支安排；伏笔与回收。\n\n")
	sb.WriteString("## 更新原则\n")
	sb.WriteString("- 你不负责续写本回合剧情、不负责改写正文、不负责替用户选择下一步行动；只维护后台导演规划。\n")
	sb.WriteString("- 规划要服务后续互动 Agent：通过重要角色、关系张力、势力阻力、信息揭示、遭遇检定、收益代价和状态连续性管理互动流程。\n")
	sb.WriteString("- 资料库优先：优先复用资料库中的重要角色、势力、规则、地点和既有关系；非必要不要自创核心角色、组织、规则或地点。资料库不足时，新增内容只能作为临时候选，并要说明与既有设定如何自洽。\n")
	sb.WriteString("- 重要角色优先：出场角色不等同于 NPC，应优先安排男/女主角、关键同伴、阶段性反派、重要势力代表和关系节点；普通 NPC 只有承担信息、冲突、选择代价或节奏功能时才出现。\n")
	sb.WriteString("- 高信息密度：最近安排要让用户每个可玩回合都体验到有效信息、关系变化、压力升级、收益/代价或新悬念，避免连续空转和纯氛围描写。\n")
	sb.WriteString("- 兼顾用户自由选择：给主线牵引和合理后续安排，但不要锁死唯一解，不要替用户做下一步选择。\n")
	sb.WriteString("- “正文Agent可读”区只放本轮后正文 Agent 可使用的信息；不得放会剧透关键真相、幕后动机或未来答案的内容。\n")
	sb.WriteString("- “后台导演私密”区可保存隐藏真相、长期反转、未公开角色动机、备用代价和伏笔回收条件。\n")
	sb.WriteString("- 事件目录只是规划输入；不要做强制/禁用队列，事件要融入当前设定、角色关系、冲突源和 RuleResolution 结果。\n")
	sb.WriteString("- 如果本回合出现终局、重大失败或用户偏离主线，要承接为分支状态和后续代价，而不是强行圆回原主线。\n")
	sb.WriteString("- 保存后的 director.md 必须包含全部固定标题，且不超过后端字节上限。\n\n")
	writeBlock(&sb, "故事标题", in.Title)
	writeBlock(&sb, "开局设定", in.Origin)
	writeBlock(&sb, "叙事风格 ID", in.StoryTellerID)
	writeBlock(&sb, "故事导演 ID", in.StoryDirectorID)
	writeBlock(&sb, "当前分支", in.BranchID)
	writeBlock(&sb, "叙事风格记忆沉淀规则（source: Teller state_memory, bounded）", in.StoryTellerMemoryRules)
	if in.BranchPlanningTurns > 0 {
		writeBlock(&sb, "最近分支规划回合数", fmt.Sprint(in.BranchPlanningTurns))
	}
	writeBlock(&sb, "允许读写的导演规划文件路径（source: backend guard）", in.DirectorPlanPaths)
	writeBlock(&sb, "当前导演规划文档快照（source: DirectorPlan docs, bounded）", in.DirectorPlanDocs)
	writeBlock(&sb, "导演规划模板要求（source: StoryDirector.strategy.planning_templates, bounded）", in.PlanningTemplates)
	writeBlock(&sb, "资料库导演上下文（source: lore index and bounded relevant entries）", in.LoreContext)
	writeBlock(&sb, "本回合 RuleResolution / TerminalOutcome 审计 JSON（source: turn audit, bounded）", in.TurnAuditJSON)
	writeBlock(&sb, "近期剧情历史（source: current branch turns, bounded）", in.TurnHistory)
	writeBlock(&sb, "故事记忆结构与字段协议（source: story memory schema, bounded）", in.StoryMemorySchema)
	writeBlock(&sb, "当前分支故事记忆（source: story memory, bounded）", firstNonEmpty(in.StoryMemory, in.StoryMemorySummary))
	writeBlock(&sb, "状态系统 Schema（source: story director actor_state, bounded）", in.ActorStateSchema)
	writeBlock(&sb, "当前状态系统快照（source: Snapshot.State.actors, bounded）", in.ActorState)
	writeBlock(&sb, "当前分支故事记忆摘要（source: story memory, bounded）", in.StoryMemorySummary)
	writeBlock(&sb, "故事导演规划配置（source: StoryDirector, bounded）", in.StoryDirectorPlan)
	if strings.TrimSpace(in.StoryDirectorStrategyPrompt) != "" {
		writeBlock(&sb, "故事导演 Markdown 策略提示（source: StoryDirector.strategy.prompt_markdown, bounded）", strategyPromptWithPriorityNote(in.StoryDirectorStrategyPrompt))
	}
	writeBlock(&sb, "可用事件类型目录（source: built-in + story director, bounded）", in.DirectorEventCatalog)
	sb.WriteString("\n请完成必要工具调用和文件编辑后，只输出一句中文摘要，不要输出故事正文、完整 Markdown 或 JSON patch。\n")
	return sb.String()
}

func strategyPromptWithPriorityNote(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ""
	}
	return "【优先级】结构化导演策略、工具权限、输出协议、RuleResolution、上下文上限和安全边界优先；本 Markdown 只用于补充导演偏好、禁忌、节奏和调度说明。\n\n" + prompt
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func InteractiveHotChoicesInstruction(in InteractiveHotChoicesPromptInput) string {
	var sb strings.Builder
	sb.WriteString("请基于以下互动故事上下文，生成下一轮快捷行动建议。\n\n")
	if strings.TrimSpace(in.LoreItems) != "" {
		writeBlock(&sb, "资料库", in.LoreItems)
	}
	if strings.TrimSpace(in.DirectorPlan) != "" {
		writeBlock(&sb, "后台导演规划可读区（source: director.md visible section, bounded）", in.DirectorPlan)
	}
	writeBlock(&sb, "历史回合", in.TurnHistory)
	if strings.TrimSpace(in.ExcludeChoices) != "" {
		writeBlock(&sb, "已展示过的选择（不要重复）", in.ExcludeChoices)
	}
	sb.WriteString("\n只输出 JSON，例如：{\"choices\":[\"我先观察门缝里的动静。\",\"我压低声音询问身边的人。\"]}。\n")
	return sb.String()
}

func writeField(sb *strings.Builder, name, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "（空）"
	}
	fmt.Fprintf(sb, "- %s：%s\n", name, value)
}

func writeBlock(sb *strings.Builder, title, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "（空）"
	}
	fmt.Fprintf(sb, "\n## %s\n\n%s\n", title, value)
}
