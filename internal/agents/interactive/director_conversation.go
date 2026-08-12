package interactive

import (
	"sync"

	"denova/internal/agents/conversation"
	"denova/internal/agents/run"
	"denova/internal/agents/session"
)

// DirectorConversationOptions binds the reusable single-instruction model
// conversation to optional display and durable domain-commit participants.
type DirectorConversationOptions struct {
	Instruction     conversation.InstructionOptions
	Display         any
	CanonicalOutput DirectorCanonicalOutput
}

// DirectorConversation adapts an interactive Director maintenance task to the
// shared chat runtime without mixing Director display policy into chat itself.
type DirectorConversation struct {
	*conversation.InstructionConversation
	display         any
	canonicalOutput DirectorCanonicalOutput
	mu              sync.Mutex
	directorTools   map[string]*directorToolDisplayState
}

// NewDirectorConversation creates a Director-specific conversation adapter.
func NewDirectorConversation(options DirectorConversationOptions) *DirectorConversation {
	return &DirectorConversation{
		InstructionConversation: conversation.NewInstructionConversation(options.Instruction),
		display:                 options.Display,
		canonicalOutput:         options.CanonicalOutput,
		directorTools:           make(map[string]*directorToolDisplayState),
	}
}

// CanonicalOutput returns the product-owned Director plan commit adapter. It
// may be nil only for read-only Session inspection, which never executes the
// canonical commit methods.
func (c *DirectorConversation) CanonicalOutput() DirectorCanonicalOutput {
	if c == nil {
		return nil
	}
	return c.canonicalOutput
}

// BindAgentCycleIdentity forwards the durable coordinator identity to the
// App-owned Director commit participant. The model conversation itself never
// invents a domain identity.
func (c *DirectorConversation) BindAgentCycleIdentity(identity agentrun.CycleIdentity) {
	if c == nil || c.canonicalOutput == nil {
		return
	}
	c.canonicalOutput.BindAgentCycleIdentity(identity)
}

type displayEventAppender interface {
	AppendDisplayEvent(event session.DisplayEvent) error
	UpdateDisplayToolStatus(id, name, status string) error
}

type displayToolArgsAppender interface {
	AppendDisplayToolArgs(id, name, delta string) error
}

type displayToolResultUpdater interface {
	UpdateDisplayToolResult(id, name, status, result string) error
}

type displayEventContentAppender interface {
	AppendDisplayEventContent(id, role, delta string) error
}

type displayEventContentFlusher interface {
	FlushDisplayEventContent(id, role string) error
}

type displayAssistantRunFinalizer interface {
	FinalizeDisplayAssistantRun(runID, finalSegmentID, terminalPhase string) error
}
