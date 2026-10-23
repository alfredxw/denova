package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	agenthistory "github.com/alfredxw/denova/agent/context/history"
	agentschema "github.com/alfredxw/denova/agent/schema"
	agentsession "github.com/alfredxw/denova/agent/session"
	agentcanonical "github.com/alfredxw/denova/agent/session/canonical"
)

// CanonicalImportRequest is a detached snapshot. Planning never mutates the
// journal or the caller's capability map.
type CanonicalImportRequest struct {
	Session         agentsession.Key
	Messages        []*agentschema.Message
	Head            agentcanonical.CanonicalHistoryHead
	State           json.RawMessage
	Checkpoint      PersistedMessageCheckpoint
	Capabilities    map[string]json.RawMessage
	Active          bool
	OutputCommitted bool
	Canonical       bool
}

// CanonicalImport carries the complete projection and invalidations to commit
// together. Lifecycle publishes these values only after journal acceptance.
type CanonicalImport struct {
	State         json.RawMessage
	Checkpoint    PersistedMessageCheckpoint
	Invalidated   []string
	CompletionIDs []string
	Persist       bool
}

func PrepareCanonicalImport(ctx context.Context, request CanonicalImportRequest) (CanonicalImport, error) {
	ordered := agentcanonical.CanonicalContextStateOrder(request.Messages)
	if err := agentcanonical.ValidateImportedTranscript(ordered); err != nil {
		return CanonicalImport{}, fmt.Errorf("%w: %v", agentcanonical.ErrInvalidCanonicalMessages, err)
	}
	contextState, err := agenthistory.RebuildContextStateSnapshot(ordered)
	if err != nil {
		return CanonicalImport{}, fmt.Errorf("%w: %v", agentcanonical.ErrInvalidCanonicalMessages, err)
	}
	result := CanonicalImport{}
	capabilities := agentschema.CloneRawStateMap(request.Capabilities)
	hadCurrentTranscript := len(request.State) != 0
	currentMessageCount := 0
	currentMatches := false
	currentCompatible := false
	if hadCurrentTranscript {
		current, decodeErr := decodeEngineTranscript(request.State)
		if decodeErr != nil {
			return CanonicalImport{}, decodeErr
		}
		currentMessageCount = current.Archive.Count(current.Messages)
		if currentMessageCount <= len(ordered) && (request.Head.Identity == "" || current.HistoryHead.Identity == "" || request.Head.Identity == current.HistoryHead.Identity) {
			currentHash, hashErr := agentschema.HashCanonicalMessages(current.Messages)
			if hashErr != nil {
				return CanonicalImport{}, hashErr
			}
			prefix, selectErr := current.Archive.SelectMessages(ordered[:currentMessageCount])
			if selectErr != nil {
				return CanonicalImport{}, selectErr
			}
			prefixHash, hashErr := agentschema.HashCanonicalMessages(prefix)
			if hashErr != nil {
				return CanonicalImport{}, hashErr
			}
			currentCompatible = currentHash == prefixHash
			currentMatches = currentCompatible && currentMessageCount == len(ordered)
		}
	}
	hadCheckpoint := strings.TrimSpace(request.Checkpoint.Hash) != ""
	checkpointCompatible := false
	var metadata engineTranscript
	if len(request.Checkpoint.Metadata) != 0 {
		if err := json.Unmarshal(request.Checkpoint.Metadata, &metadata); err != nil {
			return CanonicalImport{}, err
		}
	}
	if hadCheckpoint && request.Checkpoint.MessageCount <= len(ordered) && (request.Head.Identity == "" || metadata.HistoryHead.Identity == "" || request.Head.Identity == metadata.HistoryHead.Identity) {
		prefix, selectErr := request.Checkpoint.Archive.SelectMessages(ordered[:request.Checkpoint.MessageCount])
		if selectErr != nil {
			return CanonicalImport{}, selectErr
		}
		prefixHash, hashErr := agentschema.HashCanonicalMessages(prefix)
		if hashErr != nil {
			return CanonicalImport{}, hashErr
		}
		checkpointCompatible = request.Checkpoint.Hash == prefixHash
	}
	outputCommitted := request.OutputCommitted
	if request.Active && hadCheckpoint && !checkpointCompatible && !outputCommitted {
		return CanonicalImport{}, fmt.Errorf("%w: canonical history changed under an unfinished Run (imported=%d checkpoint=%d)", agentcanonical.ErrInvalidCanonicalMessages, len(ordered), request.Checkpoint.MessageCount)
	}
	// Capability records live in the same canonical journal as host messages.
	// A cold session with no message checkpoint can therefore trust them (this
	// is also how released Product Session compactions are imported). Only a
	// previously observed prefix can prove that source messages were edited or
	// removed underneath those projections. Ordinary append-only progress is
	// compatible even when a crash occurred before the next checkpoint.
	if (hadCurrentTranscript || hadCheckpoint) && !currentCompatible && !checkpointCompatible {
		var invalidated []string
		for _, capability := range []string{
			agenthistory.ClearCapability, agenthistory.CleanupCapability, agenthistory.ElisionCapability, agenthistory.CompactionCapability, agenthistory.CompactionHealthCapability,
		} {
			if _, present := capabilities[capability]; present {
				invalidated = append(invalidated, capability)
			}
		}
		if len(invalidated) > 0 {
			compaction, _, compactionErr := agenthistory.CompactionStateFrom(capabilities)
			slog.InfoContext(ctx, "invalidating Agent history-dependent capabilities after canonical history changed",
				"session_namespace", request.Session.Namespace, "session_id", request.Session.ID,
				"reason", "canonical_history_prefix_mismatch", "capabilities", invalidated,
				"compaction_id", compaction.ID, "compaction_decode_error", compactionErr,
				"message_count", len(ordered), "previous_message_count", currentMessageCount,
				"had_current_transcript", hadCurrentTranscript,
				"checkpoint_message_count", request.Checkpoint.MessageCount, "had_checkpoint", hadCheckpoint)
			for _, capability := range invalidated {
				delete(capabilities, capability)
			}
			result.Invalidated = invalidated
		}
	}
	next := engineTranscript{Version: engineTranscriptVersion, Messages: agentschema.CloneMessages(ordered), ContextState: contextState}
	if request.Active && len(request.Checkpoint.Metadata) != 0 {
		if err := json.Unmarshal(request.Checkpoint.Metadata, &next); err != nil {
			return CanonicalImport{}, err
		}
		next.Archive = nil
		next.Messages = agentschema.CloneMessages(ordered)
		if outputCommitted {
			next.ActiveModelUser, next.ActiveUserIndex = nil, 0
			next.ContextState = contextState
		} else {
			next.Messages = append(next.Messages, agentschema.CloneMessages(request.Checkpoint.Pending)...)
		}
	}
	next.HistoryHead = request.Head
	if request.Canonical && request.Head.Identity != "" {
		compact, present, compactErr := agenthistory.CompactionStateFrom(capabilities)
		if compactErr != nil {
			return CanonicalImport{}, compactErr
		}
		clear, clearPresent, clearErr := agenthistory.ClearStateFrom(capabilities)
		if clearErr != nil {
			return CanonicalImport{}, clearErr
		}
		compact, present = agenthistory.ClearCompaction(compact, present, clear, clearPresent)
		if present {
			next.Messages, next.Archive = agenthistory.ArchiveHistory(next.Messages, nil, compact, next.ContextState)
			next.Version = transcriptVersion(next.Archive)
		}
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return CanonicalImport{}, fmt.Errorf("encode canonical Agent messages: %w", err)
	}
	if _, err := decodeEngineTranscript(encoded); err != nil {
		return CanonicalImport{}, err
	}
	result.State = encoded
	result.Persist = request.Canonical || !currentMatches
	result.Checkpoint, err = CanonicalMessageCheckpoint(encoded)
	if err != nil {
		return CanonicalImport{}, err
	}

	for _, message := range ordered {
		if message == nil || message.TaskCompletion == nil {
			continue
		}
		if id := strings.TrimSpace(message.TaskCompletion.CompletionID); id != "" {
			result.CompletionIDs = append(result.CompletionIDs, id)
		}
	}
	return result, nil
}
