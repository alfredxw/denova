package context

import (
	"context"
	"fmt"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

// UserReference is display-only provenance attached to one accepted user
// message. It is deliberately excluded from model-visible content.
type UserReference struct {
	Kind      string `json:"kind"`
	ID        string `json:"id,omitempty"`
	Label     string `json:"label"`
	Detail    string `json:"detail,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

// ModelContextInput is the complete turn-scoped assembly request. The raw
// transcript is supplied separately by the Conversation; UserReferences is
// display-only commit metadata, while Fragments contains only intentionally
// projected model context with explicit provenance.
type ModelContextInput struct {
	UserMessage    string
	Attachments    []agent.Attachment
	UserReferences []UserReference
	Fragments      []Fragment
	Budget         Budget
}

// ModelContextResult returns both exact model messages and the assembly audit.
// Callers must not persist Messages as display history.
type ModelContextResult struct {
	Messages []*agent.Message
	Context  Result
	// CommitState is opaque conversation-owned data derived during pure
	// assembly and applied only by CommitModelInput. Generic callers must not
	// inspect or persist it.
	CommitState any
}

// ModelContextAssembler owns pure model-message assembly for one Conversation.
// It must not append history, stage domain intents, or mutate durable state.
type ModelContextAssembler interface {
	AssembleModelContext(context.Context, string, ModelContextInput) (ModelContextResult, error)
}

// ModelInputCommitter publishes the accepted user input after assembly has
// succeeded. The supplied result is the exact assembly that will be executed.
type ModelInputCommitter interface {
	CommitModelInput(context.Context, string, ModelContextResult) error
}

type ModelContextBudgetProvider interface {
	ModelContextBudget() Budget
}

// ModelContextBudgetFor returns a conversation-specific budget when available.
func ModelContextBudgetFor(conversation any) Budget {
	if provider, ok := conversation.(ModelContextBudgetProvider); ok {
		return provider.ModelContextBudget()
	}
	return DefaultBudget()
}

// ContextBudgetForAgent resolves the configured hard injection budget for one
// Agent profile.
func ContextBudgetForAgent(cfg *config.Config, agentKind string) Budget {
	resolved := config.ResolveAgentContext(cfg, agentKind)
	return Budget{
		MaxFragmentBytes:      resolved.MaxFragmentBytes,
		MaxTotalBytes:         resolved.MaxTotalInjectedBytes,
		MaxFragments:          resolved.MaxFragments,
		MaxMetadataFieldBytes: resolved.MaxMetadataFieldBytes,
	}
}

// AssembleSingleUserModelContext is the shared pure implementation for
// stateless Conversations whose model input is one user message plus bounded
// turn fragments.
func AssembleSingleUserModelContext(ctx context.Context, input ModelContextInput) (ModelContextResult, error) {
	assembled, err := NewAssembler(input.Budget).Assemble(ctx, AssembleRequest{
		Messages: []*agent.Message{agent.UserMessageWithAttachments(input.UserMessage, input.Attachments)}, Fragments: input.Fragments,
	})
	if err != nil {
		return ModelContextResult{}, err
	}
	return ModelContextResult{Messages: assembled.Messages, Context: assembled}, nil
}

// AssembleModelContext builds the exact model-visible messages without
// changing conversation history or domain commit state.
func AssembleModelContext(
	ctx context.Context,
	conversation any,
	originalMessage string,
	input ModelContextInput,
) (ModelContextResult, error) {
	if conversation == nil {
		return ModelContextResult{}, fmt.Errorf("conversation is required")
	}
	assembler, ok := conversation.(ModelContextAssembler)
	if !ok {
		return ModelContextResult{}, fmt.Errorf("conversation %T does not support pure model context assembly", conversation)
	}
	return assembler.AssembleModelContext(ctx, originalMessage, input)
}

// CommitModelInput is the sole model-input publication boundary. Conversations
// without a canonical user-input store intentionally treat it as a no-op.
func CommitModelInput(ctx context.Context, conversation any, originalMessage string, assembled ModelContextResult) error {
	if conversation == nil {
		return fmt.Errorf("conversation is required")
	}
	if committer, ok := conversation.(ModelInputCommitter); ok {
		return committer.CommitModelInput(ctx, originalMessage, assembled)
	}
	return nil
}
