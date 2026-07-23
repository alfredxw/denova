package agentruntime

import (
	"fmt"
	"sort"
)

func (s *harnessState) validateDomainCommitIdentity(identity DomainCommitIdentity) error {
	if identity.Stage != DomainCommitInput && identity.Stage != DomainCommitOutput {
		return fmt.Errorf("%w: unsupported domain commit stage %q", ErrDomainCommitRejected, identity.Stage)
	}
	if identity.OperationID != s.activeOperation || identity.Cycle != s.activeCycle || identity.CommandID != s.activeCycleCommandID {
		return fmt.Errorf("%w: domain commit identity does not match active cycle", ErrDomainCommitRejected)
	}
	return nil
}

func (s *harnessState) domainCommit(stage DomainCommitStage) *DomainCommitState {
	if s == nil {
		return nil
	}
	return s.domainCommits[stage]
}

func (s *harnessState) outputCommitFinalizing() bool {
	return s.domainCommit(DomainCommitOutput) != nil
}

func (s *harnessState) pendingDomainCommit() *DomainCommitState {
	for _, stage := range []DomainCommitStage{DomainCommitInput, DomainCommitOutput} {
		if commit := s.domainCommit(stage); commit != nil && commit.Revision == "" {
			return commit
		}
	}
	return nil
}

func (s *harnessState) acknowledgedOutputCommit() *DomainCommitState {
	commit := s.domainCommit(DomainCommitOutput)
	if commit == nil || commit.Revision == "" {
		return nil
	}
	return commit
}

func (s *harnessState) turnSnapshot(id SnapshotID) TurnSnapshot {
	if id == "" {
		id = s.activeSnapshotID
	}
	return TurnSnapshot{
		ID: id, Binding: s.binding, OperationID: s.activeOperation,
		CommandID: s.activeCycleCommandID,
		Cycle:     s.activeCycle, Input: cloneUserInput(s.activeInput),
		ContextCursor: s.cursor,
	}
}

func (s *harnessState) hasQueued(delivery DeliveryKind) bool {
	for _, item := range s.queue {
		if item.Delivery == delivery {
			return true
		}
	}
	return false
}

func (s *harnessState) firstQueued(deliveries ...DeliveryKind) (QueuedInput, bool) {
	for _, delivery := range deliveries {
		for _, item := range s.queue {
			if item.Delivery == delivery {
				return cloneQueuedInput(item), true
			}
		}
	}
	return QueuedInput{}, false
}

func (s *harnessState) removeQueued(commandID CommandID) bool {
	for index, item := range s.queue {
		if item.CommandID == commandID {
			copy(s.queue[index:], s.queue[index+1:])
			s.queue[len(s.queue)-1] = QueuedInput{}
			s.queue = s.queue[:len(s.queue)-1]
			return true
		}
	}
	return false
}

func (s *harnessState) activeToolCalls() []ToolCallState {
	calls := make([]ToolCallState, 0)
	for _, call := range s.openToolCalls {
		if call.OperationID == s.activeOperation {
			call = normalizeToolCallState(call)
			calls = append(calls, call)
		}
	}
	sort.Slice(calls, func(i, j int) bool { return calls[i].CallID < calls[j].CallID })
	return calls
}
