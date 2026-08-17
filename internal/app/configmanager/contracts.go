// Package configmanager owns resource-configuration conversations, their
// scoped session identity, and reconnectable in-process display tasks.
package configmanager

import (
	"context"

	"denova/config"
	agentexecution "denova/internal/agents/execution"
	"denova/internal/agents/session"
	agenttool "denova/internal/agents/tool"
	apptask "denova/internal/app/task"
	"denova/internal/book"
	projectdomain "denova/internal/project"
)

const RuntimeMode = "config_manager"

type Request struct {
	ProjectID   string            `json:"-"`
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

// Runtime is an immutable explicit-Project dependency snapshot. Config Manager
// pages can therefore run inside AgentChat without changing the foreground
// Writing Book.
type Runtime struct {
	ProjectID        string
	Config           config.Config
	Workspace        string
	State            *book.State
	SessionStore     *session.Store
	BookService      *book.Service
	VersionService   *book.VersionService
	ExecutionRuntime *agentexecution.Runtime
	ProjectRegistry  *projectdomain.Registry
}

type Operation interface {
	Context() context.Context
	Release()
}

// Host centralizes Project runtime resolution and lifecycle fencing. Service
// owns Config Manager policy; Host never consults foreground navigation state.
type Host interface {
	ProjectRuntime(context.Context, string) (Runtime, error)
	AcquireProjectOperation(context.Context, string) (Operation, error)
	IsCurrent(Runtime) bool
	RegisterTask(*apptask.Task, Runtime) error
	UnregisterTask(*apptask.Task)
	OnVerifiedMutations(context.Context, string, *book.VersionService, config.Config, []agenttool.Mutation, agenttool.Verification)
}
