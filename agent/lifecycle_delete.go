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
	keys, err := agent.store.List(ctx, selector)
	if err != nil {
		return err
	}
	byCanonical := make(map[string]SessionKey, len(keys))
	queue := make([]string, 0, len(keys))
	remember := func(key SessionKey) error {
		canonical, canonicalErr := agentsession.CanonicalKey(key)
		if canonicalErr != nil {
			return canonicalErr
		}
		if _, exists := byCanonical[canonical]; exists {
			return nil
		}
		byCanonical[canonical] = key
		queue = append(queue, canonical)
		return nil
	}
	for _, key := range keys {
		if err := remember(key); err != nil {
			return err
		}
	}
	agent.mu.RLock()
	for _, session := range agent.sessions {
		key := session.key
		if !sessionSelectorMatchesTree(selector, key) {
			continue
		}
		if err := remember(key); err != nil {
			agent.mu.RUnlock()
			return err
		}
	}
	agent.mu.RUnlock()
	for next := 0; next < len(queue); next++ {
		canonical := queue[next]
		parent := byCanonical[canonical]
		attributes, attributeErr := ChildSessionAttributes(parent)
		if attributeErr != nil {
			return attributeErr
		}
		children, listErr := agent.store.List(ctx, SessionSelector{Attributes: attributes})
		if listErr != nil {
			return listErr
		}
		for _, child := range children {
			if err := remember(child); err != nil {
				return err
			}
		}
	}
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
