package prompts

import (
	"fmt"
	"strings"
)

// PlanMode 在用户消息前追加规划模式指令，让模型先提问和形成可审阅计划。
func PlanMode(message string) string {
	return `[Plan Mode / 规划模式] 请先协作制定计划，不要直接执行。

要求：
1. 先分析用户需求、当前上下文和风险；需要了解现状时，可优先使用只读方式收集信息。
2. 在 Plan Mode 中不要主动执行会改变作品、代码、配置、资料库或工作区状态的操作；等用户确认计划后再进入执行。
3. 如果需求、范围、交互、数据结构或实现取舍存在高影响不确定性，先输出结构化问题卡，不要把不确定点偷偷写成假设。
4. 一旦决定输出问题卡或最终方案卡，立即从对应开始标签写起，不要在标签前输出解释、寒暄、思路铺垫或 Markdown 正文。
5. 问题卡必须只输出一个 <plan_questions>...</plan_questions> 块，块内是 JSON；如果有多个关键确认点，可一次给出多个问题，前端会像 Codex Plan Mode 一样逐个向用户确认；问题和选项保持精简，避免让用户等待冗长说明。推荐格式：
{
  "questions": [
    {
      "id": "scope",
      "type": "single",
      "question": "要解决的关键选择是什么？",
      "description": "可选，解释这个问题为什么影响方案。",
      "options": [
        {"id": "recommended", "label": "推荐方案", "description": "推荐理由", "recommended": true},
        {"id": "alternative", "label": "备选方案", "description": "取舍说明"}
      ],
      "allow_custom": true
    }
  ]
}
6. type 只使用 "single" 或 "multi"。每个问题至少给 2 个有效选项；能推荐时用 recommended=true 标记推荐项。
7. 收到用户提交的整组回答后继续停留在 Plan Mode；如果仍有关键不确定性，可继续输出下一组问题。
8. 当方案已经足够明确时，输出最终方案卡：只输出一个 <proposed_plan>...</proposed_plan> 块，块内使用清晰 Markdown；不要输出测试计划或假设小节。使用这个轻量模板，并保留必要空行：
# 计划标题

## Summary
- **目标**：一句话说明要达成什么。
- **结果**：一句话说明确认后会进入什么执行方向。

## Key Changes
- **关键方向**：用短 bullet 分组说明会怎么做。
- **取舍**：只列影响执行的重点取舍，不写长段落。
9. 不要在 <proposed_plan> 外输出执行结果。

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

// StyleReference 表示一个可由 Agent 按路径读取的共享文风参考。
type StyleReference struct {
	Name        string
	Description string
	Path        string
	DisplayPath string
	Missing     bool
	Error       string
}

// StyleRule 表示全局或「场景 → 文风参考」映射。
type StyleRule struct {
	Global          bool
	Scene           string
	StyleReferences []StyleReference
	StyleContents   []string
}

// StyleRulesInstruction 把导演的文风参考索引拼成稳定 system prompt 片段。
func StyleRulesInstruction(rules []StyleRule) string {
	var sb strings.Builder
	sb.WriteString("## 文风参考\n\n")
	sb.WriteString("当前叙事风格配置了以下文风参考索引。文风参考是所有叙事风格共享的 Markdown 文件，统一存放在 `.denova/styles/`；索引只提供 name、description、path，不包含全文。\n")
	wrote := 0
	for _, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if !rule.Global && scene == "" {
			continue
		}
		if len(rule.StyleReferences) == 0 && len(rule.StyleContents) == 0 {
			continue
		}
		wrote++
		if rule.Global {
			fmt.Fprintf(&sb, "%d. 全局文风参考：所有正文生成默认生效\n", wrote)
		} else {
			fmt.Fprintf(&sb, "%d. 场景：%s\n", wrote, scene)
		}
		for _, ref := range rule.StyleReferences {
			name := strings.TrimSpace(ref.Name)
			if name == "" {
				name = strings.TrimSpace(ref.DisplayPath)
			}
			path := strings.TrimSpace(ref.Path)
			if path == "" {
				path = strings.TrimSpace(ref.DisplayPath)
			}
			if name == "" || path == "" {
				continue
			}
			fmt.Fprintf(&sb, "   - name: %s\n", name)
			if desc := strings.TrimSpace(ref.Description); desc != "" {
				fmt.Fprintf(&sb, "     description: %s\n", desc)
			}
			if display := strings.TrimSpace(ref.DisplayPath); display != "" {
				fmt.Fprintf(&sb, "     display_path: %s\n", display)
			}
			fmt.Fprintf(&sb, "     path: %s\n", path)
			if ref.Missing {
				fmt.Fprintf(&sb, "     status: missing")
				if err := strings.TrimSpace(ref.Error); err != "" {
					fmt.Fprintf(&sb, " (%s)", err)
				}
				sb.WriteString("\n")
			}
		}
		for j, content := range rule.StyleContents {
			content = strings.TrimSpace(content)
			if content == "" {
				continue
			}
			fmt.Fprintf(&sb, "   旧版内联风格内容 %d：\n```markdown\n%s\n```\n", j+1, content)
		}
	}
	if wrote == 0 {
		return ""
	}
	sb.WriteString("\n触发规则：仅当本轮要执行『章节正文的创作 / 续写 / 重写』或『互动故事下一回合正文生成』时使用文风参考。全局文风参考默认适用于所有正文生成；分场景文风参考需要先根据当前章节内容、互动场景或本轮 # 场景选择出最贴近的场景。若需要使用某条文风参考 path，必须先用 read_file 读取真正需要的参考文件，再把其作为文风、节奏、叙述方式、句式和氛围参考；不要照搬其中的人物、情节或设定。\n")
	sb.WriteString("若本轮属于脑暴、大纲、设定、问答、规划等非正文生成场景，请完全忽略以上参考；若没有场景明显匹配，也不必强行选择分场景参考。\n")
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
