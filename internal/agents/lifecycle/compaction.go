package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	agent "github.com/alfredxw/denova/agent"
)

// AgentCompactionContextProvider lets a Denova product conversation attach a
// bounded cursor to an Agent-owned checkpoint. Agent persists the data opaquely
// and returns it to ContextSource on later cycles.
type AgentCompactionContextProvider interface {
	AgentCompactionContext(context.Context, agent.CompactionCompactRequest) (*agent.HostData, error)
}

// AgentCompactionBinder lets a conversation project the current Agent-owned
// checkpoint onto host context assembly without creating a competing durable
// compaction record in the product store.
type AgentCompactionBinder interface {
	BindAgentCompaction(*agent.CompactionState) error
}

type conversationCompactionManager struct {
	delegate agent.CompactionManager
	provider AgentCompactionContextProvider
	identity agent.CapabilityIdentity
}

// BindConversationCompaction decorates a generic manager only when the
// conversation supplies host-context metadata. All planning and summarization
// remain owned by the selected manager.
func BindConversationCompaction(
	manager agent.CompactionManager,
	conversation any,
) agent.CompactionManager {
	provider, ok := conversation.(AgentCompactionContextProvider)
	if manager == nil || !ok || provider == nil {
		return manager
	}
	encoded, _ := json.Marshal(manager.Identity())
	digest := sha256.Sum256(encoded)
	return &conversationCompactionManager{
		delegate: manager, provider: provider,
		identity: agent.CapabilityIdentity{
			Kind: "denova.compaction.context", Version: 1,
			ConfigHash: hex.EncodeToString(digest[:]),
		},
	}
}

func (manager *conversationCompactionManager) Identity() agent.CapabilityIdentity {
	if manager == nil {
		return agent.CapabilityIdentity{}
	}
	return manager.identity
}

func (manager *conversationCompactionManager) Plan(
	ctx context.Context,
	request agent.CompactionPlanRequest,
) (agent.CompactionPlan, error) {
	if manager == nil || manager.delegate == nil {
		return agent.CompactionPlan{}, errors.New("Denova Compaction delegate is unavailable")
	}
	return manager.delegate.Plan(ctx, request)
}

func (manager *conversationCompactionManager) Compact(
	ctx context.Context,
	request agent.CompactionCompactRequest,
) (agent.CompactionCheckpoint, error) {
	if manager == nil || manager.delegate == nil || manager.provider == nil {
		return agent.CompactionCheckpoint{}, errors.New("Denova Compaction context provider is unavailable")
	}
	checkpoint, err := manager.delegate.Compact(ctx, request)
	if err != nil {
		return agent.CompactionCheckpoint{}, err
	}
	data, err := manager.provider.AgentCompactionContext(ctx, request)
	if err != nil {
		return agent.CompactionCheckpoint{}, err
	}
	checkpoint.ContextData = data
	return checkpoint, nil
}

func bindAgentCompaction(conversation any, state *agent.CompactionState) error {
	binder, ok := conversation.(AgentCompactionBinder)
	if !ok || binder == nil {
		return nil
	}
	return binder.BindAgentCompaction(state)
}

var _ agent.CompactionManager = (*conversationCompactionManager)(nil)
