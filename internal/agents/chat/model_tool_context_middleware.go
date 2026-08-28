package chat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"

	"denova/internal/agents/toolresult"

	agent "github.com/alfredxw/denova/agent"
)

type modelHistoryProjectionMiddleware struct {
	*agent.BaseMiddleware
	policy toolresult.ContextPolicy
}

// NewModelHistoryProjectionMiddleware applies Denova's product history policy
// only before the active raw user message. The current cycle's tool exchange
// remains visible. Provider adapters own reasoning replay because signed or
// encrypted reasoning state is protocol-specific and may be required on the
// next turn.
func NewModelHistoryProjectionMiddleware(policy toolresult.ContextPolicy) agent.IdentifiedMiddleware {
	return &modelHistoryProjectionMiddleware{
		BaseMiddleware: &agent.BaseMiddleware{}, policy: policy.Normalize(),
	}
}

func (middleware *modelHistoryProjectionMiddleware) Identity() agent.CapabilityIdentity {
	encoded, _ := json.Marshal(middleware.policy)
	digest := sha256.Sum256(encoded)
	return agent.CapabilityIdentity{
		Kind: "denova.model.history_projection", Version: 1,
		ConfigHash: hex.EncodeToString(digest[:]),
	}
}

func (middleware *modelHistoryProjectionMiddleware) BeforeModelCall(
	ctx context.Context,
	call *agent.ModelCall,
	modelContext *agent.ModelContext,
) (context.Context, *agent.ModelCall, error) {
	if middleware == nil || call == nil {
		return ctx, call, nil
	}
	activeUser := lastModelUserIndex(call.Messages)
	if activeUser <= 0 {
		return ctx, call, nil
	}
	projected := toolresult.ApplyContextPolicy(call.Messages[:activeUser], middleware.policy)
	messages := make([]*agent.Message, 0, len(projected)+len(call.Messages)-activeUser)
	messages = append(messages, projected...)
	for _, message := range call.Messages[activeUser:] {
		messages = append(messages, agent.CloneMessage(message))
	}
	if reflect.DeepEqual(call.Messages, messages) {
		return ctx, call, nil
	}
	if modelContext != nil {
		modelContext.ReportContextNormalization(agent.ContextNormalizationMetrics{
			RepairCount: 1, MessagesBefore: len(call.Messages), MessagesAfter: len(messages),
		})
	}
	next := *call
	next.Messages = messages
	return ctx, &next, nil
}

func lastModelUserIndex(messages []*agent.Message) int {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index] != nil && messages[index].Role == agent.User {
			return index
		}
	}
	return -1
}
