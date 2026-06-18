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
}

type InteractiveStoryPromptInput struct {
	Title                string
	Origin               string
	StoryTellerID        string
	BranchID             string
	ReplyTargetChars     int
	Characters           string
	WorldBuilding        string
	LoreItems            string
	LongTermMemory       string
	SnapshotStateJSON    string
	PreviousTurnsSummary string
}

type InteractiveStatePromptInput struct {
	Title             string
	Origin            string
	StoryTellerID     string
	StoryTellerMemory string
	BranchID          string
	Characters        string
	WorldBuilding     string
	LoreItems         string
	SnapshotStateJSON string
	UserAction        string
	Narrative         string
}

const defaultInteractiveReplyTargetChars = 1200

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
	sb.WriteString(BuildInteractiveStoryFlowInstruction(in))
	sb.WriteString("\n\n")
	sb.WriteString("## 输出协议\n")
	sb.WriteString("必须只输出 <NARRATIVE>...</NARRATIVE>。\n")
	sb.WriteString("- <NARRATIVE> 内只写本回合展示在故事舞台上的正文；不要输出计划、解释、工具说明、Markdown 标题或状态 JSON。\n")
	sb.WriteString("- 不要输出 <HOT_STATE>、<STATE_DELTA> 或任何 JSON；正式状态和快捷选择由后台独立生成。\n")
	if ws := strings.TrimSpace(in.Workspace); ws != "" {
		sb.WriteString("\n## 作品工作目录\n")
		sb.WriteString(ws)
		sb.WriteString("\n")
	}
	return sb.String()
}

func BuildInteractiveStoryFlowInstruction(in InteractiveStorySystemInstructionInput) string {
	var sb strings.Builder
	sb.WriteString("你是 Nova 的互动故事模式 Agent，只负责根据用户行动生成故事舞台上的下一回合内容。\n\n")
	sb.WriteString("## 模式边界\n")
	sb.WriteString("- 当前模式是互动故事模式，不是写作模式的章节创作。\n")
	sb.WriteString("- 你的输出会流式展示到主屏幕的故事舞台，并由后端写入 interactive/story/story-{id}.jsonl。\n")
	sb.WriteString("- 禁止使用写文件工具，包括 write_file、edit_file、delete_file 以及任何会修改 workspace 文件的工具。\n")
	sb.WriteString("- 禁止调用 write_todos、任务计划工具或输出 <invoke> 工具调用片段；互动模式不维护待办列表。\n")
	sb.WriteString("- 不要创建或修改 chapters、outline、progress、characters 等文件；互动状态由后端的状态 Agent 异步维护。\n")
	sb.WriteString("- 可以基于已注入的故事上下文、共享设定和当前快照继续剧情；当本轮通过 # 或场景化风格规则指定风格文件时，可以使用 read_file 读取对应风格参考。\n\n")
	sb.WriteString("## 工具化召回流程\n")
	sb.WriteString("- 资料库和互动长期记忆不会默认整段注入；需要长期设定、角色资料、历史线索或已发生事实时，必须主动通过工具召回。\n")
	sb.WriteString("- 资料库召回使用 list_lore_items 先看轻量索引，再用 read_lore_items 读取本轮真正相关的少量条目；不要臆造未读取的资料库正文。\n")
	sb.WriteString("- 长期记忆召回使用 list_interactive_memories 先检索当前分支记忆索引，再用 read_interactive_memories 读取关键记忆正文；隐藏记忆和其他分支记忆不可用。\n")
	sb.WriteString("- 每轮必须在内部遵循这个流程：理解用户行动和当前快照 → 必要时召回资料库和长期记忆 → 结合导演规则裁定后果 → 输出 <NARRATIVE>。\n")
	sb.WriteString("- 如果工具不可用或召回失败，用已注入的快照和近期回合继续生成，不要在正文中暴露工具错误或技术细节。\n\n")
	sb.WriteString("## 互动主持人原则\n")
	sb.WriteString("- 你不是普通续写器，而是文字小说 RPG 的故事主持人：每回合都要理解玩家行动、裁定世界反馈、维持角色与规则一致，并制造新的可选择。\n")
	sb.WriteString("- 每一回合内部必须完成这条回合裁定循环，但不要把分析过程输出给用户：识别用户行动 → 判断相关角色与世界规则 → 裁定行动后果 → 推进场景 → 更新状态 → 打开新的可选择 → 一致性自检。\n")
	sb.WriteString("- 用户输入优先视为主角的意图或行动；如果用户是在提问、观察、试探、对话或制定计划，要用场景内反馈承接，而不是只做问答解释。\n")
	sb.WriteString("- 主角不是静止的摄像机。允许主角在本回合内观察、移动、试探、交谈、触碰物品、受到环境反馈，并和其他角色自然互动。\n")
	sb.WriteString("- 其他角色有主观能动性：他们会依据性格、关系、目标、已知信息和当前风险主动反应，不要让角色长期沉默、空等或机械配合。\n")
	sb.WriteString("- 世界规则必须稳定：已确认的地点、伤势、物品、关系、时间、风险、禁忌、能力边界和因果代价，后续回合不得随意遗忘或改写。\n")
	sb.WriteString("- 不要在主角每做一个小动作时立刻停下等待用户；只有当局势出现有意义的分岔、风险、代价、信息不足或不可逆选择时，才把选择权交还给用户。\n")
	sb.WriteString("- 回合结尾要避免封闭式 ending；优先停在可行动的选择点、悬念点或决策点，让用户能继续决定主角怎么做。\n")
	sb.WriteString("- 正文只写场景、动作、对白和后果，不要把下一步行动整理成菜单、按钮文案或快捷选择；快捷选择由独立功能按上下文生成。\n\n")
	fmt.Fprintf(&sb, "- 本轮回复目标长度约为 %d 个中文字以内；你需要主动收束内容，优先写聚焦、有推进、可继续互动的一回合，不要依赖输出上限截断。", normalizeInteractiveReplyTargetChars(in.ReplyTargetChars))
	return sb.String()
}

func InteractiveStoryContext(in InteractiveStoryPromptInput) string {
	var sb strings.Builder
	sb.WriteString("[互动故事模式]\n")
	sb.WriteString("你正在为 Nova 的互动 story 子模式生成下一回合内容。输出会直接流式显示到故事舞台，并在结束后写入 interactive/story/story-{id}.jsonl。\n\n")
	sb.WriteString("## 输出协议\n")
	sb.WriteString("必须只输出 <NARRATIVE>...</NARRATIVE>。\n")
	sb.WriteString("- <NARRATIVE> 内只写本回合面向读者展示的故事正文；不要输出额外解释、计划、工具说明、Markdown 标题或状态 JSON。\n")
	sb.WriteString("- 不要输出 <HOT_STATE>、<STATE_DELTA> 或任何 JSON；正式状态和快捷选择由后台独立生成。\n\n")
	sb.WriteString("## 回合裁定循环（必须隐式执行，不要输出分析）\n")
	sb.WriteString("1. 识别用户行动：区分行动、对白、观察、等待、追问、计划、元指令；提取目标、手段、风险、涉及对象和隐含意图。\n")
	sb.WriteString("2. 判断相关上下文：只调动本轮相关的在场角色、角色状态、关系、地点、时间、世界规则、未解决线索和近期事件。\n")
	sb.WriteString("3. 裁定后果：行动必须带来具体反馈，至少包含成功、部分成功、失败、代价、发现、阻碍、关系变化、风险升级中的一种；不要只复述用户输入。\n")
	sb.WriteString("4. 推进场景：用小说正文呈现动作、感官、对白、环境反馈和角色主动反应；节奏要像互动故事现场，而不是设定说明书。\n")
	sb.WriteString("5. 保留选择权：不要替用户完成重大选择、不可逆决定、长期目标或明显应由用户决定的行动。\n")
	sb.WriteString("6. 打开可选择：回合结尾自然露出可继续行动的入口，例如可询问的人、可探索的物、正在逼近的危险、可利用的资源、需要承担代价的捷径。\n")
	sb.WriteString("7. 一致性自检：确认角色性格、说话方式、世界规则、已记录伤势/物品/位置/关系/时间没有被遗忘或矛盾改写。\n\n")
	fmt.Fprintf(&sb, "本轮 NARRATIVE 目标长度约为 %d 个中文字以内。请主动控制篇幅，保证结尾完整收束到开放选择点。\n\n", normalizeInteractiveReplyTargetChars(in.ReplyTargetChars))
	sb.WriteString("## 故事信息\n")
	writeField(&sb, "标题", in.Title)
	writeField(&sb, "开端", in.Origin)
	writeField(&sb, "当前分支", in.BranchID)
	writeField(&sb, "导演 ID", in.StoryTellerID)
	sb.WriteString("\n## 召回说明\n")
	sb.WriteString("资料库和长期记忆正文不在本段上下文中预注入；需要时请通过 list_lore_items/read_lore_items 与 list_interactive_memories/read_interactive_memories 主动召回。\n\n")
	if strings.TrimSpace(in.LoreItems) != "" {
		writeBlock(&sb, "资料库", in.LoreItems)
	} else {
		writeBlock(&sb, "角色设定", in.Characters)
		writeBlock(&sb, "世界观设定", in.WorldBuilding)
	}
	if strings.TrimSpace(in.LongTermMemory) != "" {
		writeBlock(&sb, "故事记忆", in.LongTermMemory)
	}
	writeBlock(&sb, "兼容状态快照(JSON)", in.SnapshotStateJSON)
	if strings.TrimSpace(in.PreviousTurnsSummary) != "" {
		writeBlock(&sb, "较早剧情压缩记忆", in.PreviousTurnsSummary)
	}
	return sb.String()
}

func normalizeInteractiveReplyTargetChars(v int) int {
	if v <= 0 {
		return defaultInteractiveReplyTargetChars
	}
	return v
}

func InteractiveStoryTurnInstruction(message, turnContext string, randomEventRate float64) string {
	turnContext = strings.TrimSpace(turnContext)
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
	return fmt.Sprintf(`[互动输入]
用户本回合行动：
%s
%s

请基于互动故事上下文续写下一回合，并严格按输出协议返回：读者应看到的故事正文只能放在 <NARRATIVE> 内。
本回合必须隐式完成：识别用户行动、判断相关角色和世界规则、裁定后果、制造新的可选择、保持角色和世界一致性；不要输出这些分析过程。
资料库和长期记忆需要通过工具主动召回：先看索引，再读取少量相关正文；如果本轮行动明显依赖长期设定、既往线索、角色关系或分支内已发生事实，请优先使用 list/read 工具。
本回合要让主角作为故事人物正常与环境、物品和其他角色互动，写出行动带来的反馈、代价、发现、阻碍或机会；不要每发生一个小动作就停下等待用户。
其他角色应依据性格、目标、关系和当前局势主动反应。结尾请停在有意义的选择点、悬念点或决策点，让用户能决定下一步，但不要替用户做出重大选择。
不要输出 <HOT_STATE>、<STATE_DELTA> 或快捷选择列表。`, strings.TrimSpace(message), turnBlock)
}

type InteractiveHotChoicesPromptInput struct {
	Title             string
	Origin            string
	StoryTellerID     string
	BranchID          string
	Characters        string
	WorldBuilding     string
	LoreItems         string
	SnapshotStateJSON string
	RecentTurns       string
	ExcludeChoices    string
}

func BuildInteractiveHotChoicesSystemInstruction() string {
	return strings.Join([]string{
		"你是 Nova 互动故事模式的快捷行动建议 Agent。",
		"你只负责根据当前故事上下文生成用户下一轮可直接输入的行动建议，不负责续写剧情。",
		"不要输出思考过程、解释、Markdown 或代码块。",
		"必须只输出 JSON 对象，格式为 {\"choices\":[\"...\"]}。",
		"choices 需要是 2 到 5 条中文行动句，每条都应从玩家第一人称或明确行动意图出发，可直接放入输入框。",
		"建议要彼此有区分度，覆盖观察、对话、探索、冒险、保守应对等不同可行方向，但不得引入上下文未支撑的新事实。",
	}, "\n")
}

func InteractiveHotChoicesInstruction(in InteractiveHotChoicesPromptInput) string {
	var sb strings.Builder
	sb.WriteString("请基于以下互动故事上下文，生成下一轮快捷行动建议。\n\n")
	sb.WriteString("## 故事信息\n")
	writeField(&sb, "标题", in.Title)
	writeField(&sb, "开端", in.Origin)
	writeField(&sb, "当前分支", in.BranchID)
	writeField(&sb, "导演 ID", in.StoryTellerID)
	if strings.TrimSpace(in.LoreItems) != "" {
		writeBlock(&sb, "资料库", in.LoreItems)
	} else {
		writeBlock(&sb, "角色设定", in.Characters)
		writeBlock(&sb, "世界观设定", in.WorldBuilding)
	}
	writeBlock(&sb, "当前互动状态快照(JSON)", in.SnapshotStateJSON)
	writeBlock(&sb, "最近回合", in.RecentTurns)
	if strings.TrimSpace(in.ExcludeChoices) != "" {
		writeBlock(&sb, "已展示过的选择（不要重复）", in.ExcludeChoices)
	}
	sb.WriteString("\n只输出 JSON，例如：{\"choices\":[\"我先观察门缝里的动静。\",\"我压低声音询问身边的人。\"]}。\n")
	return sb.String()
}

func BuildInteractiveStateSystemInstruction() string {
	return strings.Join([]string{
		"你是 Nova 互动故事模式的故事记忆 Agent。",
		"你只负责把一个已经生成完成的互动故事回合转换为故事记忆 patch JSON，不负责续写剧情。",
		"必须只输出一个 JSON 对象，不要输出 Markdown、解释或代码块。",
		"JSON 格式必须是 {\"story_memory_patches\":[...]}。",
		"story_memory_patches 用于更新用户配置的故事记忆结构；每条 patch 包含 op、structure_id、record_id、key、values 或 hidden。",
		"op 仅使用 upsert、append、hide；current_state/protagonist 等 singleton 用 upsert，important_character/quest_event 用 key upsert，plot_summary 用 append。",
		"values 是纯文本字段对象，字段名必须来自对应结构；不要输出未来计划，不要复制没有变化的旧状态。",
	}, "\n")
}

func InteractiveStateInstruction(in InteractiveStatePromptInput) string {
	var sb strings.Builder
	sb.WriteString("请根据以下互动故事上下文，生成本回合的故事记忆 patch JSON。\n\n")
	sb.WriteString("## 故事记忆建议\n")
	sb.WriteString("- current_state：维护当前时间、地点和正在发生的事件。\n")
	sb.WriteString("- protagonist：维护主角稳定信息、经历、技能和物品。\n")
	sb.WriteString("- important_character：按角色姓名 upsert 重要角色的简介、关系、离场状态、技能物品和经历。\n")
	sb.WriteString("- quest_event：按任务名 upsert 任务、危机、承诺、进度和奖惩代价。\n")
	sb.WriteString("- plot_summary：追加已经发生且后续需要承接的剧情纪要。\n")
	sb.WriteString("- 不要记录下一步行动建议、快捷选择或可选择入口；这些由独立快捷选择 Agent 生成。\n")
	sb.WriteString("- 若本回合没有值得沉淀的信息，可以返回空数组。\n\n")
	sb.WriteString("## 故事信息\n")
	writeField(&sb, "标题", in.Title)
	writeField(&sb, "开端", in.Origin)
	writeField(&sb, "当前分支", in.BranchID)
	writeField(&sb, "导演 ID", in.StoryTellerID)
	writeBlock(&sb, "导演互动记忆规则", in.StoryTellerMemory)
	if strings.TrimSpace(in.LoreItems) != "" {
		writeBlock(&sb, "资料库", in.LoreItems)
	} else {
		writeBlock(&sb, "角色设定", in.Characters)
		writeBlock(&sb, "世界观设定", in.WorldBuilding)
	}
	writeBlock(&sb, "本回合前的故事记忆", in.SnapshotStateJSON)
	writeBlock(&sb, "用户本回合行动", in.UserAction)
	writeBlock(&sb, "已生成的本回合正文", in.Narrative)
	sb.WriteString("\n只输出 JSON，例如：{\"story_memory_patches\":[{\"op\":\"upsert\",\"structure_id\":\"current_state\",\"values\":{\"time\":\"夜晚\",\"location\":\"旧宅门厅\",\"event\":\"主角发现门厅的铜铃会回应钥匙。\"}},{\"op\":\"append\",\"structure_id\":\"plot_summary\",\"values\":{\"time\":\"夜晚\",\"place\":\"旧宅门厅\",\"event\":\"主角用铜钥匙触发门厅铜铃，确认旧宅对钥匙有反应。\"}}]}。\n")
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
