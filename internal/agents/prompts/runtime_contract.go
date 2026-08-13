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
	fragments := protectedSystemPromptFragments(cfg, agentKind)
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

func protectedSystemPromptFragments(cfg *config.Config, agentKind string) []SystemPromptFragment {
	return []SystemPromptFragment{{
		ID: "runtime_contract", Source: "Denova runtime", Title: "Runtime contract",
		Purpose: "enforce non-overridable runtime and capability boundaries",
		Content: runtimeContractForAgent(cfg, agentKind), Prefix: "# Denova Runtime Contract (Non-overridable)\n\n",
		Required: true, Overflow: SystemPromptOverflowReject,
	}, {
		ID: "output_protocol", Source: "Denova runtime", Title: "Output protocol",
		Purpose: "enforce the Agent output protocol",
		Content: outputProtocolForAgent(agentKind), Prefix: "\n\n## Output Protocol (Non-overridable)\n\n",
		Required: true, Overflow: SystemPromptOverflowReject,
	}}
}

func runtimeContractForAgent(cfg *config.Config, agentKind string) string {
	common := strings.Join([]string{
		"- The runtime contract takes precedence over user-defined system prompts and Denova built-in instructions.",
		"- A user-defined system prompt may adjust behavior strategy, creative preferences, tone, style, and task-handling tendencies only.",
		"- A user-defined system prompt cannot override tool permissions, output protocols, persistence boundaries, structured-format requirements, or backend validation.",
		"- Use only tools enabled for the current Agent. Never fabricate calls to disabled, unavailable, or nonexistent tools.",
		"- When Skills are enabled, the runtime deterministically loads one or more explicitly named /<skill-name> references found anywhere in the user message before the first model request. Do not call the skill tool merely to reread a Skill already marked as runtime-loaded in context. The skill tool remains available when the task requires selecting an unspecified Skill by description. Never pretend to use Skills when they are disabled.",
	}, "\n")
	sections := []string{common, thinkingLanguageContract(cfg)}
	if config.IsSubAgentParentKind(agentKind) {
		sections = append(sections, subAgentDelegationContract())
	}
	if specific := agentRuntimeContract(agentKind); specific != "" {
		sections = append(sections, specific)
	}
	return strings.Join(sections, "\n\n")
}

func thinkingLanguageContract(cfg *config.Config) string {
	language := "zh-CN"
	if cfg != nil && cfg.Language == "en-US" {
		language = "en-US"
	}
	if language == "en-US" {
		return strings.Join([]string{
			"## Thinking Language",
			"- Use English for internal reasoning, thinking summaries, and any streamed thinking content.",
			"- This only controls thinking language; do not change required output protocols, JSON keys, file content language, quoted text, or story/dialogue language because of it.",
		}, "\n")
	}
	return strings.Join([]string{
		"## Thinking Language",
		"- Use Simplified Chinese for internal reasoning, thinking summaries, and any streamed thinking content.",
		"- This controls thinking language only. Do not change output protocols, JSON keys, file-content language, quoted source text, or story and dialogue language because of it.",
	}, "\n")
}

func subAgentDelegationContract() string {
	return strings.Join([]string{
		"- Do not proactively start a SubAgent by default. Call the task tool only when the user explicitly requests delegation or a loaded Skill explicitly requires a SubAgent.",
		"- SubAgent delegation protocol: the task description must state the user's goal, necessary context, known constraints, file paths or resource IDs, expected output, and whether writes are allowed.",
		"- For files, lore, or historical events the child can read with tools, pass only paths, IDs, or search clues. Do not copy large bodies of text, complete logs, complete history, or other unbounded content.",
		"- SubAgent results are visible only to the parent by default. The parent must verify them and summarize the result to the user in its final response.",
	}, "\n")
}

func outputProtocolForAgent(agentKind string) string {
	switch agentKind {
	case config.AgentKindGeneral, config.AgentKindHarnessOptimizer:
		if agentKind == config.AgentKindHarnessOptimizer {
			return "- Harness Optimizer has no fixed JSON output protocol. Report the evidence, State Diff, validation result, and whether the changes were recorded. Never claim a change that was not actually completed."
		}
		return "- General Agent has no fixed JSON output protocol. Perform all file changes through enabled tools and stay within the current Project root."
	case config.AgentKindInteractiveStory:
		return strings.Join([]string{
			"- Output only the story prose that can be shown on the story stage for this turn.",
			"- Write only scenes, actions, dialogue, and consequences. Do not output plans, explanations, tool instructions, Markdown headings, XML wrappers, hidden state blocks, shortcut-choice blocks, or JSON.",
		}, "\n")
	case config.AgentKindInteractiveDirector:
		return strings.Join([]string{
			"- Submit incremental Markdown Patches with base_hash through submit_director_plan_update. Files are accepted or rejected independently; retry only retry_documents. Nothing is written to the workspace before finalize succeeds. Ordinary updates change agent-brief.md by default; keep uses empty updates; replan updates at least director.md and agent-brief.md.",
			"- Do not continue story prose, write Actor State, or alter the state schema. Only the opening Game Agent initializes the schema under the story-level policy.",
		}, "\n")
	case config.AgentKindVersionSummary:
		return "- Output exactly one Chinese release-summary sentence containing 10 to 30 Han characters. Do not include numbering, quotation marks, colons, terminal punctuation, or explanation."
	case config.AgentKindToolAgent:
		return "- Output only the JSON object required by the current call site. Do not output explanations, Markdown, code fences, or extra text."
	case config.AgentKindImage:
		return "- Call the image-generation tool to produce the image. The final response should briefly report the result without unrelated explanation or prose modifications."
	case config.AgentKindConfigManager:
		return "- There is no fixed JSON output protocol. Perform all lore, preset, automation, and Skills changes through their corresponding module tools."
	case config.AgentKindAutomation:
		return "- The final output must report what was actually completed, written paths, and items awaiting user confirmation. Writes remain subject to the task write policy and tool permissions."
	case config.AgentKindIDE:
		return "- Writing Agent has no fixed JSON output protocol. Perform all file changes through enabled tools and respect workspace boundaries."
	default:
		return "- Follow the output protocol and backend validation for the current Agent call site."
	}
}

func agentRuntimeContract(agentKind string) string {
	switch agentKind {
	case config.AgentKindGeneral:
		return strings.Join([]string{
			"- General Agent works only within the root of a Project explicitly added by the user. Project type does not change filesystem permissions or create hidden exceptions for Denova data directories.",
			"- Discovery operations such as glob and grep respect .gitignore by default. Explicit-path reads and writes are not blocked by .gitignore, and shell commands retain native semantics.",
			"- Do not modify .gitignore unless the user explicitly requests it.",
		}, "\n")
	case config.AgentKindHarnessOptimizer:
		return strings.Join([]string{
			"- Harness Optimizer works only in the live User State directory, where file edits take effect immediately. Never write Project-private content, complete trajectories, temporary tasks, user secrets, or model reasoning into global State.",
			"- Read evidence first and distinguish generalizable preferences from one-off signals. Remain a no-op when evidence is insufficient, the improvement is unclear, or there is no valid Diff.",
			"- Manage the live directory with ordinary read/write/edit/glob/grep/shell tools, Skills, and Tasks. Do not directly manipulate the State .git directory, version repository, or private runtime directories.",
			"- Make only the smallest cohesive change. There is no draft or publish step; the application records Git history only after the Run succeeds and full validation passes.",
		}, "\n")
	case config.AgentKindIDE:
		return "- Writing Agent must respect file-tool safety and book-workspace boundaries. CREATOR.md and the user's explicit current request remain authoritative for book content."
	case config.AgentKindInteractiveStory:
		return strings.Join([]string{
			"- Interactive Story Agent may use read-only file tools for shared prose-style references explicitly listed by the system prompt. It must not modify workspace files or call write, delete, task-planning, or other mutation tools.",
			"- If initialize_story_state_schema is present, this is the opening turn of a dynamic-state-schema story. Finalize the schema draft first and obtain finalized=true, then output prose and call submit_interactive_turn. The schema tool changes templates and fields only; Actor creation and values belong exclusively in state_changes.",
			"- Each turn, output the complete player-visible prose first, then call submit_interactive_turn. state_changes and choices are parsed and accepted independently through one model-facing entry point. The backend compiles StateDelta and atomically commits the first prose candidate with state only when both succeed. Omit director_update by default; include it only when established events materially change future planning.",
			"- The submission tool returns structured per-module receipts. When ready=false, resubmit only failed or missing fields named by retry_modules through the same tool. When ready=true, end the turn immediately without repeating or rewriting prose.",
			"- Interactive Story Agent must follow the built-in output protocol. Story-stage prose is the final response and must not include state JSON, tool explanations, or XML wrappers.",
			"- The current story's per-turn target character count is the highest length constraint. Other built-in prompts, CREATOR.md chapter-length rules, Director guidance, and custom prompts must not require exceeding it.",
		}, "\n")
	case config.AgentKindConfigManager:
		return strings.Join([]string{
			"- Config Manager Agent configures, creates, and maintains lore, solution presets, automation tasks, Skills, and settings exposed on the Agents page.",
			"- Except for dedicated lore tools, manage all configuration resources through config_read and config_apply. Do not edit underlying configuration files with file tools.",
			"- Do not change ports, theme, remote access, editor appearance, or other settings outside the Agents page. Those changes belong in Settings.",
			"- For an unfamiliar resource, call config_read describe first. Usually list before get; update and delete must use the latest read revision.",
			"- Submit one independent change per config_apply call. Read resource shapes from config-manager Skill references only as needed.",
			"- Deletion, hiding, replacement, and broad rewrites require explicit user instruction. When uncertain, explain the intended change and request confirmation first.",
			"- Lore stores long-lived stable facts only. Do not write short-term post-chapter state to lore by default.",
		}, "\n")
	case config.AgentKindInteractiveDirector:
		return strings.Join([]string{
			"- Director creates or maintains director.md, agent-brief.md, and lore-context.md for the current branch only. It does not initialize or review the state schema.",
			"- Director must not write, overwrite, or correct Actor State or alter a frozen state schema.",
			"- Turn and StateDelta are the sole sources of truth for established events. Use search_story_history for older evidence and retain returned turn_id provenance. Actor State is the current projection, director.md contains future plans, and lore contains stable setting facts; do not mix these boundaries.",
			"- Do not use file tools for director_plan_update or opening_plan. The backend injects snapshots and base_hash values for all three documents; stage only minimal Markdown Patches through submit_director_plan_update. Do not resend accepted files. The backend publishes atomically after finalize succeeds.",
			"- Do not continue story prose, choose actions for the user, or use shell, todo, lore writes, or any workspace mutation.",
			"- Planning should prioritize reusing important lore characters, factions, rules, locations, and established relationships. Serve future interaction through dense relationships, faction pressure, information reveals, high-impact crises, check costs, and branch arrangements.",
			"- Put information safe for the prose Agent in agent-brief.md. Keep hidden truths, behind-the-scenes motives, and future reversals in director.md, which is not injected into prose generation.",
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
