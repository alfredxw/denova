package interactiveapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	agentlifecycle "denova/internal/agents/lifecycle"
	agentrun "denova/internal/agents/run"
	"denova/internal/interactive"

	agent "github.com/alfredxw/denova/agent"
)

const interactiveCompactionContextType = "denova.interactive.compaction"

type interactiveCompactionContext struct {
	StoryID         string `json:"story_id"`
	BranchID        string `json:"branch_id"`
	SourceTurnCount int    `json:"source_turn_count"`
	RetainedTurns   int    `json:"retained_turns"`
}

// PrepareAgentCompaction supplies the exact canonical story-turn delta and the
// bounded product cursor for one Agent-owned checkpoint. The generic Agent
// transcript contains rendered story prompts, so summarizing it directly would
// repeatedly summarize the same history and lose Game-specific boundaries.
func (c *Conversation) PrepareAgentCompaction(
	ctx context.Context,
	_ agent.CompactionCompactRequest,
) (agentlifecycle.ProductCompactionProjection, error) {
	if c == nil || c.store == nil {
		return agentlifecycle.ProductCompactionProjection{}, errors.New("interactive Compaction context is unavailable")
	}
	storyContext, err := c.storyContextForCycle()
	if err != nil {
		return agentlifecycle.ProductCompactionProjection{}, err
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return agentlifecycle.ProductCompactionProjection{}, err
		}
	}
	history, active, err := c.modelHistoryForCycle(storyContext)
	if err != nil {
		return agentlifecycle.ProductCompactionProjection{}, err
	}
	projection, err := BuildModelContextProjection(
		history, active, storyContext.Snapshot, c.ToolResultContextPolicy(), c.AgentCycleIdentitySnapshot(),
	)
	if err != nil {
		return agentlifecycle.ProductCompactionProjection{}, err
	}
	if len(projection.SourceMessages) == 0 {
		return agentlifecycle.ProductCompactionProjection{}, errors.New("interactive Compaction has no complete canonical turns to summarize")
	}
	metadata := interactiveCompactionContext{
		StoryID: c.storyID, BranchID: storyContext.Snapshot.BranchID,
		SourceTurnCount: projection.SourceTurnCount,
		RetainedTurns:   config.DefaultContextCompactionRetainedTurns,
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return agentlifecycle.ProductCompactionProjection{}, fmt.Errorf("encode interactive Compaction context: %w", err)
	}
	return agentlifecycle.ProductCompactionProjection{
		SourceMessages: projection.SourceMessages,
		ContextData:    &agent.HostData{Type: interactiveCompactionContextType, Version: 1, Data: encoded},
	}, nil
}

// BindAgentCompaction applies the Agent-owned checkpoint to process-local
// story context assembly. It never writes a second checkpoint to Story Store.
func (c *Conversation) BindAgentCompaction(state *agent.CompactionState) error {
	if c == nil {
		return errors.New("interactive Conversation is unavailable")
	}
	if state == nil {
		c.mu.Lock()
		c.agentCompaction = nil
		c.modelHistory = nil
		c.modelHistoryKey = ""
		c.mu.Unlock()
		return nil
	}
	if state.ContextData == nil || state.ContextData.Type != interactiveCompactionContextType || state.ContextData.Version != 1 {
		return errors.New("interactive Agent Compaction context is unavailable")
	}
	var metadata interactiveCompactionContext
	if err := json.Unmarshal(state.ContextData.Data, &metadata); err != nil {
		return fmt.Errorf("decode interactive Agent Compaction context: %w", err)
	}
	metadata.StoryID = strings.TrimSpace(metadata.StoryID)
	metadata.BranchID = strings.TrimSpace(metadata.BranchID)
	if metadata.StoryID != c.storyID || c.branchID != "" && metadata.BranchID != c.branchID || metadata.SourceTurnCount < 0 {
		return errors.New("interactive Agent Compaction context does not match the conversation")
	}
	if metadata.RetainedTurns <= 0 {
		metadata.RetainedTurns = config.DefaultContextCompactionRetainedTurns
	}
	checkpoint := interactiveAgentCompactionEvent(
		state.ID, state.Revision, state.Summary, state.TokenEstimate, metadata,
	)
	c.mu.Lock()
	c.agentCompaction = checkpoint
	c.modelHistory = nil
	c.modelHistoryKey = ""
	c.mu.Unlock()
	return nil
}

// ProjectAgentCompaction converts the bounded runtime state to the existing
// game API shape without persisting it in Story Store.
func ProjectAgentCompaction(
	state *agentrun.AgentCompactionState,
	storyID, branchID string,
) (*interactive.ContextCompactionProjection, error) {
	if state == nil {
		return nil, nil
	}
	if state.ContextData == nil || state.ContextData.Type != interactiveCompactionContextType || state.ContextData.Version != 1 {
		return nil, errors.New("interactive Agent Compaction projection is unavailable")
	}
	var metadata interactiveCompactionContext
	if err := json.Unmarshal(state.ContextData.Data, &metadata); err != nil {
		return nil, fmt.Errorf("decode interactive Agent Compaction projection: %w", err)
	}
	if metadata.StoryID != strings.TrimSpace(storyID) || metadata.BranchID != strings.TrimSpace(branchID) {
		return nil, errors.New("interactive Agent Compaction projection does not match the story")
	}
	if metadata.RetainedTurns <= 0 {
		metadata.RetainedTurns = config.DefaultContextCompactionRetainedTurns
	}
	return interactiveAgentCompactionEvent(
		state.ID, state.Revision, state.Summary, state.TokenEstimate, metadata,
	), nil
}

// BindRuntimeCompaction adapts the app-facing runtime projection back to the
// process-local context binder used by context analysis outside an Agent run.
func (c *Conversation) BindRuntimeCompaction(state *agentrun.AgentCompactionState) error {
	if state == nil {
		return c.BindAgentCompaction(nil)
	}
	projected := &agent.CompactionState{
		ID: state.ID, Revision: state.Revision, Summary: state.Summary,
		TokenEstimate: state.TokenEstimate, ReplacementFrom: state.ReplacementFrom,
		ReplacementTo: state.ReplacementTo,
	}
	if state.ContextData != nil {
		projected.ContextData = &agent.HostData{
			Type: state.ContextData.Type, Version: state.ContextData.Version,
			Data: append([]byte(nil), state.ContextData.Data...),
		}
	}
	return c.BindAgentCompaction(projected)
}

func interactiveAgentCompactionEvent(
	id string,
	revision uint64,
	summary string,
	tokenEstimate int,
	metadata interactiveCompactionContext,
) *interactive.ContextCompactionProjection {
	return &interactive.ContextCompactionProjection{
		ID: id, BranchID: metadata.BranchID,
		CompactionCheckpoint: agentcontext.CompactionCheckpoint{
			AgentKind: config.AgentKindInteractiveStory, Epoch: int(revision),
			Summary: summary, RetainedTurns: metadata.RetainedTurns,
			TokensAfter: tokenEstimate, Phase: "agent",
		},
		SourceTurnCount: metadata.SourceTurnCount,
	}
}

func (c *Conversation) boundAgentCompaction(snapshot interactive.Snapshot) *interactive.ContextCompactionProjection {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.agentCompaction == nil || c.agentCompaction.BranchID != snapshot.BranchID ||
		c.agentCompaction.SourceTurnCount > SnapshotTurnCount(snapshot) {
		return nil
	}
	copy := *c.agentCompaction
	return &copy
}

// AgentCompactionProjection returns the current process-local projection for
// an authoritative Story snapshot. It never reads a Story Store checkpoint.
func (c *Conversation) AgentCompactionProjection(snapshot interactive.Snapshot) *interactive.ContextCompactionProjection {
	return c.boundAgentCompaction(snapshot)
}
