package agentrun

import (
	"context"
	"strings"
)

// CycleIdentity is the durable coordinator identity of one accepted model
// cycle. Domain stores persist it with canonical commits for exact replay.
type CycleIdentity struct {
	CommandID   CommandID
	OperationID OperationID
	Cycle       int
}

// ValidCycleIdentity reports whether identity can safely key a canonical
// domain commit.
func ValidCycleIdentity(identity CycleIdentity) bool {
	return strings.TrimSpace(string(identity.CommandID)) != "" &&
		strings.TrimSpace(string(identity.OperationID)) != "" && identity.Cycle > 0
}

// OutcomeMayCommitDomain reports whether a settled model cycle is allowed to
// publish its staged canonical output.
func OutcomeMayCommitDomain(outcome Outcome) bool {
	return outcome.Status == OutcomeCompleted || outcome.Status == OutcomePreempted
}

// CycleIdentityBinder receives the selected durable identity before any model
// or tool effect begins.
type CycleIdentityBinder interface {
	BindAgentCycleIdentity(CycleIdentity)
}

// AgentKindBinder aligns canonical input metadata with the accepted profile.
type AgentKindBinder interface {
	BindHarnessAgentKind(string)
}

// CyclePreparer reconciles domain-owned outboxes after identity binding and
// before model or tool effects.
type CyclePreparer interface {
	PrepareAgentCycle(context.Context) error
}

// CycleCommitter publishes or reconciles a domain projection at the same
// boundary where the durable runtime settles a cycle.
type CycleCommitter interface {
	CommitAgentCycle(context.Context, Outcome) error
}

// DomainCommitIntent is the bounded declaration persisted before a domain may
// publish its staged canonical output.
type DomainCommitIntent struct {
	Identity CycleIdentity
	Stage    DomainCommitStage
	Hash     string
}

// DomainCommitReceipt is the canonical proof returned after publication.
type DomainCommitReceipt struct {
	Identity CycleIdentity
	Stage    DomainCommitStage
	Hash     string
	Revision string
}

// DomainCommitParticipant exposes staged output without coupling the runtime
// to writing- or game-specific stores.
type DomainCommitParticipant interface {
	PendingAgentCycleCommit(DomainCommitStage) (DomainCommitIntent, bool, error)
	CommitAgentCycleStage(context.Context, DomainCommitStage, Outcome) error
	LastAgentCycleCommitReceipt(DomainCommitStage) (DomainCommitReceipt, bool)
}

// InputDomainCommitBinder installs the synchronous authorization callback used
// by CommitModelInput before model execution.
type InputDomainCommitBinder interface {
	BindAgentCycleInputCommit(func() error)
}
