package agent

import (
	"fmt"
	"strings"

	"nova/config"
	"nova/internal/book"
)

// AutomationTaskInstruction carries the user-owned automation contract into the Agent prompt.
type AutomationTaskInstruction struct {
	Name         string
	Template     string
	Prompt       string
	WritePolicy  string
	OutputPolicy string
	OutputPath   string
	Workspace    string
}

func BuildAutomationInstruction(cfg *config.Config, state *book.State, task AutomationTaskInstruction) string {
	workspace := task.Workspace
	if workspace == "" && cfg != nil {
		workspace = cfg.Workspace
	}
	if workspace == "" && state != nil {
		workspace = state.Workspace()
	}
	var sb strings.Builder
	sb.WriteString("你是 Nova 的 Automation Agent，负责按用户配置的后台自动化任务自主完成工作。\n\n")
	sb.WriteString("## 工作方式\n\n")
	sb.WriteString("- 你可以根据任务目标自行使用已启用工具读取所需文件、资料库和项目状态，不需要用户预先选择上下文来源。\n")
	sb.WriteString("- 读取内容时要先用 `ls`、`glob`、`grep` 或资料库索引定位相关范围，再按需读取；不要无目的读取整本书或大型无关文件。\n")
	sb.WriteString("- 所有写入必须遵守本轮写入策略和实际启用工具。没有写权限时，只输出建议和补丁计划，不要声称已经修改。\n")
	sb.WriteString("- 如果任务需要续写章节，先检查 `setting/outline.md`、`setting/chapter-groups/`、`progress.md`、`setting/character-states.md`、最近章节和资料库，再决定目标章节路径；写入前后要保持章节、进度和角色状态边界清晰。\n")
	sb.WriteString("- 输出最终摘要时说明你实际完成了什么、写入了哪些路径、还有哪些需要用户确认。\n\n")
	sb.WriteString("## 任务配置\n\n")
	sb.WriteString(fmt.Sprintf("- 名称：%s\n", strings.TrimSpace(task.Name)))
	sb.WriteString(fmt.Sprintf("- 模板：%s\n", strings.TrimSpace(task.Template)))
	sb.WriteString(fmt.Sprintf("- 工作区：%s\n", workspace))
	sb.WriteString(fmt.Sprintf("- 写入策略：%s\n", strings.TrimSpace(task.WritePolicy)))
	sb.WriteString(fmt.Sprintf("- 输出策略：%s\n", strings.TrimSpace(task.OutputPolicy)))
	if strings.TrimSpace(task.OutputPath) != "" {
		sb.WriteString(fmt.Sprintf("- 输出路径：%s\n", strings.TrimSpace(task.OutputPath)))
	}
	if prompt := strings.TrimSpace(task.Prompt); prompt != "" {
		sb.WriteString("\n## 用户任务\n\n")
		sb.WriteString(prompt)
	} else {
		sb.WriteString("\n## 用户任务\n\n")
		sb.WriteString(defaultAutomationTaskPrompt(task.Template))
	}
	return protectedSystemInstruction(cfg, config.AgentKindAutomation, sb.String())
}

func defaultAutomationTaskPrompt(template string) string {
	switch template {
	case "memory_consolidation":
		return "整理最近创作和互动信息。请自行读取必要的章节、资料库、进度和互动状态，提炼长期稳定记忆、待确认记忆和不应沉淀的短期噪音；有资料库写入权限时可更新资料库，否则输出建议。"
	case "review":
		return "Review 当前作品中最需要检查的内容。请自行定位相关章节、设定和资料库，检查结构、连续性、设定一致性和语言问题，并按严重程度输出建议；有文件写入权限时可把报告写到配置的输出路径。"
	case "continue_writing":
		return "续写下一段或下一章。请自行读取大纲、章节组细纲、进度、角色状态、资料库和最近章节，确定目标章节路径并写入正文；完成后按需同步 progress.md 和 setting/character-states.md。"
	default:
		return "根据任务名称和当前工作区内容完成用户配置的自动化任务。请先自行读取必要信息，再执行。"
	}
}
