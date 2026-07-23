package app

import (
	"fmt"
	"sync"
)

const (
	// Active Tasks reserve up to 64 MiB each (raw replay plus its compact
	// checkpoint). Eight concurrent Agent products therefore remain available
	// without letting independently owned registries multiply memory without a
	// process-wide ceiling.
	defaultActiveReplayByteLimit = 512 << 20
	maxActiveReplayTasks         = 8
)

// activeTaskReplayAdmission is the process-wide pre-admission budget for
// reconnectable Task buffers that may grow while a Runtime command is active.
// Product registries still own command identity and settled LRU eviction; this
// seam prevents independent registries from each reserving the full budget.
type activeTaskReplayAdmission struct {
	mu         sync.Mutex
	active     map[*Task]int
	byteLimit  int
	countLimit int
}

type activeTaskReplayReservation struct {
	admission *activeTaskReplayAdmission
	task      *Task
	once      sync.Once
}

func (a *activeTaskReplayAdmission) reserve(task *Task) (*activeTaskReplayReservation, error) {
	if task == nil {
		return nil, fmt.Errorf("cannot reserve replay capacity for a nil Task")
	}
	charge := task.displayReplayRegistryCharge()
	if charge <= 0 {
		return nil, fmt.Errorf("Task replay reservation has no bounded charge")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active == nil {
		a.active = make(map[*Task]int)
	}
	// App registration, not a sampled Task status, owns this reservation.
	// Inferring release from Finished would let replay capacity escape before
	// the matching lifecycle lease and observable registry entry are removed.
	if _, exists := a.active[task]; exists {
		return nil, fmt.Errorf("Task replay capacity is already reserved")
	}
	countLimit := a.countLimit
	if countLimit <= 0 {
		countLimit = maxActiveReplayTasks
	}
	if len(a.active) >= countLimit || a.activeBytesLocked()+charge > effectiveActiveReplayByteLimit(a.byteLimit) {
		return nil, fmt.Errorf("%w: active_tasks=%d requested_bytes=%d retained_bytes=%d", ErrAgentReplayCapacity, len(a.active), charge, a.activeBytesLocked())
	}
	a.active[task] = charge
	return &activeTaskReplayReservation{admission: a, task: task}, nil
}

func effectiveActiveReplayByteLimit(configured int) int {
	if configured > 0 {
		return configured
	}
	return defaultActiveReplayByteLimit
}

func (a *activeTaskReplayAdmission) activeBytesLocked() int {
	total := 0
	for _, charge := range a.active {
		total += charge
	}
	return total
}

func (r *activeTaskReplayReservation) release() {
	if r == nil || r.admission == nil {
		return
	}
	r.once.Do(func() {
		r.admission.mu.Lock()
		delete(r.admission.active, r.task)
		r.admission.mu.Unlock()
	})
}
