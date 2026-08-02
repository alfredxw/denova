package compactionapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	agentstructural "denova/internal/agents/context/structural"
	agentharness "denova/internal/agents/harness"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/session"
	"denova/internal/interactive"
)

type SessionDirectoryResolver func(agentrun.RuntimeBinding) (string, error)

// RestoreSpec rebuilds deterministic canonical commit/reconcile behavior from
// a frozen mutation. Cold recovery never invokes a model, tool, or current UI
// state through this path.
func RestoreSpec(ctx context.Context, request agentharness.StructuralRestoreRequest, sessionDirectory SessionDirectoryResolver) (agentstructural.Spec, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return agentstructural.Spec{}, err
	}
	operation, err := restoreOperation(request, sessionDirectory)
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

// RestoreOperation exposes the same deterministic operation to domain-commit
// reconciliation, which already owns the surrounding runtime snapshot.
func RestoreOperation(request agentharness.StructuralRestoreRequest, sessionDirectory SessionDirectoryResolver) (agentstructural.Operation, error) {
	return restoreOperation(request, sessionDirectory)
}

func restoreOperation(request agentharness.StructuralRestoreRequest, sessionDirectory SessionDirectoryResolver) (agentstructural.Operation, error) {
	switch request.Plan.Domain {
	case agentstructural.DomainSession:
		return restoreSessionOperation(request, sessionDirectory)
	case agentstructural.DomainStory:
		return restoreStoryOperation(request)
	default:
		return nil, fmt.Errorf("unsupported structural context domain %q", request.Plan.Domain)
	}
}

func restoreSessionOperation(request agentharness.StructuralRestoreRequest, sessionDirectory SessionDirectoryResolver) (agentstructural.Operation, error) {
	binding := request.Binding
	if (binding.AgentKind != agentrun.AgentKindGeneral && binding.AgentKind != agentrun.AgentKindIDE &&
		binding.AgentKind != agentrun.AgentKindConfigManager && binding.AgentKind != agentrun.AgentKindImage) ||
		strings.TrimSpace(binding.SessionID) == "" || request.Snapshot.Ref.Resource != binding.SessionID {
		return nil, fmt.Errorf("structural Session restore binding does not match its resource")
	}
	if sessionDirectory == nil {
		return nil, fmt.Errorf("Session restore directory resolver is unavailable")
	}
	directory, err := sessionDirectory(request.Binding)
	if err != nil {
		return nil, err
	}
	expectedRevision, err := parseSessionRevision(request.Snapshot.Ref.ExpectedRevision)
	if err != nil {
		return nil, err
	}
	plan := request.Plan
	switch plan.Action {
	case agentstructural.Compact:
		var record session.ContextCompaction
		if err := decodeMutation(plan.Mutation, &record); err != nil {
			return nil, err
		}
		if record.ID != plan.RecordID {
			return nil, fmt.Errorf("Session compaction mutation record id does not match restore plan")
		}
		return agentstructural.FixedOperation(plan,
			func(ctx context.Context) (agentstructural.Receipt, error) {
				committed, err := session.CommitStoredContextCompaction(ctx, directory, binding.SessionID, expectedRevision, record)
				if err != nil {
					return agentstructural.Receipt{}, err
				}
				if !SameSessionMutation(committed, record) {
					return agentstructural.Receipt{}, fmt.Errorf("canonical Session compaction differs from frozen restore mutation")
				}
				return agentstructural.Receipt{Revision: SessionRevision(committed.ContextRevision)}, nil
			},
			func(context.Context) (agentstructural.Receipt, bool, error) {
				committed, found, err := session.FindStoredContextCompaction(directory, binding.SessionID, plan.RecordID)
				if err != nil || !found {
					return agentstructural.Receipt{}, false, err
				}
				if !SameSessionMutation(committed, record) {
					return agentstructural.Receipt{}, false, fmt.Errorf("canonical Session compaction conflicts with frozen restore mutation")
				}
				return agentstructural.Receipt{Revision: SessionRevision(committed.ContextRevision)}, true, nil
			}), nil
	case agentstructural.Remove:
		var record session.ContextCompactionRemoval
		if err := decodeMutation(plan.Mutation, &record); err != nil {
			return nil, err
		}
		if record.ID != plan.RecordID {
			return nil, fmt.Errorf("Session compaction removal record id does not match restore plan")
		}
		return agentstructural.FixedOperation(plan,
			func(ctx context.Context) (agentstructural.Receipt, error) {
				committed, removed, err := session.CommitStoredContextCompactionRemoval(ctx, directory, binding.SessionID, expectedRevision, record)
				if err != nil {
					return agentstructural.Receipt{}, err
				}
				if !removed {
					return agentstructural.Receipt{}, fmt.Errorf("Session compaction disappeared before frozen removal commit")
				}
				if !SameSessionRemoval(committed, record) {
					return agentstructural.Receipt{}, fmt.Errorf("canonical Session compaction removal differs from frozen restore mutation")
				}
				return agentstructural.Receipt{Revision: SessionRevision(committed.ContextRevision)}, nil
			},
			func(context.Context) (agentstructural.Receipt, bool, error) {
				committed, found, err := session.FindStoredContextCompactionRemoval(directory, binding.SessionID, plan.RecordID)
				if err != nil || !found {
					return agentstructural.Receipt{}, false, err
				}
				if !SameSessionRemoval(committed, record) {
					return agentstructural.Receipt{}, false, fmt.Errorf("canonical Session compaction removal conflicts with frozen restore mutation")
				}
				return agentstructural.Receipt{Revision: SessionRevision(committed.ContextRevision)}, true, nil
			}), nil
	default:
		return nil, fmt.Errorf("unsupported Session structural action %q", plan.Action)
	}
}

func restoreStoryOperation(request agentharness.StructuralRestoreRequest) (agentstructural.Operation, error) {
	binding := request.Binding
	if binding.AgentKind != agentrun.AgentKindInteractiveStory ||
		strings.TrimSpace(binding.Workspace) == "" || strings.TrimSpace(binding.StoryID) == "" ||
		strings.TrimSpace(binding.BranchID) == "" || request.Snapshot.Ref.Resource != binding.StoryID+"/"+binding.BranchID {
		return nil, fmt.Errorf("structural Story restore binding does not match its resource")
	}
	expectedParent, err := parseStoryRevision(request.Snapshot.Ref.ExpectedRevision)
	if err != nil {
		return nil, err
	}
	store := interactive.NewStore(binding.Workspace)
	plan := request.Plan
	switch plan.Action {
	case agentstructural.Compact:
		var event interactive.ContextCompactionEvent
		if err := decodeMutation(plan.Mutation, &event); err != nil {
			return nil, err
		}
		if event.ID != plan.RecordID {
			return nil, fmt.Errorf("Story compaction mutation identity does not match restore plan")
		}
		event.ExpectedParentID = &expectedParent
		return agentstructural.FixedOperation(plan,
			func(context.Context) (agentstructural.Receipt, error) {
				committed, err := store.AppendContextCompaction(binding.StoryID, binding.BranchID, event)
				if err != nil {
					return agentstructural.Receipt{}, err
				}
				if !interactive.SameContextCompactionMutation(committed, event) {
					return agentstructural.Receipt{}, fmt.Errorf("canonical Story compaction differs from frozen restore mutation")
				}
				return agentstructural.Receipt{Revision: "story-head:" + committed.ID}, nil
			},
			func(context.Context) (agentstructural.Receipt, bool, error) {
				committed, found, err := store.ContextCompactionByID(binding.StoryID, plan.RecordID)
				if err != nil || !found {
					return agentstructural.Receipt{}, false, err
				}
				if committed.BranchID != binding.BranchID || !interactive.SameContextCompactionMutation(committed, event) {
					return agentstructural.Receipt{}, false, fmt.Errorf("canonical Story compaction conflicts with frozen restore mutation")
				}
				return agentstructural.Receipt{Revision: "story-head:" + committed.ID}, true, nil
			}), nil
	case agentstructural.Remove:
		var event interactive.ContextCompactionRemovalEvent
		if err := decodeMutation(plan.Mutation, &event); err != nil {
			return nil, err
		}
		if event.ID != plan.RecordID {
			return nil, fmt.Errorf("Story compaction removal identity does not match restore plan")
		}
		event.ExpectedParentID = &expectedParent
		return agentstructural.FixedOperation(plan,
			func(context.Context) (agentstructural.Receipt, error) {
				committed, err := store.AppendContextCompactionRemoval(binding.StoryID, binding.BranchID, event)
				if err != nil {
					return agentstructural.Receipt{}, err
				}
				if !interactive.SameContextCompactionRemovalMutation(committed, event) {
					return agentstructural.Receipt{}, fmt.Errorf("canonical Story compaction removal differs from frozen restore mutation")
				}
				return agentstructural.Receipt{Revision: "story-head:" + committed.ID}, nil
			},
			func(context.Context) (agentstructural.Receipt, bool, error) {
				committed, found, err := store.ContextCompactionRemovalByID(binding.StoryID, plan.RecordID)
				if err != nil || !found {
					return agentstructural.Receipt{}, false, err
				}
				if committed.BranchID != binding.BranchID || !interactive.SameContextCompactionRemovalMutation(committed, event) {
					return agentstructural.Receipt{}, false, fmt.Errorf("canonical Story compaction removal conflicts with frozen restore mutation")
				}
				return agentstructural.Receipt{Revision: "story-head:" + committed.ID}, true, nil
			}), nil
	default:
		return nil, fmt.Errorf("unsupported Story structural action %q", plan.Action)
	}
}

func parseSessionRevision(value string) (uint64, error) {
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

func parseStoryRevision(value string) (string, error) {
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

func decodeMutation(data json.RawMessage, target any) error {
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
