package conversation

import (
	"context"
	"fmt"

	agentschema "github.com/alfredxw/denova/agent/schema"
	agentcanonical "github.com/alfredxw/denova/agent/session/canonical"
)

func (c *SessionConversation) CanonicalHistoryHead(ctx context.Context) (agentcanonical.CanonicalHistoryHead, error) {
	return c.session.CanonicalHistoryHead(ctx)
}

// CanonicalMessages returns the complete model-visible lane. The Product
// Session journal is the sole durable source. Agent reads this full projection
// when rebuilding its active window or explicitly removing compaction.
func (c *SessionConversation) CanonicalMessages(ctx context.Context) ([]*agentschema.Message, error) {
	if c == nil || c.session == nil {
		return nil, fmt.Errorf("session canonical transcript is unavailable")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	return c.session.ReadCanonicalMessages(ctx)
}

func (c *SessionConversation) VisitCanonicalMessages(ctx context.Context, visit func(*agentschema.Message) error) error {
	return c.session.VisitCanonicalMessages(ctx, visit)
}
