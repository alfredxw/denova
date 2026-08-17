package toolruntime

import (
	"encoding/json"
	"fmt"
	"strings"

	agenttool "denova/internal/agents/tool"

	agent "github.com/alfredxw/denova/agent"
)

const AgentToolMutationEffectKind = "denova.tool_mutation.v1"

type agentToolMutationEffect struct {
	Version  int                `json:"version"`
	Mutation agenttool.Mutation `json:"mutation"`
}

// AgentToolMutationEffect converts a Denova mutation receipt into Agent's
// canonical effect format. The trusted Adapter adds product identity.
func AgentToolMutationEffect(record agenttool.ExecutionRecord) (agent.Effect, bool, error) {
	mutation, ok := agenttool.MutationFromExecutionRecord(record)
	if !ok {
		return agent.Effect{}, false, nil
	}
	payload, err := json.Marshal(agentToolMutationEffect{Version: 1, Mutation: mutation})
	if err != nil {
		return agent.Effect{}, false, fmt.Errorf("encode Denova Agent Tool mutation effect: %w", err)
	}
	return agent.Effect{Kind: AgentToolMutationEffectKind, Data: payload}, true, nil
}

func DecodeAgentToolMutationEffect(effect agent.Effect) (agenttool.Mutation, error) {
	if strings.TrimSpace(effect.Kind) != AgentToolMutationEffectKind {
		return agenttool.Mutation{}, fmt.Errorf("unsupported Denova Agent Tool effect %q", effect.Kind)
	}
	var payload agentToolMutationEffect
	if err := json.Unmarshal(effect.Data, &payload); err != nil {
		return agenttool.Mutation{}, fmt.Errorf("decode Denova Agent Tool mutation effect: %w", err)
	}
	if payload.Version != 1 || strings.TrimSpace(payload.Mutation.ToolName) == "" {
		return agenttool.Mutation{}, fmt.Errorf("invalid Denova Agent Tool mutation effect")
	}
	payload.Mutation.LoreItemIDs = append([]string(nil), payload.Mutation.LoreItemIDs...)
	payload.Mutation.DeletedLoreItemIDs = append([]string(nil), payload.Mutation.DeletedLoreItemIDs...)
	return payload.Mutation, nil
}
