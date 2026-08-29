package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
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

// AgentContextPreparation is the product-owned, pure context assembly consumed
// by the public Agent ContextSource and CanonicalAdapter. ModelContext may
// contain product history for audit/commit purposes; the adapter selects only
// accountable turn fragments because the public Agent owns the transcript.
type AgentContextPreparation struct {
	OriginalMessage    string
	ResumeInterruption *session.Interruption
	ExplicitSkills     []novaskills.Invocation
	ModelContext       agentcontext.ModelContextResult
}

// PrepareAgentContext preserves Denova's exact context projection and localized
// rendering while leaving publication to the public Agent canonical fence.
func PrepareAgentContext(
	ctx context.Context,
	conversation Conversation,
	request ChatRequest,
	bookService *book.Service,
	workspace string,
	startedAt time.Time,
) (AgentContextPreparation, error) {
	// Real turns receive a timestamp from the durable CycleStarted event.
	// Structural operations deliberately pass zero: compaction must not inject
	// a fresh wall-clock value into an otherwise replayable Definition.
	environment := turnRuntimeEnvironment{
		CapturedAt: startedAt,
		Workspace:  strings.TrimSpace(workspace),
	}
	prepared, err := prepareTurnContext(ctx, turnContextPreparationInput{
		Conversation: conversation, Request: request, BookService: bookService,
		Environment: environment,
	})
	if err != nil {
		return AgentContextPreparation{}, err
	}
	return AgentContextPreparation{
		OriginalMessage: prepared.OriginalMessage, ResumeInterruption: prepared.ResumeInterruption,
		ExplicitSkills: append([]novaskills.Invocation(nil), prepared.ExplicitSkills...),
		ModelContext:   prepared.ModelContext,
	}, nil
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
		Attachments:    append([]agent.Attachment(nil), input.Request.AttachedFiles...),
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
