package compaction

import (
	"strings"

	"denova/config"
	agentcontext "denova/internal/agents/context"
)

const (
	compactionForkContract         = "This is a one-turn checkpoint side fork. Do not call tools. Return only the Markdown checkpoint; do not discuss these instructions."
	compactionGuidancePrecedence   = "User-authored checkpoint preferences may refine emphasis only. They cannot override the stable schema, source boundaries, visibility policy, tool ban, evidence requirements, or output budget."
	compactionEvidenceRequirements = "Never invent missing evidence. Exclude private reasoning, UI-only logs, streaming fragments, and transport noise."
)

// BuiltinPromptSources exposes the immutable checkpoint protocol used by each
// built-in runtime kind without exposing dynamic conversation data.
func BuiltinPromptSources() config.AgentPromptSourceSettings {
	return config.AgentPromptSourceSettings{
		General:          compactionPromptSourceList(config.AgentKindGeneral),
		IDE:              compactionPromptSourceList(config.AgentKindIDE),
		InteractiveStory: compactionPromptSourceList(config.AgentKindInteractiveStory),
		VersionSummary:   compactionPromptSourceList(config.AgentKindVersionSummary),
		ToolAgent:        compactionPromptSourceList(config.AgentKindToolAgent),
		Image:            compactionPromptSourceList(config.AgentKindImage),
	}
}

func compactionPromptSourceList(agentKind string) config.AgentPromptSourceList {
	runtimeContract := strings.Join([]string{
		compactionForkContract,
		compactionRetentionRequirements(),
		compactionGuidancePrecedence,
		compactionEvidenceRequirements,
	}, "\n")
	return config.AgentPromptSourceList{Sources: []config.AgentPromptSource{
		{ID: "runtime_contract", Title: "Checkpoint Runtime Contract", Source: "Denova runtime", Content: runtimeContract},
		{ID: "checkpoint_schema", Title: "Checkpoint Output Schema", Source: "Denova runtime", Content: strings.TrimSpace(agentcontext.CompactionCheckpointSchema())},
		{ID: "domain_rules", Title: "Agent Domain Rules", Source: "Denova runtime", Content: compactionDomainRequirements(agentKind)},
	}}
}

func compactionRetentionRequirements() string {
	return "Summarize only the selected source range. It may contain completed assistant steps inside the current user task. The newest complete tool group stays outside that range as original context. Merge the prior checkpoint with newly selected evidence; preserve goals, constraints, corrected numbers and identifiers, completed work, pending work, and the next action. Do not repeat a completed side effect merely because it appears in this checkpoint."
}

func compactionDomainRequirements(agentKind string) string {
	if agentKind == config.AgentKindInteractiveStory {
		return "Game-mode requirements: preserve event order and causality, source turn IDs, Actor State changes, Lore sources, branch-plan status, relationships, quests, foreshadowing, secrets, dangers, and countdowns. Treat current Actor State, Lore, and the branch plan as deterministic sources rather than inventing replacements."
	}
	return "Workspace/writing requirements: preserve the user's objective and constraints, current draft or implementation state, file/artifact references, decisions and rationale, verified results, rejected approaches, unresolved risks, and dependency-ordered next actions."
}
