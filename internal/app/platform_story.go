package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"denova/internal/agents/conversationjournal"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	"denova/internal/interactive"
	"denova/internal/platform"
	"github.com/google/uuid"
)

type platformStoryHost struct{ app *App }

func platformStoryError(code, diagnostic string) error {
	return &platform.Error{Code: code, MessageKey: "platform.errors." + code, Diagnostic: diagnostic}
}

// acquire pins the current Project generation across existing App operations.
// Selecting another Project is an explicit user action, never an extension side
// effect. Reads and record storage can use inactive registered Projects.
func (h platformStoryHost) acquire(ctx context.Context, projectID string) (*appOperation, error) {
	a := h.app
	a.mu.RLock()
	workspace := a.workspace
	current := ""
	if a.cfg != nil {
		current = a.cfg.ProjectID
	}
	a.mu.RUnlock()
	if projectID == "" || current != projectID {
		return nil, platformStoryError("NOT_CONFIGURED", "Open the bound Project before running its Story")
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

func (h platformStoryHost) store(ctx context.Context, projectID string) (*platformStoryStore, error) {
	operation, err := h.app.AcquireProjectOperation(ctx, projectID)
	if err != nil {
		return nil, err
	}
	h.app.mu.RLock()
	var current *interactive.Store
	if h.app.cfg != nil && h.app.cfg.ProjectID == projectID {
		current = h.app.interactive
	}
	h.app.mu.RUnlock()
	if current != nil {
		return &platformStoryStore{Store: current, operation: operation}, nil
	}
	return &platformStoryStore{Store: interactive.NewStore(operation.Layout().ContentRoot), operation: operation, ownsStore: true}, nil
}

type platformStoryStore struct {
	*interactive.Store
	operation *ProjectOperation
	ownsStore bool
}

func (store *platformStoryStore) Close() {
	if store.ownsStore {
		_ = store.Store.Close()
	}
	store.operation.Release()
}
func platformStoryTurns(turns []interactive.TurnEvent) []platform.StoryTurn {
	result := make([]platform.StoryTurn, 0, len(turns))
	for _, turn := range turns {
		item := platform.StoryTurn{ID: turn.ID, Revision: interactive.TurnNarrativeRevision(turn), User: turn.User, Narrative: turn.Narrative, Choices: []string{}, Versions: []string{}}
		if turn.UserContextOnly {
			item.User = ""
		}
		if turn.HotState != nil {
			item.Choices = append(item.Choices, turn.HotState.Choices...)
		}
		for _, version := range turn.Versions {
			item.Versions = append(item.Versions, version.TurnID)
		}
		result = append(result, item)
	}
	return result
}

func (h platformStoryHost) Snapshot(ctx context.Context, scope platform.Scope) (platform.StorySnapshot, error) {
	store, err := h.store(ctx, scope.ProjectID)
	if err != nil {
		return platform.StorySnapshot{}, err
	}
	defer store.Close()
	story, err := store.StoryContext(scope.StoryID, scope.BranchID)
	if err != nil {
		return platform.StorySnapshot{}, err
	}
	snapshot := story.Snapshot
	result := platform.StorySnapshot{StoryID: scope.StoryID, BranchID: snapshot.BranchID, Title: story.Meta.Title, Turns: platformStoryTurns(snapshot.Turns), Branches: []platform.StoryBranch{}, BeforeCursor: snapshot.HistoryBeforeCursor, HasMore: snapshot.HasEarlierTurns, Status: "idle"}
	for _, branch := range snapshot.Graph.Branches {
		result.Branches = append(result.Branches, platform.StoryBranch{ID: branch.ID, Title: branch.Title, Current: branch.Current})
	}
	h.app.mu.RLock()
	current := h.app.cfg != nil && h.app.cfg.ProjectID == scope.ProjectID
	h.app.mu.RUnlock()
	if current {
		view := h.app.InteractiveAgentActiveView(ctx, scope.StoryID, snapshot.BranchID)
		if view.RuntimeProjectionOK {
			result.Status = string(view.Runtime.Phase)
			result.OperationID = string(view.Runtime.ActiveOperation)
		}
		result.InterruptionID = view.PendingInterruptionID
	}
	return result, nil
}

func (h platformStoryHost) History(ctx context.Context, scope platform.Scope, before string, limit int) (platform.StoryHistory, error) {
	store, err := h.store(ctx, scope.ProjectID)
	if err != nil {
		return platform.StoryHistory{}, err
	}
	defer store.Close()
	page, err := store.ReadHistoryPage(scope.StoryID, scope.BranchID, before, limit)
	return platform.StoryHistory{Turns: platformStoryTurns(page.Turns), BeforeCursor: page.BeforeCursor, HasMore: page.HasMore}, err
}

func (h platformStoryHost) Command(ctx context.Context, scope platform.Scope, command platform.StoryCommand) (platform.StorySnapshot, error) {
	operation, err := h.acquire(ctx, scope.ProjectID)
	if err != nil {
		return platform.StorySnapshot{}, err
	}
	defer operation.Release()
	ctx = operation.Context()
	if strings.TrimSpace(command.CommandID) == "" || len(command.CommandID) > 256 {
		return platform.StorySnapshot{}, platformStoryError("INVALID_ARGUMENT", "commandId must contain 1..256 bytes")
	}
	snapshot, err := h.app.InteractiveSnapshot(scope.StoryID, command.BranchID)
	if err != nil {
		return platform.StorySnapshot{}, err
	}
	branch := snapshot.BranchID
	switch command.Kind {
	case platform.StoryAdvance, platform.StoryResume, platform.StoryRegenerate:
		if command.Kind == platform.StoryResume && command.InterruptionID == "" {
			return platform.StorySnapshot{}, platformStoryError("INVALID_ARGUMENT", "resume requires interruptionId")
		}
		if command.Kind == platform.StoryRegenerate && command.TurnID == "" {
			return platform.StorySnapshot{}, platformStoryError("INVALID_ARGUMENT", "regenerate requires turnId")
		}
		request := InteractiveAgentStartRequest{CommandID: "platform-" + scope.InstanceID + "-" + command.CommandID, StoryID: scope.StoryID, BranchID: branch, Message: command.Message, Locale: command.Locale}
		if command.Kind == platform.StoryResume {
			request.ResumeInterruptionID = command.InterruptionID
			if strings.TrimSpace(request.Message) == "" {
				request.Message = "Continue."
			}
		}
		if command.Kind == platform.StoryRegenerate {
			request.RegenerateFromTurnID = command.TurnID
			if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.ID != command.TurnID {
				return platform.StorySnapshot{}, platformStoryError("DOCUMENT_CONFLICT", "Regenerate requires the latest turn; fork from history first")
			}
			if strings.TrimSpace(request.Message) == "" {
				request.Message = snapshot.CurrentTurn.User
				if strings.TrimSpace(request.Message) == "" {
					request.Message = snapshot.CurrentTurn.Narrative
				}
			}
			if snapshot.CurrentTurn.UserContextOnly {
				request.InputVisibility = agentrun.InputModelOnly
			}
		}
		_, err = h.app.StartInteractiveTaskWithError(ctx, request)
	case platform.StoryStop:
		if command.OperationID == "" {
			return platform.StorySnapshot{}, platformStoryError("INVALID_ARGUMENT", "stop requires the observed operationId")
		}
		_, err = h.app.SubmitInteractiveAgentCommand(ctx, InteractiveAgentCommand{Kind: agentexecution.CommandSuspend, CommandID: "platform-" + scope.InstanceID + "-" + command.CommandID, OperationID: agentrun.OperationID(command.OperationID), StoryID: scope.StoryID, BranchID: branch, Reason: "Stopped by the game extension"})
	case platform.StoryFork:
		_, err = h.app.CreateInteractiveBranch(scope.StoryID, interactive.CreateBranchRequest{ParentEventID: command.TurnID, Title: command.Title})
	case platform.StorySwitchBranch:
		if command.BranchID == "" {
			return platform.StorySnapshot{}, platformStoryError("INVALID_ARGUMENT", "switchBranch requires branchId")
		}
		err = h.app.SwitchInteractiveBranch(scope.StoryID, command.BranchID)
	case platform.StorySwitchVersion:
		if command.TurnID == "" || command.VersionTurnID == "" {
			return platform.StorySnapshot{}, platformStoryError("INVALID_ARGUMENT", "switchVersion requires turnId and versionTurnId")
		}
		err = h.app.SwitchInteractiveTurnVersion(scope.StoryID, interactive.SwitchTurnVersionRequest{BranchID: branch, TurnID: command.TurnID, VersionTurnID: command.VersionTurnID})
	default:
		err = platformStoryError("INVALID_ARGUMENT", fmt.Sprintf("Unsupported Story command %q", command.Kind))
	}
	if err != nil {
		return platform.StorySnapshot{}, err
	}
	scope.BranchID = ""
	return h.Snapshot(ctx, scope)
}

// StopRuntime suspends only operations admitted by this exact game instance.
// Closing an extension must never interrupt a task started by another view.
func (h platformStoryHost) StopRuntime(ctx context.Context, scope platform.Scope) error {
	operation, err := h.acquire(ctx, scope.ProjectID)
	if err != nil {
		return nil
	}
	defer operation.Release()
	view := h.app.InteractiveAgentActiveView(ctx, scope.StoryID, "")
	if !view.RuntimeProjectionOK || view.Runtime.Phase == agentrun.RunPhaseIdle || view.Runtime.Phase == agentrun.RunPhaseSuspended || !strings.HasPrefix(string(view.Runtime.ActiveCommandID), "platform-"+scope.InstanceID+"-") {
		return nil
	}
	h.app.mu.RLock()
	task, _ := activeInteractiveTaskLocked(h.app, scope.StoryID, view.Runtime.Binding.BranchID)
	if task != nil && (view.Task == nil || task.ID() != view.Task.ID) {
		task = nil
	}
	h.app.mu.RUnlock()
	_, err = h.app.SubmitInteractiveAgentCommand(ctx, InteractiveAgentCommand{Kind: agentexecution.CommandSuspend, CommandID: "platform-stop-" + uuid.NewString(), OperationID: view.Runtime.ActiveOperation, StoryID: scope.StoryID, BranchID: view.Runtime.Binding.BranchID, Reason: "Game extension runtime stopped"})
	if err != nil {
		return err
	}
	if task != nil {
		select {
		case <-task.Done():
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	slog.InfoContext(ctx, "platform_story_runtime_stopped", "instance", scope.InstanceID, "story", scope.StoryID, "operation", view.Runtime.ActiveOperation)
	return nil
}

func (h platformStoryHost) ReadRecord(ctx context.Context, scope platform.Scope, owner string, request platform.StoryRecordRequest) (platform.StoryRecord, error) {
	store, err := h.store(ctx, scope.ProjectID)
	if err != nil {
		return platform.StoryRecord{}, err
	}
	defer store.Close()
	record, revision, err := store.ExtensionRecord(ctx, scope.StoryID, request.BranchID, interactive.ExtensionRecord{Owner: owner, Key: request.Key, TurnID: request.TurnID, SourceRevision: request.SourceRevision})
	if record.Value == nil {
		record.Value = []byte("null")
	}
	return platform.StoryRecord{Revision: revision, SchemaVersion: record.SchemaVersion, Value: record.Value}, err
}

func (h platformStoryHost) WriteRecord(ctx context.Context, scope platform.Scope, owner string, request platform.StoryRecordRequest) (platform.StoryRecord, error) {
	store, err := h.store(ctx, scope.ProjectID)
	if err != nil {
		return platform.StoryRecord{}, err
	}
	defer store.Close()
	record := interactive.ExtensionRecord{Owner: owner, Key: request.Key, TurnID: request.TurnID, SourceRevision: request.SourceRevision, SchemaVersion: request.SchemaVersion, Value: request.Value}
	revision, err := store.SetExtensionRecord(ctx, scope.StoryID, request.BranchID, request.ExpectedRevision, record)
	if errors.Is(err, conversationjournal.ErrConflict) {
		err = platformStoryError("DOCUMENT_CONFLICT", err.Error())
	}
	if err == nil {
		slog.InfoContext(ctx, "platform_story_record_committed", "story", scope.StoryID, "branch", request.BranchID, "owner", owner, "key", request.Key, "turn", request.TurnID, "revision", revision, "bytes", len(record.Value))
	}
	return platform.StoryRecord{Revision: revision, SchemaVersion: record.SchemaVersion, Value: record.Value}, err
}
