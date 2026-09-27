package conversation

import (
	"context"
	"fmt"

	agent "github.com/alfredxw/denova/agent"
)

func (c *SessionConversation) CanonicalHistoryHead(ctx context.Context) (agent.CanonicalHistoryHead, error) {
	return c.session.CanonicalHistoryHead(ctx)
}

// CanonicalMessages returns the complete model-visible lane. The Product
// Session journal is the sole durable source. Agent reads this full projection
// when rebuilding its active window or explicitly removing compaction.
func (c *SessionConversation) CanonicalMessages(ctx context.Context) ([]*agent.Message, error) {
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
