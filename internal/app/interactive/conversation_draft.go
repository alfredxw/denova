package interactiveapp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"denova/internal/interactive"

	agent "github.com/alfredxw/denova/agent"
)

func (c *Conversation) turnDraftIdentity() interactive.DomainCommitIdentity {
	identity := c.agentCycleIdentity
	return interactive.DomainCommitIdentity{CommandID: string(identity.CommandID), OperationID: string(identity.OperationID), Cycle: identity.Cycle}
}

// loadTurnDraft is serialized with all draft changes by turnCheckMu. It
// rebuilds protocol projections once per cycle from the canonical Story log.
func (c *Conversation) loadTurnDraft() error {
	c.mu.Lock()
	if c.turnDraftLoaded {
		c.mu.Unlock()
		return nil
	}
	identity := c.turnDraftIdentity()
	branchID := c.branchID
	c.mu.Unlock()
	if branchID == "" {
		storyContext, err := c.store.StoryContext(c.storyID, "")
		if err != nil {
			return err
		}
		branchID = storyContext.Snapshot.BranchID
	}
	draft, found, err := c.store.LoadTurnDraft(c.storyID, branchID, identity)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.turnDraftLoaded = true
	c.branchID = branchID
	if found {
		c.turnDraft = draft
		c.ruleResolution = draft.RuleResolution
		if draft.Submission != nil {
			c.turnProtocol.update(draft.Submission.Prepared())
		}
	} else {
		c.turnDraft = interactive.TurnDraft{Identity: identity}
	}
	return nil
}

func (c *Conversation) LoadNarrativeCandidate(ctx context.Context) (string, error) {
	c.turnCheckMu.Lock()
	defer c.turnCheckMu.Unlock()
	if err := c.loadTurnDraft(); err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.turnDraft.Narrative, nil
}

func (c *Conversation) AcceptNarrativeCandidate(ctx context.Context, narrative string) error {
	if strings.TrimSpace(narrative) == "" {
		return nil
	}
	c.turnCheckMu.Lock()
	defer c.turnCheckMu.Unlock()
	if err := c.loadTurnDraft(); err != nil {
		return err
	}
	c.mu.Lock()
	draft := c.turnDraft
	c.mu.Unlock()
	if draft.Narrative != "" {
		return nil
	}
	draft.Narrative, draft.NarrativeSource = narrative, "complete_model_response"
	if err := c.commitTurnDraft(ctx, draft, nil); err != nil {
		return err
	}
	c.mu.Lock()
	c.turnDraft = draft
	c.mu.Unlock()
	return nil
}

func (c *Conversation) PendingOutput(ctx context.Context, identity agent.CommitIdentity) (*agent.Message, error) {
	narrative, err := c.LoadNarrativeCandidate(ctx)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.turnDraft.Identity != (interactive.DomainCommitIdentity{CommandID: identity.CommandID, OperationID: identity.RunID, Cycle: identity.Cycle}) {
		return nil, errors.New("Game prepared output belongs to a different Agent cycle")
	}
	if !c.turnProtocol.narrativeReady() || narrative == "" {
		return nil, nil
	}
	return agent.AssistantMessage(narrative, nil), nil
}

func (c *Conversation) commitTurnDraft(ctx context.Context, draft interactive.TurnDraft, result *agent.ToolResult) error {
	if draft.Identity.CommandID == "" {
		return errors.New("Game draft acceptance requires a bound Agent cycle")
	}
	return agent.CommitProductAcceptance(ctx, result, func(checkpoint agent.CanonicalCheckpoint) error {
		return c.store.SaveTurnDraft(c.storyID, c.branchID, draft, checkpoint)
	})
}

func turnSubmissionToolResult(receipt interactive.TurnSubmissionReceipt) (*agent.ToolResult, error) {
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return nil, err
	}
	result := agent.TextToolResult(string(data))
	result.Details = data
	return &result, nil
}

func ruleResolutionToolResult(resolution interactive.RuleResolution) (*agent.ToolResult, error) {
	model, err := json.MarshalIndent(resolution.ModelToolOutput(), "", "  ")
	if err != nil {
		return nil, err
	}
	display, err := json.MarshalIndent(resolution.ToolOutput(), "", "  ")
	if err != nil {
		return nil, err
	}
	return &agent.ToolResult{ModelContent: string(model), DisplayContent: string(display), Details: display, Status: agent.ToolResultSuccess}, nil
}
