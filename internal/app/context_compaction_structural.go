package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"denova/config"
	agents "denova/internal/agents"
	"denova/internal/agents/session"
	"denova/internal/interactive"
)

type contextStructuralOperationFuncs struct {
	prepare   func(context.Context, agents.ContextStructuralIdentity, func(agents.Event)) (agents.ContextStructuralIntent, error)
	commit    func(context.Context, agents.ContextStructuralIdentity, agents.ContextStructuralIntent) (agents.ContextStructuralReceipt, error)
	reconcile func(context.Context) (agents.ContextStructuralResult, agents.ContextStructuralReceipt, bool, error)
}

func (o contextStructuralOperationFuncs) Prepare(ctx context.Context, identity agents.ContextStructuralIdentity, emit func(agents.Event)) (agents.ContextStructuralIntent, error) {
	return o.prepare(ctx, identity, emit)
}

func (o contextStructuralOperationFuncs) Commit(ctx context.Context, identity agents.ContextStructuralIdentity, intent agents.ContextStructuralIntent) (agents.ContextStructuralReceipt, error) {
	return o.commit(ctx, identity, intent)
}

func (o contextStructuralOperationFuncs) Reconcile(ctx context.Context) (agents.ContextStructuralResult, agents.ContextStructuralReceipt, bool, error) {
	return o.reconcile(ctx)
}

func (s *ChatAppService) executeWritingContextCompaction(ctx context.Context, requestedCommandID string) (agents.ContextCompactionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	if s.hasActiveWritingStructuralRecovery() {
		return agents.ContextCompactionResult{}, ErrAgentOperationActive
	}
	if recovered, resumed, err := s.resumeWritingContextStructuralOperation(ctx, agents.ContextStructuralCompact); err != nil {
		return recovered.Compaction, err
	} else if resumed {
		return recovered.Compaction, nil
	}
	fence, err := s.drainWritingBinding(ctx, "")
	if err != nil {
		return agents.ContextCompactionResult{}, err
	}
	runtime, _, err := s.prepareIDEChatRuntime(ctx, agents.ChatRequest{})
	if err != nil {
		return agents.ContextCompactionResult{}, err
	}
	a := s.app
	a.mu.RLock()
	if err := fence.validateLocked(a, true); err != nil || runtime.sess != fence.selected {
		a.mu.RUnlock()
		if err != nil {
			return agents.ContextCompactionResult{}, err
		}
		return agents.ContextCompactionResult{}, ErrAgentContextChanged
	}
	messages, cursor := runtime.sess.GetEffectiveMessagesWithCursor()
	a.mu.RUnlock()
	active, hasActive := runtime.sess.LatestContextCompaction(config.AgentKindIDE)
	sourceStart := cursor.ClearAfterIndex
	existingCheckpoint := ""
	epoch := runtime.sess.NextContextCompactionEpoch(config.AgentKindIDE)
	if hasActive {
		existingCheckpoint = active.Summary
		if active.SourceEndIndex > sourceStart {
			sourceStart = active.SourceEndIndex
		}
	}
	sourceEnd := cursor.MessageCount
	effectiveStart := cursor.MessageCount - len(messages)
	startOffset := sourceStart - effectiveStart
	endOffset := sourceEnd - effectiveStart
	if startOffset < 0 {
		startOffset = 0
	}
	if startOffset > len(messages) {
		startOffset = len(messages)
	}
	if endOffset < startOffset {
		endOffset = startOffset
	}
	if endOffset > len(messages) {
		endOffset = len(messages)
	}
	source := append([]*agents.Message(nil), messages[startOffset:endOffset]...)
	commandID, err := resolveContextStructuralCommandID(
		requestedCommandID,
		contextStructuralCommandID("writing-compact", runtime.workspace, runtime.sess.ID, fmt.Sprint(cursor.Revision)),
	)
	if err != nil {
		return agents.ContextCompactionResult{}, err
	}
	recordID := contextStructuralRecordID("cc", commandID)
	if committed, found := runtime.sess.ContextCompactionByID(recordID); found {
		return contextCompactionResultFromSession(committed), nil
	}
	if len(source) == 0 {
		return agents.ContextCompactionResult{Phase: "manual", SkippedReason: "empty_source"}, fmt.Errorf("没有可压缩的上下文")
	}
	_, prepared, err := agents.PrepareContextCompaction(ctx, &runtime.cfg, config.AgentKindIDE, agents.ContextCompactionInput{
		Messages: messages, SourceMessages: source, Phase: "manual", Force: true,
		ExistingCheckpoint: existingCheckpoint, KeepLatestUser: true,
	}, epoch)
	if err != nil {
		return prepared, err
	}
	if !prepared.Triggered {
		return prepared, fmt.Errorf("没有可压缩的上下文")
	}
	record := sessionCompactionRecord(recordID, config.AgentKindIDE, sourceStart, sourceEnd, prepared)
	ref := agents.ContextCompactionRef{
		Source: "session.effective_messages", Purpose: "persist a bounded model-history checkpoint",
		Resource: runtime.sess.ID, ExpectedRevision: fmt.Sprintf("session-context:%d", cursor.Revision), Force: true,
	}
	binding := writingContextStructuralBinding(runtime.workspace, runtime.sess.ID)
	plan, err := newContextStructuralRestorePlan(
		agents.ContextStructuralDomainSession, agents.ContextStructuralCompact, binding, ref, recordID,
		agents.ContextStructuralResult{Compaction: prepared}, record,
	)
	if err != nil {
		return prepared, err
	}
	operation := fixedContextStructuralOperation(plan,
		func(commitCtx context.Context) (agents.ContextStructuralReceipt, error) {
			a.mu.Lock()
			defer a.mu.Unlock()
			if err := fence.validateLocked(a, true); err != nil || a.session != runtime.sess {
				if err != nil {
					return agents.ContextStructuralReceipt{}, err
				}
				return agents.ContextStructuralReceipt{}, ErrAgentContextChanged
			}
			committed, err := runtime.sess.AppendContextCompactionAtContext(commitCtx, cursor, record)
			if err != nil {
				return agents.ContextStructuralReceipt{}, err
			}
			if !sameSessionContextCompactionMutation(committed, record) {
				return agents.ContextStructuralReceipt{}, fmt.Errorf("canonical writing compaction differs from frozen mutation")
			}
			return agents.ContextStructuralReceipt{Revision: fmt.Sprintf("session-context:%d", committed.ContextRevision)}, nil
		},
		func(context.Context) (agents.ContextStructuralReceipt, bool, error) {
			committed, found := runtime.sess.ContextCompactionByID(recordID)
			if !found {
				return agents.ContextStructuralReceipt{}, false, nil
			}
			if !sameSessionContextCompactionMutation(committed, record) {
				return agents.ContextStructuralReceipt{}, false, fmt.Errorf("canonical writing compaction conflicts with frozen mutation")
			}
			return agents.ContextStructuralReceipt{Revision: fmt.Sprintf("session-context:%d", committed.ContextRevision)}, true, nil
		})
	result, err := runtime.chatService.ExecuteContextStructuralOperation(ctx, agents.ContextStructuralSpec{
		CommandID: commandID, Action: agents.ContextStructuralCompact,
		Ref: ref, Options: agents.RunOptions{AgentKind: agents.AgentKindIDE, Workspace: runtime.workspace, SessionID: runtime.sess.ID, Mode: "ide"},
		Operation: operation, RestorePlan: &plan,
	})
	if err != nil {
		return result.Compaction, err
	}
	if !result.Compaction.Triggered {
		return result.Compaction, fmt.Errorf("没有可压缩的上下文")
	}
	return result.Compaction, nil
}

func (s *ChatAppService) executeWritingContextCompactionRemoval(ctx context.Context, requestedCommandID string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	if s.hasActiveWritingStructuralRecovery() {
		return false, ErrAgentOperationActive
	}
	if recovered, resumed, err := s.resumeWritingContextStructuralOperation(ctx, agents.ContextStructuralRemove); err != nil {
		return recovered.Removed, err
	} else if resumed {
		return recovered.Removed, nil
	}
	fence, err := s.drainWritingBinding(ctx, "")
	if err != nil {
		return false, err
	}
	sess := fence.selected
	if sess == nil {
		return false, ErrNoWorkspace
	}
	cursor := sess.ContextCursor()
	commandID := ""
	if strings.TrimSpace(requestedCommandID) != "" {
		commandID, err = resolveContextStructuralCommandID(requestedCommandID, "")
		if err != nil {
			return false, err
		}
		if _, found := sess.ContextCompactionRemovalByID(contextStructuralRecordID("ccr", commandID)); found {
			return true, nil
		}
	}
	compaction, active := sess.LatestContextCompaction(config.AgentKindIDE)
	if !active {
		return false, nil
	}
	if commandID == "" {
		commandID, err = resolveContextStructuralCommandID(
			"",
			contextStructuralCommandID("writing-remove-compaction", fence.workspace, sess.ID, fmt.Sprint(cursor.Revision), compaction.ID),
		)
		if err != nil {
			return false, err
		}
	}
	recordID := contextStructuralRecordID("ccr", commandID)
	if _, found := sess.ContextCompactionRemovalByID(recordID); found {
		return true, nil
	}
	record := session.ContextCompactionRemoval{
		ID: recordID, AgentKind: config.AgentKindIDE, CompactionID: compaction.ID,
		SourceStartIndex: compaction.SourceStartIndex, SourceEndIndex: compaction.SourceEndIndex, Reason: "user_removed",
	}
	ref := agents.ContextCompactionRef{
		Source: "session.context_compaction", Purpose: "restore raw canonical session history",
		Resource: sess.ID, ExpectedRevision: fmt.Sprintf("session-context:%d", cursor.Revision), CompactionID: compaction.ID,
	}
	binding := writingContextStructuralBinding(fence.workspace, sess.ID)
	plan, err := newContextStructuralRestorePlan(
		agents.ContextStructuralDomainSession, agents.ContextStructuralRemove, binding, ref, recordID,
		agents.ContextStructuralResult{Removed: true}, record,
	)
	if err != nil {
		return false, err
	}
	operation := fixedContextStructuralOperation(plan,
		func(commitCtx context.Context) (agents.ContextStructuralReceipt, error) {
			a := s.app
			a.mu.Lock()
			defer a.mu.Unlock()
			if err := fence.validateLocked(a, true); err != nil || a.session != sess {
				if err != nil {
					return agents.ContextStructuralReceipt{}, err
				}
				return agents.ContextStructuralReceipt{}, ErrAgentContextChanged
			}
			committed, removed, err := sess.CommitContextCompactionRemovalAtContext(commitCtx, cursor, record)
			if err != nil {
				return agents.ContextStructuralReceipt{}, err
			}
			if !removed {
				return agents.ContextStructuralReceipt{}, fmt.Errorf("context compaction disappeared before removal commit")
			}
			if !sameSessionContextCompactionRemovalMutation(committed, record) {
				return agents.ContextStructuralReceipt{}, fmt.Errorf("canonical writing compaction removal differs from frozen mutation")
			}
			return agents.ContextStructuralReceipt{Revision: fmt.Sprintf("session-context:%d", committed.ContextRevision)}, nil
		},
		func(context.Context) (agents.ContextStructuralReceipt, bool, error) {
			committed, found := sess.ContextCompactionRemovalByID(recordID)
			if !found {
				return agents.ContextStructuralReceipt{}, false, nil
			}
			if !sameSessionContextCompactionRemovalMutation(committed, record) {
				return agents.ContextStructuralReceipt{}, false, fmt.Errorf("canonical writing compaction removal conflicts with frozen mutation")
			}
			return agents.ContextStructuralReceipt{Revision: fmt.Sprintf("session-context:%d", committed.ContextRevision)}, true, nil
		})
	result, err := fence.chat.ExecuteContextStructuralOperation(ctx, agents.ContextStructuralSpec{
		CommandID: commandID, Action: agents.ContextStructuralRemove,
		Ref: ref, Options: agents.RunOptions{AgentKind: agents.AgentKindIDE, Workspace: fence.workspace, SessionID: sess.ID, Mode: "ide"},
		Operation: operation, RestorePlan: &plan,
	})
	return result.Removed, err
}

func sessionCompactionRecord(id, agentKind string, sourceStart, sourceEnd int, result agents.ContextCompactionResult) session.ContextCompaction {
	return session.ContextCompaction{
		ID: id, AgentKind: agentKind, Epoch: result.Epoch, Summary: result.Summary,
		SourceStartIndex: sourceStart, SourceEndIndex: sourceEnd, SourceMessageCount: result.SourceMessageCount,
		RetainedTurns: result.RetainedTurns, TokensBefore: result.TokensBefore, TokensAfter: result.TokensAfter,
		TargetRatio: result.TargetRatio, ContextWindowTokens: result.ContextWindowTokens,
		Strategy: result.Strategy, Threshold: result.Threshold, Reason: "manual", Phase: result.Phase,
	}
}

func interactiveCompactionEvent(id, expectedParent string, sourceTurns int, result agents.ContextCompactionResult) interactive.ContextCompactionEvent {
	return interactive.ContextCompactionEvent{
		ID: id, AgentKind: config.AgentKindInteractiveStory, Epoch: result.Epoch, Summary: result.Summary,
		SourceTurnCount: sourceTurns, RetainedTurns: result.RetainedTurns,
		TokensBefore: result.TokensBefore, TokensAfter: result.TokensAfter, TargetRatio: result.TargetRatio,
		ContextWindowTokens: result.ContextWindowTokens, Strategy: result.Strategy, Threshold: result.Threshold,
		Reason: "manual", Phase: result.Phase, ExpectedParentID: &expectedParent,
	}
}

func contextCompactionResultFromSession(record session.ContextCompaction) agents.ContextCompactionResult {
	return agents.ContextCompactionResult{
		Triggered: true, Phase: record.Phase, TokensBefore: record.TokensBefore, TokensAfter: record.TokensAfter,
		ContextWindowTokens: record.ContextWindowTokens, Strategy: record.Strategy, Threshold: record.Threshold,
		Epoch: record.Epoch, Summary: record.Summary, TargetRatio: record.TargetRatio,
		SourceMessageCount: record.SourceMessageCount, RetainedTurns: record.RetainedTurns,
	}
}

func contextCompactionResultFromInteractive(event interactive.ContextCompactionEvent) agents.ContextCompactionResult {
	return agents.ContextCompactionResult{
		Triggered: true, Phase: event.Phase, TokensBefore: event.TokensBefore, TokensAfter: event.TokensAfter,
		ContextWindowTokens: event.ContextWindowTokens, Strategy: event.Strategy, Threshold: event.Threshold,
		Epoch: event.Epoch, Summary: event.Summary, TargetRatio: event.TargetRatio, RetainedTurns: event.RetainedTurns,
	}
}

func contextStructuralCommandID(prefix string, parts ...string) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(strings.TrimSpace(prefix)))
	for _, part := range parts {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(strings.TrimSpace(part)))
	}
	return strings.TrimSpace(prefix) + "-" + hex.EncodeToString(hash.Sum(nil))
}

func contextStructuralRecordID(prefix, commandID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(commandID)))
	return strings.TrimSpace(prefix) + "-" + hex.EncodeToString(sum[:16])
}

func resolveContextStructuralCommandID(requested, fallback string) (string, error) {
	commandID := strings.TrimSpace(requested)
	if commandID == "" {
		commandID = strings.TrimSpace(fallback)
	}
	if err := agents.ValidateCommandID(commandID); err != nil {
		return "", err
	}
	return commandID, nil
}

func contextStructuralValueHash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode structural context identity: %w", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func contextStoryRevision(head string) string {
	head = strings.TrimSpace(head)
	if head == "" {
		head = "root"
	}
	return "story-head:" + head
}
