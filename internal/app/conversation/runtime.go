// Package conversation owns the shared project conversation runtime policy
// used by foreground Writing and independent AgentChat sessions.
package conversationapp

import (
	"denova/config"
	"denova/internal/agents/harness"
	"denova/internal/agents/prompts"
	"denova/internal/agents/session"
	"denova/internal/book"
	projectdomain "denova/internal/project"
)

// Runtime is an immutable project adapter snapshot. Request preparation may
// replace Config and IDETeller in the returned value but never re-resolves host
// state, preserving one project generation throughout admission.
type Runtime struct {
	ProjectID      string
	ProjectType    projectdomain.Type
	ProjectState   string
	AgentKind      string
	Session        *session.Session
	State          *book.State
	BookService    *book.Service
	ChatService    *harness.Service
	Workspace      string
	VersionService *book.VersionService
	Config         config.Config
	IDETeller      prompts.IDEStoryTeller
}
