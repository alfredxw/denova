package chat

import (
	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/session"
)

// Conversation is the stable orchestration boundary used by chat and harness.
// Concrete storage-backed implementations belong to the conversation package.
type Conversation interface {
	agentcontext.ModelContextAssembler
	AppendAssistant(content string) error
	MarkInterrupted(userMessage, assistantContent, reason string) error
	PendingInterruption() *session.Interruption
	ResolveInterruption(id string) error
}

// ContextSourceReporter exposes the business context sources assembled for the
// current model input without leaking storage implementation details.
type ContextSourceReporter interface {
	ContextSourceSummary() string
}

type toolArtifactStoreProvider interface {
	ToolArtifactStore() agent.ToolArtifactStore
}

// ContextLedgerReporter reports bounded metadata for assembled domain context.
// Full fragment content must never be persisted in the runtime ledger.
type ContextLedgerReporter interface {
	ContextLedgerParts() []agentcontext.AuditPart
}

// FinalContextLedgerReporter rebuilds audit metadata from the exact messages
// sent after context maintenance without retaining message bodies.
type FinalContextLedgerReporter interface {
	ContextLedgerPartsForMessages(messages []*agent.Message) []agentcontext.AuditPart
}

// InteractiveNarrativeReadinessReporter marks successful hidden TurnResult
// staging so protocol cancellation can settle as a completed game turn.
type InteractiveNarrativeReadinessReporter interface {
	InteractiveNarrativeReady() bool
}
