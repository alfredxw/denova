package app

import (
	"context"

	"denova/config"
	agentexecution "denova/internal/agents/execution"
	platformapp "denova/internal/app/platform"
	booklore "denova/internal/book/lore"
	"denova/internal/interactive"
	"denova/internal/platform"
	"denova/internal/project"
)

// platformHost keeps App locks, leases and native Story admission in App.
// Public extension projections and protocol adapters live in app/platform.
type platformHost struct{ *App }

func platformHostError(code, diagnostic string) error {
	return &platform.Error{Code: code, MessageKey: "platform.errors." + code, Diagnostic: diagnostic}
}
func (h platformHost) AcquireProject(ctx context.Context, id string) (platformapp.ProjectLease, error) {
	return h.AcquireProjectOperation(ctx, id)
}
func (h platformHost) WithLoreStore(ctx context.Context, id string, action func(*booklore.Store) error) (string, error) {
	return (loreHost{app: h.App}).WithLoreStore(ctx, id, action)
}
func (h platformHost) ImageConfigSnapshot() config.Config {
	return (imageHost{app: h.App}).ImageConfigSnapshot()
}
func (h platformHost) ModelConfigSnapshot() config.Config {
	return (modelHost{app: h.App}).ModelConfigSnapshot()
}
func (h platformHost) CurrentProjectID() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.cfg == nil {
		return ""
	}
	return h.cfg.ProjectID
}
func (h platformHost) DataDir() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg.DataDir()
}
func (h platformHost) Projects() ([]project.Record, error) { return h.projectRegistry.List(false) }
func (h platformHost) StoryActivity(ctx context.Context, storyID, branchID string) platformapp.StoryActivity {
	view := h.InteractiveAgentActiveView(ctx, storyID, branchID)
	task, info := h.ActiveInteractiveTaskFor(storyID, view.Runtime.Binding.BranchID)
	if view.Task == nil || info.TaskID != view.Task.ID {
		task = nil
	}
	return platformapp.StoryActivity{Runtime: view.Runtime, RuntimeProjectionOK: view.RuntimeProjectionOK, PendingInterruptionID: view.PendingInterruptionID, Task: task}
}
func (h platformHost) StartStory(ctx context.Context, request platformapp.StartRequest) error {
	_, err := h.StartInteractiveTaskWithError(ctx, InteractiveAgentStartRequest{
		CommandID: request.CommandID, StoryID: request.StoryID, BranchID: request.BranchID,
		Message: request.Message, ResumeInterruptionID: request.ResumeInterruptionID,
		RegenerateFromTurnID: request.RegenerateFromTurnID, Locale: request.Locale, InputVisibility: request.InputVisibility,
	})
	return err
}
func (h platformHost) SuspendStory(ctx context.Context, request platformapp.SuspendRequest) error {
	_, err := h.SubmitInteractiveAgentCommand(ctx, InteractiveAgentCommand{Kind: agentexecution.CommandSuspend,
		CommandID: request.CommandID, OperationID: request.OperationID, StoryID: request.StoryID, BranchID: request.BranchID, Reason: request.Reason,
	})
	return err
}

// WithStoryOpening shares native admission with Story starts. The callback may
// prepare a portable opening but only Commit may mutate the canonical journal.
func (h platformHost) WithStoryOpening(ctx context.Context, storyID string, action func(platformapp.Opening) error) error {
	service := h.interactiveService()
	service.admission.Lock()
	defer service.admission.Unlock()
	store := service.store()
	if store == nil {
		return ErrNoWorkspace
	}
	h.mu.RLock()
	active := interactiveTaskForScopeLocked(h.App, h.workspace, storyID, "")
	h.mu.RUnlock()
	if active != nil {
		return platformHostError("DOCUMENT_CONFLICT", "Stop the active Story operation before configuring its opening")
	}
	return action(platformOpening{service: service, store: store})
}

type platformOpening struct {
	service *InteractiveAppService
	store   *interactive.Store
}

func (o platformOpening) Store() *interactive.Store { return o.store }
func (o platformOpening) ResolveProtagonist(ctx context.Context, input interactive.StoryProtagonist) (interactive.StoryProtagonist, error) {
	return o.service.resolveStoryProtagonist(ctx, input)
}
func (o platformOpening) PrepareUpdate(input interactive.UpdateStoryRequest) (interactive.UpdateStoryRequest, error) {
	return o.service.withStoryStateSchemaUpdateDefaults(input)
}
func (o platformOpening) Commit(ctx context.Context, storyID string, request interactive.UpdateStoryRequest) error {
	fence, err := o.service.drainInteractiveBinding(ctx, storyID, "")
	if err != nil {
		return err
	}
	a := o.service.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := fence.validateLocked(a); err != nil {
		return err
	}
	_, err = o.store.UpdateStory(storyID, request)
	return err
}
func (h platformHost) AcquireStory(ctx context.Context, projectID string) (platformapp.Lease, error) {
	a := h.App
	a.mu.RLock()
	workspace := a.workspace
	current := ""
	if a.cfg != nil {
		current = a.cfg.ProjectID
	}
	a.mu.RUnlock()
	if projectID == "" || current != projectID {
		return nil, platformHostError("NOT_CONFIGURED", "Open the bound Project before running its Story")
	}
	operation, err := a.acquireWorkspaceOperation(ctx, workspace, true)
	if err != nil {
		return nil, err
	}
	a.mu.RLock()
	valid := a.cfg != nil && a.cfg.ProjectID == projectID
	a.mu.RUnlock()
	if !valid {
		operation.Release()
		return nil, ErrWorkspaceChanged
	}
	return operation, nil
}

func (h platformHost) OpenStoryStore(ctx context.Context, projectID string) (*platformapp.StoryStore, error) {
	operation, err := h.App.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return nil, err
	}
	h.App.mu.RLock()
	var current *interactive.Store
	if h.App.cfg != nil && h.App.cfg.ProjectID == projectID {
		current = h.App.interactive
	}
	h.App.mu.RUnlock()
	if current != nil {
		return &platformapp.StoryStore{Store: current, Operation: operation}, nil
	}
	return &platformapp.StoryStore{Store: interactive.NewStore(operation.Layout().ContentRoot), Operation: operation, OwnsStore: true}, nil
}

func (a *App) Platform() *platform.Manager { return a.platform }
