package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"

	"denova/internal/agent"
	"denova/internal/agentruntime"
	"denova/internal/interactive"
	"denova/internal/session"
)

func (s *ChatAppService) resumeWritingContextStructuralOperation(
	ctx context.Context,
	action agent.ContextStructuralAction,
) (agent.ContextStructuralResult, bool, error) {
	if s == nil || s.app == nil {
		return agent.ContextStructuralResult{}, false, nil
	}
	a := s.app
	a.mu.RLock()
	chat := a.chatService
	workspace := a.workspace
	sessionID := ""
	selected := a.session
	if a.session != nil {
		sessionID = a.session.ID
	}
	a.mu.RUnlock()
	if chat == nil || strings.TrimSpace(workspace) == "" || strings.TrimSpace(sessionID) == "" {
		return agent.ContextStructuralResult{}, false, nil
	}
	result, resumed, err := chat.ResumeRecoveredContextStructuralOperation(ctx, agent.RunOptions{
		AgentKind: agent.AgentKindIDE, Workspace: workspace, SessionID: sessionID, Mode: "ide",
	}, action)
	if !resumed || selected == nil {
		return result, resumed, err
	}
	// Cold restore commits through an independently loaded Session. Record the
	// obligation before refresh: if this read fails, every later Start, session
	// mutation, and exact structural retry remains fenced from stale state.
	recoveryAction := agent.RuntimeRecoveryAction{
		Kind: runtimeRecoveryActionForStructural(action),
	}
	s.markRecoveryRefreshPending(workspace, sessionID, recoveryAction)
	// Observation failure can race with a canonical commit. Keep the
	// conservative obligation pending; the next exact retry or admission fence
	// will refresh safely even when the actor ultimately aborted before commit.
	if err != nil {
		return result, true, err
	}
	matched, refreshErr := s.retryRecoveryRefresh(ctx, workspace, sessionID, recoveryAction, selected.RefreshCanonical)
	if !matched && refreshErr == nil {
		_, refreshErr = s.retryAnyRecoveryRefresh(ctx, workspace, sessionID, selected.RefreshCanonical)
	}
	if refreshErr != nil {
		return result, true, fmt.Errorf("refresh recovered writing context: %w", refreshErr)
	}
	return result, true, nil
}

func runtimeRecoveryActionForStructural(action agent.ContextStructuralAction) agent.RuntimeRecoveryActionKind {
	switch action {
	case agent.ContextStructuralCompact:
		return agent.RuntimeRecoveryCompactContext
	case agent.ContextStructuralRemove:
		return agent.RuntimeRecoveryRemoveCompaction
	default:
		return ""
	}
}

func (s *InteractiveAppService) resumeStoryContextStructuralOperation(
	ctx context.Context,
	workspace, storyID, branchID string,
	action agent.ContextStructuralAction,
) (agent.ContextStructuralResult, bool, error) {
	if s == nil || s.app == nil {
		return agent.ContextStructuralResult{}, false, nil
	}
	s.app.mu.RLock()
	chat := s.app.chatService
	s.app.mu.RUnlock()
	if chat == nil {
		return agent.ContextStructuralResult{}, false, nil
	}
	return chat.ResumeRecoveredContextStructuralOperation(ctx, agent.RunOptions{
		AgentKind: agent.AgentKindInteractiveStory, Workspace: workspace,
		StoryID: storyID, BranchID: branchID, Mode: "interactive",
	}, action)
}

// restoreContextStructuralOperation rebuilds only deterministic canonical
// commit/reconcile code. The descriptor already contains the bounded prepared
// result, so cold recovery never invokes a model, tool, or current UI state.
func (a *App) restoreContextStructuralOperation(
	ctx context.Context,
	request agent.HarnessStructuralRestoreRequest,
) (agent.ContextStructuralSpec, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return agent.ContextStructuralSpec{}, err
	}
	operation, err := a.contextStructuralOperationForRestore(request)
	if err != nil {
		return agent.ContextStructuralSpec{}, err
	}
	plan := request.Plan
	plan.Mutation = append(json.RawMessage(nil), request.Plan.Mutation...)
	return agent.ContextStructuralSpec{
		CommandID: string(request.Snapshot.CommandID), Action: request.Plan.Action,
		Ref: request.Snapshot.Ref, Options: request.Options,
		Operation: operation, RestorePlan: &plan,
	}, nil
}

func (a *App) contextStructuralOperationForRestore(
	request agent.HarnessStructuralRestoreRequest,
) (agent.ContextStructuralOperation, error) {
	switch request.Plan.Domain {
	case agent.ContextStructuralDomainSession:
		return a.restoreSessionContextStructuralOperation(request)
	case agent.ContextStructuralDomainStory:
		return restoreStoryContextStructuralOperation(request)
	default:
		return nil, fmt.Errorf("unsupported structural context domain %q", request.Plan.Domain)
	}
}

func (a *App) restoreSessionContextStructuralOperation(
	request agent.HarnessStructuralRestoreRequest,
) (agent.ContextStructuralOperation, error) {
	binding := request.Binding
	if binding.Kind != agentruntime.BindingWriting || binding.Profile != agentruntime.ProfileWriting ||
		strings.TrimSpace(binding.SessionID) == "" || request.Snapshot.Ref.Resource != binding.SessionID {
		return nil, fmt.Errorf("structural Session restore binding does not match its resource")
	}
	dir, err := a.sessionDirectoryForBinding(binding)
	if err != nil {
		return nil, err
	}
	expectedRevision, err := parseSessionContextRevision(request.Snapshot.Ref.ExpectedRevision)
	if err != nil {
		return nil, err
	}
	plan := request.Plan
	switch plan.Action {
	case agent.ContextStructuralCompact:
		var record session.ContextCompaction
		if err := decodeContextStructuralMutation(plan.Mutation, &record); err != nil {
			return nil, err
		}
		if record.ID != plan.RecordID {
			return nil, fmt.Errorf("Session compaction mutation record id does not match restore plan")
		}
		return fixedContextStructuralOperation(plan,
			func(ctx context.Context) (agent.ContextStructuralReceipt, error) {
				committed, err := session.CommitStoredContextCompaction(ctx, dir, binding.SessionID, expectedRevision, record)
				if err != nil {
					return agent.ContextStructuralReceipt{}, err
				}
				if !sameSessionContextCompactionMutation(committed, record) {
					return agent.ContextStructuralReceipt{}, fmt.Errorf("canonical Session compaction differs from frozen restore mutation")
				}
				return agent.ContextStructuralReceipt{Revision: fmt.Sprintf("session-context:%d", committed.ContextRevision)}, nil
			},
			func(context.Context) (agent.ContextStructuralReceipt, bool, error) {
				committed, found, err := session.FindStoredContextCompaction(dir, binding.SessionID, plan.RecordID)
				if err != nil || !found {
					return agent.ContextStructuralReceipt{}, false, err
				}
				if !sameSessionContextCompactionMutation(committed, record) {
					return agent.ContextStructuralReceipt{}, false, fmt.Errorf("canonical Session compaction conflicts with frozen restore mutation")
				}
				return agent.ContextStructuralReceipt{Revision: fmt.Sprintf("session-context:%d", committed.ContextRevision)}, true, nil
			}), nil
	case agent.ContextStructuralRemove:
		var record session.ContextCompactionRemoval
		if err := decodeContextStructuralMutation(plan.Mutation, &record); err != nil {
			return nil, err
		}
		if record.ID != plan.RecordID {
			return nil, fmt.Errorf("Session compaction removal record id does not match restore plan")
		}
		return fixedContextStructuralOperation(plan,
			func(ctx context.Context) (agent.ContextStructuralReceipt, error) {
				committed, removed, err := session.CommitStoredContextCompactionRemoval(ctx, dir, binding.SessionID, expectedRevision, record)
				if err != nil {
					return agent.ContextStructuralReceipt{}, err
				}
				if !removed {
					return agent.ContextStructuralReceipt{}, fmt.Errorf("Session compaction disappeared before frozen removal commit")
				}
				if !sameSessionContextCompactionRemovalMutation(committed, record) {
					return agent.ContextStructuralReceipt{}, fmt.Errorf("canonical Session compaction removal differs from frozen restore mutation")
				}
				return agent.ContextStructuralReceipt{Revision: fmt.Sprintf("session-context:%d", committed.ContextRevision)}, nil
			},
			func(context.Context) (agent.ContextStructuralReceipt, bool, error) {
				committed, found, err := session.FindStoredContextCompactionRemoval(dir, binding.SessionID, plan.RecordID)
				if err != nil || !found {
					return agent.ContextStructuralReceipt{}, false, err
				}
				if !sameSessionContextCompactionRemovalMutation(committed, record) {
					return agent.ContextStructuralReceipt{}, false, fmt.Errorf("canonical Session compaction removal conflicts with frozen restore mutation")
				}
				return agent.ContextStructuralReceipt{Revision: fmt.Sprintf("session-context:%d", committed.ContextRevision)}, true, nil
			}), nil
	default:
		return nil, fmt.Errorf("unsupported Session structural action %q", plan.Action)
	}
}

func restoreStoryContextStructuralOperation(
	request agent.HarnessStructuralRestoreRequest,
) (agent.ContextStructuralOperation, error) {
	binding := request.Binding
	if binding.Kind != agentruntime.BindingGame || binding.Profile != agentruntime.ProfileGame ||
		strings.TrimSpace(binding.Workspace) == "" || strings.TrimSpace(binding.StoryID) == "" ||
		strings.TrimSpace(binding.BranchID) == "" || request.Snapshot.Ref.Resource != binding.StoryID+"/"+binding.BranchID {
		return nil, fmt.Errorf("structural Story restore binding does not match its resource")
	}
	expectedParent, err := parseStoryContextRevision(request.Snapshot.Ref.ExpectedRevision)
	if err != nil {
		return nil, err
	}
	store := interactive.NewStore(binding.Workspace)
	plan := request.Plan
	switch plan.Action {
	case agent.ContextStructuralCompact:
		var event interactive.ContextCompactionEvent
		if err := decodeContextStructuralMutation(plan.Mutation, &event); err != nil {
			return nil, err
		}
		if event.ID != plan.RecordID {
			return nil, fmt.Errorf("Story compaction mutation identity does not match restore plan")
		}
		event.ExpectedParentID = &expectedParent
		return fixedContextStructuralOperation(plan,
			func(context.Context) (agent.ContextStructuralReceipt, error) {
				committed, err := store.AppendContextCompaction(binding.StoryID, binding.BranchID, event)
				if err != nil {
					return agent.ContextStructuralReceipt{}, err
				}
				if !sameStoryContextCompactionMutation(committed, event) {
					return agent.ContextStructuralReceipt{}, fmt.Errorf("canonical Story compaction differs from frozen restore mutation")
				}
				return agent.ContextStructuralReceipt{Revision: "story-head:" + committed.ID}, nil
			},
			func(context.Context) (agent.ContextStructuralReceipt, bool, error) {
				committed, found, err := store.ContextCompactionByID(binding.StoryID, plan.RecordID)
				if err != nil || !found {
					return agent.ContextStructuralReceipt{}, false, err
				}
				if committed.BranchID != binding.BranchID || !sameStoryContextCompactionMutation(committed, event) {
					return agent.ContextStructuralReceipt{}, false, fmt.Errorf("canonical Story compaction conflicts with frozen restore mutation")
				}
				return agent.ContextStructuralReceipt{Revision: "story-head:" + committed.ID}, true, nil
			}), nil
	case agent.ContextStructuralRemove:
		var event interactive.ContextCompactionRemovalEvent
		if err := decodeContextStructuralMutation(plan.Mutation, &event); err != nil {
			return nil, err
		}
		if event.ID != plan.RecordID {
			return nil, fmt.Errorf("Story compaction removal identity does not match restore plan")
		}
		event.ExpectedParentID = &expectedParent
		return fixedContextStructuralOperation(plan,
			func(context.Context) (agent.ContextStructuralReceipt, error) {
				committed, err := store.AppendContextCompactionRemoval(binding.StoryID, binding.BranchID, event)
				if err != nil {
					return agent.ContextStructuralReceipt{}, err
				}
				if !sameStoryContextCompactionRemovalMutation(committed, event) {
					return agent.ContextStructuralReceipt{}, fmt.Errorf("canonical Story compaction removal differs from frozen restore mutation")
				}
				return agent.ContextStructuralReceipt{Revision: "story-head:" + committed.ID}, nil
			},
			func(context.Context) (agent.ContextStructuralReceipt, bool, error) {
				committed, found, err := store.ContextCompactionRemovalByID(binding.StoryID, plan.RecordID)
				if err != nil || !found {
					return agent.ContextStructuralReceipt{}, false, err
				}
				if committed.BranchID != binding.BranchID || !sameStoryContextCompactionRemovalMutation(committed, event) {
					return agent.ContextStructuralReceipt{}, false, fmt.Errorf("canonical Story compaction removal conflicts with frozen restore mutation")
				}
				return agent.ContextStructuralReceipt{Revision: "story-head:" + committed.ID}, true, nil
			}), nil
	default:
		return nil, fmt.Errorf("unsupported Story structural action %q", plan.Action)
	}
}

func fixedContextStructuralOperation(
	plan agent.ContextStructuralRestorePlan,
	commit func(context.Context) (agent.ContextStructuralReceipt, error),
	reconcile func(context.Context) (agent.ContextStructuralReceipt, bool, error),
) agent.ContextStructuralOperation {
	return contextStructuralOperationFuncs{
		prepare: func(ctx context.Context, _ agent.ContextStructuralIdentity, _ func(agent.Event)) (agent.ContextStructuralIntent, error) {
			if ctx != nil {
				if err := ctx.Err(); err != nil {
					return agent.ContextStructuralIntent{Result: plan.Result}, err
				}
			}
			return agent.ContextStructuralIntent{Hash: plan.IntentHash, Commit: plan.Commit, Result: plan.Result}, nil
		},
		commit: func(ctx context.Context, _ agent.ContextStructuralIdentity, intent agent.ContextStructuralIntent) (agent.ContextStructuralReceipt, error) {
			if intent.Hash != plan.IntentHash || intent.Commit != plan.Commit || !reflect.DeepEqual(intent.Result, plan.Result) {
				return agent.ContextStructuralReceipt{}, fmt.Errorf("structural context intent changed before frozen commit")
			}
			if !plan.Commit {
				return agent.ContextStructuralReceipt{}, fmt.Errorf("non-committing structural plan reached canonical commit")
			}
			return commit(ctx)
		},
		reconcile: func(ctx context.Context) (agent.ContextStructuralResult, agent.ContextStructuralReceipt, bool, error) {
			if !plan.Commit {
				return plan.Result, agent.ContextStructuralReceipt{}, false, nil
			}
			receipt, found, err := reconcile(ctx)
			return plan.Result, receipt, found, err
		},
	}
}

func newContextStructuralRestorePlan(
	domain agent.ContextStructuralDomain,
	action agent.ContextStructuralAction,
	binding agentruntime.BindingRef,
	ref agentruntime.ContextCompactionRef,
	recordID string,
	result agent.ContextStructuralResult,
	mutation any,
) (agent.ContextStructuralRestorePlan, error) {
	encoded, err := json.Marshal(mutation)
	if err != nil {
		return agent.ContextStructuralRestorePlan{}, fmt.Errorf("encode structural context mutation: %w", err)
	}
	hash, err := agent.ContextStructuralIntentHash(action, binding, ref.ExpectedRevision, recordID, encoded)
	if err != nil {
		return agent.ContextStructuralRestorePlan{}, err
	}
	return agent.ContextStructuralRestorePlan{
		Version: agent.ContextStructuralRestorePlanVersion, Domain: domain, Action: action,
		Commit: true, IntentHash: hash, RecordID: recordID, Result: result, Mutation: encoded,
	}, nil
}

func writingContextStructuralBinding(workspace, sessionID string) (agentruntime.BindingRef, error) {
	return agentruntime.BindingReference(agentruntime.WritingBinding{
		Workspace: workspace, SessionID: sessionID, Profile: agentruntime.ProfileWriting,
	})
}

func storyContextStructuralBinding(workspace, storyID, branchID string) (agentruntime.BindingRef, error) {
	return agentruntime.BindingReference(agentruntime.GameBinding{
		Workspace: workspace, StoryID: storyID, BranchID: branchID, Profile: agentruntime.ProfileGame,
	})
}

func parseSessionContextRevision(value string) (uint64, error) {
	const prefix = "session-context:"
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, prefix) {
		return 0, fmt.Errorf("invalid Session context revision %q", value)
	}
	revision, err := strconv.ParseUint(strings.TrimPrefix(value, prefix), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid Session context revision %q: %w", value, err)
	}
	return revision, nil
}

func parseStoryContextRevision(value string) (string, error) {
	const prefix = "story-head:"
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, prefix) {
		return "", fmt.Errorf("invalid Story context revision %q", value)
	}
	head := strings.TrimSpace(strings.TrimPrefix(value, prefix))
	if head == "root" {
		return "", nil
	}
	if head == "" {
		return "", fmt.Errorf("invalid Story context revision %q", value)
	}
	return head, nil
}

func decodeContextStructuralMutation(data json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode structural context mutation: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values are not allowed")
		}
		return fmt.Errorf("decode structural context mutation: %w", err)
	}
	return nil
}

func sameSessionContextCompactionMutation(actual, expected session.ContextCompaction) bool {
	actual.Type, expected.Type = "", ""
	actual.CreatedAt, expected.CreatedAt = time.Time{}, time.Time{}
	actual.ContextRevision, expected.ContextRevision = 0, 0
	return reflect.DeepEqual(actual, expected)
}

func sameSessionContextCompactionRemovalMutation(actual, expected session.ContextCompactionRemoval) bool {
	actual.Type, expected.Type = "", ""
	actual.CreatedAt, expected.CreatedAt = time.Time{}, time.Time{}
	actual.ContextRevision, expected.ContextRevision = 0, 0
	return reflect.DeepEqual(actual, expected)
}

func sameStoryContextCompactionMutation(actual, expected interactive.ContextCompactionEvent) bool {
	normalizeStoryCompactionEnvelope := func(event *interactive.ContextCompactionEvent) {
		event.V = 0
		event.Type = ""
		event.ParentID = ""
		event.BranchID = ""
		event.Ts = ""
		event.ExpectedParentID = nil
	}
	normalizeStoryCompactionEnvelope(&actual)
	normalizeStoryCompactionEnvelope(&expected)
	return reflect.DeepEqual(actual, expected)
}

func sameStoryContextCompactionRemovalMutation(actual, expected interactive.ContextCompactionRemovalEvent) bool {
	normalizeStoryCompactionEnvelope := func(event *interactive.ContextCompactionRemovalEvent) {
		event.V = 0
		event.Type = ""
		event.ParentID = ""
		event.BranchID = ""
		event.Ts = ""
		event.ExpectedParentID = nil
	}
	normalizeStoryCompactionEnvelope(&actual)
	normalizeStoryCompactionEnvelope(&expected)
	return reflect.DeepEqual(actual, expected)
}
