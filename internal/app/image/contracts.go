// Package image owns application-level image generation for both writing and
// game modes. It shares one workspace lease and one Image Agent implementation
// so asset writes and interactive projections obey the same lifecycle rules.
package imageapp

import (
	"context"
	"errors"
	"fmt"

	"denova/config"
	agentharness "denova/internal/agents/harness"
	"denova/internal/agents/session"
	"denova/internal/book"
	"denova/internal/interactive"
)

var (
	ErrNoWorkspace             = errors.New("no workspace is selected")
	ErrReplayResultUnavailable = errors.New("replayed image Agent result is not yet projected")
	ErrExecution               = errors.New("image Agent execution failed")
)

// Operation pins one workspace generation until Release. Implementations must
// cancel Context when that generation starts draining.
type Operation interface {
	Context() context.Context
	Release()
}

// Runtime is an immutable image-operation snapshot captured atomically by the
// Host. Services must not re-resolve any of these adapters while it is leased.
type Runtime struct {
	Operation    Operation
	Workspace    string
	Config       config.Config
	BookState    *book.State
	BookService  *book.Service
	Interactive  *interactive.Store
	SessionStore *session.Store
	ChatService  *agentharness.Service
}

func (runtime *Runtime) Context() context.Context {
	if runtime == nil || runtime.Operation == nil {
		return context.Background()
	}
	return runtime.Operation.Context()
}

func (runtime *Runtime) Release() {
	if runtime != nil && runtime.Operation != nil {
		runtime.Operation.Release()
	}
}

func (runtime *Runtime) requireAgentAdapters() error {
	if runtime == nil || runtime.BookState == nil || runtime.BookService == nil || runtime.ChatService == nil {
		return fmt.Errorf("image Agent workspace runtime is incomplete")
	}
	return nil
}

// Host owns workspace generation fencing; Service owns all image behavior.
type Host interface {
	AcquireImageRuntime(context.Context, string) (*Runtime, error)
}

type Service struct {
	host Host
}

func NewService(host Host) *Service {
	return &Service{host: host}
}

// AcquireRuntime exposes the shared leased snapshot to adjacent app services,
// such as Lore image generation, without duplicating workspace fencing.
func (service *Service) AcquireRuntime(ctx context.Context, expectedWorkspace string) (*Runtime, error) {
	if service == nil || service.host == nil {
		return nil, ErrNoWorkspace
	}
	return service.host.AcquireImageRuntime(ctx, expectedWorkspace)
}
