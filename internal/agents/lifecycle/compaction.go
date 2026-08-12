package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	agent "github.com/alfredxw/denova/agent"
)

// ProductCompactionProjection is the product-owned portion of one Agent
// checkpoint. SourceMessages is the exact canonical history delta to summarize;
// ContextData is the bounded cursor needed to re-project that summary later.
// Agent still owns source-range CAS, revisioning, persistence, and recovery.
type ProductCompactionProjection struct {
	SourceMessages []*agent.Message
	ContextData    *agent.HostData
}

// AgentCompactionProjectionProvider lets a product conversation replace the
// generic transcript summary source with its canonical domain history. This is
// required by projections such as Game, whose current user message contains a
// rendered view of story turns rather than the turns themselves.
type AgentCompactionProjectionProvider interface {
	PrepareAgentCompaction(context.Context, agent.CompactionCompactRequest) (ProductCompactionProjection, error)
}

// AgentCompactionBinder lets a conversation project the current Agent-owned
// checkpoint onto host context assembly without creating a competing durable
// compaction record in the product store.
type AgentCompactionBinder interface {
	BindAgentCompaction(*agent.CompactionState) error
}

type conversationCompactionManager struct {
	delegate agent.CompactionManager
	provider AgentCompactionProjectionProvider
	identity agent.CapabilityIdentity
}

// BindConversationCompaction decorates a generic manager only when the
// conversation supplies host-context metadata. All planning and summarization
// remain owned by the selected manager.
func BindConversationCompaction(
	manager agent.CompactionManager,
	conversation any,
) agent.CompactionManager {
	provider, ok := conversation.(AgentCompactionProjectionProvider)
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

func (manager *conversationCompactionManager) SummaryLimitBytes() int {
	if manager == nil || manager.delegate == nil {
		return 0
	}
	return manager.delegate.SummaryLimitBytes()
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
		return agent.CompactionCheckpoint{}, errors.New("Denova Compaction projection provider is unavailable")
	}
	projection, err := manager.provider.PrepareAgentCompaction(ctx, request)
	if err != nil {
		return agent.CompactionCheckpoint{}, err
	}
	if len(projection.SourceMessages) == 0 {
		return agent.CompactionCheckpoint{}, errors.New("Denova Compaction canonical source is empty")
	}
	request.SourceMessages = productCompactionSource(request, projection.SourceMessages)
	checkpoint, err := manager.delegate.Compact(ctx, request)
	if err != nil {
		return agent.CompactionCheckpoint{}, err
	}
	checkpoint.ContextData = projection.ContextData
	return checkpoint, nil
}

func productCompactionSource(request agent.CompactionCompactRequest, canonical []*agent.Message) []*agent.Message {
	result := make([]*agent.Message, 0, len(canonical)+1)
	if request.Present && !request.Current.Removed &&
		request.Current.ReplacementFrom == request.Plan.SourceFrom &&
		request.Current.ReplacementTo >= request.Plan.SourceFrom &&
		request.Current.ReplacementTo <= request.Plan.SourceTo && len(request.SourceMessages) > 0 {
		// Agent already rendered the exact active checkpoint marker as the first
		// incremental source message. Preserve it, but replace the repeated host
		// prompt tail with the product's canonical delta.
		result = append(result, request.SourceMessages[0].Clone())
	}
	for _, message := range canonical {
		result = append(result, message.Clone())
	}
	return result
}

func bindAgentCompaction(conversation any, state *agent.CompactionState) error {
	binder, ok := conversation.(AgentCompactionBinder)
	if !ok || binder == nil {
		return nil
	}
	return binder.BindAgentCompaction(state)
}

var _ agent.CompactionManager = (*conversationCompactionManager)(nil)
