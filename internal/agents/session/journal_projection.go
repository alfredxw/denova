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

	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/conversationjournal"
)

const (
	sessionProjectionVersion      = 17
	sessionRecentTransactionLimit = 200
	sessionRecentCommitLimit      = 200
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
	Index       int                        `json:"index"`
	Cursor      conversationjournal.Cursor `json:"cursor"`
	RecordIndex int                        `json:"record_index,omitempty"`
}

type domainCommitLocator struct {
	MessageIndex int                        `json:"message_index"`
	Cursor       conversationjournal.Cursor `json:"cursor"`
	Role         agent.RoleType             `json:"role"`
	Metadata     MessageMetadata            `json:"metadata"`
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
	State    []byte `json:"state"`
}

// sessionJournalProjection is the bounded, rebuildable domain section of the
// conversation index. It deliberately stores locators and current state only;
// the canonical JSONL remains the sole source of transcript and display text.
type sessionJournalProjection struct {
	Version               int                        `json:"version"`
	SessionID             string                     `json:"session_id"`
	Generation            string                     `json:"generation"`
	Title                 string                     `json:"title"`
	CreatedAt             time.Time                  `json:"created_at"`
	UpdatedAt             time.Time                  `json:"updated_at"`
	MessageCount          int                        `json:"message_count"`
	VisibleMessageCount   int                        `json:"visible_message_count"`
	HistoryCount          int                        `json:"history_count"`
	ClearAfter            int                        `json:"clear_after"`
	ClearCursor           conversationjournal.Cursor `json:"clear_cursor,omitempty"`
	ContextRevision       uint64                     `json:"context_revision"`
	RuntimeConfig         *conversationconfig.Config `json:"runtime_config,omitempty"`
	RuntimeConfigRevision uint64                     `json:"runtime_config_revision,omitempty"`

	RecentCursors              []conversationjournal.Cursor `json:"recent_cursors,omitempty"`
	MessageLocators            []messageLocator             `json:"message_locators,omitempty"`
	MessageTransactionLocators []messageLocator             `json:"message_transaction_locators,omitempty"`
	HistoryAnchors             []historyAnchor              `json:"history_anchors,omitempty"`
	RecentCommits              []domainCommitLocator        `json:"recent_commits,omitempty"`
	PendingInterrupt           *Interruption                `json:"pending_interrupt,omitempty"`
	PendingInterruptCursor     conversationjournal.Cursor   `json:"pending_interrupt_cursor,omitempty"`
	AssistantRuns              []assistantRunCheckpoint     `json:"active_assistant_runs,omitempty"`
	AssistantTargets           []assistantTargetCheckpoint  `json:"active_assistant_targets,omitempty"`

	expectedID         string
	expectedGeneration string
	lastCursor         conversationjournal.Cursor
	assistantDigests   map[string]hash.Hash
	assistantTargets   map[string]string
	assistantSegments  map[string]hash.Hash
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
		assistantDigests: make(map[string]hash.Hash), assistantTargets: make(map[string]string), assistantSegments: make(map[string]hash.Hash),
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
		return fmt.Errorf("%w: unsupported session projection version %d", conversationjournal.ErrProjectionCheckpointIncompatible, restored.Version)
	}
	if strings.TrimSpace(restored.SessionID) != expectedID || strings.TrimSpace(restored.Generation) != expectedGeneration {
		return fmt.Errorf("session projection identity mismatch")
	}
	if err := validateRuntimeConfigState(restored.RuntimeConfig, restored.RuntimeConfigRevision, ""); err != nil {
		return fmt.Errorf("restore session runtime config: %w", err)
	}
	restored.expectedID = expectedID
	restored.expectedGeneration = expectedGeneration
	if len(restored.RecentCursors) > 0 {
		restored.lastCursor = restored.RecentCursors[len(restored.RecentCursors)-1]
	}
	if err := restored.restoreAssistantDigests(); err != nil {
		return err
	}
	*projection = restored
	return nil
}

func (projection *sessionJournalProjection) Checkpoint() (json.RawMessage, error) {
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
		if patch.RuntimeConfig != nil {
			if patch.RuntimeConfigRevision == 0 || patch.RuntimeConfigRevision <= projection.RuntimeConfigRevision {
				return fmt.Errorf("session runtime config revision is not monotonic")
			}
			expectedKind := ""
			if projection.RuntimeConfig != nil {
				expectedKind = projection.RuntimeConfig.AgentKind
			}
			if err := validateRuntimeConfigState(patch.RuntimeConfig, patch.RuntimeConfigRevision, expectedKind); err != nil {
				return fmt.Errorf("session runtime config: %w", err)
			}
			value := *patch.RuntimeConfig
			projection.RuntimeConfig = &value
			projection.RuntimeConfigRevision = patch.RuntimeConfigRevision
		}
		if patch.RuntimeConfig == nil && patch.RuntimeConfigRevision != 0 {
			return fmt.Errorf("session runtime config revision exists without a config")
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
	case "session":
		return fmt.Errorf("session header can only be the first record")
	default:
		if isRetiredSessionJournalRecordType(typed.Type) {
			return nil
		}
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
	if header.RuntimeConfig != nil {
		if header.RuntimeConfigRevision != 1 {
			return fmt.Errorf("session header runtime config revision must be 1")
		}
		if err := validateRuntimeConfigState(header.RuntimeConfig, header.RuntimeConfigRevision, ""); err != nil {
			return fmt.Errorf("session header runtime config: %w", err)
		}
		value := *header.RuntimeConfig
		projection.RuntimeConfig = &value
		projection.RuntimeConfigRevision = header.RuntimeConfigRevision
	} else if header.RuntimeConfigRevision != 0 {
		return fmt.Errorf("session header runtime config revision exists without a config")
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
	projection.rememberMessage(record.Location, message.Message.Role, metadata)
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
	projection.rememberMessage(record.Location, message.Role, MessageMetadata{})
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

func (projection *sessionJournalProjection) rememberMessage(location conversationjournal.Location, role agent.RoleType, metadata MessageMetadata) {
	index := projection.MessageCount
	projection.MessageCount++
	locator := messageLocator{Index: index, Cursor: location.Cursor, RecordIndex: location.RecordIndex}
	if len(projection.MessageTransactionLocators) == 0 || projection.MessageTransactionLocators[len(projection.MessageTransactionLocators)-1].Cursor != location.Cursor {
		projection.MessageTransactionLocators = append(projection.MessageTransactionLocators, locator)
		if overflow := len(projection.MessageTransactionLocators) - sessionRecentTransactionLimit; overflow > 0 {
			projection.MessageTransactionLocators = append([]messageLocator(nil), projection.MessageTransactionLocators[overflow:]...)
		}
	}
	projection.MessageLocators = append(projection.MessageLocators, locator)
	if overflow := len(projection.MessageLocators) - sessionRecentTransactionLimit; overflow > 0 {
		projection.MessageLocators = append([]messageLocator(nil), projection.MessageLocators[overflow:]...)
	}
	if metadata.AgentCommandID == "" {
		return
	}
	projection.RecentCommits = append(projection.RecentCommits, domainCommitLocator{
		MessageIndex: index, Cursor: location.Cursor, Role: role, Metadata: metadata,
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
		segment := sha256.New()
		_, _ = segment.Write([]byte(display.Content))
		projection.assistantSegments[recordID] = segment
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
	if segment := projection.assistantSegments[strings.TrimSpace(patch.TargetRecordID)]; segment != nil {
		_, _ = segment.Write([]byte(patch.ContentAppend))
	}
}

func (projection *sessionJournalProjection) consumeCompleteAssistantRun(runID, content string) bool {
	runID = strings.TrimSpace(runID)
	digest := projection.assistantDigests[runID]
	if runID == "" || digest == nil {
		return false
	}
	want := sha256.Sum256([]byte(content))
	complete := bytes.Equal(digest.Sum(nil), want[:])
	if !complete {
		for recordID, targetRunID := range projection.assistantTargets {
			if targetRunID != runID {
				continue
			}
			if segment := projection.assistantSegments[recordID]; segment != nil && bytes.Equal(segment.Sum(nil), want[:]) {
				complete = true
				break
			}
		}
	}
	delete(projection.assistantDigests, runID)
	for recordID, targetRunID := range projection.assistantTargets {
		if targetRunID == runID {
			delete(projection.assistantTargets, recordID)
			delete(projection.assistantSegments, recordID)
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
	projection.assistantSegments = make(map[string]hash.Hash)
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
		segment := projection.assistantSegments[recordID]
		marshaler, ok := segment.(encoding.BinaryMarshaler)
		if !ok {
			return fmt.Errorf("assistant segment digest for record %q cannot be checkpointed", recordID)
		}
		state, err := marshaler.MarshalBinary()
		if err != nil {
			return fmt.Errorf("checkpoint assistant segment digest for record %q: %w", recordID, err)
		}
		projection.AssistantTargets = append(projection.AssistantTargets, assistantTargetCheckpoint{
			RecordID: recordID, RunID: projection.assistantTargets[recordID], State: state,
		})
	}
	return nil
}

func (projection *sessionJournalProjection) restoreAssistantDigests() error {
	projection.assistantDigests = make(map[string]hash.Hash, len(projection.AssistantRuns))
	projection.assistantTargets = make(map[string]string, len(projection.AssistantTargets))
	projection.assistantSegments = make(map[string]hash.Hash, len(projection.AssistantTargets))
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
		digest := sha256.New()
		unmarshaler, ok := digest.(encoding.BinaryUnmarshaler)
		if !ok {
			return fmt.Errorf("assistant segment digest for record %q cannot be restored", recordID)
		}
		if err := unmarshaler.UnmarshalBinary(checkpoint.State); err != nil {
			return fmt.Errorf("restore assistant segment digest for record %q: %w", recordID, err)
		}
		projection.assistantTargets[recordID] = runID
		projection.assistantSegments[recordID] = digest
	}
	return nil
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

// messageTransactionsBefore returns retained canonical message transactions
// that precede the mixed-event resident window. A long Agent turn can emit
// hundreds of display transactions between two canonical messages; those
// messages must remain resident even after the display prefix is paged out.
func (projection *sessionJournalProjection) messageTransactionsBefore(cursor conversationjournal.Cursor) []messageLocator {
	end := 0
	for end < len(projection.MessageTransactionLocators) && projection.MessageTransactionLocators[end].Cursor < cursor {
		end++
	}
	return projection.MessageTransactionLocators[:end]
}

func (projection *sessionJournalProjection) messageBaseForCursor(cursor conversationjournal.Cursor) int {
	for _, locator := range projection.MessageTransactionLocators {
		if locator.Cursor >= cursor {
			return locator.Index
		}
	}
	// Rebuilt projections always carry transaction locators. Keep the older
	// logical-locator fallback local so an in-memory projection remains
	// diagnosable if construction stops before its first message transaction.
	for _, locator := range projection.MessageLocators {
		if locator.Cursor >= cursor {
			return locator.Index
		}
	}
	return projection.MessageCount
}
