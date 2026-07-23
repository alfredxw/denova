package automation

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"denova/internal/filelease"
)

// AcquireTriggerEvaluationLease serializes one task/state-key coordinator
// across goroutines and processes. The lease is intentionally held across the
// model call and action reconciliation; a process crash releases the OS lock,
// while the persisted state machine tells the next owner where to resume.
func (s *Store) AcquireTriggerEvaluationLease(ctx context.Context, taskID, stateKey string) (func() error, error) {
	taskID = strings.TrimSpace(taskID)
	stateKey = strings.TrimSpace(stateKey)
	if taskID == "" || stateKey == "" {
		return nil, fmt.Errorf("task id and trigger state key are required")
	}
	for _, location := range s.taskLocations() {
		path, err := location.store.pathForScope(location.scope)
		if err != nil {
			return nil, err
		}
		unlock := storePathLocks.Lock(path)
		tasks, readErr := location.store.readScope(location.scope)
		found := false
		if readErr == nil {
			for _, task := range tasks {
				if taskMatchesID(task, taskID) {
					found = true
					break
				}
			}
		}
		unlock()
		if readErr != nil {
			return nil, readErr
		}
		if !found {
			continue
		}
		leaseName := deterministicTriggerHash(taskID, stateKey) + ".lock"
		return filelease.Acquire(ctx, filepath.Join(filepath.Dir(path), ".trigger-leases", leaseName))
	}
	return nil, fmt.Errorf("automation task %s not found", taskID)
}

// ClaimTriggerEvaluation writes the observation before model evaluation. A
// semantically identical incomplete record is resumed, while completed state
// is returned as an exact-replay receipt.
func (s *Store) ClaimTriggerEvaluation(ctx context.Context, taskID, stateKey string, claim TriggerEvaluationRecord) (TriggerEvaluationRecord, TriggerEvaluationClaimDisposition, error) {
	if err := validateTriggerEvaluationClaim(taskID, claim); err != nil {
		return TriggerEvaluationRecord{}, "", err
	}
	disposition := TriggerEvaluationClaimed
	record, err := s.mutateTriggerEvaluation(ctx, taskID, stateKey, func(state TriggerState) (TriggerState, TriggerEvaluationRecord, bool, error) {
		if existing := state.Evaluation; existing != nil {
			if existing.ID == claim.ID {
				if existing.IntentHash != claim.IntentHash {
					return state, TriggerEvaluationRecord{}, false, triggerEvaluationConflict(claim.ID, "intent hash differs")
				}
				if existing.Status == TriggerEvaluationStatusCompleted {
					disposition = TriggerEvaluationReplayed
				} else {
					disposition = TriggerEvaluationResumed
				}
				return state, *existing, false, nil
			}
			if existing.Status != TriggerEvaluationStatusCompleted {
				// Finish the older durable command before admitting a newer
				// observation for the same state key.
				disposition = TriggerEvaluationResumed
				return state, *existing, false, nil
			}
		}
		canonical := claim
		canonical.Status = TriggerEvaluationStatusClaimed
		canonical.Decision = nil
		canonical.DecisionHash = ""
		canonical.Match = nil
		canonical.Action = nil
		canonical.DecidedAt = time.Time{}
		canonical.CompletedAt = time.Time{}
		canonical.ClaimedAt = canonical.ClaimedAt.UTC()
		canonical.UpdatedAt = canonical.ClaimedAt
		state.LastCheckedAt = canonical.ClaimedAt
		state.Evaluation = &canonical
		return state, canonical, true, nil
	})
	return record, disposition, err
}

// DecideTriggerEvaluation persists both the canonical model decision and every
// action identity before the app is allowed to touch inbox or run state.
func (s *Store) DecideTriggerEvaluation(ctx context.Context, taskID, stateKey, evaluationID, intentHash string, decision SemanticEvaluation, match *TriggerMatch, decidedAt time.Time) (TriggerEvaluationRecord, error) {
	evaluationID = strings.TrimSpace(evaluationID)
	intentHash = strings.TrimSpace(intentHash)
	if evaluationID == "" || intentHash == "" {
		return TriggerEvaluationRecord{}, fmt.Errorf("evaluation id and intent hash are required")
	}
	if decidedAt.IsZero() {
		decidedAt = time.Now().UTC()
	} else {
		decidedAt = decidedAt.UTC()
	}
	return s.mutateTriggerEvaluation(ctx, taskID, stateKey, func(state TriggerState) (TriggerState, TriggerEvaluationRecord, bool, error) {
		if state.Evaluation == nil || state.Evaluation.ID != evaluationID || state.Evaluation.IntentHash != intentHash {
			return state, TriggerEvaluationRecord{}, false, triggerEvaluationConflict(evaluationID, "claim identity differs")
		}
		record := *state.Evaluation
		canonicalDecision, decisionErr := ValidateSemanticEvaluationEvidence(decision, record.Context)
		if decisionErr != nil {
			return state, TriggerEvaluationRecord{}, false, fmt.Errorf("invalid semantic trigger decision: %w", decisionErr)
		}
		decision = canonicalDecision
		var canonicalMatch *TriggerMatch
		var action *TriggerActionPlan
		if match != nil {
			copyMatch := *match
			copyMatch.TaskID = strings.TrimSpace(copyMatch.TaskID)
			copyMatch.TriggerID = strings.TrimSpace(copyMatch.TriggerID)
			copyMatch.Fingerprint = strings.TrimSpace(copyMatch.Fingerprint)
			if copyMatch.TaskID == "" || copyMatch.TriggerID == "" || copyMatch.Fingerprint == "" || !TaskMatchesID(Task{ID: record.TaskID}, copyMatch.TaskID) || copyMatch.TriggerID != record.TriggerID {
				return state, TriggerEvaluationRecord{}, false, fmt.Errorf("semantic trigger match does not belong to evaluation %s", evaluationID)
			}
			canonicalMatch = &copyMatch
			plan, err := NewTriggerActionPlan(record, copyMatch)
			if err != nil {
				return state, TriggerEvaluationRecord{}, false, err
			}
			action = &plan
		}
		decisionHash := triggerEvaluationDecisionHash(decision, canonicalMatch, action)
		switch record.Status {
		case TriggerEvaluationStatusClaimed:
			copyDecision := decision
			record.Status = TriggerEvaluationStatusDecided
			record.Decision = &copyDecision
			record.DecisionHash = decisionHash
			record.Match = canonicalMatch
			record.Action = action
			record.DecidedAt = decidedAt
			record.UpdatedAt = decidedAt
			state.Evaluation = &record
			return state, record, true, nil
		case TriggerEvaluationStatusDecided, TriggerEvaluationStatusCompleted:
			if record.DecisionHash != decisionHash {
				return state, TriggerEvaluationRecord{}, false, triggerEvaluationConflict(evaluationID, "decision differs")
			}
			return state, record, false, nil
		default:
			return state, TriggerEvaluationRecord{}, false, triggerEvaluationConflict(evaluationID, "invalid persisted status")
		}
	})
}

// CompleteTriggerEvaluation is the durable output barrier. Last-observation
// dedupe state advances only after every planned external effect succeeded (or
// an unmatched decision required no effect).
func (s *Store) CompleteTriggerEvaluation(ctx context.Context, taskID, stateKey, evaluationID, intentHash string, completedAt time.Time) (TriggerEvaluationRecord, error) {
	evaluationID = strings.TrimSpace(evaluationID)
	intentHash = strings.TrimSpace(intentHash)
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	} else {
		completedAt = completedAt.UTC()
	}
	return s.mutateTriggerEvaluation(ctx, taskID, stateKey, func(state TriggerState) (TriggerState, TriggerEvaluationRecord, bool, error) {
		if state.Evaluation == nil || state.Evaluation.ID != evaluationID || state.Evaluation.IntentHash != intentHash {
			return state, TriggerEvaluationRecord{}, false, triggerEvaluationConflict(evaluationID, "completion identity differs")
		}
		record := *state.Evaluation
		if record.Status == TriggerEvaluationStatusCompleted {
			return state, record, false, nil
		}
		if record.Status != TriggerEvaluationStatusDecided || record.Decision == nil {
			return state, TriggerEvaluationRecord{}, false, triggerEvaluationConflict(evaluationID, "evaluation is not decided")
		}
		record.Status = TriggerEvaluationStatusCompleted
		record.CompletedAt = completedAt
		record.UpdatedAt = completedAt
		state.LastObservationFingerprint = record.ObservationFingerprint
		if record.Match != nil {
			state.LastMatchedAt = completedAt
			state.LastEvidenceFingerprint = record.Match.Fingerprint
		}
		state.Evaluation = &record
		return state, record, true, nil
	})
}

// EnsureInboxItem creates a deterministic inbox action once. Mutable handling
// fields (status/read/run timestamps) do not change the immutable intent used
// for exact replay.
func (s *Store) EnsureInboxItem(ctx context.Context, item TriggerInboxItem) (TriggerInboxItem, bool, error) {
	if strings.TrimSpace(item.ID) == "" {
		return TriggerInboxItem{}, false, fmt.Errorf("deterministic inbox item id is required")
	}
	if strings.TrimSpace(item.Workspace) == "" && strings.TrimSpace(s.workspace) != "" {
		item.Workspace = s.workspace
	}
	normalized, err := NormalizeInboxItem(item)
	if err != nil {
		return TriggerInboxItem{}, false, err
	}
	destination := s
	if normalized.Scope == ScopeWorkspace && strings.TrimSpace(normalized.Workspace) != "" {
		destination = NewStore(s.userDir, normalized.Workspace)
	}
	path, err := destination.inboxPathForScope(normalized.Scope)
	if err != nil {
		return TriggerInboxItem{}, false, err
	}
	unlock := storePathLocks.Lock(path)
	defer unlock()
	release, err := filelease.Acquire(ctx, path+".lock")
	if err != nil {
		return TriggerInboxItem{}, false, err
	}
	defer func() { _ = release() }()
	items, err := destination.readInboxScope(normalized.Scope)
	if err != nil {
		return TriggerInboxItem{}, false, err
	}
	for _, existing := range items {
		if existing.ID != normalized.ID {
			continue
		}
		if triggerInboxIntentHash(existing) != triggerInboxIntentHash(normalized) {
			return TriggerInboxItem{}, false, fmt.Errorf("%w: inbox_id=%s", ErrTriggerActionConflict, normalized.ID)
		}
		return existing, false, nil
	}
	items = append([]TriggerInboxItem{normalized}, items...)
	items = boundInboxProjection(items)
	if err := destination.writeInboxScope(normalized.Scope, items); err != nil {
		return TriggerInboxItem{}, false, err
	}
	return normalized, true, nil
}

func (s *Store) mutateTriggerEvaluation(ctx context.Context, taskID, stateKey string, mutate func(TriggerState) (TriggerState, TriggerEvaluationRecord, bool, error)) (TriggerEvaluationRecord, error) {
	taskID = strings.TrimSpace(taskID)
	stateKey = strings.TrimSpace(stateKey)
	if taskID == "" || stateKey == "" {
		return TriggerEvaluationRecord{}, fmt.Errorf("task id and trigger state key are required")
	}
	type mutationResult struct {
		record TriggerEvaluationRecord
		found  bool
	}
	for _, location := range s.taskLocations() {
		path, err := location.store.pathForScope(location.scope)
		if err != nil {
			return TriggerEvaluationRecord{}, err
		}
		result, err := withTaskStoreWriteLease(ctx, path, func() (mutationResult, error) {
			tasks, readErr := location.store.readScope(location.scope)
			if readErr != nil {
				return mutationResult{}, readErr
			}
			for index := range tasks {
				if !taskMatchesID(tasks[index], taskID) {
					continue
				}
				state := tasks[index].TriggerState[stateKey]
				next, record, changed, mutateErr := mutate(state)
				if mutateErr != nil {
					return mutationResult{}, mutateErr
				}
				if changed {
					if tasks[index].TriggerState == nil {
						tasks[index].TriggerState = map[string]TriggerState{}
					}
					tasks[index].TriggerState[stateKey] = next
					tasks[index].UpdatedAt = time.Now().UTC()
					normalized, normalizeErr := location.store.normalizeTaskTarget(tasks[index])
					if normalizeErr != nil {
						return mutationResult{}, normalizeErr
					}
					tasks[index] = normalized
					if writeErr := location.store.writeScope(location.scope, tasks); writeErr != nil {
						return mutationResult{}, writeErr
					}
				}
				return mutationResult{record: record, found: true}, nil
			}
			return mutationResult{}, nil
		})
		if err != nil {
			return TriggerEvaluationRecord{}, err
		}
		if result.found {
			return result.record, nil
		}
	}
	return TriggerEvaluationRecord{}, fmt.Errorf("automation task %s not found", taskID)
}

func validateTriggerEvaluationClaim(taskID string, claim TriggerEvaluationRecord) error {
	if strings.TrimSpace(claim.ID) == "" || strings.TrimSpace(claim.IntentHash) == "" || strings.TrimSpace(claim.ObservationFingerprint) == "" {
		return fmt.Errorf("complete trigger claim identity is required")
	}
	localTaskID := strings.TrimSpace(claim.TaskID)
	taskID = strings.TrimSpace(taskID)
	if taskID != localTaskID && taskID != CatalogTaskID(claim.Scope, claim.Workspace, localTaskID) {
		return fmt.Errorf("semantic trigger claim belongs to a different task")
	}
	if claim.Status != TriggerEvaluationStatusClaimed || claim.ClaimedAt.IsZero() {
		return fmt.Errorf("trigger claim must be newly claimed")
	}
	switch claim.TriggerType {
	case TriggerTypeSemantic:
		if strings.TrimSpace(claim.Instruction) == "" || strings.TrimSpace(claim.Condition) == "" || claim.CandidateMatch != nil {
			return fmt.Errorf("semantic trigger claim requires condition and instruction only")
		}
	case TriggerTypeSchedule, TriggerTypeChapterBatch:
		if claim.CandidateMatch == nil || strings.TrimSpace(claim.Instruction) != "" {
			return fmt.Errorf("deterministic trigger claim requires a persisted candidate match")
		}
	default:
		return fmt.Errorf("unsupported trigger claim type %q", claim.TriggerType)
	}
	return nil
}

func triggerEvaluationConflict(evaluationID, reason string) error {
	return fmt.Errorf("%w: evaluation_id=%s reason=%s", ErrTriggerEvaluationConflict, strings.TrimSpace(evaluationID), reason)
}

func triggerInboxIntentHash(item TriggerInboxItem) string {
	return deterministicTriggerHash(struct {
		TaskID       string            `json:"task_id"`
		TriggerID    string            `json:"trigger_id"`
		Purpose      string            `json:"purpose"`
		Scope        string            `json:"scope"`
		Workspace    string            `json:"workspace,omitempty"`
		ActionPolicy string            `json:"action_policy"`
		NotifyPolicy string            `json:"notify_policy"`
		Title        string            `json:"title"`
		Summary      string            `json:"summary"`
		Evidence     []TriggerEvidence `json:"evidence"`
		Fingerprint  string            `json:"fingerprint"`
	}{
		strings.TrimSpace(item.TaskID), strings.TrimSpace(item.TriggerID), normalizeInboxPurpose(item.Purpose), item.Scope,
		canonicalStoreRoot(item.Workspace), normalizeActionPolicy(item.ActionPolicy, ActionPolicyConfirm), normalizeNotifyPolicy(item.NotifyPolicy, NotifyPolicyInbox),
		strings.TrimSpace(item.Title), strings.TrimSpace(item.Summary), item.Evidence, strings.TrimSpace(item.Fingerprint),
	})
}
