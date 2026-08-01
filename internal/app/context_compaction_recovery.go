package app

import (
	"bytes"
	"context"
	agentstructural "denova/internal/agents/context/structural"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"

	"denova/internal/agents/session"
	"denova/internal/interactive"
)

func (s *ChatAppService) resumeWritingContextStructuralOperation(
	ctx context.Context,
	action agentstructural.Action,
) (agentstructural.Result, bool, error) {
	if s == nil || s.app == nil {
		return agentstructural.Result{}, false, nil
	}
	a := s.app
	a.mu.RLock()
	chat := a.chatService
	workspace := a.workspace
	stateRoot := ""
	if a.cfg != nil {
		stateRoot = a.cfg.ProjectStateDir
	}
	sessionID := ""
	selected := a.session
	if a.session != nil {
		sessionID = a.session.ID
	}
	a.mu.RUnlock()
	if chat == nil || strings.TrimSpace(workspace) == "" || strings.TrimSpace(sessionID) == "" {
		return agentstructural.Result{}, false, nil
	}
	result, resumed, err := chat.ResumeRecoveredStructuralOperation(ctx, agentrun.Options{
		AgentKind: agentrun.AgentKindIDE, StateRoot: stateRoot, Workspace: workspace, SessionID: sessionID, Mode: "ide",
	}, action)
	if !resumed || selected == nil {
		return result, resumed, err
	}
	// Cold restore commits through an independently loaded Session. Record the
	// obligation before refresh: if this read fails, every later Start, session
	// mutation, and exact structural retry remains fenced from stale state.
	recoveryAction := agentharness.RuntimeRecoveryAction{
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

func runtimeRecoveryActionForStructural(action agentstructural.Action) agentharness.RuntimeRecoveryActionKind {
	switch action {
	case agentstructural.Compact:
		return agentharness.RuntimeRecoveryCompactContext
	case agentstructural.Remove:
		return agentharness.RuntimeRecoveryRemoveCompaction
	default:
		return ""
	}
}

func (s *InteractiveAppService) resumeStoryContextStructuralOperation(
	ctx context.Context,
	workspace, storyID, branchID string,
	action agentstructural.Action,
) (agentstructural.Result, bool, error) {
	if s == nil || s.app == nil {
		return agentstructural.Result{}, false, nil
	}
	s.app.mu.RLock()
	chat := s.app.chatService
	s.app.mu.RUnlock()
	if chat == nil {
		return agentstructural.Result{}, false, nil
	}
	return chat.ResumeRecoveredStructuralOperation(ctx, agentrun.Options{
		AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace,
		StoryID: storyID, BranchID: branchID, Mode: "interactive",
	}, action)
}

// restoreContextStructuralOperation rebuilds only deterministic canonical
// commit/reconcile code. The descriptor already contains the bounded prepared
// result, so cold recovery never invokes a model, tool, or current UI state.
func (a *App) restoreContextStructuralOperation(
	ctx context.Context,
	request agentharness.StructuralRestoreRequest,
) (agentstructural.Spec, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return agentstructural.Spec{}, err
	}
	operation, err := a.contextStructuralOperationForRestore(request)
	if err != nil {
		return agentstructural.Spec{}, err
	}
	plan := request.Plan
	plan.Mutation = append(json.RawMessage(nil), request.Plan.Mutation...)
	return agentstructural.Spec{
		CommandID: string(request.Snapshot.CommandID), Action: request.Plan.Action,
		Ref: request.Snapshot.Ref, Options: request.Options,
		Operation: operation, RestorePlan: &plan,
	}, nil
}

func (a *App) contextStructuralOperationForRestore(
	request agentharness.StructuralRestoreRequest,
) (agentstructural.Operation, error) {
	switch request.Plan.Domain {
	case agentstructural.DomainSession:
		return a.restoreSessionContextStructuralOperation(request)
	case agentstructural.DomainStory:
		return restoreStoryContextStructuralOperation(request)
	default:
		return nil, fmt.Errorf("unsupported structural context domain %q", request.Plan.Domain)
	}
}

func (a *App) restoreSessionContextStructuralOperation(
	request agentharness.StructuralRestoreRequest,
) (agentstructural.Operation, error) {
	binding := request.Binding
	if (binding.AgentKind != agentrun.AgentKindGeneral && binding.AgentKind != agentrun.AgentKindIDE &&
		binding.AgentKind != agentrun.AgentKindConfigManager && binding.AgentKind != agentrun.AgentKindImage) ||
		strings.TrimSpace(binding.SessionID) == "" || request.Snapshot.Ref.Resource != binding.SessionID {
		return nil, fmt.Errorf("structural Session restore binding does not match its resource")
	}
	dir, err := a.sessionDirectoryForBinding(request.Binding)
	if err != nil {
		return nil, err
	}
	expectedRevision, err := parseSessionContextRevision(request.Snapshot.Ref.ExpectedRevision)
	if err != nil {
		return nil, err
	}
	plan := request.Plan
	switch plan.Action {
	case agentstructural.Compact:
		var record session.ContextCompaction
		if err := decodeContextStructuralMutation(plan.Mutation, &record); err != nil {
			return nil, err
		}
		if record.ID != plan.RecordID {
			return nil, fmt.Errorf("Session compaction mutation record id does not match restore plan")
		}
		return fixedContextStructuralOperation(plan,
			func(ctx context.Context) (agentstructural.Receipt, error) {
				committed, err := session.CommitStoredContextCompaction(ctx, dir, binding.SessionID, expectedRevision, record)
				if err != nil {
					return agentstructural.Receipt{}, err
				}
				if !sameSessionContextCompactionMutation(committed, record) {
					return agentstructural.Receipt{}, fmt.Errorf("canonical Session compaction differs from frozen restore mutation")
				}
				return agentstructural.Receipt{Revision: fmt.Sprintf("session-context:%d", committed.ContextRevision)}, nil
			},
			func(context.Context) (agentstructural.Receipt, bool, error) {
				committed, found, err := session.FindStoredContextCompaction(dir, binding.SessionID, plan.RecordID)
				if err != nil || !found {
					return agentstructural.Receipt{}, false, err
				}
				if !sameSessionContextCompactionMutation(committed, record) {
					return agentstructural.Receipt{}, false, fmt.Errorf("canonical Session compaction conflicts with frozen restore mutation")
				}
				return agentstructural.Receipt{Revision: fmt.Sprintf("session-context:%d", committed.ContextRevision)}, true, nil
			}), nil
	case agentstructural.Remove:
		var record session.ContextCompactionRemoval
		if err := decodeContextStructuralMutation(plan.Mutation, &record); err != nil {
			return nil, err
		}
		if record.ID != plan.RecordID {
			return nil, fmt.Errorf("Session compaction removal record id does not match restore plan")
		}
		return fixedContextStructuralOperation(plan,
			func(ctx context.Context) (agentstructural.Receipt, error) {
				committed, removed, err := session.CommitStoredContextCompactionRemoval(ctx, dir, binding.SessionID, expectedRevision, record)
				if err != nil {
					return agentstructural.Receipt{}, err
				}
				if !removed {
					return agentstructural.Receipt{}, fmt.Errorf("Session compaction disappeared before frozen removal commit")
				}
				if !sameSessionContextCompactionRemovalMutation(committed, record) {
					return agentstructural.Receipt{}, fmt.Errorf("canonical Session compaction removal differs from frozen restore mutation")
				}
				return agentstructural.Receipt{Revision: fmt.Sprintf("session-context:%d", committed.ContextRevision)}, nil
			},
			func(context.Context) (agentstructural.Receipt, bool, error) {
				committed, found, err := session.FindStoredContextCompactionRemoval(dir, binding.SessionID, plan.RecordID)
				if err != nil || !found {
					return agentstructural.Receipt{}, false, err
				}
				if !sameSessionContextCompactionRemovalMutation(committed, record) {
					return agentstructural.Receipt{}, false, fmt.Errorf("canonical Session compaction removal conflicts with frozen restore mutation")
				}
				return agentstructural.Receipt{Revision: fmt.Sprintf("session-context:%d", committed.ContextRevision)}, true, nil
			}), nil
	default:
		return nil, fmt.Errorf("unsupported Session structural action %q", plan.Action)
	}
}

func restoreStoryContextStructuralOperation(
	request agentharness.StructuralRestoreRequest,
) (agentstructural.Operation, error) {
	binding := request.Binding
	if binding.AgentKind != agentrun.AgentKindInteractiveStory ||
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
	case agentstructural.Compact:
		var event interactive.ContextCompactionEvent
		if err := decodeContextStructuralMutation(plan.Mutation, &event); err != nil {
			return nil, err
		}
		if event.ID != plan.RecordID {
			return nil, fmt.Errorf("Story compaction mutation identity does not match restore plan")
		}
		event.ExpectedParentID = &expectedParent
		return fixedContextStructuralOperation(plan,
			func(context.Context) (agentstructural.Receipt, error) {
				committed, err := store.AppendContextCompaction(binding.StoryID, binding.BranchID, event)
				if err != nil {
					return agentstructural.Receipt{}, err
				}
				if !sameStoryContextCompactionMutation(committed, event) {
					return agentstructural.Receipt{}, fmt.Errorf("canonical Story compaction differs from frozen restore mutation")
				}
				return agentstructural.Receipt{Revision: "story-head:" + committed.ID}, nil
			},
			func(context.Context) (agentstructural.Receipt, bool, error) {
				committed, found, err := store.ContextCompactionByID(binding.StoryID, plan.RecordID)
				if err != nil || !found {
					return agentstructural.Receipt{}, false, err
				}
				if committed.BranchID != binding.BranchID || !sameStoryContextCompactionMutation(committed, event) {
					return agentstructural.Receipt{}, false, fmt.Errorf("canonical Story compaction conflicts with frozen restore mutation")
				}
				return agentstructural.Receipt{Revision: "story-head:" + committed.ID}, true, nil
			}), nil
	case agentstructural.Remove:
		var event interactive.ContextCompactionRemovalEvent
		if err := decodeContextStructuralMutation(plan.Mutation, &event); err != nil {
			return nil, err
		}
		if event.ID != plan.RecordID {
			return nil, fmt.Errorf("Story compaction removal identity does not match restore plan")
		}
		event.ExpectedParentID = &expectedParent
		return fixedContextStructuralOperation(plan,
			func(context.Context) (agentstructural.Receipt, error) {
				committed, err := store.AppendContextCompactionRemoval(binding.StoryID, binding.BranchID, event)
				if err != nil {
					return agentstructural.Receipt{}, err
				}
				if !sameStoryContextCompactionRemovalMutation(committed, event) {
					return agentstructural.Receipt{}, fmt.Errorf("canonical Story compaction removal differs from frozen restore mutation")
				}
				return agentstructural.Receipt{Revision: "story-head:" + committed.ID}, nil
			},
			func(context.Context) (agentstructural.Receipt, bool, error) {
				committed, found, err := store.ContextCompactionRemovalByID(binding.StoryID, plan.RecordID)
				if err != nil || !found {
					return agentstructural.Receipt{}, false, err
				}
				if committed.BranchID != binding.BranchID || !sameStoryContextCompactionRemovalMutation(committed, event) {
					return agentstructural.Receipt{}, false, fmt.Errorf("canonical Story compaction removal conflicts with frozen restore mutation")
				}
				return agentstructural.Receipt{Revision: "story-head:" + committed.ID}, true, nil
			}), nil
	default:
		return nil, fmt.Errorf("unsupported Story structural action %q", plan.Action)
	}
}

func fixedContextStructuralOperation(
	plan agentstructural.RestorePlan,
	commit func(context.Context) (agentstructural.Receipt, error),
	reconcile func(context.Context) (agentstructural.Receipt, bool, error),
) agentstructural.Operation {
	return contextStructuralOperationFuncs{
		prepare: func(ctx context.Context, _ agentstructural.Identity, _ func(agentrun.Event)) (agentstructural.Intent, error) {
			if ctx != nil {
				if err := ctx.Err(); err != nil {
					return agentstructural.Intent{Result: plan.Result}, err
				}
			}
			return agentstructural.Intent{Hash: plan.IntentHash, Commit: plan.Commit, Result: plan.Result}, nil
		},
		commit: func(ctx context.Context, _ agentstructural.Identity, intent agentstructural.Intent) (agentstructural.Receipt, error) {
			if intent.Hash != plan.IntentHash || intent.Commit != plan.Commit || !reflect.DeepEqual(intent.Result, plan.Result) {
				return agentstructural.Receipt{}, fmt.Errorf("structural context intent changed before frozen commit")
			}
			if !plan.Commit {
				return agentstructural.Receipt{}, fmt.Errorf("non-committing structural plan reached canonical commit")
			}
			return commit(ctx)
		},
		reconcile: func(ctx context.Context) (agentstructural.Result, agentstructural.Receipt, bool, error) {
			if !plan.Commit {
				return plan.Result, agentstructural.Receipt{}, false, nil
			}
			receipt, found, err := reconcile(ctx)
			return plan.Result, receipt, found, err
		},
	}
}

func newContextStructuralRestorePlan(
	domain agentstructural.Domain,
	action agentstructural.Action,
	binding agentrun.RuntimeBinding,
	ref agentrun.ContextCompactionRef,
	recordID string,
	result agentstructural.Result,
	mutation any,
) (agentstructural.RestorePlan, error) {
	encoded, err := json.Marshal(mutation)
	if err != nil {
		return agentstructural.RestorePlan{}, fmt.Errorf("encode structural context mutation: %w", err)
	}
	hash, err := agentstructural.IntentHash(action, binding, ref.ExpectedRevision, recordID, encoded)
	if err != nil {
		return agentstructural.RestorePlan{}, err
	}
	return agentstructural.RestorePlan{
		Version: agentstructural.RestorePlanVersion, Domain: domain, Action: action,
		Commit: true, IntentHash: hash, RecordID: recordID, Result: result, Mutation: encoded,
	}, nil
}

func writingContextStructuralBinding(workspace, sessionID string) agentrun.RuntimeBinding {
	return agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindIDE, Workspace: workspace, SessionID: sessionID}
}

func storyContextStructuralBinding(workspace, storyID, branchID string) agentrun.RuntimeBinding {
	return agentrun.RuntimeBinding{AgentKind: agentrun.AgentKindInteractiveStory, Workspace: workspace, StoryID: storyID, BranchID: branchID}
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
	// Cursor fields are a derived migration of legacy index bounds. A frozen
	// pre-commit mutation may contain only indices while the canonical record
	// read through the projection has stable cursors filled in.
	if expected.SourceStartCursor == 0 {
		actual.SourceStartCursor = 0
	}
	if expected.SourceEndCursor == 0 {
		actual.SourceEndCursor = 0
	}
	return reflect.DeepEqual(actual, expected)
}

func sameSessionContextCompactionRemovalMutation(actual, expected session.ContextCompactionRemoval) bool {
	actual.Type, expected.Type = "", ""
	actual.CreatedAt, expected.CreatedAt = time.Time{}, time.Time{}
	actual.ContextRevision, expected.ContextRevision = 0, 0
	if expected.SourceStartCursor == 0 {
		actual.SourceStartCursor = 0
	}
	if expected.SourceEndCursor == 0 {
		actual.SourceEndCursor = 0
	}
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
