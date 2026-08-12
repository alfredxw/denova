package runtime

import (
	"encoding/json"
	"strings"
	"time"
)

type subscriber struct {
	id     uint64
	events chan Event
	errors chan error
}

type harnessState struct {
	binding                BindingRef
	cursor                 Cursor
	phase                  Phase
	activeOperation        OperationID
	activeCycle            int
	activeCycleDelivery    DeliveryKind
	activeCycleAutonomous  bool
	activeCycleStartedAt   time.Time
	activeSnapshotID       SnapshotID
	activeStructural       *StructuralOperationSnapshot
	recoveryPaused         bool
	inputRecovery          *InputMaterializationRecovery
	activeInput            UserInput
	activeContent          strings.Builder
	activeThinking         strings.Builder
	engineState            json.RawMessage
	capabilityStates       map[string]json.RawMessage
	interactions           map[string]InteractionSnapshot
	activeOutputError      *ByteBudgetError
	activeOutputRehydrated bool
	messages               []Message
	queue                  []QueuedInput
	preemptQueuedCommandID CommandID
	openToolCalls          map[string]ToolCallState
	pendingHostEffects     map[HostEffectID]HostEffect
	pendingHostEffectOrder []HostEffectID
	events                 []Event
	receipts               map[CommandID]Receipt
	fingerprints           map[CommandID]string
	commandOrder           []CommandID
	operationCommands      map[OperationID]CommandID
	operationAcceptances   map[OperationID]CommandRecord
	activeCommandID        CommandID
	activeCycleCommandID   CommandID
	pendingCycleCommandID  CommandID
	lastOperation          *OperationSummary
	recentOperations       []OperationSummary
	abortReason            string
	domainCommits          map[DomainCommitStage]*DomainCommitState
	lastDomainCommits      map[DomainCommitStage]*DomainCommitState
	lastDomainCommit       *DomainCommitState
	abortRequested         bool
	subscribers            map[uint64]*subscriber
	nextSubscriber         uint64
	engineControls         chan EngineControl
	pendingEngineDone      *engineDoneRequest
	closing                bool
	closeWaiters           []chan error
	retainTimeline         bool
	maxRetainedEvents      int
	maxRetainedMessages    int
	maxRetainedCommands    int
	memoryLimits           BindingMemoryLimits
	retainedEventBytes     int64
	retainedMessageBytes   int64
	retainedCommandBytes   int64
	messagesTruncated      bool
	retainCommandIndex     bool
}

func newHarnessState(binding BindingRef) harnessState {
	return harnessState{
		binding:              binding.Clone(),
		phase:                PhaseIdle,
		openToolCalls:        make(map[string]ToolCallState),
		pendingHostEffects:   make(map[HostEffectID]HostEffect),
		capabilityStates:     make(map[string]json.RawMessage),
		interactions:         make(map[string]InteractionSnapshot),
		receipts:             make(map[CommandID]Receipt),
		fingerprints:         make(map[CommandID]string),
		operationCommands:    make(map[OperationID]CommandID),
		operationAcceptances: make(map[OperationID]CommandRecord),
		domainCommits:        make(map[DomainCommitStage]*DomainCommitState),
		lastDomainCommits:    make(map[DomainCommitStage]*DomainCommitState),
		subscribers:          make(map[uint64]*subscriber),
		retainTimeline:       true,
		retainCommandIndex:   true,
		memoryLimits:         DefaultBindingMemoryLimits(),
	}
}

func newProjectionState(binding BindingRef) harnessState {
	state := newHarnessState(binding)
	state.retainTimeline = false
	state.retainCommandIndex = false
	return state
}
