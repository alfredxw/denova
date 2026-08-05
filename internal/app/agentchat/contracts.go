// Package agentchat owns project-scoped Agent conversations. Unlike foreground
// Writing, every operation is bound to an explicit stable Project and Session
// and never changes the currently open Book.
package agentchat

import (
	"context"
	"time"

	"denova/config"
	chatagent "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentharness "denova/internal/agents/harness"
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
	BaseRuntime() (config.Config, *agentharness.Service)
	ProjectVersionService(string) (*book.VersionService, error)
	CurrentWorkspace() string
	ResolveAsk(context.Context, *session.Session, string, string, string, string, []agentconversation.HostAskAnswer, string) (agentconversation.HostAskResolution, error)
	OnVerifiedMutations(context.Context, string, *book.VersionService, config.Config, []agenttool.Mutation, agenttool.Verification)
}

// Binding is the explicit Project conversation identity carried by every
// AgentChat operation. Workspace is derived from ProjectID and never accepted
// as caller authority.
type Binding struct {
	ProjectID string `json:"project_id"`
	SessionID string `json:"session_id"`
	Workspace string `json:"-"`

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
	Task                *apptask.Snapshot
	Runtime             agentrun.RuntimeStatus
	RuntimeProjectionOK bool
	StreamAttached      bool
	PendingAsk          *session.AskInteraction
}

type Session struct {
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
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
	Search    string
	Offset    int
	Limit     int
}

type ChatRequest = chatagent.ChatRequest
