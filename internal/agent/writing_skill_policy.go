package agent

import "nova/config"

func boolPtr(v bool) *bool { return &v }

// ApplyWritingSkillRolePolicy adjusts only the lightweight subagent surface for
// built-in writing presets. User-defined IDE skills keep the normal configured
// agent capabilities and express their workflow through SKILL.md.
func ApplyWritingSkillRolePolicy(cfg *config.Config, skillName string) {
	if cfg == nil || !isBuiltinWritingSkill(skillName) {
		return
	}
	switch skillName {
	case "novel-lite":
		cfg.GeneralSubAgents.IDE = boolPtr(false)
		cfg.SubAgents = nil
	case "novel-standard":
		applyPresetWritingSubAgents(cfg, standardWritingSubAgents())
	case "novel-heavy":
		applyPresetWritingSubAgents(cfg, heavyWritingSubAgents())
	}
}

func applyPresetWritingSubAgents(cfg *config.Config, roles []config.SubAgentConfig) {
	cfg.GeneralSubAgents.IDE = boolPtr(false)
	cfg.SubAgents = config.MergeSubAgents(roles, matchingSubAgentOverrides(cfg.SubAgents, roles))
}

func standardWritingSubAgents() []config.SubAgentConfig {
	return []config.SubAgentConfig{
		writingRoleSubAgent("writer", "Writer", "Drafts the requested prose according to the selected Writing Skill.", "Create the first draft for the requested writing task. Follow the user's requested range exactly and do not assume the task is always the next chapter."),
		writingRoleSubAgent("reviewer", "Reviewer", "Reviews the draft and returns structured issues without editing the prose.", "Review the draft only. Return structured feedback with severity, dimension, problem, fix_instruction, and keep. Do not rewrite the prose."),
		writingRoleSubAgent("fixer", "Fixer", "Repairs the draft according to reviewer feedback while preserving the story.", "Apply only necessary fixes from the reviewer. Preserve the draft's plot, voice, and useful passages; do not turn it into a different story."),
	}
}

func heavyWritingSubAgents() []config.SubAgentConfig {
	return []config.SubAgentConfig{
		writingRoleSubAgent("context-planner", "Context Planner", "Prepares the lightweight context plan for this writing turn.", "Summarize the relevant context and produce the required Context Plan before drafting. Infer the writing range from the user's wording."),
		writingRoleSubAgent("writer", "Writer", "Drafts the requested prose from the context plan.", "Create the draft from the approved context plan and requested writing range."),
		writingRoleSubAgent("reviewer", "Reviewer", "Performs one comprehensive review pass.", "Review continuity, voice, pacing, prose, dialogue, plot logic, style, and user requirements. Return structured issues only."),
		writingRoleSubAgent("fixer", "Fixer", "Repairs the draft according to the review.", "Fix the specific problems raised by the reviewer while preserving the story and useful text."),
		writingRoleSubAgent("final-gate", "Final Gate", "Checks final quality before output.", "Verify the fixed draft satisfies the user request, context plan, canon constraints, and style constraints. Return pass/fail with blocking issues only."),
		writingRoleSubAgent("memory-patcher", "Memory Patcher", "Produces progress and character-state updates after writing.", "Generate memory patches for progress and character states. Suggest stable lore updates only when a major confirmed change requires user confirmation."),
	}
}

func writingRoleSubAgent(id, name, description, prompt string) config.SubAgentConfig {
	return config.SubAgentConfig{
		ID:           id,
		Name:         name,
		Description:  description,
		SystemPrompt: prompt,
		Parents:      []string{config.AgentKindIDE},
		Enabled:      boolPtr(true),
	}
}

func matchingSubAgentOverrides(subAgents, roles []config.SubAgentConfig) []config.SubAgentConfig {
	roleIDs := make(map[string]bool, len(roles))
	for _, role := range roles {
		roleIDs[config.NormalizeSubAgentID(role.ID)] = true
	}
	out := make([]config.SubAgentConfig, 0, len(subAgents))
	for _, sub := range subAgents {
		if roleIDs[config.NormalizeSubAgentID(sub.ID)] {
			out = append(out, sub)
		}
	}
	return out
}
