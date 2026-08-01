package conversation

import agentcompaction "denova/internal/agents/context/compaction"

func coldCompactionTestInput(input agentcompaction.Input, summarize agentcompaction.SummaryFunc) agentcompaction.Input {
	input.ColdFallbackReason = "test_fixture"
	input.Summarize = summarize
	return input
}
