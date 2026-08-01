// Package structural defines the stable protocol for durable context
// mutations. Conversation domains prepare operations; the Agent harness owns
// their admission, authorization, execution, and recovery.
package structural

import (
	"context"

	"denova/internal/agents/context/compaction"
	agentrun "denova/internal/agents/run"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

// Action is closed because every structural mutation must be understood by
// the durable actor, journal reducer, restore codec, and canonical store.
type Action string

const (
	Compact Action = "compact_context"
	Remove  Action = "remove_compaction"
)

type Identity struct {
	CommandID   agentrun.CommandID
	OperationID agentrun.OperationID
	Cycle       int
}

// Intent is prepared without canonical writes. Commit=false represents a
// policy skip and settles without opening a domain commit barrier.
type Intent struct {
	Hash   string
	Commit bool
	Result Result
}

type Receipt struct {
	Revision string
}

type Result struct {
	Compaction compaction.Result
	Removed    bool
}

// Operation separates expensive/model preparation from the actor-authorized
// compare-and-swap write. Reconcile recognizes an exact deterministic identity
// after an ambiguous write or transport retry.
type Operation interface {
	Prepare(context.Context, Identity, func(agentrun.Event)) (Intent, error)
	Commit(context.Context, Identity, Intent) (Receipt, error)
	Reconcile(context.Context) (Result, Receipt, bool, error)
}

type Spec struct {
	CommandID string
	Action    Action
	Ref       agentrun.ContextCompactionRef
	Options   agentrun.Options
	Emit      func(agentrun.Event)
	Operation Operation
	// RestorePlan is encoded into Ref before durable admission and is the exact
	// bounded mutation used to rebuild Operation after process restart.
	RestorePlan *RestorePlan
}

// RuntimeKind projects a product action into the durable runtime vocabulary.
func RuntimeKind(action Action) runstate.StructuralOperationKind {
	switch action {
	case Compact:
		return runstate.StructuralCompactContext
	case Remove:
		return runstate.StructuralRemoveCompaction
	default:
		return ""
	}
}

// ActionFromRuntimeKind performs the exhaustive reverse projection.
func ActionFromRuntimeKind(kind runstate.StructuralOperationKind) Action {
	switch kind {
	case runstate.StructuralCompactContext:
		return Compact
	case runstate.StructuralRemoveCompaction:
		return Remove
	default:
		return ""
	}
}
