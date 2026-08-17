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
			"runtime_interruption_resume", "runtime.interruption", "Interrupted-turn Recovery Context",
			"resume an interrupted request without replaying completed work",
			buildInterruptedResumeMessage("", input.PendingInterruption), 0,
		))
	}
	if req.PlanMode {
		appendFragment(turnFragment(
			"turn_rule_plan_mode", "turn.rule.plan_mode", "Plan Mode",
			"constrain this turn to collaborative planning",
			prompts.PlanModeInstruction(), 0,
		))
	}
	if skillName := strings.TrimSpace(req.WritingSkill); skillName != "" {
		appendFragment(turnFragment(
			"turn_skill_selection", "turn.skill.selection", "On-demand Writing Skill Loading",
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
			"workspace_review_feedback", "workspace.review.feedback", "Review Feedback Explicitly Referenced by the User",
			"apply trusted server-resolved review feedback to this turn",
			block, agentreview.MaxContextBytes,
		))
	}
	appendFragment(turnFragment(
		"turn_rule_context_boundary", "turn.rule.context_boundary", "Context Boundary",
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
	content.WriteString("- Captured at: ")
	content.WriteString(capturedAt.Format(time.RFC3339))
	content.WriteString("\n- Time zone: ")
	content.WriteString(location)
	content.WriteString(" (UTC")
	content.WriteString(capturedAt.Format("-07:00"))
	content.WriteString(")")
	if workspace != "" {
		content.WriteString("\n- Workspace: ")
		content.WriteString(workspace)
	}
	content.WriteString("\n- Note: this is a turn-scoped real-world runtime snapshot, not in-story time. Story chronology remains governed by workspace state.")
	fragment := turnFragment(
		"runtime_environment",
		"runtime.environment",
		"Current Runtime Environment",
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
			content = "Read failed: the current workspace is unavailable."
			note += "; read_failed"
		} else {
			read, err := readReferencedFile(bookService, ref)
			if err != nil {
				content = "Read failed: " + err.Error()
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
		loadError = fmt.Errorf("the current workspace is unavailable")
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
		title := "@Lore:" + ref
		note := "source=" + sourcePath + "; explicit structured lore reference"
		if loadError != nil {
			content = "Lore read failed: " + loadError.Error()
			note += "; read_failed"
		} else if item, ok := itemsByID[ref]; ok {
			content = lore.ReferenceMarkdown(item)
			title = "@Lore:" + item.Name
			note += "; id=" + item.ID
		} else {
			content = "Read failed: the lore item does not exist."
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
			title = "Untitled Selection"
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
	return "The Writing Skill selected for the current creative Agent is `" + skillName + "`.\n\n" +
		"- If this request involves continuing novel prose, creating chapter prose, rewriting prose, or polishing prose, and the current Agent has the `skill` tool enabled while context does not mark the Skill as runtime-loaded, call `skill` to load `" + skillName + "` and read its complete SKILL.md before proceeding.\n" +
		"- For Q&A, analysis, organization, outline or setting discussion, configuration, or planning, do not load the Writing Skill; handle the request directly.\n" +
		"- Before calling `skill`, do not claim to have read its complete instructions. The writing scope is determined only by the user's natural-language request for this turn; there is no separate `writing_scope` field."
}
