// Package configmanager owns resource-configuration conversations, their
// scoped session identity, reconnectable display tasks, and cold recovery.
package configmanager

import (
	"context"

	"denova/config"
	agentconversation "denova/internal/agents/conversation"
	agentharness "denova/internal/agents/harness"
	"denova/internal/agents/session"
	agenttool "denova/internal/agents/tool"
	apptask "denova/internal/app/task"
	"denova/internal/book"
	projectdomain "denova/internal/project"
)

const RuntimeMode = "config_manager"

type Request struct {
	CommandID   string            `json:"command_id"`
	Instruction string            `json:"instruction"`
	Origin      string            `json:"origin,omitempty"`
	ResourceID  string            `json:"resource_id,omitempty"`
	StoryID     string            `json:"story_id,omitempty"`
	BranchID    string            `json:"branch_id,omitempty"`
	References  []string          `json:"references,omitempty"`
	Context     map[string]string `json:"context,omitempty"`
	Locale      string            `json:"-"`
}

// Runtime is an immutable foreground-workspace dependency snapshot captured
// under the root App lock before admission begins.
type Runtime struct {
	Config          config.Config
	Workspace       string
	State           *book.State
	SessionStore    *session.Store
	BookService     *book.Service
	VersionService  *book.VersionService
	ChatService     *agentharness.Service
	ProjectRegistry *projectdomain.Registry
}

type Operation interface {
	Context() context.Context
	Release()
}

// Host centralizes the foreground workspace generation fence. Service owns
// Config Manager policy; Host only validates and registers captured resources.
type Host interface {
	Snapshot() Runtime
	ResolveAsk(context.Context, *session.Session, string, string, string, string, []agentconversation.HostAskAnswer, string) (agentconversation.HostAskResolution, error)
	AcquireWorkspaceOperation(context.Context, string) (Operation, error)
	IsCurrent(Runtime) bool
	RegisterTask(*apptask.Task, Runtime) error
	UnregisterTask(*apptask.Task)
	OnVerifiedMutations(context.Context, string, *book.VersionService, config.Config, []agenttool.Mutation, agenttool.Verification)
}
