package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	agents "denova/internal/agents"
	"denova/internal/automation"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

const admittedToolMutationVersion = 1

// admittedToolMutationPayload is the application-owned representation of a
// Runtime host effect. Runtime keeps the original outbox until this exact
// payload is durable; the application store then owns all slower reconciliation.
type admittedToolMutationPayload struct {
	Version          int                       `json:"version"`
	Binding          runstate.BindingRef       `json:"binding"`
	RuntimeOperation runstate.OperationID      `json:"runtime_operation"`
	RuntimeCycle     int                       `json:"runtime_cycle"`
	ToolCallID       string                    `json:"tool_call_id"`
	Origin           agents.ToolMutationOrigin `json:"origin"`
	Mutation         agents.ToolMutation       `json:"mutation"`
}

func (a *App) reconcileHarnessHostEffect(ctx context.Context, committed agents.CommittedToolMutation) error {
	if a == nil || a.cfg == nil {
		return fmt.Errorf("admit agent host effect: app configuration is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := json.Marshal(admittedToolMutationPayload{
		Version: admittedToolMutationVersion, Binding: committed.Binding,
		RuntimeOperation: committed.RuntimeOperation, RuntimeCycle: committed.RuntimeCycle,
		ToolCallID: committed.ToolCallID, Origin: committed.Origin, Mutation: committed.Mutation,
	})
	if err != nil {
		return fmt.Errorf("encode agent host effect %q: %w", committed.EffectID, err)
	}
	workspace := strings.TrimSpace(committed.Mutation.Workspace)
	if workspace == "" {
		workspace = strings.TrimSpace(committed.Origin.Workspace)
	}
	store := automation.NewStore(a.cfg.DataDir(), "")
	admitted, err := store.AdmitHostEffect(ctx, automation.HostEffectObligation{
		ID: string(committed.EffectID), Kind: string(runstate.HostEffectToolMutationCommitted),
		Workspace: workspace, Payload: payload,
	})
	if err != nil {
		return fmt.Errorf("admit agent host effect %q: %w", committed.EffectID, err)
	}
	// Automation mutations can usually transfer into their run ledger without
	// waiting for the scheduler. Failure is safe: the admitted generic outbox is
	// still authoritative and the wake-up retries after run admission/restart.
	if committed.Origin.AgentKind == agents.AgentKindAutomation {
		if _, reconcileErr := a.automation().reconcilePersistedHostEffect(context.WithoutCancel(ctx), admitted); reconcileErr != nil {
			log.Printf("[automation-host-effect] immediate transfer deferred effect_id=%s run_id=%s operation_id=%s err=%v", committed.EffectID, committed.Origin.TaskID, committed.RuntimeOperation, reconcileErr)
		}
	}
	a.signalAutomationEffectReconciliation()
	return nil
}

func (s *AutomationAppService) reconcilePersistedHostEffects(ctx context.Context) {
	if s == nil || s.app == nil {
		return
	}
	store := s.storeAllWorkspaces()
	effects, err := store.ListHostEffects()
	if err != nil {
		log.Printf("[automation-host-effect] list durable obligations failed err=%v", err)
		return
	}
	for _, effect := range effects {
		queued, reconcileErr := s.reconcilePersistedHostEffect(ctx, effect)
		if reconcileErr != nil {
			log.Printf("[automation-host-effect] obligation remains pending effect_id=%s workspace=%q err=%v", effect.ID, effect.Workspace, reconcileErr)
			continue
		}
		if queued {
			log.Printf("[automation-host-effect] obligation transferred effect_id=%s workspace=%q", effect.ID, effect.Workspace)
		}
	}
}

// reconcilePersistedHostEffect transfers one application obligation either to
// an Automation run outbox or to the workspace trigger coordinator. The bool
// reports successful admission/acknowledgement; false with nil means an async
// trigger pass now owns completion.
func (s *AutomationAppService) reconcilePersistedHostEffect(ctx context.Context, effect automation.HostEffectObligation) (bool, error) {
	payload, err := decodeAdmittedToolMutation(effect)
	if err != nil {
		return false, err
	}
	if payload.Origin.AgentKind == agents.AgentKindAutomation {
		if s.hostEffectTransfer != nil {
			return s.hostEffectTransfer(ctx, effect, payload)
		}
		return s.transferAutomationHostEffect(ctx, effect, payload)
	}
	if s.hostEffectOperationActive(ctx, payload) {
		return false, fmt.Errorf("agent operation %s is still active", payload.RuntimeOperation)
	}
	paths := automationCompletionMutationPaths([]agents.ToolMutation{payload.Mutation})
	workspace := strings.TrimSpace(effect.Workspace)
	if workspace == "" || len(paths) == 0 {
		return true, s.storeAllWorkspaces().AcknowledgeHostEffect(ctx, effect)
	}
	snap, operation, err := s.acquireTargetRuntime(ctx, automation.ExecutionTarget{
		Kind: automation.TargetKindWorkspace, Workspace: workspace,
	})
	if err != nil {
		return false, err
	}
	targets := s.chapterContentMutationPaths(snap, paths)
	if len(targets) == 0 {
		operation.Release()
		return true, s.storeAllWorkspaces().AcknowledgeHostEffect(ctx, effect)
	}
	if s.app.automationTriggers == nil {
		operation.Release()
		return false, fmt.Errorf("automation mutation-effect coordinator is unavailable")
	}
	enqueued := s.app.automationTriggers.EnqueueWithCompletion(
		s,
		snap,
		"agent_host_effect:"+effect.ID,
		targets,
		func(processErr error) {
			if processErr != nil {
				log.Printf("[automation-host-effect] trigger pass failed effect_id=%s workspace=%q err=%v", effect.ID, effect.Workspace, processErr)
				return
			}
			if ackErr := s.storeAllWorkspaces().AcknowledgeHostEffect(context.Background(), effect); ackErr != nil {
				log.Printf("[automation-host-effect] persist trigger receipt failed effect_id=%s workspace=%q err=%v", effect.ID, effect.Workspace, ackErr)
			}
		},
	)
	operation.Release()
	if !enqueued {
		return false, fmt.Errorf("workspace trigger pass could not be admitted")
	}
	return false, nil
}

// drainAutomationRunHostEffects transfers every global obligation owned by a
// run, regardless of whether it names the current or a historical operation.
// The final scan is the admission fence: a transfer error or any remaining
// obligation rejects a successor before its write-ahead command intent exists.
func (s *AutomationAppService) drainAutomationRunHostEffects(ctx context.Context, runID string) error {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return fmt.Errorf("automation host-effect drain requires a run identity")
	}
	store := s.storeAllWorkspaces()
	effects, err := store.ListHostEffects()
	if err != nil {
		return fmt.Errorf("list Automation HostEffect obligations for run %s: %w", runID, err)
	}
	for _, effect := range effects {
		owned, classifyErr := automationHostEffectOwnedByRun(effect, runID)
		if classifyErr != nil {
			return classifyErr
		}
		if !owned {
			continue
		}
		transferred, transferErr := s.reconcilePersistedHostEffect(ctx, effect)
		if transferErr != nil {
			return fmt.Errorf("transfer Automation HostEffect %s for run %s: %w", effect.ID, runID, transferErr)
		}
		if !transferred {
			return fmt.Errorf("Automation HostEffect %s for run %s remains pending", effect.ID, runID)
		}
	}
	remaining, err := store.ListHostEffects()
	if err != nil {
		return fmt.Errorf("verify Automation HostEffect drain for run %s: %w", runID, err)
	}
	for _, effect := range remaining {
		owned, classifyErr := automationHostEffectOwnedByRun(effect, runID)
		if classifyErr != nil {
			return classifyErr
		}
		if owned {
			return fmt.Errorf("Automation HostEffect %s for run %s remains after transfer", effect.ID, runID)
		}
	}
	return nil
}

func automationHostEffectOwnedByRun(effect automation.HostEffectObligation, runID string) (bool, error) {
	if effect.Kind != string(runstate.HostEffectToolMutationCommitted) {
		return false, nil
	}
	var owner struct {
		Origin agents.ToolMutationOrigin `json:"origin"`
	}
	if err := json.Unmarshal(effect.Payload, &owner); err != nil {
		return false, fmt.Errorf("classify admitted host effect %q: %w", effect.ID, err)
	}
	if owner.Origin.AgentKind != agents.AgentKindAutomation {
		return false, nil
	}
	return strings.TrimSpace(owner.Origin.TaskID) == strings.TrimSpace(runID), nil
}

func (s *AutomationAppService) hostEffectOperationActive(ctx context.Context, payload admittedToolMutationPayload) bool {
	if s == nil || s.app == nil || s.app.chatService == nil {
		return true
	}
	status, err := s.app.chatService.RuntimeStatusProjection(ctx, agents.RunOptions{
		AgentKind: payload.Origin.AgentKind, TaskID: payload.Origin.TaskID,
		AutomationTaskID: payload.Origin.AutomationTaskID, SessionID: payload.Origin.SessionID,
		ReviewThreadID: payload.Origin.ReviewThreadID, StoryID: payload.Origin.StoryID,
		BranchID: payload.Origin.BranchID, TurnID: payload.Origin.TurnID,
		MaintenanceTask: payload.Origin.MaintenanceTask, Workspace: payload.Origin.Workspace,
		Mode: payload.Origin.Mode,
	})
	if err != nil {
		log.Printf("[automation-host-effect] runtime projection unavailable effect_operation=%s err=%v", payload.RuntimeOperation, err)
		return true
	}
	return status.ActiveOperation == payload.RuntimeOperation
}

func (s *AutomationAppService) transferAutomationHostEffect(
	ctx context.Context,
	effect automation.HostEffectObligation,
	payload admittedToolMutationPayload,
) (bool, error) {
	runID := strings.TrimSpace(payload.Origin.TaskID)
	operationID := strings.TrimSpace(string(payload.RuntimeOperation))
	paths := automationCompletionMutationPaths([]agents.ToolMutation{payload.Mutation})
	store := automation.NewStore(s.app.cfg.DataDir(), effect.Workspace)
	_, _, err := store.MergeRunMutationEffect(ctx, runID, operationID, effect.ID, paths)
	if err != nil {
		if errors.Is(err, automation.ErrRunNotFound) {
			return false, fmt.Errorf("automation run has not materialized yet: %w", err)
		}
		return false, err
	}
	if err := s.storeAllWorkspaces().AcknowledgeHostEffect(ctx, effect); err != nil {
		return false, err
	}
	return true, nil
}

func decodeAdmittedToolMutation(effect automation.HostEffectObligation) (admittedToolMutationPayload, error) {
	if effect.Kind != string(runstate.HostEffectToolMutationCommitted) {
		return admittedToolMutationPayload{}, fmt.Errorf("unsupported admitted host effect kind %q", effect.Kind)
	}
	var payload admittedToolMutationPayload
	if err := json.Unmarshal(effect.Payload, &payload); err != nil {
		return admittedToolMutationPayload{}, fmt.Errorf("decode admitted host effect %q: %w", effect.ID, err)
	}
	if payload.Version != admittedToolMutationVersion || strings.TrimSpace(payload.ToolCallID) == "" ||
		strings.TrimSpace(payload.Mutation.ToolName) == "" || payload.Mutation.ToolCallID != payload.ToolCallID ||
		strings.TrimSpace(string(payload.RuntimeOperation)) == "" {
		return admittedToolMutationPayload{}, fmt.Errorf("admitted host effect %q has invalid identity", effect.ID)
	}
	return payload, nil
}
