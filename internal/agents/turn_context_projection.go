package agents

import (
	"fmt"
	"strings"

	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/prompts"
	"denova/internal/workspacepath"
)

type turnInputProjection struct {
	OriginalMessage    string
	Fragments          []agentcontext.Fragment
	ResumeInterruption *session.Interruption
}

func projectTurnInput(req ChatRequest, pending *session.Interruption, bookService *book.Service, budget agentcontext.Budget) turnInputProjection {
	resumeInterruption := pending
	if !shouldResumeInterruptedRequest(req.Message) {
		resumeInterruption = nil
	}
	return turnInputProjection{
		OriginalMessage:    req.Message,
		Fragments:          projectTurnContextFragments(req, resumeInterruption, bookService, budget),
		ResumeInterruption: resumeInterruption,
	}
}

func projectTurnContextFragments(req ChatRequest, pending *session.Interruption, bookService *book.Service, budget agentcontext.Budget) []agentcontext.Fragment {
	fragments := make([]agentcontext.Fragment, 0, 4+len(req.References)+len(req.LoreReferences)+len(req.Selections))
	appendFragment := func(fragment agentcontext.Fragment) {
		if strings.TrimSpace(fragment.Content) != "" {
			fragments = append(fragments, fragment)
		}
	}
	if pending != nil {
		appendFragment(turnFragment(
			"runtime_interruption_resume", "runtime.interruption", "异常中断恢复上下文",
			"resume an interrupted request without replaying completed work",
			buildInterruptedResumeMessage("", pending), 0,
		))
	}
	if req.PlanMode {
		appendFragment(turnFragment(
			"turn_rule_plan_mode", "turn.rule.plan_mode", "规划模式",
			"constrain this turn to collaborative planning",
			prompts.PlanMode(""), 0,
		))
	}
	if skillName := strings.TrimSpace(req.WritingSkill); skillName != "" {
		appendFragment(turnFragment(
			"turn_skill_selection", "turn.skill.selection", "Writing Skill 按需加载提示",
			"identify the explicitly selected writing skill without injecting its body",
			writingSkillLoadHintContent(skillName), 0,
		))
	}
	fragments = append(fragments, projectReferenceFragments(bookService, req.References, budget)...)
	fragments = append(fragments, projectLoreReferenceFragments(bookService, req.LoreReferences)...)
	fragments = append(fragments, projectSelectionFragments(req.Selections)...)
	if block, ok := reviewFeedbackContextBlockFromNormalized(req.ResolvedReviewFeedback.normalized()); ok {
		appendFragment(turnFragment(
			"workspace_review_feedback", "workspace.review.feedback", "用户明确引用的审阅意见",
			"apply trusted server-resolved review feedback to this turn",
			block, MaxReviewFeedbackContextBytes,
		))
	}
	appendFragment(turnFragment(
		"turn_rule_context_boundary", "turn.rule.context_boundary", "上下文边界",
		"keep the current user request authoritative over historical intent",
		prompts.ContextBoundary(""), 0,
	))
	return fragments
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
			read, _, err := readReferencedFile(bookService, ref, limit, limit)
			if err != nil {
				content = "读取失败：" + err.Error()
				note += "; read_failed"
			} else {
				content = "```markdown\n" + read + "\n```"
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
	itemsByID := map[string]book.LoreItem{}
	loadError := error(nil)
	sourcePath := "lore/items.json"
	if bookService == nil || bookService.Workspace() == "" {
		loadError = fmt.Errorf("当前 workspace 不可用")
	} else {
		sourcePath = workspacepath.Rel(bookService.Workspace(), "lore", "items.json")
		items, err := book.NewLoreStore(bookService.Workspace()).List()
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
			content = formatLoreReference(item)
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
		"- 若本轮请求涉及小说正文续写、章节正文创作、正文重写或润色，且当前 Agent 已启用 `skill` 工具，请先调用 `skill` 工具加载 `" + skillName + "`，读取完整 SKILL.md 后再执行。\n" +
		"- 若本轮请求是问答、分析、整理、大纲/设定讨论、配置或规划，不要加载 Writing Skill，直接按本轮请求处理。\n" +
		"- 在调用 `skill` 工具前，不要假装已经读取了该 Skill 的完整说明；写作范围仍只由用户本轮自然语言指令决定，不存在单独的 `writing_scope` 字段。"
}
