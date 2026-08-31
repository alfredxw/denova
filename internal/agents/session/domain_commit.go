package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	agent "github.com/alfredxw/denova/agent"

	agentcontext "denova/internal/agents/context"
)

var (
	// ErrDomainCommitIdentityConflict means one coordinator cycle attempted to
	// publish different canonical content under an already-used identity.
	ErrDomainCommitIdentityConflict = errors.New("session domain commit identity conflict")
	// ErrContextRevisionConflict protects structural context changes from a
	// stale UI or Agent snapshot.
	ErrContextRevisionConflict = errors.New("session context revision conflict")
)

// DomainCommitIdentity is the stable coordinator identity shared by runtime
// and canonical writing/game storage. All fields are required together.
type DomainCommitIdentity struct {
	CommandID   string `json:"command_id"`
	OperationID string `json:"operation_id"`
	Cycle       int    `json:"cycle"`
}

// ContextCursor is an immutable revision barrier for model-visible session
// state. MessageCount and ClearAfterIndex make diagnostics actionable while
// Revision is the compare-and-swap key.
type ContextCursor struct {
	Revision        uint64 `json:"revision"`
	MessageCount    int    `json:"message_count"`
	ClearAfterIndex int    `json:"clear_after_index"`
}

// ContextSnapshot atomically captures every session component used to build
// one model input. Cursor is the CAS barrier for publishing that input.
type ContextSnapshot struct {
	EffectiveMessages []*agent.Message
	Cursor            ContextCursor
}

// DomainCommitIntent is staged by a conversation and authorized by the
// durable Agent actor before it reaches canonical storage.
type DomainCommitIntent struct {
	Identity       DomainCommitIdentity
	Message        agent.Message
	Metadata       MessageMetadata
	Hash           string
	ExpectedCursor *ContextCursor
}

// DomainCommitReceipt proves the exact canonical message and context revision
// written for an authorized Agent cycle.
type DomainCommitReceipt struct {
	Identity           DomainCommitIdentity `json:"identity"`
	MessageID          string               `json:"message_id"`
	Hash               string               `json:"hash"`
	AgentCanonicalHash string               `json:"agent_canonical_hash,omitempty"`
	ContextRevision    uint64               `json:"context_revision"`
}

func NewDomainCommitIntent(identity DomainCommitIdentity, message *agent.Message, metadata MessageMetadata) (DomainCommitIntent, error) {
	identity = normalizeDomainCommitIdentity(identity)
	if err := validateDomainCommitIdentity(identity); err != nil {
		return DomainCommitIntent{}, err
	}
	if message == nil || (message.Role == "" && strings.TrimSpace(message.Content) == "" && len(message.ToolCalls) == 0) {
		return DomainCommitIntent{}, fmt.Errorf("domain commit message is empty")
	}
	messageCopy := *message
	metadata = sanitizeMessageMetadata(metadata)
	hash, err := domainMessageHash(messageCopy, metadata)
	if err != nil {
		return DomainCommitIntent{}, err
	}
	metadata.MessageID = deterministicDomainMessageID(identity, messageCopy.Role)
	metadata.AgentCommandID = identity.CommandID
	metadata.AgentOperationID = identity.OperationID
	metadata.AgentCycle = identity.Cycle
	metadata.DomainCommitHash = hash
	return DomainCommitIntent{Identity: identity, Message: messageCopy, Metadata: metadata, Hash: hash}, nil
}

// WithExpectedContextCursor adds an optional CAS barrier. Most Agent output is
// committed against the latest context; structural operations should opt in.
func (i DomainCommitIntent) WithExpectedContextCursor(cursor ContextCursor) DomainCommitIntent {
	i.ExpectedCursor = &cursor
	return i
}

// WithAgentCanonicalHash binds the product-domain write to the exact hash
// journaled by the public Agent lifecycle. The domain payload hash remains
// independent so Denova can reject richer product-state drift as well.
func (i DomainCommitIntent) WithAgentCanonicalHash(hash string) (DomainCommitIntent, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return DomainCommitIntent{}, fmt.Errorf("%w: Agent canonical hash is required", ErrDomainCommitIdentityConflict)
	}
	i.Metadata.AgentCanonicalHash = hash
	return i, nil
}

// CommitDomainMessage performs canonical idempotent publication. Exact retries
// return the original receipt; semantic identity reuse is rejected.
func (s *Session) CommitDomainMessage(intent DomainCommitIntent) (DomainCommitReceipt, error) {
	return s.CommitDomainMessageContext(context.Background(), intent)
}

// CommitDomainMessageContext serializes canonical domain messages across
// independently loaded Session instances, refreshes the latest journal before
// comparing identity+hash, and appends at most one exact record.
func (s *Session) CommitDomainMessageContext(ctx context.Context, intent DomainCommitIntent) (_ DomainCommitReceipt, resultErr error) {
	var receipt DomainCommitReceipt
	resultErr = s.withCanonicalMutation(ctx, "domain commit", func() error {
		identity := normalizeDomainCommitIdentity(intent.Identity)
		if err := validateDomainCommitIdentity(identity); err != nil {
			return err
		}
		intent.Metadata = sanitizeMessageMetadata(intent.Metadata)
		canonicalHash := intent.Metadata.AgentCanonicalHash
		actualHash, err := domainMessageHash(intent.Message, intent.Metadata)
		if err != nil {
			return err
		}
		if strings.TrimSpace(intent.Hash) == "" || actualHash != strings.TrimSpace(intent.Hash) {
			return fmt.Errorf("%w: intent hash does not match message", ErrDomainCommitIdentityConflict)
		}
		existing, found, err := s.findDomainCommitLocked(identity, intent.Message.Role, actualHash, canonicalHash)
		if err != nil || found {
			receipt = existing
			return err
		}
		if intent.ExpectedCursor != nil {
			if current := s.contextCursorLocked(); current.Revision != intent.ExpectedCursor.Revision {
				return fmt.Errorf("%w: expected=%d current=%d", ErrContextRevisionConflict, intent.ExpectedCursor.Revision, current.Revision)
			}
		}
		metadata := sanitizeMessageMetadata(intent.Metadata)
		metadata.MessageID = deterministicDomainMessageID(identity, intent.Message.Role)
		metadata.AgentCommandID = identity.CommandID
		metadata.AgentOperationID = identity.OperationID
		metadata.AgentCycle = identity.Cycle
		metadata.DomainCommitHash = actualHash
		message := intent.Message
		kind := historyTypeMessage
		if metadata.ContextOnly {
			kind = historyTypeContextMessage
		}
		if err := s.appendDomainMessageLocked(&message, metadata, kind); err != nil {
			recoveryErr := s.refreshCanonicalTailLocked()
			if recoveryErr == nil {
				reconciled, found, reconcileErr := s.findDomainCommitLocked(identity, intent.Message.Role, actualHash, canonicalHash)
				if reconcileErr != nil {
					return errors.Join(err, reconcileErr)
				}
				if found {
					receipt = reconciled
					return nil
				}
			}
			if recoveryErr != nil {
				return errors.Join(err, fmt.Errorf("reconcile ambiguous session domain commit: %w", recoveryErr))
			}
			return err
		}
		metadata.ContextRevision = s.contextRevision
		receipt = domainCommitReceipt(identity, metadata)
		return nil
	})
	return receipt, resultErr
}

// appendDomainMessageLocked publishes the canonical message and its optional
// interruption resolution as one physical journal transaction. Callers hold
// s.mu under the canonical journal lease.
func (s *Session) appendDomainMessageLocked(message *agent.Message, metadata MessageMetadata, kind string) error {
	if message == nil {
		return errors.New("domain commit message is nil")
	}
	now := time.Now().UTC()
	metadata = sanitizeMessageMetadata(metadata)
	metadata.ContextRevision = s.contextRevision + 1
	records := []any{messageRecord{Type: kind, CreatedAt: now, Message: *message, MessageMetadata: metadata}}
	var interruptionPatch *interruptionPatchRecord
	if id := metadata.ResolveInterruptionID; id != "" {
		if !s.pendingInterruptionLocked(id) {
			return fmt.Errorf("canonical output interruption %q is not pending", id)
		}
		patch := interruptionPatchRecord{
			Type: historyTypeInterruptionPatch, TargetID: id, Status: InterruptionResolved,
			ResolvedAt: &now, UpdatedAt: now,
		}
		interruptionPatch = &patch
		records = append(records, patch)
	}
	if _, err := s.appendJournalRecordsLocked(records...); err != nil {
		return err
	}
	s.messages = append(s.messages, message)
	s.messageCount++
	s.records = append(s.records, historyRecord{
		kind: kind, message: message, messageMetadata: metadata, createdAt: now,
	})
	if s.title == defaultSessionTitle && message.Role == agent.User && strings.TrimSpace(message.Content) != "" {
		s.title = deriveTitle(message.Content)
	}
	s.contextRevision = metadata.ContextRevision
	if interruptionPatch != nil {
		s.applyInterruptionResolutionLocked(*interruptionPatch)
	}
	advanceUpdatedAt(s, now)
	return nil
}

func (s *Session) pendingInterruptionLocked(id string) bool {
	if s.projection != nil && s.projection.PendingInterrupt != nil &&
		s.projection.PendingInterrupt.ID == id && s.projection.PendingInterrupt.Status == InterruptionPending {
		return true
	}
	for index := len(s.records) - 1; index >= 0; index-- {
		record := s.records[index]
		if record.kind == historyTypeInterrupt && record.interruption != nil && record.interruption.ID == id {
			return record.interruption.Status == InterruptionPending
		}
	}
	return false
}

func (s *Session) applyInterruptionResolutionLocked(patch interruptionPatchRecord) {
	for index := range s.records {
		interruption := s.records[index].interruption
		if interruption == nil || interruption.ID != patch.TargetID {
			continue
		}
		interruption.Status = patch.Status
		interruption.ResolvedAt = patch.ResolvedAt
	}
	if s.projection != nil && s.projection.PendingInterrupt != nil && s.projection.PendingInterrupt.ID == patch.TargetID {
		s.projection.PendingInterrupt = nil
		s.projection.PendingInterruptCursor = 0
	}
}

// RefreshCanonical reloads the complete append-only journal under the same
// lease used by every journal mutation. It lets a long-lived Conversation
// observe input materialized through another Session instance before building
// model context.
func (s *Session) RefreshCanonical(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("session is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshCanonicalTailLocked(); err != nil {
		return fmt.Errorf("refresh session canonical state: %w", err)
	}
	return nil
}

// FindDomainCommit performs a read-only exact receipt lookup for crash
// recovery. A command reused with a different operation, cycle, role, or hash
// is a conflict rather than evidence that the requested write committed.
func (s *Session) FindDomainCommit(identity DomainCommitIdentity, role agent.RoleType, hash string) (DomainCommitReceipt, bool, error) {
	if s == nil {
		return DomainCommitReceipt{}, false, fmt.Errorf("session is nil")
	}
	identity = normalizeDomainCommitIdentity(identity)
	if err := validateDomainCommitIdentity(identity); err != nil {
		return DomainCommitReceipt{}, false, err
	}
	if role != agent.User && role != agent.Assistant {
		return DomainCommitReceipt{}, false, fmt.Errorf("%w: unsupported message role %q", ErrDomainCommitIdentityConflict, role)
	}
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return DomainCommitReceipt{}, false, fmt.Errorf("%w: commit hash is required", ErrDomainCommitIdentityConflict)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshCanonicalTailLocked(); err != nil {
		return DomainCommitReceipt{}, false, fmt.Errorf("refresh session before domain commit lookup: %w", err)
	}
	return s.findDomainCommitLocked(identity, role, hash, "")
}

// FindAgentCanonicalCommit proves that the exact public Agent stage hash was
// published in the same journal transaction as the product message.
func (s *Session) FindAgentCanonicalCommit(identity DomainCommitIdentity, role agent.RoleType, hash string) (DomainCommitReceipt, bool, error) {
	if s == nil {
		return DomainCommitReceipt{}, false, fmt.Errorf("session is nil")
	}
	identity = normalizeDomainCommitIdentity(identity)
	if err := validateDomainCommitIdentity(identity); err != nil {
		return DomainCommitReceipt{}, false, err
	}
	if role != agent.User && role != agent.Assistant {
		return DomainCommitReceipt{}, false, fmt.Errorf("%w: unsupported message role %q", ErrDomainCommitIdentityConflict, role)
	}
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return DomainCommitReceipt{}, false, fmt.Errorf("%w: Agent canonical hash is required", ErrDomainCommitIdentityConflict)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshCanonicalTailLocked(); err != nil {
		return DomainCommitReceipt{}, false, fmt.Errorf("refresh session before Agent canonical lookup: %w", err)
	}
	return s.findAgentCanonicalCommitLocked(identity, role, hash)
}

func (s *Session) ContextCursor() ContextCursor {
	if s == nil {
		return ContextCursor{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.contextCursorLocked()
}

// GetEffectiveMessagesWithCursor snapshots both model-visible messages and
// their revision under one lock, avoiding a check-then-read race at cycle start.
func (s *Session) GetEffectiveMessagesWithCursor() ([]*agent.Message, ContextCursor) {
	if s == nil {
		return nil, ContextCursor{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.effectiveTranscriptMessagesLocked(), s.contextCursorLocked()
}

// SnapshotContext captures effective messages, structural projections, and
// their shared revision under one lock. Corrupt durable projections are an
// error so callers cannot silently fall back to discarded raw transcript.
func (s *Session) SnapshotContext() (ContextSnapshot, error) {
	if s == nil {
		return ContextSnapshot{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshotContextLocked()
}

// SnapshotContextForDomainCommit atomically returns model-visible context and
// the effective message index of one exact canonical commit. A commit hidden
// behind /clear remains durable but returns found=false so the current turn can
// still project its accepted input once.
func (s *Session) SnapshotContextForDomainCommit(
	identity DomainCommitIdentity,
	role agent.RoleType,
	hash string,
) (ContextSnapshot, int, bool, error) {
	if s == nil {
		return ContextSnapshot{}, 0, false, fmt.Errorf("session is nil")
	}
	identity = normalizeDomainCommitIdentity(identity)
	if err := validateDomainCommitIdentity(identity); err != nil {
		return ContextSnapshot{}, 0, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot, err := s.snapshotContextLocked()
	if err != nil {
		return ContextSnapshot{}, 0, false, err
	}
	messageIndex, receipt, found, err := s.findDomainCommitMessageIndexLocked(identity, role, strings.TrimSpace(hash), "")
	if err != nil || !found {
		return snapshot, 0, false, err
	}
	if receipt.Hash != strings.TrimSpace(hash) || messageIndex < s.clearAfterIndex {
		return snapshot, 0, false, nil
	}
	effectiveStart := max(s.clearAfterIndex, s.messageBaseIndex)
	effectiveIndex := messageIndex - effectiveStart
	if effectiveIndex < 0 || effectiveIndex >= len(snapshot.EffectiveMessages) {
		return ContextSnapshot{}, 0, false, fmt.Errorf("domain commit context index is inconsistent with session history")
	}
	return snapshot, effectiveIndex, true, nil
}

func (s *Session) snapshotContextLocked() (ContextSnapshot, error) {
	return ContextSnapshot{EffectiveMessages: s.effectiveTranscriptMessagesLocked(), Cursor: s.contextCursorLocked()}, nil
}

func (s *Session) AppendContextMessageAt(expected ContextCursor, msg *agent.Message) error {
	return s.AppendContextMessagesAt(expected, msg)
}

// AppendContextMessagesAt atomically publishes a context-only protocol batch
// against the exact model-visible revision used to produce it.
func (s *Session) AppendContextMessagesAt(expected ContextCursor, messages ...*agent.Message) error {
	if len(messages) == 0 {
		return nil
	}
	metadata := make([]MessageMetadata, len(messages))
	return s.withCanonicalMutation(context.Background(), "append context message with revision", func() error {
		if current := s.contextCursorLocked(); current.Revision != expected.Revision {
			return fmt.Errorf("%w: expected=%d current=%d", ErrContextRevisionConflict, expected.Revision, current.Revision)
		}
		return s.appendMessagesLocked(messages, metadata, historyTypeContextMessage)
	})
}

func (s *Session) contextCursorLocked() ContextCursor {
	return ContextCursor{Revision: s.contextRevision, MessageCount: s.messageCount, ClearAfterIndex: s.clearAfterIndex}
}

// AppendClearMarkerAt atomically rejects a clear based on stale model-visible
// state. Callers that intentionally target the latest state use AppendClearMarker.
func (s *Session) AppendClearMarkerAt(expected ContextCursor) error {
	return s.withCanonicalMutation(context.Background(), "append clear marker with revision", func() error {
		current := s.contextCursorLocked()
		if current.Revision != expected.Revision {
			return fmt.Errorf("%w: expected=%d current=%d", ErrContextRevisionConflict, expected.Revision, current.Revision)
		}
		return s.appendClearMarkerLocked()
	})
}

func normalizeDomainCommitIdentity(identity DomainCommitIdentity) DomainCommitIdentity {
	identity.CommandID = strings.TrimSpace(identity.CommandID)
	identity.OperationID = strings.TrimSpace(identity.OperationID)
	return identity
}

func validateDomainCommitIdentity(identity DomainCommitIdentity) error {
	if identity.CommandID == "" || identity.OperationID == "" || identity.Cycle <= 0 {
		return fmt.Errorf("%w: command_id, operation_id, and positive cycle are required", ErrDomainCommitIdentityConflict)
	}
	return nil
}

func domainMessageHash(message agent.Message, metadata MessageMetadata) (string, error) {
	metadata = sanitizeMessageMetadata(metadata)
	payload := struct {
		Message  agent.Message `json:"message"`
		Metadata struct {
			RunID                 string                       `json:"run_id,omitempty"`
			AgentKind             string                       `json:"agent_kind,omitempty"`
			AgentName             string                       `json:"agent_name,omitempty"`
			RootAgentName         string                       `json:"root_agent_name,omitempty"`
			RunPath               []string                     `json:"run_path,omitempty"`
			SubAgent              bool                         `json:"subagent,omitempty"`
			SubAgentSessionID     string                       `json:"subagent_session_id,omitempty"`
			SubAgentType          string                       `json:"subagent_type,omitempty"`
			UserReferences        []agentcontext.UserReference `json:"user_references,omitempty"`
			DisplayContent        string                       `json:"display_content,omitempty"`
			ContextOnly           bool                         `json:"context_only,omitempty"`
			ResolveInterruptionID string                       `json:"resolve_interruption_id,omitempty"`
		} `json:"metadata"`
	}{Message: message}
	payload.Metadata.RunID = metadata.RunID
	payload.Metadata.AgentKind = metadata.AgentKind
	payload.Metadata.AgentName = metadata.AgentName
	payload.Metadata.RootAgentName = metadata.RootAgentName
	payload.Metadata.RunPath = append([]string(nil), metadata.RunPath...)
	payload.Metadata.SubAgent = metadata.SubAgent
	payload.Metadata.SubAgentSessionID = metadata.SubAgentSessionID
	payload.Metadata.SubAgentType = metadata.SubAgentType
	payload.Metadata.UserReferences = append([]agentcontext.UserReference(nil), metadata.UserReferences...)
	payload.Metadata.DisplayContent = metadata.DisplayContent
	payload.Metadata.ContextOnly = metadata.ContextOnly
	payload.Metadata.ResolveInterruptionID = metadata.ResolveInterruptionID
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("hash domain message: %w", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func deterministicDomainMessageID(identity DomainCommitIdentity, role agent.RoleType) string {
	sum := sha256.Sum256([]byte(identity.CommandID + "\x00" + identity.OperationID + "\x00" + fmt.Sprint(identity.Cycle) + "\x00" + string(role)))
	return "agent-message-" + hex.EncodeToString(sum[:16])
}

func domainCommitReceipt(identity DomainCommitIdentity, metadata MessageMetadata) DomainCommitReceipt {
	return DomainCommitReceipt{
		Identity: identity, MessageID: metadata.MessageID,
		Hash: metadata.DomainCommitHash, AgentCanonicalHash: metadata.AgentCanonicalHash,
		ContextRevision: metadata.ContextRevision,
	}
}

func (s *Session) findDomainCommitLocked(identity DomainCommitIdentity, role agent.RoleType, hash, canonicalHash string) (DomainCommitReceipt, bool, error) {
	_, receipt, found, err := s.findDomainCommitMessageIndexLocked(identity, role, hash, canonicalHash)
	return receipt, found, err
}

func (s *Session) findDomainCommitMessageIndexLocked(
	identity DomainCommitIdentity,
	role agent.RoleType,
	hash, canonicalHash string,
) (int, DomainCommitReceipt, bool, error) {
	wantedMessageID := deterministicDomainMessageID(identity, role)
	messageIndex := s.messageBaseIndex
	for _, record := range s.records {
		if record.message == nil {
			continue
		}
		metadata := record.messageMetadata
		if metadata.AgentCommandID != identity.CommandID {
			messageIndex++
			continue
		}
		if metadata.AgentOperationID != identity.OperationID || metadata.AgentCycle != identity.Cycle {
			return 0, DomainCommitReceipt{}, false, fmt.Errorf("%w: command_id=%q operation_id=%q cycle=%d", ErrDomainCommitIdentityConflict, identity.CommandID, identity.OperationID, identity.Cycle)
		}
		if metadata.MessageID != wantedMessageID {
			messageIndex++
			continue
		}
		if metadata.DomainCommitHash != hash {
			return 0, DomainCommitReceipt{}, false, fmt.Errorf("%w: command_id=%q operation_id=%q cycle=%d", ErrDomainCommitIdentityConflict, identity.CommandID, identity.OperationID, identity.Cycle)
		}
		if canonicalHash != "" && metadata.AgentCanonicalHash != canonicalHash {
			return 0, DomainCommitReceipt{}, false, fmt.Errorf("%w: Agent canonical hash changed for command_id=%q operation_id=%q cycle=%d", ErrDomainCommitIdentityConflict, identity.CommandID, identity.OperationID, identity.Cycle)
		}
		return messageIndex, domainCommitReceipt(identity, metadata), true, nil
	}
	if s.projection != nil {
		wantedMessageID := deterministicDomainMessageID(identity, role)
		for index := len(s.projection.RecentCommits) - 1; index >= 0; index-- {
			commit := s.projection.RecentCommits[index]
			metadata := commit.Metadata
			if metadata.AgentCommandID != identity.CommandID {
				continue
			}
			if metadata.AgentOperationID != identity.OperationID || metadata.AgentCycle != identity.Cycle {
				return 0, DomainCommitReceipt{}, false, fmt.Errorf("%w: command_id=%q operation_id=%q cycle=%d", ErrDomainCommitIdentityConflict, identity.CommandID, identity.OperationID, identity.Cycle)
			}
			if commit.Role != role || metadata.MessageID != wantedMessageID {
				continue
			}
			if metadata.DomainCommitHash != hash {
				return 0, DomainCommitReceipt{}, false, fmt.Errorf("%w: command_id=%q operation_id=%q cycle=%d", ErrDomainCommitIdentityConflict, identity.CommandID, identity.OperationID, identity.Cycle)
			}
			if canonicalHash != "" && metadata.AgentCanonicalHash != canonicalHash {
				return 0, DomainCommitReceipt{}, false, fmt.Errorf("%w: Agent canonical hash changed for command_id=%q operation_id=%q cycle=%d", ErrDomainCommitIdentityConflict, identity.CommandID, identity.OperationID, identity.Cycle)
			}
			return commit.MessageIndex, domainCommitReceipt(identity, metadata), true, nil
		}
	}
	return 0, DomainCommitReceipt{}, false, nil
}

func (s *Session) findAgentCanonicalCommitLocked(identity DomainCommitIdentity, role agent.RoleType, hash string) (DomainCommitReceipt, bool, error) {
	wantedMessageID := deterministicDomainMessageID(identity, role)
	for _, record := range s.records {
		if record.message == nil {
			continue
		}
		metadata := record.messageMetadata
		if metadata.AgentCommandID != identity.CommandID {
			continue
		}
		if metadata.AgentOperationID != identity.OperationID || metadata.AgentCycle != identity.Cycle {
			return DomainCommitReceipt{}, false, fmt.Errorf("%w: Agent canonical commit does not match command_id=%q operation_id=%q cycle=%d", ErrDomainCommitIdentityConflict, identity.CommandID, identity.OperationID, identity.Cycle)
		}
		if metadata.MessageID != wantedMessageID {
			continue
		}
		if metadata.AgentCanonicalHash != hash {
			return DomainCommitReceipt{}, false, fmt.Errorf("%w: Agent canonical hash changed for command_id=%q operation_id=%q cycle=%d", ErrDomainCommitIdentityConflict, identity.CommandID, identity.OperationID, identity.Cycle)
		}
		return domainCommitReceipt(identity, metadata), true, nil
	}
	if s.projection != nil {
		for index := len(s.projection.RecentCommits) - 1; index >= 0; index-- {
			commit := s.projection.RecentCommits[index]
			metadata := commit.Metadata
			if metadata.AgentCommandID != identity.CommandID {
				continue
			}
			if metadata.AgentOperationID != identity.OperationID || metadata.AgentCycle != identity.Cycle {
				return DomainCommitReceipt{}, false, fmt.Errorf("%w: Agent canonical commit does not match command_id=%q operation_id=%q cycle=%d", ErrDomainCommitIdentityConflict, identity.CommandID, identity.OperationID, identity.Cycle)
			}
			if commit.Role != role || metadata.MessageID != wantedMessageID {
				continue
			}
			if metadata.AgentCanonicalHash != hash {
				return DomainCommitReceipt{}, false, fmt.Errorf("%w: Agent canonical hash changed for command_id=%q operation_id=%q cycle=%d", ErrDomainCommitIdentityConflict, identity.CommandID, identity.OperationID, identity.Cycle)
			}
			return domainCommitReceipt(identity, metadata), true, nil
		}
	}
	return DomainCommitReceipt{}, false, nil
}

func (s *Session) replaceCanonicalStateLocked(recovered *Session) {
	if s == nil || recovered == nil {
		return
	}
	s.ID = recovered.ID
	s.CreatedAt = recovered.CreatedAt
	s.UpdatedAt = recovered.UpdatedAt
	s.title = recovered.title
	s.clearAfterIndex = recovered.clearAfterIndex
	s.contextRevision = recovered.contextRevision
	s.runtimeConfig = nil
	s.runtimeConfigRevision = recovered.runtimeConfigRevision
	if recovered.runtimeConfig != nil {
		value := *recovered.runtimeConfig
		s.runtimeConfig = &value
	}
	s.journalSize = recovered.journalSize
	s.journalOffset = recovered.journalOffset
	s.journalIncarnation = recovered.journalIncarnation
	s.journalNeedsLF = recovered.journalNeedsLF
	s.journalLineCount = recovered.journalLineCount
	s.lastReplayBytes = recovered.lastReplayBytes
	s.lastReplayRecords = recovered.lastReplayRecords
	s.journal = recovered.journal
	s.projection = recovered.projection
	s.materializedCursor = recovered.materializedCursor
	s.messageBaseIndex = recovered.messageBaseIndex
	s.messageCount = recovered.messageCount
	s.historyBaseIndex = recovered.historyBaseIndex
	s.partialMaterialization = recovered.partialMaterialization
	s.messages = recovered.messages
	s.records = recovered.records
}
