package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"denova/internal/agent"
	"denova/internal/agentruntime"
)

const maxRememberedWritingStarts = 128

// writingTaskRun binds the reconnectable display task to the exact immutable
// runtime snapshot admitted for its root operation. Typed commands never
// reconstruct a binding from whichever session happens to be selected later.
type writingTaskRun struct {
	task               *Task
	runtime            ideChatRuntime
	recovery           *agent.RecoveryObservation
	recoveryActions    map[string]agentruntime.Receipt
	recoveryStructural bool

	recoveryMutationMu sync.Mutex
	recoveryMutations  []writingRecoveryMutationBatch

	// A recovered structural commit can settle before the long-lived selected
	// Session successfully reloads canonical state. Keep its display Task alive
	// until the exact recovery POST closes this latch.
	recoveryRefreshReady chan struct{}
	recoveryRefreshOnce  sync.Once
}

func (run *writingTaskRun) resolveRecoveryRefresh() {
	if run == nil || run.recoveryRefreshReady == nil {
		return
	}
	run.recoveryRefreshOnce.Do(func() { close(run.recoveryRefreshReady) })
}

func (run *writingTaskRun) waitForRecoveryRefresh(ctx context.Context) bool {
	if run == nil || run.recoveryRefreshReady == nil {
		return true
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-run.recoveryRefreshReady:
		return true
	case <-ctx.Done():
		return false
	}
}

type writingStartRecord struct {
	commandID   string
	workspace   string
	sessionID   string
	fingerprint string
	task        *Task
}

// writingStartRegistry is a bounded process-local display replay index. The
// durable runtime remains the authority after eviction or process restart;
// this index only lets an immediate retry attach to the exact original Task.
type writingStartRegistry struct {
	mu              sync.Mutex
	records         map[string]writingStartRecord
	order           []string
	replayByteLimit int
}

type writingStartReservation struct {
	registry *writingStartRegistry
	record   writingStartRecord
	inserted bool
	rebound  bool
	bound    bool
}

func (r *writingStartRegistry) replay(commandID, workspace, sessionID, fingerprint string) (*Task, bool, error) {
	commandID = strings.TrimSpace(commandID)
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
	if record.workspace != workspace || record.sessionID != sessionID || record.fingerprint != fingerprint {
		r.pruneLocked()
		return nil, false, fmt.Errorf(
			"%w: command_id=%q", ErrAgentCommandConflict, commandID,
		)
	}
	r.order = touchTaskReplayKey(r.order, commandID)
	r.pruneLocked()
	record = r.records[commandID]
	if record.task == nil {
		// Identity survives display eviction. The caller now crosses the durable
		// runtime seam to rebuild a bounded reconnectable Task.
		return nil, false, nil
	}
	return record.task, true, nil
}

// reserve installs command identity and its maximum active display charge
// before Runtime.StartTurn can become durable. commit is infallible; rollback
// removes only the process-local reservation when acceptance did not occur.
func (r *writingStartRegistry) reserve(record writingStartRecord) (*writingStartReservation, error) {
	record.commandID = strings.TrimSpace(record.commandID)
	if record.commandID == "" {
		return nil, ErrAgentCommandIDRequired
	}
	if record.task == nil {
		return nil, fmt.Errorf("cannot reserve a nil Writing task")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.records == nil {
		r.records = make(map[string]writingStartRecord)
	}
	if existing, ok := r.records[record.commandID]; ok {
		if existing.workspace != record.workspace || existing.sessionID != record.sessionID || existing.fingerprint != record.fingerprint || (existing.task != nil && existing.task != record.task) {
			return nil, fmt.Errorf("%w: command_id=%q", ErrAgentCommandConflict, record.commandID)
		}
		rebound := existing.task == nil
		if existing.task == nil {
			existing.task = record.task
			r.records[record.commandID] = existing
		}
		r.order = touchTaskReplayKey(r.order, record.commandID)
		r.pruneLocked()
		if r.registryChargeLocked() > effectiveTaskRegistryReplayByteLimit(r.replayByteLimit) {
			if rebound {
				existing.task = nil
				r.records[record.commandID] = existing
			}
			return nil, fmt.Errorf("%w: Writing command_id=%q", ErrAgentReplayCapacity, record.commandID)
		}
		return &writingStartReservation{registry: r, record: record, rebound: rebound}, nil
	}
	for len(r.records) >= maxRememberedWritingStarts {
		if !r.removeOldestSettledIdentityLocked() {
			return nil, fmt.Errorf("%w: Writing records=%d", ErrAgentReplayCapacity, len(r.records))
		}
	}
	r.records[record.commandID] = record
	r.order = touchTaskReplayKey(r.order, record.commandID)
	r.pruneLocked()
	if r.registryChargeLocked() > effectiveTaskRegistryReplayByteLimit(r.replayByteLimit) {
		delete(r.records, record.commandID)
		r.removeOrderKeyLocked(record.commandID)
		return nil, fmt.Errorf("%w: Writing command_id=%q", ErrAgentReplayCapacity, record.commandID)
	}
	return &writingStartReservation{registry: r, record: record, inserted: true}, nil
}

func (reservation *writingStartReservation) commit() {
	if reservation != nil {
		reservation.bound = true
	}
}

func (reservation *writingStartReservation) rollback() {
	if reservation == nil || reservation.registry == nil || reservation.bound {
		return
	}
	r := reservation.registry
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.records[reservation.record.commandID]
	if !ok || existing.task != reservation.record.task {
		return
	}
	if reservation.inserted {
		delete(r.records, reservation.record.commandID)
		r.removeOrderKeyLocked(reservation.record.commandID)
	} else if reservation.rebound {
		existing.task = nil
		r.records[reservation.record.commandID] = existing
	}
}

func (r *writingStartRegistry) remember(record writingStartRecord) error {
	reservation, err := r.reserve(record)
	if err != nil {
		return err
	}
	reservation.commit()
	return nil
}

func (r *writingStartRegistry) pruneLocked() {
	for len(r.records) > maxRememberedWritingStarts {
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
			log.Printf("[agent-task] pruned settled Writing replay identity command_id=%s task_id=%s released_bytes=%d max_records=%d", commandID, taskID, released, maxRememberedWritingStarts)
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
		log.Printf("[agent-task] evicted settled Writing display replay command_id=%s task_id=%s released_bytes=%d retained_bytes=%d budget_bytes=%d", commandID, taskID, released, totalBytes, byteLimit)
	}
}

func (r *writingStartRegistry) registryChargeLocked() int {
	total := 0
	for _, record := range r.records {
		total += record.task.displayReplayRegistryCharge()
	}
	return total
}

func (r *writingStartRegistry) removeOrderKeyLocked(commandID string) {
	for index, candidate := range r.order {
		if candidate == commandID {
			r.order = removeTaskReplayKey(r.order, index)
			return
		}
	}
}

func (r *writingStartRegistry) removeOldestSettledIdentityLocked() bool {
	for index, commandID := range r.order {
		record, ok := r.records[commandID]
		if !ok {
			r.order = removeTaskReplayKey(r.order, index)
			return true
		}
		if record.task != nil && !record.task.Finished() {
			continue
		}
		if record.task != nil {
			record.task.releaseDisplayReplay()
		}
		delete(r.records, commandID)
		r.order = removeTaskReplayKey(r.order, index)
		return true
	}
	return false
}

func (s *ChatAppService) replayDurableWritingStart(
	ctx context.Context,
	req agent.ChatRequest,
	workspace string,
	sessionID string,
	fingerprint string,
) (*Task, bool, error) {
	a := s.app
	a.mu.RLock()
	chatService := a.chatService
	sess := a.session
	bookService := a.bookService
	a.mu.RUnlock()
	if chatService == nil || sess == nil || sess.ID != sessionID {
		return nil, false, nil
	}
	options := agent.RunOptions{
		AgentKind: agent.AgentKindIDE, SessionID: sessionID,
		Workspace: workspace, Mode: "ide",
	}
	status, err := chatService.RuntimeStatusProjection(ctx, options)
	if err != nil {
		return nil, false, err
	}
	if !agentStatusOwnsCommand(status, req.CommandID) {
		return nil, false, nil
	}

	runtime := ideChatRuntime{
		app: a, sess: sess, bookService: bookService,
		chatService: chatService, workspace: workspace,
	}
	var accepted *agent.AcceptedRun
	task, err := NewDeferredRegisteredTask(func(task *Task) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.workspaceTransition {
			return ErrWorkspaceTransition
		}
		if a.workspace != workspace || a.session != sess {
			return ErrAgentContextChanged
		}
		if a.activeTask != nil && !a.activeTask.Finished() {
			return ErrAgentOperationActive
		}
		if err := a.registerWorkspaceTaskLocked(task, workspace, true); err != nil {
			return err
		}
		a.activeTask = task
		a.activeWritingRun = &writingTaskRun{task: task, runtime: runtime}
		return nil
	})
	if err != nil {
		return nil, true, err
	}
	acceptCtx, releaseAcceptance := taskAcceptanceContext(ctx, task)
	accepted, err = chatService.StartWithOptions(acceptCtx, nil, nil, bookService, req, options, task.emit)
	releaseAcceptance()
	if err != nil {
		rollbackWritingReplayTask(a, task, err)
		if errors.Is(err, agentruntime.ErrInvalidCommand) {
			return nil, true, fmt.Errorf("%w: command_id=%q", ErrAgentCommandConflict, req.CommandID)
		}
		return nil, true, err
	}
	if !accepted.Receipt().Replayed {
		err := fmt.Errorf("durable Writing replay unexpectedly accepted a new command")
		task.Abort()
		_ = accepted.Wait(task.ctx)
		rollbackWritingReplayTask(a, task, err)
		return nil, true, err
	}
	if err := task.Start(func(ctx context.Context, task *Task, _ func(agent.Event)) {
		defer a.unregisterWorkspaceTask(task)
		outcome := accepted.Wait(ctx)
		log.Printf("[agent-task] replay end id=%s command_id=%s status=%s", task.ID(), req.CommandID, outcome.Status)
	}); err != nil {
		task.Abort()
		_ = accepted.Wait(task.ctx)
		rollbackWritingReplayTask(a, task, err)
		return nil, true, err
	}
	if err := s.starts.remember(writingStartRecord{
		commandID: req.CommandID, workspace: workspace, sessionID: sessionID,
		fingerprint: fingerprint, task: task,
	}); err != nil {
		return nil, true, err
	}
	return task, true, nil
}

func agentStatusOwnsCommand(status agentruntime.StatusSnapshot, commandID string) bool {
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

func rollbackWritingReplayTask(a *App, task *Task, err error) {
	task.failBeforeStart(err)
	a.unregisterWorkspaceTask(task)
	a.mu.Lock()
	if a.activeTask == task {
		a.activeTask = nil
	}
	if a.activeWritingRun != nil && a.activeWritingRun.task == task {
		a.activeWritingRun = nil
	}
	a.mu.Unlock()
}

func (run *writingTaskRun) matchesCurrent(a *App) bool {
	if run == nil || run.task == nil || a == nil || a.session == nil {
		return false
	}
	return run.runtime.workspace == a.workspace && run.runtime.sess == a.session
}
