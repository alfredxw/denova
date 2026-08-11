package interactiveapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"denova/config"
	agentcontext "denova/internal/agents/context"
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
	SourceHead      string `json:"source_head,omitempty"`
}

// AgentCompactionContext freezes the product cursor needed to project an
// Agent-owned summary over interactive story history on later cycles.
func (c *Conversation) AgentCompactionContext(
	ctx context.Context,
	_ agent.CompactionCompactRequest,
) (*agent.HostData, error) {
	if c == nil || c.store == nil {
		return nil, errors.New("interactive Compaction context is unavailable")
	}
	storyContext, err := c.store.StoryContext(c.storyID, c.branchID)
	if err != nil {
		return nil, err
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	branchID := storyContext.Snapshot.BranchID
	sourceHead := ""
	if branch, ok := storyContext.Meta.Branches[branchID]; ok {
		sourceHead = branch.Head
	}
	metadata := interactiveCompactionContext{
		StoryID: c.storyID, BranchID: branchID,
		SourceTurnCount: SnapshotTurnCount(storyContext.Snapshot),
		RetainedTurns:   config.DefaultContextCompactionRetainedTurns,
		SourceHead:      sourceHead,
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("encode interactive Compaction context: %w", err)
	}
	return &agent.HostData{Type: interactiveCompactionContextType, Version: 1, Data: encoded}, nil
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
) (*interactive.ContextCompactionEvent, error) {
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
) *interactive.ContextCompactionEvent {
	return &interactive.ContextCompactionEvent{
		ID: id, BranchID: metadata.BranchID,
		CompactionCheckpoint: agentcontext.CompactionCheckpoint{
			AgentKind: config.AgentKindInteractiveStory, Epoch: int(revision),
			Summary: summary, RetainedTurns: metadata.RetainedTurns,
			TokensAfter: tokenEstimate, Phase: "agent",
		},
		SourceTurnCount: metadata.SourceTurnCount,
	}
}

func (c *Conversation) boundAgentCompaction(snapshot interactive.Snapshot) *interactive.ContextCompactionEvent {
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
func (c *Conversation) AgentCompactionProjection(snapshot interactive.Snapshot) *interactive.ContextCompactionEvent {
	return c.boundAgentCompaction(snapshot)
}
