package chat

import (
	"fmt"
	"strings"
	"time"

	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/prompts"
	agentreview "denova/internal/agents/review"
	"denova/internal/agents/session"
	novaskills "denova/internal/agents/skills"
	"denova/internal/book"
	"denova/internal/book/lore"
)

type turnInputProjection struct {
	OriginalMessage    string
	Fragments          []agentcontext.Fragment
	ResumeInterruption *session.Interruption
}

// turnRuntimeEnvironment is one immutable real-world snapshot captured for a
// model turn. It must never be interpreted as fictional or in-story time.
type turnRuntimeEnvironment struct {
	CapturedAt time.Time
	Workspace  string
}

// turnContextProjectionInput keeps every turn-scoped source explicit inside
// the shared preparation module. Callers use prepareTurnContext instead.
type turnContextProjectionInput struct {
	Request             ChatRequest
	PendingInterruption *session.Interruption
	BookService         *book.Service
	Budget              agentcontext.Budget
	Environment         turnRuntimeEnvironment
	ExplicitSkills      []novaskills.Invocation
}

func newTurnRuntimeEnvironment(workspace string) turnRuntimeEnvironment {
	return turnRuntimeEnvironment{CapturedAt: time.Now(), Workspace: strings.TrimSpace(workspace)}
}

func projectTurnInput(input turnContextProjectionInput) turnInputProjection {
	resumeInterruption := input.PendingInterruption
	if !shouldResumeInterruptedRequest(input.Request.Message) {
		resumeInterruption = nil
	}
	input.PendingInterruption = resumeInterruption
	return turnInputProjection{
		OriginalMessage:    input.Request.Message,
		Fragments:          projectTurnContextFragments(input),
		ResumeInterruption: resumeInterruption,
	}
}

func projectTurnContextFragments(input turnContextProjectionInput) []agentcontext.Fragment {
	req := input.Request
	fragments := make([]agentcontext.Fragment, 0, 5+len(req.References)+len(req.LoreReferences)+len(req.Selections))
	appendFragment := func(fragment agentcontext.Fragment) {
		if strings.TrimSpace(fragment.Content) != "" {
			fragments = append(fragments, fragment)
		}
	}
	appendFragment(runtimeEnvironmentFragment(input.Environment))
	if input.PendingInterruption != nil {
		appendFragment(turnFragment(
			"runtime_interruption_resume", "runtime.interruption", "异常中断恢复上下文",
			"resume an interrupted request without replaying completed work",
			buildInterruptedResumeMessage("", input.PendingInterruption), 0,
		))
	}
	if req.PlanMode {
		appendFragment(turnFragment(
			"turn_rule_plan_mode", "turn.rule.plan_mode", "规划模式",
			"constrain this turn to collaborative planning",
			prompts.PlanModeInstruction(), 0,
		))
	}
	if skillName := strings.TrimSpace(req.WritingSkill); skillName != "" {
		appendFragment(turnFragment(
			"turn_skill_selection", "turn.skill.selection", "Writing Skill 按需加载提示",
			"identify the explicitly selected writing skill without injecting its body",
			writingSkillLoadHintContent(skillName), 0,
		))
	}
	for _, fragment := range explicitSkillFragments(input.ExplicitSkills) {
		appendFragment(fragment)
	}
	fragments = append(fragments, projectReferenceFragments(input.BookService, req.References, input.Budget)...)
	fragments = append(fragments, projectLoreReferenceFragments(input.BookService, req.LoreReferences)...)
	fragments = append(fragments, projectSelectionFragments(req.Selections)...)
	if block, ok := req.ResolvedReviewFeedback.ModelContextBlock(); ok {
		appendFragment(turnFragment(
			"workspace_review_feedback", "workspace.review.feedback", "用户明确引用的审阅意见",
			"apply trusted server-resolved review feedback to this turn",
			block, agentreview.MaxContextBytes,
		))
	}
	appendFragment(turnFragment(
		"turn_rule_context_boundary", "turn.rule.context_boundary", "上下文边界",
		"keep the current user request authoritative over historical intent",
		prompts.ContextBoundary(""), 0,
	))
	return fragments
}

func runtimeEnvironmentFragment(environment turnRuntimeEnvironment) agentcontext.Fragment {
	capturedAt := environment.CapturedAt
	if capturedAt.IsZero() {
		return agentcontext.Fragment{}
	}
	workspace := strings.ReplaceAll(strings.TrimSpace(environment.Workspace), "\\", "/")
	location := strings.TrimSpace(capturedAt.Location().String())
	zoneName, _ := capturedAt.Zone()
	if location == "" || location == "Local" {
		location = strings.TrimSpace(zoneName)
	}
	if location == "" {
		location = "UTC"
	}

	var content strings.Builder
	content.WriteString("- 上下文快照时间 / Captured at: ")
	content.WriteString(capturedAt.Format(time.RFC3339))
	content.WriteString("\n- 时区 / Time zone: ")
	content.WriteString(location)
	content.WriteString(" (UTC")
	content.WriteString(capturedAt.Format("-07:00"))
	content.WriteString(")")
	if workspace != "" {
		content.WriteString("\n- 当前工作区 / Workspace: ")
		content.WriteString(workspace)
	}
	content.WriteString("\n- 说明 / Note: 这是现实运行环境的本轮快照，不是作品或互动故事中的世界时间；作品时间线仍以作品状态为准。 / This is a turn-scoped real-world runtime snapshot, not in-story time; story chronology remains governed by workspace state.")
	fragment := turnFragment(
		"runtime_environment",
		"runtime.environment",
		"当前运行环境 / Current runtime environment",
		"provide turn-scoped real-world time and active workspace without changing the stable system prompt",
		content.String(),
		0,
	)
	fragment.Note = "source=server runtime; captured during turn context assembly; transient"
	return fragment
}

func turnFragment(id, source, title, purpose, content string, limit int) agentcontext.Fragment {
	return agentcontext.Fragment{
		ID: id, Source: source, Title: title, Purpose: purpose,
		Content: content, Placement: agentcontext.PlacementFinalUserPrefix,
		Limit: limit, Included: true,
	}
}

func projectReferenceFragments(bookService *book.Service, references []string, budget agentcontext.Budget) []agentcontext.Fragment {
	limit := budget.MaxFragmentBytes
	if limit <= 0 {
		limit = agentcontext.DefaultMaxFragmentBytes
	}
	seen := make(map[string]bool)
	fragments := make([]agentcontext.Fragment, 0, len(references))
	for index, raw := range references {
		ref := strings.TrimSpace(raw)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		content := ""
		note := "source=workspace file; explicit user reference"
		if bookService == nil {
			content = "读取失败：当前 workspace 不可用。"
			note += "; read_failed"
		} else {
			read, err := readReferencedFile(bookService, ref)
			if err != nil {
				content = "读取失败：" + err.Error()
				note += "; read_failed"
			} else {
				content = read
			}
		}
		fragment := turnFragment(
			fmt.Sprintf("workspace_file_reference_%d", index+1),
			"workspace.file.reference", "@"+ref,
			"provide the workspace file explicitly referenced by the user",
			content, limit,
		)
		fragment.Note = note
		fragments = append(fragments, fragment)
	}
	return fragments
}

func projectLoreReferenceFragments(bookService *book.Service, references []string) []agentcontext.Fragment {
	if len(references) == 0 {
		return nil
	}
	itemsByID := map[string]lore.Item{}
	loadError := error(nil)
	sourcePath := lore.ItemsRelativePath
	if bookService == nil || bookService.Workspace() == "" {
		loadError = fmt.Errorf("当前 workspace 不可用")
	} else {
		items, err := lore.NewStore(bookService.Workspace()).List()
		if err != nil {
			loadError = err
		} else {
			for _, item := range items {
				itemsByID[item.ID] = item
			}
		}
	}
	seen := make(map[string]bool)
	fragments := make([]agentcontext.Fragment, 0, len(references))
	for index, raw := range references {
		ref := strings.TrimSpace(raw)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		content := ""
		title := "@资料:" + ref
		note := "source=" + sourcePath + "; explicit structured lore reference"
		if loadError != nil {
			content = "资料库读取失败：" + loadError.Error()
			note += "; read_failed"
		} else if item, ok := itemsByID[ref]; ok {
			content = lore.ReferenceMarkdown(item)
			title = "@资料:" + item.Name
			note += "; id=" + item.ID
		} else {
			content = "读取失败：资料条目不存在"
			note += "; read_failed"
		}
		fragment := turnFragment(
			fmt.Sprintf("workspace_lore_reference_%d", index+1),
			"workspace.lore.reference", title,
			"provide the structured lore item explicitly referenced by the user",
			content, 0,
		)
		fragment.Note = note
		fragments = append(fragments, fragment)
	}
	return fragments
}

func projectSelectionFragments(selections []TextSelectionRef) []agentcontext.Fragment {
	fragments := make([]agentcontext.Fragment, 0, len(selections))
	for index, selection := range selections {
		content := strings.TrimSpace(selection.Content)
		if content == "" {
			continue
		}
		title := strings.TrimSpace(selection.FileName)
		if title == "" {
			title = "未命名选区"
		}
		if selection.StartLine > 0 || selection.EndLine > 0 {
			title = fmt.Sprintf("%s:L%d-L%d", title, selection.StartLine, selection.EndLine)
		}
		fragment := turnFragment(
			fmt.Sprintf("editor_selection_%d", index+1),
			"editor.selection", title,
			"operate on the exact editor text selected by the user",
			"```\n"+content+"\n```", 0,
		)
		fragment.Note = "source=editor selection; turn-scoped"
		fragments = append(fragments, fragment)
	}
	return fragments
}

func writingSkillLoadHintContent(skillName string) string {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return ""
	}
	return "当前创作 Agent 选中的 Writing Skill 是 `" + skillName + "`。\n\n" +
		"- 若本轮请求涉及小说正文续写、章节正文创作、正文重写或润色，且当前 Agent 已启用 `skill` 工具，同时上下文没有标记该 Skill 已由运行时加载，请先调用 `skill` 工具加载 `" + skillName + "`，读取完整 SKILL.md 后再执行。\n" +
		"- 若本轮请求是问答、分析、整理、大纲/设定讨论、配置或规划，不要加载 Writing Skill，直接按本轮请求处理。\n" +
		"- 在调用 `skill` 工具前，不要假装已经读取了该 Skill 的完整说明；写作范围仍只由用户本轮自然语言指令决定，不存在单独的 `writing_scope` 字段。"
}
