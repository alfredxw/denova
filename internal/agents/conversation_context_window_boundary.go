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

// providerVisibleCompactionSource maps only the accepted user message whose
// exact canonical/effective pair was frozen at the provider boundary. Other
// source messages remain canonical, so transient wrappers cannot leak into the
// durable checkpoint payload.
func (c *SessionConversation) providerVisibleCompactionSource(
	source []*agent.Message,
	sourceStart int,
) ([]*agent.Message, bool) {
	if c == nil || len(source) == 0 {
		return nil, false
	}
	c.cycleMu.Lock()
	base := c.contextWindowBase
	if base != nil {
		base = &contextWindowModelBase{
			cursor: base.cursor, canonical: cloneContextMessages(base.canonical), effective: cloneContextMessages(base.effective),
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
	// rememberContextWindowModelBase runs after the accepted input commit, so
	// the committed user occupies the message immediately before this cursor.
	offset := base.cursor.MessageCount - 1 - sourceStart
	if offset < 0 || offset >= len(source) || !reflect.DeepEqual(source[offset], canonicalUser) {
		// A durable rewind makes raw message indexes intentionally non-linear.
		// Accept the current-user mapping only when the entire supplied source is
		// exactly the frozen canonical provider projection (minus checkpoint
		// records), never by searching for similar text.
		frozenSource := compactionSourceMessages(base.canonical, true)
		if len(frozenSource) == 0 || len(source) < len(frozenSource) ||
			!contextMessagesEqual(source[:len(frozenSource)], frozenSource) ||
			!reflect.DeepEqual(source[len(frozenSource)-1], canonicalUser) {
			return nil, false
		}
		offset = len(frozenSource) - 1
	}
	if reflect.DeepEqual(canonicalUser, providerUser) {
		return nil, false
	}
	mapped := cloneContextMessages(source)
	mapped[offset] = providerUser
	return mapped, true
}

// stagedRewindCompactionSource resolves a same-cycle rewind before its
// structural operation is durably attached to the assistant commit. The
// frozen boundary is the only authority for mapping its canonical and
// provider-visible prefixes; failure to prove the exact prefix is fatal so a
// cold fallback can never summarize the discarded raw branch.
func (c *SessionConversation) stagedRewindCompactionSource(
	modelMessages []*agent.Message,
	keepLatestUser bool,
) (canonical, provider []*agent.Message, existingCheckpoint string, sourceStart, sourceEnd int, found bool, err error) {
	if c == nil || c.session == nil {
		return nil, nil, "", 0, 0, false, nil
	}
	c.cycleMu.Lock()
	var rewind *session.ContextOperation
	for index := len(c.pendingContextOps) - 1; index >= 0; index-- {
		operation := c.pendingContextOps[index]
		if operation.Kind == session.ContextOperationRewind && operation.AgentKind == c.agentKind {
			copy := operation
			rewind = &copy
			break
		}
	}
	c.cycleMu.Unlock()
	if rewind == nil {
		return nil, nil, "", 0, 0, false, nil
	}
	boundary := rewind.ResolvedBoundary
	if boundary == nil {
		return nil, nil, "", 0, 0, true, fmt.Errorf("staged context rewind %q has no resolved boundary", rewind.CheckpointID)
	}
	rewindSummary := newContextRewindSummaryMessage(*rewind)
	effectivePrefix := append(cloneContextMessages(boundary.EffectivePrefix), rewindSummary.Clone())
	if !contextMessagesHavePrefix(modelMessages, effectivePrefix) {
		return nil, nil, "", 0, 0, true, fmt.Errorf("staged context rewind %q does not match the current provider prefix", rewind.CheckpointID)
	}
	canonicalBody := cloneContextMessages(boundary.CanonicalPrefix)
	protectedPrefix, _ := splitProtectedModelPrefix(boundary.EffectivePrefix)
	injectedPrefixCount := len(boundary.EffectivePrefix) - len(canonicalBody)
	if injectedPrefixCount < 0 || injectedPrefixCount > len(protectedPrefix) {
		return nil, nil, "", 0, 0, true, fmt.Errorf(
			"staged context rewind %q canonical/provider boundary lengths cannot be aligned: canonical=%d effective=%d protected=%d",
			rewind.CheckpointID, len(canonicalBody), len(boundary.EffectivePrefix), len(protectedPrefix),
		)
	}
	providerBody := cloneContextMessages(boundary.EffectivePrefix[injectedPrefixCount:])
	suffix := cloneContextMessages(modelMessages[len(effectivePrefix):])
	canonicalProjection := append(canonicalBody, rewindSummary.Clone())
	canonicalProjection = append(canonicalProjection, suffix...)
	providerProjection := append(cloneContextMessages(providerBody), rewindSummary.Clone())
	providerProjection = append(providerProjection, cloneContextMessages(suffix)...)
	latestUserExcluded := !keepLatestUser && len(canonicalProjection) > 0 &&
		canonicalProjection[len(canonicalProjection)-1] != nil && canonicalProjection[len(canonicalProjection)-1].Role == agent.User
	canonical = compactionSourceMessages(canonicalProjection, keepLatestUser)
	provider = compactionSourceMessages(providerProjection, keepLatestUser)
	if len(canonical) != len(provider) {
		return nil, nil, "", 0, 0, true, fmt.Errorf("staged context rewind %q produced an invalid source mapping", rewind.CheckpointID)
	}
	snapshot, snapshotErr := c.session.SnapshotContext(c.agentKind)
	if snapshotErr != nil {
		return nil, nil, "", 0, 0, true, snapshotErr
	}
	if snapshot.Compaction != nil {
		existingCheckpoint = snapshot.Compaction.Summary
	}
	sourceStart = snapshot.Cursor.ClearAfterIndex
	sourceEnd = snapshot.Cursor.MessageCount
	if latestUserExcluded && sourceEnd > sourceStart {
		sourceEnd--
	}
	return canonical, provider, existingCheckpoint, sourceStart, sourceEnd, true, nil
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
