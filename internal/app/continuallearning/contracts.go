// Package continuallearning owns Denova's user-level Harness State management
// workflow. The live directory is authoritative; Git history, optimizer
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
	restoreDataVersion = 2
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
	// Evidence is nil for autonomous scheduled discovery and non-nil for an
	// explicit UI selection. An empty selection therefore means "analyze no
	// trajectories" instead of silently broadening back to the whole catalog.
	Evidence []string `json:"evidence"`
	Trigger  string   `json:"trigger,omitempty"`
	Locale   string   `json:"-"`
}

type StateFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type StateSnapshot struct {
	Revision string      `json:"revision"`
	Files    []StateFile `json:"files"`
	Source   string      `json:"source"`
}

const (
	StateSourceUser    = "user"
	StateSourceBuiltin = "builtin"
)

type TrajectorySummary struct {
	URI          string    `json:"uri"`
	Kind         string    `json:"kind"`
	ProjectID    string    `json:"project_id"`
	ProjectName  string    `json:"project_name"`
	ID           string    `json:"id"`
	Title        string    `json:"title,omitempty"`
	AgentKind    string    `json:"agent_kind,omitempty"`
	Status       string    `json:"status,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count,omitempty"`
	EventCount   int       `json:"event_count,omitempty"`
	ToolCalls    int       `json:"tool_calls,omitempty"`
	DurationMS   int64     `json:"duration_ms,omitempty"`
}

type TrajectoryIssue struct {
	ProjectID string `json:"project_id"`
	Message   string `json:"message"`
}

type TrajectoryList struct {
	Since  time.Time           `json:"since"`
	Items  []TrajectorySummary `json:"items"`
	Issues []TrajectoryIssue   `json:"issues,omitempty"`
}

type TrajectoryContent struct {
	URI     string `json:"uri"`
	Kind    string `json:"kind"`
	Content string `json:"content"`
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
