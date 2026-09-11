package interactiveapp

import (
	"errors"

	agentcontext "denova/internal/agents/context"
	agentrun "denova/internal/agents/run"
	"denova/internal/interactive"
	agent "github.com/alfredxw/denova/agent"
)

// BindAgentCompaction mirrors Agent's single checkpoint for context inspection
// and the Game UI. Story identity is enforced by the branch-owned Session;
// message coverage belongs to Agent, with no second story-turn cursor.
func (c *Conversation) BindAgentCompaction(state *agent.CompactionState) error {
	if c == nil {
		return errors.New("interactive Conversation is unavailable")
	}
	var checkpoint *agent.CompactionState
	if state != nil {
		clone := *state
		checkpoint = &clone
	}
	c.mu.Lock()
	c.agentCompaction = checkpoint
	c.mu.Unlock()
	return nil
}

// ProjectAgentCompaction preserves the Game API's display shape. Source turns
// are deliberately not inferred from the message boundary: checkpoints can
// advance several times before one narrative Turn is committed.
func ProjectAgentCompaction(state *agentrun.AgentCompactionState, storyID, branchID string) (*interactive.ContextCompactionProjection, error) {
	if state == nil {
		return nil, nil
	}
	return interactiveAgentCompactionEvent(state.ID, state.Revision, state.Summary, state.TokensAfter, state.SourceMessageCount, branchID), nil
}

func interactiveAgentCompactionEvent(id string, revision uint64, summary string, tokenEstimate, sourceMessages int, branchID string) *interactive.ContextCompactionProjection {
	return &interactive.ContextCompactionProjection{
		ID: id, BranchID: branchID,
		CompactionCheckpoint: agentcontext.CompactionCheckpoint{
			Revision: revision, Summary: summary, SourceMessageCount: sourceMessages,
			TokensAfter: tokenEstimate,
		},
	}
}

func (c *Conversation) boundAgentCompaction() *agent.CompactionState {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.agentCompaction == nil {
		return nil
	}
	clone := *c.agentCompaction
	return &clone
}

func (c *Conversation) AgentCompactionProjection(snapshot interactive.Snapshot) *interactive.ContextCompactionProjection {
	state := c.boundAgentCompaction()
	if state == nil {
		return nil
	}
	return interactiveAgentCompactionEvent(state.ID, state.Revision, state.Summary, state.TokensAfter, state.SourceMessageCount, snapshot.BranchID)
}
