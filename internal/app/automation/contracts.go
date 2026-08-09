package automationapp

import (
	"context"
	"errors"

	"denova/config"
	agentharness "denova/internal/agents/harness"
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
	// ErrOperationActive rejects a second root operation for the same run.
	ErrOperationActive = errors.New("agent operation is already active")
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

// Runtime is an immutable view of the resources required by one automation
// execution. A Host captures all fields atomically before returning it.
type Runtime struct {
	ProjectID    string
	ProjectType  projectdomain.Type
	StateRoot    string
	Workspace    string
	DataDir      string
	Config       config.Config
	BookState    *book.State
	BookService  *book.Service
	SessionStore *session.Store
	ChatService  *agentharness.Service
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
	SessionStrategy  string
}

// ProjectConversationExecution is the durable AgentChat command accepted for
// one automation run. The automation worker owns the single Wait call.
type ProjectConversationExecution interface {
	Receipt() agentrun.CommandReceipt
	Wait(context.Context) agentrun.Outcome
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
	AcceptProjectConversationTurn(context.Context, *apptask.Task, ProjectConversationTurn, func(agentrun.Event)) (ProjectConversationExecution, error)
	RegisterTask(*apptask.Task, string) error
	UnregisterTask(*apptask.Task)
}

func snapshotFromRuntime(runtime Runtime) *automationWorkspaceSnapshot {
	return &automationWorkspaceSnapshot{
		projectID:    runtime.ProjectID,
		projectType:  runtime.ProjectType,
		stateRoot:    runtime.StateRoot,
		workspace:    runtime.Workspace,
		novaDir:      runtime.DataDir,
		cfg:          runtime.Config,
		bookState:    runtime.BookState,
		bookService:  runtime.BookService,
		sessionStore: runtime.SessionStore,
		chatService:  runtime.ChatService,
	}
}
