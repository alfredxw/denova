package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

const defaultMaxRememberedStarts = 128

var (
	// ErrCommandIDRequired rejects root commands that cannot be replayed safely.
	ErrCommandIDRequired = errors.New("agent command_id is required")
	// ErrCommandConflict reports reuse of one command identity for a different
	// runtime binding or semantic request.
	ErrCommandConflict = errors.New("agent command_id was already used for a different request")
)

// StartIdentity is the semantic identity of one durable root Agent start.
// Scope is the stable workspace or Project owner; SessionID narrows it to one
// conversation. Fingerprint covers caller input that must match on replay.
type StartIdentity struct {
	CommandID   string
	Scope       string
	SessionID   string
	Fingerprint string
}

// StartRecord binds durable start identity to its optional process-local
// reconnectable display Task. Identity survives display replay eviction.
type StartRecord struct {
	Identity StartIdentity
	Task     *Task
}

// StartRegistryOptions configures a product-local replay index. Non-positive
// limits select the shared bounded defaults.
type StartRegistryOptions struct {
	Label           string
	MaxRecords      int
	ReplayByteLimit int
}

// StartRegistry is a bounded process-local display replay index. The durable
// Agent runtime remains authoritative after eviction or process restart.
// Its zero value is ready for use.
type StartRegistry struct {
	mu      sync.Mutex
	records map[string]StartRecord
	order   []string
	options StartRegistryOptions
}

func NewStartRegistry(options StartRegistryOptions) StartRegistry {
	return StartRegistry{options: options}
}

func (registry *StartRegistry) Replay(identity StartIdentity) (*Task, bool, error) {
	identity = normalizeStartIdentity(identity)
	if identity.CommandID == "" {
		return nil, false, ErrCommandIDRequired
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	record, found := registry.records[identity.CommandID]
	if !found {
		registry.pruneLocked()
		return nil, false, nil
	}
	if record.Identity != identity {
		registry.pruneLocked()
		return nil, false, fmt.Errorf("%w: command_id=%q", ErrCommandConflict, identity.CommandID)
	}
	registry.order = TouchReplayKey(registry.order, identity.CommandID)
	registry.pruneLocked()
	record = registry.records[identity.CommandID]
	if record.Task == nil {
		return nil, false, nil
	}
	return record.Task, true, nil
}

// StartReservation freezes an identity before durable runtime acceptance.
// Commit is infallible; Rollback removes only an unaccepted local reservation.
type StartReservation struct {
	registry *StartRegistry
	record   StartRecord
	inserted bool
	rebound  bool
	bound    bool
}

func (registry *StartRegistry) Reserve(record StartRecord) (*StartReservation, error) {
	record.Identity = normalizeStartIdentity(record.Identity)
	if record.Identity.CommandID == "" {
		return nil, ErrCommandIDRequired
	}
	if record.Task == nil {
		return nil, fmt.Errorf("cannot reserve a nil %s task", registry.label())
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.records == nil {
		registry.records = make(map[string]StartRecord)
	}
	commandID := record.Identity.CommandID
	if existing, found := registry.records[commandID]; found {
		if existing.Identity != record.Identity || (existing.Task != nil && existing.Task != record.Task) {
			return nil, fmt.Errorf("%w: command_id=%q", ErrCommandConflict, commandID)
		}
		rebound := existing.Task == nil
		if rebound {
			existing.Task = record.Task
			registry.records[commandID] = existing
		}
		registry.order = TouchReplayKey(registry.order, commandID)
		registry.pruneLocked()
		if registry.registryChargeLocked() > registry.replayByteLimit() {
			if rebound {
				existing.Task = nil
				registry.records[commandID] = existing
			}
			return nil, registry.capacityError("command_id=%q", commandID)
		}
		return &StartReservation{registry: registry, record: record, rebound: rebound}, nil
	}

	for len(registry.records) >= registry.maxRecords() {
		if !registry.removeOldestSettledIdentityLocked() {
			return nil, registry.capacityError("records=%d", len(registry.records))
		}
	}
	registry.records[commandID] = record
	registry.order = TouchReplayKey(registry.order, commandID)
	registry.pruneLocked()
	if registry.registryChargeLocked() > registry.replayByteLimit() {
		delete(registry.records, commandID)
		registry.removeOrderKeyLocked(commandID)
		return nil, registry.capacityError("command_id=%q", commandID)
	}
	return &StartReservation{registry: registry, record: record, inserted: true}, nil
}

func (reservation *StartReservation) Commit() {
	if reservation != nil {
		reservation.bound = true
	}
}

func (reservation *StartReservation) Rollback() {
	if reservation == nil || reservation.registry == nil || reservation.bound {
		return
	}
	registry := reservation.registry
	registry.mu.Lock()
	defer registry.mu.Unlock()
	commandID := reservation.record.Identity.CommandID
	existing, found := registry.records[commandID]
	if !found || existing.Task != reservation.record.Task {
		return
	}
	if reservation.inserted {
		delete(registry.records, commandID)
		registry.removeOrderKeyLocked(commandID)
	} else if reservation.rebound {
		existing.Task = nil
		registry.records[commandID] = existing
	}
}

func (registry *StartRegistry) Remember(record StartRecord) error {
	reservation, err := registry.Reserve(record)
	if err != nil {
		return err
	}
	reservation.Commit()
	return nil
}

// Latest returns the most recently used display owner for an exact scope and
// session. It never crosses conversation bindings.
func (registry *StartRegistry) Latest(scope, sessionID string) StartRecord {
	if registry == nil {
		return StartRecord{}
	}
	scope = strings.TrimSpace(scope)
	sessionID = strings.TrimSpace(sessionID)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.pruneLocked()
	for index := len(registry.order) - 1; index >= 0; index-- {
		commandID := registry.order[index]
		record, found := registry.records[commandID]
		if !found || record.Identity.Scope != scope || record.Identity.SessionID != sessionID || record.Task == nil {
			continue
		}
		registry.order = TouchReplayKey(registry.order, commandID)
		return record
	}
	return StartRecord{}
}

// ReleaseScope invalidates display-only replay after the durable conversation
// binding has been drained and cleared by its owner.
func (registry *StartRegistry) ReleaseScope(scope, sessionID string) {
	if registry == nil {
		return
	}
	scope = strings.TrimSpace(scope)
	sessionID = strings.TrimSpace(sessionID)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for index := len(registry.order) - 1; index >= 0; index-- {
		commandID := registry.order[index]
		record, found := registry.records[commandID]
		if !found {
			registry.order = RemoveReplayKey(registry.order, index)
			continue
		}
		if record.Identity.Scope != scope || record.Identity.SessionID != sessionID {
			continue
		}
		if record.Task != nil {
			record.Task.ReleaseDisplayReplay()
		}
		delete(registry.records, commandID)
		registry.order = RemoveReplayKey(registry.order, index)
	}
}

func (registry *StartRegistry) pruneLocked() {
	for len(registry.records) > registry.maxRecords() {
		if !registry.removeOldestSettledIdentityLocked() {
			break
		}
	}

	totalBytes := registry.registryChargeLocked()
	byteLimit := registry.replayByteLimit()
	for _, commandID := range registry.order {
		if totalBytes <= byteLimit {
			break
		}
		record, found := registry.records[commandID]
		if !found || record.Task == nil || !record.Task.Finished() {
			continue
		}
		taskID := record.Task.ID()
		released := record.Task.ReleaseDisplayReplay()
		totalBytes -= released
		record.Task = nil
		registry.records[commandID] = record
		slog.InfoContext(context.Background(), fmt.Sprintf(
			"[app/task] evicted settled %s display replay command_id=%s task_id=%s released_bytes=%d retained_bytes=%d budget_bytes=%d",
			registry.label(), commandID, taskID, released, totalBytes, byteLimit,
		))
	}
}

func (registry *StartRegistry) registryChargeLocked() int {
	total := 0
	for _, record := range registry.records {
		if record.Task != nil {
			total += record.Task.DisplayReplayCharge()
		}
	}
	return total
}

func (registry *StartRegistry) removeOrderKeyLocked(commandID string) {
	for index, candidate := range registry.order {
		if candidate == commandID {
			registry.order = RemoveReplayKey(registry.order, index)
			return
		}
	}
}

func (registry *StartRegistry) removeOldestSettledIdentityLocked() bool {
	for index, commandID := range registry.order {
		record, found := registry.records[commandID]
		if !found {
			registry.order = RemoveReplayKey(registry.order, index)
			return true
		}
		if record.Task != nil && !record.Task.Finished() {
			continue
		}
		if record.Task != nil {
			record.Task.ReleaseDisplayReplay()
		}
		delete(registry.records, commandID)
		registry.order = RemoveReplayKey(registry.order, index)
		return true
	}
	return false
}

func (registry *StartRegistry) maxRecords() int {
	if registry != nil && registry.options.MaxRecords > 0 {
		return registry.options.MaxRecords
	}
	return defaultMaxRememberedStarts
}

func (registry *StartRegistry) replayByteLimit() int {
	if registry == nil {
		return EffectiveRegistryReplayByteLimit(0)
	}
	return EffectiveRegistryReplayByteLimit(registry.options.ReplayByteLimit)
}

func (registry *StartRegistry) label() string {
	if registry != nil {
		if label := strings.TrimSpace(registry.options.Label); label != "" {
			return label
		}
	}
	return "Agent"
}

func (registry *StartRegistry) capacityError(format string, args ...any) error {
	detail := fmt.Sprintf(format, args...)
	return fmt.Errorf("%w: %s %s", ErrReplayCapacity, registry.label(), detail)
}

func normalizeStartIdentity(identity StartIdentity) StartIdentity {
	identity.CommandID = strings.TrimSpace(identity.CommandID)
	identity.Scope = strings.TrimSpace(identity.Scope)
	identity.SessionID = strings.TrimSpace(identity.SessionID)
	identity.Fingerprint = strings.TrimSpace(identity.Fingerprint)
	return identity
}
