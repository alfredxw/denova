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
		"- Use the task tool only when the user or a loaded Skill requests delegation.",
		"- Give the SubAgent a self-contained goal, constraints, relevant paths or resource IDs, expected output, and write scope. Pass references instead of copying content it can read itself.",
		"- Verify the returned result before reporting it to the user.",
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
		return "- Stay within the current Project root. Discovery respects .gitignore; explicitly named paths remain addressable and shell commands retain native semantics. Modify .gitignore only when the user explicitly requests it."
	case config.AgentKindHarnessOptimizer:
		return strings.Join([]string{
			"- Harness Optimizer reads trajectory and current User Harness resources through read, then modifies User State only through update_harness_state against the current revision.",
			"- State updates are complete validated ChangeSets. Never write Project-private content, complete trajectories, temporary tasks, user secrets, or model reasoning into global State.",
			"- Read evidence first and distinguish generalizable preferences from one-off signals. Remain a no-op when evidence is insufficient, the improvement is unclear, or there is no valid Diff.",
			"- Make only the smallest cohesive change. Do not request filesystem paths, Git handles, or private runtime resources.",
		}, "\n")
	case config.AgentKindIDE:
		return "- Writing Agent must respect file-tool safety and book-workspace boundaries. CREATOR.md and the user's explicit current request remain authoritative for book content."
	case config.AgentKindInteractiveStory:
		return strings.Join([]string{
			"- Game Mode is read-only for workspace files. Use only listed read-only references; persist prose, state, and choices through the Game Mode submission tools.",
			"- The current story's per-turn target character count is the highest length constraint. Other built-in prompts, CREATOR.md chapter-length rules, Director guidance, and custom prompts must not require exceeding it.",
		}, "\n")
	case config.AgentKindConfigManager:
		return "- Manage only Agents-page resources, using dedicated lore tools or config_read/config_apply. Do not edit backing configuration files or Settings-only options."
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
