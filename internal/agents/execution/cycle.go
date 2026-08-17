package execution

import (
	"context"
	"errors"

	agentchat "denova/internal/agents/chat"
	agentrun "denova/internal/agents/run"
	"denova/internal/book"

	agent "github.com/alfredxw/denova/agent"
)

// ErrCyclePreparationUnavailable means a durable public Agent turn cannot be
// rebound to its Denova product dependencies in the current process.
var ErrCyclePreparationUnavailable = errors.New("Agent execution cycle preparation dependency is unavailable")

// Cycle is the process-local Denova composition for one public Agent cycle.
// Durable recovery stores only stable Definition/host data and rebuilds these
// dependencies through the selected Profile.
type Cycle struct {
	Definition   agent.Definition
	Conversation agentchat.Conversation
	BookService  *book.Service
	Request      agentchat.ChatRequest
	Options      agentrun.Options
	Successor    SuccessorPolicy
}

// CanonicalTranscriptSynchronizer is the narrow product-history bootstrap
// seam used before public Run admission. It must be provider-free and return a
// complete canonical raw transcript for this exact durable Session lane.
// Session.SyncTranscript owns idle fencing, provenance CAS, validation, and
// structural rebuild semantics.
type CanonicalTranscriptSynchronizer interface {
	CanonicalTranscript(context.Context) (agent.TranscriptSyncRequest, error)
}

type SuccessorPolicy func(context.Context, agentrun.OperationID, agentrun.Outcome) error

type CycleRestoreRequest struct {
	Binding          agentrun.RuntimeBinding
	Kind             CommandKind
	CommandID        agentrun.CommandID
	OperationID      agentrun.OperationID
	AfterOperationID agentrun.OperationID
	Request          agentchat.ChatRequest
	Options          agentrun.Options
	Deferred         bool
	Emit             func(agentrun.Event)
}

func emitExecutionError(emit func(agentrun.Event), err error) {
	if emit != nil && err != nil {
		emit(agentrun.Event{Type: "error", Data: map[string]string{"message": err.Error()}})
	}
}
