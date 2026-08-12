package execution

import (
	"context"
	"errors"
	"fmt"

	agentchat "denova/internal/agents/chat"
	agentinteractive "denova/internal/agents/interactive"
	agentlifecycle "denova/internal/agents/lifecycle"

	agent "github.com/alfredxw/denova/agent"
)

// directorConversationCommitter joins the public Agent canonical fence to the
// existing Story plan transaction. Director input has no separate product
// projection: the source Turn is already durable, so a successful no-op is the
// complete side effect and the Agent journal owns its exact receipt. Output is
// always delegated to the canonical Story plan transaction and is queryable by
// the exact Agent identity/hash after a crash.
type directorConversationCommitter struct {
	conversation *agentinteractive.DirectorConversation
	output       agentinteractive.DirectorCanonicalOutput
	applyEffects agentlifecycle.ToolEffectApplier
}

func newDirectorConversationCommitter(
	conversation *agentinteractive.DirectorConversation,
	applyEffects agentlifecycle.ToolEffectApplier,
) (agentlifecycle.ConversationCommitter, error) {
	if conversation == nil {
		return nil, errors.New("Denova Director committer requires a conversation")
	}
	return &directorConversationCommitter{
		conversation: conversation,
		output:       conversation.CanonicalOutput(),
		applyEffects: applyEffects,
	}, nil
}

func (committer *directorConversationCommitter) MaterializeInput(
	ctx context.Context,
	request agent.InputCommitRequest,
) (agent.CommitReceipt, error) {
	if request.Identity.Stage != agent.CommitInput {
		return agent.CommitReceipt{}, errors.New("Denova Director received a non-input materialization")
	}
	if err := ctx.Err(); err != nil {
		return agent.CommitReceipt{}, err
	}
	// There is intentionally no product write here. Returning an identity-bound
	// revision lets the public runtime durably record that the empty effect was
	// completed; Reconcile can prove the same fact without process state.
	return agent.CommitReceipt{Revision: "director-input:" + request.Hash}, nil
}

func (committer *directorConversationCommitter) ApplyPreparedContext(
	ctx context.Context,
	prepared agentchat.AgentContextPreparation,
) error {
	return committer.conversation.CommitModelInput(ctx, prepared.OriginalMessage, prepared.ModelContext)
}

func (committer *directorConversationCommitter) CommitOutput(
	ctx context.Context,
	_ agentchat.AgentContextPreparation,
	request agent.OutputCommitRequest,
) (agent.OutputCommitReceipt, error) {
	if committer.output == nil {
		return agent.OutputCommitReceipt{}, errors.New("Denova Director canonical output is unavailable")
	}
	return committer.output.CommitDirectorCanonicalOutput(ctx, request)
}

func (committer *directorConversationCommitter) Reconcile(
	ctx context.Context,
	request agent.ReconcileRequest,
) (agent.ReconcileResult, error) {
	switch request.Identity.Stage {
	case agent.CommitInput:
		if err := ctx.Err(); err != nil {
			return agent.ReconcileResult{}, err
		}
		return agent.ReconcileResult{Found: true, Revision: "director-input:" + request.Hash}, nil
	case agent.CommitOutput:
		if committer.output == nil {
			return agent.ReconcileResult{}, errors.New("Denova Director canonical output is unavailable")
		}
		return committer.output.ReconcileDirectorCanonicalOutput(ctx, request)
	default:
		return agent.ReconcileResult{}, fmt.Errorf("unsupported Director canonical stage %q", request.Identity.Stage)
	}
}

func (committer *directorConversationCommitter) ApplyEffects(
	ctx context.Context,
	requests []agent.EffectRequest,
) ([]agent.EffectResult, error) {
	if len(requests) == 0 {
		return nil, nil
	}
	if committer.applyEffects == nil {
		return nil, errors.New("Denova Director has Tool effects but no effect applier")
	}
	return committer.applyEffects(ctx, requests)
}

var _ agentlifecycle.ConversationCommitter = (*directorConversationCommitter)(nil)
