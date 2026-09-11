package execution

import (
	"context"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
)

// taskSessions discovers only exact descendants of this product Session.
// Requests and answers stay in the owning journal; discovery never resumes a
// Run or copies child interactions into a second durable queue.
func (backend *publicBackend) taskSessions(ctx context.Context, root *agent.Session) ([]*agent.Session, error) {
	sessions := []*agent.Session{root}
	seen := make(map[string]bool)
	for index := 0; index < len(sessions); index++ {
		attributes, err := agent.ChildSessionAttributes(sessions[index].Key())
		if err != nil {
			return nil, err
		}
		keys, err := backend.agent.ListSessions(ctx, agent.SessionSelector{Attributes: attributes})
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			canonical, err := agentsession.CanonicalKey(key)
			if err != nil {
				return nil, err
			}
			if seen[canonical] {
				continue
			}
			seen[canonical] = true
			child, err := backend.agent.Session(ctx, key)
			if err != nil {
				return nil, err
			}
			sessions = append(sessions, child)
		}
	}
	return sessions, nil
}
