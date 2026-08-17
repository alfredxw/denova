package app

import (
	"context"
	"fmt"

	agent "github.com/alfredxw/denova/agent"

	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentlifecycle "denova/internal/agents/lifecycle"
	agentrun "denova/internal/agents/run"
	"denova/internal/book"
	"denova/internal/interactive"
)

type publicReplayConversationCommitter struct{}

func (publicReplayConversationCommitter) MaterializeInput(
	context.Context,
	agent.InputCommitRequest,
) (agent.CommitReceipt, error) {
	return agent.CommitReceipt{Revision: "test-input"}, nil
}

func (publicReplayConversationCommitter) ApplyPreparedContext(
	context.Context,
	agentchat.AgentContextPreparation,
) error {
	return nil
}

func (publicReplayConversationCommitter) CommitOutput(
	context.Context,
	agentchat.AgentContextPreparation,
	agent.OutputCommitRequest,
) (agent.OutputCommitReceipt, error) {
	return agent.OutputCommitReceipt{Revision: "test-output"}, nil
}

func (publicReplayConversationCommitter) ApplyEffects(
	_ context.Context,
	requests []agent.EffectRequest,
) ([]agent.EffectResult, error) {
	results := make([]agent.EffectResult, len(requests))
	for index, request := range requests {
		results[index] = agent.EffectResult{ID: request.ID, Revision: "test-effect"}
	}
	return results, nil
}

type interactiveReplayConversationCommitter struct {
	publicReplayConversationCommitter
	conversation *interactiveReplayConversation
}

func (committer interactiveReplayConversationCommitter) MaterializeInput(
	_ context.Context,
	request agent.InputCommitRequest,
) (agent.CommitReceipt, error) {
	conversation := committer.conversation
	if conversation == nil || conversation.store == nil {
		return agent.CommitReceipt{Revision: "test-input"}, nil
	}
	intent, err := interactive.NewPlayerInputIntent(interactive.DomainCommitIdentity{
		CommandID: request.Identity.CommandID, OperationID: request.Identity.RunID, Cycle: request.Identity.Cycle,
	}, conversation.branchID, conversation.message)
	if err != nil {
		return agent.CommitReceipt{}, err
	}
	intent, err = intent.WithAgentCanonicalHash(request.Hash)
	if err != nil {
		return agent.CommitReceipt{}, err
	}
	receipt, err := conversation.store.CommitPlayerInput(conversation.storyID, intent)
	if err != nil {
		return agent.CommitReceipt{}, err
	}
	return agent.CommitReceipt{Revision: receipt.Revision}, nil
}

func (conversation *interactiveReplayConversation) NewAgentConversationCommitter(
	agentrun.Options,
	agentlifecycle.ToolEffectApplier,
) (agentlifecycle.ConversationCommitter, error) {
	return interactiveReplayConversationCommitter{conversation: conversation}, nil
}

func publicReplayDefinition(model agent.BaseChatModel, name string) agent.Definition {
	return agent.Definition{
		Key: "denova.test.public-replay", Name: name, Model: model,
		ModelIdentity: agent.CapabilityIdentity{Kind: "model.denova.test.public-replay", Version: 1},
	}
}

func startPublicExecutionCycle(
	runtime *agentexecution.Runtime,
	ctx context.Context,
	model agent.BaseChatModel,
	conversation agentchat.Conversation,
	bookService *book.Service,
	request agentchat.ChatRequest,
	options agentrun.Options,
	emit func(agentrun.Event),
) (*agentexecution.Operation, error) {
	options = options.Normalize(options.Workspace)
	return runtime.Start(ctx, agentexecution.StartRequest{
		Cycle: agentexecution.Cycle{
			Definition: publicReplayDefinition(model, options.RootAgentName), Conversation: conversation, BookService: bookService,
			Request: request, Options: options,
		},
		Emit: emit,
	})
}

var _ agentlifecycle.ConversationCommitter = publicReplayConversationCommitter{}

func executionProfileForTest(application *App, binding agentrun.RuntimeBinding) (agentexecution.Profile, error) {
	profileID, err := binding.ProfileID()
	if err != nil {
		return nil, err
	}
	for _, profile := range application.executionProfiles() {
		if profile.ID() == agentexecution.ProfileID(profileID) {
			return profile, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", agentexecution.ErrProfileNotFound, profileID)
}

func prepareProfileCycleForTest(application *App, ctx context.Context, request agentexecution.CycleRestoreRequest) (agentexecution.Cycle, error) {
	profile, err := executionProfileForTest(application, request.Binding)
	if err != nil {
		return agentexecution.Cycle{}, err
	}
	queued, ok := profile.(agentexecution.QueuedCycleProfile)
	if !ok {
		return agentexecution.Cycle{}, fmt.Errorf("%w: profile %q", agentexecution.ErrCyclePreparationUnavailable, profile.ID())
	}
	return queued.PrepareCycle(ctx, request)
}
