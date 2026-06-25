package agent

import (
	"fmt"
	"strings"

	"nova/config"
)

func protectedSystemInstruction(cfg *config.Config, agentKind, builtIn string) string {
	builtIn = strings.TrimSpace(builtIn)
	var sb strings.Builder
	sb.WriteString("# Nova 运行时契约（不可覆盖）\n\n")
	sb.WriteString(runtimeContractForAgent(cfg, agentKind))
	if outputProtocol := outputProtocolForAgent(agentKind); strings.TrimSpace(outputProtocol) != "" {
		sb.WriteString("\n\n## 输出格式（不可覆盖）\n\n")
		sb.WriteString(outputProtocol)
	}
	resolvedPrompt := config.ResolveAgentPrompt(cfg, agentKind)
	if flow := resolvedPrompt.FlowPrompt; flow != "" {
		sb.WriteString("\n\n---\n\n")
		sb.WriteString("# 用户自定义流程规则（受保护高优先级）\n\n")
		sb.WriteString("以下流程规则优先于 Nova 内置流程规则；但不得覆盖运行时契约、输出格式、工具权限和后端校验。若存在冲突，必须忽略冲突部分。\n\n")
		sb.WriteString(flow)
	}
	if custom := resolvedPrompt.SystemPrompt; custom != "" {
		sb.WriteString("\n\n---\n\n")
		sb.WriteString("# 用户自定义系统提示（受保护最高优先级）\n\n")
		sb.WriteString("以下提示在 Agent 行为、创作偏好、策略和风格上优先于 Nova 内置提示；但不得覆盖上一节运行时契约。若以下提示与运行时契约冲突，必须忽略冲突部分。\n\n")
		sb.WriteString(custom)
	}
	if builtIn != "" {
		sb.WriteString("\n\n---\n\n")
		sb.WriteString("# Nova 内置系统提示\n\n")
		sb.WriteString(builtIn)
	}
	return sb.String()
}

func runtimeContractForAgent(cfg *config.Config, agentKind string) string {
	common := strings.Join([]string{
		"- 运行时契约高于用户自定义系统提示和 Nova 内置提示。",
		"- 用户自定义系统提示只能调整 Agent 的行为策略、创作偏好、语气、风格和任务处理倾向。",
		"- 用户自定义系统提示不能覆盖工具权限、输出协议、数据保存边界、结构化格式要求或后端校验规则。",
		"- 只能使用当前 Agent 已启用的工具；未启用、未提供或不存在的工具不得臆造调用。",
		"- 如果当前 Agent 已启用 Skills，用户输入 /<skill-name> 表示要求你调用 skill 工具加载该 Skill 后再继续处理；未启用 Skills 时不得假装使用。",
	}, "\n")
	sections := []string{common, thinkingLanguageContract(cfg)}
	if config.IsDeepAgentParentKind(agentKind) {
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
		"- SubAgent 委派协议：调用 task 工具时，必须在 description 中写清用户目标、必要上下文、已知约束、文件路径或资源 ID、期望输出，以及是否允许写入。",
		"- 子 Agent 能通过工具自行读取的文件、资料库或故事记忆，只传路径、ID 或检索线索；不要复制大段正文、完整日志、完整历史或其他无界内容。",
		"- SubAgent 返回结果默认只对父 Agent 可见；父 Agent 必须自行核对结果，并在最终回复中向用户总结。",
	}, "\n")
}

func outputProtocolForAgent(agentKind string) string {
	switch agentKind {
	case config.AgentKindInteractiveStory:
		return strings.Join([]string{
			"- 必须只输出 <NARRATIVE>...</NARRATIVE>。",
			"- <NARRATIVE> 内只写展示在故事舞台上的正文；不要输出计划、解释、工具说明、Markdown 标题",
		}, "\n")
	case config.AgentKindInteractiveState:
		return "- 必须只输出符合互动记忆 schema 的 JSON object，格式为 {\"story_memory_patches\":[...]}；每条 patch 必须按目标表的字段协议填写完整 values，所有字段都必须出现且不能为空，不得输出 Markdown、解释或代码块。"
	case config.AgentKindInteractiveHotChoices:
		return "- 必须只输出 JSON object，格式为 {\"choices\":[\"...\"]}；不得续写剧情或修改故事状态。"
	case config.AgentKindVersionSummary:
		return "- 必须只输出一句中文版本说明，10 到 30 个汉字，不要编号、引号、冒号、句号或解释。"
	case config.AgentKindToolAgent:
		return "- 必须只输出当前调用点要求的 JSON object，不得输出解释、Markdown、代码块或额外文本。"
	case config.AgentKindConfigManager:
		return "- 没有固定 JSON 输出协议；所有资料库、叙事编排、自动化、Skills、故事记忆变更必须通过对应模块工具执行。"
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
	case config.AgentKindIDE:
		return "- 写作 Agent 必须遵守文件工具安全边界和作品工作区边界；书籍内容规则仍以 CREATOR.md 和用户本轮明确要求为准。"
	case config.AgentKindInteractiveStory:
		return strings.Join([]string{
			"- 互动叙事 Agent 禁止修改 workspace 文件，禁止输出或调用写文件、删除文件、任务计划等工具。",
			"- 互动叙事 Agent 必须遵守内置输出协议，面向故事舞台的正文只能放在 <NARRATIVE>...</NARRATIVE> 内。",
			"- 互动叙事 Agent 的篇幅必须以当前 story 的每轮目标字数为最高约束；其它内置提示、CREATOR.md 章节篇幅、导演规则或用户自定义提示中的篇幅倾向都不得要求超过该目标。",
		}, "\n")
	case config.AgentKindConfigManager:
		return strings.Join([]string{
			"- 配置管理 Agent 负责资料库、叙事编排、自动化任务、Skills、故事记忆结构、故事记忆记录和 Agents 页配置的配置、新建与维护。",
			"- Agent 模型、Prompt、工具权限、Skills 可用性、上下文压缩和 SubAgent 配置只能通过 list_agent_configs/write_agent_configs 管理；不得通过文件工具直接改配置文件。",
			"- 不负责修改端口、主题、远程访问、编辑器外观等非 Agent 页设置；这些必须由设置页完成。",
			"- 资源读取先用对应 list 工具索引，再用 read 工具读取详情；故事记忆结构例外，list_story_memory_structures 已返回完整结构。",
			"- 资源写入必须使用对应 write_* 批量工具；不得通过文件工具绕过模块校验直接改资源存储文件。",
			"- 删除、隐藏、覆盖和大范围重写必须来自用户明确指令；不确定时先说明将如何修改并请求用户确认。",
			"- 资料库只沉淀长期稳定设定；章节后的短期状态不默认写入资料库。",
		}, "\n")
	case config.AgentKindInteractiveState:
		return "- 互动记忆 Agent 必须只输出符合内置 schema 的 story_memory_patches JSON object；structure_id、op、key、字段 ID 和内容边界仍由后端校验。"
	case config.AgentKindInteractiveHotChoices:
		return "- 快捷选项 Agent 必须只输出符合内置 schema 的 JSON object；不得续写剧情或修改故事状态。"
	case config.AgentKindVersionSummary:
		return "- 版本说明 Agent 必须只输出一句版本说明，不得输出解释、编号、Markdown 或多行内容。"
	case config.AgentKindToolAgent:
		return strings.Join([]string{
			"- 工具 Agent 是 model-only 结构化任务 Agent，不得读取或写入 workspace，不得调用文件、命令、资料库、Skills 或 todo 工具。",
			"- 工具 Agent 必须只输出当前调用点要求的 JSON object，不得输出解释、Markdown、代码块或额外文本。",
		}, "\n")
	case config.AgentKindAutomation:
		return strings.Join([]string{
			"- Automation Agent 可以按任务目标自行使用已启用工具读取必要文件、资料库和项目状态。",
			"- Automation Agent 的写文件和写资料库能力必须同时满足任务写入策略与 Agent 工具权限；任一关闭都不得写入。",
			"- Automation Agent 不得无界读取完整历史、日志、大型文件或整本书；应先定位相关范围，再按需读取。",
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
