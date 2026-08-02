package app

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentcontext "denova/internal/agents/context"
	agentstructural "denova/internal/agents/context/structural"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"fmt"
	"strings"

	"denova/config"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/prompts"
	"denova/internal/agents/session"
	appagentruntime "denova/internal/app/agentruntime"
	compactionapp "denova/internal/app/compaction"
)

func (s *ChatAppService) executeWritingContextCompaction(ctx context.Context, requestedCommandID string) (agentcompaction.Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.admission.Lock()
	defer s.admission.Unlock()
	if s.hasActiveWritingStructuralRecovery() {
		return agentcompaction.Result{}, ErrAgentOperationActive
	}
	if recovered, resumed, err := s.resumeWritingContextStructuralOperation(ctx, agentstructural.Compact); err != nil {
		return recovered.Compaction, err
	} else if resumed {
		return recovered.Compaction, nil
	}
	fence, err := s.drainWritingBinding(ctx, "")
	if err != nil {
		return agentcompaction.Result{}, err
	}
	runtime, _, err := s.prepareIDEChatRuntime(ctx, agentchat.ChatRequest{})
	if err != nil {
		return agentcompaction.Result{}, err
	}
	a := s.app
	a.mu.RLock()
	if err := fence.validateLocked(a, true); err != nil || runtime.sess != fence.selected {
		a.mu.RUnlock()
		if err != nil {
			return agentcompaction.Result{}, err
		}
		return agentcompaction.Result{}, ErrAgentContextChanged
	}
	runtimeContexts := prompts.IDEWorkspaceRuntimeContextsForContext(runtime.state, prompts.IDEContextRef{})
	conversation := agentconversation.NewSessionConversationForAgentWithRuntimeContexts(
		runtime.sess, &runtime.cfg, config.AgentKindIDE,
		runtimeContexts.StableTitle, runtimeContexts.Stable,
		runtimeContexts.DynamicTitle, runtimeContexts.Dynamic,
	)
	projection, err := conversation.SnapshotContextCompaction(ctx, true)
	if err != nil {
		a.mu.RUnlock()
		return agentcompaction.Result{}, fmt.Errorf("snapshot writing compaction source: %w", err)
	}
	assembled, err := conversation.AssembleModelContext(ctx, "", agentcontext.ModelContextInput{
		UserMessage: "", Budget: conversation.ModelContextBudget(),
	})
	if err != nil {
		a.mu.RUnlock()
		return agentcompaction.Result{}, fmt.Errorf("assemble writing compaction context: %w", err)
	}
	messages, cursor := assembled.Messages, projection.Cursor
	a.mu.RUnlock()
	source := projection.Source
	sourceStart, sourceEnd := projection.SourceStartIndex, projection.SourceEndIndex
	existingCheckpoint := projection.ExistingCheckpoint
	commandID, err := compactionapp.ResolveCommandID(
		requestedCommandID,
		agentstructural.CommandID("writing-compact", runtime.workspace, runtime.sess.ID, fmt.Sprint(cursor.Revision)),
	)
	if err != nil {
		return agentcompaction.Result{}, err
	}
	recordID := agentstructural.RecordID("cc", commandID)
	if committed, found := runtime.sess.ContextCompactionByID(recordID); found {
		return compactionapp.ResultFromSession(committed), nil
	}
	manualHealthFingerprint := "manual:" + commandID
	if _, err := runtime.sess.CommitContextCompactionHealthAtContext(ctx, cursor, session.ContextCompactionHealth{
		ID: agentstructural.RecordID("cch-manual", commandID), AgentKind: config.AgentKindIDE,
		StructureFingerprint: manualHealthFingerprint, Outcome: "manual_retry",
	}); err != nil {
		return agentcompaction.Result{}, fmt.Errorf("reset writing compaction health: %w", err)
	}
	if len(source) == 0 {
		return agentcompaction.Result{Phase: "manual", SkippedReason: "empty_source"}, fmt.Errorf("没有可压缩的上下文")
	}
	runner, _, err := appagentruntime.BuildConversation(
		ctx, &runtime.cfg, runtime.state, runtime.ideTeller, agentrun.AgentKindIDE,
	)
	if err != nil {
		return agentcompaction.Result{}, fmt.Errorf("assemble writing compaction request: %w", err)
	}
	primarySnapshot, err := runner.PrepareModelRequest(ctx, messages)
	if err != nil {
		return agentcompaction.Result{}, fmt.Errorf("capture writing compaction request: %w", err)
	}
	tools := primarySnapshot.ResolvedOptions().Tools
	// Manual Writing compaction uses the same SessionConversation path as
	// automatic maintenance. That path maps canonical source messages to the
	// normalizer-produced provider projection and validates/re-measures the
	// compacted candidate before publication. Calling the lower-level helper
	// here would compare pre-normalizer history with the final request snapshot.
	_, prepared, err := conversation.CompactContextIfNeeded(ctx, agentcompaction.Input{
		Messages: messages, Tools: tools, Phase: "manual", Force: true,
		ExistingCheckpoint: existingCheckpoint, KeepLatestUser: true, PrimaryRequestSnapshot: primarySnapshot,
	})
	if err != nil {
		return prepared, err
	}
	if !prepared.Triggered {
		return prepared, fmt.Errorf("没有可压缩的上下文")
	}
	record := compactionapp.SessionRecord(recordID, config.AgentKindIDE, sourceStart, sourceEnd, prepared)
	ref := agentrun.ContextCompactionRef{
		Source: "session.effective_messages", Purpose: "persist a bounded model-history checkpoint",
		Resource: runtime.sess.ID, ExpectedRevision: fmt.Sprintf("session-context:%d", cursor.Revision), Force: true,
	}
	binding := compactionapp.WritingBinding(runtime.workspace, runtime.sess.ID)
	plan, err := agentstructural.NewRestorePlan(
		agentstructural.DomainSession, agentstructural.Compact, binding, ref, recordID,
		agentstructural.Result{Compaction: prepared}, record,
	)
	if err != nil {
		return prepared, err
	}
	operation := agentstructural.FixedOperation(plan,
		func(commitCtx context.Context) (agentstructural.Receipt, error) {
			a.mu.Lock()
			defer a.mu.Unlock()
			if err := fence.validateLocked(a, true); err != nil || a.session != runtime.sess {
				if err != nil {
					return agentstructural.Receipt{}, err
				}
				return agentstructural.Receipt{}, ErrAgentContextChanged
			}
			committed, err := runtime.sess.AppendContextCompactionAtContext(commitCtx, cursor, record)
			if err != nil {
				return agentstructural.Receipt{}, err
			}
			if !compactionapp.SameSessionMutation(committed, record) {
				return agentstructural.Receipt{}, fmt.Errorf("canonical writing compaction differs from frozen mutation")
			}
			return agentstructural.Receipt{Revision: fmt.Sprintf("session-context:%d", committed.ContextRevision)}, nil
		},
		func(context.Context) (agentstructural.Receipt, bool, error) {
			committed, found := runtime.sess.ContextCompactionByID(recordID)
			if !found {
				return agentstructural.Receipt{}, false, nil
			}
			if !compactionapp.SameSessionMutation(committed, record) {
				return agentstructural.Receipt{}, false, fmt.Errorf("canonical writing compaction conflicts with frozen mutation")
			}
			return agentstructural.Receipt{Revision: fmt.Sprintf("session-context:%d", committed.ContextRevision)}, true, nil
		})
	result, err := runtime.chatService.ExecuteStructuralOperation(ctx, agentstructural.Spec{
		CommandID: commandID, Action: agentstructural.Compact,
		Ref: ref, Options: agentrun.Options{AgentKind: agentrun.AgentKindIDE, StateRoot: runtime.projectState, Workspace: runtime.workspace, SessionID: runtime.sess.ID, Mode: "ide"},
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
	if recovered, resumed, err := s.resumeWritingContextStructuralOperation(ctx, agentstructural.Remove); err != nil {
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
		commandID, err = compactionapp.ResolveCommandID(requestedCommandID, "")
		if err != nil {
			return false, err
		}
		if _, found := sess.ContextCompactionRemovalByID(agentstructural.RecordID("ccr", commandID)); found {
			return true, nil
		}
	}
	compaction, active := sess.LatestContextCompaction(config.AgentKindIDE)
	if !active {
		return false, nil
	}
	if commandID == "" {
		commandID, err = compactionapp.ResolveCommandID(
			"",
			agentstructural.CommandID("writing-remove-compaction", fence.workspace, sess.ID, fmt.Sprint(cursor.Revision), compaction.ID),
		)
		if err != nil {
			return false, err
		}
	}
	recordID := agentstructural.RecordID("ccr", commandID)
	if _, found := sess.ContextCompactionRemovalByID(recordID); found {
		return true, nil
	}
	record := session.ContextCompactionRemoval{
		ID: recordID, AgentKind: config.AgentKindIDE, CompactionID: compaction.ID,
		SourceStartIndex: compaction.SourceStartIndex, SourceEndIndex: compaction.SourceEndIndex, Reason: "user_removed",
	}
	ref := agentrun.ContextCompactionRef{
		Source: "session.context_compaction", Purpose: "restore raw canonical session history",
		Resource: sess.ID, ExpectedRevision: fmt.Sprintf("session-context:%d", cursor.Revision), CompactionID: compaction.ID,
	}
	binding := compactionapp.WritingBinding(fence.workspace, sess.ID)
	plan, err := agentstructural.NewRestorePlan(
		agentstructural.DomainSession, agentstructural.Remove, binding, ref, recordID,
		agentstructural.Result{Removed: true}, record,
	)
	if err != nil {
		return false, err
	}
	operation := agentstructural.FixedOperation(plan,
		func(commitCtx context.Context) (agentstructural.Receipt, error) {
			a := s.app
			a.mu.Lock()
			defer a.mu.Unlock()
			if err := fence.validateLocked(a, true); err != nil || a.session != sess {
				if err != nil {
					return agentstructural.Receipt{}, err
				}
				return agentstructural.Receipt{}, ErrAgentContextChanged
			}
			committed, removed, err := sess.CommitContextCompactionRemovalAtContext(commitCtx, cursor, record)
			if err != nil {
				return agentstructural.Receipt{}, err
			}
			if !removed {
				return agentstructural.Receipt{}, fmt.Errorf("context compaction disappeared before removal commit")
			}
			if !compactionapp.SameSessionRemoval(committed, record) {
				return agentstructural.Receipt{}, fmt.Errorf("canonical writing compaction removal differs from frozen mutation")
			}
			return agentstructural.Receipt{Revision: fmt.Sprintf("session-context:%d", committed.ContextRevision)}, nil
		},
		func(context.Context) (agentstructural.Receipt, bool, error) {
			committed, found := sess.ContextCompactionRemovalByID(recordID)
			if !found {
				return agentstructural.Receipt{}, false, nil
			}
			if !compactionapp.SameSessionRemoval(committed, record) {
				return agentstructural.Receipt{}, false, fmt.Errorf("canonical writing compaction removal conflicts with frozen mutation")
			}
			return agentstructural.Receipt{Revision: fmt.Sprintf("session-context:%d", committed.ContextRevision)}, true, nil
		})
	result, err := fence.chat.ExecuteStructuralOperation(ctx, agentstructural.Spec{
		CommandID: commandID, Action: agentstructural.Remove,
		Ref: ref, Options: agentrun.Options{AgentKind: agentrun.AgentKindIDE, StateRoot: fence.stateRoot, Workspace: fence.workspace, SessionID: sess.ID, Mode: "ide"},
		Operation: operation, RestorePlan: &plan,
	})
	return result.Removed, err
}
