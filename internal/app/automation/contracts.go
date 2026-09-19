package automationapp

import (
	"context"

	"denova/config"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	appagentruntime "denova/internal/app/agentruntime"
	apptask "denova/internal/app/task"
	"denova/internal/automation"
	"denova/internal/book"
	projectdomain "denova/internal/project"
)

var (
	// ErrNoWorkspace reports that an operation requires a selected workspace.
	ErrNoWorkspace = appagentruntime.ErrNoWorkspace
	// ErrOperationActive leaves delivery pending while the Project Agent is busy.
	ErrOperationActive = appagentruntime.ErrOperationActive
	// ErrCommandIDRequired rejects commands that cannot be replayed safely.
	ErrCommandIDRequired = apptask.ErrCommandIDRequired
	// ErrCommandConflict reports reuse of a command identity for another intent.
	ErrCommandConflict = apptask.ErrCommandConflict
	// ErrReplayCapacity reports that all bounded display replay slots are live.
	ErrReplayCapacity = apptask.ErrReplayCapacity
)

// Operation keeps one root or workspace runtime generation alive while a
// synchronous action is being admitted. Callers must always release it.
type Operation interface {
	Context() context.Context
	Release()
}

// Runtime is an immutable view used for trigger delivery and result projection.
// A Host captures all fields atomically before returning it.
type Runtime struct {
	ProjectID        string
	ProjectType      projectdomain.Type
	StateRoot        string
	Workspace        string
	DataDir          string
	Config           config.Config
	BookState        *book.State
	BookService      *book.Service
	SessionStore     *session.Store
	ExecutionRuntime *agentexecution.Runtime
}

// Catalog describes every project-backed automation store visible to the
// current user without exposing the application project registry itself.
type Catalog struct {
	DataDir          string
	CurrentWorkspace string
	Projects         []automation.ProjectLocation
}

// ProjectConversationTurn is the complete automation-owned intent admitted by
// the target Project's AgentChat runtime. Automation supplies scheduling and
// run identity; AgentChat owns conversation creation, capabilities, and execution.
type ProjectConversationTurn struct {
	ProjectID        string
	SessionID        string
	CommandID        string
	Message          string
	AutomationTaskID string
	RunID            string
	SessionTitle     string
	ModelProfileID   string
}

// ProjectConversationExecution is a durable admission handle. Start delegates
// execution and lifecycle ownership to the common Project Agent service.
type ProjectConversationExecution interface {
	Receipt() agentrun.CommandReceipt
	Start() error
	Task() *apptask.Task
}

// Host is the narrow process boundary used by automation. It owns workspace
// generations and task admission; Service owns all automation state.
type Host interface {
	CurrentWorkspace() string
	CurrentRuntime() (Runtime, error)
	BaseRuntime() Runtime
	ResolveTarget(automation.ExecutionTarget) (automation.ExecutionTarget, error)
	RuntimeForTarget(context.Context, automation.ExecutionTarget) (Runtime, error)
	Catalog() (Catalog, error)
	AcquireRootOperation(context.Context) (Operation, error)
	AcquireProjectOperation(context.Context, string) (Operation, error)
	AcquireWorkspaceOperation(context.Context, string) (Operation, error)
	AcceptProjectConversationTurn(context.Context, ProjectConversationTurn) (ProjectConversationExecution, error)
}

func snapshotFromRuntime(runtime Runtime) *automationWorkspaceSnapshot {
	return &automationWorkspaceSnapshot{
		projectID:        runtime.ProjectID,
		projectType:      runtime.ProjectType,
		stateRoot:        runtime.StateRoot,
		workspace:        runtime.Workspace,
		novaDir:          runtime.DataDir,
		cfg:              runtime.Config,
		bookState:        runtime.BookState,
		bookService:      runtime.BookService,
		sessionStore:     runtime.SessionStore,
		executionRuntime: runtime.ExecutionRuntime,
	}
}
