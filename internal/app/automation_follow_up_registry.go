package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"sync"

	"denova/internal/agentruntime"
	"denova/internal/automation"
)

const maxRememberedAutomationFollowUps = 128

type automationFollowUpIdentity struct {
	commandID   string
	runID       string
	message     string
	fingerprint string
}

func newAutomationFollowUpIdentity(runID, commandID, message string) (automationFollowUpIdentity, error) {
	runID = strings.TrimSpace(runID)
	commandID = strings.TrimSpace(commandID)
	message = strings.TrimSpace(message)
	if commandID == "" {
		return automationFollowUpIdentity{}, ErrAgentCommandIDRequired
	}
	if err := agentruntime.ValidateCommandID(commandID, agentruntime.DefaultInputLimits()); err != nil {
		return automationFollowUpIdentity{}, err
	}
	if runID == "" || message == "" {
		return automationFollowUpIdentity{}, fmt.Errorf("automation run and follow-up message are required")
	}
	sum := sha256.Sum256([]byte(runID + "\x00" + message))
	return automationFollowUpIdentity{
		commandID: commandID, runID: runID, message: message,
		fingerprint: hex.EncodeToString(sum[:]),
	}, nil
}

type automationFollowUpRecord struct {
	identity automationFollowUpIdentity
	task     *Task
	run      automation.RunRecord
}

type automationFollowUpReservation struct {
	registry *automationFollowUpRegistry
	identity automationFollowUpIdentity
	task     *Task
	inserted bool
	rebound  bool
	bound    bool
}

// automationFollowUpRegistry is only a display-task replay index. The Agent
// Runtime journal remains authoritative after process restart or eviction.
type automationFollowUpRegistry struct {
	mu              sync.Mutex
	records         map[string]automationFollowUpRecord
	order           []string
	replayByteLimit int
}

func (r *automationFollowUpRegistry) replay(identity automationFollowUpIdentity) (*Task, automation.RunRecord, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.records[identity.commandID]
	if !ok {
		r.pruneLocked()
		return nil, automation.RunRecord{}, false, nil
	}
	if record.identity.runID != identity.runID || record.identity.fingerprint != identity.fingerprint || record.task == nil {
		if record.identity.runID == identity.runID && record.identity.fingerprint == identity.fingerprint && record.task == nil {
			r.order = touchTaskReplayKey(r.order, identity.commandID)
			r.pruneLocked()
			return nil, automation.RunRecord{}, false, nil
		}
		return nil, automation.RunRecord{}, false, fmt.Errorf("%w: command_id=%q", ErrAgentCommandConflict, identity.commandID)
	}
	r.order = touchTaskReplayKey(r.order, identity.commandID)
	r.pruneLocked()
	record = r.records[identity.commandID]
	if record.task == nil {
		return nil, automation.RunRecord{}, false, nil
	}
	return record.task, record.run, true, nil
}

// reserve admits display retention before Runtime.StartWithOptions can durably
// accept the command. Once it succeeds, bind is capacity-infallible.
func (r *automationFollowUpRegistry) reserve(identity automationFollowUpIdentity, task *Task) (*automationFollowUpReservation, error) {
	if task == nil {
		return nil, fmt.Errorf("cannot reserve a nil automation follow-up task")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.records == nil {
		r.records = make(map[string]automationFollowUpRecord)
	}
	if existing, ok := r.records[identity.commandID]; ok {
		if existing.identity.runID != identity.runID || existing.identity.fingerprint != identity.fingerprint || existing.task != task {
			if existing.identity.runID != identity.runID || existing.identity.fingerprint != identity.fingerprint || existing.task != nil {
				return nil, fmt.Errorf("%w: command_id=%q", ErrAgentCommandConflict, identity.commandID)
			}
		}
		existing.task = task
		r.records[identity.commandID] = existing
		r.order = touchTaskReplayKey(r.order, identity.commandID)
		reservation := &automationFollowUpReservation{registry: r, identity: identity, task: task, rebound: true}
		r.pruneLocked()
		if r.registryChargeLocked() > effectiveTaskRegistryReplayByteLimit(r.replayByteLimit) {
			existing.task = nil
			r.records[identity.commandID] = existing
			return nil, fmt.Errorf("%w: automation follow-up command_id=%q", ErrAgentReplayCapacity, identity.commandID)
		}
		return reservation, nil
	}
	for len(r.records) >= maxRememberedAutomationFollowUps {
		if !r.removeOldestSettledIdentityLocked() {
			return nil, fmt.Errorf("%w: automation follow-up records=%d", ErrAgentReplayCapacity, len(r.records))
		}
	}
	r.records[identity.commandID] = automationFollowUpRecord{identity: identity, task: task}
	r.order = touchTaskReplayKey(r.order, identity.commandID)
	reservation := &automationFollowUpReservation{registry: r, identity: identity, task: task, inserted: true}
	r.pruneLocked()
	if r.registryChargeLocked() > effectiveTaskRegistryReplayByteLimit(r.replayByteLimit) {
		delete(r.records, identity.commandID)
		for index, commandID := range r.order {
			if commandID == identity.commandID {
				r.order = removeTaskReplayKey(r.order, index)
				break
			}
		}
		return nil, fmt.Errorf("%w: automation follow-up command_id=%q", ErrAgentReplayCapacity, identity.commandID)
	}
	return reservation, nil
}

func (reservation *automationFollowUpReservation) bind(run automation.RunRecord) {
	if reservation == nil || reservation.registry == nil || reservation.bound {
		return
	}
	r := reservation.registry
	r.mu.Lock()
	record, ok := r.records[reservation.identity.commandID]
	if !ok {
		// Capacity was already reserved before Runtime admission. Rebuilding this
		// record is therefore an invariant repair, not a second admission.
		record = automationFollowUpRecord{identity: reservation.identity, task: reservation.task}
		r.order = touchTaskReplayKey(r.order, reservation.identity.commandID)
	}
	record.run = run
	r.records[reservation.identity.commandID] = record
	reservation.bound = true
	r.mu.Unlock()
}

func (reservation *automationFollowUpReservation) rollback() {
	if reservation == nil || reservation.registry == nil || reservation.bound {
		return
	}
	r := reservation.registry
	r.mu.Lock()
	record, ok := r.records[reservation.identity.commandID]
	if ok && record.task == reservation.task {
		if reservation.inserted {
			delete(r.records, reservation.identity.commandID)
			for index, commandID := range r.order {
				if commandID == reservation.identity.commandID {
					r.order = removeTaskReplayKey(r.order, index)
					break
				}
			}
		} else if reservation.rebound {
			record.task = nil
			r.records[reservation.identity.commandID] = record
		}
	}
	r.mu.Unlock()
}

func (r *automationFollowUpRegistry) remember(identity automationFollowUpIdentity, task *Task, run automation.RunRecord) error {
	reservation, err := r.reserve(identity, task)
	if err != nil {
		return err
	}
	reservation.bind(run)
	return nil
}

func (r *automationFollowUpRegistry) pruneLocked() {
	for len(r.records) > maxRememberedAutomationFollowUps && r.removeOldestSettledIdentityLocked() {
	}
	totalBytes := r.registryChargeLocked()
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
		log.Printf("[automation] evicted settled follow-up display replay command_id=%s task_id=%s released_bytes=%d retained_bytes=%d budget_bytes=%d", commandID, taskID, released, totalBytes, byteLimit)
	}
}

func (r *automationFollowUpRegistry) registryChargeLocked() int {
	total := 0
	for _, record := range r.records {
		total += record.task.displayReplayRegistryCharge()
	}
	return total
}

func (r *automationFollowUpRegistry) removeOldestSettledIdentityLocked() bool {
	for index, commandID := range r.order {
		record, ok := r.records[commandID]
		if !ok {
			r.order = removeTaskReplayKey(r.order, index)
			return true
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
		log.Printf("[automation] pruned settled follow-up replay identity command_id=%s task_id=%s released_bytes=%d max_records=%d", commandID, taskID, released, maxRememberedAutomationFollowUps)
		return true
	}
	return false
}
