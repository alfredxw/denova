package prompts

import (
	"fmt"
	"strings"
)

// PlanMode 在用户消息前追加规划模式指令，允许读取文件但禁止写操作，只输出结构化计划。
func PlanMode(message string) string {
	return `[规划模式] 请你先制定计划，不要执行任何写操作。

要求：
1. 你可以使用 read_file 工具读取文件内容来了解当前状态
2. 分析用户的需求，列出需要完成的步骤
3. 说明每一步涉及哪些文件、要做什么操作
4. 如果有多种方案，列出利弊供用户选择
5. 禁止使用 write_file、edit_file、delete_file 等任何写操作工具
6. 等待用户确认或调整计划后再执行

用户需求：
` + message
}

// ContextBoundary 在用户消息前追加上下文边界说明，强调当前请求才是“这次要做什么”，
// 工作区/已确认小说状态是“背景是什么”，历史对话只能用于辅助理解。
func ContextBoundary(message string) string {
	return `[上下文边界]
- 当前用户请求是“这次要做什么”，请只按本轮请求、显式 @ 引用、# 场景风格选择和编辑器选区行动。
- 工作区与已确认的小说状态只用于判断“背景是什么”，不能替代本轮明确请求。
- 历史对话只能辅助理解上下文，不要把上一轮的待办、工具意图或未完成动作当成本轮指令，除非用户在本轮明确延续。
- 如果当前请求与历史看起来无关或冲突，以当前请求为准，不要继续执行上一轮的工具调用或修改。

本轮请求：
` + message
}

// InterruptedResume 描述上一轮异常中断的现场。
type InterruptedResume struct {
	UserMessage      string
	AssistantContent string
	Reason           string
}

// ResumeFromInterruption 在用户输入“继续”等指令时，把上一轮中断现场拼成本轮提示。
func ResumeFromInterruption(current string, prev InterruptedResume) string {
	var sb strings.Builder
	sb.WriteString("[异常中断恢复]\n")
	sb.WriteString("用户当前要求继续。请从上一轮异常中断的位置继续，不要重做已经完成且已经写入文件的工作。\n")
	sb.WriteString("如果上一轮已有部分助手输出，请把它作为已完成内容的上下文，继续完成原始请求。\n\n")
	sb.WriteString("上一轮原始请求：\n")
	sb.WriteString(prev.UserMessage)
	if prev.AssistantContent != "" {
		sb.WriteString("\n\n上一轮中断前已生成的助手内容：\n")
		sb.WriteString(prev.AssistantContent)
	}
	if prev.Reason != "" {
		sb.WriteString("\n\n上一轮中断原因：\n")
		sb.WriteString(prev.Reason)
	}
	sb.WriteString("\n\n本轮用户继续请求：\n")
	sb.WriteString(current)
	return sb.String()
}

// StyleRule 表示「场景 → 风格内容」映射。
type StyleRule struct {
	Scene         string
	StyleContents []string
}

// StyleRulesInstruction 把导演的「场景 → 风格内容」映射拼成稳定 system prompt 片段。
func StyleRulesInstruction(rules []StyleRule) string {
	var sb strings.Builder
	sb.WriteString("## 场景化风格规则\n\n")
	sb.WriteString("当前叙事方案配置了以下「场景 → 风格内容」规则：\n")
	for i, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if scene == "" || len(rule.StyleContents) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "%d. 场景：%s\n", i+1, scene)
		for j, content := range rule.StyleContents {
			content = strings.TrimSpace(content)
			if content == "" {
				continue
			}
			fmt.Fprintf(&sb, "   风格内容 %d：\n```markdown\n%s\n```\n", j+1, content)
		}
	}
	sb.WriteString("\n触发规则：仅当本轮要执行『章节正文的创作 / 续写 / 重写』或『互动故事下一回合正文生成』时，先根据当前章节内容或互动场景选出最贴近的场景，把对应风格内容作为文风、节奏、叙述方式、句式和氛围参考；不要照搬其中的人物、情节或设定。\n")
	sb.WriteString("若本轮属于脑暴、大纲、设定、问答、规划等非正文生成场景，请完全忽略以上规则；若没有场景明显匹配，也不必强行选择。\n")
	return sb.String()
}

// ReferenceHeader 在用户 @ 引用文件块前追加的固定标题。
const ReferenceHeader = "\n\n---\n以下是用户引用的文件：\n"

// ReferenceOverflowHint 引用内容总量超限时，提示后续文件未读取。
const ReferenceOverflowHint = "引用内容总量已超过限制，后续文件未读取。\n"

// SelectionHeader 在编辑器选中片段块前追加的固定标题。
const SelectionHeader = "\n\n---\n以下是用户在编辑器中选中的文本片段，请针对这些内容进行操作：\n"

// UnknownToolMessage LLM 调用了不存在工具时回灌给模型的可读错误。
func UnknownToolMessage(name string) string {
	return fmt.Sprintf(
		"[tool error] 工具 %q 不存在或当前不可用。请基于该错误自我分析：\n"+
			"1) 如果是工具名拼写错误（例如 write_todo 应为 write_todos），请在下一步使用正确的工具名重新调用；\n"+
			"2) 如果该能力无法通过现有工具完成，请改用其他可用工具或直接以文本回复用户；\n"+
			"3) 不要重复调用同一个不存在的工具。",
		name,
	)
}
