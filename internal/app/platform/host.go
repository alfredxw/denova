// Package platform adapts native application services to the extension platform.
// It owns public projections and protocol translation, not App lifecycle locks,
// runtime selection, or the native Story execution and commit pipeline.
package platform

import (
	"context"

	"denova/config"
	agentrun "denova/internal/agents/run"
	apptask "denova/internal/app/task"
	booklore "denova/internal/book/lore"
	"denova/internal/interactive"
	"denova/internal/project"
)

// Lease pins a foreground workspace until Release. ProjectLease additionally
// pins an immutable Project layout and may address a background Project.
type Lease interface {
	Context() context.Context
	Release()
}
type ProjectLease interface {
	Lease
	Layout() project.Layout
}

// ResourceHost supplies native configuration and leased access. Config
// snapshots must be detached from mutable App state; credentials stay in-process.
type ResourceHost interface {
	AcquireProject(context.Context, string) (ProjectLease, error)
	WithLoreStore(context.Context, string, func(*booklore.Store) error) (string, error)
	ImageConfigSnapshot() config.Config
}

// StoryActivity pairs the observed runtime with its matching live task. A nil
// Task means the observed operation has no safe process-local replay source.
type StoryActivity struct {
	Runtime               agentrun.RuntimeStatus
	RuntimeProjectionOK   bool
	PendingInterruptionID string
	Task                  *apptask.Task
}

// StartRequest contains only the native input required by extension commands.
// The host retains native admission, command identity and runtime dispatch.
type StartRequest struct {
	CommandID, StoryID, BranchID, Message, ResumeInterruptionID, RegenerateFromTurnID, Locale string
	InputVisibility                                                                           agentrun.InputVisibility
}

// SuspendRequest targets an observed native operation, never an unrelated task.
type SuspendRequest struct {
	CommandID, StoryID, BranchID, Reason string
	OperationID                          agentrun.OperationID
}

// Opening is valid only inside WithStoryOpening. The host holds native admission
// throughout the callback; Commit drains and validates the native binding fence.
type Opening interface {
	Store() *interactive.Store
	ResolveProtagonist(context.Context, interactive.StoryProtagonist) (interactive.StoryProtagonist, error)
	PrepareUpdate(interactive.UpdateStoryRequest) (interactive.UpdateStoryRequest, error)
	Commit(context.Context, string, interactive.UpdateStoryRequest) error
}

// StoryHost is the native boundary. Mutations require an AcquireStory lease;
// OpenStoryStore supports inactive Projects without changing foreground state.
// WithStoryOpening must reject active tasks and serialize against native starts.
type StoryHost interface {
	AcquireStory(context.Context, string) (Lease, error)
	OpenStoryStore(context.Context, string) (*StoryStore, error)
	CurrentProjectID() string
	DataDir() string
	Projects() ([]project.Record, error)
	StoryActivity(context.Context, string, string) StoryActivity
	StartStory(context.Context, StartRequest) error
	SuspendStory(context.Context, SuspendRequest) error
	WithStoryOpening(context.Context, string, func(Opening) error) error
	InteractiveSnapshot(string, string) (interactive.Snapshot, error)
	CreateInteractiveStoryContext(context.Context, interactive.CreateStoryRequest) (interactive.StorySummary, error)
	UpdateInteractiveStory(string, interactive.UpdateStoryRequest) (interactive.StorySummary, error)
	CreateInteractiveBranch(string, interactive.CreateBranchRequest) (interactive.BranchSummary, error)
	SwitchInteractiveBranch(string, string) error
	SwitchInteractiveTurnVersion(string, interactive.SwitchTurnVersionRequest) error
}

// StoryStore owns a Project lease; Close releases it and closes only stores
// opened for background access. Foreground stores remain owned by App.
type StoryStore struct {
	*interactive.Store
	Operation ProjectLease
	OwnsStore bool
}

func (store *StoryStore) Close() {
	if store.OwnsStore {
		_ = store.Store.Close()
	}
	store.Operation.Release()
}

// Stories implements the extension Story API using leased native services.
type Stories struct{ host StoryHost }

func NewStories(host StoryHost) *Stories { return &Stories{host: host} }

// Resources exposes native library and image services without accepting host
// paths or model credentials from extensions.
type Resources struct{ host ResourceHost }

func NewResources(host ResourceHost) *Resources { return &Resources{host: host} }
