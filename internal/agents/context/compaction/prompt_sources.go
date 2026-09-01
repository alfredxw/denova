package compaction

import (
	"fmt"
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
		compactionRetentionRequirements(config.DefaultContextCompactionRetainedTurns),
		compactionGuidancePrecedence,
		compactionEvidenceRequirements,
	}, "\n")
	return config.AgentPromptSourceList{Sources: []config.AgentPromptSource{
		{ID: "runtime_contract", Title: "Checkpoint Runtime Contract", Source: "Denova runtime", Content: runtimeContract},
		{ID: "checkpoint_schema", Title: "Checkpoint Output Schema", Source: "Denova runtime", Content: strings.TrimSpace(agentcontext.CompactionCheckpointSchema())},
		{ID: "domain_rules", Title: "Agent Domain Rules", Source: "Denova runtime", Content: compactionDomainRequirements(agentKind)},
	}}
}

func compactionRetentionRequirements(retainedTurns int) string {
	return fmt.Sprintf("Keep the most recent %d complete user turn(s) as a verbatim convenience tail in the primary context. The checkpoint must still cover durable facts from the entire canonical source range, including that retained tail, because a later compaction can age those turns out. Summarize those facts concisely instead of copying the tail verbatim.", retainedTurns)
}

func compactionDomainRequirements(agentKind string) string {
	if agentKind == config.AgentKindInteractiveStory {
		return "Game-mode requirements: preserve event order and causality, source turn IDs, Actor State changes, Lore sources, branch-plan status, relationships, quests, foreshadowing, secrets, dangers, and countdowns. Treat current Actor State, Lore, and the branch plan as deterministic sources rather than inventing replacements."
	}
	return "Workspace/writing requirements: preserve the user's objective and constraints, current draft or implementation state, file/artifact references, decisions and rationale, verified results, rejected approaches, unresolved risks, and dependency-ordered next actions."
}

func appendCheckpointGuidance(builder *strings.Builder, guidance string) {
	guidance = strings.TrimSpace(guidance)
	if guidance == "" {
		return
	}
	builder.WriteString("<checkpoint_guidance>\n")
	builder.WriteString(guidance)
	builder.WriteString("\n</checkpoint_guidance>\n")
	builder.WriteString(compactionGuidancePrecedence)
	builder.WriteString("\n\n")
}
