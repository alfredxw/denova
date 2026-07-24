package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	agents "denova/internal/agents"
)

const maxRememberedInteractiveStarts = 128

// InteractiveAgentStartRequest is the caller-owned identity and bounded game
// payload for one root operation. BranchID is resolved before fingerprinting.
type InteractiveAgentStartRequest struct {
	CommandID            string
	StoryID              string
	BranchID             string
	Message              string
	StyleScenes          []string
	RegenerateFromTurnID string
	Locale               string
}

type interactiveStartIdentity struct {
	request     InteractiveAgentStartRequest
	workspace   string
	fingerprint string
	chatRequest agents.ChatRequest
}

type interactiveStartRecord struct {
	commandID   string
	fingerprint string
	task        *Task
}

// interactiveStartRegistry is a bounded process-local display replay index.
// The durable game binding remains authoritative after eviction or restart.
type interactiveStartRegistry struct {
	mu              sync.Mutex
	records         map[string]interactiveStartRecord
	order           []string
	replayByteLimit int
}

func (r *interactiveStartRegistry) replay(identity interactiveStartIdentity) (*Task, bool, error) {
	commandID := strings.TrimSpace(identity.request.CommandID)
	if commandID == "" {
		return nil, false, ErrAgentCommandIDRequired
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.records[commandID]
	if !ok {
		r.pruneLocked()
		return nil, false, nil
	}
	if record.fingerprint != identity.fingerprint {
		r.pruneLocked()
		return nil, false, fmt.Errorf("%w: command_id=%q", ErrAgentCommandConflict, commandID)
	}
	r.order = touchTaskReplayKey(r.order, commandID)
	r.pruneLocked()
	record = r.records[commandID]
	if record.task == nil {
		return nil, false, nil
	}
	return record.task, true, nil
}

func (r *interactiveStartRegistry) remember(identity interactiveStartIdentity, task *Task) error {
	commandID := strings.TrimSpace(identity.request.CommandID)
	if commandID == "" {
		return ErrAgentCommandIDRequired
	}
	if task == nil {
		return fmt.Errorf("cannot remember a nil Game task")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.records == nil {
		r.records = make(map[string]interactiveStartRecord)
	}
	if existing, ok := r.records[commandID]; ok {
		if existing.fingerprint != identity.fingerprint || (existing.task != nil && existing.task != task) {
			return fmt.Errorf("%w: command_id=%q", ErrAgentCommandConflict, commandID)
		}
		if existing.task == nil {
			existing.task = task
			r.records[commandID] = existing
		}
		r.order = touchTaskReplayKey(r.order, commandID)
		r.pruneLocked()
		return nil
	}
	r.records[commandID] = interactiveStartRecord{
		commandID: commandID, fingerprint: identity.fingerprint, task: task,
	}
	r.order = touchTaskReplayKey(r.order, commandID)
	r.pruneLocked()
	return nil
}

func (r *interactiveStartRegistry) pruneLocked() {
	for len(r.records) > maxRememberedInteractiveStarts {
		removed := false
		for index, commandID := range r.order {
			record, ok := r.records[commandID]
			if !ok {
				r.order = removeTaskReplayKey(r.order, index)
				removed = true
				break
			}
			if record.task != nil && !record.task.Finished() {
				continue
			}
			taskID := ""
			released := 0
			if record.task != nil {
				taskID = record.task.ID()
				released = record.task.releaseDisplayReplay()
			}
			delete(r.records, commandID)
			r.order = removeTaskReplayKey(r.order, index)
			log.Printf("[interactive-agent-task] pruned settled Game replay identity command_id=%s task_id=%s released_bytes=%d max_records=%d", commandID, taskID, released, maxRememberedInteractiveStarts)
			removed = true
			break
		}
		if !removed {
			break
		}
	}

	totalBytes := 0
	for _, record := range r.records {
		totalBytes += record.task.displayReplayRegistryCharge()
	}
	byteLimit := effectiveTaskRegistryReplayByteLimit(r.replayByteLimit)
	for _, commandID := range r.order {
		if totalBytes <= byteLimit {
			break
		}
		record, ok := r.records[commandID]
		if !ok || record.task == nil || !record.task.Finished() {
			continue
		}
		taskID := record.task.ID()
		released := record.task.releaseDisplayReplay()
		totalBytes -= released
		record.task = nil
		r.records[commandID] = record
		log.Printf("[interactive-agent-task] evicted settled Game display replay command_id=%s task_id=%s released_bytes=%d retained_bytes=%d budget_bytes=%d", commandID, taskID, released, totalBytes, byteLimit)
	}
}

func (s *InteractiveAppService) resolveInteractiveStart(request InteractiveAgentStartRequest) (interactiveStartIdentity, error) {
	request.CommandID = strings.TrimSpace(request.CommandID)
	request.StoryID = strings.TrimSpace(request.StoryID)
	request.BranchID = strings.TrimSpace(request.BranchID)
	request.Message = strings.TrimSpace(request.Message)
	request.RegenerateFromTurnID = strings.TrimSpace(request.RegenerateFromTurnID)
	request.Locale = strings.TrimSpace(request.Locale)
	request.StyleScenes = normalizeInteractiveStartStyleScenes(request.StyleScenes)
	if request.CommandID == "" {
		return interactiveStartIdentity{}, ErrAgentCommandIDRequired
	}
	if err := agents.ValidateCommandID(request.CommandID); err != nil {
		return interactiveStartIdentity{}, err
	}
	if request.StoryID == "" || request.Message == "" {
		return interactiveStartIdentity{}, fmt.Errorf("interactive story and message are required")
	}
	if s == nil || s.app == nil {
		return interactiveStartIdentity{}, ErrNoWorkspace
	}
	a := s.app
	a.mu.RLock()
	workspace := strings.TrimSpace(a.workspace)
	store := a.interactive
	a.mu.RUnlock()
	if workspace == "" || store == nil {
		return interactiveStartIdentity{}, ErrNoWorkspace
	}
	branchID, err := resolveInteractiveProjectionBranch(store, request.StoryID, request.BranchID)
	if err != nil {
		return interactiveStartIdentity{}, err
	}
	request.BranchID = branchID
	chatRequest := agents.CaptureChatRequestCallerInput(agents.ChatRequest{
		CommandID: request.CommandID, Message: request.Message,
		StyleScenes: append([]string(nil), request.StyleScenes...), Locale: request.Locale,
	})
	descriptor := struct {
		Workspace            string `json:"workspace"`
		StoryID              string `json:"story_id"`
		BranchID             string `json:"branch_id"`
		RegenerateFromTurnID string `json:"regenerate_from_turn_id"`
		Request              string `json:"request"`
	}{
		Workspace: workspace, StoryID: request.StoryID, BranchID: request.BranchID,
		RegenerateFromTurnID: request.RegenerateFromTurnID,
		Request:              agents.ChatRequestSemanticFingerprint(chatRequest),
	}
	encoded, _ := json.Marshal(descriptor)
	sum := sha256.Sum256(encoded)
	return interactiveStartIdentity{
		request: request, workspace: workspace,
		fingerprint: hex.EncodeToString(sum[:]), chatRequest: chatRequest,
	}, nil
}

func normalizeInteractiveStartStyleScenes(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (identity interactiveStartIdentity) options(taskID string) agents.RunOptions {
	return agents.RunOptions{
		AgentKind: agents.AgentKindInteractiveStory, TaskID: strings.TrimSpace(taskID),
		StoryID: identity.request.StoryID, BranchID: identity.request.BranchID,
		TurnID:    identity.request.RegenerateFromTurnID,
		Workspace: identity.workspace, Mode: "interactive",
	}
}

func (identity interactiveStartIdentity) taskInfo(taskID string) InteractiveTaskInfo {
	return InteractiveTaskInfo{
		TaskID: strings.TrimSpace(taskID), CommandID: identity.request.CommandID,
		Workspace: identity.workspace, StoryID: identity.request.StoryID,
		BranchID: identity.request.BranchID, Message: identity.request.Message,
		RegenerateFromTurnID: identity.request.RegenerateFromTurnID,
	}
}

func (s *InteractiveAppService) replayDurableInteractiveStart(
	ctx context.Context,
	identity interactiveStartIdentity,
) (*Task, bool, error) {
	a := s.app
	a.mu.RLock()
	chatService := a.chatService
	bookService := a.bookService
	store := a.interactive
	a.mu.RUnlock()
	if chatService == nil || store == nil {
		return nil, false, nil
	}
	status, err := chatService.RuntimeStatusProjection(ctx, identity.options(""))
	if err != nil {
		return nil, false, err
	}
	if !interactiveStatusOwnsCommand(status, identity.request.CommandID) {
		return nil, false, nil
	}

	var accepted *agents.AcceptedRun
	task, err := NewDeferredRegisteredTask(func(task *Task) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.workspaceTransition {
			return ErrWorkspaceTransition
		}
		if a.workspace != identity.workspace || a.interactive != store || a.chatService != chatService {
			return ErrAgentContextChanged
		}
		if a.activeInteractiveRun != nil && a.activeInteractiveRun.task != nil && !a.activeInteractiveRun.task.Finished() {
			return ErrAgentOperationActive
		}
		if err := a.registerWorkspaceTaskLocked(task, identity.workspace, true); err != nil {
			return err
		}
		a.activeInteractiveRun = &interactiveTaskRun{task: task, info: identity.taskInfo(task.ID())}
		return nil
	})
	if err != nil {
		return nil, true, err
	}
	acceptCtx, releaseAcceptance := taskAcceptanceContext(ctx, task)
	accepted, err = chatService.StartWithOptions(
		acceptCtx, nil, nil, bookService, identity.chatRequest, identity.options(task.ID()), task.emit,
	)
	releaseAcceptance()
	if err != nil {
		rollbackInteractiveReplayTask(a, task, err)
		if errors.Is(err, agents.ErrInvalidCommand) {
			return nil, true, fmt.Errorf("%w: command_id=%q", ErrAgentCommandConflict, identity.request.CommandID)
		}
		return nil, true, err
	}
	if !accepted.Receipt().Replayed {
		err := fmt.Errorf("durable Game replay unexpectedly accepted a new command")
		task.Abort()
		_ = accepted.Wait(task.ctx)
		rollbackInteractiveReplayTask(a, task, err)
		return nil, true, err
	}
	if err := task.Start(func(ctx context.Context, task *Task, _ func(agents.Event)) {
		defer a.unregisterWorkspaceTask(task)
		outcome := accepted.Wait(ctx)
		log.Printf("[interactive-agent-task] replay end id=%s command_id=%s status=%s", task.ID(), identity.request.CommandID, outcome.Status)
	}); err != nil {
		task.Abort()
		_ = accepted.Wait(task.ctx)
		rollbackInteractiveReplayTask(a, task, err)
		return nil, true, err
	}
	if err := s.starts.remember(identity, task); err != nil {
		return nil, true, err
	}
	return task, true, nil
}

func interactiveStatusOwnsCommand(status agents.RuntimeStatus, commandID string) bool {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return false
	}
	if string(status.ActiveCommandID) == commandID {
		return true
	}
	if status.LastOperation != nil && string(status.LastOperation.CommandID) == commandID {
		return true
	}
	for index := len(status.RecentOperations) - 1; index >= 0; index-- {
		if string(status.RecentOperations[index].CommandID) == commandID {
			return true
		}
	}
	return false
}

func rollbackInteractiveReplayTask(a *App, task *Task, err error) {
	task.failBeforeStart(err)
	a.unregisterWorkspaceTask(task)
	a.mu.Lock()
	if a.activeInteractiveRun != nil && a.activeInteractiveRun.task == task {
		a.activeInteractiveRun = nil
	}
	a.mu.Unlock()
}

// InteractiveTaskInfo identifies the game-mode turn owned by a background
// task. The identity is kept separate from the Task event buffer so reconnect
// requests cannot attach a different story or branch by accident.
type InteractiveTaskInfo struct {
	TaskID               string
	CommandID            string
	Workspace            string
	StoryID              string
	BranchID             string
	Message              string
	RegenerateFromTurnID string
}

type interactiveTaskRun struct {
	task            *Task
	info            InteractiveTaskInfo
	recovery        *agents.RecoveryObservation
	recoveryActions map[string]agents.CommandReceipt
}

func (s *InteractiveAppService) bindActiveInteractiveTask(task *Task, info InteractiveTaskInfo) bool {
	if s == nil || s.app == nil || task == nil {
		return false
	}
	info.TaskID = task.ID()
	info.Workspace = strings.TrimSpace(info.Workspace)
	info.StoryID = strings.TrimSpace(info.StoryID)
	info.BranchID = strings.TrimSpace(info.BranchID)
	info.RegenerateFromTurnID = strings.TrimSpace(info.RegenerateFromTurnID)

	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if info.Workspace == "" || a.workspace != info.Workspace {
		return false
	}
	a.activeInteractiveRun = &interactiveTaskRun{task: task, info: info}
	return true
}

// ActiveInteractiveTaskFor returns the reconnectable task only when the
// current workspace, story, and branch all match the request.
func (a *App) ActiveInteractiveTaskFor(storyID, branchID string) (*Task, InteractiveTaskInfo) {
	return a.interactiveService().ActiveInteractiveTaskFor(storyID, branchID)
}

func (s *InteractiveAppService) ActiveInteractiveTaskFor(storyID, branchID string) (*Task, InteractiveTaskInfo) {
	if s == nil || s.app == nil {
		return nil, InteractiveTaskInfo{}
	}
	storyID = strings.TrimSpace(storyID)
	branchID = strings.TrimSpace(branchID)
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	run := a.activeInteractiveRun
	if run == nil || run.task == nil || run.info.Workspace == "" || run.info.Workspace != a.workspace {
		return nil, InteractiveTaskInfo{}
	}
	if storyID != "" && run.info.StoryID != storyID {
		return nil, InteractiveTaskInfo{}
	}
	if branchID != "" && run.info.BranchID != branchID {
		return nil, InteractiveTaskInfo{}
	}
	return run.task, run.info
}
