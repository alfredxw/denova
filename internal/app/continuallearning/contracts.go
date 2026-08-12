// Package continuallearning owns Denova's user-level Harness State product
// workflow. Generic State mechanics stay in agent/state; Git history, Agent
// execution, trajectory discovery, scheduling, and UI contracts live here.
package continuallearning

import (
	"context"
	"errors"
	"time"

	"denova/config"
	agentconversation "denova/internal/agents/conversation"
	agentexecution "denova/internal/agents/execution"
	"denova/internal/agents/session"
	"denova/internal/agents/trajectory"

	agentstate "github.com/alfredxw/denova/agent/state"
)

const (
	RuntimeMode        = "harness_optimizer"
	TriggerManual      = "manual"
	TriggerScheduled   = "scheduled"
	userScope          = "user"
	restoreDataType    = "denova.harness_optimizer.request"
	restoreDataVersion = 1
)

type Outcome = trajectory.Outcome

var ErrStateVersionNotFound = errors.New("Harness State version does not exist")

// ErrStateConflict keeps HTTP and UI adapters on the application module
// contract instead of coupling them to the reusable Agent State package.
var ErrStateConflict = agentstate.ErrConflict

// StateVersionID is an application-owned opaque reference into the local
// Harness State history. The reusable Agent packages never interpret it.
type StateVersionID string

type StateVersion struct {
	ID        StateVersionID `json:"id"`
	Revision  string         `json:"revision"`
	Summary   string         `json:"summary"`
	CreatedAt time.Time      `json:"created_at"`
}

type StateVersionDiff struct {
	From  StateVersionID `json:"from"`
	To    StateVersionID `json:"to"`
	Patch string         `json:"patch"`
}

type StateUpdateResult struct {
	Version *StateVersion `json:"version,omitempty"`
	Changed bool          `json:"changed"`
}

type Runtime struct {
	Config    config.Config
	Execution *agentexecution.Runtime
}

type Operation interface {
	Context() context.Context
	Release()
}

type Host interface {
	Runtime() Runtime
	AcquireRootOperation(context.Context) (Operation, error)
	TrajectorySources(context.Context) ([]trajectory.Source, error)
	ResolveAsk(context.Context, *session.Session, string, string, []agentconversation.HostAskAnswer, string) (agentconversation.HostAskResolution, error)
}

type Request struct {
	CommandID   string `json:"command_id"`
	Instruction string `json:"instruction,omitempty"`
	Trigger     string `json:"trigger,omitempty"`
	Locale      string `json:"-"`
}

type StateFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type StateSnapshot struct {
	Revision string      `json:"revision"`
	Files    []StateFile `json:"files"`
}

type StateChange struct {
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Delete  bool   `json:"delete,omitempty"`
}

type StateUpdateRequest struct {
	BaseRevision string        `json:"base_revision"`
	Summary      string        `json:"summary"`
	Changes      []StateChange `json:"changes"`
}
