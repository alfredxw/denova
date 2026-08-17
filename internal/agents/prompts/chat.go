package prompts

import (
	"fmt"
	"strings"
)

// PlanModeInstruction constrains one turn to read-only research, durable Ask
// clarification, and a reviewable final proposal.
func PlanModeInstruction() string {
	return `[Plan Mode] Collaborate on a plan before execution.

Requirements:
1. Analyze the user's request, current context, and risks first. Use only currently available read-only tools when additional facts are needed.
2. Plan Mode permits only reading and in-session planning operations. Do not call tools that change the work, code, configuration, lore, workspace, host environment, or external systems. Execution begins only after the user approves the plan.
3. If the requirements, scope, interaction, data structure, or implementation tradeoffs contain a high-impact uncertainty and the ask tool is available, call ask immediately instead of hiding the uncertainty as an assumption. Do not first output an explanation, greeting, preamble, or Markdown body.
4. Each ask call should collect only the one to three most important current decisions. Multiple-choice questions should offer two or three concise options and mark a recommendation when appropriate; use a free-text question when an open answer is required. Use the user's current language for questions, options, and descriptions.
5. After ask returns, remain in the same Plan Mode run and refine the proposal. Call ask again if a critical uncertainty remains. If the user cancels or ask is unavailable, choose the safest, narrowest direction supported by known facts; do not fabricate a tool call.
6. Do not output <plan_questions>, question JSON, or another custom question protocol. Use ask for all interactive clarification.
7. Once the proposal is sufficiently clear, begin immediately with <proposed_plan> and output exactly one <proposed_plan>...</proposed_plan> block containing clear Markdown. Do not include Test Plan or Assumptions sections. Use this lightweight template and preserve the necessary blank lines:
# Plan title

## Summary
- **Goal**: One sentence describing the intended outcome.
- **Result**: One sentence describing the execution direction after approval.

## Key Changes
- **Direction**: Group the approach into short bullets.
- **Tradeoffs**: Include only material execution tradeoffs; avoid long paragraphs.
8. Do not output execution results outside <proposed_plan>.`
}

// ContextBoundary 在用户消息前追加上下文边界说明，强调当前请求才是“这次要做什么”，
// 工作区/已确认小说状态是“背景是什么”，历史对话只能用于辅助理解。
func ContextBoundary(message string) string {
	return `[Context Boundary]
- The current user request defines what to do now. Act only on this turn's request, explicit @ references, # scene-style selection, and editor selection.
- Workspace state and confirmed novel state provide background only; they do not replace the explicit current request.
- Conversation history may help interpret context, but prior todos, tool intentions, or unfinished actions are not current instructions unless the user explicitly continues them in this turn.
- If the current request is unrelated to or conflicts with history, follow the current request and do not continue prior tool calls or modifications.
- If this turn explicitly requires a tool, actually call it in this turn and answer from the current tool result. Without a current result, do not claim to have called, read, searched, verified, or modified anything.

Current request:
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
	sb.WriteString("[Interrupted Run Recovery]\n")
	sb.WriteString("The user asked to continue. Resume from the previous interruption without redoing work that was already completed and written to files.\n")
	sb.WriteString("Treat any partial assistant output from the previous run as completed context and continue the original request.\n\n")
	sb.WriteString("Previous original request:\n")
	sb.WriteString(prev.UserMessage)
	if prev.AssistantContent != "" {
		sb.WriteString("\n\nAssistant content generated before the interruption:\n")
		sb.WriteString(prev.AssistantContent)
	}
	if prev.Reason != "" {
		sb.WriteString("\n\nInterruption reason:\n")
		sb.WriteString(prev.Reason)
	}
	sb.WriteString("\n\nCurrent continuation request:\n")
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
	sb.WriteString("## Prose Style References\n\n")
	sb.WriteString("The current narrative style provides the following prose-style reference index. References are shared Markdown files stored under `.denova/styles/`; the index provides only name, description, and path, not full content.\n")
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
			fmt.Fprintf(&sb, "%d. Global prose-style references: apply to all prose generation by default\n", wrote)
		} else {
			fmt.Fprintf(&sb, "%d. Scene: %s\n", wrote, scene)
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
			fmt.Fprintf(&sb, "   Legacy inline style content %d:\n```markdown\n%s\n```\n", j+1, content)
		}
	}
	if wrote == 0 {
		return ""
	}
	sb.WriteString("\nTrigger rule: use prose-style references only for chapter prose creation, continuation, or rewriting, and for generating the next interactive-story turn. Global references apply to all prose generation by default. Before writing an interactive-story turn, use read to load every global reference path listed here. Choose scene-specific references only when they closely match the current chapter, interactive scene, or this turn's # scene selection; do not force a match. Before using a scene-specific path, read the actual file and use it only as guidance for voice, pacing, narration, sentence structure, and atmosphere. Do not copy its characters, plot, or setting.\n")
	sb.WriteString("Ignore these references for brainstorming, outlines, setting work, Q&A, planning, and other non-prose tasks. If no scene clearly matches, do not select a scene-specific reference.\n")
	return sb.String()
}

// ReferenceHeader 在用户 @ 引用文件块前追加的固定标题。
const ReferenceHeader = "\n\n---\nFiles referenced by the user:\n"

// ReferenceOverflowHint 引用内容总量超限时，提示后续文件未读取。
const ReferenceOverflowHint = "The total referenced content exceeded the limit; subsequent files were not read.\n"

// SelectionHeader 在编辑器选中片段块前追加的固定标题。
const SelectionHeader = "\n\n---\nText selected by the user in the editor; operate on this content:\n"

// UnknownToolMessage LLM 调用了不存在工具时回灌给模型的可读错误。
func UnknownToolMessage(name string) string {
	return fmt.Sprintf(
		"[tool error] Tool %q does not exist or is currently unavailable. Diagnose the error:\n"+
			"1) If the tool name is misspelled (for example, tod instead of todo), call the correct tool next.\n"+
			"2) If available tools cannot provide the capability, use another supported approach or answer the user in text.\n"+
			"3) Do not call the same nonexistent tool again.",
		name,
	)
}
