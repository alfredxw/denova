package interactive

import (
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/session"
)

func (c *DirectorConversation) AppendDisplayEvent(event session.DisplayEvent) error {
	if c == nil {
		return nil
	}
	return c.forwardDisplayEvent(decorateDirectorDisplayEvent(event))
}

func (c *DirectorConversation) AppendDisplayToolArgs(id, name, delta string) error {
	if c == nil || delta == "" {
		return nil
	}
	if appender, ok := c.display.(displayToolArgsAppender); ok {
		return appender.AppendDisplayToolArgs(id, name, delta)
	}
	return nil
}

func (c *DirectorConversation) AppendDisplayEventContent(id, role, delta string) error {
	if c == nil || delta == "" {
		return nil
	}
	if appender, ok := c.display.(displayEventContentAppender); ok {
		return appender.AppendDisplayEventContent(id, role, delta)
	}
	return nil
}

func (c *DirectorConversation) FlushDisplayEventContent(id, role string) error {
	if c == nil {
		return nil
	}
	if flusher, ok := c.display.(displayEventContentFlusher); ok {
		return flusher.FlushDisplayEventContent(id, role)
	}
	return nil
}

func (c *DirectorConversation) FinalizeDisplayAssistantRun(runID, finalSegmentID, terminalPhase string) error {
	if c == nil {
		return nil
	}
	if finalizer, ok := c.display.(displayAssistantRunFinalizer); ok {
		return finalizer.FinalizeDisplayAssistantRun(runID, finalSegmentID, terminalPhase)
	}
	return nil
}

func (c *DirectorConversation) UpdateDisplayToolStatus(id, name, status string) error {
	if c == nil {
		return nil
	}
	if updater, ok := c.display.(displayEventAppender); ok {
		return updater.UpdateDisplayToolStatus(id, name, status)
	}
	return nil
}

func (c *DirectorConversation) UpdateDisplayToolResult(id, name, status, result string, presentation *agent.ToolPresentation) error {
	if c == nil {
		return nil
	}
	if updater, ok := c.display.(displayToolResultUpdater); ok {
		return updater.UpdateDisplayToolResult(id, name, status, result, presentation)
	}
	if updater, ok := c.display.(displayEventAppender); ok {
		return updater.UpdateDisplayToolStatus(id, name, status)
	}
	return nil
}

func (c *DirectorConversation) forwardDisplayEvent(event session.DisplayEvent) error {
	if appender, ok := c.display.(displayEventAppender); ok {
		return appender.AppendDisplayEvent(event)
	}
	return nil
}

func decorateDirectorDisplayEvent(event session.DisplayEvent) session.DisplayEvent {
	if strings.TrimSpace(event.AgentKind) == "" {
		event.AgentKind = config.AgentKindInteractiveDirector
	}
	if strings.TrimSpace(event.AgentName) == "" {
		event.AgentName = "interactive_director"
	}
	if strings.TrimSpace(event.RootAgentName) == "" {
		event.RootAgentName = event.AgentName
	}
	if strings.TrimSpace(event.Content) == "" && strings.TrimSpace(event.Name) != "" {
		event.Content = strings.TrimSpace(event.Name)
	}
	return event
}
