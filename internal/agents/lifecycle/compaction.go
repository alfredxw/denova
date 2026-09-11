package lifecycle

import agent "github.com/alfredxw/denova/agent"

// AgentCompactionBinder lets a conversation project the current Agent-owned
// checkpoint onto host context assembly without creating a competing durable
// compaction record in the product store.
type AgentCompactionBinder interface {
	BindAgentCompaction(*agent.CompactionState) error
}

func bindAgentCompaction(conversation any, state *agent.CompactionState) error {
	binder, ok := conversation.(AgentCompactionBinder)
	if !ok || binder == nil {
		return nil
	}
	return binder.BindAgentCompaction(state)
}
