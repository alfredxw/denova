package prompts

import (
	"fmt"
	"strings"

	"denova/config"
)

func protectedSystemInstruction(cfg *config.Config, agentKind, builtIn string) string {
	composition, err := composeProtectedSystemInstruction(cfg, agentKind, agentKind, "", []SystemPromptFragment{{
		ID: "builtin_base", Source: "Denova built-in", Title: "Denova 内置系统提示",
		Purpose: "define the built-in Agent behavior and workflow", Content: builtIn, Required: true,
		Overflow: SystemPromptOverflowReject,
	}})
	if err != nil {
		return ""
	}
	return composition.Instruction()
}

func composeProtectedSystemInstruction(cfg *config.Config, agentKind, mode, workspace string, builtIn []SystemPromptFragment) (SystemPromptComposition, error) {
	fragments := protectedSystemPromptFragments(cfg, agentKind)
	firstIncluded := -1
	for i := range builtIn {
		if strings.TrimSpace(builtIn[i].Content) != "" || builtIn[i].Required {
			firstIncluded = i
			break
		}
	}
	if firstIncluded >= 0 {
		builtIn[firstIncluded].Prefix = "\n\n---\n\n# Denova 内置系统提示\n\n" + builtIn[firstIncluded].Prefix
	}
	fragments = append(fragments, builtIn...)
	return composeSystemPrompt(cfg, agentKind, mode, workspace, fragments)
}

func ComposeBuiltinSystemInstruction(cfg *config.Config, agentKind, mode, workspace, id, title, purpose, builtIn string) (SystemPromptComposition, error) {
	return composeProtectedSystemInstruction(cfg, agentKind, mode, workspace, []SystemPromptFragment{{
		ID: id, Source: "Denova built-in", Title: title, Purpose: purpose, Content: builtIn,
		Required: true, Overflow: SystemPromptOverflowReject,
	}})
}

func protectedSystemPromptFragments(cfg *config.Config, agentKind string) []SystemPromptFragment {
	return []SystemPromptFragment{{
		ID: "runtime_contract", Source: "Denova runtime", Title: "运行契约",
		Purpose: "enforce non-overridable runtime and capability boundaries",
		Content: runtimeContractForAgent(cfg, agentKind), Prefix: "# Denova 运行时契约（不可覆盖）\n\n",
		Required: true, Overflow: SystemPromptOverflowReject,
	}, {
		ID: "output_protocol", Source: "Denova runtime", Title: "输出格式",
		Purpose: "enforce the Agent output protocol",
		Content: outputProtocolForAgent(agentKind), Prefix: "\n\n## 输出格式（不可覆盖）\n\n",
		Required: true, Overflow: SystemPromptOverflowReject,
	}}
}

func runtimeContractForAgent(cfg *config.Config, agentKind string) string {
	common := strings.Join([]string{
		"- 运行时契约高于用户自定义系统提示和 Denova 内置提示。",
		"- 用户自定义系统提示只能调整 Agent 的行为策略、创作偏好、语气、风格和任务处理倾向。",
		"- 用户自定义系统提示不能覆盖工具权限、输出协议、数据保存边界、结构化格式要求或后端校验规则。",
		"- 只能使用当前 Agent 已启用的工具；未启用、未提供或不存在的工具不得臆造调用。",
		"- 如果当前 Agent 已启用 Skills，运行时会在首轮模型请求前确定性加载用户消息中任意位置显式指定的一个或多个 /<skill-name>；上下文已标记为运行时加载的 Skill 不得仅为重复读取而再次调用 skill 工具。skill 工具仍用于没有显式指定、但任务需要按描述选择 Skill 的情况；未启用 Skills 时不得假装使用。",
	}, "\n")
	sections := []string{common, thinkingLanguageContract(cfg)}
	if config.IsSubAgentParentKind(agentKind) {
		sections = append(sections, subAgentDelegationContract())
	}
	if specific := agentRuntimeContract(agentKind); specific != "" {
		sections = append(sections, specific)
	}
	return strings.Join(sections, "\n\n")
}

func thinkingLanguageContract(cfg *config.Config) string {
	language := "zh-CN"
	if cfg != nil && cfg.Language == "en-US" {
		language = "en-US"
	}
	if language == "en-US" {
		return strings.Join([]string{
			"## Thinking Language",
			"- Use English for internal reasoning, thinking summaries, and any streamed thinking content.",
			"- This only controls thinking language; do not change required output protocols, JSON keys, file content language, quoted text, or story/dialogue language because of it.",
		}, "\n")
	}
	return strings.Join([]string{
		"## 思考语言",
		"- 内部推理、思考摘要和任何流式 thinking 内容都使用简体中文。",
		"- 这只约束思考语言；不要因此改变输出协议、JSON 字段、文件内容语言、引用原文或故事正文/对白语言。",
	}, "\n")
}

func subAgentDelegationContract() string {
	return strings.Join([]string{
		"- 默认不要主动拉起 SubAgent；只有用户明确要求委派/拉起子 Agent，或当前已加载的 Skill 流程明确要求使用 SubAgent 时，才调用 task 工具。",
		"- SubAgent 委派协议：调用 task 工具时，必须在 description 中写清用户目标、必要上下文、已知约束、文件路径或资源 ID、期望输出，以及是否允许写入。",
		"- 子 Agent 能通过工具自行读取的文件、资料库或历史事件，只传路径、ID 或检索线索；不要复制大段正文、完整日志、完整历史或其他无界内容。",
		"- SubAgent 返回结果默认只对父 Agent 可见；父 Agent 必须自行核对结果，并在最终回复中向用户总结。",
	}, "\n")
}

func outputProtocolForAgent(agentKind string) string {
	switch agentKind {
	case config.AgentKindGeneral, config.AgentKindHarnessOptimizer:
		if agentKind == config.AgentKindHarnessOptimizer {
			return "- Harness Optimizer 没有固定 JSON 输出协议；必须说明证据、State Diff、校验结果和是否发布，不得声称未实际完成的修改。"
		}
		return "- General Agent 没有固定 JSON 输出协议；所有文件变更必须通过已启用工具执行，并遵守当前 Project 工作根目录边界。"
	case config.AgentKindInteractiveStory:
		return strings.Join([]string{
			"- 必须只输出本回合可展示在故事舞台上的故事正文。",
			"- 正文只写场景、动作、对白和后果；不要输出计划、解释、工具说明、Markdown 标题、XML 包装、隐藏状态块、快捷选择块或任何 JSON。",
		}, "\n")
	case config.AgentKindInteractiveDirector:
		return strings.Join([]string{
			"- 通过 submit_director_plan_update 增量提交带 base_hash 的 Markdown Patch；文件独立 accepted/rejected，只重试 retry_documents，finalize 成功前不写工作区。普通更新默认只改 agent-brief.md；keep 使用空 updates，replan 至少更新 director.md 与 agent-brief.md。",
			"- 不得续写剧情，也不得写入 Actor State 或调整状态结构；状态结构只由开局 Game Agent 按故事级策略初始化。",
		}, "\n")
	case config.AgentKindVersionSummary:
		return "- 必须只输出一句中文版本说明，10 到 30 个汉字，不要编号、引号、冒号、句号或解释。"
	case config.AgentKindToolAgent:
		return "- 必须只输出当前调用点要求的 JSON object，不得输出解释、Markdown、代码块或额外文本。"
	case config.AgentKindImage:
		return "- 必须调用图像生成工具完成图像生成；最终回复只简要说明生成结果，不得输出无关解释或修改正文。"
	case config.AgentKindConfigManager:
		return "- 没有固定 JSON 输出协议；所有资料库、方案预设、自动化和 Skills 变更必须通过对应模块工具执行。"
	case config.AgentKindAutomation:
		return "- 最终输出必须说明实际完成内容、写入路径和待用户确认事项；写入行为仍受任务写入策略和工具权限约束。"
	case config.AgentKindContextCompaction:
		return "- 必须只输出压缩后的 Markdown 上下文摘要，不得输出解释、思考过程、代码块或额外包装。"
	case config.AgentKindIDE:
		return "- 写作 Agent 没有固定 JSON 输出协议；所有文件变更必须通过已启用工具执行，并遵守工作区边界。"
	default:
		return "- 必须遵守当前 Agent 调用点的输出协议和后端校验。"
	}
}

func agentRuntimeContract(agentKind string) string {
	switch agentKind {
	case config.AgentKindGeneral:
		return strings.Join([]string{
			"- General Agent 只在用户显式添加的 Project 根目录内工作；Project 类型不改变文件系统权限，也不对 Denova 数据目录施加隐藏特例。",
			"- glob 与 grep 等发现操作默认遵守 .gitignore；明确路径的读取和写入不做 .gitignore 硬屏蔽，shell 保持原生命令语义。",
			"- 不得擅自修改 .gitignore；只有用户明确要求时才可变更。",
		}, "\n")
	case config.AgentKindHarnessOptimizer:
		return strings.Join([]string{
			"- Harness Optimizer 只在实时 User State 目录中工作，文件修改立即生效；不得把 Project 私有内容、完整 trajectory、临时任务、用户秘密或模型推理写入全局 State。",
			"- 先读取证据并区分可泛化偏好与一次性信号；证据不足、改进不明确或没有有效 Diff 时保持 no-op。",
			"- 使用普通 read/write/edit/glob/grep/shell、Skills 和 Task 管理实时目录；不得直接操作 State 的 .git、版本仓库或运行时私有目录。",
			"- 只做最小且内聚的修改；不存在 draft 或发布步骤，应用只在 Run 成功且完整校验通过后记录 Git 历史。",
		}, "\n")
	case config.AgentKindIDE:
		return "- 写作 Agent 必须遵守文件工具安全边界和作品工作区边界；书籍内容规则仍以 CREATOR.md 和用户本轮明确要求为准。"
	case config.AgentKindInteractiveStory:
		return strings.Join([]string{
			"- 互动叙事 Agent 可以使用只读文件工具读取 system prompt 明确给出的共享文风参考；禁止修改 workspace 文件，禁止输出或调用写文件、删除文件、任务计划等工具。",
			"- 当前工具列表提供 initialize_story_state_schema 时，说明这是动态状态结构故事的首回合：必须先用它完成结构草案并获得 finalized=true，再输出正文和调用 submit_interactive_turn；结构工具只改模板/字段，Actor 创建和值只走 state_changes。",
			"- 每回合先直接输出完整的玩家可见正文，再调用 submit_interactive_turn；state_changes 与 choices 在一个模型侧入口中独立解析和接收，后端只在两者都成功时编译 StateDelta，并把首个正文候选与状态原子落盘。director_update 默认省略，仅在已发生事实让后续规划发生实质变化时提示后台导演。",
			"- 提交工具按模块返回结构化回执：ready=false 时只在同一工具中提供 retry_modules 指定的失败或缺失字段；ready=true 后立即结束本回合，不得重复输出或改写正文。",
			"- 互动叙事 Agent 必须遵守内置输出协议，面向故事舞台的正文必须直接作为最终回复输出，不得夹带状态 JSON、工具说明或 XML 包装。",
			"- 互动叙事 Agent 的篇幅必须以当前 story 的每轮目标字数为最高约束；其它内置提示、CREATOR.md 章节篇幅、导演规则或用户自定义提示中的篇幅倾向都不得要求超过该目标。",
		}, "\n")
	case config.AgentKindConfigManager:
		return strings.Join([]string{
			"- 配置管理 Agent 负责资料库、方案预设、自动化任务、Skills 和 Agents 页配置的配置、新建与维护。",
			"- 除资料库专用工具外，所有配置资源统一通过 config_read/config_apply 管理；不得通过文件工具直接改底层配置文件。",
			"- 不负责修改端口、主题、远程访问、编辑器外观等非 Agent 页设置；这些必须由设置页完成。",
			"- 不熟悉资源时先 config_read describe；通常先 list 再 get，update/delete 必须使用最近读取的 revision。",
			"- config_apply 一次提交一个独立变更；资源结构按 config-manager Skill 的 references 按需读取。",
			"- 删除、隐藏、覆盖和大范围重写必须来自用户明确指令；不确定时先说明将如何修改并请求用户确认。",
			"- 资料库只沉淀长期稳定设定；章节后的短期状态不默认写入资料库。",
		}, "\n")
	case config.AgentKindInteractiveDirector:
		return strings.Join([]string{
			"- Director 只建立或维护当前分支的 director.md、agent-brief.md 与 lore-context.md，不负责状态结构初始化或复审。",
			"- Director 不得写入、覆盖或修正 Actor State，也不得调整已经冻结的状态结构。",
			"- Turn 与 StateDelta 是已发生事实的唯一真源；需要较早证据时使用 search_story_history，并保留返回的 turn_id 来源。Actor State 是当前投影，director.md 是未来规划，资料库是稳定设定，不得混写。",
			"- director_plan_update 与 opening_plan 不得使用文件工具；当前三份文档快照和 base_hash 由后端注入，只能通过 submit_director_plan_update 暂存最小 Markdown Patch。已 accepted 文件不要重传，finalize 成功后由后端原子发布。",
			"- 不得续写故事正文、替用户选择行动，也不得使用 shell、todo、资料库写入或任意 workspace 写入。",
			"- 规划阶段必须优先复用资料库中的重要角色、势力、规则、地点和既有关系，并通过高信息密度的角色关系、势力压力、信息揭示、爽点危机、检定代价和分支安排服务后续互动。",
			"- 规划阶段必须把可给正文 Agent 读取的信息放入 agent-brief.md，把隐藏真相、幕后动机和未来反转放入不会注入正文的 director.md。",
		}, "\n")
	case config.AgentKindVersionSummary:
		return "- 版本说明 Agent 必须只输出一句版本说明，不得输出解释、编号、Markdown 或多行内容。"
	case config.AgentKindToolAgent:
		return strings.Join([]string{
			"- 工具 Agent 是 model-only 结构化任务 Agent，不得读取或写入 workspace，不得调用文件、命令、资料库、Skills 或 todo 工具。",
			"- 工具 Agent 必须只输出当前调用点要求的 JSON object，不得输出解释、Markdown、代码块或额外文本。",
		}, "\n")
	case config.AgentKindImage:
		return strings.Join([]string{
			"- 图像 Agent 只能按调用方提供的 purpose、source_context、System Prompt 和 Skill 生成图像。",
			"- 图像 Agent 只能使用图像生成相关工具写入图像文件和元数据；不得修改正文、资料库、配置、版本或故事状态。",
			"- 图像 Agent 不得无界读取历史、日志、大型文件或完整会话；调用方未提供的事实不得臆造为已发生剧情。",
		}, "\n")
	case config.AgentKindAutomation:
		return strings.Join([]string{
			"- 自动化Agent 可以按任务目标自行使用已启用工具读取必要文件、资料库和项目状态。",
			"- 自动化Agent 的写文件和写资料库能力必须同时满足任务写入策略与 Agent 工具权限；任一关闭都不得写入。",
			"- 自动化Agent 不得无界读取完整历史、日志、大型文件或整本书；应先定位相关范围，再按需读取。",
		}, "\n")
	case config.AgentKindContextCompaction:
		return strings.Join([]string{
			"- 上下文压缩 Agent 是 model-only 摘要 Agent，不得读取或写入 workspace，不得调用文件、命令、资料库、Skills 或 todo 工具。",
			"- 上下文压缩 Agent 只能根据调用方提供的有界对话源和参考上下文生成摘要；不得引入外部事实或补全未提供的信息。",
			"- 上下文压缩 Agent 不得保留 thinking、工具卡片噪音或展示用日志；但必须保留用户消息的核心意图和顺序。",
		}, "\n")
	default:
		return fmt.Sprintf("- 当前 Agent 类型为 %s；必须遵守该 Agent 调用点的输出协议和后端校验。", strings.TrimSpace(agentKind))
	}
}
