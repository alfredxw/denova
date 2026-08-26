// Package continuallearning owns Denova's user-level Harness State management
// workflow. The live directory is authoritative; Git history, optimizer
// execution, trajectory discovery, scheduling, and UI contracts live here.
package continuallearning

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"denova/config"
	"denova/internal/agents/trajectory"
	apptask "denova/internal/app/task"

	agentstate "github.com/alfredxw/denova/agent/state"
)

const (
	TriggerManual    = "manual"
	TriggerScheduled = "scheduled"
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
	Version  *StateVersion `json:"version,omitempty"`
	Revision string        `json:"revision"`
	Changed  bool          `json:"changed"`
}

type Runtime struct {
	Config config.Config
}

type Host interface {
	Runtime() Runtime
	TrajectorySources(context.Context) ([]trajectory.Source, error)
	StartHarnessTurn(context.Context, HarnessTurnRequest) (*apptask.Task, error)
}

type HarnessTurnRequest struct {
	CommandID string
	SessionID string
	Message   string
	Locale    string
}

type Request struct {
	CommandID   string `json:"command_id"`
	Instruction string `json:"instruction,omitempty"`
	// Evidence is nil for autonomous global discovery and non-nil for an
	// explicit selection. An empty explicit scope means "analyze no trajectories"
	// instead of silently broadening back to the whole catalog.
	Evidence []string `json:"evidence"`
	Trigger  string   `json:"trigger,omitempty"`
	Locale   string   `json:"-"`
}

type StateFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Re-export the reusable State validation contract at the application/API
// seam instead of maintaining an identical second representation.
type StateDiagnostic = agentstate.Diagnostic
type StateValidationError = agentstate.ValidationError

type ScriptToolSummary struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Agents      []string        `json:"agents"`
	Enabled     bool            `json:"enabled"`
	Resource    string          `json:"resource"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type StateSnapshot struct {
	Revision    string              `json:"revision"`
	Files       []StateFile         `json:"files"`
	ScriptTools []ScriptToolSummary `json:"script_tools,omitempty"`
	Diagnostics []StateDiagnostic   `json:"diagnostics,omitempty"`
	Source      string              `json:"source"`
}

const (
	StateSourceUser    = "user"
	StateSourceBuiltin = "builtin"
)

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
