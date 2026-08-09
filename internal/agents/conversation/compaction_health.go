package conversation

import (
	"context"
	"denova/internal/agents/run"
	"errors"
	"fmt"
	"strings"

	"denova/config"
	agentcompaction "denova/internal/agents/context/compaction"
	"denova/internal/agents/session"
)

// PostSettlementContextCompactionHealthProvider publishes the model-invisible
// health transition staged by one automatic maintenance attempt. It is called
// for every non-replayed terminal outcome, including failed turns.
type PostSettlementContextCompactionHealthProvider interface {
	CommitPostSettlementContextCompactionHealth(context.Context, agentrun.OperationID, agentcompaction.Publication) error
}

type preparedSessionContextCompactionHealth struct {
	Expected             session.ContextCursor
	StructureFingerprint string
	Outcome              agentcompaction.HealthOutcome
	FailureCode          string
}

func (c *SessionConversation) compactionStructureFingerprint(input agentcompaction.Input) string {
	if c == nil {
		return ""
	}
	model := config.ResolveAgentModel(c.cfg, c.agentKind)
	policy := c.compactionPolicy()
	anchors := []string{
		"agent=" + strings.TrimSpace(c.agentKind),
		fmt.Sprintf("model=%s|%s|%s|%s|%s|%v|%d|%s", model.ProfileID, model.Provider, model.Protocol, model.BaseURL, model.Model,
			model.Temperature, model.ContextWindowTokens, model.ThinkingLevel),
		fmt.Sprintf("policy=%s|%g|%g|%d|%g|%g", policy.Strategy, policy.Threshold, policy.RecoveryBand,
			policy.RetainedTurns, policy.TargetMinRatio, policy.TargetMaxRatio),
		fmt.Sprintf("candidate=%s|%d", strings.TrimSpace(input.CandidateFingerprint), input.CandidateGeneration),
	}
	if c.session != nil {
		if active, ok := c.session.LatestContextCompaction(c.agentKind); ok {
			anchors = append(anchors, fmt.Sprintf("compaction=%s|%d|%d", active.ID, active.Epoch, active.SourceEndIndex))
		}
		if removal, ok := c.session.LatestContextCompactionRemoval(c.agentKind); ok {
			anchors = append(anchors, fmt.Sprintf("removal=%s|%s|%d", removal.ID, removal.CompactionID, removal.SourceEndIndex))
		}
		if cleanup, ok := c.session.LatestToolResultCleanup(c.agentKind); ok {
			anchors = append(anchors, fmt.Sprintf("cleanup=%s|%d|%d", cleanup.ID, cleanup.SourceStart, cleanup.SourceEnd))
		}
	}
	return agentcompaction.StructureFingerprint(c.leadingRuntimeMessages(), input.Tools, anchors...)
}

func (c *SessionConversation) stageSessionCompactionHealth(
	expected session.ContextCursor,
	structureFingerprint string,
	outcome agentcompaction.HealthOutcome,
	result *agentcompaction.Result,
) {
	if c == nil || c.session == nil || result == nil || strings.TrimSpace(result.Phase) != agentcompaction.PhaseModelStep {
		return
	}
	failureCode := ""
	if outcome == agentcompaction.HealthFailure {
		failureCode = boundedCompactionFailureCode(agentcompaction.FailureReason(*result))
		state := agentcompaction.FailureState{}
		if previous, ok := c.session.LatestContextCompactionHealth(c.agentKind); ok {
			state.StructureFingerprint = previous.StructureFingerprint
			state.ConsecutiveFailures = previous.ConsecutiveFailures
		}
		next := state.NextFailure(structureFingerprint)
		result.ConsecutiveFailures = next.ConsecutiveFailures
		result.FailureFuseOpen = next.ConsecutiveFailures >= c.compactionPolicy().MaxConsecutiveFailures
	} else {
		result.ConsecutiveFailures = 0
		result.FailureFuseOpen = false
	}
	c.cycleMu.Lock()
	c.pendingCompactionHealth = &preparedSessionContextCompactionHealth{
		Expected: expected, StructureFingerprint: strings.TrimSpace(structureFingerprint),
		Outcome: outcome, FailureCode: failureCode,
	}
	c.cycleMu.Unlock()
}

func boundedCompactionFailureCode(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "compaction_failed"
	}
	if len(reason) > 128 {
		return reason[:128]
	}
	return reason
}

func (c *SessionConversation) CommitPostSettlementContextCompactionHealth(
	ctx context.Context,
	settledOperationID agentrun.OperationID,
	publication agentcompaction.Publication,
) error {
	if c == nil || c.session == nil {
		return nil
	}
	c.cycleMu.Lock()
	prepared := c.pendingCompactionHealth
	c.pendingCompactionHealth = nil
	c.cycleMu.Unlock()
	if prepared == nil || strings.TrimSpace(prepared.StructureFingerprint) == "" {
		return nil
	}
	outcome := prepared.Outcome
	failureCode := prepared.FailureCode
	if outcome == agentcompaction.HealthSuccess && (!publication.Attempted || publication.Err != nil) {
		outcome = agentcompaction.HealthFailure
		failureCode = "checkpoint_not_published"
		if publication.Err != nil {
			failureCode = "checkpoint_publish_failed"
		}
	}
	expected := c.session.ContextCursor()
	id := postSettlementContextRecordID("cch", postSettlementContextCommandID(
		"context-compaction-health", string(settledOperationID), c.session.ID,
		fmt.Sprint(prepared.Expected.Revision), prepared.StructureFingerprint,
		string(outcome), failureCode,
	))
	_, err := c.session.CommitContextCompactionHealthAtContext(ctx, expected, session.ContextCompactionHealth{
		ID: id, AgentKind: c.agentKind, StructureFingerprint: prepared.StructureFingerprint,
		Outcome: string(outcome), FailureCode: failureCode,
	})
	if errors.Is(err, session.ErrContextRevisionConflict) {
		// The owning turn changed model context after the attempt. Its health
		// transition is obsolete and must not attach to the new revision.
		return nil
	}
	return err
}
