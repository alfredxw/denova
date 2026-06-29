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
	WriteMode    string
	WriteScope   string
	OutputPolicy string
	OutputPath   string
	Workspace    string
}

func BuildAutomationInstruction(cfg *config.Config, state *book.State, task AutomationTaskInstruction) string {
	return protectedSystemInstruction(cfg, config.AgentKindAutomation, editableAutomationBuiltinInstruction(cfg, state, task))
}

func editableAutomationBuiltinInstruction(cfg *config.Config, state *book.State, task AutomationTaskInstruction) string {
	workspace := task.Workspace
	if workspace == "" && cfg != nil {
		workspace = cfg.Workspace
	}
	if workspace == "" && state != nil {
		workspace = state.Workspace()
	}
	var sb strings.Builder
	sb.WriteString("你是 Nova 的自动化Agent，负责按用户配置的后台自动化任务自主完成工作。\n\n")
	sb.WriteString("## 工作方式\n\n")
	sb.WriteString("- 你可以根据任务目标自行使用已启用工具读取所需文件、资料库和项目状态，不需要用户预先选择上下文来源。\n")
	sb.WriteString("- 读取内容时要先用 `ls`、`glob`、`grep` 或资料库索引定位相关范围，再按需读取；不要无目的读取整本书或大型无关文件。\n")
	sb.WriteString("- 所有写入必须遵守本轮执行模式、写入范围和实际启用工具。没有写权限时，只输出建议和补丁计划，不要声称已经修改。\n")
	sb.WriteString("- `read_only` 模式只能输出 review、建议或方案；`confirm_write` 的首轮也是只读方案；`auto_write` 或用户确认后的写入 run 才能在写入范围内实际修改。\n")
	sb.WriteString("- 如果任务需要续写章节，先检查 `setting/outline.md`、`setting/chapter-groups/`、`progress.md`、`setting/character-states.md`、最近章节和资料库，再决定目标章节路径；写入前后要保持章节、进度和角色状态边界清晰。\n")
	sb.WriteString("- 输出最终摘要时说明你实际完成了什么、写入了哪些路径、还有哪些需要用户确认。\n\n")
	sb.WriteString("## 任务配置\n\n")
	sb.WriteString(fmt.Sprintf("- 名称：%s\n", strings.TrimSpace(task.Name)))
	sb.WriteString(fmt.Sprintf("- 工作区：%s\n", workspace))
	sb.WriteString(fmt.Sprintf("- 执行模式：%s\n", strings.TrimSpace(task.WriteMode)))
	sb.WriteString(fmt.Sprintf("- 写入范围：%s\n", strings.TrimSpace(task.WriteScope)))
	sb.WriteString(fmt.Sprintf("- 输出策略：%s\n", strings.TrimSpace(task.OutputPolicy)))
	if strings.TrimSpace(task.OutputPath) != "" {
		sb.WriteString(fmt.Sprintf("- 输出路径：%s\n", strings.TrimSpace(task.OutputPath)))
	}
	if prompt := strings.TrimSpace(task.Prompt); prompt != "" {
		sb.WriteString("\n## 用户任务\n\n")
		sb.WriteString(prompt)
	} else {
		sb.WriteString("\n## 用户任务\n\n")
		sb.WriteString("根据任务配置完成这次自动化。请先自行读取必要信息，再执行；如果任务目标不明确，只输出你需要用户补充的配置建议。")
	}
	return sb.String()
}
