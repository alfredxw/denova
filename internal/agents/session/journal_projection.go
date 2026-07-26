package session

import (
	"bytes"
	"crypto/sha256"
	"encoding"
	"encoding/json"
	"fmt"
	"hash"
	"sort"
	"strings"
	"time"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/conversationjournal"
)

const (
	sessionProjectionVersion      = 7
	sessionRecentTransactionLimit = 200
	sessionRecentCommitLimit      = 200
	sessionStructuralRecordLimit  = 64
	sessionHistoryAnchorEvery     = 256
)

// historyAnchor maps a stable visible-history position to the physical
// transaction containing the next row. Anchors contain no user content and
// make backwards paging independent from the total journal size.
type historyAnchor struct {
	Before int                        `json:"before"`
	Cursor conversationjournal.Cursor `json:"cursor"`
}

type messageLocator struct {
	Index  int                        `json:"index"`
	Cursor conversationjournal.Cursor `json:"cursor"`
}

type domainCommitLocator struct {
	MessageIndex int                        `json:"message_index"`
	Cursor       conversationjournal.Cursor `json:"cursor"`
	Role         agent.RoleType             `json:"role"`
	Metadata     MessageMetadata            `json:"metadata"`
}

type structuralProjectionRecord struct {
	Cursor     conversationjournal.Cursor `json:"cursor"`
	Compaction *ContextCompaction         `json:"compaction,omitempty"`
	Removal    *ContextCompactionRemoval  `json:"removal,omitempty"`
}

// assistantRunCheckpoint stores only a resumable SHA-256 state. It lets an
// index checkpoint taken mid-stream determine whether the later canonical
// assistant message is already represented by display segments, without ever
// putting assistant prose in the sidecar.
type assistantRunCheckpoint struct {
	RunID string `json:"run_id"`
	State []byte `json:"state"`
}

type assistantTargetCheckpoint struct {
	RecordID string `json:"record_id"`
	RunID    string `json:"run_id"`
}

// sessionJournalProjection is the bounded, rebuildable domain section of the
// conversation index. It deliberately stores locators and current state only;
// the canonical JSONL remains the sole source of transcript and display text.
type sessionJournalProjection struct {
	Version             int                        `json:"version"`
	SessionID           string                     `json:"session_id"`
	Generation          string                     `json:"generation"`
	Title               string                     `json:"title"`
	CreatedAt           time.Time                  `json:"created_at"`
	UpdatedAt           time.Time                  `json:"updated_at"`
	MessageCount        int                        `json:"message_count"`
	VisibleMessageCount int                        `json:"visible_message_count"`
	HistoryCount        int                        `json:"history_count"`
	ClearAfter          int                        `json:"clear_after"`
	ClearCursor         conversationjournal.Cursor `json:"clear_cursor,omitempty"`
	ContextRevision     uint64                     `json:"context_revision"`

	RecentCursors                  []conversationjournal.Cursor   `json:"recent_cursors,omitempty"`
	MessageLocators                []messageLocator               `json:"message_locators,omitempty"`
	MessageAnchors                 []messageLocator               `json:"message_anchors,omitempty"`
	HistoryAnchors                 []historyAnchor                `json:"history_anchors,omitempty"`
	RecentCommits                  []domainCommitLocator          `json:"recent_commits,omitempty"`
	Structural                     []structuralProjectionRecord   `json:"structural,omitempty"`
	PendingInterrupt               *Interruption                  `json:"pending_interrupt,omitempty"`
	PendingInterruptCursor         conversationjournal.Cursor     `json:"pending_interrupt_cursor,omitempty"`
	PendingAsk                     *AskInteraction                `json:"pending_ask,omitempty"`
	PendingAskCursor               conversationjournal.Cursor     `json:"pending_ask_cursor,omitempty"`
	ContextWindows                 []agentContextWindowProjection `json:"context_windows,omitempty"`
	ContextWindowProjectionInvalid bool                           `json:"context_window_projection_invalid,omitempty"`
	AssistantRuns                  []assistantRunCheckpoint       `json:"active_assistant_runs,omitempty"`
	AssistantTargets               []assistantTargetCheckpoint    `json:"active_assistant_targets,omitempty"`

	expectedID         string
	expectedGeneration string
	lastCursor         conversationjournal.Cursor
	assistantDigests   map[string]hash.Hash
	assistantTargets   map[string]string
}

func newSessionJournalProjection(id, generation string) *sessionJournalProjection {
	projection := &sessionJournalProjection{expectedID: strings.TrimSpace(id), expectedGeneration: strings.TrimSpace(generation)}
	_ = projection.Reset()
	return projection
}

func (projection *sessionJournalProjection) Reset() error {
	expectedID := projection.expectedID
	expectedGeneration := projection.expectedGeneration
	*projection = sessionJournalProjection{
		Version: sessionProjectionVersion, SessionID: expectedID, Generation: expectedGeneration,
		Title: defaultSessionTitle, expectedID: expectedID, expectedGeneration: expectedGeneration,
		assistantDigests: make(map[string]hash.Hash), assistantTargets: make(map[string]string),
	}
	return nil
}

func (projection *sessionJournalProjection) Restore(data json.RawMessage) error {
	expectedID := projection.expectedID
	expectedGeneration := projection.expectedGeneration
	var restored sessionJournalProjection
	if err := json.Unmarshal(data, &restored); err != nil {
		return err
	}
	if restored.Version != sessionProjectionVersion {
		return fmt.Errorf("unsupported session projection version %d", restored.Version)
	}
	if strings.TrimSpace(restored.SessionID) != expectedID || strings.TrimSpace(restored.Generation) != expectedGeneration {
		return fmt.Errorf("session projection identity mismatch")
	}
	restored.expectedID = expectedID
	restored.expectedGeneration = expectedGeneration
	if len(restored.RecentCursors) > 0 {
		restored.lastCursor = restored.RecentCursors[len(restored.RecentCursors)-1]
	}
	if err := restored.validateContextWindows(); err != nil {
		return fmt.Errorf("restore context window projection: %w", err)
	}
	if err := restored.restoreAssistantDigests(); err != nil {
		return err
	}
	*projection = restored
	return nil
}

func (projection *sessionJournalProjection) Checkpoint() (json.RawMessage, error) {
	if err := projection.validateContextWindows(); err != nil {
		return nil, fmt.Errorf("checkpoint context window projection: %w", err)
	}
	checkpoint := *projection
	if err := checkpoint.captureAssistantDigests(); err != nil {
		return nil, err
	}
	if projection.PendingInterrupt != nil {
		pending := *projection.PendingInterrupt
		pending.UserMessage = ""
		pending.AssistantContent = ""
		pending.Reason = ""
		checkpoint.PendingInterrupt = &pending
	}
	if projection.PendingAsk != nil {
		pending := cloneAskInteraction(*projection.PendingAsk)
		// The canonical journal owns model-generated question text. The sidecar
		// needs only enough identity to locate that pending record during reload.
		pending.Questions = nil
		pending.Answers = nil
		checkpoint.PendingAsk = &pending
	}
	return json.Marshal(&checkpoint)
}

func (projection *sessionJournalProjection) Apply(record conversationjournal.Record) error {
	projection.rememberCursor(record.Location.Cursor)
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(record.Payload, &typed); err != nil {
		return err
	}
	if record.Location.Cursor == 1 && typed.Type == "session" {
		return projection.applyHeader(record.Payload)
	}
	switch typed.Type {
	case "":
		return projection.applyLegacyMessage(record)
	case historyTypeMessage, historyTypeContextMessage:
		return projection.applyMessage(record, typed.Type)
	case historyTypeClear:
		var marker clearRecord
		if err := json.Unmarshal(record.Payload, &marker); err != nil {
			return err
		}
		projection.clearAssistantDigests()
		projection.rememberHistoryRow(record.Location.Cursor, true)
		projection.ClearAfter = projection.MessageCount
		projection.ClearCursor = record.Location.Cursor
		projection.advanceRevision(marker.ContextRevision)
		projection.advanceUpdatedAt(marker.CreatedAt)
		// A checkpoint before /clear cannot affect the new effective context.
		projection.Structural = nil
		projection.resetContextWindows()
		return nil
	case historyTypeDisplay:
		var display displayRecord
		if err := json.Unmarshal(record.Payload, &display); err != nil {
			return err
		}
		if strings.TrimSpace(display.Role) == "" {
			return fmt.Errorf("display record role is empty")
		}
		projection.rememberAssistantDisplay(display)
		projection.rememberHistoryRow(record.Location.Cursor, false)
		projection.advanceUpdatedAt(display.CreatedAt)
		return nil
	case historyTypeSessionPatch:
		var patch sessionPatchRecord
		if err := json.Unmarshal(record.Payload, &patch); err != nil {
			return err
		}
		if patch.Title != nil {
			title := strings.TrimSpace(*patch.Title)
			if title == "" {
				return fmt.Errorf("session patch title is empty")
			}
			projection.Title = title
		}
		projection.advanceUpdatedAt(patch.UpdatedAt)
		return nil
	case historyTypeDisplayPatch:
		var patch displayPatchRecord
		if err := json.Unmarshal(record.Payload, &patch); err != nil {
			return err
		}
		projection.appendAssistantDisplayPatch(patch)
		projection.advanceUpdatedAt(patch.CreatedAt)
		return nil
	case historyTypeInterrupt:
		var marker interruptionRecord
		if err := json.Unmarshal(record.Payload, &marker); err != nil {
			return err
		}
		interrupt := marker.Interruption
		if strings.TrimSpace(interrupt.Status) == "" {
			interrupt.Status = InterruptionPending
		}
		if interrupt.Status == InterruptionPending {
			projection.PendingInterrupt = &interrupt
			projection.PendingInterruptCursor = record.Location.Cursor
		}
		projection.advanceUpdatedAt(interrupt.CreatedAt)
		return nil
	case historyTypeInterruptionPatch:
		var patch interruptionPatchRecord
		if err := json.Unmarshal(record.Payload, &patch); err != nil {
			return err
		}
		if projection.PendingInterrupt != nil && projection.PendingInterrupt.ID == patch.TargetID {
			projection.PendingInterrupt.Status = patch.Status
			projection.PendingInterrupt.ResolvedAt = patch.ResolvedAt
			if patch.Status != InterruptionPending {
				projection.PendingInterrupt = nil
				projection.PendingInterruptCursor = 0
			}
		}
		projection.advanceUpdatedAt(patch.UpdatedAt)
		return nil
	case historyTypeAsk:
		var marker askRecord
		if err := json.Unmarshal(record.Payload, &marker); err != nil {
			return err
		}
		interaction, err := normalizeAskInteraction(marker.AskInteraction)
		if err != nil {
			return err
		}
		projection.PendingAsk = &interaction
		projection.PendingAskCursor = record.Location.Cursor
		projection.rememberHistoryRow(record.Location.Cursor, false)
		projection.advanceUpdatedAt(interaction.CreatedAt)
		return nil
	case historyTypeAskPatch:
		var patch askPatchRecord
		if err := json.Unmarshal(record.Payload, &patch); err != nil {
			return err
		}
		if projection.PendingAsk == nil || projection.PendingAsk.ID != patch.TargetID {
			return fmt.Errorf("ask patch target does not match the pending interaction: %s", patch.TargetID)
		}
		projection.PendingAsk = nil
		projection.PendingAskCursor = 0
		projection.advanceUpdatedAt(patch.UpdatedAt)
		return nil
	case historyTypeCompaction:
		var compaction ContextCompaction
		if err := json.Unmarshal(record.Payload, &compaction); err != nil {
			return err
		}
		compaction.Type = historyTypeCompaction
		if compaction.SourceStartCursor == 0 && compaction.SourceStartIndex < compaction.SourceEndIndex {
			compaction.SourceStartCursor = projection.messageCursorAt(compaction.SourceStartIndex)
		}
		if compaction.SourceEndCursor == 0 && compaction.SourceEndIndex > 0 {
			compaction.SourceEndCursor = projection.messageCursorAt(compaction.SourceEndIndex - 1)
		}
		projection.rememberStructural(structuralProjectionRecord{Cursor: record.Location.Cursor, Compaction: &compaction})
		projection.advanceRevision(compaction.ContextRevision)
		projection.advanceUpdatedAt(compaction.CreatedAt)
		return nil
	case historyTypeCompactionRemoved:
		var removal ContextCompactionRemoval
		if err := json.Unmarshal(record.Payload, &removal); err != nil {
			return err
		}
		removal.Type = historyTypeCompactionRemoved
		projection.rememberStructural(structuralProjectionRecord{Cursor: record.Location.Cursor, Removal: &removal})
		projection.advanceRevision(removal.ContextRevision)
		projection.advanceUpdatedAt(removal.CreatedAt)
		return nil
	case "session":
		return fmt.Errorf("session header can only be the first record")
	default:
		return fmt.Errorf("unknown session journal record type %q", typed.Type)
	}
}

func (projection *sessionJournalProjection) applyHeader(payload json.RawMessage) error {
	var header sessionHeader
	if err := json.Unmarshal(payload, &header); err != nil {
		return err
	}
	header.ID = firstNonEmpty(header.ID, projection.expectedID)
	if header.ID != projection.expectedID {
		return fmt.Errorf("session header id mismatch: have=%q want=%q", header.ID, projection.expectedID)
	}
	generation := sessionHeaderIncarnation(header)
	if generation != projection.expectedGeneration {
		return fmt.Errorf("session header generation mismatch")
	}
	projection.SessionID = header.ID
	projection.Generation = generation
	projection.CreatedAt = header.CreatedAt
	projection.UpdatedAt = header.UpdatedAt
	if projection.UpdatedAt.IsZero() {
		projection.UpdatedAt = projection.CreatedAt
	}
	if title := strings.TrimSpace(header.Title); title != "" {
		projection.Title = title
	}
	return nil
}

func (projection *sessionJournalProjection) applyMessage(record conversationjournal.Record, kind string) error {
	var message messageRecord
	if err := json.Unmarshal(record.Payload, &message); err != nil {
		return err
	}
	if message.Message.Role == "" && message.Message.Content == "" && len(message.Message.ToolCalls) == 0 {
		return fmt.Errorf("message record is empty")
	}
	metadata := sanitizeMessageMetadata(message.MessageMetadata)
	messageIndex := projection.MessageCount
	projection.rememberMessage(record.Location.Cursor, message.Message.Role, metadata)
	projection.rememberContextOperations(record.Location, messageIndex, metadata.ContextRevision, metadata.ContextOperations)
	if kind == historyTypeMessage {
		projection.VisibleMessageCount++
		visible := true
		safeBoundary := message.Message.Role == agent.User
		if safeBoundary {
			projection.clearAssistantDigests()
		} else if message.Message.Role == agent.Assistant && !metadata.SubAgent {
			visible = !projection.consumeCompleteAssistantRun(metadata.RunID, message.Message.Content)
		}
		if visible {
			projection.rememberHistoryRow(record.Location.Cursor, safeBoundary)
		}
	}
	if projection.Title == defaultSessionTitle && message.Message.Role == agent.User && strings.TrimSpace(message.Message.Content) != "" {
		projection.Title = deriveTitle(message.Message.Content)
	}
	projection.advanceRevision(metadata.ContextRevision)
	projection.advanceUpdatedAt(message.CreatedAt)
	return nil
}

func (projection *sessionJournalProjection) applyLegacyMessage(record conversationjournal.Record) error {
	var message agent.Message
	if err := json.Unmarshal(record.Payload, &message); err != nil {
		return err
	}
	if message.Role == "" && message.Content == "" && len(message.ToolCalls) == 0 {
		return fmt.Errorf("legacy session message is empty")
	}
	projection.rememberMessage(record.Location.Cursor, message.Role, MessageMetadata{})
	projection.VisibleMessageCount++
	safeBoundary := message.Role == agent.User
	if safeBoundary {
		projection.clearAssistantDigests()
	}
	projection.rememberHistoryRow(record.Location.Cursor, safeBoundary)
	if projection.Title == defaultSessionTitle && message.Role == agent.User && strings.TrimSpace(message.Content) != "" {
		projection.Title = deriveTitle(message.Content)
	}
	projection.advanceRevision(0)
	return nil
}

func (projection *sessionJournalProjection) rememberCursor(cursor conversationjournal.Cursor) {
	if cursor == 0 || cursor == projection.lastCursor {
		return
	}
	projection.lastCursor = cursor
	projection.RecentCursors = append(projection.RecentCursors, cursor)
	if overflow := len(projection.RecentCursors) - sessionRecentTransactionLimit; overflow > 0 {
		projection.RecentCursors = append([]conversationjournal.Cursor(nil), projection.RecentCursors[overflow:]...)
	}
}

func (projection *sessionJournalProjection) rememberMessage(cursor conversationjournal.Cursor, role agent.RoleType, metadata MessageMetadata) {
	index := projection.MessageCount
	projection.MessageCount++
	if index%sessionHistoryAnchorEvery == 0 {
		projection.MessageAnchors = append(projection.MessageAnchors, messageLocator{Index: index, Cursor: cursor})
	}
	projection.MessageLocators = append(projection.MessageLocators, messageLocator{Index: index, Cursor: cursor})
	if overflow := len(projection.MessageLocators) - sessionRecentTransactionLimit; overflow > 0 {
		projection.MessageLocators = append([]messageLocator(nil), projection.MessageLocators[overflow:]...)
	}
	if metadata.AgentCommandID == "" {
		return
	}
	projection.RecentCommits = append(projection.RecentCommits, domainCommitLocator{
		MessageIndex: index, Cursor: cursor, Role: role, Metadata: metadata,
	})
	if overflow := len(projection.RecentCommits) - sessionRecentCommitLimit; overflow > 0 {
		projection.RecentCommits = append([]domainCommitLocator(nil), projection.RecentCommits[overflow:]...)
	}
}

func (projection *sessionJournalProjection) rememberHistoryRow(cursor conversationjournal.Cursor, safeBoundary bool) {
	if len(projection.HistoryAnchors) == 0 || (safeBoundary && projection.HistoryCount-projection.HistoryAnchors[len(projection.HistoryAnchors)-1].Before >= sessionHistoryAnchorEvery) {
		projection.HistoryAnchors = append(projection.HistoryAnchors, historyAnchor{Before: projection.HistoryCount, Cursor: cursor})
	}
	projection.HistoryCount++
}

func (projection *sessionJournalProjection) rememberAssistantDisplay(display displayRecord) {
	runID := strings.TrimSpace(display.RunID)
	if display.Role != "assistant" || display.SubAgent || runID == "" {
		return
	}
	digest := projection.assistantDigest(runID)
	_, _ = digest.Write([]byte(display.Content))
	if recordID := strings.TrimSpace(display.RecordID); recordID != "" {
		projection.assistantTargets[recordID] = runID
	}
}

func (projection *sessionJournalProjection) appendAssistantDisplayPatch(patch displayPatchRecord) {
	if patch.ContentAppend == "" {
		return
	}
	runID := projection.assistantTargets[strings.TrimSpace(patch.TargetRecordID)]
	if runID == "" {
		return
	}
	_, _ = projection.assistantDigest(runID).Write([]byte(patch.ContentAppend))
}

func (projection *sessionJournalProjection) consumeCompleteAssistantRun(runID, content string) bool {
	runID = strings.TrimSpace(runID)
	digest := projection.assistantDigests[runID]
	if runID == "" || digest == nil {
		return false
	}
	want := sha256.Sum256([]byte(content))
	complete := bytes.Equal(digest.Sum(nil), want[:])
	delete(projection.assistantDigests, runID)
	for recordID, targetRunID := range projection.assistantTargets {
		if targetRunID == runID {
			delete(projection.assistantTargets, recordID)
		}
	}
	return complete
}

func (projection *sessionJournalProjection) assistantDigest(runID string) hash.Hash {
	if projection.assistantDigests == nil {
		projection.assistantDigests = make(map[string]hash.Hash)
	}
	if projection.assistantTargets == nil {
		projection.assistantTargets = make(map[string]string)
	}
	if digest := projection.assistantDigests[runID]; digest != nil {
		return digest
	}
	digest := sha256.New()
	projection.assistantDigests[runID] = digest
	return digest
}

func (projection *sessionJournalProjection) clearAssistantDigests() {
	projection.assistantDigests = make(map[string]hash.Hash)
	projection.assistantTargets = make(map[string]string)
}

func (projection *sessionJournalProjection) captureAssistantDigests() error {
	projection.AssistantRuns = make([]assistantRunCheckpoint, 0, len(projection.assistantDigests))
	runIDs := make([]string, 0, len(projection.assistantDigests))
	for runID := range projection.assistantDigests {
		runIDs = append(runIDs, runID)
	}
	sort.Strings(runIDs)
	for _, runID := range runIDs {
		marshaler, ok := projection.assistantDigests[runID].(encoding.BinaryMarshaler)
		if !ok {
			return fmt.Errorf("assistant digest for run %q cannot be checkpointed", runID)
		}
		state, err := marshaler.MarshalBinary()
		if err != nil {
			return fmt.Errorf("checkpoint assistant digest for run %q: %w", runID, err)
		}
		projection.AssistantRuns = append(projection.AssistantRuns, assistantRunCheckpoint{RunID: runID, State: state})
	}
	projection.AssistantTargets = make([]assistantTargetCheckpoint, 0, len(projection.assistantTargets))
	recordIDs := make([]string, 0, len(projection.assistantTargets))
	for recordID := range projection.assistantTargets {
		recordIDs = append(recordIDs, recordID)
	}
	sort.Strings(recordIDs)
	for _, recordID := range recordIDs {
		projection.AssistantTargets = append(projection.AssistantTargets, assistantTargetCheckpoint{RecordID: recordID, RunID: projection.assistantTargets[recordID]})
	}
	return nil
}

func (projection *sessionJournalProjection) restoreAssistantDigests() error {
	projection.assistantDigests = make(map[string]hash.Hash, len(projection.AssistantRuns))
	projection.assistantTargets = make(map[string]string, len(projection.AssistantTargets))
	for _, checkpoint := range projection.AssistantRuns {
		runID := strings.TrimSpace(checkpoint.RunID)
		if runID == "" {
			return fmt.Errorf("assistant digest checkpoint has empty run id")
		}
		digest := sha256.New()
		unmarshaler, ok := digest.(encoding.BinaryUnmarshaler)
		if !ok {
			return fmt.Errorf("assistant digest for run %q cannot be restored", runID)
		}
		if err := unmarshaler.UnmarshalBinary(checkpoint.State); err != nil {
			return fmt.Errorf("restore assistant digest for run %q: %w", runID, err)
		}
		projection.assistantDigests[runID] = digest
	}
	for _, checkpoint := range projection.AssistantTargets {
		recordID := strings.TrimSpace(checkpoint.RecordID)
		runID := strings.TrimSpace(checkpoint.RunID)
		if recordID == "" || projection.assistantDigests[runID] == nil {
			return fmt.Errorf("assistant target checkpoint is inconsistent")
		}
		projection.assistantTargets[recordID] = runID
	}
	return nil
}

func (projection *sessionJournalProjection) rememberStructural(record structuralProjectionRecord) {
	projection.Structural = append(projection.Structural, record)
	if overflow := len(projection.Structural) - sessionStructuralRecordLimit; overflow > 0 {
		projection.Structural = append([]structuralProjectionRecord(nil), projection.Structural[overflow:]...)
	}
}

func (projection *sessionJournalProjection) advanceRevision(persisted uint64) {
	if persisted > projection.ContextRevision {
		projection.ContextRevision = persisted
		return
	}
	projection.ContextRevision++
}

func (projection *sessionJournalProjection) advanceUpdatedAt(candidate time.Time) {
	if candidate.After(projection.UpdatedAt) {
		projection.UpdatedAt = candidate
	}
}

func (projection *sessionJournalProjection) recentStartCursor() conversationjournal.Cursor {
	if len(projection.RecentCursors) == 0 {
		return 1
	}
	return projection.RecentCursors[0]
}

func (projection *sessionJournalProjection) messageBaseForCursor(cursor conversationjournal.Cursor) int {
	for _, locator := range projection.MessageLocators {
		if locator.Cursor >= cursor {
			return locator.Index
		}
	}
	return projection.MessageCount
}

func (projection *sessionJournalProjection) messageCursorAt(index int) conversationjournal.Cursor {
	for _, locator := range projection.MessageLocators {
		if locator.Index == index {
			return locator.Cursor
		}
	}
	for _, locator := range projection.MessageAnchors {
		if locator.Index == index {
			return locator.Cursor
		}
	}
	return 0
}

func (projection *sessionJournalProjection) messageAnchorAt(index int) messageLocator {
	anchor := messageLocator{Index: 0, Cursor: 1}
	for _, candidate := range projection.MessageAnchors {
		if candidate.Index > index {
			break
		}
		anchor = candidate
	}
	return anchor
}
