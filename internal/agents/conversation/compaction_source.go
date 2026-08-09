package conversation

import (
	"reflect"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/session"
)

// compactionModelBase keeps the exact canonical/provider pair for the accepted
// user message so automatic compaction can preserve turn-scoped context.
type compactionModelBase struct {
	cursor    session.ContextCursor
	canonical []*agent.Message
	effective []*agent.Message
}

func (c *SessionConversation) rememberCompactionModelBase(canonical, effective []*agent.Message) {
	if c == nil {
		return
	}
	c.cycleMu.Lock()
	c.compactionModelBase = &compactionModelBase{
		cursor:    c.cycleCursor,
		canonical: agentcontext.CloneMessages(canonical),
		effective: agentcontext.CloneMessages(effective),
	}
	c.cycleMu.Unlock()
}

// providerVisibleCompactionSource replaces only the accepted canonical user
// message with the exact provider-visible form captured for the current turn.
func (c *SessionConversation) providerVisibleCompactionSource(source []*agent.Message, sourceStart int) ([]*agent.Message, bool) {
	if c == nil || len(source) == 0 {
		return nil, false
	}
	c.cycleMu.Lock()
	base := c.compactionModelBase
	if base != nil {
		base = &compactionModelBase{
			cursor:    base.cursor,
			canonical: agentcontext.CloneMessages(base.canonical),
			effective: agentcontext.CloneMessages(base.effective),
		}
	}
	c.cycleMu.Unlock()
	if base == nil || len(base.canonical) == 0 || len(base.effective) == 0 {
		return nil, false
	}
	canonicalUser := sanitizeCompactionSourceMessage(base.canonical[len(base.canonical)-1])
	providerUser := sanitizeCompactionSourceMessage(base.effective[len(base.effective)-1])
	if canonicalUser == nil || providerUser == nil || canonicalUser.Role != agent.User || providerUser.Role != agent.User {
		return nil, false
	}
	offset := base.cursor.MessageCount - 1 - sourceStart
	if offset < 0 || offset >= len(source) || !reflect.DeepEqual(source[offset], canonicalUser) || reflect.DeepEqual(canonicalUser, providerUser) {
		return nil, false
	}
	mapped := agentcontext.CloneMessages(source)
	mapped[offset] = providerUser
	return mapped, true
}
