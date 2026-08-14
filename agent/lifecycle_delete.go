package agent

import (
	"context"
	"errors"
	"sort"

	agentsession "github.com/alfredxw/denova/agent/session"
)

// DeleteSessions closes matching in-process handles, then removes their
// transcript logs. The selector must be constrained so product lifecycle code
// cannot accidentally erase every conversation.
func (agent *Agent) DeleteSessions(ctx context.Context, selector SessionSelector) error {
	if agent == nil || agent.store == nil {
		return ErrAgentClosed
	}
	if err := selector.Validate(); err != nil {
		return err
	}
	if selector.All {
		return errors.New("delete Agent Sessions requires a constrained selector")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	keys, err := agent.store.List(ctx, SessionSelector{All: true})
	if err != nil {
		return err
	}
	byCanonical := make(map[string]SessionKey, len(keys))
	for _, key := range keys {
		if !sessionSelectorMatchesTree(selector, key) {
			continue
		}
		canonical, canonicalErr := agentsession.CanonicalKey(key)
		if canonicalErr != nil {
			return canonicalErr
		}
		byCanonical[canonical] = key
	}
	agent.mu.RLock()
	for canonical, session := range agent.sessions {
		if sessionSelectorMatchesTree(selector, session.key) {
			byCanonical[canonical] = session.key
		}
	}
	agent.mu.RUnlock()
	ordered := make([]string, 0, len(byCanonical))
	for canonical := range byCanonical {
		ordered = append(ordered, canonical)
	}
	sort.Slice(ordered, func(left, right int) bool {
		return sessionTreeDepth(byCanonical[ordered[left]]) > sessionTreeDepth(byCanonical[ordered[right]])
	})
	var result error
	for _, canonical := range ordered {
		key := byCanonical[canonical]
		agent.mu.RLock()
		open := agent.sessions[canonical]
		agent.mu.RUnlock()
		if open != nil {
			result = errors.Join(result, open.Close(ctx))
		}
		if deleteErr := agent.store.Delete(ctx, key); deleteErr != nil {
			result = errors.Join(result, deleteErr)
		}
	}
	return result
}

func sessionSelectorMatchesTree(selector SessionSelector, key SessionKey) bool {
	for depth := 0; depth <= agentsession.MaxAttributes; depth++ {
		if selector.Matches(key) {
			return true
		}
		parent, ok := childSessionParent(key)
		if !ok {
			return false
		}
		key = parent
	}
	return false
}

func childSessionParent(key SessionKey) (SessionKey, bool) {
	parent, err := ParentSessionKey(key)
	return parent, err == nil
}

func sessionTreeDepth(key SessionKey) int {
	for depth := 0; depth <= agentsession.MaxAttributes; depth++ {
		parent, ok := childSessionParent(key)
		if !ok {
			return depth
		}
		key = parent
	}
	return agentsession.MaxAttributes + 1
}

func (session *Session) Delete(ctx context.Context) error {
	if session == nil {
		return nil
	}
	if session.agent == nil {
		return ErrSessionClosed
	}
	key := session.Key()
	if err := session.Close(ctx); err != nil {
		return err
	}
	return session.agent.store.Delete(ctx, key)
}
