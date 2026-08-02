package automationapp

import (
	"context"
	apptask "denova/internal/app/task"
	"errors"
	"fmt"
	"time"

	"denova/internal/automation"
)

// processDurableBuiltInTrigger runs schedule and chapter evaluators through
// the same durable protocol as semantic triggers. The persisted candidate
// match is the claimed input; deterministic inbox/run IDs are the decision;
// trigger dedupe state advances only after action reconciliation.
func (s *Service) processDurableBuiltInTrigger(
	ctx context.Context,
	snap *automationWorkspaceSnapshot,
	store *automation.Store,
	now time.Time,
	listedTask automation.Task,
	listedTrigger automation.TriggerDefinition,
	stateKey string,
) (item automation.TriggerInboxItem, run automation.RunResult, processed bool, err error) {
	return s.processDurableBuiltInTriggerWithStarter(ctx, snap, store, now, listedTask, listedTrigger, stateKey, func(ctx context.Context, taskID, trigger, runID string, evidence []automation.TriggerEvidence) (*apptask.Task, automation.RunRecord, error) {
		return s.startTaskWithSourceRunID(ctx, snap, taskID, trigger, "", runID, evidence)
	})
}

func (s *Service) processDurableBuiltInTriggerWithStarter(
	ctx context.Context,
	snap *automationWorkspaceSnapshot,
	store *automation.Store,
	now time.Time,
	listedTask automation.Task,
	listedTrigger automation.TriggerDefinition,
	stateKey string,
	start durableTriggerRunStarter,
) (item automation.TriggerInboxItem, run automation.RunResult, processed bool, err error) {
	release, err := store.AcquireTriggerEvaluationLease(ctx, automationTaskStoreID(listedTask), stateKey)
	if err != nil {
		return automation.TriggerInboxItem{}, automation.RunResult{}, false, err
	}
	defer func() {
		if releaseErr := release(); err == nil && releaseErr != nil {
			err = releaseErr
		}
	}()

	task, err := store.Get(automationTaskStoreID(listedTask))
	if err != nil {
		return automation.TriggerInboxItem{}, automation.RunResult{}, false, err
	}
	trigger, ok := enabledBuiltInTrigger(task, listedTrigger.ID, listedTrigger.Type)
	if !ok {
		return automation.TriggerInboxItem{}, automation.RunResult{}, false, nil
	}
	state := task.TriggerState[stateKey]
	record, ready, err := s.claimOrResumeBuiltInTrigger(ctx, snap, store, now, task, trigger, stateKey, state)
	if err != nil || !ready {
		return automation.TriggerInboxItem{}, automation.RunResult{}, false, err
	}
	if record.Status == automation.TriggerEvaluationStatusClaimed {
		decision, match, decisionErr := automation.DeterministicTriggerDecision(record)
		if decisionErr != nil {
			return automation.TriggerInboxItem{}, automation.RunResult{}, false, decisionErr
		}
		record, err = store.DecideTriggerEvaluation(ctx, automationTaskStoreID(task), stateKey, record.ID, record.IntentHash, decision, &match, now)
		if err != nil {
			return automation.TriggerInboxItem{}, automation.RunResult{}, false, err
		}
	}
	if record.Status == automation.TriggerEvaluationStatusCompleted {
		return automation.TriggerInboxItem{}, automation.RunResult{}, true, nil
	}
	if record.Status != automation.TriggerEvaluationStatusDecided || record.Action == nil {
		return automation.TriggerInboxItem{}, automation.RunResult{}, false, fmt.Errorf("trigger evaluation %s has invalid decided state", record.ID)
	}
	item, run, processed, err = s.reconcileDurableTriggerAction(ctx, snap, store, task, record, start)
	if err != nil || !processed {
		return item, run, processed, err
	}
	if _, err = store.CompleteTriggerEvaluation(ctx, automationTaskStoreID(task), stateKey, record.ID, record.IntentHash, time.Now().UTC()); err != nil {
		return item, run, false, err
	}
	return item, run, true, nil
}

func (s *Service) claimOrResumeBuiltInTrigger(
	ctx context.Context,
	snap *automationWorkspaceSnapshot,
	store *automation.Store,
	now time.Time,
	task automation.Task,
	trigger automation.TriggerDefinition,
	stateKey string,
	state automation.TriggerState,
) (automation.TriggerEvaluationRecord, bool, error) {
	if existing := state.Evaluation; existing != nil && existing.Status != automation.TriggerEvaluationStatusCompleted {
		if existing.TaskID != task.ID || existing.TriggerID != trigger.ID || existing.TriggerType != trigger.Type {
			return automation.TriggerEvaluationRecord{}, false, fmt.Errorf("pending trigger evaluation does not belong to %s/%s/%s", task.ID, trigger.ID, trigger.Type)
		}
		return *existing, true, nil
	}
	match, nextState, matched, evaluationErr := s.evaluateTrigger(ctx, snap, now, "durable_trigger", task, trigger)
	if evaluationErr != nil {
		_, persistErr := store.UpdateTriggerState(automationTaskStoreID(task), stateKey, nextState)
		return automation.TriggerEvaluationRecord{}, false, errors.Join(evaluationErr, persistErr)
	}
	if !matched {
		_, err := store.UpdateTriggerState(automationTaskStoreID(task), stateKey, nextState)
		return automation.TriggerEvaluationRecord{}, false, err
	}
	legacyHandled, err := s.reconcileLegacyBuiltInTriggerState(store, task, trigger, stateKey, nextState, match)
	if err != nil || legacyHandled {
		return automation.TriggerEvaluationRecord{}, false, err
	}
	claim, err := automation.NewMatchedTriggerEvaluation(automation.MatchedTriggerIntent{
		Scope: task.Scope, ProjectID: snap.projectID, Workspace: snap.workspace, TaskID: task.ID,
		TriggerID: trigger.ID, TriggerType: trigger.Type, Match: match,
		ActionPolicy: automation.EffectiveActionPolicy(task, trigger),
		NotifyPolicy: automation.EffectiveNotifyPolicy(task, trigger),
	}, now)
	if err != nil {
		return automation.TriggerEvaluationRecord{}, false, err
	}
	record, disposition, err := store.ClaimTriggerEvaluation(ctx, automationTaskStoreID(task), stateKey, claim)
	if err != nil {
		return automation.TriggerEvaluationRecord{}, false, err
	}
	if disposition == automation.TriggerEvaluationReplayed {
		return record, false, nil
	}
	return record, true, nil
}

// reconcileLegacyBuiltInTriggerState is a one-way migration guard for inboxes
// created before deterministic action IDs existed. It records their completed
// fingerprint rather than creating a second notification after state loss.
func (s *Service) reconcileLegacyBuiltInTriggerState(store *automation.Store, task automation.Task, trigger automation.TriggerDefinition, stateKey string, nextState automation.TriggerState, match automation.TriggerMatch) (bool, error) {
	var found bool
	var existing automation.TriggerInboxItem
	var err error
	if trigger.Type == automation.TriggerTypeChapterBatch {
		existing, found, err = store.FindInboxItemByEvidence(task.ID, trigger.ID, match.Evidence)
	} else {
		existing, found, err = store.FindOpenInboxItem(task.ID, trigger.ID, match.Fingerprint)
	}
	if err != nil || !found {
		return false, err
	}
	nextState.LastObservationFingerprint = match.Fingerprint
	nextState.LastEvidenceFingerprint = match.Fingerprint
	nextState.LastMatchedAt = existing.CreatedAt
	_, err = store.UpdateTriggerState(automationTaskStoreID(task), stateKey, nextState)
	return true, err
}

func enabledBuiltInTrigger(task automation.Task, triggerID, triggerType string) (automation.TriggerDefinition, bool) {
	for _, trigger := range task.Triggers {
		if trigger.ID == triggerID && trigger.Type == triggerType && trigger.Enabled && (trigger.Type == automation.TriggerTypeSchedule || trigger.Type == automation.TriggerTypeChapterBatch) {
			return trigger, true
		}
	}
	return automation.TriggerDefinition{}, false
}
