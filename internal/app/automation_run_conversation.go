package app

import (
	"context"
	"strings"

	agents "denova/internal/agents"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/session"
)

type automationOutputConversation interface {
	agents.Conversation
	Output() string
}

type automationRunConversation struct {
	base   *agents.SessionConversation
	output string
}

func (c *automationRunConversation) ModelContextBudget() agentcontext.Budget {
	return c.base.ModelContextBudget()
}

func (c *automationRunConversation) ResolveExplicitSkills(ctx context.Context, message string) ([]agents.ExplicitSkillInvocation, error) {
	return c.base.ResolveExplicitSkills(ctx, message)
}

func (c *automationRunConversation) AssembleModelContext(ctx context.Context, originalMessage string, input agents.ModelContextInput) (agents.ModelContextResult, error) {
	return c.base.AssembleModelContext(ctx, originalMessage, input)
}

func (c *automationRunConversation) CommitModelInput(ctx context.Context, originalMessage string, assembled agents.ModelContextResult) error {
	return c.base.CommitModelInput(ctx, originalMessage, assembled)
}

func (c *automationRunConversation) AppendAssistant(content string) error {
	c.output = content
	return c.base.AppendAssistant(content)
}

func (c *automationRunConversation) AppendAssistantWithThinking(content, _ string) error {
	c.output = content
	return c.base.AppendAssistant(content)
}

func (c *automationRunConversation) AppendAssistantWithMetadata(content, thinking string, metadata session.MessageMetadata) error {
	c.output = content
	return c.base.AppendAssistantWithMetadata(content, thinking, metadata)
}

func (c *automationRunConversation) AppendDisplayEvent(event session.DisplayEvent) error {
	return c.base.AppendDisplayEvent(event)
}

func (c *automationRunConversation) UpdateDisplayToolStatus(id, name, status string) error {
	return c.base.UpdateDisplayToolStatus(id, name, status)
}

func (c *automationRunConversation) UpdateDisplayToolArgs(id, name, delta string) error {
	return c.base.AppendDisplayToolArgs(id, name, delta)
}

func (c *automationRunConversation) AppendDisplayEventContent(id, role, delta string) error {
	return c.base.AppendDisplayEventContent(id, role, delta)
}

func (c *automationRunConversation) FlushDisplayEventContent(id, role string) error {
	return c.base.FlushDisplayEventContent(id, role)
}

func (c *automationRunConversation) UpdateDisplayToolResult(id, name, status, result string) error {
	return c.base.UpdateDisplayToolResult(id, name, status, result)
}

func (c *automationRunConversation) MarkInterrupted(userMessage, assistantContent, reason string) error {
	return c.base.MarkInterrupted(userMessage, assistantContent, reason)
}

func (c *automationRunConversation) PendingInterruption() *session.Interruption {
	return c.base.PendingInterruption()
}

func (c *automationRunConversation) ResolveInterruption(id string) error {
	return c.base.ResolveInterruption(id)
}

func (c *automationRunConversation) Output() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.output)
}
