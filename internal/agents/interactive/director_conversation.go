package interactive

import (
	"context"
	"sync"

	"denova/internal/agents/conversation"
	"denova/internal/agents/run"
	"denova/internal/agents/session"
)

// DirectorConversationOptions binds the reusable single-instruction model
// conversation to optional display and durable domain-commit participants.
type DirectorConversationOptions struct {
	Instruction   conversation.InstructionOptions
	Display       any
	DomainCommit  agentrun.DomainCommitParticipant
	HideToolInput bool
}

// DirectorConversation adapts an interactive Director maintenance task to the
// shared chat runtime without mixing Director display policy into chat itself.
type DirectorConversation struct {
	*conversation.InstructionConversation
	display               any
	domainCommit          agentrun.DomainCommitParticipant
	hideDirectorToolInput bool
	mu                    sync.Mutex
	directorTools         map[string]*directorToolDisplayState
}

// NewDirectorConversation creates a Director-specific conversation adapter.
func NewDirectorConversation(options DirectorConversationOptions) *DirectorConversation {
	return &DirectorConversation{
		InstructionConversation: conversation.NewInstructionConversation(options.Instruction),
		display:                 options.Display,
		domainCommit:            options.DomainCommit,
		hideDirectorToolInput:   options.HideToolInput,
		directorTools:           make(map[string]*directorToolDisplayState),
	}
}

// BindAgentCycleIdentity forwards the durable coordinator identity to the
// App-owned Director commit participant. The model conversation itself never
// invents a domain identity.
func (c *DirectorConversation) BindAgentCycleIdentity(identity agentrun.CycleIdentity) {
	if c == nil || c.domainCommit == nil {
		return
	}
	if binder, ok := c.domainCommit.(agentrun.CycleIdentityBinder); ok {
		binder.BindAgentCycleIdentity(identity)
	}
}

func (c *DirectorConversation) PendingAgentCycleCommit(stage agentrun.DomainCommitStage) (agentrun.DomainCommitIntent, bool, error) {
	if c == nil || c.domainCommit == nil {
		return agentrun.DomainCommitIntent{}, false, nil
	}
	return c.domainCommit.PendingAgentCycleCommit(stage)
}

func (c *DirectorConversation) CommitAgentCycleStage(ctx context.Context, stage agentrun.DomainCommitStage, outcome agentrun.Outcome) error {
	if c == nil || c.domainCommit == nil {
		return nil
	}
	return c.domainCommit.CommitAgentCycleStage(ctx, stage, outcome)
}

func (c *DirectorConversation) LastAgentCycleCommitReceipt(stage agentrun.DomainCommitStage) (agentrun.DomainCommitReceipt, bool) {
	if c == nil || c.domainCommit == nil {
		return agentrun.DomainCommitReceipt{}, false
	}
	return c.domainCommit.LastAgentCycleCommitReceipt(stage)
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
