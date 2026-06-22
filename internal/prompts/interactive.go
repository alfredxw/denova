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
	LongTermMemory       string
	PreviousTurnsSummary string
}

type InteractiveStatePromptInput struct {
	Title             string
	Origin            string
	StoryTellerID     string
	StoryTellerMemory string
	BranchID          string
	LoreItems         string
	StoryMemorySchema string
	StoryMemory       string
	TurnHistory       string
	UserAction        string
	Narrative         string
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
	sb.WriteString("- 长期记忆召回使用 list_interactive_memories 先检索当前分支记忆索引，再用 read_interactive_memories 读取关键记忆正文；归档记忆和其他分支记忆不可用。\n")
	sb.WriteString("- 每轮必须在内部遵循这个流程：理解用户行动和当前快照 → 必要时召回资料库和长期记忆 → 结合导演规则裁定后果 → 输出 <NARRATIVE>。\n")
	sb.WriteString("- 如果工具不可用或召回失败，用已注入的快照和历史上下文继续生成，不要在正文中暴露工具错误或技术细节。\n\n")
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
	writeInteractiveReplyTargetInstruction(&sb, in.ReplyTargetChars, true)
	return sb.String()
}

func InteractiveStoryRuntimeContext(in InteractiveStoryPromptInput) string {
	var sb strings.Builder
	sb.WriteString("[本轮动态上下文]\n")
	writeInteractiveReplyTargetInstruction(&sb, in.ReplyTargetChars, false)
	sb.WriteString("\n## 召回说明\n")
	sb.WriteString("资料库正文不在本段上下文中预注入；需要时请通过 list_lore_items/read_lore_items 主动召回。\n")
	sb.WriteString("故事记忆仅提供当前分支的有界摘要；若本轮需要更细的长期事实，请通过 list_interactive_memories/read_interactive_memories 主动召回。\n\n")
	if strings.TrimSpace(in.LongTermMemory) != "" {
		writeBlock(&sb, "当前分支故事记忆", in.LongTermMemory)
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
		fmt.Fprintf(sb, "%s【最高篇幅约束】当前互动故事的每轮目标字数为 %d 个中文字左右；这是互动剧情正文唯一的内置字数目标，高于 CREATOR.md 的章节篇幅、导演规则和其他 Nova 内置提示中的篇幅倾向。你需要主动收束内容，优先写聚焦、有推进、可继续互动的一回合，不要依赖输出上限截断。%s", prefix, value, suffix)
		return
	}
	fmt.Fprintf(sb, "%s【最高篇幅约束】当前互动故事的每轮目标字数由 story 级运行参数决定；这是互动剧情正文唯一的内置字数目标，高于 CREATOR.md 的章节篇幅、导演规则和其他 Nova 内置提示中的篇幅倾向。运行时拿到具体目标后必须主动收束内容，优先写聚焦、有推进、可继续互动的一回合，不要依赖输出上限截断。%s", prefix, suffix)
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

请基于互动故事上下文续写下一回合，并严格按输出协议返回：读者应看到的故事正文只能放在 <NARRATIVE> 内。
本回合必须隐式完成：识别用户行动、判断相关角色和世界规则、裁定后果、制造新的可选择、保持角色和世界一致性；不要输出这些分析过程。
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
	TurnHistory    string
	ExcludeChoices string
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
	if strings.TrimSpace(in.LoreItems) != "" {
		writeBlock(&sb, "资料库", in.LoreItems)
	}
	writeBlock(&sb, "历史回合", in.TurnHistory)
	if strings.TrimSpace(in.ExcludeChoices) != "" {
		writeBlock(&sb, "已展示过的选择（不要重复）", in.ExcludeChoices)
	}
	sb.WriteString("\n只输出 JSON，例如：{\"choices\":[\"我先观察门缝里的动静。\",\"我压低声音询问身边的人。\"]}。\n")
	return sb.String()
}

func BuildInteractiveStateSystemInstruction() string {
	return strings.Join([]string{
		"你是 Nova 互动故事模式的互动记忆 Agent。",
		"你只负责把已经生成完成的互动故事回合整理为故事记忆表格 patch JSON，不负责续写剧情。",
		"必须只输出一个 JSON 对象，不要输出 Markdown、解释或代码块。",
		"JSON 格式必须是 {\"story_memory_patches\":[...]}。",
		"story_memory_patches 用于更新用户配置的故事记忆表；每条 patch 包含 op、structure_id、record_id、key、values 或 archived。",
		"必须基于注入的“故事记忆结构与字段协议”输出 patch；structure_id、key_field_id、values 字段名和值的写法要求都只能来自该协议。",
		"每次写入某张表时，values 必须按该表的字段列表逐字段填写：优先满足 required 字段，同时尽量补齐全部字段；字段值必须遵守表级 generation_instruction 和字段级 generation_instruction。",
		"不能只填 required 字段或本回合变化字段；有既有记录时必须沿用并整合未变化字段，不得省略字段、写空字符串或 null。",
		"字段值必须综合三类来源：历史回合上下文、资料库相关人物与设定、本回合前的既有故事记忆；新剧情负责更新变化，资料库负责校准设定，既有记忆负责保留未变化字段。",
		"op 仅使用 upsert、append、archive、restore；singleton 用 upsert，keyed 用带 key 的 upsert，append 结构记录新发生且后续需要承接的事实；结束或不再参与后续判断的记录用 archive。",
		"keyed 结构必须输出非空 key，且 values 必须包含 key_field_id 对应字段；key 必须等于该字段值。",
		"values 是纯文本字段对象，字段名必须来自对应结构；不要输出未来计划、快捷选择或没有依据的新设定。",
	}, "\n")
}

func InteractiveStateInstruction(in InteractiveStatePromptInput) string {
	var sb strings.Builder
	sb.WriteString("请根据以下互动故事上下文，生成本回合的故事记忆 patch JSON。\n\n")
	sb.WriteString("## 故事记忆建议\n")
	sb.WriteString("- 先读取“故事记忆结构与字段协议”，只按其中列出的结构、字段和字段要求生成 patch。\n")
	sb.WriteString("- 每条 patch 的 values 必须按目标表的字段逐项填写：required 字段不能为空；非 required 字段如果资料库、历史回合或既有记忆可支持，也要填写；已有值未变化时应沿用既有记忆，不要因为本回合没提到就清空。\n")
	sb.WriteString("- 信息来源优先级：本回合用户行动与正文用于判断最新变化；历史回合上下文用于补足连续事件、地点、时间和关系；资料库用于校准人物、设定、规则、地点、物品；本回合前的故事记忆作为填表基础和未变化字段来源。\n")
	sb.WriteString("- singleton 结构维护当前状态类信息，必须表现为回合结束后的最新状态；keyed 结构按 key_field_id 对应字段 upsert，更新同一个人物、地点、物品或任务时要保留并整合原记录；append 结构只追加已经发生且后续需要承接的事实。\n")
	sb.WriteString("- 资料库是稳定设定校准来源；故事记忆不得写入与资料库冲突的身份、规则、地点、物品或关系。若本回合正文和资料库疑似冲突，只记录已发生事实和待核对点，不要把矛盾扩写成新设定。\n")
	sb.WriteString("- 不要记录下一步行动建议、快捷选择或可选择入口；这些由独立快捷选择 Agent 生成。\n")
	sb.WriteString("- 若本回合没有值得沉淀的信息，可以返回空数组。\n\n")
	if strings.TrimSpace(in.LoreItems) != "" {
		writeBlock(&sb, "资料库", in.LoreItems)
	}
	writeBlock(&sb, "故事记忆结构与字段协议", in.StoryMemorySchema)
	writeBlock(&sb, "本回合前的故事记忆", in.StoryMemory)
	writeBlock(&sb, "历史回合上下文", in.TurnHistory)
	writeBlock(&sb, "用户本回合行动", in.UserAction)
	writeBlock(&sb, "已生成的本回合正文", in.Narrative)
	sb.WriteString("\n只输出 JSON，例如：{\"story_memory_patches\":[{\"op\":\"upsert\",\"structure_id\":\"current_state\",\"values\":{\"location\":\"旧宅门厅\",\"time\":\"2026-06-19 22:10\",\"previous_time\":\"2026-06-19 22:00\",\"elapsed_time\":\"约十分钟\",\"event\":\"主角发现门厅的铜铃会回应钥匙。\"}},{\"op\":\"upsert\",\"structure_id\":\"important_character\",\"key\":\"林川\",\"values\":{\"name\":\"林川\",\"brief\":\"熟悉旧宅机关的同行者\",\"relationship\":\"提醒主角谨慎使用铜钥匙\",\"status\":\"与主角同在旧宅门厅\"}},{\"op\":\"append\",\"structure_id\":\"plot_summary\",\"values\":{\"time\":\"2026-06-19 22:10\",\"place\":\"旧宅门厅\",\"event\":\"主角用铜钥匙触发门厅铜铃，确认旧宅对钥匙有反应。\"}}]}。\n")
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
