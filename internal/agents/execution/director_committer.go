package execution

import (
	"context"
	"errors"

	agentchat "denova/internal/agents/chat"
	agentinteractive "denova/internal/agents/interactive"
	agentlifecycle "denova/internal/agents/lifecycle"

	agent "github.com/alfredxw/denova/agent"
)

// directorConversationCommitter joins the public Agent canonical fence to the
// existing Story plan transaction. Director input has no separate product
// projection: the source Turn is already stored, so a successful no-op is the
// complete side effect. Output is delegated to the Story plan transaction.
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
	// There is intentionally no product write here. The source Turn already
	// carries the accepted input.
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
