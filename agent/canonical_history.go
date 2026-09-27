package agent

import (
	"context"
	"encoding/json"
	"errors"
)

// CanonicalHistoryHead identifies the immutable product lane and its current
// model-visible revision. Revision must use the same identity as CommitReceipt;
// display-only appends must not advance it. Neither field may contain host paths.
// Identity must change on Clear or any edit/reordering of an existing prefix,
// including edits to messages currently covered by a compaction checkpoint.
type CanonicalHistoryHead struct {
	Identity string `json:"identity"`
	Revision string `json:"revision"`
}

// CanonicalHistorySource owns the full history needed for reconstruction and
// explicit compaction removal. Aligned checkpoints need only the cheap head.
// Read and Head must observe one canonical lane, including external edits/Clear.
type CanonicalHistorySource interface {
	CanonicalHistoryHead(context.Context) (CanonicalHistoryHead, error)
	CanonicalMessages(context.Context) ([]*Message, error)
}

// LoadCanonicalHistory reuses an aligned active checkpoint or reconstructs it
// from the canonical source. Call it before admission and structural operations.
func (session *Session) LoadCanonicalHistory(ctx context.Context, source CanonicalHistorySource) error {
	if err := session.usable(); err != nil {
		return err
	}
	if source == nil {
		return errors.New("canonical history source is required")
	}
	head, err := source.CanonicalHistoryHead(ctx)
	if err != nil {
		return err
	}
	if head.Identity == "" || head.Revision == "" {
		return errors.New("canonical history head is incomplete")
	}
	session.mu.Lock()
	if session.active != nil && !session.active.isSuspended() || session.maintenance {
		session.mu.Unlock()
		return ErrSessionBusy
	}
	session.canonicalSource = source
	checkpoint := session.messageCheckpoint
	state := session.engineState
	if len(state) == 0 && checkpoint.Archive != nil {
		state = checkpoint.Metadata
	}
	if len(state) != 0 {
		transcript, decodeErr := decodeEngineTranscript(state)
		if decodeErr != nil {
			session.mu.Unlock()
			return decodeErr
		}
		if transcript.HistoryHead == head {
			session.engineState = append(json.RawMessage(nil), state...)
			session.mu.Unlock()
			return nil
		}
	}
	session.mu.Unlock()
	messages, err := source.CanonicalMessages(ctx)
	if err != nil {
		return err
	}
	after, err := source.CanonicalHistoryHead(ctx)
	if err != nil {
		return err
	}
	if after != head {
		return ErrSessionBusy
	}
	return session.loadCanonicalMessages(ctx, messages, head)
}

// expandCanonicalArchive is reserved for explicit removal/rebuild. Normal
// model steps never expand archived bodies. The source check is performed by
// the same import boundary as a cold reconstruction.
func (session *Session) expandCanonicalArchive(ctx context.Context, state engineTranscript) (engineTranscript, error) {
	session.mu.RLock()
	source := session.canonicalSource
	session.mu.RUnlock()
	if source == nil {
		return engineTranscript{}, errors.New("canonical history source is unavailable for compaction removal")
	}
	head, err := source.CanonicalHistoryHead(ctx)
	if err != nil {
		return engineTranscript{}, err
	}
	if head != state.HistoryHead {
		return engineTranscript{}, ErrDefinitionMismatch
	}
	messages, err := source.CanonicalMessages(ctx)
	if err != nil {
		return engineTranscript{}, err
	}
	after, err := source.CanonicalHistoryHead(ctx)
	if err != nil {
		return engineTranscript{}, err
	}
	if head != after {
		return engineTranscript{}, ErrSessionBusy
	}
	ordered := canonicalContextStateOrder(messages)
	if err := validateImportedTranscript(ordered); err != nil {
		return engineTranscript{}, err
	}
	if len(ordered) != state.Archive.count(state.Messages) {
		return engineTranscript{}, ErrDefinitionMismatch
	}
	state.Messages, state.Archive = ordered, nil
	state.Version = engineTranscriptVersion
	return state, nil
}

// encodeCanonicalWindow retains only the bodies needed by the accepted
// compaction projection. Direct imports without an archive source stay whole.
func encodeCanonicalWindow(state engineTranscript, capabilities map[string]json.RawMessage) (json.RawMessage, error) {
	if state.HistoryHead.Identity == "" && state.Archive == nil {
		return json.Marshal(state)
	}
	clear, clearPresent, err := clearStateFrom(capabilities)
	if err != nil {
		return nil, err
	}
	compact, present, err := compactionStateFrom(capabilities)
	if err != nil {
		return nil, err
	}
	compact, present = clearCompaction(compact, present, clear, clearPresent)
	if state.HistoryHead.Identity != "" && present && !compact.Removed {
		state.Messages, state.Archive = archiveHistory(state.Messages, state.Archive, compact, state.ContextState)
	}
	if state.Archive != nil && (!present || compact.Removed) {
		return nil, errors.New("canonical archive requires its accepted compaction")
	}
	state.Version = transcriptVersion(state.Archive)
	return json.Marshal(state)
}
