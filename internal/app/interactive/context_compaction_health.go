package interactiveapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"denova/config"
	agents "denova/internal/agents"
	agentcompaction "denova/internal/agents/context/compaction"
	agentstructural "denova/internal/agents/context/structural"
	agentrun "denova/internal/agents/run"
	"denova/internal/interactive"
)

type preparedInteractiveContextCompactionHealth struct {
	ExpectedRevision     uint64
	BranchID             string
	StructureFingerprint string
	Outcome              agentcompaction.HealthOutcome
	FailureCode          string
}

func (c *Conversation) contextCompactionStructureFingerprint(
	storyCtx interactive.StoryContext,
	input agentcompaction.Input,
) (string, error) {
	model := config.ResolveAgentModel(c.cfg, config.AgentKindInteractiveStory)
	structureHash, err := agentstructural.ValueHash(struct {
		Title             string
		Origin            string
		StoryTellerID     string
		StoryDirectorID   string
		ReplyTargetChars  int
		ChoiceCount       int
		Opening           interactive.StoryOpeningConfig
		ImageSettings     interactive.StoryImageSettings
		StateSchemaPolicy *interactive.StoryStateSchemaPolicy
		ActorStateSchema  *interactive.ActorStateSchemaSnapshot
	}{
		Title: storyCtx.Meta.Title, Origin: storyCtx.Meta.Origin,
		StoryTellerID: storyCtx.Meta.StoryTellerID, StoryDirectorID: storyCtx.Meta.StoryDirectorID,
		ReplyTargetChars: storyCtx.Meta.ReplyTargetChars, ChoiceCount: storyCtx.Meta.ChoiceCount,
		Opening: storyCtx.Meta.Opening, ImageSettings: storyCtx.Meta.ImageSettings,
		StateSchemaPolicy: storyCtx.Meta.StateSchemaPolicy, ActorStateSchema: storyCtx.Meta.ActorStateSchema,
	})
	if err != nil {
		return "", err
	}
	anchors := []string{
		"agent=" + config.AgentKindInteractiveStory,
		fmt.Sprintf("model=%s|%s|%s|%s|%s|%v|%d|%s", model.ProfileID, model.Provider, model.Protocol, model.BaseURL, model.Model,
			model.Temperature, model.ContextWindowTokens, model.ThinkingLevel),
		"story_structure=" + structureHash,
		fmt.Sprintf("candidate=%s|%d", strings.TrimSpace(input.CandidateFingerprint), input.CandidateGeneration),
	}
	if active := storyCtx.Snapshot.ContextCompaction; active != nil {
		anchors = append(anchors, fmt.Sprintf("compaction=%s|%d|%d", active.ID, active.Epoch, active.SourceTurnCount))
	}
	if removal := storyCtx.Snapshot.ContextCompactionRemoval; removal != nil {
		anchors = append(anchors, fmt.Sprintf("removal=%s|%s|%d", removal.ID, removal.CompactionID, removal.SourceTurnCount))
	}
	if cleanup := storyCtx.Snapshot.ToolResultCleanup; cleanup != nil {
		anchors = append(anchors, fmt.Sprintf("cleanup=%s|%d|%d", cleanup.ID, cleanup.SourceStart, cleanup.SourceEnd))
	}
	stable := c.stableLeadingMessageSnapshot()
	var leading []*agents.Message
	if strings.TrimSpace(stable) != "" {
		leading = []*agents.Message{agents.UserMessage(stable)}
	}
	return agentcompaction.StructureFingerprint(leading, input.Tools, anchors...), nil
}

func (c *Conversation) stageInteractiveCompactionHealth(
	expectedRevision uint64,
	branchID, structureFingerprint string,
	outcome agentcompaction.HealthOutcome,
	result *agentcompaction.Result,
) {
	if c == nil || c.store == nil || result == nil || strings.TrimSpace(result.Phase) != "model_step" {
		return
	}
	failureCode := ""
	if outcome == agentcompaction.HealthFailure {
		failureCode = interactiveCompactionFailureCode(*result)
		_, previous, ok, _ := c.store.ContextCompactionHealthState(c.storyID, branchID, config.AgentKindInteractiveStory)
		state := agentcompaction.FailureState{}
		if ok {
			state.StructureFingerprint = previous.StructureFingerprint
			state.ConsecutiveFailures = previous.ConsecutiveFailures
		}
		next := state.NextFailure(structureFingerprint)
		result.ConsecutiveFailures = next.ConsecutiveFailures
		maximum := config.DefaultContextCompactionMaxConsecutiveFailures
		result.FailureFuseOpen = next.ConsecutiveFailures >= maximum
	} else {
		result.ConsecutiveFailures = 0
		result.FailureFuseOpen = false
	}
	c.mu.Lock()
	c.pendingCompactionHealth = &preparedInteractiveContextCompactionHealth{
		ExpectedRevision: expectedRevision, BranchID: strings.TrimSpace(branchID),
		StructureFingerprint: strings.TrimSpace(structureFingerprint), Outcome: outcome, FailureCode: failureCode,
	}
	c.mu.Unlock()
}

func interactiveCompactionFailureCode(result agentcompaction.Result) string {
	reason := strings.TrimSpace(result.FallbackReason)
	if reason == "" {
		reason = strings.TrimSpace(result.SkippedReason)
	}
	if reason == "" {
		reason = "compaction_failed"
	}
	if len(reason) > 128 {
		return reason[:128]
	}
	return reason
}

func (c *Conversation) CommitPostSettlementContextCompactionHealth(
	ctx context.Context,
	settledOperationID agentrun.OperationID,
	publication agentcompaction.Publication,
) error {
	if c == nil || c.store == nil {
		return nil
	}
	c.mu.Lock()
	prepared := c.pendingCompactionHealth
	c.pendingCompactionHealth = nil
	c.mu.Unlock()
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
	revision, _, _, err := c.store.ContextCompactionHealthState(c.storyID, prepared.BranchID, config.AgentKindInteractiveStory)
	if err != nil {
		return err
	}
	id := agentstructural.RecordID("cch", agentstructural.CommandID(
		"game-context-compaction-health", string(settledOperationID), c.storyID, prepared.BranchID,
		fmt.Sprint(prepared.ExpectedRevision), prepared.StructureFingerprint, string(outcome), failureCode,
	))
	_, err = c.store.AppendContextCompactionHealth(c.storyID, prepared.BranchID, interactive.ContextCompactionHealthEvent{
		ID: id, AgentKind: config.AgentKindInteractiveStory, StructureFingerprint: prepared.StructureFingerprint,
		Outcome: string(outcome), FailureCode: failureCode, ExpectedContextRevision: revision,
	})
	if errors.Is(err, interactive.ErrStoryContextRevisionConflict) {
		return nil
	}
	return err
}
