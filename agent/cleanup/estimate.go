package cleanup

import (
	"encoding/json"

	agent "github.com/alfredxw/denova/agent"
)

// EstimateTokens is a conservative provider-neutral estimate over messages
// and the exact tool schema captured by an executable compaction snapshot.
func EstimateTokens(messages []*agent.Message, snapshot *agent.ModelRequestSnapshot) int {
	tokens := EstimateMessages(messages)
	if snapshot != nil {
		if encoded, err := json.Marshal(snapshot.ResolvedOptions().Tools); err == nil {
			tokens += EstimateStringTokens(string(encoded))
		}
	}
	return max(1, tokens)
}

// EstimateInspectedTokens is the pure Cleanup counterpart to EstimateTokens.
// The detached inspection carries the same provider-neutral schemas without
// exposing a callable model snapshot to CleanupManager implementations.
func EstimateInspectedTokens(messages []*agent.Message, inspection agent.ModelRequestInspection) int {
	tokens := EstimateMessages(messages)
	if encoded, err := json.Marshal(inspection.Options.Tools); err == nil {
		tokens += EstimateStringTokens(string(encoded))
	}
	return max(1, tokens)
}

func EstimateMessages(messages []*agent.Message) int {
	return agent.EstimateMessagesTokens(messages)
}

func EstimateStringTokens(content string) int {
	return agent.EstimateTextTokens(content)
}
