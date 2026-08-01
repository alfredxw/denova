package task

import (
	"errors"
	"fmt"
	"sync"
)

const (
	// Active Tasks reserve both their raw replay window and compact display
	// checkpoint. The defaults cap the process at eight concurrent owners and
	// 512 MiB even when several product registries are active independently.
	DefaultActiveReplayByteLimit = 512 << 20
	DefaultMaxActiveReplayTasks  = 8
)

// ErrReplayCapacity is returned before a durable Agent command is admitted.
// The caller may therefore retry the same command identity without an
// uncertain acceptance.
var ErrReplayCapacity = errors.New("agent display replay capacity is full")

// ReplayAdmission owns the process-wide pre-admission budget for reconnectable
// Task display buffers. Product registries remain responsible for command
// identity and settled LRU eviction.
type ReplayAdmission struct {
	mu         sync.Mutex
	active     map[*Task]int
	byteLimit  int
	countLimit int
}

// ReplayAdmissionLimits overrides the process defaults. Non-positive fields
// select their respective defaults.
type ReplayAdmissionLimits struct {
	MaxBytes  int
	MaxActive int
}

// ReplayAdmissionStats is a consistent diagnostic snapshot.
type ReplayAdmissionStats struct {
	ActiveTasks int
	ActiveBytes int
}

// ReplayReservation releases exactly one admitted Task charge.
type ReplayReservation struct {
	admission *ReplayAdmission
	task      *Task
	once      sync.Once
}

// Configure applies admission limits. It is intended for application startup
// and tests; changing limits never evicts already admitted Tasks.
func (a *ReplayAdmission) Configure(limits ReplayAdmissionLimits) {
	a.mu.Lock()
	a.byteLimit = limits.MaxBytes
	a.countLimit = limits.MaxActive
	a.mu.Unlock()
}

// Reserve atomically charges a Task's maximum bounded display footprint.
func (a *ReplayAdmission) Reserve(task *Task) (*ReplayReservation, error) {
	if task == nil {
		return nil, fmt.Errorf("cannot reserve replay capacity for a nil Task")
	}
	charge := task.DisplayReplayCharge()
	if charge <= 0 {
		return nil, fmt.Errorf("Task replay reservation has no bounded charge")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active == nil {
		a.active = make(map[*Task]int)
	}
	// Registration, not a sampled Task status, owns the reservation. Inferring
	// release from Finished could free memory before the matching registry entry
	// and lifecycle lease are removed.
	if _, exists := a.active[task]; exists {
		return nil, fmt.Errorf("Task replay capacity is already reserved")
	}
	countLimit, byteLimit := a.effectiveLimitsLocked()
	activeBytes := a.activeBytesLocked()
	if len(a.active) >= countLimit || activeBytes+charge > byteLimit {
		return nil, fmt.Errorf("%w: active_tasks=%d requested_bytes=%d retained_bytes=%d", ErrReplayCapacity, len(a.active), charge, activeBytes)
	}
	a.active[task] = charge
	return &ReplayReservation{admission: a, task: task}, nil
}

func (a *ReplayAdmission) Stats() ReplayAdmissionStats {
	a.mu.Lock()
	defer a.mu.Unlock()
	return ReplayAdmissionStats{ActiveTasks: len(a.active), ActiveBytes: a.activeBytesLocked()}
}

func (a *ReplayAdmission) effectiveLimitsLocked() (int, int) {
	countLimit := a.countLimit
	if countLimit <= 0 {
		countLimit = DefaultMaxActiveReplayTasks
	}
	byteLimit := a.byteLimit
	if byteLimit <= 0 {
		byteLimit = DefaultActiveReplayByteLimit
	}
	return countLimit, byteLimit
}

func (a *ReplayAdmission) activeBytesLocked() int {
	total := 0
	for _, charge := range a.active {
		total += charge
	}
	return total
}

func (r *ReplayReservation) Release() {
	if r == nil || r.admission == nil {
		return
	}
	r.once.Do(func() {
		r.admission.mu.Lock()
		delete(r.admission.active, r.task)
		r.admission.mu.Unlock()
	})
}
