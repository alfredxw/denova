package automationapp

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	"strings"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/session"
	novaskills "denova/internal/agents/skills"
)

type automationOutputConversation interface {
	agentchat.Conversation
	Output() string
	RuntimeConfig() config.Config
}

type automationRunConversation struct {
	base          *agentconversation.SessionConversation
	runtimeConfig config.Config
	output        string
}

// RuntimeConfig returns the request-local configuration already resolved from
// this durable conversation. Task constraints may narrow it further, but model,
// reasoning, and approval selections must never be re-read from Settings.
func (c *automationRunConversation) RuntimeConfig() config.Config {
	if c == nil {
		return config.Config{}
	}
	return c.runtimeConfig
}

func (c *automationRunConversation) ModelContextBudget() agentcontext.Budget {
	return c.base.ModelContextBudget()
}

func (c *automationRunConversation) ResolveExplicitSkills(ctx context.Context, message string) ([]novaskills.Invocation, error) {
	return c.base.ResolveExplicitSkills(ctx, message)
}

func (c *automationRunConversation) AssembleModelContext(ctx context.Context, originalMessage string, input agentcontext.ModelContextInput) (agentcontext.ModelContextResult, error) {
	return c.base.AssembleModelContext(ctx, originalMessage, input)
}

func (c *automationRunConversation) CommitModelInput(ctx context.Context, originalMessage string, assembled agentcontext.ModelContextResult) error {
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
