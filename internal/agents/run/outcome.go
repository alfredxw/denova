package agentrun

// ControlKind is the closed set of controls accepted by one Agent run.
type ControlKind string

const (
	ControlPreempt ControlKind = "preempt"
	ControlAbort   ControlKind = "abort"
)

// Control requests a lifecycle transition for the run attached to its channel.
// Closing the channel does not imply either control.
type Control struct {
	Kind   ControlKind
	Reason string
}

// OutcomeStatus describes why the current execution handle ended. Suspended
// preserves the logical Run and requires an explicit continuation.
type OutcomeStatus string

const (
	OutcomeCompleted OutcomeStatus = "completed"
	OutcomePreempted OutcomeStatus = "preempted"
	OutcomeAborted   OutcomeStatus = "aborted"
	OutcomeFailed    OutcomeStatus = "failed"
	OutcomeSuspended OutcomeStatus = "suspended"
)

// Outcome is the transport-independent terminal result of one Agent run.
type Outcome struct {
	Status   OutcomeStatus
	Error    error
	Reason   string
	Content  string
	Thinking string
	// MaintenanceOnly means the model call was intentionally deferred after a
	// valid structural checkpoint was staged.
	MaintenanceOnly bool
}

// NewOutcome builds an outcome from already-classified output.
func NewOutcome(status OutcomeStatus, err error, reason, content, thinking string) Outcome {
	return Outcome{Status: status, Error: err, Reason: reason, Content: content, Thinking: thinking}
}
