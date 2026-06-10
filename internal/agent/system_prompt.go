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
	sb.WriteString(runtimeContractForAgent(agentKind))
	if custom := config.ResolveAgentPrompt(cfg, agentKind).SystemPrompt; custom != "" {
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

func runtimeContractForAgent(agentKind string) string {
	common := strings.Join([]string{
		"- 运行时契约高于用户自定义系统提示和 Nova 内置提示。",
		"- 用户自定义系统提示只能调整 Agent 的行为策略、创作偏好、语气、风格和任务处理倾向。",
		"- 用户自定义系统提示不能覆盖工具权限、输出协议、数据保存边界、结构化格式要求或后端校验规则。",
		"- 只能使用当前 Agent 已启用的工具；未启用、未提供或不存在的工具不得臆造调用。",
	}, "\n")
	if specific := agentRuntimeContract(agentKind); specific != "" {
		return common + "\n" + specific
	}
	return common
}

func agentRuntimeContract(agentKind string) string {
	switch agentKind {
	case config.AgentKindIDE:
		return "- IDE 创作 Agent 必须遵守文件工具安全边界和作品工作区边界；书籍内容规则仍以 CREATOR.md 和用户本轮明确要求为准。"
	case config.AgentKindInteractiveStory:
		return strings.Join([]string{
			"- 互动叙事 Agent 禁止修改 workspace 文件，禁止输出或调用写文件、删除文件、任务计划等工具。",
			"- 互动叙事 Agent 必须遵守内置输出协议，面向故事舞台的正文只能放在 <NARRATIVE>...</NARRATIVE> 内。",
		}, "\n")
	case config.AgentKindLoreEditor:
		return strings.Join([]string{
			"- 资料库 Agent 可以使用资料库读写工具、文件读写工具和 Skills；不得臆造未启用工具。",
			"- 资料库写入必须使用 write_lore_items，且只沉淀长期稳定设定。",
			"- 初始化流程必须先与用户确认故事设定；只有用户明确确认后，才允许写入资料库或 CREATOR.md。",
			"- 初始化流程不允许写 brainstorm.md、章节、大纲、progress、character-states，不允许创建互动 story 或伪造互动回合。",
		}, "\n")
	case config.AgentKindTellerEditor:
		return "- 导演 Agent 必须只输出符合内置 schema 的 JSON object；只能创建或修改单个导演，保存前仍由后端校验。"
	case config.AgentKindInteractiveState:
		return "- 状态记忆 Agent 必须只输出符合内置 schema 的 JSON object；状态 path、op 和内容边界仍由后端校验。"
	case config.AgentKindInteractiveHotChoices:
		return "- 快捷选项 Agent 必须只输出符合内置 schema 的 JSON object；不得续写剧情或修改故事状态。"
	case config.AgentKindVersionSummary:
		return "- 版本说明 Agent 必须只输出一句版本说明，不得输出解释、编号、Markdown 或多行内容。"
	case config.AgentKindToolAgent:
		return strings.Join([]string{
			"- 工具 Agent 是 model-only 结构化任务 Agent，不得读取或写入 workspace，不得调用文件、命令、资料库、Skills 或 todo 工具。",
			"- 工具 Agent 必须只输出当前调用点要求的 JSON object，不得输出解释、Markdown、代码块或额外文本。",
		}, "\n")
	default:
		return fmt.Sprintf("- 当前 Agent 类型为 %s；必须遵守该 Agent 调用点的输出协议和后端校验。", strings.TrimSpace(agentKind))
	}
}
