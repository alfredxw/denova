package prompts

import (
	"fmt"
	"strings"

	"denova/config"
)

func protectedSystemInstruction(cfg *config.Config, agentKind, builtIn string) string {
	composition, err := composeProtectedSystemInstruction(cfg, agentKind, agentKind, "", []SystemPromptFragment{{
		ID: "builtin_base", Source: "Denova built-in", Title: "Denova built-in system instruction",
		Purpose: "define the built-in Agent behavior and workflow", Content: builtIn, Required: true,
		Overflow: SystemPromptOverflowReject,
	}})
	if err != nil {
		return ""
	}
	return composition.Instruction()
}

func composeProtectedSystemInstruction(cfg *config.Config, agentKind, mode, workspace string, builtIn []SystemPromptFragment) (SystemPromptComposition, error) {
	fragments := protectedSystemPromptFragments(agentKind)
	firstIncluded := -1
	for i := range builtIn {
		if strings.TrimSpace(builtIn[i].Content) != "" || builtIn[i].Required {
			firstIncluded = i
			break
		}
	}
	if firstIncluded >= 0 {
		builtIn[firstIncluded].Prefix = "\n\n---\n\n# Denova Built-in System Instruction\n\n" + builtIn[firstIncluded].Prefix
	}
	fragments = append(fragments, builtIn...)
	return composeSystemPrompt(cfg, agentKind, mode, workspace, fragments)
}

func ComposeBuiltinSystemInstruction(cfg *config.Config, agentKind, mode, workspace, id, title, purpose, builtIn string) (SystemPromptComposition, error) {
	return composeProtectedSystemInstruction(cfg, agentKind, mode, workspace, []SystemPromptFragment{{
		ID: id, Source: "Denova built-in", Title: title, Purpose: purpose, Content: builtIn,
		Required: true, Overflow: SystemPromptOverflowReject,
	}})
}

func protectedSystemPromptFragments(agentKind string) []SystemPromptFragment {
	return []SystemPromptFragment{{
		ID: "runtime_contract", Source: "Denova runtime", Title: "Runtime contract",
		Purpose: "define shared runtime behavior",
		Content: runtimeContractForAgent(agentKind), Prefix: "# Denova Runtime Contract\n\n",
		Required: true, Overflow: SystemPromptOverflowReject,
	}, {
		ID: "output_protocol", Source: "Denova runtime", Title: "Output protocol",
		Purpose: "define the Agent output protocol",
		Content: outputProtocolForAgent(agentKind), Prefix: "\n\n## Output Protocol\n\n",
		Required: true, Overflow: SystemPromptOverflowReject,
	}}
}

func runtimeContractForAgent(agentKind string) string {
	common := strings.Join([]string{
		"- Follow the current user request, applicable project instructions, and this Agent's workflow.",
		"- Use the available tools and their schemas; backend receipts determine which operations were accepted.",
		"- If a tool call or permission is denied, adapt to the result instead of repeating the same request unchanged.",
		"- Explicitly named /<skill-name> instructions may already be loaded in context. Use the skill tool only to load an additional Skill selected by description.",
	}, "\n")
	sections := []string{common}
	if config.IsSubAgentParentKind(agentKind) {
		sections = append(sections, subAgentDelegationContract())
	}
	if specific := agentRuntimeContract(agentKind); specific != "" {
		sections = append(sections, specific)
	}
	// Retain the universal contract for model entry points without turn context.
	// Turn-aware conversations repeat it beside the current request for recency.
	sections = append(sections, currentInputLanguageContract())
	return strings.Join(sections, "\n\n")
}

func currentInputLanguageContract() string {
	return strings.Join([]string{
		"## Language Alignment",
		"- Use the same language as the user's current input for internal reasoning, thinking summaries, streamed thinking, intermediate progress updates, and all user-facing output.",
		"- Preserve fixed output protocols, JSON keys, code, paths, quoted source text, and any language explicitly required by the user or task.",
	}, "\n")
}

func subAgentDelegationContract() string {
	return strings.Join([]string{
		"- Use the task tool only when the current user explicitly requests delegation or multi-Agent work, or when a loaded Skill explicitly requires delegation. Otherwise do the work yourself, even when delegation could be helpful.",
		"- Do not delegate merely to parallelize, review, research, or save time.",
		"- Starting a task returns immediately. Start independent tasks together, continue useful local work, and call task_wait with all relevant TaskRefs only when their results become a dependency.",
		"- User steering can interrupt task_wait without aborting child tasks. Resume waiting only when their results are still needed.",
		"- Give the SubAgent a self-contained goal, constraints, relevant paths or resource IDs, expected output, and write scope. Pass references instead of copying content it can read itself.",
		"- Verify the returned result before reporting it to the user.",
	}, "\n")
}

func outputProtocolForAgent(agentKind string) string {
	switch agentKind {
	case config.AgentKindGeneral:
		return "- General Agent has no fixed JSON output protocol. Perform all file changes through enabled tools. Mutations must stay within the current Project root; explicitly requested external local sources may be read through read, glob, or grep subject to permission."
	case config.AgentKindHarness:
		return "- Harness Agent has no fixed JSON output protocol. Report the evidence inspected, files actually changed, current validation result, and any rejected Harness contribution."
	case config.AgentKindInteractiveStory:
		return strings.Join([]string{
			"- Output only the story prose that can be shown on the story stage for this turn.",
			"- Write only scenes, actions, dialogue, and consequences. Do not output plans, explanations, tool instructions, Markdown headings, XML wrappers, hidden state blocks, shortcut-choice blocks, or JSON.",
		}, "\n")
	case config.AgentKindVersionSummary:
		return "- Output exactly one Chinese release-summary sentence containing 10 to 30 Han characters. Do not include numbering, quotation marks, colons, terminal punctuation, or explanation."
	case config.AgentKindToolAgent:
		return "- Output only the JSON object required by the current call site. Do not output explanations, Markdown, code fences, or extra text."
	case config.AgentKindImage:
		return "- Call the image-generation tool to produce the image. The final response should briefly report the result without unrelated explanation or prose modifications."
	case config.AgentKindAutomation:
		return "- The final output must report what was actually completed, written paths, and items awaiting user confirmation. Writes remain subject to the task write policy and tool permissions."
	case config.AgentKindIDE:
		return "- Writing Agent has no fixed JSON output protocol. Perform all file changes through enabled tools. Book mutations must stay within the current Project; explicitly identified external references may be read subject to permission."
	default:
		return "- Follow the output protocol and backend validation for the current Agent call site."
	}
}

func agentRuntimeContract(agentKind string) string {
	switch agentKind {
	case config.AgentKindGeneral:
		return "- Keep all mutations within the current Project root. Discovery respects .gitignore; read, glob, and grep may inspect explicitly requested external local sources subject to permission. Shell commands retain native semantics. Modify .gitignore only when the user explicitly requests it."
	case config.AgentKindHarness:
		return strings.Join([]string{
			"- The current Project root is the live Harness State directory. Inspect trajectory resources through read, and modify State with ordinary workspace file or shell tools.",
			"- Writes are intentionally unrestricted by Harness schema validation. Consumers validate the complete snapshot and reject the entire user contribution when it is invalid, so inspect harness://state/current after edits and repair diagnostics before finishing.",
			"- Never write Project-private content, complete trajectories, temporary tasks, user secrets, or model reasoning into global State.",
			"- Read evidence first and distinguish generalizable preferences from one-off signals. Remain a no-op when evidence is insufficient, the improvement is unclear, or there is no valid Diff.",
			"- Make only the smallest cohesive change. Existing Agent runs keep their current definition; valid State is consumed when later Agent definitions are built.",
		}, "\n")
	case config.AgentKindIDE:
		return "- Writing Agent must respect file-tool safety and book-workspace boundaries. CREATOR.md and the user's explicit current request remain authoritative for book content."
	case config.AgentKindInteractiveStory:
		return strings.Join([]string{
			"- Game Mode is read-only for workspace files. Use only listed read-only references; persist prose, state, and choices through the Game Mode submission tools.",
			"- The current story's per-turn target character count is the highest length constraint. Other built-in prompts, CREATOR.md chapter-length rules, game-preset guidance, and custom prompts must not require exceeding it.",
		}, "\n")
	case config.AgentKindVersionSummary:
		return "- Version Summary Agent must output exactly one release-summary sentence, with no explanation, numbering, Markdown, or multiple lines."
	case config.AgentKindToolAgent:
		return strings.Join([]string{
			"- Tool Agent is a model-only structured-task Agent. It must not read or write the workspace or call file, command, lore, Skills, or todo tools.",
			"- Tool Agent must output only the JSON object required by the current call site, without explanations, Markdown, code fences, or extra text.",
		}, "\n")
	case config.AgentKindImage:
		return strings.Join([]string{
			"- Image Agent generates images only from the caller-provided purpose, source_context, System Prompt, and Skill.",
			"- Image Agent may write image files and metadata only through image-generation tools. It must not modify prose, lore, configuration, versions, or story state.",
			"- Image Agent must not read unbounded history, logs, large files, or complete conversations. Never invent caller-omitted facts as established story events.",
		}, "\n")
	case config.AgentKindAutomation:
		return strings.Join([]string{
			"- Automation Agent may use enabled tools to read files, lore, and Project state necessary for the task objective.",
			"- File and lore writes require both the task write policy and Agent tool permission. If either disallows writing, do not write.",
			"- Automation Agent must not read complete history, logs, large files, or an entire book without bounds. Locate the relevant scope first, then read only what is needed.",
		}, "\n")
	default:
		return fmt.Sprintf("- The current Agent kind is %s. Follow the output protocol and backend validation for its call site.", strings.TrimSpace(agentKind))
	}
}
