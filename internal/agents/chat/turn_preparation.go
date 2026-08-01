package chat

import (
	"context"
	agentcontext "denova/internal/agents/context"
	"fmt"

	"denova/internal/agents/session"
	novaskills "denova/internal/agents/skills"
	"denova/internal/book"
)

// turnContextPreparationInput declares the turn-scoped sources needed to
// build the exact model input. Preparation is pure with respect to durable
// conversation state; callers publish an accepted input through CommitModelInput.
type turnContextPreparationInput struct {
	Conversation        Conversation
	Request             ChatRequest
	PendingInterruption *session.Interruption
	BookService         *book.Service
	Environment         turnRuntimeEnvironment
}

// preparedTurnContext is the single result consumed by both model execution
// and Context Analysis. ModelContext is the exact conversation assembly; it
// must never be persisted as display history.
type preparedTurnContext struct {
	OriginalMessage    string
	ResumeInterruption *session.Interruption
	ExplicitSkills     []novaskills.Invocation
	ModelContext       agentcontext.ModelContextResult
}

func prepareTurnContext(ctx context.Context, input turnContextPreparationInput) (preparedTurnContext, error) {
	if input.Conversation == nil {
		return preparedTurnContext{}, fmt.Errorf("conversation is required")
	}
	pending := input.PendingInterruption
	if pending == nil && shouldResumeInterruptedRequest(input.Request.Message) {
		pending = input.Conversation.PendingInterruption()
	}
	explicitSkills := []novaskills.Invocation(nil)
	if resolver, ok := input.Conversation.(novaskills.ExplicitResolver); ok {
		resolved, err := resolver.ResolveExplicitSkills(ctx, input.Request.Message)
		if err != nil {
			return preparedTurnContext{}, fmt.Errorf("resolve explicit skills: %w", err)
		}
		explicitSkills = resolved
	}
	budget := agentcontext.ModelContextBudgetFor(input.Conversation)
	projection := projectTurnInput(turnContextProjectionInput{
		Request:             input.Request,
		PendingInterruption: pending,
		BookService:         input.BookService,
		Budget:              budget,
		Environment:         input.Environment,
		ExplicitSkills:      explicitSkills,
	})
	assembled, err := agentcontext.AssembleModelContext(ctx, input.Conversation, projection.OriginalMessage, agentcontext.ModelContextInput{
		UserMessage:    input.Request.Message,
		UserReferences: userMessageReferencesForRequest(input.Request),
		Fragments:      projection.Fragments,
		Budget:         budget,
	})
	if err != nil {
		return preparedTurnContext{}, err
	}
	if err := validateExplicitSkillProjection(assembled.Context, explicitSkills); err != nil {
		return preparedTurnContext{}, err
	}
	return preparedTurnContext{
		OriginalMessage:    projection.OriginalMessage,
		ResumeInterruption: projection.ResumeInterruption,
		ExplicitSkills:     explicitSkills,
		ModelContext:       assembled,
	}, nil
}
