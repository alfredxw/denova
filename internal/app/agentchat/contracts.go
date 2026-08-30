// Package agentchat owns project-scoped Agent conversations. Unlike foreground
// Writing, every operation is bound to an explicit stable Project and Session
// and never changes the currently open Book.
package agentchat

import (
	"context"
	"time"

	"denova/config"
	agents "denova/internal/agents"
	chatagent "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	agenttool "denova/internal/agents/tool"
	conversationapp "denova/internal/app/conversation"
	apptask "denova/internal/app/task"
	"denova/internal/book"
	projectdomain "denova/internal/project"
)

const RuntimeMode = "agent_chat"

// Host supplies the small amount of process-wide policy that cannot be owned
// by one Project runtime. Project identity and session state stay in Service.
type Host interface {
	BaseRuntime() (config.Config, *agentexecution.Runtime)
	ProjectVersionService(string) (*book.VersionService, error)
	CurrentWorkspace() string
	OnVerifiedMutations(context.Context, string, *book.VersionService, config.Config, []agenttool.Mutation, agenttool.Verification)
	ProjectAgentHostCapabilities(context.Context, projectdomain.Type, *config.Config, string) (agents.AgentHostCapabilities, error)
}

// Binding is the explicit Project conversation identity carried by every
// AgentChat operation. Workspace is derived from ProjectID and never accepted
// as caller authority.
type Binding struct {
	ProjectID string `json:"project_id"`
	SessionID string `json:"session_id"`
	// Channel is an optional immutable classification assertion. Runtime keys
	// remain ProjectID + SessionID; omission preserves existing internal callers.
	Channel   session.Channel `json:"channel,omitempty"`
	Workspace string          `json:"-"`

	agentKind string
	stateRoot string
}

// ProjectRuntime is a read-only dependency snapshot for other application
// services that execute against a registered Project, such as Automation.
type ProjectRuntime struct {
	Conversation conversationapp.Runtime
	SessionStore *session.Store
}

type ActiveView struct {
	Task                  *apptask.Snapshot
	Runtime               agentrun.RuntimeStatus
	RuntimeProjectionOK   bool
	StreamAttached        bool
	PendingAsk            *session.AskInteraction
	PendingInterruptionID string
}

type Session struct {
	ID            string          `json:"id"`
	Channel       session.Channel `json:"channel"`
	CustomAgentID string          `json:"custom_agent_id,omitempty"`
	Title         string          `json:"title"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	MessageCount  int             `json:"message_count"`
	// Active is retained by the transport contract. AgentChat has no singleton
	// active session; Running is the exact scoped execution state.
	Active  bool `json:"active"`
	Running bool `json:"running"`
}

type Project struct {
	ID       string               `json:"id"`
	Type     projectdomain.Type   `json:"type"`
	Path     string               `json:"path"`
	Name     string               `json:"name"`
	Status   projectdomain.Status `json:"status"`
	Current  bool                 `json:"current"`
	Total    int                  `json:"total"`
	Sessions []Session            `json:"sessions"`
	Error    string               `json:"error,omitempty"`
}

type HistoryItem struct {
	ProjectID   string  `json:"project_id"`
	ProjectName string  `json:"project_name"`
	Session     Session `json:"session"`
}

type HistoryPage struct {
	Items   []HistoryItem `json:"items"`
	Total   int           `json:"total"`
	Offset  int           `json:"offset"`
	HasMore bool          `json:"has_more"`
}

type HistoryQuery struct {
	ProjectID string
	Channel   session.Channel
	Search    string
	Offset    int
	Limit     int
}

type ChatRequest = chatagent.ChatRequest

const TurnOriginAutomation = "automation"

type TurnBusyPolicy string

const (
	// TurnBusyReject preserves interactive AgentChat's immediate feedback when
	// another operation already owns the conversation.
	TurnBusyReject TurnBusyPolicy = "reject"
	// TurnBusyWait serializes durable background turns behind the current
	// conversation operation without imposing an LLM timeout.
	TurnBusyWait TurnBusyPolicy = "wait"
)

// TurnPolicy narrows one project-Agent turn without creating another Agent
// kind. DisabledCapabilities is an invocation ceiling: it can remove project
// capabilities but can never enable a capability disabled by project settings.
type TurnPolicy struct {
	Origin               string
	OriginID             string
	TraceID              string
	SessionTitle         string
	ModelProfileID       string
	BusyPolicy           TurnBusyPolicy
	DisabledCapabilities []string
}

// TurnRequest is the complete admission input shared by interactive AgentChat
// and background project automation. Task is optional; callers that already
// own a reconnectable display task may supply it so one task identity spans
// automation accounting and AgentChat execution.
type TurnRequest struct {
	Binding Binding
	ChatRequest
	Task   *apptask.Task
	Policy TurnPolicy
	Emit   func(agentrun.Event)
}
