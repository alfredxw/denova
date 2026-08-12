package agent

import (
	"context"
	"errors"
	"sort"

	agentsession "github.com/alfredxw/denova/agent/session"
)

// DeleteSessions is the durable lifecycle counterpart to CloseSessions. It
// fences every matching runtime lane, releases its execution lease, and then
// removes the exact Session records from the configured Store. A later Open
// with the same Key starts from an empty Session.
func (agent *Agent) DeleteSessions(ctx context.Context, selector SessionSelector) error {
	if agent == nil || agent.runtime == nil || agent.store == nil {
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
	// Only one delete owns the Store catalog at a time. The fence blocks new
	// matching Session calls while allowing unrelated Sessions to keep opening.
	// Waiting for already admitted opens before Runtime.CloseBindings closes the
	// otherwise racy public-registry -> runtime-registry handoff.
	var fence *sessionDeletionFence
	for {
		agent.mu.Lock()
		if agent.closed {
			agent.mu.Unlock()
			return ErrAgentClosed
		}
		if prior := agent.sessionDeletion; prior != nil {
			done := prior.done
			agent.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return ctx.Err()
			case <-agent.ctx.Done():
				return ErrAgentClosed
			}
		}
		// Descendants encode the exact canonical parent key. The predicate can
		// therefore fence the whole tree immediately, without consulting a
		// process-local task registry or blocking on Store I/O while holding mu.
		matches := func(key SessionKey) bool { return sessionSelectorMatchesTree(selector, key) }
		fence = &sessionDeletionFence{matches: matches, done: make(chan struct{})}
		agent.sessionDeletion = fence
		openings := make([]<-chan struct{}, 0, len(agent.sessionOpenings))
		for _, opening := range agent.sessionOpenings {
			if matches(opening.key) {
				openings = append(openings, opening.done)
			}
		}
		agent.mu.Unlock()
		for _, done := range openings {
			select {
			case <-done:
			case <-ctx.Done():
				agent.releaseSessionDeletion(fence)
				return ctx.Err()
			case <-agent.ctx.Done():
				agent.releaseSessionDeletion(fence)
				return ErrAgentClosed
			}
		}
		break
	}
	defer agent.releaseSessionDeletion(fence)

	keys, err := agent.store.List(ctx, SessionSelector{All: true})
	if err != nil {
		return err
	}
	// Include a binding that has opened in this process but has not yet become
	// visible to an eventually consistent custom Store catalog.
	byCanonical := make(map[string]SessionKey, len(keys))
	for _, key := range keys {
		canonical, canonicalErr := agentsessionCanonical(key)
		if canonicalErr != nil {
			return canonicalErr
		}
		if fence.matches(key) {
			byCanonical[canonical] = key
		}
	}
	agent.mu.RLock()
	for canonical, key := range agent.sessions {
		if fence.matches(key) {
			byCanonical[canonical] = key
		}
	}
	agent.mu.RUnlock()
	canonicalKeys := make([]string, 0, len(byCanonical))
	for canonical := range byCanonical {
		canonicalKeys = append(canonicalKeys, canonical)
	}
	sort.Strings(canonicalKeys)
	var result error
	// Descendants close before parents. This prevents a parent deletion from
	// completing while an active child still owns a durable host-effect fence.
	sort.SliceStable(canonicalKeys, func(left, right int) bool {
		return sessionTreeDepth(byCanonical[canonicalKeys[left]]) > sessionTreeDepth(byCanonical[canonicalKeys[right]])
	})
	for _, canonical := range canonicalKeys {
		key := byCanonical[canonical]
		if closeErr := agent.runtime.CloseBinding(ctx, bindingForSession(key)); closeErr != nil {
			// Never erase a Session whose runtime lane could not be durably
			// fenced. It may still own an output commit or unresolved host effect.
			result = errors.Join(result, mapRuntimeError(closeErr))
			continue
		}
		if deleteErr := agent.store.Delete(ctx, key); deleteErr != nil {
			result = errors.Join(result, deleteErr)
			continue
		}
		agent.mu.Lock()
		delete(agent.sessions, canonical)
		agent.mu.Unlock()
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
	if err != nil {
		return SessionKey{}, false
	}
	return parent, true
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

func (agent *Agent) releaseSessionDeletion(fence *sessionDeletionFence) {
	if agent == nil || fence == nil {
		return
	}
	agent.mu.Lock()
	if agent.sessionDeletion == fence {
		agent.sessionDeletion = nil
		close(fence.done)
	}
	agent.mu.Unlock()
}

// Delete closes this handle and removes the complete durable Session. Other
// handles for the same Key become unusable when the runtime lane is fenced.
func (session *Session) Delete(ctx context.Context) error {
	if session == nil {
		return nil
	}
	if session.agent == nil {
		return ErrSessionClosed
	}
	session.mu.Lock()
	session.closed = true
	session.mu.Unlock()
	return session.agent.DeleteSessions(ctx, SessionSelector{
		Namespace: session.key.Namespace, ID: session.key.ID, Attributes: cloneStringMap(session.key.Attributes),
	})
}
