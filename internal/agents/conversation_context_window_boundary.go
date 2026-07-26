package agents

import (
	"fmt"
	"reflect"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/agents/session"
)

type contextWindowModelBase struct {
	cursor    session.ContextCursor
	canonical []*agent.Message
	effective []*agent.Message
}

func (c *SessionConversation) rememberContextWindowModelBase(canonical, effective []*agent.Message) {
	if c == nil {
		return
	}
	c.cycleMu.Lock()
	c.contextWindowBase = &contextWindowModelBase{
		cursor: c.cycleCursor, canonical: cloneContextMessages(canonical), effective: cloneContextMessages(effective),
	}
	c.cycleMu.Unlock()
}

// FreezeContextWindowBoundary maps the initial exact Agent RunState back to the
// canonical modelHistory projection captured before turn-scoped assembly. The
// controller extends this frozen boundary with later model/tool messages only
// when the prior effective projection is still an exact prefix.
func (c *SessionConversation) FreezeContextWindowBoundary(messages []*agent.Message) (*session.ContextBoundarySnapshot, error) {
	if c == nil || c.session == nil {
		return nil, fmt.Errorf("conversation is unavailable")
	}
	c.cycleMu.Lock()
	base := c.contextWindowBase
	if base != nil {
		base = &contextWindowModelBase{
			cursor: base.cursor, canonical: cloneContextMessages(base.canonical), effective: cloneContextMessages(base.effective),
		}
	}
	agentKind, cfg := c.agentKind, c.cfg
	c.cycleMu.Unlock()
	if base == nil {
		return nil, fmt.Errorf("context checkpoint requires a committed model input projection")
	}
	if len(messages) < len(base.effective) {
		return nil, fmt.Errorf("current Agent context is shorter than the committed model input projection")
	}
	baseIndex := len(messages) - len(base.effective)
	if !contextMessagesEqual(messages[baseIndex:], base.effective) {
		return nil, fmt.Errorf("current Agent context does not end with the committed model input projection")
	}
	for _, message := range messages[:baseIndex] {
		if message == nil || message.Role != agent.System {
			return nil, fmt.Errorf("current Agent context has a non-system message before the committed model input projection")
		}
	}
	suffixStart := baseIndex + len(base.effective)
	canonical := append(cloneContextMessages(base.canonical), cloneContextMessages(messages[suffixStart:])...)
	limit := config.ResolveAgentContext(cfg, agentKind).MaxProviderInputBytes
	return session.NewContextBoundarySnapshot(base.cursor, messages, canonical, limit)
}

func (c *SessionConversation) StoreContextWindowBoundary(
	boundaryID string,
	boundary *session.ContextBoundarySnapshot,
) (session.ContextBoundaryLocator, error) {
	if c == nil || c.session == nil {
		return session.ContextBoundaryLocator{}, fmt.Errorf("conversation is unavailable")
	}
	return c.session.StoreContextBoundary(boundaryID, boundary)
}

func contextMessagesEqual(left, right []*agent.Message) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !reflect.DeepEqual(left[index], right[index]) {
			return false
		}
	}
	return true
}
