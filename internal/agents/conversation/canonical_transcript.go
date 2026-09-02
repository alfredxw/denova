package conversation

import (
	"context"
	"fmt"

	agent "github.com/alfredxw/denova/agent"
)

// CanonicalMessages returns the complete model-visible lane. The Product
// Session journal is the sole durable source; Agent loads this projection into
// memory before admission and never writes a second transcript.
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
